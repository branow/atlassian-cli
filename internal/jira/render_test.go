package jira

import (
	"reflect"
	"testing"
)

func sampleIssue() Issue {
	return Issue{
		Key: "PROJ-1",
		Fields: IssueFields{
			Summary:   "Fix the thing",
			Status:    &NamedRef{Name: "In Progress"},
			Assignee:  &User{DisplayName: "Alice"},
			IssueType: &NamedRef{Name: "Bug"},
			Priority:  &NamedRef{Name: "High"},
		},
	}
}

func TestSelectIssueColumnsDefault(t *testing.T) {
	cols, apiFields, err := SelectIssueColumns(nil, DefaultListColumns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	headers, cells := Render(cols, []Issue{sampleIssue()})
	wantHeaders := []string{"KEY", "SUMMARY", "STATUS", "ASSIGNEE"}
	if !reflect.DeepEqual(headers, wantHeaders) {
		t.Errorf("headers = %v, want %v", headers, wantHeaders)
	}
	wantRow := []string{"PROJ-1", "Fix the thing", "In Progress", "Alice"}
	if !reflect.DeepEqual(cells[0], wantRow) {
		t.Errorf("row = %v, want %v", cells[0], wantRow)
	}
	// "key" has no API field, so only the value columns are requested.
	wantFields := []string{"summary", "status", "assignee"}
	if !reflect.DeepEqual(apiFields, wantFields) {
		t.Errorf("apiFields = %v, want %v", apiFields, wantFields)
	}
}

func TestSelectIssueColumnsCustomAndADF(t *testing.T) {
	issue := sampleIssue()
	issue.Fields.Description = map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "paragraph", "content": []any{
				map[string]any{"type": "text", "text": "Some detail"},
			}},
		},
	}
	cols, apiFields, err := SelectIssueColumns([]string{"key", "description"}, DefaultGetColumns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"description"}; !reflect.DeepEqual(apiFields, want) {
		t.Errorf("apiFields = %v, want %v", apiFields, want)
	}
	_, cells := Render(cols, []Issue{issue})
	if cells[0][1] != "Some detail" {
		t.Errorf("description column = %q, want extracted ADF text", cells[0][1])
	}
}

func TestSelectIssueColumnsUnknown(t *testing.T) {
	_, _, err := SelectIssueColumns([]string{"nope"}, DefaultListColumns)
	if err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}

func TestSelectIssueColumnsNilRefsRenderEmpty(t *testing.T) {
	cols, _, err := SelectIssueColumns([]string{"status", "assignee", "priority"}, DefaultListColumns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, cells := Render(cols, []Issue{{Key: "X-1"}})
	for _, got := range cells[0] {
		if got != "" {
			t.Errorf("unassigned ref rendered %q, want empty", got)
		}
	}
}
