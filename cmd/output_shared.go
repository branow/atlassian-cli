package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// writeResultJSON is the canonical -o json path for every curated command
// (Jira and Confluence alike): it pretty-prints the untouched server
// response, preserving the API's field order and losing nothing, so the two
// curated packages render JSON identically and the same way "atl api" does.
// A body that is not JSON (an empty 204, say) is emitted verbatim.
func writeResultJSON(f *cmdutil.Factory, raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, trimmed, "", "  "); err != nil {
		_, err := fmt.Fprintln(f.IOStreams.Out, string(trimmed))
		return err
	}
	buf.WriteByte('\n')
	_, err := f.IOStreams.Out.Write(buf.Bytes())
	return err
}

// writeKeyValues prints an aligned key/value block, the canonical table-mode
// rendering for a single-object result (a page, an issue, a sprint) where a
// columnar table reads oddly. The key column is padded to the widest key so
// both curated packages align single-object output the same way.
func writeKeyValues(f *cmdutil.Factory, pairs [][2]string) error {
	width := 0
	for _, p := range pairs {
		if len(p[0]) > width {
			width = len(p[0])
		}
	}
	for _, p := range pairs {
		if _, err := fmt.Fprintf(f.IOStreams.Out, "%-*s  %s\n", width, p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}
