package cmd

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/jira"
)

// newJiraSprintCmd groups sprint commands (Jira Software agile API).
func newJiraSprintCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sprint",
		Short: "Work with Jira Software sprints",
	}
	cmd.AddCommand(
		newJiraSprintListCmd(f),
		newJiraSprintGetCmd(f),
	)
	return cmd
}

// newJiraSprintListCmd lists the sprints of a board. --board is required
// because sprints only exist in the context of a board.
func newJiraSprintListCmd(f *cmdutil.Factory) *cobra.Command {
	var board int
	var limit int
	var paginate bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List a board's sprints",
		Long: `List the sprints of a board (--board is required). --limit caps the
number returned; --paginate follows every page until exhausted (or --limit is
reached), otherwise only the first page is shown.`,
		Args:    cobra.NoArgs,
		Aliases: []string{"list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("board") {
				return &cmdutil.ValidationError{Message: "--board is required"}
			}
			path := "/rest/agile/1.0/board/" + strconv.Itoa(board) + "/sprint"
			sprints, err := jira.PaginateStartAt[jira.Sprint](pageFetcher(cmd, f, path, nil, limit), paginate, limit)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return jiraAggregateJSON(f, "values", sprints)
			}
			return jiraTable(f, jira.SprintColumns, sprints)
		},
	}
	cmd.Flags().IntVar(&board, "board", 0, "board id whose sprints to list (required)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of sprints to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow every page until exhausted or --limit is reached")
	return cmd
}

// newJiraSprintGetCmd fetches one sprint by id.
func newJiraSprintGetCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a sprint by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := doJira(cmd.Context(), f, http.MethodGet, "/rest/agile/1.0/sprint/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			if wantJSON(f) {
				return jiraJSON(f, resp)
			}
			var sprint jira.Sprint
			if err := json.Unmarshal(resp.Raw, &sprint); err != nil {
				return err
			}
			return jiraKeyValues(f, jira.SprintColumns, sprint)
		},
	}
	return cmd
}
