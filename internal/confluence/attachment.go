package confluence

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
	"strings"
	"time"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/credentials"
)

// AttachmentPath is the v1 create-or-update-attachment endpoint. v2 has no
// upload endpoint, and this PUT is idempotent by filename (re-uploading the
// same name adds a version rather than a duplicate), which is the friendlier
// behavior for a scripted CLI.
const AttachmentPath = "/wiki/rest/api/content/%s/child/attachment"

// attachmentTimeout is generous relative to the JSON client's, since an
// upload streams a whole file.
const attachmentTimeout = 60 * time.Second

// UploadAttachment uploads filePath as an attachment on pageID. It exists
// outside the atlapi JSON client because attachments require a
// multipart/form-data request with Confluence's XSRF opt-out header, which
// the JSON-only client.Do cannot express. It is still authenticated with the
// same HTTP Basic credentials the factory holds, so no separate auth path is
// introduced. baseURL is the scheme+host prefix (e.g.
// https://your-org.atlassian.net); a non-2xx response is returned as an
// *atlapi.APIError so it maps to the same exit codes as every other call.
func UploadAttachment(ctx context.Context, baseURL string, creds credentials.Credentials, pageID, filePath string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	// Escape pageID before interpolating it into the path (as the JSON client
	// does for its path params), so a nonnumeric or odd id cannot inject path
	// segments or query separators into the request URL.
	target := strings.TrimRight(baseURL, "/") + fmt.Sprintf(AttachmentPath, url.PathEscape(pageID))
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

// buildAttachmentRequest assembles the multipart PUT: a single "file" part
// carrying the bytes, the XSRF-bypass header Confluence requires for
// attachment uploads, JSON Accept, and HTTP Basic auth. It is separated from
// the network call so the request shape can be unit-tested directly.
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	// Confluence rejects attachment uploads without this XSRF opt-out.
	req.Header.Set("X-Atlassian-Token", "no-check")
	req.SetBasicAuth(creds.Email, creds.APIToken)
	return req, nil
}

// attachmentErrorMessage best-effort trims a Confluence error body to a
// short single line for the APIError message.
func attachmentErrorMessage(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}

// AttachmentsPath lists a page's attachments (v2). AttachmentByIDPath reads
// one attachment's metadata, including the link serving its bytes.
const (
	AttachmentsPath    = "/wiki/api/v2/pages/%s/attachments"
	AttachmentByIDPath = "/wiki/api/v2/attachments/%s"
)

// wikiContext is the path Confluence Cloud is mounted under. A v2
// attachment's downloadLink is relative to it, and the response does not
// always carry the _links.base naming it, so this is the fallback root.
const wikiContext = "/wiki"

// Attachment is the subset of a v2 attachment the curated commands render
// and download. DownloadLink is a site-relative URL (e.g.
// /download/attachments/123/report.xlsx?version=1) rather than a path the
// client can use as-is; DownloadPath reconciles it.
type Attachment struct {
	ID           FlexString `json:"id"`
	Title        string     `json:"title"`
	MediaType    string     `json:"mediaType"`
	FileSize     int64      `json:"fileSize"`
	PageID       FlexString `json:"pageId"`
	Status       string     `json:"status"`
	DownloadLink string     `json:"downloadLink"`
	Links        struct {
		Base     string `json:"base"`
		Download string `json:"download"`
	} `json:"_links"`
}

// DownloadPath returns the post-host path serving the attachment's bytes,
// or "" when the response carried no download link. The link is relative to
// the /wiki context — the same mismatch that made paginated search drop the
// prefix — so it is resolved against _links.base and defaulted to /wiki.
func (a *Attachment) DownloadPath() string {
	link := a.DownloadLink
	if link == "" {
		link = a.Links.Download
	}
	if link == "" {
		return ""
	}
	path := resolveLinkPath(link, a.Links.Base, "")
	if !hasPathPrefix(path, wikiContext) {
		path = wikiContext + path
	}
	return path
}

// AttachmentList is the v2 list envelope for a page's attachments.
type AttachmentList struct {
	Results []Attachment `json:"results"`
}

// ParseAttachmentList decodes a v2 attachments page (or a merged set of
// pages from CollectPages).
func ParseAttachmentList(raw []byte) (*AttachmentList, error) {
	var list AttachmentList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decoding attachments response: %w", err)
	}
	return &list, nil
}

// ParseAttachment decodes a single v2 attachment response.
func ParseAttachment(raw []byte) (*Attachment, error) {
	var att Attachment
	if err := json.Unmarshal(raw, &att); err != nil {
		return nil, fmt.Errorf("decoding attachment response: %w", err)
	}
	return &att, nil
}
