package jira

import (
	"fmt"
	"sort"
	"strings"
)

// Column names one table column and extracts its cell value from a row of
// type T. Columns are data: a command declares a slice of them and hands it,
// with its rows, to Render — there is no per-resource formatting branch. The
// same shape backs issue, project, board, and sprint tables.
type Column[T any] struct {
	Header string
	Value  func(T) string
}

// Render walks rows through cols, producing the headers and string cells the
// output.WriteTable renderer consumes. It is the single engine every curated
// list/get table flows through.
func Render[T any](cols []Column[T], rows []T) (headers []string, cells [][]string) {
	headers = make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.Header
	}
	cells = make([][]string, 0, len(rows))
	for _, row := range rows {
		cell := make([]string, len(cols))
		for i, c := range cols {
			cell[i] = c.Value(row)
		}
		cells = append(cells, cell)
	}
	return headers, cells
}

// issueColumn augments a Column with the Jira field name the API must return
// for the extractor to have data. issue list/get request exactly the fields
// their selected columns need, keeping the payload small and predictable.
type issueColumn struct {
	Column[Issue]
	APIField string
}

// issueColumns is the ordered registry of selectable issue columns, keyed by
// the lowercase name a user passes to --fields. Adding a column is a data
// edit here, not a change to any renderer.
var issueColumns = []struct {
	Key string
	Col issueColumn
}{
	{"key", issueColumn{Column[Issue]{"KEY", func(i Issue) string { return i.Key }}, ""}},
	{"summary", issueColumn{Column[Issue]{"SUMMARY", func(i Issue) string { return i.Fields.Summary }}, "summary"}},
	{"status", issueColumn{Column[Issue]{"STATUS", func(i Issue) string { return refName(i.Fields.Status) }}, "status"}},
	{"assignee", issueColumn{Column[Issue]{"ASSIGNEE", func(i Issue) string { return userName(i.Fields.Assignee) }}, "assignee"}},
	{"reporter", issueColumn{Column[Issue]{"REPORTER", func(i Issue) string { return userName(i.Fields.Reporter) }}, "reporter"}},
	{"type", issueColumn{Column[Issue]{"TYPE", func(i Issue) string { return refName(i.Fields.IssueType) }}, "issuetype"}},
	{"priority", issueColumn{Column[Issue]{"PRIORITY", func(i Issue) string { return refName(i.Fields.Priority) }}, "priority"}},
	{"labels", issueColumn{Column[Issue]{"LABELS", func(i Issue) string { return strings.Join(i.Fields.Labels, ",") }}, "labels"}},
	{"created", issueColumn{Column[Issue]{"CREATED", func(i Issue) string { return i.Fields.Created }}, "created"}},
	{"updated", issueColumn{Column[Issue]{"UPDATED", func(i Issue) string { return i.Fields.Updated }}, "updated"}},
	{"description", issueColumn{Column[Issue]{"DESCRIPTION", func(i Issue) string { return ExtractText(i.Fields.Description) }}, "description"}},
}

// DefaultGetColumns and DefaultListColumns are the column sets shown when
// --fields is not given: get is detailed, list is compact.
var (
	DefaultGetColumns  = []string{"key", "summary", "status", "assignee", "type", "priority"}
	DefaultListColumns = []string{"key", "summary", "status", "assignee"}
)

// SelectIssueColumns resolves user-facing column names (or the given default
// when names is empty) to the concrete columns to render plus the distinct
// Jira API field names those columns require. An unknown name is an error
// listing the valid ones, so a typo fails loudly instead of silently
// dropping a column.
func SelectIssueColumns(names, fallback []string) (cols []Column[Issue], apiFields []string, err error) {
	if len(names) == 0 {
		names = fallback
	}
	seenField := map[string]bool{}
	for _, raw := range names {
		key := strings.ToLower(strings.TrimSpace(raw))
		ic, ok := lookupIssueColumn(key)
		if !ok {
			return nil, nil, fmt.Errorf("unknown field %q; available: %s", raw, availableIssueFields())
		}
		cols = append(cols, ic.Column)
		if ic.APIField != "" && !seenField[ic.APIField] {
			seenField[ic.APIField] = true
			apiFields = append(apiFields, ic.APIField)
		}
	}
	return cols, apiFields, nil
}

func lookupIssueColumn(key string) (issueColumn, bool) {
	for _, entry := range issueColumns {
		if entry.Key == key {
			return entry.Col, true
		}
	}
	return issueColumn{}, false
}

// availableIssueFields lists the selectable --fields names, sorted, for
// error messages.
func availableIssueFields() string {
	names := make([]string, 0, len(issueColumns))
	for _, entry := range issueColumns {
		names = append(names, entry.Key)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ProjectColumns, BoardColumns, and SprintColumns are the fixed tables for
// the list commands whose columns the user does not choose.
var (
	ProjectColumns = []Column[Project]{
		{"KEY", func(p Project) string { return p.Key }},
		{"NAME", func(p Project) string { return p.Name }},
		{"ID", func(p Project) string { return p.ID }},
		{"TYPE", func(p Project) string { return p.ProjectTypeKey }},
	}
	BoardColumns = []Column[Board]{
		{"ID", func(b Board) string { return fmt.Sprint(b.ID) }},
		{"NAME", func(b Board) string { return b.Name }},
		{"TYPE", func(b Board) string { return b.Type }},
	}
	SprintColumns = []Column[Sprint]{
		{"ID", func(s Sprint) string { return fmt.Sprint(s.ID) }},
		{"NAME", func(s Sprint) string { return s.Name }},
		{"STATE", func(s Sprint) string { return s.State }},
	}
)

func refName(ref *NamedRef) string {
	if ref == nil {
		return ""
	}
	return ref.Name
}

func userName(u *User) string {
	if u == nil {
		return ""
	}
	return u.DisplayName
}
