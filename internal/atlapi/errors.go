package atlapi

import "fmt"

// APIError is a non-2xx REST response from Atlassian Cloud. Atlassian
// signals both transport/auth failures and business-rule rejections via the
// HTTP status plus an error body, so this one type covers them all. Message
// is the best-effort human text pulled from the body; Raw is the untouched
// body for callers that need the full error envelope.
type APIError struct {
	Status  int
	Message string
	Raw     []byte
	// RateLimited marks a response the caller should treat as throttling and
	// back off on: a 429, or a 503 carrying a Retry-After header. It is false
	// for other 5xx (e.g. a 503 for a permanently suspended site), so callers
	// can tell a retry-worthy condition from a hard failure by status alone.
	RateLimited bool
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Atlassian API returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("Atlassian API returned HTTP %d: %s", e.Status, e.Message)
}

// UnknownOperationError is a request for an operationId absent from the
// embedded catalog — a typo, or an ambiguous id that must be reached via a
// namespace. It is caught before any network call.
type UnknownOperationError struct {
	Operation string
}

func (e *UnknownOperationError) Error() string {
	return fmt.Sprintf("unknown operation %q (run atl api --list to see all operations)", e.Operation)
}

// UnsupportedOperationError is a catalogued operation with no usable REST
// binding (e.g. a missing HTTP method). It is caught before any network
// call.
type UnsupportedOperationError struct {
	Operation string
}

func (e *UnsupportedOperationError) Error() string {
	return fmt.Sprintf("operation %q has no usable REST binding", e.Operation)
}
