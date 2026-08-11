package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// newAuthCmd groups the auth login/logout/status/switch subcommands.
func newAuthCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and credentials",
	}
	cmd.AddCommand(
		newAuthLoginCmd(f),
		newAuthLogoutCmd(f),
		newAuthStatusCmd(f),
		newAuthSwitchCmd(f),
	)
	return cmd
}
