package cmd_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/branow/atlassian-cli/cmd"
	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// jsonHandler responds with body and records each request's method, path,
// query, and JSON payload for assertions.
type jsonHandler struct {
	body string

	requests  int
	gotMethod string
	gotPath   string
	gotQuery  map[string][]string
	gotBody   map[string]any
}

func (h *jsonHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.requests++
	h.gotMethod = r.Method
	h.gotPath = r.URL.Path
	h.gotQuery = r.URL.Query()
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		json.Unmarshal(data, &h.gotBody)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(h.body))
}

func TestAPIHappyPathFillsPathParam(t *testing.T) {
	handler := &jsonHandler{body: `{"id": "10001", "key": "TEST"}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getProject", "-f", "projectIdOrKey=TEST", "-f", "expand=description"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if handler.gotMethod != http.MethodGet || handler.gotPath != "/rest/api/3/project/TEST" {
		t.Errorf("got %s %s, want GET /rest/api/3/project/TEST", handler.gotMethod, handler.gotPath)
	}
	if got := handler.gotQuery["expand"]; len(got) != 1 || got[0] != "description" {
		t.Errorf("got query expand=%v, want [description]", got)
	}
	if !strings.Contains(out.String(), `"key": "TEST"`) {
		t.Errorf("got output %q, want it to contain the response key", out.String())
	}
}

func TestAPITypedFieldsRouteToQueryAndBody(t *testing.T) {
	handler := &jsonHandler{body: `{"id": "10002", "key": "TEST-1"}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "createIssue",
		"-f", `fields={"summary":"hi"}`,
		"-f", "updateHistory=true",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if handler.gotMethod != http.MethodPost || handler.gotPath != "/rest/api/3/issue" {
		t.Errorf("got %s %s, want POST /rest/api/3/issue", handler.gotMethod, handler.gotPath)
	}
	if got := handler.gotQuery["updateHistory"]; len(got) != 1 || got[0] != "true" {
		t.Errorf("got query updateHistory=%v, want [true] (documented query param)", got)
	}
	if _, ok := handler.gotBody["fields"].(map[string]any); !ok {
		t.Errorf("got body %v, want a nested fields object", handler.gotBody)
	}
	if _, present := handler.gotBody["updateHistory"]; present {
		t.Errorf("updateHistory must not be in the body: %v", handler.gotBody)
	}
}

func TestAPIUnauthorizedMapsToAuthExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages": ["Client must be authenticated"]}`))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getProject", "-f", "projectIdOrKey=TEST"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an HTTP 401 response")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAPINotLoggedIn(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getProject", "-f", "projectIdOrKey=TEST"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error when no credentials are configured")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAPIUnknownOperationBeatsNotLoggedIn(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "noSuchOperation"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown operation")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d (a typo must not be masked by auth state)", got, cmdutil.ExitValidation)
	}
}

// getIssue exists in both jira and jira-software; the top-level api must
// reject it as ambiguous, before any credential or network use, and point
// at the namespaced form.
func TestAPIAmbiguousOperationTopLevel(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getIssue", "-f", "issueIdOrKey=TEST-1"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an ambiguity error for a cross-product operationId")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
	if !strings.Contains(err.Error(), "atl jira api") {
		t.Errorf("expected the error to suggest a namespaced form, got %q", err.Error())
	}
}

func TestAPINamespacedResolvesJiraOperation(t *testing.T) {
	handler := &jsonHandler{body: `{"key": "TEST"}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"jira", "api", "getProject", "-f", "projectIdOrKey=TEST"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if handler.gotPath != "/rest/api/3/project/TEST" {
		t.Errorf("got path %q, want /rest/api/3/project/TEST", handler.gotPath)
	}
}

func TestAPIListPrintsOperationsWithoutCredentials(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "--list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "createIssue") {
		t.Errorf("expected the operation list to contain createIssue")
	}
}

func TestAPIListNamespacedJira(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"jira", "api", "--list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "getProject") {
		t.Errorf("expected the jira operation list to contain getProject")
	}
	if strings.Contains(out.String(), "getPageById") {
		t.Errorf("jira namespace list should not contain a confluence-only operation")
	}
}

func TestAPIDescribePrintsFieldsWithoutCredentials(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getProject", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"getProject", "product jira",
		"GET /rest/api/3/project/{projectIdOrKey}",
		"Path parameters", "projectIdOrKey", "required",
		"Query parameters", "expand",
		"Response",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output missing %q, got:\n%s", want, got)
		}
	}
}

func TestAPIDescribeJSON(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "json", "api", "createIssue", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var view map[string]any
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("describe -o json is not valid JSON: %v", err)
	}
	if view["operation"] != "createIssue" || view["product"] != "jira" || view["method"] != "POST" {
		t.Errorf("got %v, want createIssue/jira/POST", view)
	}
	if _, ok := view["body"].([]any); !ok {
		t.Errorf("want a body field list in %v", view)
	}
}

func TestAPIDescribeRejectsFieldCombination(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "getProject", "--describe", "-f", "projectIdOrKey=TEST"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for --describe with --field")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAPIDescribeUnknownOperation(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "notAnOperation", "--describe"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error describing an uncatalogued operation")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAPIRejectsInvalidOutputFlag(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "yaml", "api", "--list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for -o yaml")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

// "atl jira api v2" reaches the Jira REST v2 surface, whose wiki-markup
// comment body is what renders an attached image or video inline.
func TestAPIV2RoutesToRESTv2(t *testing.T) {
	handler := &jsonHandler{body: `{"id": "10000"}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"jira", "api", "v2", "addComment",
		"-f", "issueIdOrKey=TEST-1", "-f", "body=!walkthrough.mp4!"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if handler.gotMethod != http.MethodPost || handler.gotPath != "/rest/api/2/issue/TEST-1/comment" {
		t.Errorf("got %s %s, want POST /rest/api/2/issue/TEST-1/comment", handler.gotMethod, handler.gotPath)
	}
	if got := handler.gotBody["body"]; got != "!walkthrough.mp4!" {
		t.Errorf("got body %v, want the wiki markup passed through verbatim", got)
	}
}

// The same operationId under "atl jira api" must keep routing to v3, so
// adding v2 changes nothing for callers who did not ask for it.
func TestAPIV3UnaffectedByV2(t *testing.T) {
	handler := &jsonHandler{body: `{"id": "10000"}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"jira", "api", "addComment", "-f", "issueIdOrKey=TEST-1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if handler.gotPath != "/rest/api/3/issue/TEST-1/comment" {
		t.Errorf("got path %q, want /rest/api/3/issue/TEST-1/comment", handler.gotPath)
	}
}

// --product pins v2 from the ordinary "atl jira api" command, the escape
// hatch for callers already composing a --product call.
func TestAPIV2ReachableByProductPin(t *testing.T) {
	handler := &jsonHandler{body: `{"id": "10000"}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"jira", "api", "addComment", "--product", "jira-v2", "-f", "issueIdOrKey=TEST-1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if handler.gotPath != "/rest/api/2/issue/TEST-1/comment" {
		t.Errorf("got path %q, want /rest/api/2/issue/TEST-1/comment", handler.gotPath)
	}
}

func TestAPIV2ListAndDescribe(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"jira", "api", "v2", "--list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "addComment") {
		t.Error("expected the v2 operation list to contain addComment")
	}

	f, out, _ = newTestFactory(t, "")
	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"jira", "api", "v2", "addComment", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "/rest/api/2/issue/{issueIdOrKey}/comment") {
		t.Errorf("got describe output %q, want the v2 path", out.String())
	}
	// The body is a wiki-markup string in v2; describing it with v3's ADF
	// shape would send users back to hand-built media nodes.
	if !strings.Contains(out.String(), "body                    string") {
		t.Errorf("got describe output %q, want body typed as string", out.String())
	}
}
