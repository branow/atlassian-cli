package jira

import (
	"fmt"
	"regexp"
	"strings"
)

// Mention is a resolved user reference substituted into a markdown body's
// @[query] tokens: the accountId the ADF mention node requires plus the
// display name rendered after the "@". The caller resolves each query to one
// of these (via user search) before building the document.
type Mention struct {
	AccountID string
	Display   string
}

// mentionToken matches @[query], where query is any run up to the closing
// bracket — a display name or email the caller resolves to an account.
var mentionToken = regexp.MustCompile(`@\[([^\]]+)\]`)

// MentionQueries returns the distinct @[...] queries in text, in order of
// first appearance, so the caller resolves each to an account exactly once
// before calling MarkdownDocument.
func MentionQueries(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range mentionToken.FindAllStringSubmatch(text, -1) {
		q := strings.TrimSpace(m[1])
		if q == "" || seen[q] {
			continue
		}
		seen[q] = true
		out = append(out, q)
	}
	return out
}

// spanRule is one inline-markup construct: a pattern with capture groups and
// a builder turning the match into ADF inline node(s). Holding the constructs
// as data keeps the scanner a lookup loop rather than a branch ladder, and
// makes the supported subset a single readable list.
type spanRule struct {
	re    *regexp.Regexp
	build func(sub []string, mentions map[string]Mention) []any
}

// inlineRules is the supported inline subset, in precedence order: when two
// constructs start at the same index the earlier rule wins, so code (whose
// content is literal) and strong (`**`) take priority over emphasis (`*`).
var inlineRules = []spanRule{
	{ // `code`
		re: regexp.MustCompile("`([^`]+)`"),
		build: func(sub []string, _ map[string]Mention) []any {
			return []any{textNode(sub[1], mark("code"))}
		},
	},
	{ // @[name] mention
		re: mentionToken,
		build: func(sub []string, mentions map[string]Mention) []any {
			q := strings.TrimSpace(sub[1])
			if m, ok := mentions[q]; ok && m.AccountID != "" {
				return []any{map[string]any{
					"type":  "mention",
					"attrs": map[string]any{"id": m.AccountID, "text": "@" + m.Display},
				}}
			}
			// Unresolved: keep the literal token so nothing silently vanishes.
			return []any{textNode(sub[0])}
		},
	},
	{ // [text](url) link
		re: regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`),
		build: func(sub []string, _ map[string]Mention) []any {
			return []any{textNode(sub[1], map[string]any{
				"type":  "link",
				"attrs": map[string]any{"href": sub[2]},
			})}
		},
	},
	{ // **strong**
		re: regexp.MustCompile(`\*\*([^*]+)\*\*`),
		build: func(sub []string, _ map[string]Mention) []any {
			return []any{textNode(sub[1], mark("strong"))}
		},
	},
	{ // *emphasis*
		re: regexp.MustCompile(`\*([^*]+)\*`),
		build: func(sub []string, _ map[string]Mention) []any {
			return []any{textNode(sub[1], mark("em"))}
		},
	},
}

// MarkdownDocument converts a lightweight-markdown body into an ADF document
// for a comment or description. It mirrors Document's block model —
// blank-line-separated paragraphs, single newlines as hardBreaks — and adds
// inline marks (bold/italic/code), [text](url) links, and @[query] mentions
// resolved through the supplied map. A query absent from the map (or resolved
// to an empty account) is left as its literal token rather than dropped. With
// no markup and an empty map the output is identical to Document(text).
func MarkdownDocument(text string, mentions map[string]Mention) map[string]any {
	paragraphs := strings.Split(text, "\n\n")
	content := make([]any, 0, len(paragraphs))
	for _, para := range paragraphs {
		content = append(content, markdownParagraph(para, mentions))
	}
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}

// markdownParagraph builds one ADF paragraph, splitting embedded single
// newlines into hardBreak-joined lines and each line into inline nodes.
func markdownParagraph(text string, mentions map[string]Mention) map[string]any {
	lines := strings.Split(text, "\n")
	inline := make([]any, 0, len(lines))
	for i, line := range lines {
		if i > 0 {
			inline = append(inline, map[string]any{"type": "hardBreak"})
		}
		if line != "" {
			inline = append(inline, inlineNodes(line, mentions)...)
		}
	}
	return map[string]any{"type": "paragraph", "content": inline}
}

// inlineNodes scans one line left to right, at each step choosing the
// earliest-starting inline construct among inlineRules, emitting the text
// before it as a plain node and the construct's node(s) in place. Text with
// no remaining construct becomes a single plain node.
func inlineNodes(line string, mentions map[string]Mention) []any {
	var nodes []any
	for len(line) > 0 {
		bestStart := -1
		bestEnd := 0
		var best spanRule
		var bestSub []string
		for _, r := range inlineRules {
			loc := r.re.FindStringSubmatchIndex(line)
			if loc == nil || (bestStart != -1 && loc[0] >= bestStart) {
				continue
			}
			bestStart, bestEnd, best = loc[0], loc[1], r
			bestSub = submatchStrings(line, loc)
		}
		if bestStart == -1 {
			nodes = append(nodes, textNode(line))
			break
		}
		if bestStart > 0 {
			nodes = append(nodes, textNode(line[:bestStart]))
		}
		nodes = append(nodes, best.build(bestSub, mentions)...)
		line = line[bestEnd:]
	}
	return nodes
}

// submatchStrings turns FindStringSubmatchIndex output into the string groups
// a builder consumes; an unset group (index -1) becomes "".
func submatchStrings(s string, loc []int) []string {
	out := make([]string, len(loc)/2)
	for i := range out {
		if loc[2*i] >= 0 {
			out[i] = s[loc[2*i]:loc[2*i+1]]
		}
	}
	return out
}

// textNode builds an ADF text node with the given marks (if any).
func textNode(text string, marks ...map[string]any) map[string]any {
	n := map[string]any{"type": "text", "text": text}
	if len(marks) > 0 {
		ms := make([]any, len(marks))
		for i, m := range marks {
			ms[i] = m
		}
		n["marks"] = ms
	}
	return n
}

// mark is a bare ADF mark (strong, em, code) carrying no attributes.
func mark(kind string) map[string]any {
	return map[string]any{"type": kind}
}

// PickUser resolves a mention query against user-search results: a single
// result is taken as-is; multiple results require an exact case-insensitive
// match on display name or email, otherwise the query is reported as
// ambiguous with the candidates so the caller can disambiguate.
func PickUser(users []User, query string) (User, error) {
	switch len(users) {
	case 0:
		return User{}, fmt.Errorf("no user matches mention %q", query)
	case 1:
		return users[0], nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var exact []User
	for _, u := range users {
		if strings.ToLower(u.DisplayName) == q || strings.ToLower(u.EmailAddress) == q {
			exact = append(exact, u)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = u.DisplayName
	}
	return User{}, fmt.Errorf("mention %q is ambiguous (%d matches: %s); use a full name or email", query, len(users), strings.Join(names, ", "))
}
