package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/jira"
)

// newJiraIssueAttachmentCmd groups the read side of issue attachments:
// listing what an issue carries and pulling one down. `issue attach` is the
// write side; the two stay separate commands because the upload takes an
// issue key and a local file while a download takes an attachment id.
func newJiraIssueAttachmentCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attachment",
		Short: "List and download issue attachments",
	}
	cmd.AddCommand(
		newJiraIssueAttachmentListCmd(f),
		newJiraIssueAttachmentDownloadCmd(f),
	)
	return cmd
}

// --- list ---

func newJiraIssueAttachmentListCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list <KEY>",
		Aliases: []string{"ls"},
		Short:   "List an issue's attachments",
		Long: `List the attachments on an issue (e.g. PROJ-123) with the id that
"issue attachment download" takes. -o json emits the server's attachment
objects untouched.`,
		Args: cobra.ExactArgs(1),
		Example: `  atl jira issue attachment list PROJ-123
  atl jira issue attachment ls PROJ-123 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJiraIssueAttachmentList(cmd.Context(), f, args[0])
		},
	}
	return cmd
}

func runJiraIssueAttachmentList(ctx context.Context, f *cmdutil.Factory, key string) error {
	atts, raw, err := fetchIssueAttachments(ctx, f, key)
	if err != nil {
		return err
	}
	if wantJSON(f) {
		return writeResultJSON(f, raw)
	}
	return jiraTable(f, jira.AttachmentColumns, atts)
}

// fetchIssueAttachments GETs only the attachment field of an issue, the
// smallest payload that answers "what is attached here".
func fetchIssueAttachments(ctx context.Context, f *cmdutil.Factory, key string) ([]jira.Attachment, []byte, error) {
	resp, err := doJira(ctx, f, http.MethodGet, "/rest/api/3/issue/"+url.PathEscape(key), url.Values{"fields": {"attachment"}}, nil)
	if err != nil {
		return nil, nil, err
	}
	return jira.ParseIssueAttachments(resp.Raw)
}

// --- download ---

func newJiraIssueAttachmentDownloadCmd(f *cmdutil.Factory) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "download <attachmentId>",
		Short: "Download an attachment's contents",
		Long: `Download an issue attachment's bytes by attachment id (the id column of
"issue attachment list", not the issue key). The file is written to the
attachment's own filename in the working directory unless --out names a file
or a directory; --out - streams it to stdout for piping.

Note that -o/--output is the global output format, not the destination: the
destination flag is --out.`,
		Args: cobra.ExactArgs(1),
		Example: `  atl jira issue attachment download 12345
  atl jira issue attachment download 12345 --out ./report.xlsx
  atl jira issue attachment download 12345 --out - | head -c 100`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJiraIssueAttachmentDownload(cmd.Context(), f, args[0], out)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", `destination file or directory ("-" for stdout; default: the attachment's filename)`)
	return cmd
}

func runJiraIssueAttachmentDownload(ctx context.Context, f *cmdutil.Factory, id, out string) error {
	spec := downloadSpec{
		path: fmt.Sprintf(jira.AttachmentContentPath, url.PathEscape(id)),
		id:   id,
	}
	// The content endpoint names the file in Content-Disposition, but only
	// the metadata endpoint is guaranteed to, so look the name up when the
	// destination depends on it — and skip the extra call when --out already
	// names the file.
	if destinationNeedsName(out) {
		name, err := jiraAttachmentFilename(ctx, f, id)
		if err != nil {
			return err
		}
		spec.name = name
	}
	return runAttachmentDownload(ctx, f, spec, out)
}

// jiraAttachmentFilename reads an attachment's own filename from its
// metadata.
func jiraAttachmentFilename(ctx context.Context, f *cmdutil.Factory, id string) (string, error) {
	resp, err := doJira(ctx, f, http.MethodGet, fmt.Sprintf(jira.AttachmentMetadataPath, url.PathEscape(id)), nil, nil)
	if err != nil {
		return "", err
	}
	att, err := jira.ParseAttachment(resp.Raw)
	if err != nil {
		return "", err
	}
	return att.Filename, nil
}
