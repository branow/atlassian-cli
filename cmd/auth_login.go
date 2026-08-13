package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/config"
	"github.com/branow/atlassian-cli/internal/credentials"
)

// newAuthLoginCmd builds "atl auth login", supporting both interactive
// prompts and fully non-interactive, scriptable operation via flags so it
// is not a dead end for CI use. It stores the API token in the OS keychain
// and the (non-secret) site and email in the config profile.
func newAuthLoginCmd(f *cmdutil.Factory) *cobra.Command {
	var site, email string
	var tokenStdin, noVerify bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to an Atlassian site and store credentials",
		Long: `Log in to an Atlassian Cloud site. Authentication is HTTP Basic using
your account email and an API token (create one at
https://id.atlassian.com/manage-profile/security/api-tokens). One token
serves both Jira and Confluence on the site. The token is stored in the OS
keychain; the site and email are stored in the config profile.`,
		Example: `  # Interactive login, prompting for each value
  atl auth login

  # Non-interactive login for CI, reading the token from stdin
  echo "$ATL_API_TOKEN" | atl auth login --site your-org.atlassian.net --email you@example.com --token-stdin

  # Log in under a named profile
  atl auth login --profile sandbox --site sandbox.atlassian.net --email you@example.com --token-stdin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(cmd.Context(), f, site, email, tokenStdin, noVerify)
		},
	}

	cmd.Flags().StringVar(&site, "site", "", "Atlassian site host, e.g. your-org.atlassian.net")
	cmd.Flags().StringVar(&email, "email", "", "Atlassian account email")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read the API token from stdin")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "store the credentials without verifying them against the API")
	return cmd
}

func runAuthLogin(ctx context.Context, f *cmdutil.Factory, site, email string, tokenStdin, noVerify bool) error {
	profile := f.ActiveProfile()
	reader := bufio.NewReader(f.IOStreams.In)

	site, err := resolvePromptValue(f, reader, site, "Site")
	if err != nil {
		return err
	}
	site = normalizeSite(site)
	if err := validateSite(site); err != nil {
		return err
	}
	email, err = resolvePromptValue(f, reader, email, "Email")
	if err != nil {
		return err
	}
	token, err := resolveToken(f, reader, tokenStdin)
	if err != nil {
		return err
	}

	creds := credentials.Credentials{Site: site, Email: email, APIToken: token}
	if !noVerify {
		if err := verifyLogin(ctx, f, creds); err != nil {
			return err
		}
	}
	if err := f.CredentialsStore.Set(profile, creds); err != nil {
		return err
	}

	// Preserve any output preference already stored for this profile, but do
	// not bake a resolved default or a transient ATL_OUTPUT into it — an
	// unset Output stays unset so the env/default keep applying at read time.
	f.Config.SetProfile(profile, config.Profile{
		Site:   site,
		Email:  email,
		Output: f.Config.ProfileOutput(profile),
	})
	if err := f.Config.SwitchProfile(profile); err != nil {
		return err
	}
	if err := f.Config.Save(); err != nil {
		return err
	}

	if !f.Quiet {
		fmt.Fprintf(f.IOStreams.Out, "Logged in to %s as %s using profile %q\n", site, email, profile)
	}
	return nil
}

// resolvePromptValue returns flagValue if set, otherwise prompts for it
// interactively, otherwise fails with a validation error naming the flag
// to use non-interactively.
func resolvePromptValue(f *cmdutil.Factory, reader *bufio.Reader, flagValue, label string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if !f.IOStreams.CanPrompt() {
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("%s is required (pass the corresponding flag in non-interactive mode)", label)}
	}
	fmt.Fprintf(f.IOStreams.Out, "%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("%s is required", label)}
	}
	return value, nil
}

// resolveToken reads the API token from stdin when tokenStdin is set,
// otherwise prompts with hidden input on a real terminal, otherwise fails
// with a validation error. Only the trailing line ending is stripped.
func resolveToken(f *cmdutil.Factory, reader *bufio.Reader, tokenStdin bool) (string, error) {
	if tokenStdin {
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		return nonEmptyToken(string(data))
	}
	if stdin := f.IOStreams.StdinFd(); stdin != nil && f.IOStreams.CanPrompt() {
		fmt.Fprint(f.IOStreams.Out, "API token: ")
		byteToken, err := term.ReadPassword(int(stdin.Fd()))
		fmt.Fprintln(f.IOStreams.Out)
		if err != nil {
			return "", err
		}
		return nonEmptyToken(string(byteToken))
	}
	return "", &cmdutil.ValidationError{Message: "API token is required (use --token-stdin in non-interactive mode)"}
}

func nonEmptyToken(raw string) (string, error) {
	token := strings.TrimRight(raw, "\r\n")
	if token == "" {
		return "", &cmdutil.ValidationError{Message: "API token is required"}
	}
	return token, nil
}

// verifyPath is the read-only endpoint used to prove credentials work:
// Jira's "get current user" is parameterless and available to any
// authenticated account on a site.
const verifyPath = "/rest/api/3/myself"

// verifyLogin checks the credentials against the API before they are
// stored, so a wrong email/token fails at login rather than on the first
// real call. Only definitive rejections block the login — 401/403 (bad
// credentials). Anything else the probe cannot interpret (an unreachable
// host, a 5xx) is inconclusive: the credentials are stored with a warning,
// since blocking on it would lock out otherwise-valid logins.
func verifyLogin(ctx context.Context, f *cmdutil.Factory, creds credentials.Credentials) error {
	baseURL, err := atlapi.BaseURL(creds.Site)
	if err != nil {
		return err
	}
	client := atlapi.New(baseURL, creds)
	_, err = client.Do(ctx, http.MethodGet, verifyPath, nil, nil)
	if err == nil {
		return nil
	}
	var apiErr *atlapi.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("login verification failed (use --no-verify to store the credentials anyway): %w", err)
		}
	}
	fmt.Fprintf(f.IOStreams.ErrOut, "warning: could not verify the credentials (%v); storing them anyway\n", err)
	return nil
}
