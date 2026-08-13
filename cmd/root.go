// Package cmd wires atl's Cobra command tree to a cmdutil.Factory;
// commands hold no business logic beyond flag parsing and calling into
// internal packages.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/config"
	"github.com/branow/atlassian-cli/internal/credentials"
	"github.com/branow/atlassian-cli/internal/iostreams"
)

// Execute builds the real Factory and runs the CLI, returning any error
// for main.go to translate into a process exit code.
func Execute() error {
	f, err := newRealFactory()
	if err != nil {
		return err
	}
	return NewRootCmd(f).Execute()
}

// NewRootCmd builds the root "atl" command and its full subcommand tree
// around f.
func NewRootCmd(f *cmdutil.Factory) *cobra.Command {
	var noColor bool

	root := &cobra.Command{
		Use:           "atl",
		Short:         "Command-line interface for Atlassian Cloud (Jira + Confluence)",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if f.Output != "" && !validOutputFormat(f.Output) {
				return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --output %q (expected table or json)", f.Output)}
			}
			f.IOStreams.SetNoColor(noColor)
			f.IOStreams.SetNoInput(f.NoInput)
			return nil
		},
	}

	root.PersistentFlags().StringVarP(&f.Output, "output", "o", "", "output format: table|json")
	root.PersistentFlags().StringVarP(&f.Profile, "profile", "p", "", "profile to use")
	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	root.PersistentFlags().BoolVar(&f.NoInput, "no-input", false, "disable interactive prompts")
	root.PersistentFlags().BoolVarP(&f.Quiet, "quiet", "q", false, "suppress non-essential output")
	// No -f shorthand here: "api" already uses -f for --field, and pflag
	// panics on a shorthand collision when persistent flags are merged in.
	root.PersistentFlags().BoolVar(&f.Force, "force", false, "skip confirmation prompts")

	root.AddCommand(
		newAuthCmd(f),
		newConfigCmd(f),
		newAPICmd(f, ""),
		newJiraCmd(f),
		newConfluenceCmd(f),
		newCompletionCmd(),
		newVersionCmd(f),
	)
	return root
}

func newRealFactory() (*cmdutil.Factory, error) {
	streams := iostreams.System()
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	credPath, err := credentials.DefaultPlaintextPath()
	if err != nil {
		return nil, err
	}

	f := &cmdutil.Factory{
		IOStreams:        streams,
		Config:           cfg,
		CredentialsStore: credentials.NewKeyringStore(streams, credPath),
	}
	f.ClientFn = func() (atlapi.Client, error) {
		return newClient(f)
	}
	return f, nil
}

// newClient constructs the real API client from the active profile's stored
// credentials. The base URL is derived from the credentials' site host.
func newClient(f *cmdutil.Factory) (atlapi.Client, error) {
	profile := f.ActiveProfile()
	creds, err := f.CredentialsStore.Get(profile)
	if err != nil {
		return nil, cmdutil.ErrNotLoggedIn
	}
	baseURL, err := atlapi.BaseURL(creds.Site)
	if err != nil {
		return nil, err
	}
	return atlapi.New(baseURL, creds), nil
}
