package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// newJiraCmd is the Jira command group (Jira Cloud platform + Jira
// Software). It hosts the namespaced generic invoker (atl jira api) and the
// curated Jira subcommands.
func newJiraCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jira",
		Short: "Work with Jira (Cloud platform + Jira Software)",
	}
	cmd.AddCommand(newAPICmd(f, "jira"))
	// Curated Jira subcommands are attached here (see cmd/jira_*.go).
	addJiraCommands(cmd, f)
	return cmd
}
