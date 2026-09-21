package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/credentials"
)

// AttachmentPath is the add-attachment endpoint. The issue is addressed by
// key or numeric id, and the same POST both creates a new attachment and adds
// another under the same filename (Jira never overwrites, it versions by
// keeping both), which is the expected behavior for a scripted CLI.
const AttachmentPath = "/rest/api/3/issue/%s/attachments"

// attachmentTimeout is generous relative to the JSON client's, since an
// upload streams a whole file.
const attachmentTimeout = 60 * time.Second

// UploadAttachment uploads filePath as an attachment on the issue identified
// by issueKey (key or numeric id). It exists outside the atlapi JSON client
// because Jira attachments require a multipart/form-data request with the
// XSRF opt-out header, which the JSON-only client.Do cannot express. It is
// still authenticated with the same HTTP Basic credentials the factory holds,
// so no separate auth path is introduced. baseURL is the scheme+host prefix
// (e.g. https://your-org.atlassian.net); a non-2xx response is returned as an
// *atlapi.APIError so it maps to the same exit codes as every other call.
func UploadAttachment(ctx context.Context, baseURL string, creds credentials.Credentials, issueKey, filePath string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	// Escape issueKey before interpolating it into the path (as the JSON
	// client does for its path params), so an odd key cannot inject path
	// segments or query separators into the request URL.
	target := strings.TrimRight(baseURL, "/") + fmt.Sprintf(AttachmentPath, url.PathEscape(issueKey))
	req, err := buildAttachmentRequest(ctx, target, creds, filepath.Base(filePath), content)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: attachmentTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &atlapi.APIError{Status: resp.StatusCode, Message: attachmentErrorMessage(body), Raw: body}
	}
	return body, nil
}

// buildAttachmentRequest assembles the multipart POST: a single "file" part
// carrying the bytes, the XSRF-bypass header Jira requires for attachment
// uploads, JSON Accept, and HTTP Basic auth. It is separated from the network
// call so the request shape can be unit-tested directly.
func buildAttachmentRequest(ctx context.Context, url string, creds credentials.Credentials, filename string, content []byte) (*http.Request, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	// Jira rejects attachment uploads without this XSRF opt-out.
	req.Header.Set("X-Atlassian-Token", "no-check")
	req.SetBasicAuth(creds.Email, creds.APIToken)
	return req, nil
}

// attachmentErrorMessage best-effort trims a Jira error body to a short
// single line for the APIError message.
func attachmentErrorMessage(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}

// AttachmentMetadataPath is the per-attachment metadata endpoint: filename,
// size, media type, and the URL serving the bytes.
const AttachmentMetadataPath = "/rest/api/3/attachment/%s"

// AttachmentContentPath serves an attachment's bytes. Jira answers it with a
// 303 to a short-lived presigned media URL rather than the bytes inline, so
// a client must follow the redirect — and must not forward its Atlassian
// credentials to the redirect target, which carries its own.
const AttachmentContentPath = "/rest/api/3/attachment/content/%s"

// Attachment is the subset of a Jira issue attachment the curated commands
// render. It is the shape both the issue's fields.attachment array and the
// standalone attachment endpoint return.
type Attachment struct {
	// ID is a FlexString because Jira spells it both ways: a string inside
	// an issue's fields.attachment, a number from the attachment endpoint.
	ID       atlapi.FlexString `json:"id"`
	Filename string            `json:"filename"`
	MimeType string            `json:"mimeType"`
	Size     int64             `json:"size"`
	Created  string            `json:"created"`
	Author   *User             `json:"author"`
	Content  string            `json:"content"`
}

// AttachmentColumns is the fixed table for `issue attachment list`.
var AttachmentColumns = []Column[Attachment]{
	{"ID", func(a Attachment) string { return a.ID.String() }},
	{"FILENAME", func(a Attachment) string { return a.Filename }},
	{"SIZE", func(a Attachment) string { return strconv.FormatInt(a.Size, 10) }},
	{"TYPE", func(a Attachment) string { return a.MimeType }},
	{"CREATED", func(a Attachment) string { return a.Created }},
	{"AUTHOR", func(a Attachment) string { return userName(a.Author) }},
}

// issueAttachments is the sliver of an issue response the attachment list
// reads: the attachment array nested under the fields envelope, kept as raw
// JSON alongside the typed form so -o json can echo the server's own objects
// instead of a lossy re-marshal.
type issueAttachments struct {
	Fields struct {
		Attachment json.RawMessage `json:"attachment"`
	} `json:"fields"`
}

// ParseIssueAttachments decodes the attachments of an issue fetched with
// fields=attachment, returning both the typed list for the table and the
// untouched array for -o json. An issue with no attachments yields an empty
// list and an empty JSON array, not an error.
func ParseIssueAttachments(raw []byte) ([]Attachment, []byte, error) {
	var envelope issueAttachments
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, nil, fmt.Errorf("decoding issue attachments: %w", err)
	}
	rawList := []byte(envelope.Fields.Attachment)
	if len(bytes.TrimSpace(rawList)) == 0 || string(bytes.TrimSpace(rawList)) == "null" {
		return []Attachment{}, []byte("[]"), nil
	}
	var atts []Attachment
	if err := json.Unmarshal(rawList, &atts); err != nil {
		return nil, nil, fmt.Errorf("decoding issue attachments: %w", err)
	}
	return atts, rawList, nil
}

// ParseAttachment decodes a single attachment metadata response.
func ParseAttachment(raw []byte) (*Attachment, error) {
	var att Attachment
	if err := json.Unmarshal(raw, &att); err != nil {
		return nil, fmt.Errorf("decoding attachment: %w", err)
	}
	return &att, nil
}
