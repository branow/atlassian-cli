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

// newConfluenceSearchCmd runs a CQL search. Confluence v2 has no CQL endpoint,
// so this uses the v1 search API, which is the canonical CQL surface.
func newConfluenceSearchCmd(f *cmdutil.Factory) *cobra.Command {
	var limit int
	var paginate bool
	cmd := &cobra.Command{
		Use:   "search <cql>",
		Short: "Search content with CQL",
		Args:  cobra.ExactArgs(1),
		Long: `Search Confluence content with a CQL query (Confluence Query Language).
CQL is the v1 search surface; v2 does not expose it. By default only the first
page is returned (capped by --limit if given); --paginate follows the
response's _links.next until results are exhausted or --limit is reached.`,
		Example: `  atl confluence search 'space = DS and type = page'
  atl confluence search 'title ~ "release notes"' --paginate --limit 100 -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfluenceSearch(cmd.Context(), f, args[0], limit, paginate)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of results to return")
	cmd.Flags().BoolVar(&paginate, "paginate", false, "follow all pages until exhausted or --limit is reached")
	return cmd
}

func runConfluenceSearch(ctx context.Context, f *cmdutil.Factory, cql string, limit int, paginate bool) error {
	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	// Cap the first request's page size to --limit only when not paginating;
	// while paginating we take the server's default page size and stop
	// client-side once --limit items are collected. The _links.next follow-up
	// carries the cql forward, so it is only set on the first request here.
	query := url.Values{}
	query.Set("cql", cql)
	if limit > 0 && !paginate {
		query.Set("limit", strconv.Itoa(limit))
	}
	items, collectErr := confluence.CollectPages(pathWithQuery("/wiki/rest/api/search", query), limit, paginate, fetchRaw(ctx, client))
	// Emit collected pages first — always on success, and on a mid-pagination
	// failure whenever some pages were fetched — so a partial failure still
	// yields the results already in hand rather than an empty stdout.
	if collectErr == nil || len(items) > 0 {
		if err := emitSearchResults(f, items); err != nil {
			return err
		}
	}
	if collectErr != nil {
		if len(items) > 0 {
			fmt.Fprintf(f.IOStreams.ErrOut, "warning: emitted %d partial result(s) before pagination failed: %v\n", len(items), collectErr)
		}
		return collectErr
	}
	return nil
}

// emitSearchResults renders collected raw search items as JSON or a table.
func emitSearchResults(f *cmdutil.Factory, items []json.RawMessage) error {
	merged, err := confluence.MergeResults(items)
	if err != nil {
		return err
	}
	if f.OutputFormat() == "json" {
		return writeRawJSON(f, merged)
	}
	results, err := confluence.ParseSearch(merged)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(results.Results))
	for _, r := range results.Results {
		id, typ, title, space := r.Row()
		rows = append(rows, []string{id, typ, title, space})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"ID", "TYPE", "TITLE", "SPACE"}, rows)
}
