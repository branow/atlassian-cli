// Package jira holds the value logic for atl's curated Jira commands: the
// typed request/response shapes the commands decode into, plain-text
// extraction from Atlassian Document Format (ADF) bodies, minimal ADF
// construction for writes, name-to-id resolution (e.g. transitions), and a
// small data-driven table-rendering engine. Keeping this logic here — as
// pure, unit-tested functions independent of Cobra and HTTP — lets the
// cmd/jira_*.go files stay thin flag-parsing shells over client.Do.
package jira

// NamedRef is the shape Jira uses for most referenced entities (status,
// issue type, priority, transition target): an id paired with a display
// name. Both are optional because different endpoints populate different
// subsets.
type NamedRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// User is the subset of a Jira user object the curated commands display.
type User struct {
	AccountID    string `json:"accountId,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
}

// Issue is the subset of a Jira issue the curated commands read. Fields
// nests the value fields under Jira's "fields" envelope; Description is kept
// as untyped JSON because it is an ADF document, decoded to text lazily via
// ExtractText only when displayed.
type Issue struct {
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

// IssueFields mirrors the Jira "fields" object for the columns atl renders.
// Pointer fields distinguish "absent/unassigned" (nil) from a zero value.
type IssueFields struct {
	Summary     string    `json:"summary"`
	Status      *NamedRef `json:"status"`
	Assignee    *User     `json:"assignee"`
	Reporter    *User     `json:"reporter"`
	IssueType   *NamedRef `json:"issuetype"`
	Priority    *NamedRef `json:"priority"`
	Labels      []string  `json:"labels"`
	Created     string    `json:"created"`
	Updated     string    `json:"updated"`
	Description any       `json:"description"`
}

// SearchResult is the /search/jql response envelope: the page of matching
// issues plus the opaque cursor for the next page.
type SearchResult struct {
	Issues        []Issue `json:"issues"`
	NextPageToken string  `json:"nextPageToken,omitempty"`
	IsLast        bool    `json:"isLast,omitempty"`
}

// Project is the subset of a project/search entry atl renders. The Jira
// platform returns project ids as strings (unlike the numeric ids of the
// agile API), so ID is a string here. The paginated envelope that wraps these
// (values/startAt/isLast/...) is decoded generically by PaginateStartAt, so
// no per-resource page struct is needed here.
type Project struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	ProjectTypeKey string `json:"projectTypeKey"`
}

// Board is the subset of an agile board atl renders. The agile REST API
// returns board and sprint ids as JSON numbers, hence int.
type Board struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Sprint is the subset of an agile sprint atl renders.
type Sprint struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// Transition is one available workflow transition for an issue: its id, the
// action name a user types, and the status it leads to.
type Transition struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	To   *NamedRef `json:"to"`
}

// TransitionList is the getTransitions response envelope.
type TransitionList struct {
	Transitions []Transition `json:"transitions"`
}
