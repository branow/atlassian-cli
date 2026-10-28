package credentials

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/branow/gokey"

	"github.com/branow/atlassian-cli/internal/iostreams"
)

// fakeKeychain stands in for the OS keychain: a map, plus an error every
// operation returns instead of touching it. That is what drives the
// fallback and authorization branches with no real keychain present.
type fakeKeychain struct {
	items map[string]string
	err   error
}

func newFakeKeychain() *fakeKeychain {
	return &fakeKeychain{items: map[string]string{}}
}

func (k *fakeKeychain) get(_, account string, _ ...gokey.Option) (string, error) {
	if k.err != nil {
		return "", k.err
	}
	value, ok := k.items[account]
	if !ok {
		return "", gokey.ErrNotFound
	}
	return value, nil
}

func (k *fakeKeychain) set(_, account, secret string, _ ...gokey.Option) error {
	if k.err != nil {
		return k.err
	}
	k.items[account] = secret
	return nil
}

func (k *fakeKeychain) remove(_, account string, _ ...gokey.Option) error {
	if k.err != nil {
		return k.err
	}
	if _, ok := k.items[account]; !ok {
		return gokey.ErrNotFound
	}
	delete(k.items, account)
	return nil
}

// newTestStore returns a KeyringStore backed by keychain, its plaintext
// fallback path, and the buffer its warnings go to.
func newTestStore(t *testing.T, keychain *fakeKeychain) (*KeyringStore, string, *bytes.Buffer) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.yml")
	streams, _, _, errOut := iostreams.Test()
	store := NewKeyringStore(streams, path)
	store.get, store.set, store.remove = keychain.get, keychain.set, keychain.remove
	return store, path, errOut
}

var testCreds = Credentials{Site: "acme.atlassian.net", Email: "alice@example.com", APIToken: "t0ken"}

func TestKeyringRoundTripsThroughTheKeychain(t *testing.T) {
	store, path, _ := newTestStore(t, newFakeKeychain())

	if err := store.Set("default", testCreds); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != testCreds {
		t.Errorf("got %+v, want %+v", got, testCreds)
	}
	// The keychain took the entry, so nothing may have been written to the
	// plaintext file.
	if _, err := NewPlaintextStore(path).Get("default"); !errors.Is(err, ErrNotFound) {
		t.Errorf("plaintext fallback error = %v, want ErrNotFound", err)
	}
	if err := store.Delete("default"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("default"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
}

// A profile absent from the keychain must still resolve via the plaintext
// fallback — its entry may have been written there while the keychain was
// unavailable.
func TestKeyringGetFallsBackToPlaintextWhenKeychainHasNoEntry(t *testing.T) {
	store, path, _ := newTestStore(t, newFakeKeychain())

	if err := NewPlaintextStore(path).Set("default", testCreds); err != nil {
		t.Fatalf("seed plaintext store: %v", err)
	}

	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != testCreds {
		t.Errorf("got %+v, want %+v", got, testCreds)
	}
}

func TestKeyringGetReportsNotFoundWhenNeitherStoreHasEntry(t *testing.T) {
	store, _, _ := newTestStore(t, newFakeKeychain())

	if _, err := store.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got error %v, want ErrNotFound", err)
	}
}

func TestKeyringSetFallsBackToPlaintextWhenKeychainIsUnavailable(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.err = fmt.Errorf("%w: no D-Bus session bus", gokey.ErrUnavailable)
	store, path, errOut := newTestStore(t, keychain)

	if err := store.Set("default", testCreds); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := NewPlaintextStore(path).Get("default")
	if err != nil {
		t.Fatalf("plaintext Get: %v", err)
	}
	if got != testCreds {
		t.Errorf("got %+v, want %+v", got, testCreds)
	}
	if !strings.Contains(errOut.String(), "falling back to plaintext") {
		t.Errorf("stderr = %q, want a fallback warning", errOut.String())
	}
}

// A keychain that is present but fails for an unexplained reason is not an
// absent one: writing the token to a plaintext file instead would quietly
// downgrade where it lives, so the failure surfaces.
func TestKeyringReportsUnexpectedKeychainFailures(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.err = errors.New("the keychain is locked")
	store, path, errOut := newTestStore(t, keychain)

	operations := map[string]func() error{
		"Get":    func() error { _, err := store.Get("default"); return err },
		"Set":    func() error { return store.Set("default", testCreds) },
		"Delete": func() error { return store.Delete("default") },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, keychain.err) {
				t.Fatalf("error = %v, want the keychain failure", err)
			}
		})
	}
	if _, err := NewPlaintextStore(path).Get("default"); !errors.Is(err, ErrNotFound) {
		t.Errorf("plaintext fallback error = %v, want ErrNotFound", err)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want no fallback warning", errOut.String())
	}
}

// An entry the keychain holds but will not release is neither a missing
// entry nor an unavailable keychain: reading or writing the plaintext file
// instead would hide it, so every operation reports it with its remedy.
func TestKeyringReportsBlockedEntryWithoutFallingBack(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.err = fmt.Errorf("%w (OSStatus -25308)", gokey.ErrBlocked)
	store, path, errOut := newTestStore(t, keychain)

	operations := map[string]func() error{
		"Get":    func() error { _, err := store.Get("default"); return err },
		"Set":    func() error { return store.Set("default", testCreds) },
		"Delete": func() error { return store.Delete("default") },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			err := operation()
			if !errors.Is(err, gokey.ErrBlocked) {
				t.Fatalf("error = %v, want ErrBlocked", err)
			}
			for _, want := range []string{"security delete-generic-password -s atlassian-cli -a default", "atl auth login --profile default"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
	if _, err := NewPlaintextStore(path).Get("default"); !errors.Is(err, ErrNotFound) {
		t.Errorf("plaintext fallback error = %v, want ErrNotFound", err)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want no fallback warning", errOut.String())
	}
}
