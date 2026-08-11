package output_test

import (
	"bytes"
	"testing"

	"github.com/branow/atlassian-cli/internal/output"
)

func TestWriteJSONStructPreservesFieldOrder(t *testing.T) {
	type record struct {
		Zebra string `json:"zebra"`
		Alpha string `json:"alpha"`
	}
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, record{Zebra: "z", Alpha: "a"}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	want := "{\n  \"zebra\": \"z\",\n  \"alpha\": \"a\"\n}\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestWriteJSONMapSortsKeys(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, map[string]string{"zebra": "z", "alpha": "a"}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	want := "{\n  \"alpha\": \"a\",\n  \"zebra\": \"z\"\n}\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}
