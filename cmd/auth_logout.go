package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/credentials"
)

// newAuthLogoutCmd builds "atl auth logout", removing stored credentials
// for a profile.
func newAuthLogoutCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials for a profile",
		Example: `  atl auth logout
  atl auth logout --profile sandbox`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogout(f)
		},
	}
}

func runAuthLogout(f *cmdutil.Factory) error {
	profile := f.ActiveProfile()
	if err := f.CredentialsStore.Delete(profile); err != nil && !errors.Is(err, credentials.ErrNotFound) {
		return err
	}
	if !f.Quiet {
		fmt.Fprintf(f.IOStreams.Out, "Logged out of profile %q\n", profile)
	}
	return nil
}
