package atlapi

import (
	"context"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

const maxAttempts = 4

// shouldRetry reports whether a request attempt should be retried. GET is
// safe to retry on any network error or retryable status. Mutating methods
// only retry on 429 — the server explicitly refused the request before
// processing it — because a network error or 5xx may have already applied
// the change. A completed non-retryable response is never retried.
func shouldRetry(method string, resp *http.Response, err error) bool {
	if method == http.MethodGet {
		if err != nil {
			return true
		}
		return isRetryableStatus(resp.StatusCode)
	}
	return err == nil && resp != nil && resp.StatusCode == http.StatusTooManyRequests
}

// isRetryableStatus reports whether status looks transient rather than a
// deterministic failure.
func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// retryDelay computes the backoff before the next attempt. It honors a
// Retry-After header (seconds or an HTTP date) when the server sent one,
// otherwise falls back to exponential backoff with jitter.
func retryDelay(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			return d
		}
	}
	const base = 200 * time.Millisecond
	backoff := base * time.Duration(1<<attempt)
	jitter := time.Duration(rand.Int63n(int64(base)))
	return backoff + jitter
}

// parseRetryAfter interprets a Retry-After header value, which is either a
// delay in seconds or an HTTP date.
func parseRetryAfter(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(value); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := time.Until(t); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

// wait blocks for d before the next retry, returning early if ctx is
// cancelled.
func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
