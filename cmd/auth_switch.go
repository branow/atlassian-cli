package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// newAuthSwitchCmd builds "atl auth switch", changing the active profile
// stored in the config file. It reuses the global --profile flag to name
// the target profile.
func newAuthSwitchCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "switch",
		Short: "Switch the active profile",
		Example: `  atl auth switch --profile sandbox
  atl auth switch --profile production`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthSwitch(f)
		},
	}
}

func runAuthSwitch(f *cmdutil.Factory) error {
	if f.Profile == "" {
		return &cmdutil.ValidationError{Message: "--profile is required"}
	}
	if err := f.Config.SwitchProfile(f.Profile); err != nil {
		return &cmdutil.ValidationError{Message: err.Error()}
	}
	if err := f.Config.Save(); err != nil {
		return err
	}
	if !f.Quiet {
		fmt.Fprintf(f.IOStreams.Out, "Switched to profile %q\n", f.Profile)
	}
	return nil
}
