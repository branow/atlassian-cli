package cmd

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/jira"
)

// newJiraProjectCmd groups project commands (Jira platform).
func newJiraProjectCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Work with Jira projects",
	}
	cmd.AddCommand(newJiraProjectListCmd(f))
	return cmd
}

// newJiraProjectListCmd lists projects visible to the caller via the
// paginated project/search endpoint.
func newJiraProjectListCmd(f *cmdutil.Factory) *cobra.Command {
	var limit int
	var paginate bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List projects",
		Long: `List projects visible to the caller. --limit caps the number returned;
--paginate follows every page until exhausted (or --limit is reached),
otherwise only the first page is shown.`,
		Args:    cobra.NoArgs,
		Aliases: []string{"list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			projects, err := jira.PaginateStartAt[jira.Project](pageFetcher(cmd, f, "/rest/api/3/project/search", nil, limit), paginate, limit)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return jiraAggregateJSON(f, "values", projects)
			}
			return jiraTable(f, jira.ProjectColumns, projects)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of projects to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow every page until exhausted or --limit is reached")
	return cmd
}

// pageFetcher builds the offset-paginated fetch callback PaginateStartAt
// drives: it GETs path with the base query plus the running startAt offset
// (omitted on the first page) and, when a limit is set, a matching maxResults
// page-size hint, returning the page's raw body. The four list commands share
// it so their startAt loops are identical.
func pageFetcher(cmd *cobra.Command, f *cmdutil.Factory, path string, base url.Values, limit int) func(int) ([]byte, error) {
	return func(startAt int) ([]byte, error) {
		query := url.Values{}
		for k, vs := range base {
			query[k] = vs
		}
		if startAt > 0 {
			query.Set("startAt", strconv.Itoa(startAt))
		}
		if limit > 0 {
			query.Set("maxResults", strconv.Itoa(limit))
		}
		resp, err := doJira(cmd.Context(), f, http.MethodGet, path, query, nil)
		if err != nil {
			return nil, err
		}
		return resp.Raw, nil
	}
}
