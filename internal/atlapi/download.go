package atlapi

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// maxRedirects caps the redirect chain a download follows. Attachment
// endpoints answer with one 303 to a presigned URL; anything longer is a
// misconfiguration or a loop, not a legitimate hop.
const maxRedirects = 5

// errorBodyLimit caps how much of a failed download's body is read for the
// error message — the endpoint may stream gigabytes, and an error envelope
// is a few hundred bytes.
const errorBodyLimit = 64 << 10

// Download is a raw (non-JSON) response body still open for reading. The
// caller owns Body and must close it. Filename and ContentType come from the
// response headers when the server sent them, so a caller can name the
// destination file the way the server names the attachment.
type Download struct {
	Body        io.ReadCloser
	ContentType string
	// Filename is the sanitized name from Content-Disposition, or "" when
	// the server sent none.
	Filename string
	// Size is the Content-Length, or -1 when the server did not send one
	// (a chunked response).
	Size int64
}

// Download GETs path and hands back the response body unread, for endpoints
// that answer with bytes rather than JSON (attachment contents, thumbnails,
// exports). path is a full post-host path and may already carry a query
// string. A non-2xx response is drained into an *APIError so it maps to the
// same exit codes as every other call; a 2xx response is the caller's to
// stream and close.
//
// This is the raw-bytes counterpart to Do: Do buffers and JSON-decodes,
// which is wrong for a file of arbitrary size and wrong for a body that is
// not JSON at all.
func (c *HTTPClient) Download(ctx context.Context, path string) (*Download, error) {
	requestURL := c.baseURL + path
	resp, err := c.retrying(ctx, http.MethodGet, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, err
		}
		// Attachment endpoints serve the file's own media type; asking for
		// JSON only would make a strict endpoint answer 406.
		req.Header.Set("Accept", "*/*")
		req.SetBasicAuth(c.creds.Email, c.creds.APIToken)
		return c.downloadClient.Do(req)
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return nil, &APIError{
			Status:      resp.StatusCode,
			Message:     errorMessage(body),
			Raw:         body,
			RateLimited: isRateLimited(resp.StatusCode, resp.Header),
		}
	}
	return &Download{
		Body:        resp.Body,
		ContentType: resp.Header.Get("Content-Type"),
		Filename:    FilenameFromDisposition(resp.Header.Get("Content-Disposition")),
		Size:        resp.ContentLength,
	}, nil
}

// newDownloadClient builds the http.Client raw downloads use. It differs
// from the JSON client in two ways that matter:
//
//   - No whole-request timeout. The JSON client's 30s covers reading the
//     body, which would cut off any attachment bigger than the link is fast.
//     A response-header timeout keeps a hung server bounded instead, leaving
//     the transfer itself as long as it needs.
//   - Authorization is dropped when a redirect leaves the Atlassian host.
//     The attachment endpoints 303 to a presigned media URL that carries its
//     own credentials in the query string; forwarding HTTP Basic
//     email:APIToken to that third-party host would leak the token.
func newDownloadClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = requestTimeout
	return &http.Client{
		Transport:     transport,
		CheckRedirect: stripAuthOnHostChange,
	}
}

// stripAuthOnHostChange is the download client's redirect policy: follow up
// to maxRedirects hops, removing the Authorization header as soon as the
// target host differs from the one the request was authenticated for. Go's
// own policy already strips it across domains, but it keeps the header for a
// subdomain; an exact-host rule is the one that matches "these credentials
// belong to this site and nowhere else".
func stripAuthOnHostChange(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if req.URL.Host != via[0].URL.Host {
		req.Header.Del("Authorization")
	}
	return nil
}

// FilenameFromDisposition extracts a usable base filename from a
// Content-Disposition header, decoding RFC 5987 (filename*) encodings and
// reducing the result to a bare name. A server-supplied name goes on to
// become a path on disk, so anything that could escape the destination
// directory — separators, "." and ".." — yields "" and the caller falls back
// to a name it chose itself.
func FilenameFromDisposition(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return SafeFilename(params["filename"])
}

// SafeFilename reduces a server-supplied name to a bare filename safe to
// join onto a destination directory, returning "" when nothing usable
// remains. It strips both separators so a Windows-style "..\\x" cannot
// survive on a Unix host.
func SafeFilename(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = filepath.Base(name)
	switch name {
	case "", ".", "..", string(filepath.Separator):
		return ""
	}
	return name
}
