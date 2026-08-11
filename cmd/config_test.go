package cmd_test

import (
	"strings"
	"testing"

	"github.com/branow/atlassian-cli/cmd"
	"github.com/branow/atlassian-cli/internal/cmdutil"
)

func TestConfigSetGetRoundTrip(t *testing.T) {
	f, out, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "site", "acme.atlassian.net"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "get", "site"})
	if err := root.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := out.String(); got != "acme.atlassian.net\n" {
		t.Errorf("got %q, want %q", got, "acme.atlassian.net\n")
	}
}

func TestConfigSetDoesNotSwitchActiveProfile(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "--profile", "sandbox", "output", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := f.Config.CurrentProfile(""); got != "default" {
		t.Errorf("config set switched the active profile to %q", got)
	}
	if got := f.Config.OutputFormat("", "sandbox"); got != "json" {
		t.Errorf("got output %q for profile sandbox, want json", got)
	}
}

func TestConfigSetRejectsInvalidOutputFormat(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "output", "yaml"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for an unsupported output format")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestConfigSetRejectsMalformedSite(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "site", "has a space.atlassian.net"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for a site with whitespace")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestConfigGetUnknownKey(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "get", "nonsense"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for an unknown config key")
	}
}

func TestConfigList(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	f.Config.SetProfile("sandbox", f.Config.Profiles["sandbox"])

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "sandbox") {
		t.Errorf("expected output to mention profile sandbox, got %q", out.String())
	}
}
