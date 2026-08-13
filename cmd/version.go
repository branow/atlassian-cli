package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// version is the atl release version. It is a plain variable rather than
// build-time ldflags injection for this iteration.
var version = "0.1.1"

// newVersionCmd builds "atl version".
func newVersionCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the atl version",
		Example: `  atl version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(f.IOStreams.Out, "atl version %s\n", version)
			return nil
		},
	}
}
