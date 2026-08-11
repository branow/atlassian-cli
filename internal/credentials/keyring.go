package credentials

import (
	"encoding/json"
	"errors"
	"fmt"

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
		s.warnFallback(err)
		return s.fallback.Delete(profile)
	}
	return nil
}

func (s *KeyringStore) warnFallback(cause error) {
	fmt.Fprintf(s.streams.ErrOut, "warning: OS keychain unavailable (%v); falling back to plaintext credential storage\n", cause)
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
