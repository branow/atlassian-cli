package cmd_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/branow/atlassian-cli/cmd"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/credentials"
)

func TestAuthLoginNonInteractiveThenStatusThenLogout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"accountId": "123", "emailAddress": "alice@example.com"}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--site", server.URL, // scheme-qualified site doubles as the base URL for the probe
		"--email", "alice@example.com",
		"--token-stdin",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out.String(), "alice@example.com") {
		t.Errorf("expected login confirmation to mention the email, got %q", out.String())
	}
	out.Reset()

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "alice@example.com") {
		t.Errorf("expected status to show the email, got %q", out.String())
	}
	out.Reset()

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "logout"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logout: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "status"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error after logout")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAuthLoginVerificationRejectsBadCredentialsWithoutStoringThem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages": ["Client must be authenticated"]}`))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--site", server.URL,
		"--email", "alice@example.com",
		"--token-stdin",
	})
	f.IOStreams.In = strings.NewReader("wrong\n")
	err := root.Execute()
	if err == nil {
		t.Fatal("expected login to fail against a 401 server")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
	if _, err := f.CredentialsStore.Get("default"); err != credentials.ErrNotFound {
		t.Errorf("rejected credentials must not be stored, got %v", err)
	}
}

func TestAuthLoginInconclusiveVerificationStoresCredentialsWithWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message": "An unexpected error"}`))
	}))
	defer server.Close()

	f, _, errOut := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--site", server.URL,
		"--email", "alice@example.com",
		"--token-stdin",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login must succeed on an inconclusive probe: %v", err)
	}
	if !strings.Contains(errOut.String(), "could not verify") {
		t.Errorf("expected a could-not-verify warning, got %q", errOut.String())
	}
	if _, err := f.CredentialsStore.Get("default"); err != nil {
		t.Errorf("credentials must be stored on an inconclusive probe, got %v", err)
	}
}

func TestAuthLoginNoVerifySkipsTheAPICall(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--site", "unreachable.atlassian.net", // must not matter with --no-verify
		"--email", "alice@example.com",
		"--token-stdin",
		"--no-verify",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login --no-verify: %v", err)
	}
	if !strings.Contains(out.String(), "unreachable.atlassian.net") {
		t.Errorf("expected login confirmation to mention the site, got %q", out.String())
	}
}

func TestAuthLoginRequiresEmailNonInteractively(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--site", "acme.atlassian.net",
		"--token-stdin",
		"--no-verify",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error when --email is missing non-interactively")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAuthSwitchRequiresExistingProfile(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "switch", "--profile", "sandbox"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error switching to an unconfigured profile")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}
