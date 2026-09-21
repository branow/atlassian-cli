package atlapi

import (
	"encoding/json"
	"mime"
	"net/http"
	"strings"
)

// Response is the decoded body of a completed (2xx) REST response.
// Atlassian endpoints return either a JSON object or a bare JSON array;
// Body holds whichever it was, Fields exposes it as a map for the generic
// api command (a top-level array is wrapped as {"values": [...]}), and Raw
// keeps the untouched bytes for curated commands that decode into typed
// structs.
type Response struct {
	Status int
	Body   any
	Fields map[string]any
	Raw    []byte
	// ContentType is the response's declared media type, verbatim.
	ContentType string
	// Binary marks a 2xx response whose body is not JSON (an attachment's
	// bytes, an export, a thumbnail). Body and Fields are empty for one;
	// Raw holds what the server sent, and the caller is expected to write
	// it out rather than render it as JSON.
	Binary bool
}

// parseResponse decodes a REST response. Non-2xx statuses become *APIError
// (flagged rate-limited when the status and headers say so); a 2xx body decodes
// into a *Response.
func parseResponse(statusCode int, header http.Header, body []byte) (*Response, error) {
	if statusCode < 200 || statusCode > 299 {
		return nil, &APIError{
			Status:      statusCode,
			Message:     errorMessage(body),
			Raw:         body,
			RateLimited: isRateLimited(statusCode, header),
		}
	}

	contentType := ""
	if header != nil {
		contentType = header.Get("Content-Type")
	}
	resp := &Response{Status: statusCode, Raw: body, ContentType: contentType}
	if len(body) == 0 {
		resp.Fields = map[string]any{}
		return resp, nil
	}
	if err := json.Unmarshal(body, &resp.Body); err != nil {
		// A body the server never claimed was JSON is data, not a failure:
		// attachment and export endpoints answer with the file's own media
		// type, and decoding it as JSON is what used to turn a successful
		// download into an error with nothing written. A body that claims
		// to be JSON and is not remains a hard error.
		if !isJSONContentType(contentType) {
			resp.Binary = true
			resp.Fields = map[string]any{}
			return resp, nil
		}
		return nil, &APIError{Status: statusCode, Message: "response was not valid JSON", Raw: body}
	}
	switch v := resp.Body.(type) {
	case map[string]any:
		resp.Fields = v
	case []any:
		resp.Fields = map[string]any{"values": v}
	default:
		resp.Fields = map[string]any{"value": v}
	}
	return resp, nil
}

// isJSONContentType reports whether a Content-Type promises JSON. An absent
// or unparseable type counts as JSON: every Atlassian REST endpoint answers
// in JSON, so an undeclared body that fails to decode is a broken response,
// not a file.
func isJSONContentType(contentType string) bool {
	if strings.TrimSpace(contentType) == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return true
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") || mediaType == "text/json"
}

// isRateLimited reports whether a non-2xx response is a throttling signal to
// back off and retry rather than a hard failure. A 429 always is. Atlassian
// also returns 503 both for transient overload (with a Retry-After header) and
// for permanent conditions like a deactivated site (without one), so a 503
// counts only when it carries a Retry-After.
func isRateLimited(status int, header http.Header) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status == http.StatusServiceUnavailable && header != nil {
		if _, ok := parseRetryAfter(header.Get("Retry-After")); ok {
			return true
		}
	}
	return false
}

// errorMessage extracts a human-readable message from an Atlassian error
// body: the first of errorMessages[], message, error, or errors — falling
// back to the trimmed raw text.
func errorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if msgs := stringList(payload["errorMessages"]); len(msgs) > 0 {
			return strings.Join(msgs, "; ")
		}
		for _, key := range []string{"message", "error", "detail", "title"} {
			if s, ok := payload[key].(string); ok && s != "" {
				return s
			}
		}
		if errs := errorsMapValues(payload["errors"]); errs != "" {
			return errs
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}

func stringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// errorsMapValues renders Atlassian's field-keyed "errors" object (e.g.
// {"summary": "must not be empty"}) as a single message.
func errorsMapValues(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	var parts []string
	for key, val := range m {
		if s, ok := val.(string); ok && s != "" {
			parts = append(parts, key+": "+s)
		}
	}
	return strings.Join(parts, "; ")
}
