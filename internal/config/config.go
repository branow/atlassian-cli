// Package config manages atl's non-secret preferences: named profiles
// (Atlassian site, account email, output format) and which one is active,
// resolved with flag > env var > config file > built-in default
// precedence. Credentials never live here; see internal/credentials.
package config

import "os"

// Profile holds the non-secret settings for one named Atlassian site.
// The API token is a secret and is never stored here (see
// internal/credentials); Site and Email are mirrored here only so
// commands like "auth status" and client construction can read them
// without unlocking the OS keychain.
type Profile struct {
	Site   string `yaml:"site"`
	Email  string `yaml:"email"`
	Output string `yaml:"output"`
}

// Config is the on-disk preferences file: the active profile name and the
// set of named profiles.
type Config struct {
	CurrentProfileName string             `yaml:"current_profile"`
	Profiles           map[string]Profile `yaml:"profiles"`
}

const (
	defaultProfileName = "default"
	defaultOutput      = "table"
)

// New returns an empty Config, as used when no config file exists yet.
func New() *Config {
	return &Config{Profiles: map[string]Profile{}}
}

// CurrentProfile resolves the active profile name: flagOverride (if
// non-empty), then ATL_PROFILE, then the config file's stored value,
// then "default".
func (c *Config) CurrentProfile(flagOverride string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("ATL_PROFILE"); v != "" {
		return v
	}
	if c.CurrentProfileName != "" {
		return c.CurrentProfileName
	}
	return defaultProfileName
}

// SetProfile stores profile as the named profile's settings. It never
// changes which profile is active — activation is a separate, explicit
// step via SwitchProfile.
func (c *Config) SetProfile(name string, profile Profile) {
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	c.Profiles[name] = profile
}

// ProfileOutput returns the output format stored for a profile, or "" when
// the profile is absent or has no explicit preference. It lets a caller
// persist a profile while leaving its output preference untouched.
func (c *Config) ProfileOutput(name string) string {
	return c.Profiles[name].Output
}

// SwitchProfile makes name the active profile without changing its stored
// settings. It fails if the profile has never been configured.
func (c *Config) SwitchProfile(name string) error {
	if _, ok := c.Profiles[name]; !ok {
		return &ProfileNotFoundError{Name: name}
	}
	c.CurrentProfileName = name
	return nil
}

// ProfileNotFoundError reports that a named profile has no stored
// settings.
type ProfileNotFoundError struct{ Name string }

func (e *ProfileNotFoundError) Error() string {
	return "profile not found: " + e.Name
}

// Site resolves the Atlassian site host for profile: flagOverride, then
// ATL_SITE, then the config file's stored value. It is a bare host such
// as "your-org.atlassian.net" (no scheme).
func (c *Config) Site(flagOverride, profile string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("ATL_SITE"); v != "" {
		return v
	}
	return c.Profiles[profile].Site
}

// Email resolves the Atlassian account email for profile: flagOverride,
// then ATL_EMAIL, then the config file's stored value.
func (c *Config) Email(flagOverride, profile string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("ATL_EMAIL"); v != "" {
		return v
	}
	return c.Profiles[profile].Email
}

// OutputFormat resolves the output format for profile: flagOverride, then
// ATL_OUTPUT, then the config file's stored value, then "table". A source
// holding an unrecognized value is skipped rather than silently disabling
// table rendering, so a stale env var or a hand-edited config never leaves
// the CLI in a no-output state.
func (c *Config) OutputFormat(flagOverride, profile string) string {
	if validOutput(flagOverride) {
		return flagOverride
	}
	if v := os.Getenv("ATL_OUTPUT"); validOutput(v) {
		return v
	}
	if p, ok := c.Profiles[profile]; ok && validOutput(p.Output) {
		return p.Output
	}
	return defaultOutput
}

// validOutput reports whether v is a supported output format.
func validOutput(v string) bool {
	return v == "table" || v == "json"
}
