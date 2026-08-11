package cmd

import (
	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/output"
)

// newAuthStatusCmd builds "atl auth status", reporting whether the active
// profile has stored credentials and, if so, the site and email they
// authenticate as.
func newAuthStatusCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status for a profile",
		Example: `  atl auth status
  atl auth status --profile sandbox
  atl auth status -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus(f)
		},
	}
}

func runAuthStatus(f *cmdutil.Factory) error {
	profile := f.ActiveProfile()
	creds, err := f.CredentialsStore.Get(profile)
	if err != nil {
		return cmdutil.ErrNotLoggedIn
	}

	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, struct {
			Profile string `json:"profile"`
			Site    string `json:"site"`
			Email   string `json:"email"`
		}{profile, creds.Site, creds.Email})
	}
	return output.WriteTable(f.IOStreams.Out,
		[]string{"PROFILE", "SITE", "EMAIL"},
		[][]string{{profile, creds.Site, creds.Email}},
	)
}
