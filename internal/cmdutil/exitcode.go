package cmdutil

import (
	"errors"

	"github.com/branow/atlassian-cli/internal/atlapi"
)

// Exit codes, per DESIGN.md's exit-code table.
const (
	ExitSuccess     = 0
	ExitError       = 1
	ExitCancelled   = 2
	ExitValidation  = 3
	ExitAuth        = 4
	ExitNotFound    = 5
	ExitRateLimited = 6
)

// ExitCode maps a command error to the documented process exit code,
// giving main.go one place to translate errors into process behavior.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, ErrCancelled) {
		return ExitCancelled
	}
	if errors.Is(err, ErrNotLoggedIn) {
		return ExitAuth
	}
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return ExitValidation
	}
	var apiErr *atlapi.APIError
	if errors.As(err, &apiErr) {
		return exitCodeForAPI(apiErr)
	}
	var unknownOp *atlapi.UnknownOperationError
	var unsupportedOp *atlapi.UnsupportedOperationError
	if errors.As(err, &unknownOp) || errors.As(err, &unsupportedOp) {
		return ExitValidation
	}
	return ExitError
}

// exitCodeForAPI classifies a non-2xx REST response by its status code.
// Atlassian conveys auth, not-found, and rate-limit conditions purely
// through the HTTP status; a 5xx that survived the retry loop is treated
// as a transient unavailability like a 429.
func exitCodeForAPI(e *atlapi.APIError) int {
	switch {
	case e.Status == 401 || e.Status == 403:
		return ExitAuth
	case e.Status == 404:
		return ExitNotFound
	case e.Status == 429 || e.Status >= 500:
		return ExitRateLimited
	}
	return ExitError
}
