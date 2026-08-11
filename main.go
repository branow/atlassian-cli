// Command atl is the entry point for atlassian-cli: one CLI for Atlassian
// Cloud (Jira + Confluence today, Bitbucket later).
package main

import (
	"fmt"
	"os"

	"github.com/branow/atlassian-cli/cmd"
	"github.com/branow/atlassian-cli/internal/cmdutil"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(cmdutil.ExitCode(err))
}
