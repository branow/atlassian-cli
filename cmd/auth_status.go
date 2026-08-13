package cmd

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/output"
)

// newAuthStatusCmd builds "atl auth status", listing every configured profile
// that has stored credentials — the site and email it authenticates as — with
// the active profile marked. It errors only when no profile is logged in.
func newAuthStatusCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "List authenticated profiles",
		Example: `  atl auth status
  atl auth status -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus(f)
		},
	}
}

// profileStatus is one row of "auth status": a logged-in profile, whether it is
// the active one, and the site/email it authenticates as.
type profileStatus struct {
	Profile string `json:"profile"`
	Active  bool   `json:"active"`
	Site    string `json:"site"`
	Email   string `json:"email"`
}

func runAuthStatus(f *cmdutil.Factory) error {
	active := f.ActiveProfile()

	// List every configured profile, plus the active one in case it was named
	// via -p/ATL_PROFILE without a config entry. Sorted for stable output.
	names := make([]string, 0, len(f.Config.Profiles)+1)
	seen := map[string]bool{}
	for name := range f.Config.Profiles {
		names = append(names, name)
		seen[name] = true
	}
	if !seen[active] {
		names = append(names, active)
	}
	sort.Strings(names)

	statuses := make([]profileStatus, 0, len(names))
	for _, name := range names {
		// A profile present in config but without credentials is logged out;
		// "auth status" reports authentication, so skip it rather than list a
		// blank row.
		creds, err := f.CredentialsStore.Get(name)
		if err != nil {
			continue
		}
		statuses = append(statuses, profileStatus{
			Profile: name,
			Active:  name == active,
			Site:    creds.Site,
			Email:   creds.Email,
		})
	}
	if len(statuses) == 0 {
		return cmdutil.ErrNotLoggedIn
	}

	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, statuses)
	}
	rows := make([][]string, 0, len(statuses))
	for _, s := range statuses {
		marker := ""
		if s.Active {
			marker = "*"
		}
		rows = append(rows, []string{marker, s.Profile, s.Site, s.Email})
	}
	return output.WriteTable(f.IOStreams.Out,
		[]string{"", "PROFILE", "SITE", "EMAIL"},
		rows,
	)
}
