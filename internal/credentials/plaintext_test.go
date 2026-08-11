package credentials_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/branow/atlassian-cli/internal/credentials"
)

func statMode(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Mode().Perm(), nil
}

func TestPlaintextStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.yml")
	store := credentials.NewPlaintextStore(path)

	if _, err := store.Get("default"); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("Get before Set: got %v, want ErrNotFound", err)
	}

	want := credentials.Credentials{Site: "acme.atlassian.net", Email: "alice@example.com", APIToken: "t0ken"}
	if err := store.Set("default", want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPlaintextStoreFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions not enforced on windows")
	}
	path := filepath.Join(t.TempDir(), "credentials.yml")
	store := credentials.NewPlaintextStore(path)
	if err := store.Set("default", credentials.Credentials{Email: "alice@example.com"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	info, err := statMode(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info != 0o600 {
		t.Errorf("got mode %o, want %o", info, 0o600)
	}
}

func TestPlaintextStoreDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.yml")
	store := credentials.NewPlaintextStore(path)

	if err := store.Delete("default"); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("Delete on empty store: got %v, want ErrNotFound", err)
	}

	if err := store.Set("default", credentials.Credentials{Email: "alice@example.com"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete("default"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("default"); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

func TestFakeStoreRoundTrip(t *testing.T) {
	store := credentials.NewFakeStore()
	if _, err := store.Get("default"); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("Get before Set: got %v, want ErrNotFound", err)
	}

	want := credentials.Credentials{Site: "acme.atlassian.net", Email: "bob@example.com", APIToken: "pw"}
	if err := store.Set("default", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	if err := store.Delete("default"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("default"); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
}
