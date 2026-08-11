package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// newConfluenceCommentCmd groups comment subcommands. Both footer and inline
// comments use public v2 endpoints (createFooterComment / createInlineComment),
// so no internal or experimental endpoint is ever needed.
func newConfluenceCommentCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Work with Confluence comments",
	}
	cmd.AddCommand(newConfluenceCommentAddCmd(f))
	return cmd
}

func newConfluenceCommentAddCmd(f *cmdutil.Factory) *cobra.Command {
	var body, selectText string
	var inline bool
	var matchIndex int
	cmd := &cobra.Command{
		Use:   "add <pageId>",
		Short: "Add a footer or inline comment to a page",
		Args:  cobra.ExactArgs(1),
		Long: `Add a comment to a page. Without --inline a footer comment is created; with
--inline the comment is anchored to the first (or --match-index'th) occurrence
of the --select text. The comment body accepts a literal string, a file path,
or "-" for stdin.`,
		Example: `  atl confluence comment add 12345 --body "Looks good"
  atl confluence comment add 12345 --inline --select "the API" --body "which one?"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluenceCommentAdd(cmd.Context(), f, args[0], commentFlags{
				body: body, selectText: selectText, inline: inline,
				matchIndex: matchIndex,
			})
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `comment body (literal, file path, or "-" for stdin)`)
	cmd.Flags().BoolVar(&inline, "inline", false, "create an inline comment anchored to --select text")
	cmd.Flags().StringVar(&selectText, "select", "", "text to anchor an inline comment to (with --inline)")
	cmd.Flags().IntVar(&matchIndex, "match-index", 0, "which occurrence of --select to anchor to (0-based)")
	return cmd
}

// commentFlags carries the comment command's flags as one argument.
type commentFlags struct {
	body       string
	selectText string
	inline     bool
	matchIndex int
}

func runConfluenceCommentAdd(ctx context.Context, f *cmdutil.Factory, pageID string, fl commentFlags) error {
	if fl.body == "" {
		return &cmdutil.ValidationError{Message: "--body is required"}
	}
	if fl.selectText != "" && !fl.inline {
		return &cmdutil.ValidationError{Message: "--select requires --inline"}
	}
	if fl.inline && fl.selectText == "" {
		return &cmdutil.ValidationError{Message: "--inline requires --select"}
	}

	value, err := resolveContent(f, fl.body)
	if err != nil {
		return err
	}

	client, err := f.ClientFn()
	if err != nil {
		return err
	}

	path, payload := commentRequest(pageID, value, fl)
	resp, err := client.Do(ctx, "POST", path, nil, payload)
	if err != nil {
		return err
	}

	if f.OutputFormat() == "json" {
		return writeRawJSON(f, resp.Raw)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Raw, &created); err != nil {
		return fmt.Errorf("decoding comment response: %w", err)
	}
	if !f.Quiet {
		fmt.Fprintf(f.IOStreams.Out, "Added comment %s\n", created.ID)
	}
	return nil
}

// commentRequest builds the endpoint and JSON body for a footer or inline
// comment. An inline comment carries the text-selection anchor Confluence uses
// to place it against the rendered page: textSelectionMatchIndex is the
// zero-based occurrence to highlight, and textSelectionMatchCount must be
// strictly greater than that index (per the v2 createInlineComment spec), so a
// selection that appears more than once anchors to the intended occurrence
// rather than always the first. We do not know the page's true occurrence
// count client-side, so we send the minimum count that keeps the index valid
// (index+1); Confluence resolves the highlight from index within that bound.
func commentRequest(pageID, value string, fl commentFlags) (string, map[string]any) {
	payload := map[string]any{
		"pageId": pageID,
		"body": map[string]any{
			"representation": "storage",
			"value":          value,
		},
	}
	if !fl.inline {
		return "/wiki/api/v2/footer-comments", payload
	}
	payload["inlineCommentProperties"] = map[string]any{
		"textSelection":           fl.selectText,
		"textSelectionMatchIndex": fl.matchIndex,
		"textSelectionMatchCount": fl.matchIndex + 1,
	}
	return "/wiki/api/v2/inline-comments", payload
}
