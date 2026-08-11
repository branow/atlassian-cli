package config_test

import (
	"path/filepath"
	"testing"

	"github.com/branow/atlassian-cli/internal/config"
)

func TestCurrentProfilePrecedence(t *testing.T) {
	c := config.New()
	c.CurrentProfileName = "from-file"

	if got := c.CurrentProfile(""); got != "from-file" {
		t.Errorf("no overrides: got %q, want %q", got, "from-file")
	}

	t.Setenv("ATL_PROFILE", "from-env")
	if got := c.CurrentProfile(""); got != "from-env" {
		t.Errorf("env override: got %q, want %q", got, "from-env")
	}

	if got := c.CurrentProfile("from-flag"); got != "from-flag" {
		t.Errorf("flag override: got %q, want %q", got, "from-flag")
	}
}

func TestCurrentProfileDefault(t *testing.T) {
	c := config.New()
	if got := c.CurrentProfile(""); got != "default" {
		t.Errorf("got %q, want %q", got, "default")
	}
}

func TestSitePrecedence(t *testing.T) {
	c := config.New()
	c.SetProfile("work", config.Profile{Site: "file.atlassian.net"})

	if got := c.Site("", "work"); got != "file.atlassian.net" {
		t.Errorf("file value: got %q", got)
	}

	t.Setenv("ATL_SITE", "env.atlassian.net")
	if got := c.Site("", "work"); got != "env.atlassian.net" {
		t.Errorf("env override: got %q", got)
	}

	if got := c.Site("flag.atlassian.net", "work"); got != "flag.atlassian.net" {
		t.Errorf("flag override: got %q", got)
	}
}

func TestEmailPrecedence(t *testing.T) {
	c := config.New()
	c.SetProfile("work", config.Profile{Email: "file@example.com"})

	if got := c.Email("", "work"); got != "file@example.com" {
		t.Errorf("file value: got %q", got)
	}

	t.Setenv("ATL_EMAIL", "env@example.com")
	if got := c.Email("", "work"); got != "env@example.com" {
		t.Errorf("env override: got %q", got)
	}

	if got := c.Email("flag@example.com", "work"); got != "flag@example.com" {
		t.Errorf("flag override: got %q", got)
	}
}

func TestOutputFormatPrecedenceAndDefault(t *testing.T) {
	c := config.New()

	if got := c.OutputFormat("", "missing"); got != "table" {
		t.Errorf("default: got %q, want %q", got, "table")
	}

	c.SetProfile("work", config.Profile{Output: "json"})
	if got := c.OutputFormat("", "work"); got != "json" {
		t.Errorf("file value: got %q", got)
	}

	t.Setenv("ATL_OUTPUT", "table")
	if got := c.OutputFormat("", "work"); got != "table" {
		t.Errorf("env override: got %q", got)
	}

	if got := c.OutputFormat("json", "work"); got != "json" {
		t.Errorf("flag override: got %q", got)
	}
}

func TestSetProfileDoesNotSwitchActiveProfile(t *testing.T) {
	c := config.New()
	c.SetProfile("work", config.Profile{Site: "work.atlassian.net"})
	if got := c.CurrentProfile(""); got != "default" {
		t.Errorf("SetProfile changed the active profile to %q", got)
	}
}

func TestSwitchProfileRequiresExistingProfile(t *testing.T) {
	c := config.New()
	if err := c.SwitchProfile("missing"); err == nil {
		t.Fatal("expected error switching to an unconfigured profile")
	}

	c.SetProfile("work", config.Profile{Site: "work.atlassian.net"})
	c.SetProfile("home", config.Profile{Site: "home.atlassian.net"})
	if err := c.SwitchProfile("work"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.CurrentProfileName != "work" {
		t.Errorf("got current profile %q, want %q", c.CurrentProfileName, "work")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(loaded.Profiles) != 0 {
		t.Fatalf("expected no profiles, got %v", loaded.Profiles)
	}

	loaded.SetProfile("sandbox", config.Profile{
		Site:   "sandbox.atlassian.net",
		Email:  "dev@example.com",
		Output: "json",
	})
	if err := loaded.SwitchProfile("sandbox"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}
	if err := loaded.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := config.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if filepath.Base(path) != "config.yml" {
		t.Fatalf("unexpected config path: %s", path)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if reloaded.CurrentProfileName != "sandbox" {
		t.Errorf("got current profile %q, want %q", reloaded.CurrentProfileName, "sandbox")
	}
	got := reloaded.Profiles["sandbox"]
	want := config.Profile{Site: "sandbox.atlassian.net", Email: "dev@example.com", Output: "json"}
	if got != want {
		t.Errorf("got profile %+v, want %+v", got, want)
	}
}
