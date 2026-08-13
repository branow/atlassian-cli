package atlapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/atlapi/catalog"
	"github.com/branow/atlassian-cli/internal/credentials"
)

func TestRouteExpandsArrayQueryIntoRepeatedKeys(t *testing.T) {
	op := catalog.Operation{
		ID:     "listBoards",
		Method: http.MethodGet,
		Path:   "/rest/agile/1.0/board",
		Query:  []catalog.Field{{Name: "id"}, {Name: "maxResults"}, {Name: "done"}},
	}
	_, _, query, _, err := atlapi.Route(op, map[string]any{
		"id":         []any{1, "TEST-2", 3},
		"maxResults": 42,
		"done":       true,
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if got := query["id"]; len(got) != 3 || got[0] != "1" || got[1] != "TEST-2" || got[2] != "3" {
		t.Errorf("array param must expand into repeated keys, got id=%v", got)
	}
	if got := query.Get("maxResults"); got != "42" {
		t.Errorf("number param must render as its literal, got maxResults=%q", got)
	}
	if got := query.Get("done"); got != "true" {
		t.Errorf("bool param must render as true/false, got done=%q", got)
	}
}

func TestBaseURLRejectsCleartextHTTP(t *testing.T) {
	if _, err := atlapi.BaseURL("http://acme.atlassian.net"); err == nil {
		t.Error("http:// base URL must be rejected without ATL_INSECURE")
	}
	if got, err := atlapi.BaseURL("acme.atlassian.net"); err != nil || got != "https://acme.atlassian.net" {
		t.Errorf("bare host must default to https, got %q err %v", got, err)
	}
	if got, err := atlapi.BaseURL("https://acme.atlassian.net/"); err != nil || got != "https://acme.atlassian.net" {
		t.Errorf("https host must pass through trimmed, got %q err %v", got, err)
	}
	t.Setenv("ATL_INSECURE", "1")
	if got, err := atlapi.BaseURL("http://127.0.0.1:8080"); err != nil || got != "http://127.0.0.1:8080" {
		t.Errorf("ATL_INSECURE must allow http, got %q err %v", got, err)
	}
}

func testCreds() credentials.Credentials {
	return credentials.Credentials{Site: "acme.atlassian.net", Email: "alice@example.com", APIToken: "t0ken"}
}

func TestCallGETFillsPathParamQueryAndBasicAuth(t *testing.T) {
	var gotMethod, gotPath string
	var gotQuery url.Values
	var gotUser, gotPass string
	var gotAuthOK bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotUser, gotPass, gotAuthOK = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id": "10001", "key": "TEST"}`))
	}))
	defer server.Close()

	client := atlapi.New(server.URL, testCreds())
	resp, err := client.Call(context.Background(), "getProject", map[string]any{
		"projectIdOrKey": "TEST",
		"expand":         "description",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("got method %q, want GET", gotMethod)
	}
	if gotPath != "/rest/api/3/project/TEST" {
		t.Errorf("got path %q, want /rest/api/3/project/TEST", gotPath)
	}
	if got := gotQuery["expand"]; len(got) != 1 || got[0] != "description" {
		t.Errorf("got query expand=%v, want [description]", got)
	}
	if _, present := gotQuery["projectIdOrKey"]; present {
		t.Error("path parameter must not leak into the query string")
	}
	if !gotAuthOK || gotUser != "alice@example.com" || gotPass != "t0ken" {
		t.Errorf("got Basic auth %q:%q (ok=%v), want alice@example.com:t0ken", gotUser, gotPass, gotAuthOK)
	}
	if resp.Fields["key"] != "TEST" {
		t.Errorf("got key %v, want TEST", resp.Fields["key"])
	}
}

func TestCallPOSTSendsJSONBodyAndRoutesQueryParams(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotQuery url.Values
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotContentType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &gotBody)
		w.Write([]byte(`{"id": "10002", "key": "TEST-1"}`))
	}))
	defer server.Close()

	client := atlapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "createIssue", map[string]any{
		"fields":        map[string]any{"summary": "hi"},
		"updateHistory": true, // a documented query parameter, not a body field
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/rest/api/3/issue" {
		t.Errorf("got %s %s, want POST /rest/api/3/issue", gotMethod, gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("got Content-Type %q, want application/json", gotContentType)
	}
	if got := gotQuery["updateHistory"]; len(got) != 1 || got[0] != "true" {
		t.Errorf("got query updateHistory=%v, want [true] (documented query param must not go in the body)", got)
	}
	if _, present := gotBody["updateHistory"]; present {
		t.Errorf("updateHistory must not appear in the JSON body: %v", gotBody)
	}
	if _, ok := gotBody["fields"].(map[string]any); !ok {
		t.Errorf("got body %v, want a nested fields object", gotBody)
	}
}

func TestDoDecodesArrayResponseIntoValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id": 1}, {"id": 2}]`))
	}))
	defer server.Close()

	client := atlapi.New(server.URL, testCreds())
	resp, err := client.Do(context.Background(), http.MethodGet, "/rest/api/3/project", nil, nil)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	values, ok := resp.Fields["values"].([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("got Fields[values]=%v, want a two-element array", resp.Fields["values"])
	}
	if _, ok := resp.Body.([]any); !ok {
		t.Errorf("got Body %T, want []any", resp.Body)
	}
}

func TestCallDecodes401AsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages": ["Client must be authenticated"]}`))
	}))
	defer server.Close()

	client := atlapi.New(server.URL, testCreds())
	resp, err := client.Call(context.Background(), "getProject", map[string]any{"projectIdOrKey": "TEST"})

	var apiErr *atlapi.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got error %v, want *atlapi.APIError", err)
	}
	if apiErr.Status != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", apiErr.Status)
	}
	if apiErr.Message != "Client must be authenticated" {
		t.Errorf("got message %q, want the errorMessages text", apiErr.Message)
	}
	if resp != nil {
		t.Errorf("got resp %v, want nil for a non-2xx response", resp)
	}
}

func TestCallGETRetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"key": "TEST"}`))
	}))
	defer server.Close()

	client := atlapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "getProject", map[string]any{"projectIdOrKey": "TEST"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("got %d attempts, want 3", got)
	}
}

func TestCallGETGivesUpAfterMaxAttempts(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := atlapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "getProject", map[string]any{"projectIdOrKey": "TEST"})

	var apiErr *atlapi.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("got error %v, want *atlapi.APIError with status 503", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 4 {
		t.Errorf("got %d attempts, want 4", got)
	}
}

func TestCallPOSTDoesNotRetryOn5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := atlapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "createIssue", map[string]any{"fields": map[string]any{}})

	var apiErr *atlapi.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got error %v, want *atlapi.APIError", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("got %d attempts, want exactly 1 (mutating calls must not retry on 5xx)", got)
	}
}

func TestCallRejectsUnroutableOperationsWithoutRequest(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer server.Close()

	client := atlapi.New(server.URL, testCreds())

	t.Run("unknown operation", func(t *testing.T) {
		_, err := client.Call(context.Background(), "noSuchOperation", nil)
		var unknownErr *atlapi.UnknownOperationError
		if !errors.As(err, &unknownErr) {
			t.Fatalf("got error %v, want *atlapi.UnknownOperationError", err)
		}
	})

	t.Run("ambiguous operation resolves as unknown top-level", func(t *testing.T) {
		// getIssue exists in both jira and jira-software, so the top-level
		// Call cannot route it and must fail before any request.
		_, err := client.Call(context.Background(), "getIssue", map[string]any{"issueIdOrKey": "TEST-1"})
		var unknownErr *atlapi.UnknownOperationError
		if !errors.As(err, &unknownErr) {
			t.Fatalf("got error %v, want *atlapi.UnknownOperationError", err)
		}
	})

	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("server was hit %d times, want 0 (routing must fail before any HTTP request)", got)
	}
}
