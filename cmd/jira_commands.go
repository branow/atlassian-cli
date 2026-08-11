package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// addJiraCommands attaches the curated Jira subcommands to the jira group.
// It is the single registration point so the group wiring stays in one
// place; individual command constructors live in cmd/jira_*.go.
func addJiraCommands(cmd *cobra.Command, f *cmdutil.Factory) {
	cmd.AddCommand(
		newJiraIssueCmd(f),
		newJiraProjectCmd(f),
		newJiraBoardCmd(f),
		newJiraSprintCmd(f),
	)
}
