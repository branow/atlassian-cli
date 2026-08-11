package jira

import (
	"errors"
	"testing"
)

// pageScript feeds PaginateStartAt/PaginateSearch canned page bodies in order,
// recording how many times it was called so a test can assert the loop stopped
// early instead of fetching every scripted page.
type pageScript struct {
	bodies []string
	calls  int
}

func (s *pageScript) next() ([]byte, error) {
	if s.calls >= len(s.bodies) {
		return nil, errors.New("fetch called more times than scripted")
	}
	body := s.bodies[s.calls]
	s.calls++
	return []byte(body), nil
}

func TestPaginateStartAt(t *testing.T) {
	tests := []struct {
		name     string
		bodies   []string
		paginate bool
		limit    int
		wantIDs  []string
		wantCall int
	}{
		{
			name:     "first page only when not paginating",
			bodies:   []string{`{"startAt":0,"maxResults":2,"total":4,"isLast":false,"values":[{"key":"A"},{"key":"B"}]}`},
			paginate: false,
			wantIDs:  []string{"A", "B"},
			wantCall: 1,
		},
		{
			name: "follows pages until isLast",
			bodies: []string{
				`{"startAt":0,"maxResults":2,"total":3,"isLast":false,"values":[{"key":"A"},{"key":"B"}]}`,
				`{"startAt":2,"maxResults":2,"total":3,"isLast":true,"values":[{"key":"C"}]}`,
			},
			paginate: true,
			wantIDs:  []string{"A", "B", "C"},
			wantCall: 2,
		},
		{
			name: "stops when total reached without isLast",
			bodies: []string{
				`{"startAt":0,"maxResults":2,"total":2,"isLast":false,"values":[{"key":"A"},{"key":"B"}]}`,
			},
			paginate: true,
			wantIDs:  []string{"A", "B"},
			wantCall: 1,
		},
		{
			name: "limit caps aggregate and stops early",
			bodies: []string{
				`{"startAt":0,"maxResults":2,"total":9,"isLast":false,"values":[{"key":"A"},{"key":"B"}]}`,
			},
			paginate: true,
			limit:    1,
			wantIDs:  []string{"A"},
			wantCall: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &pageScript{bodies: tt.bodies}
			got, err := PaginateStartAt[Project](func(int) ([]byte, error) { return s.next() }, tt.paginate, tt.limit)
			if err != nil {
				t.Fatalf("PaginateStartAt: %v", err)
			}
			if s.calls != tt.wantCall {
				t.Errorf("fetch calls = %d, want %d", s.calls, tt.wantCall)
			}
			assertKeys(t, projectKeys(got), tt.wantIDs)
		})
	}
}

func TestPaginateStartAtThreadsOffset(t *testing.T) {
	var offsets []int
	fetch := func(startAt int) ([]byte, error) {
		offsets = append(offsets, startAt)
		bodies := map[int]string{
			0: `{"startAt":0,"maxResults":2,"total":3,"isLast":false,"values":[{"key":"A"},{"key":"B"}]}`,
			2: `{"startAt":2,"maxResults":2,"total":3,"isLast":true,"values":[{"key":"C"}]}`,
		}
		return []byte(bodies[startAt]), nil
	}
	if _, err := PaginateStartAt[Project](fetch, true, 0); err != nil {
		t.Fatalf("PaginateStartAt: %v", err)
	}
	if len(offsets) != 2 || offsets[0] != 0 || offsets[1] != 2 {
		t.Errorf("offsets = %v, want [0 2] (advance by returned count)", offsets)
	}
}

func TestPaginateSearch(t *testing.T) {
	tests := []struct {
		name     string
		bodies   []string
		paginate bool
		limit    int
		wantKeys []string
		wantCall int
	}{
		{
			name:     "first page only when not paginating",
			bodies:   []string{`{"issues":[{"key":"P-1"},{"key":"P-2"}],"nextPageToken":"tok","isLast":false}`},
			paginate: false,
			wantKeys: []string{"P-1", "P-2"},
			wantCall: 1,
		},
		{
			name: "follows token until absent",
			bodies: []string{
				`{"issues":[{"key":"P-1"}],"nextPageToken":"tok","isLast":false}`,
				`{"issues":[{"key":"P-2"}],"isLast":true}`,
			},
			paginate: true,
			wantKeys: []string{"P-1", "P-2"},
			wantCall: 2,
		},
		{
			name:     "limit caps and stops early",
			bodies:   []string{`{"issues":[{"key":"P-1"},{"key":"P-2"}],"nextPageToken":"tok"}`},
			paginate: true,
			limit:    1,
			wantKeys: []string{"P-1"},
			wantCall: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &pageScript{bodies: tt.bodies}
			got, err := PaginateSearch(func(string) ([]byte, error) { return s.next() }, tt.paginate, tt.limit)
			if err != nil {
				t.Fatalf("PaginateSearch: %v", err)
			}
			if s.calls != tt.wantCall {
				t.Errorf("fetch calls = %d, want %d", s.calls, tt.wantCall)
			}
			assertKeys(t, issueKeys(got), tt.wantKeys)
		})
	}
}

func TestPaginateSearchThreadsToken(t *testing.T) {
	var tokens []string
	fetch := func(token string) ([]byte, error) {
		tokens = append(tokens, token)
		bodies := map[string]string{
			"":     `{"issues":[{"key":"P-1"}],"nextPageToken":"tok2"}`,
			"tok2": `{"issues":[{"key":"P-2"}],"isLast":true}`,
		}
		return []byte(bodies[token]), nil
	}
	if _, err := PaginateSearch(fetch, true, 0); err != nil {
		t.Fatalf("PaginateSearch: %v", err)
	}
	if len(tokens) != 2 || tokens[0] != "" || tokens[1] != "tok2" {
		t.Errorf("tokens = %v, want [\"\" \"tok2\"]", tokens)
	}
}

func projectKeys(ps []Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Key
	}
	return out
}

func issueKeys(is []Issue) []string {
	out := make([]string, len(is))
	for i, is := range is {
		out[i] = is.Key
	}
	return out
}

func assertKeys(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keys = %v, want %v", got, want)
		}
	}
}
