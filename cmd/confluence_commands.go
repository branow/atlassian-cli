package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// addConfluenceCommands attaches the curated Confluence subcommands to the
// confluence group. It is the single registration point so the group wiring
// stays in one place; individual command constructors live in
// cmd/confluence_*.go.
func addConfluenceCommands(cmd *cobra.Command, f *cmdutil.Factory) {
	cmd.AddCommand(
		newConfluenceSpaceCmd(f),
		newConfluencePageCmd(f),
		newConfluenceSearchCmd(f),
		newConfluenceAttachCmd(f),
		newConfluenceCommentCmd(f),
	)
}
