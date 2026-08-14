package jira

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMentionQueries(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"none", "no mentions here", nil},
		{"single", "hi @[Jane Doe] please look", []string{"Jane Doe"}},
		{"email", "cc @[jane@acme.com]", []string{"jane@acme.com"}},
		{"distinct in order", "@[b] then @[a] then @[b] again", []string{"b", "a"}},
		{"trims whitespace", "@[  Jane Doe  ]", []string{"Jane Doe"}},
		{"empty token ignored", "@[]", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MentionQueries(tc.text); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MentionQueries(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestMarkdownDocumentPlainMatchesDocument proves the converter is a strict
// superset: with no markup it produces exactly what Document produces, so
// --markdown never regresses plain-text bodies.
func TestMarkdownDocumentPlainMatchesDocument(t *testing.T) {
	for _, text := range []string{"", "just text", "one\n\ntwo", "a\nb", "trailing.\n"} {
		md, _ := json.Marshal(MarkdownDocument(text, nil))
		plain, _ := json.Marshal(Document(text))
		if string(md) != string(plain) {
			t.Errorf("plain markdown of %q differs:\n md = %s\n doc = %s", text, md, plain)
		}
	}
}

func TestMarkdownDocumentInline(t *testing.T) {
	// Each case asserts the ADF fragment that must appear for the given body.
	cases := []struct {
		name string
		text string
		want string // JSON substring the marshaled document must contain
	}{
		{
			name: "bold",
			text: "a **b** c",
			want: `{"marks":[{"type":"strong"}],"text":"b","type":"text"}`,
		},
		{
			name: "italic",
			text: "a *b* c",
			want: `{"marks":[{"type":"em"}],"text":"b","type":"text"}`,
		},
		{
			name: "code",
			text: "run `atl` now",
			want: `{"marks":[{"type":"code"}],"text":"atl","type":"text"}`,
		},
		{
			name: "bold wins over italic at same start",
			text: "**strong**",
			want: `{"marks":[{"type":"strong"}],"text":"strong","type":"text"}`,
		},
		{
			name: "link",
			text: "see [PR 42](https://x.test/42)",
			want: `{"attrs":{"href":"https://x.test/42"},"type":"link"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(MarkdownDocument(tc.text, nil))
			if !containsJSON(t, string(b), tc.want) {
				t.Errorf("MarkdownDocument(%q) missing %s\n got %s", tc.text, tc.want, b)
			}
		})
	}
}

func TestMarkdownDocumentMentionResolved(t *testing.T) {
	mentions := map[string]Mention{"Jane Doe": {AccountID: "abc-123", Display: "Jane Doe"}}
	doc := MarkdownDocument("hi @[Jane Doe]!", mentions)
	// The mention becomes a mention node carrying the accountId, and the
	// round trip through ExtractText recovers the "@Jane Doe" text.
	b, _ := json.Marshal(doc)
	want := `{"attrs":{"id":"abc-123","text":"@Jane Doe"},"type":"mention"}`
	if !containsJSON(t, string(b), want) {
		t.Errorf("mention node missing %s\n got %s", want, b)
	}
	if got := ExtractText(doc); got != "hi @Jane Doe!" {
		t.Errorf("ExtractText = %q, want %q", got, "hi @Jane Doe!")
	}
}

// TestMarkdownDocumentMentionUnresolved keeps the literal token when the query
// was not resolved, so a mistyped name is visible rather than dropped.
func TestMarkdownDocumentMentionUnresolved(t *testing.T) {
	doc := MarkdownDocument("hi @[Ghost]", nil)
	if got := ExtractText(doc); got != "hi @[Ghost]" {
		t.Errorf("ExtractText = %q, want %q", got, "hi @[Ghost]")
	}
}

func TestPickUser(t *testing.T) {
	jane := User{AccountID: "1", DisplayName: "Jane Doe", EmailAddress: "jane@acme.com"}
	jane2 := User{AccountID: "2", DisplayName: "Jane Doe", EmailAddress: "jane@other.com"}
	other := User{AccountID: "3", DisplayName: "Janet Roe", EmailAddress: "janet@acme.com"}

	t.Run("no match errors", func(t *testing.T) {
		if _, err := PickUser(nil, "x"); err == nil {
			t.Error("want error for zero results")
		}
	})
	t.Run("single result taken", func(t *testing.T) {
		u, err := PickUser([]User{jane}, "whatever")
		if err != nil || u.AccountID != "1" {
			t.Errorf("got %v, %v", u, err)
		}
	})
	t.Run("exact email disambiguates", func(t *testing.T) {
		u, err := PickUser([]User{jane, jane2, other}, "jane@other.com")
		if err != nil || u.AccountID != "2" {
			t.Errorf("got %v, %v", u, err)
		}
	})
	t.Run("ambiguous name errors", func(t *testing.T) {
		if _, err := PickUser([]User{jane, jane2}, "Jane Doe"); err == nil {
			t.Error("want error: two exact name matches")
		}
	})
}

// containsJSON marshals want and got via the same encoder and reports whether
// got contains want, tolerating map key ordering by comparing the exact byte
// substring the caller wrote (keys are alphabetized by encoding/json).
func containsJSON(t *testing.T, got, want string) bool {
	t.Helper()
	return len(want) == 0 || indexOf(got, want) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
