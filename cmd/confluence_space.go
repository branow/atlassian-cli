package cmd

import (
	"context"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/confluence"
	"github.com/branow/atlassian-cli/internal/output"
)

// newConfluenceSpaceCmd groups the space subcommands. Only listing is
// curated; deeper space administration stays in the generic `confluence api`.
func newConfluenceSpaceCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "space",
		Short: "Work with Confluence spaces",
	}
	cmd.AddCommand(newConfluenceSpaceListCmd(f))
	return cmd
}

func newConfluenceSpaceListCmd(f *cmdutil.Factory) *cobra.Command {
	var limit int
	var paginate bool
	cmd := &cobra.Command{
		Use:     "ls",
		Short:   "List spaces",
		Aliases: []string{"list"},
		Args:    cobra.NoArgs,
		Long: `List Confluence spaces. By default only the first page is returned (capped by
--limit if given); --paginate follows the response's _links.next cursor until
the spaces are exhausted or --limit results have been collected.`,
		Example: `  atl confluence space ls
  atl confluence space ls --paginate --limit 500 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluenceSpaceList(cmd.Context(), f, limit, paginate)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of spaces to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow all pages until exhausted or --limit is reached")
	return cmd
}

func runConfluenceSpaceList(ctx context.Context, f *cmdutil.Factory, limit int, paginate bool) error {
	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	// Cap the first request's page size to --limit only when not paginating;
	// while paginating we take the server's default page size and stop
	// client-side once --limit items are collected.
	query := url.Values{}
	if limit > 0 && !paginate {
		query.Set("limit", strconv.Itoa(limit))
	}
	items, err := confluence.CollectPages(pathWithQuery("/wiki/api/v2/spaces", query), limit, paginate, fetchRaw(ctx, client))
	if err != nil {
		return err
	}
	merged, err := confluence.MergeResults(items)
	if err != nil {
		return err
	}

	if f.OutputFormat() == "json" {
		return writeRawJSON(f, merged)
	}
	list, err := confluence.ParseSpaceList(merged)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(list.Results))
	for _, s := range list.Results {
		rows = append(rows, []string{s.ID.String(), s.Key, s.Name, s.Type})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"ID", "KEY", "NAME", "TYPE"}, rows)
}
