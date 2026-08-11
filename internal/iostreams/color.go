package iostreams

import "os"

// detectColorCapable applies the standard precedence for color output:
// NO_COLOR and TERM=dumb always disable it, and a non-TTY stdout can never
// support it.
func detectColorCapable(stdoutTTY bool) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return stdoutTTY
}
