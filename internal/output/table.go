// Package output renders command results as either an aligned table or
// pretty-printed JSON, so commands can share one -o/--output switch.
package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// WriteTable renders rows under headers as an aligned, whitespace-padded
// table.
func WriteTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}
