package cmd_test

import (
	"bytes"
	"testing"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/config"
	"github.com/branow/atlassian-cli/internal/credentials"
	"github.com/branow/atlassian-cli/internal/iostreams"
)

// newTestFactory returns a Factory wired to in-memory IO, a fake
// credentials store, and (if serverURL is non-empty) a "default" profile
// already logged in against serverURL, for command tests that need no real
// network, filesystem, or OS keychain access. The client's base URL is
// serverURL directly, so requests hit the test's httptest server.
func newTestFactory(t *testing.T, serverURL string) (f *cmdutil.Factory, out, errOut *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	streams, _, out, errOut := iostreams.Test()
	cfg := config.New()
	credStore := credentials.NewFakeStore()

	if serverURL != "" {
		cfg.SetProfile("default", config.Profile{Site: "acme.atlassian.net", Email: "alice@example.com"})
		credStore.Set("default", credentials.Credentials{Site: "acme.atlassian.net", Email: "alice@example.com", APIToken: "t0ken"})
	}

	f = &cmdutil.Factory{
		IOStreams:        streams,
		Config:           cfg,
		CredentialsStore: credStore,
	}
	f.ClientFn = func() (atlapi.Client, error) {
		profile := f.ActiveProfile()
		creds, err := f.CredentialsStore.Get(profile)
		if err != nil {
			return nil, cmdutil.ErrNotLoggedIn
		}
		return atlapi.New(serverURL, creds), nil
	}
	return f, out, errOut
}
