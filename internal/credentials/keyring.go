package credentials

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
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

	// dropLegacy deletes an entry go-keyring wrote; see Set.
	dropLegacy func(service, account string) error
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

		dropLegacy: deleteWithSecurityTool,
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
		return Credentials{}, blockedError(profile, err)
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
		s.migrate(profile, creds)
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
	err = s.set(keyringService, profile, value, s.prompt)
	if err != nil && !errors.Is(err, gokey.ErrBlocked) && !errors.Is(err, gokey.ErrUnavailable) {
		// The keychain will not let this binary replace an item another
		// program owns (errSecInvalidOwnerEdit) - every entry an older atl
		// wrote through /usr/bin/security. Its owner deletes it without
		// asking, and the new item is then bound to this binary.
		if s.dropLegacy(keyringService, profile) == nil {
			err = s.set(keyringService, profile, value, s.prompt)
		}
	}
	switch {
	case errors.Is(err, gokey.ErrBlocked):
		return blockedError(profile, err)
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
		return blockedError(profile, err)
	case errors.Is(err, gokey.ErrUnavailable):
		s.warnFallback(err)
		return s.fallback.Delete(profile)
	case err != nil:
		return err
	}
	return nil
}

// migrate stores a legacy entry again as plain JSON bound to this binary
// (see Set), so the next read raises no dialog. Failing costs only that
// dialog on later reads, so it warns instead of failing the read.
func (s *KeyringStore) migrate(profile string, creds Credentials) {
	if err := s.Set(profile, creds); err != nil {
		fmt.Fprintf(s.streams.ErrOut, "warning: could not re-save the login for profile %q, so macOS will keep asking for permission: %v\n", profile, err)
	}
}

// deleteWithSecurityTool deletes an item through /usr/bin/security, the
// owner of every entry go-keyring wrote. It only deletes: the item's
// address is passed, and no secret is read or written through it.
func deleteWithSecurityTool(service, account string) error {
	out, err := exec.Command("/usr/bin/security", "delete-generic-password", "-s", service, "-a", account).CombinedOutput()
	if err != nil {
		return fmt.Errorf("security delete-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *KeyringStore) warnFallback(cause error) {
	fmt.Fprintf(s.streams.ErrOut, "warning: OS keychain unavailable (%v); falling back to plaintext credential storage\n", cause)
}

// blockedError reports the keychain holding an entry this binary may not
// read yet: every entry an older atl wrote, and any entry after a build
// signed by another certificate. Only a person can grant it, by clicking
// Allow in the macOS dialog, which atl raises only on a terminal; so the
// error says exactly that and nothing else. gokey's cause is kept for
// errors.Is but left out of the message.
func blockedError(profile string, cause error) error {
	return &approvalError{profile: profile, cause: cause}
}

type approvalError struct {
	profile string
	cause   error
}

func (e *approvalError) Error() string {
	return fmt.Sprintf(`macOS needs your permission before atl can use the saved login for profile %q.
This is a one-time step, usually after updating atl. Run this in a terminal and click "Allow":

  atl auth status`, e.profile)
}

func (e *approvalError) Unwrap() []error { return []error{ErrNeedsApproval, e.cause} }

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
