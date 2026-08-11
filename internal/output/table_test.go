package output_test

import (
	"bytes"
	"testing"

	"github.com/branow/atlassian-cli/internal/output"
)

func TestWriteTableAlignsColumns(t *testing.T) {
	var buf bytes.Buffer
	err := output.WriteTable(&buf,
		[]string{"NAME", "STATUS"},
		[][]string{
			{"alice", "active"},
			{"bob", "inactive"},
		},
	)
	if err != nil {
		t.Fatalf("WriteTable: %v", err)
	}

	want := "NAME   STATUS\n" +
		"alice  active\n" +
		"bob    inactive\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestWriteTableNoRows(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteTable(&buf, []string{"NAME"}, nil); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}
	if buf.String() != "NAME\n" {
		t.Errorf("got %q, want %q", buf.String(), "NAME\n")
	}
}
