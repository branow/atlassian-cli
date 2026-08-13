package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/branow/atlassian-cli/internal/iostreams"
)

const keyringService = "atlassian-cli"

// KeyringStore persists credentials in the OS keychain, falling back to a
// plaintext file and warning on stderr when no keychain backend is
// available.
type KeyringStore struct {
	streams  *iostreams.IOStreams
	fallback *PlaintextStore
}

// NewKeyringStore returns a KeyringStore that warns via streams and falls
// back to fallbackPath when the OS keychain is unavailable.
func NewKeyringStore(streams *iostreams.IOStreams, fallbackPath string) *KeyringStore {
	return &KeyringStore{streams: streams, fallback: NewPlaintextStore(fallbackPath)}
}

// Get returns the stored credentials for profile. A profile absent from
// the keychain is also looked up in the plaintext fallback — its entry may
// have been written there while the keychain was unavailable.
func (s *KeyringStore) Get(profile string) (Credentials, error) {
	value, err := keyring.Get(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return s.fallback.Get(profile)
	}
	if err != nil {
		if !safeToFallback(err) {
			return Credentials{}, keychainError("reading", err)
		}
		s.warnFallback(err)
		return s.fallback.Get(profile)
	}
	return decodeCredentials(value)
}

// Set stores creds for profile, overwriting any existing entry.
func (s *KeyringStore) Set(profile string, creds Credentials) error {
	value, err := encodeCredentials(creds)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringService, profile, value); err != nil {
		if !safeToFallback(err) {
			return keychainError("storing", err)
		}
		s.warnFallback(err)
		return s.fallback.Set(profile, creds)
	}
	return nil
}

// Delete removes the stored credentials for profile, from the plaintext
// fallback when the keychain has no entry (see Get).
func (s *KeyringStore) Delete(profile string) error {
	err := keyring.Delete(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return s.fallback.Delete(profile)
	}
	if err != nil {
		if !safeToFallback(err) {
			return keychainError("deleting", err)
		}
		s.warnFallback(err)
		return s.fallback.Delete(profile)
	}
	return nil
}

func (s *KeyringStore) warnFallback(cause error) {
	fmt.Fprintf(s.streams.ErrOut, "warning: OS keychain unavailable (%v); falling back to plaintext credential storage\n", cause)
}

// safeToFallback reports whether a keyring error is a safe cue to fall back
// to the plaintext file. Only "no keychain backend on this platform" qualifies
// by default: a backend that is present but errored (a locked keychain, a
// denied permission prompt, a dbus hiccup) is transient, and silently writing
// the API token to disk in that case would leak the secret unnoticed in a
// scripted run. Setting ATL_ALLOW_PLAINTEXT_KEYRING=1 restores the old
// fall-back-on-any-error behavior for users who want it.
func safeToFallback(err error) bool {
	if errors.Is(err, keyring.ErrUnsupportedPlatform) {
		return true
	}
	v := os.Getenv("ATL_ALLOW_PLAINTEXT_KEYRING")
	return v == "1" || strings.EqualFold(v, "true")
}

func keychainError(action string, cause error) error {
	return fmt.Errorf("OS keychain error %s credentials (set ATL_ALLOW_PLAINTEXT_KEYRING=1 to fall back to plaintext file storage): %w", action, cause)
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
