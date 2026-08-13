package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// newConfluenceAttachCmd uploads a file as a page attachment. Attachments
// require a multipart/form-data request the JSON api client cannot express,
// so this drives the self-contained uploader in internal/confluence, reusing
// the profile's stored credentials for auth.
func newConfluenceAttachCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <pageId> <file>",
		Short: "Upload a file as a page attachment",
		Args:  cobra.ExactArgs(2),
		Example: `  atl confluence attach 12345 ./diagram.png
  atl confluence attach 12345 ./report.pdf -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluenceAttach(cmd.Context(), f, args[0], args[1])
		},
	}
	return cmd
}

func runConfluenceAttach(ctx context.Context, f *cmdutil.Factory, pageID, file string) error {
	creds, err := clientCredentials(f)
	if err != nil {
		return err
	}
	baseURL, err := siteBaseURL(creds.Site)
	if err != nil {
		return err
	}
	raw, err := uploadAttachment(ctx, baseURL, creds, pageID, file)
	if err != nil {
		return err
	}
	if f.OutputFormat() == "json" {
		return writeRawJSON(f, raw)
	}
	if !f.Quiet {
		return writeRawJSON(f, raw)
	}
	return nil
}
