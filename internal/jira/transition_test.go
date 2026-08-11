package jira

import (
	"strings"
	"testing"
)

func TestResolveTransition(t *testing.T) {
	available := []Transition{
		{ID: "11", Name: "To Do"},
		{ID: "21", Name: "In Progress"},
		{ID: "31", Name: "Done"},
	}
	cases := []struct {
		name     string
		list     []Transition
		selector string
		wantID   string
		wantErr  string
	}{
		{"exact id", available, "21", "21", ""},
		{"name exact case", available, "Done", "31", ""},
		{"name case-insensitive", available, "in progress", "21", ""},
		{"id beats name lookup", available, "11", "11", ""},
		{"no match lists options", available, "Closed", "", "no transition matches"},
		{"empty list", nil, "Done", "", "no transition matches"},
		{
			"ambiguous name",
			[]Transition{{ID: "1", Name: "Review"}, {ID: "2", Name: "review"}},
			"Review",
			"",
			"ambiguous",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveTransition(tc.list, tc.selector)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ID != tc.wantID {
				t.Errorf("id = %q, want %q", got.ID, tc.wantID)
			}
		})
	}
}
