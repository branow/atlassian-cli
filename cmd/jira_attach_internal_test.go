package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/branow/atlassian-cli/internal/credentials"
	"github.com/branow/atlassian-cli/internal/jira"
)

// TestJiraIssueAttachUploads drives the attach command against a real
// httptest server. The uploadJiraAttachment seam is redirected to the test
// server's base URL (attachments bypass the JSON client, so the factory's
// server URL does not reach them), exercising the real multipart uploader end
// to end: path, XSRF header, credentials, and the friendly confirmation.
func TestJiraIssueAttachUploads(t *testing.T) {
	var gotMethod, gotPath, gotToken, gotFilename string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Atlassian-Token")
		if _, header, err := r.FormFile("file"); err == nil {
			gotFilename = header.Filename
		}
		w.Write([]byte(`[{"id":"10000","filename":"notes.txt"}]`))
	}))
	defer server.Close()

	// Redirect the seam to the test server while still using the real
	// multipart uploader.
	original := uploadJiraAttachment
	uploadJiraAttachment = func(ctx context.Context, _ string, creds credentials.Credentials, key, file string) ([]byte, error) {
		return jira.UploadAttachment(ctx, server.URL, creds, key, file)
	}
	defer func() { uploadJiraAttachment = original }()

	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, out := attachTestFactory(t)
	root := NewRootCmd(f)
	root.SetArgs([]string{"jira", "issue", "attach", "PROJ-123", file})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("got method %q, want POST", gotMethod)
	}
	if gotPath != "/rest/api/3/issue/PROJ-123/attachments" {
		t.Errorf("got path %q", gotPath)
	}
	if gotToken != "no-check" {
		t.Errorf("got X-Atlassian-Token %q, want no-check", gotToken)
	}
	if gotFilename != "notes.txt" {
		t.Errorf("got uploaded filename %q, want notes.txt", gotFilename)
	}
	if !strings.Contains(out.String(), "Attached notes.txt to PROJ-123 (id 10000)") {
		t.Errorf("attach output missing confirmation:\n%s", out.String())
	}
}
