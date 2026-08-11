package cmd

import (
	"encoding/json"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/jira"
	"github.com/branow/atlassian-cli/internal/output"
)

// jiraTable renders typed rows through a data-driven column set into the
// shared aligned-table writer, so every curated Jira list/get command emits
// tables the same way instead of hand-formatting rows.
func jiraTable[T any](f *cmdutil.Factory, cols []jira.Column[T], rows []T) error {
	headers, cells := jira.Render(cols, rows)
	return output.WriteTable(f.IOStreams.Out, headers, cells)
}

// jiraKeyValues renders a single object as a vertical key/value block by
// reusing its table columns: each column's header becomes a key and its
// extracted cell the value. This is the single-object counterpart to
// jiraTable, matching Confluence's page get instead of squeezing one object
// into a one-row horizontal table.
func jiraKeyValues[T any](f *cmdutil.Factory, cols []jira.Column[T], row T) error {
	headers, cells := jira.Render(cols, []T{row})
	pairs := make([][2]string, len(headers))
	for i, h := range headers {
		pairs[i] = [2]string{h, cells[0][i]}
	}
	return writeKeyValues(f, pairs)
}

// jiraAggregateJSON is the -o json path for a paginated list: the combined
// items from every followed page are emitted as one envelope keyed like the
// endpoint's own page ("issues" for search, "values" for the rest), then
// pretty-printed through the shared writer so the shape matches a single
// server page. Aggregating necessarily re-marshals from the typed structs
// rather than echoing raw bytes, so only the modelled fields survive.
func jiraAggregateJSON[T any](f *cmdutil.Factory, key string, items []T) error {
	if items == nil {
		items = []T{}
	}
	raw, err := json.Marshal(map[string]any{key: items})
	if err != nil {
		return err
	}
	return writeResultJSON(f, raw)
}

// jiraJSON is the -o json path for curated Jira commands: it prints the
// untouched server response through the shared writer, so Jira and Confluence
// render JSON identically and no field is hidden behind the friendly table.
func jiraJSON(f *cmdutil.Factory, resp *atlapi.Response) error {
	return writeResultJSON(f, resp.Raw)
}

// wantJSON reports whether the resolved output format is JSON.
func wantJSON(f *cmdutil.Factory) bool {
	return f.OutputFormat() == "json"
}
