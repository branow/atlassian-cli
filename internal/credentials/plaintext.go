package credentials

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/branow/atlassian-cli/internal/config"
)

// PlaintextStore persists credentials in a 0600 YAML file, used as a
// fallback when no OS keychain backend is available.
type PlaintextStore struct {
	path string
}

// NewPlaintextStore returns a PlaintextStore backed by the file at path.
func NewPlaintextStore(path string) *PlaintextStore {
	return &PlaintextStore{path: path}
}

// DefaultPlaintextPath returns the standard fallback credentials file
// location, honoring XDG_CONFIG_HOME.
func DefaultPlaintextPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.yml"), nil
}

// Get returns the stored credentials for profile.
func (s *PlaintextStore) Get(profile string) (Credentials, error) {
	all, err := s.readAll()
	if err != nil {
		return Credentials{}, err
	}
	creds, ok := all[profile]
	if !ok {
		return Credentials{}, ErrNotFound
	}
	return creds, nil
}

// Set stores creds for profile, overwriting any existing entry.
func (s *PlaintextStore) Set(profile string, creds Credentials) error {
	all, err := s.readAll()
	if err != nil {
		return err
	}
	all[profile] = creds
	return s.writeAll(all)
}

// Delete removes the stored credentials for profile.
func (s *PlaintextStore) Delete(profile string) error {
	all, err := s.readAll()
	if err != nil {
		return err
	}
	if _, ok := all[profile]; !ok {
		return ErrNotFound
	}
	delete(all, profile)
	return s.writeAll(all)
}

func (s *PlaintextStore) readAll() (map[string]Credentials, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string]Credentials{}, nil
	}
	if err != nil {
		return nil, err
	}
	all := map[string]Credentials{}
	if err := yaml.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	return all, nil
}

func (s *PlaintextStore) writeAll(all map[string]Credentials) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(all)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
