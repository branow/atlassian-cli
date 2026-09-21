package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/confluence"
	"github.com/branow/atlassian-cli/internal/output"
)

// newConfluenceAttachmentCmd groups the read side of page attachments:
// listing what a page carries and pulling one down. `confluence attach` is
// the write side; the two stay separate commands because the upload takes a
// page id and a local file while a download takes an attachment id.
func newConfluenceAttachmentCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attachment",
		Short: "List and download page attachments",
	}
	cmd.AddCommand(
		newConfluenceAttachmentListCmd(f),
		newConfluenceAttachmentDownloadCmd(f),
	)
	return cmd
}

// --- list ---

func newConfluenceAttachmentListCmd(f *cmdutil.Factory) *cobra.Command {
	var limit int
	var paginate bool
	cmd := &cobra.Command{
		Use:     "list <pageId>",
		Aliases: []string{"ls"},
		Short:   "List a page's attachments",
		Long: `List the attachments on a page with the id that "attachment download"
takes. By default only the first page of results is returned (capped by
--limit if given); --paginate follows _links.next until results are
exhausted or --limit is reached.`,
		Args: cobra.ExactArgs(1),
		Example: `  atl confluence attachment list 12345
  atl confluence attachment ls 12345 --paginate -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluenceAttachmentList(cmd.Context(), f, args[0], limit, paginate)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of results to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow all pages until exhausted or --limit is reached")
	return cmd
}

func runConfluenceAttachmentList(ctx context.Context, f *cmdutil.Factory, pageID string, limit int, paginate bool) error {
	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	query := url.Values{}
	if limit > 0 && !paginate {
		query.Set("limit", strconv.Itoa(limit))
	}
	path := pathWithQuery(fmt.Sprintf(confluence.AttachmentsPath, url.PathEscape(pageID)), query)
	items, err := confluence.CollectPages(path, limit, paginate, fetchRaw(ctx, client))
	if err != nil {
		return err
	}
	return emitAttachmentList(f, items)
}

// emitAttachmentList renders collected raw attachment items as JSON or a
// table.
func emitAttachmentList(f *cmdutil.Factory, items []json.RawMessage) error {
	merged, err := confluence.MergeResults(items)
	if err != nil {
		return err
	}
	if f.OutputFormat() == "json" {
		return writeRawJSON(f, merged)
	}
	list, err := confluence.ParseAttachmentList(merged)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(list.Results))
	for _, a := range list.Results {
		rows = append(rows, []string{a.ID.String(), a.Title, strconv.FormatInt(a.FileSize, 10), a.MediaType})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"ID", "TITLE", "SIZE", "TYPE"}, rows)
}

// --- download ---

func newConfluenceAttachmentDownloadCmd(f *cmdutil.Factory) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "download <attachmentId>",
		Short: "Download an attachment's contents",
		Long: `Download a page attachment's bytes by attachment id (e.g. att3564044322,
the id column of "attachment list"). The file is written to the attachment's
own title in the working directory unless --out names a file or a directory;
--out - streams it to stdout for piping.

Note that -o/--output is the global output format, not the destination: the
destination flag is --out.`,
		Args: cobra.ExactArgs(1),
		Example: `  atl confluence attachment download att3564044322
  atl confluence attachment download att3564044322 --out ./data.xlsx
  atl confluence attachment download att3564044322 --out - | wc -c`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluenceAttachmentDownload(cmd.Context(), f, args[0], out)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", `destination file or directory ("-" for stdout; default: the attachment's title)`)
	return cmd
}

func runConfluenceAttachmentDownload(ctx context.Context, f *cmdutil.Factory, id, out string) error {
	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	// Confluence serves attachment bytes from a link the metadata carries
	// rather than from a predictable path, so the metadata read is not an
	// optimization here — it is how the download URL is discovered.
	resp, err := client.Do(ctx, "GET", fmt.Sprintf(confluence.AttachmentByIDPath, url.PathEscape(id)), nil, nil)
	if err != nil {
		return err
	}
	att, err := confluence.ParseAttachment(resp.Raw)
	if err != nil {
		return err
	}
	path := att.DownloadPath()
	if path == "" {
		return &cmdutil.ValidationError{Message: fmt.Sprintf("attachment %q carries no download link", id)}
	}
	return runAttachmentDownload(ctx, f, downloadSpec{path: path, id: id, name: att.Title}, out)
}
