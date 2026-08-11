package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/jira"
)

// newJiraIssueCmd groups the curated issue commands. Each subcommand drives a
// single REST endpoint through client.Do with an explicit path, so the
// jira/jira-software getIssue ambiguity of the generic invoker never applies.
func newJiraIssueCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Work with Jira issues",
	}
	cmd.AddCommand(
		newJiraIssueGetCmd(f),
		newJiraIssueListCmd(f),
		newJiraIssueCreateCmd(f),
		newJiraIssueEditCmd(f),
		newJiraIssueTransitionCmd(f),
		newJiraIssueCommentCmd(f),
	)
	return cmd
}

// newJiraIssueGetCmd fetches one issue and renders its key fields. Because a
// Jira description is an ADF document rather than text, the "description"
// column is decoded to best-effort plain text via jira.ExtractText.
func newJiraIssueGetCmd(f *cmdutil.Factory) *cobra.Command {
	var fields []string
	cmd := &cobra.Command{
		Use:   "get <KEY>",
		Short: "Get an issue by key",
		Long: `Get an issue by key (e.g. PROJ-123) and print its key fields as a
key/value block. --fields selects which rows to show; -o json dumps the raw API response
(descriptions and comments are ADF JSON there). The description column is
rendered as best-effort plain text extracted from ADF.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cols, _, err := jira.SelectIssueColumns(fields, jira.DefaultGetColumns)
			if err != nil {
				return &cmdutil.ValidationError{Message: err.Error()}
			}
			resp, err := doJira(cmd.Context(), f, http.MethodGet, "/rest/api/3/issue/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return jiraJSON(f, resp)
			}
			var issue jira.Issue
			if err := json.Unmarshal(resp.Raw, &issue); err != nil {
				return err
			}
			return jiraKeyValues(f, cols, issue)
		},
	}
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "columns to show (comma-separated)")
	return cmd
}

// newJiraIssueListCmd runs a JQL search via the current POST /search/jql
// endpoint (the older GET /search is deprecated) and lists matching issues.
// Only the fields the selected columns need are requested, keeping the
// response small.
func newJiraIssueListCmd(f *cmdutil.Factory) *cobra.Command {
	var jql string
	var limit int
	var paginate bool
	var fields []string
	cmd := &cobra.Command{
		Use:   "list [JQL]",
		Short: "Search issues with JQL",
		Long: `Search issues with a JQL query, given as the positional argument or via
--jql. --fields selects the columns. --limit caps the number returned;
--paginate follows every page until exhausted (or --limit is reached),
otherwise only the first page is shown.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := singleValue("JQL query", jql, args)
			if err != nil {
				return err
			}
			cols, apiFields, err := jira.SelectIssueColumns(fields, jira.DefaultListColumns)
			if err != nil {
				return &cmdutil.ValidationError{Message: err.Error()}
			}
			// The search endpoint carries its cursor in the request body
			// (nextPageToken), so each page re-POSTs the same query with the
			// previous page's token threaded back in.
			fetch := func(token string) ([]byte, error) {
				body := map[string]any{"jql": query, "fields": apiFields}
				if limit > 0 {
					body["maxResults"] = limit
				}
				if token != "" {
					body["nextPageToken"] = token
				}
				resp, err := doJira(cmd.Context(), f, http.MethodPost, "/rest/api/3/search/jql", nil, body)
				if err != nil {
					return nil, err
				}
				return resp.Raw, nil
			}
			issues, err := jira.PaginateSearch(fetch, paginate, limit)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return jiraAggregateJSON(f, "issues", issues)
			}
			return jiraTable(f, cols, issues)
		},
	}
	cmd.Flags().StringVar(&jql, "jql", "", "JQL query (alternative to the positional argument)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of issues to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow every page until exhausted or --limit is reached")
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "columns to show (comma-separated)")
	return cmd
}

// newJiraIssueCreateCmd creates an issue. The description is wrapped in a
// minimal ADF document because the API rejects a plain string there; extra
// -f field=value pairs are merged into the fields object with JSON-typed
// values, so callers can set custom fields without a bespoke flag each.
func newJiraIssueCreateCmd(f *cmdutil.Factory) *cobra.Command {
	var project, issueType, summary, description string
	var extra []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		Long: `Create an issue in a project. --project, --type, and --summary are
required. --description text is stored as ADF. Repeatable -f field=value
pairs set additional fields (values parse as JSON when they look like it).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireFlags(map[string]string{"project": project, "type": issueType, "summary": summary}); err != nil {
				return err
			}
			fields := map[string]any{
				"project":   map[string]any{"key": project},
				"issuetype": map[string]any{"name": issueType},
				"summary":   summary,
			}
			if description != "" {
				fields["description"] = jira.Document(description)
			}
			if err := mergeFields(fields, extra); err != nil {
				return err
			}
			resp, err := doJira(cmd.Context(), f, http.MethodPost, "/rest/api/3/issue", nil, map[string]any{"fields": fields})
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return jiraJSON(f, resp)
			}
			return printCreatedKey(f, resp, "Created")
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project key (required)")
	cmd.Flags().StringVar(&issueType, "type", "", "issue type name, e.g. Task or Bug (required)")
	cmd.Flags().StringVar(&summary, "summary", "", "issue summary (required)")
	cmd.Flags().StringVar(&description, "description", "", "issue description (stored as ADF)")
	cmd.Flags().StringArrayVarP(&extra, "field", "f", nil, "additional field as key=value (repeatable)")
	return cmd
}

// newJiraIssueEditCmd updates fields on an existing issue. Only flags the
// user actually set are sent, so an omitted --summary leaves the summary
// untouched rather than blanking it.
func newJiraIssueEditCmd(f *cmdutil.Factory) *cobra.Command {
	var summary, description string
	var extra []string
	cmd := &cobra.Command{
		Use:   "edit <KEY>",
		Short: "Edit an issue",
		Long: `Update fields on an issue. Only the flags you pass are changed.
--description is stored as ADF; -f field=value sets additional fields.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields := map[string]any{}
			if cmd.Flags().Changed("summary") {
				fields["summary"] = summary
			}
			if cmd.Flags().Changed("description") {
				fields["description"] = jira.Document(description)
			}
			if err := mergeFields(fields, extra); err != nil {
				return err
			}
			if len(fields) == 0 {
				return &cmdutil.ValidationError{Message: "nothing to update: pass --summary, --description, or -f field=value"}
			}
			_, err := doJira(cmd.Context(), f, http.MethodPut, "/rest/api/3/issue/"+url.PathEscape(args[0]), nil, map[string]any{"fields": fields})
			if err != nil {
				return err
			}
			return printAction(f, "Updated", args[0])
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "new summary")
	cmd.Flags().StringVar(&description, "description", "", "new description (stored as ADF)")
	cmd.Flags().StringArrayVarP(&extra, "field", "f", nil, "additional field as key=value (repeatable)")
	return cmd
}

// newJiraIssueTransitionCmd moves an issue through its workflow. Transition
// ids are opaque per-workflow numbers, so --to accepts a destination name
// resolved against the issue's currently available transitions; --list shows
// those transitions without performing one.
func newJiraIssueTransitionCmd(f *cmdutil.Factory) *cobra.Command {
	var to string
	var list bool
	cmd := &cobra.Command{
		Use:   "transition <KEY>",
		Short: "Transition an issue to another status",
		Long: `Transition an issue. --to accepts a transition name or id, resolved
against the issue's available transitions. --list prints those transitions
instead of performing one.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			path := "/rest/api/3/issue/" + url.PathEscape(key) + "/transitions"
			resp, err := doJira(cmd.Context(), f, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			var available jira.TransitionList
			if err := json.Unmarshal(resp.Raw, &available); err != nil {
				return err
			}
			if list {
				if wantJSON(f) {
					return jiraJSON(f, resp)
				}
				return jiraTable(f, transitionColumns, available.Transitions)
			}
			if to == "" {
				return &cmdutil.ValidationError{Message: "--to is required (or use --list to see available transitions)"}
			}
			target, err := jira.ResolveTransition(available.Transitions, to)
			if err != nil {
				return &cmdutil.ValidationError{Message: err.Error()}
			}
			body := map[string]any{"transition": map[string]any{"id": target.ID}}
			if _, err := doJira(cmd.Context(), f, http.MethodPost, path, nil, body); err != nil {
				return err
			}
			return printAction(f, "Transitioned", fmt.Sprintf("%s to %s", key, target.Name))
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "destination transition name or id")
	cmd.Flags().BoolVar(&list, "list", false, "list available transitions instead of performing one")
	return cmd
}

// transitionColumns is the data-driven table for --list, defined here since
// it is specific to this command rather than a shared resource table.
var transitionColumns = []jira.Column[jira.Transition]{
	{Header: "ID", Value: func(t jira.Transition) string { return t.ID }},
	{Header: "NAME", Value: func(t jira.Transition) string { return t.Name }},
	{Header: "TO", Value: func(t jira.Transition) string {
		if t.To == nil {
			return ""
		}
		return t.To.Name
	}},
}

// newJiraIssueCommentCmd adds a comment, wrapping the text as ADF.
func newJiraIssueCommentCmd(f *cmdutil.Factory) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "comment <KEY>",
		Short: "Add a comment to an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if body == "" {
				return &cmdutil.ValidationError{Message: "--body is required"}
			}
			payload := map[string]any{"body": jira.Document(body)}
			resp, err := doJira(cmd.Context(), f, http.MethodPost, "/rest/api/3/issue/"+url.PathEscape(args[0])+"/comment", nil, payload)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return jiraJSON(f, resp)
			}
			return printAction(f, "Commented on", args[0])
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "comment text (stored as ADF)")
	return cmd
}

// doJira fetches the client for the active profile and performs one request.
// It centralizes the client-construction step every curated command shares.
func doJira(ctx context.Context, f *cmdutil.Factory, method, path string, query url.Values, reqBody any) (*atlapi.Response, error) {
	client, err := f.ClientFn()
	if err != nil {
		return nil, err
	}
	return client.Do(ctx, method, path, query, reqBody)
}

// singleValue resolves a value that may come from a flag or a lone positional
// argument, rejecting both-at-once and neither-given.
func singleValue(label, flagValue string, args []string) (string, error) {
	switch {
	case len(args) == 1 && flagValue != "":
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("provide the %s once, not both as an argument and a flag", label)}
	case len(args) == 1:
		return args[0], nil
	case flagValue != "":
		return flagValue, nil
	default:
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("a %s is required", label)}
	}
}

// requireFlags reports the first missing required flag by name.
func requireFlags(values map[string]string) error {
	for _, name := range []string{"project", "type", "summary"} {
		if v, ok := values[name]; ok && v == "" {
			return &cmdutil.ValidationError{Message: fmt.Sprintf("--%s is required", name)}
		}
	}
	return nil
}

// mergeFields parses -f key=value pairs (reusing the generic api command's
// JSON-typed value parsing) into an existing fields object.
func mergeFields(fields map[string]any, pairs []string) error {
	parsed, err := parseFields(pairs)
	if err != nil {
		return err
	}
	for k, v := range parsed {
		fields[k] = v
	}
	return nil
}

// printCreatedKey prints "<verb> <key>" from a create response, falling back
// to the raw response when no key is present.
func printCreatedKey(f *cmdutil.Factory, resp *atlapi.Response, verb string) error {
	var created struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(resp.Raw, &created)
	if created.Key == "" {
		return jiraJSON(f, resp)
	}
	return printAction(f, verb, created.Key)
}

// printAction prints a one-line confirmation unless quiet.
func printAction(f *cmdutil.Factory, verb, subject string) error {
	if f.Quiet {
		return nil
	}
	_, err := fmt.Fprintf(f.IOStreams.Out, "%s %s\n", verb, subject)
	return err
}
