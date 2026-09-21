package confluence

import (
	"encoding/json"
	"fmt"

	"github.com/branow/atlassian-cli/internal/atlapi"
)

// FlexString is the shared scalar-normalizing string from atlapi, aliased
// here so the Confluence models keep reading as confluence.FlexString while
// one definition serves both products (Jira has the same string-or-number
// ids).
type FlexString = atlapi.FlexString

// Space is a Confluence space as returned by the v2 spaces endpoint, reduced
// to the columns the `space ls` table shows.
type Space struct {
	ID   FlexString `json:"id"`
	Key  string     `json:"key"`
	Name string     `json:"name"`
	Type string     `json:"type"`
}

// SpaceList is the v2 list envelope: results carry the page of spaces.
type SpaceList struct {
	Results []Space `json:"results"`
}

// ParseSpaceList decodes a v2 GET /spaces response.
func ParseSpaceList(raw []byte) (*SpaceList, error) {
	var list SpaceList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decoding spaces response: %w", err)
	}
	return &list, nil
}

// PageVersion is a page's monotonically increasing version; the number is
// what a patch increments.
type PageVersion struct {
	Number int `json:"number"`
}

// PageBody carries the storage-format representation of a page body, the
// only representation the patch engine and `page create` speak.
type PageBody struct {
	Storage struct {
		Value          string `json:"value"`
		Representation string `json:"representation"`
	} `json:"storage"`
}

// Page is a Confluence page as returned by the v2 page endpoint, reduced to
// the fields the curated commands read: identity, placement, version, and
// (when requested with body-format=storage) the storage body.
type Page struct {
	ID      FlexString  `json:"id"`
	Title   string      `json:"title"`
	SpaceID FlexString  `json:"spaceId"`
	Status  string      `json:"status"`
	Version PageVersion `json:"version"`
	Body    PageBody    `json:"body"`
}

// StorageValue returns the page's storage-format body markup.
func (p *Page) StorageValue() string { return p.Body.Storage.Value }

// ParsePage decodes a v2 page response (get or create).
func ParsePage(raw []byte) (*Page, error) {
	var page Page
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("decoding page response: %w", err)
	}
	return &page, nil
}

// SearchResult is one hit from the v1 CQL search: the interesting fields
// live under content, with the result title and container as fallbacks for
// display when content is absent (e.g. a non-content match).
type SearchResult struct {
	Content struct {
		ID    FlexString `json:"id"`
		Type  string     `json:"type"`
		Title string     `json:"title"`
		Space struct {
			Key string `json:"key"`
		} `json:"space"`
	} `json:"content"`
	Title                 string `json:"title"`
	ResultGlobalContainer struct {
		Title string `json:"title"`
	} `json:"resultGlobalContainer"`
}

// SearchResults is the v1 search envelope.
type SearchResults struct {
	Results []SearchResult `json:"results"`
}

// ParseSearch decodes a v1 GET /search response.
func ParseSearch(raw []byte) (*SearchResults, error) {
	var results SearchResults
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}
	return &results, nil
}

// Row reduces a search hit to the {id, type, title, space} columns the
// search table shows, filling display fields from content and falling back
// to the result-level title and container when content lacks them.
func (r SearchResult) Row() (id, typ, title, space string) {
	id = r.Content.ID.String()
	typ = r.Content.Type
	title = r.Content.Title
	if title == "" {
		title = r.Title
	}
	space = r.Content.Space.Key
	if space == "" {
		space = r.ResultGlobalContainer.Title
	}
	return id, typ, title, space
}
