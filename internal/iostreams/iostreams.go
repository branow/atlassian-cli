// Package iostreams provides the CLI's input/output streams with TTY and
// color-capability detection, so commands never touch os.Stdin/Stdout
// directly and can be tested against in-memory buffers instead.
package iostreams

import (
	"bytes"
	"io"
	"os"
)

// IOStreams bundles the CLI's input and output streams along with the
// terminal and color capabilities detected for them.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	stdin     *os.File
	stdinTTY  bool
	stdoutTTY bool
	stderrTTY bool

	colorCapable bool
	noColor      bool
	noInput      bool
}

// System returns IOStreams wired to the process's real stdin/stdout/stderr,
// with TTY and color detection based on the actual file descriptors.
func System() *IOStreams {
	stdoutTTY := isTerminal(os.Stdout)
	return &IOStreams{
		In:           os.Stdin,
		Out:          os.Stdout,
		ErrOut:       os.Stderr,
		stdin:        os.Stdin,
		stdinTTY:     isTerminal(os.Stdin),
		stdoutTTY:    stdoutTTY,
		stderrTTY:    isTerminal(os.Stderr),
		colorCapable: detectColorCapable(stdoutTTY),
	}
}

// Test returns IOStreams backed by in-memory buffers, for use in unit and
// command tests where no real terminal is available.
func Test() (streams *IOStreams, in, out, errOut *bytes.Buffer) {
	in = &bytes.Buffer{}
	out = &bytes.Buffer{}
	errOut = &bytes.Buffer{}
	streams = &IOStreams{In: in, Out: out, ErrOut: errOut}
	return streams, in, out, errOut
}

// SetNoColor forces color output off regardless of TTY detection, mirroring
// the --no-color flag.
func (s *IOStreams) SetNoColor(v bool) {
	s.noColor = v
}

// SetNoInput marks the streams as non-interactive, mirroring the --no-input
// flag so commands know prompting is disallowed.
func (s *IOStreams) SetNoInput(v bool) {
	s.noInput = v
}

// CanPrompt reports whether commands may interactively prompt the user:
// both stdin and stdout must be terminals and --no-input must not be set.
func (s *IOStreams) CanPrompt() bool {
	return !s.noInput && s.stdinTTY && s.stdoutTTY
}

// IsStdoutTTY reports whether standard output is attached to a terminal.
func (s *IOStreams) IsStdoutTTY() bool {
	return s.stdoutTTY
}

// IsStderrTTY reports whether standard error is attached to a terminal.
func (s *IOStreams) IsStderrTTY() bool {
	return s.stderrTTY
}

// ColorEnabled reports whether output should be colorized, honoring
// NO_COLOR, --no-color, TERM=dumb, and non-TTY stdout.
func (s *IOStreams) ColorEnabled() bool {
	return s.colorCapable && !s.noColor
}

// StdinFd returns the process's real stdin file when it is a terminal, for
// callers that need raw fd access such as hidden password input. It
// returns nil when stdin is not the process's own terminal (e.g. piped
// input or test buffers).
func (s *IOStreams) StdinFd() *os.File {
	if s.stdinTTY {
		return s.stdin
	}
	return nil
}
