package jira

import (
	"encoding/json"
	"testing"
)

// adf parses an ADF JSON literal into the untyped shape ExtractText receives
// from a decoded API response.
func adf(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad ADF literal: %v", err)
	}
	return v
}

func TestExtractText(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "nil is empty",
			doc:  `null`,
			want: "",
		},
		{
			name: "single paragraph",
			doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello world"}]}]}`,
			want: "Hello world",
		},
		{
			name: "two paragraphs separated by blank line",
			doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]},{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}`,
			want: "one\n\ntwo",
		},
		{
			name: "hard break inside a paragraph",
			doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}]}]}`,
			want: "a\nb",
		},
		{
			name: "heading and list items",
			doc:  `{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"Title"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"first"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"second"}]}]}]}]}`,
			want: "Title\n\nfirst\n\nsecond",
		},
		{
			name: "marks are dropped, text kept",
			doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"strong"}]},{"type":"text","text":" plain"}]}]}`,
			want: "bold plain",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractText(adf(t, tc.doc))
			if got != tc.want {
				t.Errorf("ExtractText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDocumentRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"single line", "Just text", "Just text"},
		{"paragraphs", "one\n\ntwo", "one\n\ntwo"},
		{"line breaks within a paragraph", "a\nb", "a\nb"},
		{"empty stays valid", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Document(tc.text)
			if doc["type"] != "doc" {
				t.Fatalf("Document type = %v, want doc", doc["type"])
			}
			// A round trip through ExtractText recovers the text, proving the
			// generated ADF is well-formed and structurally faithful.
			if got := ExtractText(doc); got != tc.want {
				t.Errorf("round trip = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDocumentIsValidJSON(t *testing.T) {
	if _, err := json.Marshal(Document("hi\n\nthere")); err != nil {
		t.Fatalf("Document is not JSON-encodable: %v", err)
	}
}
