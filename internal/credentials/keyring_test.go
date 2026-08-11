package credentials

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/branow/atlassian-cli/internal/iostreams"
)

// A profile absent from the keychain must still resolve via the plaintext
// fallback — its entry may have been written there while the keychain was
// unavailable.
func TestKeyringGetFallsBackToPlaintextWhenKeychainHasNoEntry(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "credentials.yml")
	streams, _, _, _ := iostreams.Test()
	store := NewKeyringStore(streams, path)

	want := Credentials{Site: "acme.atlassian.net", Email: "alice@example.com", APIToken: "t0ken"}
	if err := NewPlaintextStore(path).Set("default", want); err != nil {
		t.Fatalf("seed plaintext store: %v", err)
	}

	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestKeyringGetReportsNotFoundWhenNeitherStoreHasEntry(t *testing.T) {
	keyring.MockInit()
	streams, _, _, _ := iostreams.Test()
	store := NewKeyringStore(streams, filepath.Join(t.TempDir(), "credentials.yml"))

	if _, err := store.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got error %v, want ErrNotFound", err)
	}
}
