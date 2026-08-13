// Package confluence holds the value logic behind the curated `atl
// confluence` commands: the version-aware page patch engine, the typed
// models decoded from Confluence REST responses, and a self-contained
// multipart attachment uploader. Everything here is kept free of cobra and
// of the network so it can be unit-tested as pure functions; the command
// layer in cmd/confluence_*.go is a thin wrapper over it.
//
// The patch engine exists because Confluence only accepts a full-body PUT
// when updating a page: there is no server-side partial-update endpoint. To
// offer a partial `page patch`, the CLI reads the current storage body,
// rewrites one region of it locally, and PUTs the whole thing back with the
// next version number. Confluence "storage format" is XHTML, so the region
// selectors operate on the raw markup as strings — deliberately, since a
// full XML round-trip would reflow and normalize markup the user never
// touched and defeat the point of a surgical patch.
package confluence

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// Patch describes a single region rewrite of a storage-format body. Exactly
// one selector (Heading, Anchor, or Regex) must be set; the command layer
// validates that before calling Apply, but Apply re-checks so the engine is
// safe to use on its own. Content is the replacement markup for the Heading
// and Anchor selectors; Replacement is the substitution string for Regex
// (and may reference capture groups as $1, $2, ...).
type Patch struct {
	Heading     string
	Anchor      string
	Regex       string
	Content     string
	Replacement string
}

// selector pairs a Patch field's "is it set" test with the rewrite it
// performs, so Apply is a small data-driven dispatch rather than an
// if/else ladder — and adding a selector is one slice entry.
type selector struct {
	name  string
	set   bool
	apply func(body string) (string, error)
}

// Apply validates that exactly one selector is set and runs it against body,
// returning the rewritten full body. A zero or ambiguous selector count is a
// programming/usage error; a selector that matches nothing in the body is a
// runtime error the caller surfaces to the user.
func (p Patch) Apply(body string) (string, error) {
	selectors := []selector{
		{"heading", p.Heading != "", func(b string) (string, error) {
			return ReplaceHeadingSection(b, p.Heading, p.Content)
		}},
		{"anchor", p.Anchor != "", func(b string) (string, error) {
			return ReplaceAnchorRegion(b, p.Anchor, p.Content)
		}},
		{"regex", p.Regex != "", func(b string) (string, error) {
			return ReplaceRegex(b, p.Regex, p.Replacement)
		}},
	}

	var active *selector
	var names []string
	for i := range selectors {
		if selectors[i].set {
			names = append(names, selectors[i].name)
			active = &selectors[i]
		}
	}
	switch len(names) {
	case 0:
		return "", fmt.Errorf("no patch selector set (want one of --heading, --anchor, --regex)")
	case 1:
		return active.apply(body)
	default:
		return "", fmt.Errorf("multiple patch selectors set (%s); they are mutually exclusive", strings.Join(names, ", "))
	}
}

// headingRe matches one <h1>..<h6> element and captures its level and inner
// markup. Go's regexp has no backreferences, so the closing tag is matched
// as any-level </h[1-6]>; headings are never nested, so the non-greedy body
// still stops at this element's own close.
var headingRe = regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>(.*?)</h[1-6]>`)

// tagRe strips XHTML tags so a heading's visible text can be compared to the
// requested heading name.
var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// opaqueRe matches regions whose contents are literal text, not markup:
// CDATA sections and the body of a code macro (ac:plain-text-body). An
// <h2>-looking string inside one of these is content, not a heading, so
// heading selectors must not see it.
var opaqueRe = regexp.MustCompile(`(?is)<!\[CDATA\[.*?\]\]>|<ac:plain-text-body\b[^>]*>.*?</ac:plain-text-body>`)

// maskOpaqueRegions returns a copy of body with every CDATA section and
// code-macro body overwritten by spaces of the same byte length, so offsets
// still index into the original body while heading selectors run over markup
// with the literal-text regions blanked out.
func maskOpaqueRegions(body string) string {
	return opaqueRe.ReplaceAllStringFunc(body, func(m string) string {
		return strings.Repeat(" ", len(m))
	})
}

// headingText reduces a heading element's inner markup to the visible text a
// reader sees: tags stripped and HTML entities decoded (so a heading stored as
// "AT&amp;T" matches the name "AT&T"), trimmed and lowercased for comparison.
func headingText(markup string) string {
	return strings.TrimSpace(strings.ToLower(html.UnescapeString(tagRe.ReplaceAllString(markup, ""))))
}

// ReplaceHeadingSection replaces the content beneath the first heading whose
// visible text matches name (trimmed, case-insensitive) with content. The
// section runs from just after that heading's closing tag up to — but not
// including — the next heading of the same or higher level (a smaller or
// equal <hN> number), or the end of the body when there is none. The matched
// heading element itself is preserved; only what it introduces is replaced,
// which is what "the section under this heading" means to a reader.
func ReplaceHeadingSection(body, name, content string) (string, error) {
	// Match headings over a mask where CDATA/code-macro bodies are blanked, so
	// a heading-like string inside a code block is not mistaken for a real
	// heading. The mask preserves byte offsets, so indices index into body.
	matches := headingRe.FindAllStringSubmatchIndex(maskOpaqueRegions(body), -1)

	type h struct {
		level              int
		elemStart, elemEnd int
	}
	var headings []h
	target := strings.TrimSpace(strings.ToLower(html.UnescapeString(name)))
	matchIdx := -1
	for _, m := range matches {
		level := int(body[m[2]] - '0') // single digit 1..6 from group 1
		text := headingText(body[m[4]:m[5]])
		headings = append(headings, h{level: level, elemStart: m[0], elemEnd: m[1]})
		if matchIdx == -1 && text == target {
			matchIdx = len(headings) - 1
		}
	}
	if matchIdx == -1 {
		return "", fmt.Errorf("no heading matched %q", name)
	}

	matched := headings[matchIdx]
	sectionEnd := len(body)
	for _, next := range headings[matchIdx+1:] {
		if next.level <= matched.level {
			sectionEnd = next.elemStart
			break
		}
	}
	return body[:matched.elemEnd] + content + body[sectionEnd:], nil
}

// ReplaceAnchorRegion replaces the region delimited by a named marker with
// content. Two marker styles are recognized, tried in order: a pair of
// Confluence anchor macros carrying that name, and a pair of HTML comments
// `<!-- name -->` ... `<!-- /name -->`. The region is the text between the
// opening and closing markers; the markers themselves are preserved so the
// region stays addressable for the next patch. It errors when the named
// region is not present as a complete pair.
func ReplaceAnchorRegion(body, name, content string) (string, error) {
	if start, end, ok := anchorMacroRegion(body, name); ok {
		return body[:start] + content + body[end:], nil
	}
	if start, end, ok := commentMarkerRegion(body, name); ok {
		return body[:start] + content + body[end:], nil
	}
	return "", fmt.Errorf("no anchor region named %q found (need a matching pair of anchor macros or <!-- %s --> ... <!-- /%s --> markers)", name, name, name)
}

// anchorMacroRe matches a whole Confluence anchor structured-macro.
var anchorMacroRe = regexp.MustCompile(`(?is)<ac:structured-macro\b[^>]*\bac:name="anchor".*?</ac:structured-macro>`)

// anchorMacroRegion returns the inner bounds (end of the first matching
// anchor, start of the second) of the region between the two anchor macros
// whose parameter value equals name.
func anchorMacroRegion(body, name string) (start, end int, ok bool) {
	paramRe := regexp.MustCompile(`(?is)<ac:parameter\b[^>]*>\s*` + regexp.QuoteMeta(name) + `\s*</ac:parameter>`)
	var ends []int
	var starts []int
	for _, m := range anchorMacroRe.FindAllStringIndex(body, -1) {
		if paramRe.MatchString(body[m[0]:m[1]]) {
			ends = append(ends, m[1])
			starts = append(starts, m[0])
		}
	}
	if len(ends) < 2 {
		return 0, 0, false
	}
	return ends[0], starts[1], true
}

// commentMarkerRegion returns the bounds of the region between
// `<!-- name -->` and `<!-- /name -->` (whitespace-tolerant), inclusive of
// neither marker.
func commentMarkerRegion(body, name string) (start, end int, ok bool) {
	openRe := regexp.MustCompile(`(?is)<!--\s*` + regexp.QuoteMeta(name) + `\s*-->`)
	closeRe := regexp.MustCompile(`(?is)<!--\s*/\s*` + regexp.QuoteMeta(name) + `\s*-->`)
	open := openRe.FindStringIndex(body)
	if open == nil {
		return 0, 0, false
	}
	close := closeRe.FindStringIndex(body[open[1]:])
	if close == nil {
		return 0, 0, false
	}
	return open[1], open[1] + close[0], true
}

// ReplaceRegex substitutes every match of pattern in body with replacement
// (which may reference capture groups as $1, ${name}, ...). An invalid
// pattern and a pattern that matches nothing are both errors, so a patch that
// would silently be a no-op is reported instead of PUTting an unchanged body.
func ReplaceRegex(body, pattern, replacement string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	if !re.MatchString(body) {
		return "", fmt.Errorf("regex %q matched nothing in the page body", pattern)
	}
	return re.ReplaceAllString(body, replacement), nil
}
