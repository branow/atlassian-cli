package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/config"
	"github.com/branow/atlassian-cli/internal/confluence"
	"github.com/branow/atlassian-cli/internal/credentials"
	"github.com/branow/atlassian-cli/internal/iostreams"
)

// TestConfluenceAttachUploads drives the attach command against a real
// httptest server. The uploadAttachment seam is redirected to the test
// server's base URL (attachments bypass the JSON client, so the factory's
// server URL does not reach them), exercising the real multipart uploader
// end to end: path, XSRF header, and credentials.
func TestConfluenceAttachUploads(t *testing.T) {
	var gotPath, gotToken string
	var gotFilename string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Atlassian-Token")
		if _, header, err := r.FormFile("file"); err == nil {
			gotFilename = header.Filename
		}
		w.Write([]byte(`{"results":[{"id":"att-1","title":"notes.txt"}]}`))
	}))
	defer server.Close()

	// Redirect the seam to the test server while still using the real
	// multipart uploader.
	original := uploadAttachment
	uploadAttachment = func(ctx context.Context, _ string, creds credentials.Credentials, pageID, file string) ([]byte, error) {
		return confluence.UploadAttachment(ctx, server.URL, creds, pageID, file)
	}
	defer func() { uploadAttachment = original }()

	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, out := attachTestFactory(t)
	root := NewRootCmd(f)
	root.SetArgs([]string{"confluence", "attach", "12345", file})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotPath != "/wiki/rest/api/content/12345/child/attachment" {
		t.Errorf("got path %q", gotPath)
	}
	if gotToken != "no-check" {
		t.Errorf("got X-Atlassian-Token %q, want no-check", gotToken)
	}
	if gotFilename != "notes.txt" {
		t.Errorf("got uploaded filename %q, want notes.txt", gotFilename)
	}
	if !strings.Contains(out.String(), "att-1") {
		t.Errorf("attach output missing attachment id:\n%s", out.String())
	}
}

// attachTestFactory builds a minimal logged-in factory for the internal
// attach test (newTestFactory lives in the external cmd_test package and is
// not visible here).
func attachTestFactory(t *testing.T) (*cmdutil.Factory, *strings.Builder) {
	t.Helper()
	streams, _, _, _ := iostreams.Test()
	out := &strings.Builder{}
	streams.Out = out

	cfg := config.New()
	cfg.SetProfile("default", config.Profile{Site: "acme.atlassian.net", Email: "alice@example.com"})
	store := credentials.NewFakeStore()
	store.Set("default", credentials.Credentials{Site: "acme.atlassian.net", Email: "alice@example.com", APIToken: "t0ken"})

	f := &cmdutil.Factory{IOStreams: streams, Config: cfg, CredentialsStore: store}
	f.ClientFn = func() (atlapi.Client, error) {
		creds, err := store.Get(f.ActiveProfile())
		if err != nil {
			return nil, cmdutil.ErrNotLoggedIn
		}
		return atlapi.New("", creds), nil
	}
	return f, out
}
