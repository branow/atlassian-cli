package jira

import "encoding/json"

// Jira paginates its list endpoints two different ways, and the curated list
// commands must be able to follow either to completion. These helpers are the
// page-following loops kept here — pure over a "fetch one page" callback and
// the raw bodies it returns — so they can be table-tested without Cobra or
// HTTP, and so every list command follows pages identically.
//
// A limit <= 0 means "no cap". paginate == false stops after the first page
// (still honouring limit), matching the commands' default of one page unless
// --paginate is given.

// startAtPage is the common shape of Jira's offset-paginated envelopes
// (project/search and the agile board/sprint lists): a page of values plus
// the cursor fields the loop reads to decide whether to continue.
type startAtPage[T any] struct {
	StartAt    int  `json:"startAt"`
	MaxResults int  `json:"maxResults"`
	Total      int  `json:"total"`
	IsLast     bool `json:"isLast"`
	Values     []T  `json:"values"`
}

// PaginateStartAt follows an offset-paginated endpoint. fetch is called with
// the next startAt offset and returns that page's raw body; the loop appends
// each page's values and advances startAt by the number of items the page
// actually returned, stopping when the server marks the page last, returns an
// empty page, reports the running offset has reached total, or the caller's
// limit is filled. Advancing by the returned count (not the requested page
// size) means a short final page cannot cause a skipped or duplicated row.
func PaginateStartAt[T any](fetch func(startAt int) ([]byte, error), paginate bool, limit int) ([]T, error) {
	all := []T{}
	startAt := 0
	for {
		raw, err := fetch(startAt)
		if err != nil {
			return nil, err
		}
		var page startAtPage[T]
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Values...)
		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
		startAt += len(page.Values)
		if !paginate || page.IsLast || len(page.Values) == 0 {
			return all, nil
		}
		if page.Total > 0 && startAt >= page.Total {
			return all, nil
		}
	}
}

// PaginateSearch follows the token-paginated POST /search/jql endpoint. fetch
// is called with the opaque nextPageToken of the previous page (empty on the
// first call) and returns that page's raw body; the loop stops when the server
// marks the page last, omits a token, or the caller's limit is filled. Only
// issue list paginates by token, so this stays concrete over SearchResult
// rather than generic.
func PaginateSearch(fetch func(token string) ([]byte, error), paginate bool, limit int) ([]Issue, error) {
	all := []Issue{}
	token := ""
	for {
		raw, err := fetch(token)
		if err != nil {
			return nil, err
		}
		var page SearchResult
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Issues...)
		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
		if !paginate || page.IsLast || page.NextPageToken == "" {
			return all, nil
		}
		token = page.NextPageToken
	}
}
