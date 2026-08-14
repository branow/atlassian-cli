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

// recordingServer captures each request and replies with a scripted body per
// method+path, so a single fixture backs multi-request flows (e.g. transition
// = GET then POST).
type recordingServer struct {
	// routes maps "METHOD path" to the JSON body to return.
	routes map[string]string

	gotMethod string
	gotPath   string
	gotQuery  map[string][]string
	gotBody   map[string]any
	requests  int
}

func (s *recordingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.requests++
	s.gotMethod = r.Method
	s.gotPath = r.URL.Path
	s.gotQuery = r.URL.Query()
	s.gotBody = nil
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		json.Unmarshal(data, &s.gotBody)
	}
	w.Header().Set("Content-Type", "application/json")
	if body, ok := s.routes[r.Method+" "+r.URL.Path]; ok {
		w.Write([]byte(body))
		return
	}
	w.Write([]byte(`{}`))
}

func runJira(t *testing.T, serverURL string, args ...string) (out string, err error) {
	t.Helper()
	f, outBuf, _ := newTestFactory(t, serverURL)
	root := cmd.NewRootCmd(f)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), err
}

func TestJiraIssueGetTable(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/issue/PROJ-1": `{"key":"PROJ-1","fields":{"summary":"Fix it","status":{"name":"Open"},"assignee":{"displayName":"Alice"},"issuetype":{"name":"Bug"},"priority":{"name":"High"}}}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "get", "PROJ-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.gotPath != "/rest/api/3/issue/PROJ-1" || srv.gotMethod != http.MethodGet {
		t.Errorf("got %s %s", srv.gotMethod, srv.gotPath)
	}
	for _, want := range []string{"KEY", "PROJ-1", "Fix it", "Open", "Alice", "Bug", "High"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// A single issue renders as a vertical key/value block (one field per
	// line), not the horizontal one-row table it used to be.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 6 {
		t.Errorf("expected 6 key/value lines, got %d:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "KEY") || !strings.Contains(lines[0], "PROJ-1") {
		t.Errorf("first line should pair KEY with its value: %q", lines[0])
	}
}

func TestJiraIssueGetDescriptionExtractsADF(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/issue/PROJ-2": `{"key":"PROJ-2","fields":{"summary":"s","description":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello adf"}]}]}}}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "get", "PROJ-2", "--fields", "key,description")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hello adf") {
		t.Errorf("expected ADF text extracted, got:\n%s", out)
	}
}

func TestJiraIssueGetJSONDumpsRaw(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/issue/PROJ-1": `{"key":"PROJ-1","fields":{"summary":"s"}}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "-o", "json", "jira", "issue", "get", "PROJ-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if got["key"] != "PROJ-1" {
		t.Errorf("got %v", got)
	}
}

func TestJiraIssueListPostsJQL(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"POST /rest/api/3/search/jql": `{"issues":[{"key":"PROJ-1","fields":{"summary":"a","status":{"name":"Open"},"assignee":{"displayName":"Al"}}}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "list", "project = PROJ", "--limit", "5")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.gotMethod != http.MethodPost || srv.gotPath != "/rest/api/3/search/jql" {
		t.Errorf("got %s %s, want POST /rest/api/3/search/jql", srv.gotMethod, srv.gotPath)
	}
	if srv.gotBody["jql"] != "project = PROJ" {
		t.Errorf("jql = %v", srv.gotBody["jql"])
	}
	if srv.gotBody["maxResults"] != float64(5) {
		t.Errorf("maxResults = %v, want 5", srv.gotBody["maxResults"])
	}
	if _, ok := srv.gotBody["fields"].([]any); !ok {
		t.Errorf("expected a fields array in body, got %v", srv.gotBody["fields"])
	}
	if !strings.Contains(out, "PROJ-1") {
		t.Errorf("output missing issue key:\n%s", out)
	}
}

// sequenceServer replies with a scripted body per request in order, so a test
// can drive a multi-page pagination flow (page 1, page 2, ...) against one
// endpoint. It records every request body for cursor assertions.
type sequenceServer struct {
	bodies     []string
	calls      int
	gotBodies  []map[string]any
	gotQueries []map[string][]string
}

func (s *sequenceServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.gotQueries = append(s.gotQueries, r.URL.Query())
	var body map[string]any
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		json.Unmarshal(data, &body)
	}
	s.gotBodies = append(s.gotBodies, body)
	w.Header().Set("Content-Type", "application/json")
	i := s.calls
	if i >= len(s.bodies) {
		i = len(s.bodies) - 1
	}
	s.calls++
	w.Write([]byte(s.bodies[i]))
}

func TestJiraIssueListPaginateFollowsToken(t *testing.T) {
	srv := &sequenceServer{bodies: []string{
		`{"issues":[{"key":"PROJ-1","fields":{"summary":"a"}}],"nextPageToken":"tok2","isLast":false}`,
		`{"issues":[{"key":"PROJ-2","fields":{"summary":"b"}}],"isLast":true}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "list", "project = PROJ", "--paginate")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.calls != 2 {
		t.Errorf("expected 2 page requests, got %d", srv.calls)
	}
	if tok := srv.gotBodies[1]["nextPageToken"]; tok != "tok2" {
		t.Errorf("second page did not thread token: %v", srv.gotBodies[1]["nextPageToken"])
	}
	for _, want := range []string{"PROJ-1", "PROJ-2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestJiraIssueListDefaultsToFirstPage(t *testing.T) {
	srv := &sequenceServer{bodies: []string{
		`{"issues":[{"key":"PROJ-1","fields":{"summary":"a"}}],"nextPageToken":"tok2","isLast":false}`,
		`{"issues":[{"key":"PROJ-2","fields":{"summary":"b"}}],"isLast":true}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "list", "project = PROJ")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.calls != 1 {
		t.Errorf("without --paginate expected 1 request, got %d", srv.calls)
	}
	if strings.Contains(out, "PROJ-2") {
		t.Errorf("second page must not be fetched by default:\n%s", out)
	}
}

func TestJiraIssueListJSONEmitsCombinedEnvelope(t *testing.T) {
	srv := &sequenceServer{bodies: []string{
		`{"issues":[{"key":"PROJ-1","fields":{"summary":"a"}}],"nextPageToken":"tok2","isLast":false}`,
		`{"issues":[{"key":"PROJ-2","fields":{"summary":"b"}}],"isLast":true}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "-o", "json", "jira", "issue", "list", "project = PROJ", "--paginate")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got struct {
		Issues []struct {
			Key string `json:"key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not a JSON envelope: %v\n%s", err, out)
	}
	if len(got.Issues) != 2 || got.Issues[0].Key != "PROJ-1" || got.Issues[1].Key != "PROJ-2" {
		t.Errorf("combined issues = %v", got.Issues)
	}
}

func TestJiraProjectListPaginateFollowsStartAt(t *testing.T) {
	srv := &sequenceServer{bodies: []string{
		`{"startAt":0,"maxResults":1,"total":2,"isLast":false,"values":[{"id":"1","key":"A","name":"Alpha","projectTypeKey":"software"}]}`,
		`{"startAt":1,"maxResults":1,"total":2,"isLast":true,"values":[{"id":"2","key":"B","name":"Beta","projectTypeKey":"software"}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "project", "ls", "--paginate")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.calls != 2 {
		t.Errorf("expected 2 page requests, got %d", srv.calls)
	}
	if sa := srv.gotQueries[1]["startAt"]; len(sa) != 1 || sa[0] != "1" {
		t.Errorf("second page startAt = %v, want [1]", sa)
	}
	for _, want := range []string{"Alpha", "Beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestJiraProjectListLimitCaps(t *testing.T) {
	srv := &sequenceServer{bodies: []string{
		`{"startAt":0,"maxResults":5,"total":9,"isLast":false,"values":[{"id":"1","key":"A","name":"Alpha","projectTypeKey":"software"},{"id":"2","key":"B","name":"Beta","projectTypeKey":"software"}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "project", "ls", "--limit", "1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.calls != 1 {
		t.Errorf("expected 1 request, got %d", srv.calls)
	}
	if mr := srv.gotQueries[0]["maxResults"]; len(mr) != 1 || mr[0] != "1" {
		t.Errorf("maxResults hint = %v, want [1]", mr)
	}
	if strings.Contains(out, "Beta") {
		t.Errorf("--limit 1 must cap output to one row:\n%s", out)
	}
}

func TestJiraIssueListRequiresJQL(t *testing.T) {
	_, err := runJira(t, "", "jira", "issue", "list")
	if err == nil {
		t.Fatal("expected a validation error for missing JQL")
	}
	if cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Errorf("exit code = %d, want validation", cmdutil.ExitCode(err))
	}
}

func TestJiraIssueCreateBuildsFieldsAndADF(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"POST /rest/api/3/issue": `{"key":"PROJ-42"}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "create",
		"--project", "PROJ", "--type", "Bug", "--summary", "boom",
		"--description", "it broke", "-f", "labels=[\"urgent\"]")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	fields, ok := srv.gotBody["fields"].(map[string]any)
	if !ok {
		t.Fatalf("no fields object in %v", srv.gotBody)
	}
	if proj, _ := fields["project"].(map[string]any); proj["key"] != "PROJ" {
		t.Errorf("project = %v", fields["project"])
	}
	if it, _ := fields["issuetype"].(map[string]any); it["name"] != "Bug" {
		t.Errorf("issuetype = %v", fields["issuetype"])
	}
	if fields["summary"] != "boom" {
		t.Errorf("summary = %v", fields["summary"])
	}
	if desc, _ := fields["description"].(map[string]any); desc["type"] != "doc" {
		t.Errorf("description is not ADF: %v", fields["description"])
	}
	if labels, _ := fields["labels"].([]any); len(labels) != 1 || labels[0] != "urgent" {
		t.Errorf("labels = %v, want [urgent] as typed JSON", fields["labels"])
	}
	if !strings.Contains(out, "PROJ-42") {
		t.Errorf("expected created key in output:\n%s", out)
	}
}

func TestJiraIssueCreateRequiresFlags(t *testing.T) {
	_, err := runJira(t, "", "jira", "issue", "create", "--project", "PROJ")
	if err == nil {
		t.Fatal("expected a validation error for missing --type/--summary")
	}
	if cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Errorf("exit code = %d, want validation", cmdutil.ExitCode(err))
	}
}

func TestJiraIssueEditOnlySendsSetFields(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "edit", "PROJ-1", "--summary", "new title")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.gotMethod != http.MethodPut || srv.gotPath != "/rest/api/3/issue/PROJ-1" {
		t.Errorf("got %s %s", srv.gotMethod, srv.gotPath)
	}
	fields, _ := srv.gotBody["fields"].(map[string]any)
	if fields["summary"] != "new title" {
		t.Errorf("summary = %v", fields["summary"])
	}
	if _, present := fields["description"]; present {
		t.Errorf("description must not be sent when unset: %v", fields)
	}
	if !strings.Contains(out, "Updated PROJ-1") {
		t.Errorf("expected confirmation, got:\n%s", out)
	}
}

func TestJiraIssueEditNothingToUpdate(t *testing.T) {
	_, err := runJira(t, "", "jira", "issue", "edit", "PROJ-1")
	if err == nil {
		t.Fatal("expected a validation error when no fields are given")
	}
	if cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Errorf("exit code = %d, want validation", cmdutil.ExitCode(err))
	}
}

func TestJiraIssueTransitionResolvesNameToID(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/issue/PROJ-1/transitions": `{"transitions":[{"id":"11","name":"To Do"},{"id":"31","name":"Done"}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "transition", "PROJ-1", "--to", "done")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.requests != 2 {
		t.Errorf("expected GET then POST (2 requests), got %d", srv.requests)
	}
	tr, _ := srv.gotBody["transition"].(map[string]any)
	if tr["id"] != "31" {
		t.Errorf("posted transition id = %v, want 31", tr["id"])
	}
	if !strings.Contains(out, "Transitioned PROJ-1 to Done") {
		t.Errorf("output = %q", out)
	}
}

func TestJiraIssueTransitionList(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/issue/PROJ-1/transitions": `{"transitions":[{"id":"11","name":"To Do","to":{"name":"To Do"}}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "transition", "PROJ-1", "--list")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.requests != 1 {
		t.Errorf("--list must not POST, got %d requests", srv.requests)
	}
	for _, want := range []string{"ID", "NAME", "11", "To Do"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestJiraIssueTransitionUnknownName(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/issue/PROJ-1/transitions": `{"transitions":[{"id":"11","name":"To Do"}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	_, err := runJira(t, server.URL, "jira", "issue", "transition", "PROJ-1", "--to", "Nope")
	if err == nil {
		t.Fatal("expected an error for an unmatched transition")
	}
	if cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Errorf("exit code = %d, want validation", cmdutil.ExitCode(err))
	}
}

func TestJiraIssueCommentSendsADF(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"POST /rest/api/3/issue/PROJ-1/comment": `{"id":"10000"}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "comment", "PROJ-1", "--body", "looking into it")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	body, _ := srv.gotBody["body"].(map[string]any)
	if body["type"] != "doc" {
		t.Errorf("comment body is not ADF: %v", srv.gotBody["body"])
	}
	if !strings.Contains(out, "Commented on PROJ-1") {
		t.Errorf("output = %q", out)
	}
}

func TestJiraIssueCommentMarkdownResolvesMention(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/user/search":           `[{"accountId":"abc-123","displayName":"Jane Doe"}]`,
		"POST /rest/api/3/issue/PROJ-1/comment": `{"id":"10000"}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "issue", "comment", "PROJ-1",
		"--markdown", "--body", "see [PR](https://x.test) @[Jane Doe]")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.requests != 2 {
		t.Fatalf("expected user search then comment (2 requests), got %d", srv.requests)
	}
	// The posted body must carry a mention node with the resolved accountId
	// and a link node, proving markdown + resolution reached the payload.
	raw, _ := json.Marshal(srv.gotBody["body"])
	for _, want := range []string{`"type":"mention"`, `"id":"abc-123"`, `"type":"link"`, `"href":"https://x.test"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("posted body missing %s:\n%s", want, raw)
		}
	}
	if !strings.Contains(out, "Commented on PROJ-1") {
		t.Errorf("output = %q", out)
	}
}

func TestJiraIssueCommentMarkdownAmbiguousMentionFails(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/user/search": `[{"accountId":"1","displayName":"Jane Doe"},{"accountId":"2","displayName":"Jane Doe"}]`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	_, err := runJira(t, server.URL, "jira", "issue", "comment", "PROJ-1",
		"--markdown", "--body", "hi @[Jane]")
	if err == nil {
		t.Fatal("expected an error for an ambiguous mention")
	}
	if cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Errorf("exit code = %d, want validation", cmdutil.ExitCode(err))
	}
	if srv.requests != 1 {
		t.Errorf("must not POST a comment when resolution fails, got %d requests", srv.requests)
	}
}

func TestJiraProjectList(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/api/3/project/search": `{"values":[{"id":"10000","key":"PROJ","name":"Project X","projectTypeKey":"software"}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "project", "ls")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"KEY", "PROJ", "Project X", "10000", "software"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestJiraBoardListFiltersByProject(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/agile/1.0/board": `{"values":[{"id":7,"name":"Scrum","type":"scrum"}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "board", "ls", "--project", "PROJ")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := srv.gotQuery["projectKeyOrId"]; len(got) != 1 || got[0] != "PROJ" {
		t.Errorf("projectKeyOrId query = %v", got)
	}
	for _, want := range []string{"ID", "7", "Scrum", "scrum"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestJiraSprintListRequiresBoard(t *testing.T) {
	_, err := runJira(t, "", "jira", "sprint", "ls")
	if err == nil {
		t.Fatal("expected a validation error for missing --board")
	}
	if cmdutil.ExitCode(err) != cmdutil.ExitValidation {
		t.Errorf("exit code = %d, want validation", cmdutil.ExitCode(err))
	}
}

func TestJiraSprintList(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/agile/1.0/board/7/sprint": `{"values":[{"id":3,"name":"Sprint 3","state":"active"}]}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "sprint", "ls", "--board", "7")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if srv.gotPath != "/rest/agile/1.0/board/7/sprint" {
		t.Errorf("path = %s", srv.gotPath)
	}
	for _, want := range []string{"Sprint 3", "active", "3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestJiraSprintGet(t *testing.T) {
	srv := &recordingServer{routes: map[string]string{
		"GET /rest/agile/1.0/sprint/3": `{"id":3,"name":"Sprint 3","state":"closed"}`,
	}}
	server := httptest.NewServer(srv)
	defer server.Close()

	out, err := runJira(t, server.URL, "jira", "sprint", "get", "3")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Sprint 3") || !strings.Contains(out, "closed") {
		t.Errorf("output = %q", out)
	}
}
