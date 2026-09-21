package cmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/branow/atlassian-cli/cmd"
	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// runCLI executes the CLI against a stub server, the product-neutral form
// of runJira (attachment downloads exist on both products).
func runCLI(t *testing.T, serverURL string, args ...string) (out string, err error) {
	t.Helper()
	f, outBuf, _ := newTestFactory(t, serverURL)
	root := cmd.NewRootCmd(f)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), err
}

// runIn executes the CLI with the working directory set to dir, so a
// download that defaults to the attachment's filename lands in a temp
// directory rather than the repository.
func runIn(t *testing.T, dir, serverURL string, args ...string) (out string, err error) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })

	return runCLI(t, serverURL, args...)
}

func TestJiraIssueAttachmentListTable(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/issue/PROJ-1": `{"key":"PROJ-1","fields":{"attachment":[{"id":"12345","filename":"report.xlsx","size":133223,"mimeType":"application/vnd.ms-excel","created":"2026-01-02T03:04:05.000+0000","author":{"displayName":"Alice"}}]}}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runCLI(t, server.URL, "jira", "issue", "attachment", "list", "PROJ-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := srv.gotQuery["fields"]; len(got) != 1 || got[0] != "attachment" {
		t.Errorf("list must request only the attachment field, got fields=%v", got)
	}
	for _, want := range []string{"ID", "FILENAME", "12345", "report.xlsx", "133223", "Alice"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestJiraIssueAttachmentDownloadWritesFile covers the whole reported gap:
// the content endpoint 303s to a presigned host, the bytes land in a file
// named after the attachment, and nothing about it goes through the JSON
// decoder.
func TestJiraIssueAttachmentDownloadWritesFile(t *testing.T) {
	presigned := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.ms-excel")
		w.Write([]byte("PK\x03\x04spreadsheet"))
	}))
	defer presigned.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/3/attachment/12345", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"12345","filename":"report.xlsx","size":12,"mimeType":"application/vnd.ms-excel"}`))
	})
	mux.HandleFunc("/rest/api/3/attachment/content/12345", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, presigned.URL+"/media?token=x", http.StatusSeeOther)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	out, err := runIn(t, dir, server.URL, "jira", "issue", "attachment", "download", "12345")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.xlsx"))
	if err != nil {
		t.Fatalf("download must write the attachment's own filename: %v", err)
	}
	if string(data) != "PK\x03\x04spreadsheet" {
		t.Errorf("file holds %q", data)
	}
	if !strings.Contains(out, "report.xlsx") {
		t.Errorf("summary must name the file:\n%s", out)
	}
}

func TestJiraIssueAttachmentDownloadToStdout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/attachment/content/12345" {
			t.Errorf("unexpected request to %s (a stdout download needs no metadata lookup)", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte("a,b\n1,2\n"))
	}))
	defer server.Close()

	out, err := runCLI(t, server.URL, "jira", "issue", "attachment", "download", "12345", "--out", "-")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "a,b\n1,2\n" {
		t.Errorf("stdout = %q, want the raw bytes and nothing else", out)
	}
}

func TestJiraIssueAttachmentDownloadOutNamesTheFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4"))
	}))
	defer server.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "chosen.pdf")
	out, err := runCLI(t, server.URL, "jira", "issue", "attachment", "download", "12345", "--out", target, "-o", "json")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "%PDF-1.4" {
		t.Fatalf("file = %q, err %v", data, err)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("-o json must emit a JSON summary, got %q", out)
	}
	if summary["path"] != target || summary["filename"] != "chosen.pdf" {
		t.Errorf("summary = %v", summary)
	}
}

// TestAttachmentDownloadRefusesSilentOverwrite keeps a download from
// clobbering an existing file in a script, where no prompt can be answered.
func TestAttachmentDownloadRefusesSilentOverwrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("new"))
	}))
	defer server.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := runCLI(t, server.URL, "jira", "issue", "attachment", "download", "12345", "--out", target)
	var validationErr *cmdutil.ValidationError
	if err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("got %v (%T), want a refusal to overwrite", err, validationErr)
	}
	if data, _ := os.ReadFile(target); string(data) != "old" {
		t.Errorf("existing file was overwritten: %q", data)
	}

	if _, err := runCLI(t, server.URL, "jira", "issue", "attachment", "download", "12345", "--out", target, "--force"); err != nil {
		t.Fatalf("--force must overwrite: %v", err)
	}
	if data, _ := os.ReadFile(target); string(data) != "new" {
		t.Errorf("--force left %q", data)
	}
}

func TestConfluenceAttachmentListTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wiki/api/v2/pages/12345/attachments" {
			t.Errorf("got path %s", r.URL.Path)
		}
		w.Write([]byte(`{"results":[{"id":"att3564044322","title":"data.xlsx","fileSize":133223,"mediaType":"application/vnd.ms-excel"}]}`))
	}))
	defer server.Close()

	out, err := runCLI(t, server.URL, "confluence", "attachment", "list", "12345")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"ID", "TITLE", "att3564044322", "data.xlsx", "133223"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestConfluenceAttachmentDownloadFollowsDownloadLink checks the resolution
// the issue called for: metadata names a wiki-relative downloadLink, and the
// bytes come from that path with the /wiki prefix restored.
func TestConfluenceAttachmentDownloadFollowsDownloadLink(t *testing.T) {
	var contentPath, contentQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/wiki/api/v2/attachments/att3564044322", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"att3564044322","title":"data.xlsx","fileSize":9,"mediaType":"application/vnd.ms-excel","downloadLink":"/download/attachments/123/data.xlsx?version=1","_links":{"base":"https://acme.atlassian.net/wiki"}}`))
	})
	mux.HandleFunc("/wiki/download/attachments/123/data.xlsx", func(w http.ResponseWriter, r *http.Request) {
		contentPath = r.URL.Path
		contentQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/vnd.ms-excel")
		w.Write([]byte("cell-data"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	if _, err := runIn(t, dir, server.URL, "confluence", "attachment", "download", "att3564044322"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if contentPath != "/wiki/download/attachments/123/data.xlsx" || contentQuery != "version=1" {
		t.Errorf("content fetched from %s?%s", contentPath, contentQuery)
	}
	data, err := os.ReadFile(filepath.Join(dir, "data.xlsx"))
	if err != nil {
		t.Fatalf("download must write the attachment's title: %v", err)
	}
	if string(data) != "cell-data" {
		t.Errorf("file holds %q", data)
	}
}

func TestConfluenceAttachmentDownloadWithoutLinkFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"att1","title":"data.xlsx"}`))
	}))
	defer server.Close()

	_, err := runCLI(t, server.URL, "confluence", "attachment", "download", "att1", "--out", "-")
	var validationErr *cmdutil.ValidationError
	if err == nil || !strings.Contains(err.Error(), "no download link") {
		t.Fatalf("got %v (%T), want a validation error", err, validationErr)
	}
}

// TestAPIRawResponseWritesBytes covers the generic half of the gap: an
// operation whose response is not JSON used to fail with "response was not
// valid JSON" after the server had already sent the bytes.
func TestAPIRawResponseWritesBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.ms-excel")
		w.Write([]byte("PK\x03\x04raw"))
	}))
	defer server.Close()

	out, err := runCLI(t, server.URL, "jira", "api", "getAttachmentContent", "-f", "id=12345")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "PK\x03\x04raw" {
		t.Errorf("stdout = %q, want the raw bytes", out)
	}
}
