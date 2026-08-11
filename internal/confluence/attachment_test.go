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
