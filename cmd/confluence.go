package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// newConfluenceCmd is the Confluence command group, spanning the v1 and v2
// REST APIs. It hosts the namespaced generic invoker (atl confluence api)
// and the curated Confluence subcommands.
func newConfluenceCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "confluence",
		Short: "Work with Confluence (v1 + v2 REST APIs)",
	}
	cmd.AddCommand(newAPICmd(f, "confluence"))
	// Curated Confluence subcommands are attached here (see cmd/confluence_*.go).
	addConfluenceCommands(cmd, f)
	return cmd
}
