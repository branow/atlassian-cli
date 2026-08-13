package jira

import (
	"reflect"
	"testing"
)

func TestCoerceFields(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{
			name: "wraps a bare priority into a name object",
			in:   map[string]any{"priority": "High"},
			want: map[string]any{"priority": map[string]any{"name": "High"}},
		},
		{
			name: "wraps assignee and reporter by accountId",
			in:   map[string]any{"assignee": "5b10a", "reporter": "5b10b"},
			want: map[string]any{
				"assignee": map[string]any{"accountId": "5b10a"},
				"reporter": map[string]any{"accountId": "5b10b"},
			},
		},
		{
			name: "wraps components into an array of name objects",
			in:   map[string]any{"components": "API"},
			want: map[string]any{"components": []any{map[string]any{"name": "API"}}},
		},
		{
			name: "wraps a bare label into a string array",
			in:   map[string]any{"labels": "urgent"},
			want: map[string]any{"labels": []any{"urgent"}},
		},
		{
			name: "leaves an already-object value untouched",
			in:   map[string]any{"priority": map[string]any{"id": "3"}},
			want: map[string]any{"priority": map[string]any{"id": "3"}},
		},
		{
			name: "leaves an already-array value untouched",
			in:   map[string]any{"labels": []any{"a", "b"}},
			want: map[string]any{"labels": []any{"a", "b"}},
		},
		{
			name: "leaves unknown and scalar fields untouched",
			in:   map[string]any{"summary": "hello", "customfield_1": "x"},
			want: map[string]any{"summary": "hello", "customfield_1": "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			CoerceFields(tc.in)
			if !reflect.DeepEqual(tc.in, tc.want) {
				t.Errorf("CoerceFields = %#v, want %#v", tc.in, tc.want)
			}
		})
	}
}
