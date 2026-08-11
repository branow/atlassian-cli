package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/output"
)

const (
	keyOutput = "output"
	keySite   = "site"
	keyEmail  = "email"
)

// newConfigCmd groups the config get/set/list subcommands for reading and
// writing atl's non-secret preferences.
func newConfigCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage atl configuration",
	}
	cmd.AddCommand(newConfigGetCmd(f), newConfigSetCmd(f), newConfigListCmd(f))
	return cmd
}

func newConfigGetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print a configuration value for the active profile",
		Args:  cobra.ExactArgs(1),
		Example: `  atl config get site
  atl config get output
  atl config get --profile sandbox email`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(f, args[0])
		},
	}
}

func newConfigSetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value for the active profile",
		Args:  cobra.ExactArgs(2),
		Example: `  atl config set output json
  atl config set site your-org.atlassian.net
  atl config set --profile sandbox email you@example.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(f, args[0], args[1])
		},
	}
}

func newConfigListCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured profiles",
		Example: `  atl config list
  atl config list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigList(f)
		},
	}
}

func runConfigGet(f *cmdutil.Factory, key string) error {
	value, err := configValue(f, f.ActiveProfile(), key)
	if err != nil {
		return err
	}
	fmt.Fprintln(f.IOStreams.Out, value)
	return nil
}

func configValue(f *cmdutil.Factory, profile, key string) (string, error) {
	switch key {
	case keyOutput:
		return f.Config.OutputFormat("", profile), nil
	case keySite:
		return f.Config.Site("", profile), nil
	case keyEmail:
		return f.Config.Email("", profile), nil
	default:
		return "", unknownConfigKeyError(key)
	}
}

// validOutputFormat reports whether value names a supported -o/--output
// format.
func validOutputFormat(value string) bool {
	return value == "table" || value == "json"
}

// normalizeSite trims surrounding whitespace and a trailing slash from a
// site value, leaving any scheme intact (a scheme-qualified site doubles
// as an explicit base URL for local proxies and tests).
func normalizeSite(site string) string {
	return strings.TrimRight(strings.TrimSpace(site), "/")
}

// validateSite rejects values that cannot be a site host at all: empty or
// containing internal whitespace.
func validateSite(site string) error {
	if site == "" || strings.ContainsAny(site, " \t") {
		return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid site %q (expected a host like your-org.atlassian.net)", site)}
	}
	return nil
}

func runConfigSet(f *cmdutil.Factory, key, value string) error {
	profile := f.ActiveProfile()
	p := f.Config.Profiles[profile]
	switch key {
	case keyOutput:
		if !validOutputFormat(value) {
			return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid output format %q (expected table or json)", value)}
		}
		p.Output = value
	case keySite:
		value = normalizeSite(value)
		if err := validateSite(value); err != nil {
			return err
		}
		p.Site = value
	case keyEmail:
		p.Email = value
	default:
		return unknownConfigKeyError(key)
	}
	f.Config.SetProfile(profile, p)
	return f.Config.Save()
}

func runConfigList(f *cmdutil.Factory) error {
	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, f.Config.Profiles)
	}

	current := f.ActiveProfile()
	names := make([]string, 0, len(f.Config.Profiles))
	for name := range f.Config.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([][]string, 0, len(names))
	for _, name := range names {
		p := f.Config.Profiles[name]
		marker := ""
		if name == current {
			marker = "*"
		}
		rows = append(rows, []string{marker, name, p.Site, p.Email, p.Output})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"", "PROFILE", "SITE", "EMAIL", "OUTPUT"}, rows)
}

func unknownConfigKeyError(key string) error {
	return &cmdutil.ValidationError{Message: fmt.Sprintf("unknown config key %q (expected one of: output, site, email)", key)}
}
