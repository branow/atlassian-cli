package confluence

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/branow/atlassian-cli/internal/credentials"
)

func TestBuildAttachmentRequest(t *testing.T) {
	creds := credentials.Credentials{Email: "alice@example.com", APIToken: "t0ken"}
	req, err := buildAttachmentRequest(context.Background(), "https://acme.atlassian.net/wiki/rest/api/content/123/child/attachment", creds, "notes.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("buildAttachmentRequest: %v", err)
	}

	if req.Method != "PUT" {
		t.Errorf("got method %s, want PUT", req.Method)
	}
	if got := req.Header.Get("X-Atlassian-Token"); got != "no-check" {
		t.Errorf("got X-Atlassian-Token %q, want no-check", got)
	}
	if user, pass, ok := req.BasicAuth(); !ok || user != "alice@example.com" || pass != "t0ken" {
		t.Errorf("basic auth not set correctly: %q %q %v", user, pass, ok)
	}

	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("got content-type %q (%v), want multipart/form-data", req.Header.Get("Content-Type"), err)
	}

	// The body must carry a single "file" part named after the source file.
	body, _ := io.ReadAll(req.Body)
	reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		t.Fatalf("reading multipart body: %v", err)
	}
	if part.FormName() != "file" {
		t.Errorf("got form field %q, want file", part.FormName())
	}
	if part.FileName() != "notes.txt" {
		t.Errorf("got filename %q, want notes.txt", part.FileName())
	}
	content, _ := io.ReadAll(part)
	if string(content) != "hello" {
		t.Errorf("got file content %q, want hello", content)
	}
}

// TestAttachmentDownloadPath covers the /wiki context reconciliation: a
// downloadLink is relative to the wiki root, and joining it to the client's
// scheme+host base without that prefix 404s.
func TestAttachmentDownloadPath(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			"relative downloadLink gains the wiki context from _links.base",
			`{"id":"att1","downloadLink":"/download/attachments/123/report.xlsx?version=1","_links":{"base":"https://acme.atlassian.net/wiki"}}`,
			"/wiki/download/attachments/123/report.xlsx?version=1",
		},
		{
			"relative downloadLink defaults to /wiki without _links.base",
			`{"id":"att1","downloadLink":"/download/attachments/123/report.xlsx"}`,
			"/wiki/download/attachments/123/report.xlsx",
		},
		{
			"a link already rooted at /wiki is left alone",
			`{"id":"att1","downloadLink":"/wiki/rest/api/content/123/child/attachment/att1/download"}`,
			"/wiki/rest/api/content/123/child/attachment/att1/download",
		},
		{
			"an absolute link is reduced to its path",
			`{"id":"att1","downloadLink":"https://acme.atlassian.net/wiki/download/attachments/123/a.pdf?v=2"}`,
			"/wiki/download/attachments/123/a.pdf?v=2",
		},
		{
			"_links.download is the fallback source",
			`{"id":"att1","_links":{"download":"/download/attachments/123/a.pdf","base":"https://acme.atlassian.net/wiki"}}`,
			"/wiki/download/attachments/123/a.pdf",
		},
		{
			"no link at all",
			`{"id":"att1"}`,
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			att, err := ParseAttachment([]byte(tc.raw))
			if err != nil {
				t.Fatalf("ParseAttachment: %v", err)
			}
			if got := att.DownloadPath(); got != tc.want {
				t.Errorf("DownloadPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseAttachmentList(t *testing.T) {
	raw := []byte(`{"results":[{"id":"att1","title":"report.xlsx","mediaType":"application/vnd.ms-excel","fileSize":133223,"pageId":"123"}]}`)
	list, err := ParseAttachmentList(raw)
	if err != nil {
		t.Fatalf("ParseAttachmentList: %v", err)
	}
	if len(list.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(list.Results))
	}
	got := list.Results[0]
	if got.ID.String() != "att1" || got.Title != "report.xlsx" || got.FileSize != 133223 {
		t.Errorf("attachment decoded as %+v", got)
	}
}
