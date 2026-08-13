// Package atlapi is a client for the Atlassian Cloud REST APIs (Jira,
// Jira Software, Confluence). It routes operationIds to endpoints via the
// embedded catalog, authenticates with HTTP Basic email:APIToken, retries
// transient transport failures with backoff (honoring Retry-After), and
// decodes both JSON object and array responses. A lower-level Do escape
// hatch lets curated commands drive endpoints directly.
package atlapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/branow/atlassian-cli/internal/atlapi/catalog"
	"github.com/branow/atlassian-cli/internal/credentials"
)

const requestTimeout = 30 * time.Second

// Client invokes Atlassian operations. It is an interface so commands can
// be tested against a fake implementation instead of a real network call.
type Client interface {
	// Call routes operation through the catalog, fills path params from
	// fields (case-insensitive by name), sends the rest as query
	// parameters (GET/DELETE) or a JSON body (otherwise), authenticates
	// with Basic auth, and returns the parsed response.
	Call(ctx context.Context, operation string, fields map[string]any) (*Response, error)
	// Do is the lower-level escape hatch curated commands use directly:
	// method and a full post-host path, with an explicit query and body.
	Do(ctx context.Context, method, path string, query url.Values, body any) (*Response, error)
}

// HTTPClient is the real Client implementation, sending JSON requests to a
// single site's base URL with credentials as HTTP Basic auth.
type HTTPClient struct {
	baseURL    string
	creds      credentials.Credentials
	httpClient *http.Client
}

// New returns an HTTPClient. baseURL is the scheme+host prefix (e.g.
// "https://your-org.atlassian.net"); when empty it is derived from
// creds.Site, defaulting to https:// unless the site already carries a
// scheme (which lets tests and local proxies point at an http:// host).
// Tests pass an httptest server URL as baseURL directly.
func New(baseURL string, creds credentials.Credentials) *HTTPClient {
	if baseURL == "" {
		baseURL = "https://" + strings.TrimPrefix(strings.TrimPrefix(creds.Site, "https://"), "http://")
	}
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		creds:      creds,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// BaseURL normalizes a stored site host into a scheme+host base URL and
// rejects cleartext http:// — every request carries email:APIToken as HTTP
// Basic auth, so http:// would send the token in the clear. A bare host
// defaults to https://; an explicit http:// is allowed only when ATL_INSECURE
// is set, which keeps the escape hatch for local test proxies behind an opt-in
// instead of accepting it silently for real credentials. Callers that build a
// live client from stored credentials go through this; tests pass an explicit
// (loopback http) baseURL to New directly and are unaffected.
func BaseURL(site string) (string, error) {
	site = strings.TrimRight(site, "/")
	switch {
	case strings.HasPrefix(site, "https://"):
		return site, nil
	case strings.HasPrefix(site, "http://"):
		if insecureAllowed() {
			return site, nil
		}
		return "", fmt.Errorf("refusing insecure base URL %q: HTTP Basic auth would be sent in cleartext (set ATL_INSECURE=1 to override for a local proxy)", site)
	default:
		return "https://" + site, nil
	}
}

func insecureAllowed() bool {
	v := os.Getenv("ATL_INSECURE")
	return v == "1" || strings.EqualFold(v, "true")
}

// Call resolves operation top-level (all products) and invokes its
// endpoint. An ambiguous or unknown id fails before any network call.
func (c *HTTPClient) Call(ctx context.Context, operation string, fields map[string]any) (*Response, error) {
	op, ok := catalog.Lookup(operation)
	if !ok {
		return nil, &UnknownOperationError{Operation: operation}
	}
	if op.Method == "" {
		return nil, &UnsupportedOperationError{Operation: operation}
	}
	method, path, query, body, err := Route(op, fields)
	if err != nil {
		return nil, err
	}
	return c.Do(ctx, method, path, query, body)
}

// Do performs one request: baseURL+path, optional query and JSON body,
// Basic auth, with retry on transient failures. It is the shared core both
// Call and curated commands funnel through.
func (c *HTTPClient) Do(ctx context.Context, method, path string, query url.Values, body any) (*Response, error) {
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	resp, err := c.doWithRetry(ctx, method, requestURL, encoded)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseResponse(resp.StatusCode, responseBody)
}

// Route computes the concrete request for an operation and its fields: path
// params are substituted into the path (case-insensitive by name), and the
// remaining fields become query parameters for GET/DELETE or a JSON body
// otherwise (a field matching a documented query parameter always goes to
// the query). Field names are canonicalized to their documented casing.
// This is the single place the operation-to-request rule lives, shared by
// Client.Call and the namespaced api commands.
func Route(op catalog.Operation, fields map[string]any) (method, path string, query url.Values, body any, err error) {
	path = op.Path
	consumed := map[string]bool{}

	for _, param := range op.PathParams {
		key, value, found := findField(fields, param.Name)
		if !found {
			return "", "", nil, nil, fmt.Errorf("missing required path parameter %q for operation %s", param.Name, op.ID)
		}
		path = strings.ReplaceAll(path, "{"+param.Name+"}", url.PathEscape(queryValue(value)))
		consumed[key] = true
	}

	query = url.Values{}
	payload := map[string]any{}
	isQueryMethod := op.Method == http.MethodGet || op.Method == http.MethodDelete
	for key, value := range fields {
		if consumed[key] {
			continue
		}
		if isQueryMethod {
			addQuery(query, canonicalName(op.Query, key), value)
			continue
		}
		if name, ok := matchField(op.Query, key); ok {
			addQuery(query, name, value)
			continue
		}
		payload[canonicalName(op.Body, key)] = value
	}

	if len(payload) > 0 {
		body = payload
	}
	return op.Method, path, query, body, nil
}

// findField returns the field entry matching name case-insensitively,
// along with the caller's original key.
func findField(fields map[string]any, name string) (string, any, bool) {
	for key, value := range fields {
		if strings.EqualFold(key, name) {
			return key, value, true
		}
	}
	return "", nil, false
}

// matchField returns a documented field's canonical name when one matches
// key case-insensitively.
func matchField(fields []catalog.Field, key string) (string, bool) {
	for _, f := range fields {
		if strings.EqualFold(f.Name, key) {
			return f.Name, true
		}
	}
	return "", false
}

// canonicalName returns the documented casing for key when it matches a
// field, else key unchanged (the API is the authority; the catalog may be
// incomplete).
func canonicalName(fields []catalog.Field, key string) string {
	if name, ok := matchField(fields, key); ok {
		return name
	}
	return key
}

// addQuery appends value to query under name. An array value expands into
// repeated keys (ids=1&ids=2) — the serialization Atlassian list parameters
// expect — rather than a single bracketed blob; it uses Add, not Set, so a
// multi-valued parameter survives as repeated keys instead of collapsing to
// one. Scalars are rendered without JSON array/quote syntax.
func addQuery(query url.Values, name string, value any) {
	if arr, ok := value.([]any); ok {
		for _, elem := range arr {
			query.Add(name, queryValue(elem))
		}
		return
	}
	query.Add(name, queryValue(value))
}

// queryValue renders a single field value as a query-parameter or path string:
// strings pass through raw, numbers and booleans as their literal text (so
// maxResults=42 is "42", not a JSON-quoted value), and anything else as its
// JSON encoding.
func queryValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func (c *HTTPClient) doWithRetry(ctx context.Context, method, requestURL string, body []byte) (*http.Response, error) {
	var resp *http.Response
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err = c.attempt(ctx, method, requestURL, body)
		lastAttempt := attempt == maxAttempts-1
		if !shouldRetry(method, resp, err) || lastAttempt {
			return resp, err
		}
		delay := retryDelay(resp, attempt)
		if resp != nil {
			resp.Body.Close()
		}
		if werr := wait(ctx, delay); werr != nil {
			return nil, werr
		}
	}
	return resp, err
}

func (c *HTTPClient) attempt(ctx context.Context, method, requestURL string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.creds.Email, c.creds.APIToken)
	return c.httpClient.Do(req)
}
