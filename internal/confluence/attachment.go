package confluence

import (
	"bytes"
	"context"
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
