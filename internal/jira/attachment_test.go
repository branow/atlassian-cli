package jira

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
	req, err := buildAttachmentRequest(context.Background(), "https://acme.atlassian.net/rest/api/3/issue/PROJ-1/attachments", creds, "notes.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("buildAttachmentRequest: %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("got method %s, want POST", req.Method)
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

func TestParseIssueAttachments(t *testing.T) {
	raw := []byte(`{"key":"PROJ-1","fields":{"attachment":[
		{"id":"12345","filename":"report.xlsx","size":133223,"mimeType":"application/vnd.ms-excel","created":"2026-01-02T03:04:05.000+0000","author":{"displayName":"Alice"}},
		{"id":"12346","filename":"notes.txt","size":12}
	]}}`)
	atts, rawList, err := ParseIssueAttachments(raw)
	if err != nil {
		t.Fatalf("ParseIssueAttachments: %v", err)
	}
	if len(atts) != 2 {
		t.Fatalf("got %d attachments, want 2", len(atts))
	}
	if atts[0].ID != "12345" || atts[0].Filename != "report.xlsx" || atts[0].Size != 133223 {
		t.Errorf("first attachment decoded as %+v", atts[0])
	}
	if atts[0].Author == nil || atts[0].Author.DisplayName != "Alice" {
		t.Errorf("author not decoded: %+v", atts[0].Author)
	}
	if !strings.Contains(string(rawList), `"filename":"report.xlsx"`) {
		t.Errorf("raw list must be the server's own array, got %s", rawList)
	}
}

func TestParseIssueAttachmentsWithoutAttachments(t *testing.T) {
	for _, body := range []string{`{"key":"PROJ-1","fields":{}}`, `{"key":"PROJ-1","fields":{"attachment":null}}`, `{"key":"PROJ-1","fields":{"attachment":[]}}`} {
		atts, rawList, err := ParseIssueAttachments([]byte(body))
		if err != nil {
			t.Fatalf("ParseIssueAttachments(%s): %v", body, err)
		}
		if len(atts) != 0 {
			t.Errorf("%s: got %d attachments, want 0", body, len(atts))
		}
		if len(rawList) == 0 {
			t.Errorf("%s: raw list must be an empty JSON array, got %q", body, rawList)
		}
	}
}
