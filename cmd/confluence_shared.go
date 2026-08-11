package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/confluence"
	"github.com/branow/atlassian-cli/internal/credentials"
)

// uploadAttachment is the seam the attach command drives, defaulting to the
// real multipart uploader. It is a package var so tests can point it at an
// httptest server (attachments bypass the JSON client, whose base URL the
// test factory controls, so this indirection is how the command test reaches
// a stub server).
var uploadAttachment = confluence.UploadAttachment

// pageOutput is the reduced view of a page the page commands print in table
// mode; -o json prints the raw API response instead.
type pageOutput struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	SpaceID string `json:"spaceId"`
	Version int    `json:"version"`
}

// writePageResult renders a page as a table (id/title/spaceId/version) or,
// under -o json, as the untouched API response so no field is lost.
func writePageResult(f *cmdutil.Factory, page *confluence.Page, raw []byte) error {
	if f.OutputFormat() == "json" {
		return writeRawJSON(f, raw)
	}
	return writeKeyValues(f, [][2]string{
		{"id", page.ID.String()},
		{"title", page.Title},
		{"spaceId", page.SpaceID.String()},
		{"version", fmt.Sprint(page.Version.Number)},
	})
}

// writeRawJSON is the Confluence commands' -o json entry point; it delegates
// to the shared writeResultJSON so Confluence and Jira pretty-print server
// responses identically.
func writeRawJSON(f *cmdutil.Factory, raw []byte) error {
	return writeResultJSON(f, raw)
}

// pathWithQuery joins a post-host path with an encoded query string, so the
// pagination helper can drive a single client.Do("GET", path, nil, nil) for
// both the first request and each _links.next follow-up (whose relative URL
// already carries its own query). An empty query yields the bare path.
func pathWithQuery(path string, query url.Values) string {
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}

// fetchRaw adapts a client to the confluence.FetchPage the pagination helper
// follows: each call issues a GET at the given post-host path (query already
// embedded) and returns the untouched response body.
func fetchRaw(ctx context.Context, client atlapi.Client) confluence.FetchPage {
	return func(path string) ([]byte, error) {
		resp, err := client.Do(ctx, "GET", path, nil, nil)
		if err != nil {
			return nil, err
		}
		return resp.Raw, nil
	}
}

// clientCredentials returns the stored credentials for the active profile,
// mapping an absent entry to the shared not-logged-in error so the exit code
// matches every other authenticated command.
func clientCredentials(f *cmdutil.Factory) (credentials.Credentials, error) {
	creds, err := f.CredentialsStore.Get(f.ActiveProfile())
	if err != nil {
		return credentials.Credentials{}, cmdutil.ErrNotLoggedIn
	}
	return creds, nil
}

// siteBaseURL turns a stored site host into a scheme+host base URL, adding
// https:// unless the host already carries a scheme (matching atlapi.New, so
// a locally overridden http host still works).
func siteBaseURL(site string) string {
	if strings.HasPrefix(site, "http://") || strings.HasPrefix(site, "https://") {
		return strings.TrimRight(site, "/")
	}
	return "https://" + strings.TrimRight(site, "/")
}

// resolveContent interprets a --content/replacement-body value: "-" reads all
// of stdin, a value naming an existing file reads that file, and anything
// else is taken literally. This lets a caller pass small bodies inline while
// still piping or file-sourcing large ones.
func resolveContent(f *cmdutil.Factory, value string) (string, error) {
	if value == "-" {
		data, err := io.ReadAll(f.IOStreams.In)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if info, err := os.Stat(value); err == nil && !info.IsDir() {
		data, err := os.ReadFile(value)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return value, nil
}

// confirm asks the user to approve a destructive action. --force approves
// unconditionally; otherwise a prompt is shown on an interactive terminal,
// and a non-interactive session (piped or --no-input) is refused with
// guidance to pass --force, so a script never blocks on a hidden prompt.
func confirm(f *cmdutil.Factory, prompt string) error {
	if f.Force {
		return nil
	}
	if !f.IOStreams.CanPrompt() {
		return &cmdutil.ValidationError{Message: "refusing to proceed without confirmation in non-interactive mode; pass --force to override"}
	}
	fmt.Fprintf(f.IOStreams.Out, "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(f.IOStreams.In).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return cmdutil.ErrCancelled
	}
}

// resolveSpaceID turns a --space argument into a numeric space id. An
// all-digit value is already an id; otherwise it is treated as a space key
// and looked up via the v2 spaces endpoint.
func resolveSpaceID(ctx context.Context, client atlapi.Client, space string) (string, error) {
	if isAllDigits(space) {
		return space, nil
	}
	query := url.Values{}
	query.Set("keys", space)
	query.Set("limit", "1")
	resp, err := client.Do(ctx, "GET", "/wiki/api/v2/spaces", query, nil)
	if err != nil {
		return "", err
	}
	list, err := confluence.ParseSpaceList(resp.Raw)
	if err != nil {
		return "", err
	}
	if len(list.Results) == 0 {
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("no space found with key %q", space)}
	}
	return list.Results[0].ID.String(), nil
}

// isAllDigits reports whether s is a non-empty run of ASCII digits, the shape
// of a Confluence numeric id.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
