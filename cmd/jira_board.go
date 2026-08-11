package cmd

import (
	"net/url"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/jira"
)

// newJiraBoardCmd groups board commands (Jira Software agile API).
func newJiraBoardCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "board",
		Short: "Work with Jira Software boards",
	}
	cmd.AddCommand(newJiraBoardListCmd(f))
	return cmd
}

// newJiraBoardListCmd lists agile boards, optionally scoped to a project.
func newJiraBoardListCmd(f *cmdutil.Factory) *cobra.Command {
	var project string
	var limit int
	var paginate bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List boards",
		Long: `List agile boards, optionally scoped to a project. --limit caps the
number returned; --paginate follows every page until exhausted (or --limit is
reached), otherwise only the first page is shown.`,
		Args:    cobra.NoArgs,
		Aliases: []string{"list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			base := url.Values{}
			if project != "" {
				base.Set("projectKeyOrId", project)
			}
			boards, err := jira.PaginateStartAt[jira.Board](pageFetcher(cmd, f, "/rest/agile/1.0/board", base, limit), paginate, limit)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return jiraAggregateJSON(f, "values", boards)
			}
			return jiraTable(f, jira.BoardColumns, boards)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "filter boards by project key or id")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of boards to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow every page until exhausted or --limit is reached")
	return cmd
}
