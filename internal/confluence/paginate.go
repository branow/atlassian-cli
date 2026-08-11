package confluence

import (
	"encoding/json"
	"fmt"
)

// pageEnvelope is the shape both curated list endpoints share: a page of
// results plus Confluence's _links.next relative URL pointing at the following
// page (absent on the last page). Both the v2 spaces list and the v1 CQL
// search envelopes carry this pair, so one page-following loop drives both.
// The result items are kept as raw JSON so the combined set can be re-emitted
// or re-parsed with the endpoint's own typed decoder, losing no field.
type pageEnvelope struct {
	Results []json.RawMessage `json:"results"`
	Links   struct {
		Next string `json:"next"`
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
func CollectPages(firstPath string, limit int, paginate bool, fetch FetchPage) ([]json.RawMessage, error) {
	items := []json.RawMessage{}
	for path := firstPath; path != ""; {
		raw, err := fetch(path)
		if err != nil {
			return nil, err
		}
		var env pageEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decoding paginated response: %w", err)
		}
		items = append(items, env.Results...)
		if limit > 0 && len(items) >= limit {
			return items[:limit], nil
		}
		if !paginate {
			break
		}
		path = env.Links.Next
	}
	return items, nil
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
