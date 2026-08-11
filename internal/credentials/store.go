// Package credentials stores and retrieves Atlassian Cloud login secrets,
// backed by the OS keychain with a plaintext-file fallback. It is kept
// separate from internal/config so secrets never land in the preferences
// file.
package credentials

import "errors"

// Credentials are the values needed to authenticate against Atlassian
// Cloud: the site host, the account email, and an API token. Auth is HTTP
// Basic email:APIToken; one token serves both Jira and Confluence on a
// site. Site and Email are not themselves secret, but are kept alongside
// the token so a stored entry is self-contained.
type Credentials struct {
	Site     string
	Email    string
	APIToken string
}

// Store persists and retrieves Credentials per named profile.
type Store interface {
	Get(profile string) (Credentials, error)
	Set(profile string, creds Credentials) error
	Delete(profile string) error
}

// ErrNotFound indicates no credentials are stored for the requested
// profile.
var ErrNotFound = errors.New("no credentials stored for profile")
