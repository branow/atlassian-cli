package credentials

// FakeStore is an in-memory Store implementation for tests, avoiding any
// disk or OS keychain access.
type FakeStore struct {
	data map[string]Credentials
}

// NewFakeStore returns an empty in-memory credentials store.
func NewFakeStore() *FakeStore {
	return &FakeStore{data: map[string]Credentials{}}
}

// Get returns the stored credentials for profile.
func (s *FakeStore) Get(profile string) (Credentials, error) {
	creds, ok := s.data[profile]
	if !ok {
		return Credentials{}, ErrNotFound
	}
	return creds, nil
}

// Set stores creds for profile, overwriting any existing entry.
func (s *FakeStore) Set(profile string, creds Credentials) error {
	s.data[profile] = creds
	return nil
}

// Delete removes the stored credentials for profile.
func (s *FakeStore) Delete(profile string) error {
	if _, ok := s.data[profile]; !ok {
		return ErrNotFound
	}
	delete(s.data, profile)
	return nil
}
