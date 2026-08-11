package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/confluence"
)

// newConfluencePageCmd groups the curated page lifecycle commands. Reads and
// writes go through the v2 pages API; the version-aware `patch` is the reason
// this group is curated rather than left to the generic api command.
func newConfluencePageCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "page",
		Short: "Work with Confluence pages",
	}
	cmd.AddCommand(
		newConfluencePageGetCmd(f),
		newConfluencePageCreateCmd(f),
		newConfluencePageDeleteCmd(f),
		newConfluencePagePatchCmd(f),
	)
	return cmd
}

// --- get ---

func newConfluencePageGetCmd(f *cmdutil.Factory) *cobra.Command {
	var withBody bool
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Fetch a page",
		Args:  cobra.ExactArgs(1),
		Example: `  atl confluence page get 12345
  atl confluence page get 12345 --body
  atl confluence page get 12345 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluencePageGet(cmd.Context(), f, args[0], withBody)
		},
	}
	cmd.Flags().BoolVar(&withBody, "body", false, "print the page's storage-format body")
	return cmd
}

func runConfluencePageGet(ctx context.Context, f *cmdutil.Factory, id string, withBody bool) error {
	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	page, raw, err := fetchPage(ctx, client, id)
	if err != nil {
		return err
	}

	if f.OutputFormat() == "json" {
		return writeRawJSON(f, raw)
	}
	if withBody {
		_, err := fmt.Fprintln(f.IOStreams.Out, page.StorageValue())
		return err
	}
	return writePageResult(f, page, raw)
}

// fetchPage GETs a page with its storage body and version, the shape both
// `page get` and `page patch` need.
func fetchPage(ctx context.Context, client atlapi.Client, id string) (*confluence.Page, []byte, error) {
	query := url.Values{}
	query.Set("body-format", "storage")
	resp, err := client.Do(ctx, "GET", "/wiki/api/v2/pages/"+url.PathEscape(id), query, nil)
	if err != nil {
		return nil, nil, err
	}
	page, err := confluence.ParsePage(resp.Raw)
	if err != nil {
		return nil, nil, err
	}
	return page, resp.Raw, nil
}

// --- create ---

func newConfluencePageCreateCmd(f *cmdutil.Factory) *cobra.Command {
	var space, title, bodyFile, body string
	cmd := &cobra.Command{
		Use:   "create --space <key|id> --title <title>",
		Short: "Create a page",
		Args:  cobra.NoArgs,
		Long: `Create a Confluence page. The body is Confluence storage format (XHTML) and
is supplied either from a file (--body-file) or from stdin/a literal string
(--body, where "-" reads stdin). --space accepts a space key or numeric id.`,
		Example: `  atl confluence page create --space DS --title "Notes" --body-file notes.xml
  echo '<p>Hi</p>' | atl confluence page create --space 12345 --title Hi --body -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluencePageCreate(cmd.Context(), f, space, title, bodyFile, body)
		},
	}
	cmd.Flags().StringVar(&space, "space", "", "space key or id to create the page in (required)")
	cmd.Flags().StringVar(&title, "title", "", "page title (required)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read the storage-format body from a file")
	cmd.Flags().StringVar(&body, "body", "", `storage-format body as a string ("-" reads stdin)`)
	return cmd
}

func runConfluencePageCreate(ctx context.Context, f *cmdutil.Factory, space, title, bodyFile, body string) error {
	if space == "" || title == "" {
		return &cmdutil.ValidationError{Message: "--space and --title are required"}
	}
	value, err := resolveBody(f, bodyFile, body)
	if err != nil {
		return err
	}

	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	spaceID, err := resolveSpaceID(ctx, client, space)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"spaceId": spaceID,
		"status":  "current",
		"title":   title,
		"body": map[string]any{
			"representation": "storage",
			"value":          value,
		},
	}
	resp, err := client.Do(ctx, "POST", "/wiki/api/v2/pages", nil, payload)
	if err != nil {
		return err
	}
	page, err := confluence.ParsePage(resp.Raw)
	if err != nil {
		return err
	}
	return writePageResult(f, page, resp.Raw)
}

// resolveBody assembles a create body from the mutually exclusive --body-file
// and --body sources, requiring exactly one so a page is never created with
// an accidentally empty body.
func resolveBody(f *cmdutil.Factory, bodyFile, body string) (string, error) {
	switch {
	case bodyFile != "" && body != "":
		return "", &cmdutil.ValidationError{Message: "--body-file and --body cannot be combined"}
	case bodyFile != "":
		return resolveContent(f, bodyFile)
	case body != "":
		return resolveContent(f, body)
	default:
		return "", &cmdutil.ValidationError{Message: "a body is required (pass --body-file or --body)"}
	}
}

// --- delete ---

func newConfluencePageDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a page",
		Args:  cobra.ExactArgs(1),
		Example: `  atl confluence page delete 12345
  atl confluence page delete 12345 --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluencePageDelete(cmd.Context(), f, args[0])
		},
	}
	return cmd
}

func runConfluencePageDelete(ctx context.Context, f *cmdutil.Factory, id string) error {
	if err := confirm(f, fmt.Sprintf("Delete page %s?", id)); err != nil {
		return err
	}
	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	if _, err := client.Do(ctx, "DELETE", "/wiki/api/v2/pages/"+url.PathEscape(id), nil, nil); err != nil {
		return err
	}
	if !f.Quiet {
		fmt.Fprintf(f.IOStreams.Out, "Deleted page %s\n", id)
	}
	return nil
}

// --- patch ---

func newConfluencePagePatchCmd(f *cmdutil.Factory) *cobra.Command {
	var heading, anchor, regex, content, replacement string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "patch <id>",
		Short: "Partially update a page's body (version-aware)",
		Args:  cobra.ExactArgs(1),
		Long: `Partially update a page. Confluence only accepts a full-body PUT, so this
reads the current storage body and version, rewrites exactly one selected
region locally, and PUTs the result with the next version number.

Exactly one selector must be given:
  --heading <text> --content <file|-|string>   replace the section under the
      matching heading, up to the next same-or-higher-level heading
  --anchor <name>  --content <file|-|string>   replace the region between a
      pair of named anchor macros (or <!-- name --> ... <!-- /name --> markers)
  --regex <pattern> --replacement <string>     regexp substitution over the body

--dry-run prints the resulting body instead of saving it.`,
		Example: `  atl confluence page patch 12345 --heading "Status" --content status.xml
  atl confluence page patch 12345 --regex 'DRAFT' --replacement 'FINAL' --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluencePagePatch(cmd.Context(), f, args[0], patchFlags{
				heading: heading, anchor: anchor, regex: regex,
				content: content, replacement: replacement, dryRun: dryRun,
			})
		},
	}
	cmd.Flags().StringVar(&heading, "heading", "", "replace the section under this heading")
	cmd.Flags().StringVar(&anchor, "anchor", "", "replace the region named by this anchor/marker")
	cmd.Flags().StringVar(&regex, "regex", "", "regexp to substitute within the body")
	cmd.Flags().StringVar(&content, "content", "", `replacement markup for --heading/--anchor (file, "-" for stdin, or literal)`)
	cmd.Flags().StringVar(&replacement, "replacement", "", "replacement string for --regex (supports $1 group refs)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the resulting body without saving")
	return cmd
}

// patchFlags carries the patch command's flag values as one argument, keeping
// the run function signature small.
type patchFlags struct {
	heading, anchor, regex string
	content, replacement   string
	dryRun                 bool
}

func runConfluencePagePatch(ctx context.Context, f *cmdutil.Factory, id string, fl patchFlags) error {
	patch, err := buildPatch(f, fl)
	if err != nil {
		return err
	}

	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	page, _, err := fetchPage(ctx, client, id)
	if err != nil {
		return err
	}

	newBody, err := patch.Apply(page.StorageValue())
	if err != nil {
		// A no-match against real page content is user-facing input error.
		return &cmdutil.ValidationError{Message: err.Error()}
	}

	if fl.dryRun {
		_, err := fmt.Fprintln(f.IOStreams.Out, newBody)
		return err
	}

	payload := map[string]any{
		"id":     page.ID.String(),
		"status": pageStatus(page),
		"title":  page.Title,
		"body": map[string]any{
			"representation": "storage",
			"value":          newBody,
		},
		"version": map[string]any{"number": page.Version.Number + 1},
	}
	resp, err := client.Do(ctx, "PUT", "/wiki/api/v2/pages/"+url.PathEscape(id), nil, payload)
	if err != nil {
		return err
	}
	updated, err := confluence.ParsePage(resp.Raw)
	if err != nil {
		return err
	}
	return writePageResult(f, updated, resp.Raw)
}

// buildPatch validates the selector/companion flag combination and returns
// the patch engine's request. The engine also guards mutual exclusion, but
// catching it here yields a validation exit code and a message naming flags.
func buildPatch(f *cmdutil.Factory, fl patchFlags) (confluence.Patch, error) {
	selectors := 0
	for _, s := range []string{fl.heading, fl.anchor, fl.regex} {
		if s != "" {
			selectors++
		}
	}
	if selectors == 0 {
		return confluence.Patch{}, &cmdutil.ValidationError{Message: "exactly one of --heading, --anchor, or --regex is required"}
	}
	if selectors > 1 {
		return confluence.Patch{}, &cmdutil.ValidationError{Message: "--heading, --anchor, and --regex are mutually exclusive"}
	}

	patch := confluence.Patch{Heading: fl.heading, Anchor: fl.anchor, Regex: fl.regex}
	if fl.regex != "" {
		if fl.replacement == "" {
			return confluence.Patch{}, &cmdutil.ValidationError{Message: "--regex requires --replacement"}
		}
		patch.Replacement = fl.replacement
		return patch, nil
	}
	// --heading / --anchor consume --content.
	if fl.content == "" {
		return confluence.Patch{}, &cmdutil.ValidationError{Message: "--heading/--anchor require --content"}
	}
	content, err := resolveContent(f, fl.content)
	if err != nil {
		return confluence.Patch{}, err
	}
	patch.Content = content
	return patch, nil
}

// pageStatus preserves the page's current status on update, defaulting to
// "current" when the fetched page did not report one.
func pageStatus(page *confluence.Page) string {
	if page.Status != "" {
		return page.Status
	}
	return "current"
}
