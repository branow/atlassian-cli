package confluence

import (
	"fmt"
	"testing"
)

// TestCollectPages exercises the page-following loop over a fake fetcher
// keyed by path: the loop must respect the paginate switch, follow
// _links.next when told to, cap at limit, and combine result items in order.
func TestCollectPages(t *testing.T) {
	// pages maps a request path to the raw envelope the fetcher returns.
	pages := map[string]string{
		"/spaces":          `{"results":[{"id":"1"},{"id":"2"}],"_links":{"next":"/spaces?cursor=a"}}`,
		"/spaces?cursor=a": `{"results":[{"id":"3"},{"id":"4"}],"_links":{"next":"/spaces?cursor=b"}}`,
		"/spaces?cursor=b": `{"results":[{"id":"5"}]}`,
		"/one":             `{"results":[{"id":"1"}]}`,
	}
	fetch := func(path string) ([]byte, error) {
		body, ok := pages[path]
		if !ok {
			return nil, fmt.Errorf("unexpected fetch path %q", path)
		}
		return []byte(body), nil
	}

	cases := []struct {
		name     string
		first    string
		limit    int
		paginate bool
		want     int // number of items collected
	}{
		{"first page only, no paginate", "/spaces", 0, false, 2},
		{"first page capped by limit", "/spaces", 1, false, 1},
		{"paginate follows next to exhaustion", "/spaces", 0, true, 5},
		{"paginate stops at limit mid-page", "/spaces", 3, true, 3},
		{"paginate with no next link", "/one", 0, true, 1},
		{"limit larger than available", "/spaces", 99, true, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, err := CollectPages(tc.first, tc.limit, tc.paginate, fetch)
			if err != nil {
				t.Fatalf("CollectPages: %v", err)
			}
			if len(items) != tc.want {
				t.Errorf("got %d items, want %d: %v", len(items), tc.want, items)
			}
		})
	}
}

// TestCollectPagesPropagatesFetchError surfaces a fetch failure rather than
// swallowing it or returning a partial page.
func TestCollectPagesPropagatesFetchError(t *testing.T) {
	want := fmt.Errorf("boom")
	_, err := CollectPages("/x", 0, false, func(string) ([]byte, error) { return nil, want })
	if err != want {
		t.Fatalf("got %v, want %v", err, want)
	}
}

// TestMergeResults re-wraps collected items into the shared results envelope
// and round-trips through the endpoint's own decoder.
func TestMergeResults(t *testing.T) {
	items, err := CollectPages("/x", 0, false, func(string) ([]byte, error) {
		return []byte(`{"results":[{"id":"1","key":"DS","name":"Demo","type":"global"}]}`), nil
	})
	if err != nil {
		t.Fatalf("CollectPages: %v", err)
	}
	merged, err := MergeResults(items)
	if err != nil {
		t.Fatalf("MergeResults: %v", err)
	}
	list, err := ParseSpaceList(merged)
	if err != nil {
		t.Fatalf("ParseSpaceList: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].Key != "DS" {
		t.Errorf("got %+v", list.Results)
	}
}
