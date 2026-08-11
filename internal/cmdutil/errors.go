package cmdutil

import "errors"

// ErrCancelled indicates the user declined a confirmation prompt.
var ErrCancelled = errors.New("cancelled")

// ErrNotLoggedIn indicates no credentials are stored for the target
// profile.
var ErrNotLoggedIn = errors.New("not logged in")

// ValidationError indicates bad user input caught before any API call —
// a missing required value, an invalid flag combination, and the like.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
