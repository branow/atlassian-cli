package confluence

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// pageEnvelope is the shape both curated list endpoints share: a page of
// results plus Confluence's _links.next relative URL pointing at the following
// page (absent on the last page). Both the v2 spaces list and the v1 CQL
// search envelopes carry this pair, so one page-following loop drives both.
// The v1 search envelope also carries base/context naming the /wiki root that
// its next link is relative to; see resolveLinkPath. The result items are kept
// as raw JSON so the combined set can be re-emitted or re-parsed with the
// endpoint's own typed decoder, losing no field.
type pageEnvelope struct {
	Results []json.RawMessage `json:"results"`
	Links   struct {
		Next    string `json:"next"`
		Base    string `json:"base"`
		Context string `json:"context"`
	} `json:"_links"`
}

// FetchPage retrieves one raw page body for the given post-host path. The
// pagination loop is pure over this function so a test can drive it with
// canned bodies keyed by path instead of a live client.
type FetchPage func(path string) ([]byte, error)

// CollectPages follows Confluence's _links.next chain from firstPath,
// accumulating each page's raw result items in order. It stops when a page has
// no next link, when paginate is false (only the first page is fetched), or
// when limit items have been collected (limit <= 0 means no cap). The returned
// slice is the combined, order-preserving set of result items across the pages
// visited, truncated to limit. Both list commands feed the result to
// MergeResults so their -o json emits one combined results envelope.
//
// On a mid-pagination failure it returns the pages already collected alongside
// the error, so a caller can still emit the partial results rather than losing
// data it had in hand.
func CollectPages(firstPath string, limit int, paginate bool, fetch FetchPage) ([]json.RawMessage, error) {
	items := []json.RawMessage{}
	for path := firstPath; path != ""; {
		raw, err := fetch(path)
		if err != nil {
			return items, err
		}
		var env pageEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return items, fmt.Errorf("decoding paginated response: %w", err)
		}
		items = append(items, env.Results...)
		if limit > 0 && len(items) >= limit {
			return items[:limit], nil
		}
		if !paginate {
			break
		}
		path = resolveLinkPath(env.Links.Next, env.Links.Base, env.Links.Context)
	}
	return items, nil
}

// resolveLinkPath turns a link Confluence returns in a response — a page's
// _links.next, an attachment's downloadLink — into a post-host path the
// client can join to its scheme+host base URL. The v2 endpoints return such
// links already rooted at the site (e.g. /wiki/api/v2/spaces?cursor=...),
// but the v1 search API returns them relative to the /wiki context path
// (e.g. /rest/api/search?...) with _links.base/_links.context naming that
// root. Without reconciling them the /wiki prefix is dropped and the
// follow-up request 404s.
func resolveLinkPath(next, base, context string) string {
	if next == "" {
		return ""
	}
	// An absolute URL already carries the full path; reduce it to host-relative
	// so joining it to the client's scheme+host base yields the right URL.
	if u, err := url.Parse(next); err == nil && u.Host != "" {
		if u.RawQuery != "" {
			return u.Path + "?" + u.RawQuery
		}
		return u.Path
	}
	// A relative next is rooted at the context path (v1 search). Prepend it when
	// the link doesn't already include it (v2 next values already do).
	ctx := strings.TrimRight(contextPath(base, context), "/")
	if ctx != "" && !hasPathPrefix(next, ctx) {
		return ctx + next
	}
	return next
}

// contextPath is the API's context prefix for a relative next link, preferring
// the explicit _links.context and falling back to the path of _links.base.
func contextPath(base, context string) string {
	if context != "" {
		return context
	}
	if base != "" {
		if u, err := url.Parse(base); err == nil {
			return u.Path
		}
	}
	return ""
}

// hasPathPrefix reports whether path starts with prefix at a segment boundary,
// so "/wiki" matches "/wiki/rest" and "/wiki?x=1" but not "/wikifoo".
func hasPathPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := path[len(prefix):]
	return rest == "" || rest[0] == '/' || rest[0] == '?'
}

// MergeResults wraps collected raw result items back into a {"results":[...]}
// envelope, the combined shape both list commands emit under -o json so a
// paginated result reads as a single page's worth of results and re-parses
// with ParseSpaceList / ParseSearch unchanged.
func MergeResults(items []json.RawMessage) ([]byte, error) {
	if items == nil {
		items = []json.RawMessage{}
	}
	return json.Marshal(map[string]any{"results": items})
}
