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

// recordedRequest captures the parts of a request a test asserts on.
type recordedRequest struct {
	method string
	path   string
	query  map[string][]string
	body   map[string]any
}

// capture reads r into a recordedRequest, decoding a JSON body when present.
func capture(r *http.Request) recordedRequest {
	rec := recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.Query()}
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		json.Unmarshal(data, &rec.body)
	}
	return rec
}

func TestConfluenceSpaceListTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[{"id":"1","key":"DS","name":"Demo","type":"global"},{"id":2,"key":"ENG","name":"Eng","type":"personal"}]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "space", "ls"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	for _, want := range []string{"ID", "KEY", "DS", "Demo", "ENG", "2"} {
		if !strings.Contains(got, want) {
			t.Errorf("space ls output missing %q:\n%s", want, got)
		}
	}
}

func TestConfluencePageGetTableAndBody(t *testing.T) {
	body := `{"id":"12345","title":"Hello","spaceId":"9","status":"current","version":{"number":3},"body":{"storage":{"value":"<p>hi</p>","representation":"storage"}}}`
	var last recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = capture(r)
		w.Write([]byte(body))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "page", "get", "12345"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if last.path != "/wiki/api/v2/pages/12345" {
		t.Errorf("got path %q", last.path)
	}
	if got := last.query["body-format"]; len(got) != 1 || got[0] != "storage" {
		t.Errorf("got body-format=%v, want [storage]", got)
	}
	got := out.String()
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "12345") {
		t.Errorf("page get table missing fields:\n%s", got)
	}

	// --body prints just the storage markup.
	f2, out2, _ := newTestFactory(t, server.URL)
	root2 := cmd.NewRootCmd(f2)
	root2.SetArgs([]string{"confluence", "page", "get", "12345", "--body"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("Execute --body: %v", err)
	}
	if strings.TrimSpace(out2.String()) != "<p>hi</p>" {
		t.Errorf("got --body output %q, want <p>hi</p>", out2.String())
	}
}

func TestConfluencePageCreateResolvesSpaceKey(t *testing.T) {
	var createBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/wiki/api/v2/spaces", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("keys"); got != "DS" {
			t.Errorf("got keys=%q, want DS", got)
		}
		w.Write([]byte(`{"results":[{"id":"789","key":"DS","name":"Demo","type":"global"}]}`))
	})
	mux.HandleFunc("/wiki/api/v2/pages", func(w http.ResponseWriter, r *http.Request) {
		rec := capture(r)
		createBody = rec.body
		w.Write([]byte(`{"id":"999","title":"Notes","spaceId":"789","version":{"number":1}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "page", "create", "--space", "DS", "--title", "Notes", "--body", "<p>x</p>"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if createBody["spaceId"] != "789" {
		t.Errorf("got spaceId %v, want 789 (resolved from key)", createBody["spaceId"])
	}
	if createBody["title"] != "Notes" {
		t.Errorf("got title %v", createBody["title"])
	}
	if !strings.Contains(out.String(), "999") {
		t.Errorf("create output missing new page id:\n%s", out.String())
	}
}

func TestConfluencePageDeleteRequiresConfirmation(t *testing.T) {
	var deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	// Without --force in a non-interactive session, delete is refused.
	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "page", "delete", "12345"})
	err := root.Execute()
	if err == nil || cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Fatalf("expected a validation error without --force, got %v", err)
	}
	if deleted {
		t.Fatal("page must not be deleted without confirmation")
	}

	// With --force it proceeds.
	f2, _, _ := newTestFactory(t, server.URL)
	root2 := cmd.NewRootCmd(f2)
	root2.SetArgs([]string{"confluence", "page", "delete", "12345", "--force"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("Execute --force: %v", err)
	}
	if !deleted {
		t.Fatal("page should be deleted with --force")
	}
}

func TestConfluencePageDeletePurge(t *testing.T) {
	var deletes []string // records the query for each DELETE, in order
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes = append(deletes, r.URL.Query().Get("purge"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "page", "delete", "12345", "--purge", "--force"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute --purge: %v", err)
	}
	// Purge trashes first, then deletes permanently: two DELETEs, the second
	// carrying purge=true.
	if len(deletes) != 2 || deletes[0] != "" || deletes[1] != "true" {
		t.Fatalf("expected [trash, purge=true], got %v", deletes)
	}
}

func TestConfluencePagePatchHeadingDryRun(t *testing.T) {
	page := `{"id":"12345","title":"T","spaceId":"9","status":"current","version":{"number":4},"body":{"storage":{"value":"<h2>Status</h2><p>old</p><h2>Next</h2><p>keep</p>","representation":"storage"}}}`
	var putSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putSeen = true
		}
		w.Write([]byte(page))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "page", "patch", "12345", "--heading", "Status", "--content", "<p>new</p>", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if putSeen {
		t.Error("--dry-run must not PUT")
	}
	got := out.String()
	if !strings.Contains(got, "<h2>Status</h2><p>new</p><h2>Next</h2>") {
		t.Errorf("dry-run body not patched as expected:\n%s", got)
	}
}

func TestConfluencePagePatchRegexBumpsVersion(t *testing.T) {
	page := `{"id":"12345","title":"T","spaceId":"9","status":"current","version":{"number":4},"body":{"storage":{"value":"<p>DRAFT</p>","representation":"storage"}}}`
	var putBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putBody = capture(r).body
			w.Write([]byte(`{"id":"12345","title":"T","spaceId":"9","version":{"number":5}}`))
			return
		}
		w.Write([]byte(page))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "page", "patch", "12345", "--regex", "DRAFT", "--replacement", "FINAL"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if putBody == nil {
		t.Fatal("expected a PUT with the patched body")
	}
	version, _ := putBody["version"].(map[string]any)
	if version == nil || version["number"] != float64(5) {
		t.Errorf("got version %v, want number 5 (current+1)", putBody["version"])
	}
	body, _ := putBody["body"].(map[string]any)
	if body == nil || body["value"] != "<p>FINAL</p>" {
		t.Errorf("got body %v, want value <p>FINAL</p>", putBody["body"])
	}
}

func TestConfluencePagePatchNoMatchIsValidationError(t *testing.T) {
	page := `{"id":"12345","title":"T","version":{"number":1},"body":{"storage":{"value":"<p>hello</p>","representation":"storage"}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(page))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "page", "patch", "12345", "--regex", "nope", "--replacement", "x"})
	err := root.Execute()
	if err == nil || cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Fatalf("expected a validation error for a no-match patch, got %v", err)
	}
}

func TestConfluencePagePatchRejectsMultipleSelectors(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "page", "patch", "12345", "--heading", "H", "--regex", "R", "--content", "c", "--replacement", "x"})
	err := root.Execute()
	if err == nil || cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Fatalf("expected a validation error for multiple selectors, got %v", err)
	}
}

func TestConfluenceSearchTable(t *testing.T) {
	var last recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = capture(r)
		w.Write([]byte(`{"results":[{"content":{"id":"10","type":"page","title":"Hello","space":{"key":"DS"}}}]}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "search", "type = page", "--limit", "5"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if last.path != "/wiki/rest/api/search" {
		t.Errorf("got path %q, want /wiki/rest/api/search", last.path)
	}
	if got := last.query["cql"]; len(got) != 1 || got[0] != "type = page" {
		t.Errorf("got cql=%v", got)
	}
	if got := last.query["limit"]; len(got) != 1 || got[0] != "5" {
		t.Errorf("got limit=%v", got)
	}
	got := out.String()
	if !strings.Contains(got, "10") || !strings.Contains(got, "Hello") || !strings.Contains(got, "DS") {
		t.Errorf("search table missing fields:\n%s", got)
	}
}

func TestConfluenceCommentAddFooter(t *testing.T) {
	var last recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = capture(r)
		w.Write([]byte(`{"id":"555"}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "comment", "add", "12345", "--body", "Looks good"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if last.path != "/wiki/api/v2/footer-comments" {
		t.Errorf("got path %q, want footer-comments", last.path)
	}
	if last.body["pageId"] != "12345" {
		t.Errorf("got pageId %v", last.body["pageId"])
	}
	if _, ok := last.body["inlineCommentProperties"]; ok {
		t.Error("footer comment must not carry inlineCommentProperties")
	}
	if !strings.Contains(out.String(), "555") {
		t.Errorf("comment output missing id:\n%s", out.String())
	}
}

func TestConfluenceCommentAddInline(t *testing.T) {
	var last recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = capture(r)
		w.Write([]byte(`{"id":"777"}`))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "comment", "add", "12345", "--inline", "--select", "the API", "--match-index", "2", "--body", "which?"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if last.path != "/wiki/api/v2/inline-comments" {
		t.Errorf("got path %q, want inline-comments", last.path)
	}
	props, _ := last.body["inlineCommentProperties"].(map[string]any)
	if props == nil || props["textSelection"] != "the API" || props["textSelectionMatchIndex"] != float64(2) {
		t.Errorf("got inlineCommentProperties %v", last.body["inlineCommentProperties"])
	}
	// The v2 spec requires textSelectionMatchCount strictly greater than the
	// index, so a repeated selection anchors to the intended occurrence.
	if props["textSelectionMatchCount"] != float64(3) {
		t.Errorf("got textSelectionMatchCount %v, want 3 (index+1)", props["textSelectionMatchCount"])
	}
}

// TestConfluenceSpaceListPaginates follows the v2 _links.next cursor with
// --paginate, aggregating both pages into one -o json results envelope.
func TestConfluenceSpaceListPaginates(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		if r.URL.Query().Get("cursor") == "next" {
			w.Write([]byte(`{"results":[{"id":"3","key":"C","name":"Cee","type":"global"}],"_links":{}}`))
			return
		}
		w.Write([]byte(`{"results":[{"id":"1","key":"A","name":"Ay","type":"global"},{"id":"2","key":"B","name":"Bee","type":"global"}],"_links":{"next":"/wiki/api/v2/spaces?cursor=next"}}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "space", "ls", "--paginate", "-o", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 page fetches, got %d: %v", len(paths), paths)
	}
	var env struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &env); err != nil {
		t.Fatalf("output was not a results envelope: %v\n%s", err, out.String())
	}
	if len(env.Results) != 3 {
		t.Errorf("got %d combined results, want 3: %s", len(env.Results), out.String())
	}
}

func TestConfluenceCommentInlineRequiresSelect(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"confluence", "comment", "add", "12345", "--inline", "--body", "hi"})
	err := root.Execute()
	if err == nil || cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Fatalf("expected a validation error for --inline without --select, got %v", err)
	}
}
