package iostreams

import (
	"os"

	"golang.org/x/term"
)

// isTerminal reports whether f is attached to a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
