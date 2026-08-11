package confluence

import (
	"strings"
	"testing"
)

func TestReplaceHeadingSection(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		heading string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "replaces section up to next same-level heading",
			body:    `<h2>Intro</h2><p>old</p><h2>Next</h2><p>keep</p>`,
			heading: "Intro",
			content: `<p>new</p>`,
			want:    `<h2>Intro</h2><p>new</p><h2>Next</h2><p>keep</p>`,
		},
		{
			name:    "replaces to end of body when no following heading",
			body:    `<h1>Title</h1><p>a</p><p>b</p>`,
			heading: "Title",
			content: `<p>x</p>`,
			want:    `<h1>Title</h1><p>x</p>`,
		},
		{
			name:    "stops at a higher-level heading",
			body:    `<h3>Sub</h3><p>old</p><h1>Top</h1><p>keep</p>`,
			heading: "Sub",
			content: `<p>new</p>`,
			want:    `<h3>Sub</h3><p>new</p><h1>Top</h1><p>keep</p>`,
		},
		{
			name:    "does not stop at a deeper heading (nested content stays)",
			body:    `<h1>Chapter</h1><p>lead</p><h2>Part</h2><p>body</p><h1>Two</h1><p>keep</p>`,
			heading: "Chapter",
			content: `<p>new</p>`,
			want:    `<h1>Chapter</h1><p>new</p><h1>Two</h1><p>keep</p>`,
		},
		{
			name:    "matches case-insensitively and trims whitespace",
			body:    `<h2>  Overview  </h2><p>old</p>`,
			heading: "overview",
			content: `<p>new</p>`,
			want:    `<h2>  Overview  </h2><p>new</p>`,
		},
		{
			name:    "strips inline markup when matching heading text",
			body:    `<h2><strong>Goals</strong></h2><p>old</p>`,
			heading: "Goals",
			content: `<p>new</p>`,
			want:    `<h2><strong>Goals</strong></h2><p>new</p>`,
		},
		{
			name:    "first matching heading wins",
			body:    `<h2>Dup</h2><p>one</p><h2>Dup</h2><p>two</p>`,
			heading: "Dup",
			content: `<p>new</p>`,
			want:    `<h2>Dup</h2><p>new</p><h2>Dup</h2><p>two</p>`,
		},
		{
			name:    "honors heading attributes",
			body:    `<h2 class="x">Intro</h2><p>old</p>`,
			heading: "Intro",
			content: `<p>new</p>`,
			want:    `<h2 class="x">Intro</h2><p>new</p>`,
		},
		{
			name:    "no match is an error",
			body:    `<h2>Intro</h2><p>old</p>`,
			heading: "Missing",
			content: `<p>new</p>`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReplaceHeadingSection(tt.body, tt.heading, tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestReplaceAnchorRegion(t *testing.T) {
	macro := func(name string) string {
		return `<ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">` + name + `</ac:parameter></ac:structured-macro>`
	}
	tests := []struct {
		name    string
		body    string
		anchor  string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "replaces between a pair of anchor macros",
			body:    `<p>a</p>` + macro("region") + `<p>old</p>` + macro("region") + `<p>b</p>`,
			anchor:  "region",
			content: `<p>new</p>`,
			want:    `<p>a</p>` + macro("region") + `<p>new</p>` + macro("region") + `<p>b</p>`,
		},
		{
			name:    "replaces between comment markers",
			body:    `<p>a</p><!-- region --><p>old</p><!-- /region --><p>b</p>`,
			anchor:  "region",
			content: `<p>new</p>`,
			want:    `<p>a</p><!-- region --><p>new</p><!-- /region --><p>b</p>`,
		},
		{
			name:    "comment markers tolerate whitespace",
			body:    `<!--region--><p>old</p><!--/region-->`,
			anchor:  "region",
			content: `<p>new</p>`,
			want:    `<!--region--><p>new</p><!--/region-->`,
		},
		{
			name:    "single anchor macro is not a region",
			body:    `<p>a</p>` + macro("region") + `<p>b</p>`,
			anchor:  "region",
			content: `<p>new</p>`,
			wantErr: true,
		},
		{
			name:    "missing region is an error",
			body:    `<p>nothing here</p>`,
			anchor:  "region",
			content: `<p>new</p>`,
			wantErr: true,
		},
		{
			name:    "unrelated anchor name is ignored",
			body:    macro("other") + `<p>x</p>` + macro("other"),
			anchor:  "region",
			content: `<p>new</p>`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReplaceAnchorRegion(tt.body, tt.anchor, tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestReplaceRegex(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		pattern     string
		replacement string
		want        string
		wantErr     bool
	}{
		{
			name:        "replaces all matches",
			body:        `<p>foo</p><p>foo</p>`,
			pattern:     `foo`,
			replacement: `bar`,
			want:        `<p>bar</p><p>bar</p>`,
		},
		{
			name:        "supports capture group references",
			body:        `<td>Status: DRAFT</td>`,
			pattern:     `Status: (\w+)`,
			replacement: `Status: FINAL ($1)`,
			want:        `<td>Status: FINAL (DRAFT)</td>`,
		},
		{
			name:        "no match is an error",
			body:        `<p>hello</p>`,
			pattern:     `world`,
			replacement: `x`,
			wantErr:     true,
		},
		{
			name:        "invalid pattern is an error",
			body:        `<p>hello</p>`,
			pattern:     `(unterminated`,
			replacement: `x`,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReplaceRegex(tt.body, tt.pattern, tt.replacement)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPatchApply(t *testing.T) {
	body := `<h2>Intro</h2><p>old</p>`

	t.Run("dispatches the single active selector", func(t *testing.T) {
		got, err := Patch{Heading: "Intro", Content: `<p>new</p>`}.Apply(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != `<h2>Intro</h2><p>new</p>` {
			t.Errorf("got %q", got)
		}
	})

	t.Run("no selector is an error", func(t *testing.T) {
		if _, err := (Patch{}).Apply(body); err == nil {
			t.Fatal("expected an error when no selector is set")
		}
	})

	t.Run("multiple selectors are rejected", func(t *testing.T) {
		_, err := Patch{Heading: "Intro", Regex: "old", Content: "x", Replacement: "y"}.Apply(body)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("expected a mutual-exclusion error, got %v", err)
		}
	})
}
