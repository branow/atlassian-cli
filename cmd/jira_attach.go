package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/jira"
)

// uploadJiraAttachment is the seam the attach command drives, defaulting to
// the real multipart uploader. It is a package var so tests can point it at an
// httptest server (attachments bypass the JSON client, whose base URL the test
// factory controls, so this indirection is how the command test reaches a stub
// server).
var uploadJiraAttachment = jira.UploadAttachment

// newJiraIssueAttachCmd uploads a file as an issue attachment. Attachments
// require a multipart/form-data request the JSON api client cannot express, so
// this drives the self-contained uploader in internal/jira, reusing the
// profile's stored credentials for auth.
func newJiraIssueAttachCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <KEY> <file>",
		Short: "Upload a file as an issue attachment",
		Long: `Upload a file as an attachment on an issue (e.g. PROJ-123). Jira never
overwrites an existing attachment: uploading the same filename adds another
alongside it. -o json emits the raw API response, whose attachment id you can
reference from a media node when embedding the file inline in a comment.`,
		Args: cobra.ExactArgs(2),
		Example: `  atl jira issue attach PROJ-123 ./diagram.png
  atl jira issue attach PROJ-123 ./report.pdf -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJiraIssueAttach(cmd.Context(), f, args[0], args[1])
		},
	}
	return cmd
}

func runJiraIssueAttach(ctx context.Context, f *cmdutil.Factory, key, file string) error {
	creds, err := clientCredentials(f)
	if err != nil {
		return err
	}
	baseURL, err := siteBaseURL(creds.Site)
	if err != nil {
		return err
	}
	raw, err := uploadJiraAttachment(ctx, baseURL, creds, key, file)
	if err != nil {
		return err
	}
	if wantJSON(f) {
		return writeResultJSON(f, raw)
	}
	if f.Quiet {
		return nil
	}
	return printJiraAttachResult(f, key, raw)
}

// printJiraAttachResult prints a one-line confirmation per uploaded
// attachment (the endpoint returns an array), falling back to the raw
// response when the shape is unexpected so no information is silently lost.
func printJiraAttachResult(f *cmdutil.Factory, key string, raw []byte) error {
	var atts []struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(raw, &atts); err != nil || len(atts) == 0 {
		return writeResultJSON(f, raw)
	}
	for _, a := range atts {
		if _, err := fmt.Fprintf(f.IOStreams.Out, "Attached %s to %s (id %s)\n", a.Filename, key, a.ID); err != nil {
			return err
		}
	}
	return nil
}
