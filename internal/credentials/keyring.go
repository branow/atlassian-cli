package credentials

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/branow/gokey"

	"github.com/branow/atlassian-cli/internal/iostreams"
)

const keyringService = "atlassian-cli"

// KeyringStore persists credentials in the OS keychain, falling back to a
// plaintext file and warning on stderr when the machine has no keychain.
type KeyringStore struct {
	streams  *iostreams.IOStreams
	fallback *PlaintextStore

	// prompt carries whether a call may raise the OS authorization
	// dialog. It is decided once, where the terminal is known.
	prompt gokey.Option

	// The keychain calls are fields so tests drive every branch without
	// touching the machine's real keychain.
	get    func(service, account string, opts ...gokey.Option) (string, error)
	set    func(service, account, secret string, opts ...gokey.Option) error
	remove func(service, account string, opts ...gokey.Option) error
}

// NewKeyringStore returns a KeyringStore that warns via streams and falls
// back to fallbackPath when the machine has no OS keychain. The keychain
// may raise its authorization dialog only when a human is at the terminal:
// unattended, a dialog nobody can see would hang the command instead of
// failing it.
func NewKeyringStore(streams *iostreams.IOStreams, fallbackPath string) *KeyringStore {
	return &KeyringStore{
		streams:  streams,
		fallback: NewPlaintextStore(fallbackPath),
		prompt:   gokey.WithPrompt(streams.CanPrompt()),
		get:      gokey.Get,
		set:      gokey.Set,
		remove:   gokey.Delete,
	}
}

// Get returns the stored credentials for profile. A profile absent from
// the keychain is also looked up in the plaintext fallback — its entry may
// have been written there while the keychain was unavailable.
func (s *KeyringStore) Get(profile string) (Credentials, error) {
	value, err := s.get(keyringService, profile, s.prompt)
	switch {
	case errors.Is(err, gokey.ErrNotFound):
		return s.fallback.Get(profile)
	case errors.Is(err, gokey.ErrBlocked):
		return Credentials{}, blockedError("read", profile, err)
	case errors.Is(err, gokey.ErrUnavailable):
		s.warnFallback(err)
		return s.fallback.Get(profile)
	case err != nil:
		return Credentials{}, err
	}
	if plain, legacy, err := decodeLegacy(value); legacy {
		if err != nil {
			return Credentials{}, fmt.Errorf("decoding keychain entry for profile %q: %w", profile, err)
		}
		creds, err := decodeCredentials(plain)
		if err != nil {
			return Credentials{}, err
		}
		// Store it again in the current layout. The rewrite also binds the
		// item to this binary, so the next read raises no dialog; failing it
		// costs only that, so the read still succeeds.
		_ = s.Set(profile, creds)
		return creds, nil
	}
	return decodeCredentials(value)
}

// Set stores creds for profile, overwriting any existing entry.
func (s *KeyringStore) Set(profile string, creds Credentials) error {
	value, err := encodeCredentials(creds)
	if err != nil {
		return err
	}
	switch err := s.set(keyringService, profile, value, s.prompt); {
	case errors.Is(err, gokey.ErrBlocked):
		return blockedError("replace", profile, err)
	case errors.Is(err, gokey.ErrUnavailable):
		s.warnFallback(err)
		return s.fallback.Set(profile, creds)
	case err != nil:
		return err
	}
	return nil
}

// Delete removes the stored credentials for profile, from the plaintext
// fallback when the keychain has no entry (see Get).
func (s *KeyringStore) Delete(profile string) error {
	switch err := s.remove(keyringService, profile, s.prompt); {
	case errors.Is(err, gokey.ErrNotFound):
		return s.fallback.Delete(profile)
	case errors.Is(err, gokey.ErrBlocked):
		return blockedError("delete", profile, err)
	case errors.Is(err, gokey.ErrUnavailable):
		s.warnFallback(err)
		return s.fallback.Delete(profile)
	case err != nil:
		return err
	}
	return nil
}

func (s *KeyringStore) warnFallback(cause error) {
	fmt.Fprintf(s.streams.ErrOut, "warning: OS keychain unavailable (%v); falling back to plaintext credential storage\n", cause)
}

// blockedError reports the one keychain failure the user can act on: the
// entry exists but its access control does not name this binary, which is
// what every entry written by the go-keyring versions of this CLI looks
// like (they shelled out to /usr/bin/security). On a terminal the user can
// approve the dialog; otherwise deleting the entry and logging in again
// stores it under this binary's own identity.
func blockedError(action, profile string, cause error) error {
	return fmt.Errorf("cannot %s the keychain entry for profile %q: %w; approve the keychain prompt on an interactive terminal, or delete the entry with: security delete-generic-password -s %s -a %s, then run: atl auth login --profile %s",
		action, profile, cause, keyringService, profile, profile)
}

// The prefixes go-keyring put on every value it stored on macOS, which is
// every entry written before atl moved to gokey. gokey returns the stored
// bytes as they are, so they are decoded here.
const (
	legacyBase64Prefix = "go-keyring-base64:"
	legacyHexPrefix    = "go-keyring-encoded:"
)

// decodeLegacy reports whether value is in go-keyring's macOS layout and, if
// so, returns the value it encodes.
func decodeLegacy(value string) (string, bool, error) {
	trimmed := strings.TrimSpace(value)
	if rest, ok := strings.CutPrefix(trimmed, legacyBase64Prefix); ok {
		decoded, err := base64.StdEncoding.DecodeString(rest)
		return string(decoded), true, err
	}
	if rest, ok := strings.CutPrefix(trimmed, legacyHexPrefix); ok {
		decoded, err := hex.DecodeString(rest)
		return string(decoded), true, err
	}
	return value, false, nil
}

func encodeCredentials(c Credentials) (string, error) {
	data, err := json.Marshal(c)
	return string(data), err
}

func decodeCredentials(value string) (Credentials, error) {
	var c Credentials
	err := json.Unmarshal([]byte(value), &c)
	return c, err
}
