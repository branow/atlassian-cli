package jira

import "strings"

// blockNodes are ADF node types that render as their own block (line) of
// text. Extraction emits a newline after each so paragraphs, headings, and
// list items do not run together. Held as data so the walker is a lookup,
// not an if/else ladder.
var blockNodes = map[string]bool{
	"paragraph":  true,
	"heading":    true,
	"blockquote": true,
	"listItem":   true,
	"codeBlock":  true,
	"rule":       true,
}

// ExtractText renders an Atlassian Document Format (ADF) body as best-effort
// plain text. Jira stores descriptions and comments as an ADF node tree
// ({type:"doc", content:[...]}) rather than a string; commands that display
// such a body call this to recover something readable. Text nodes are
// concatenated, hardBreak nodes and block-level containers introduce
// newlines, and all marks/formatting are dropped. The argument is the value
// decoded from JSON (typically map[string]any); a nil or non-ADF value
// yields "".
func ExtractText(node any) string {
	var b strings.Builder
	walkADF(node, &b)
	return strings.TrimSpace(collapseBlankLines(b.String()))
}

func walkADF(node any, b *strings.Builder) {
	switch n := node.(type) {
	case []any:
		for _, child := range n {
			walkADF(child, b)
		}
	case map[string]any:
		nodeType, _ := n["type"].(string)
		if nodeType == "hardBreak" {
			b.WriteString("\n")
		}
		if nodeType == "mention" {
			if attrs, ok := n["attrs"].(map[string]any); ok {
				if txt, ok := attrs["text"].(string); ok {
					b.WriteString(txt)
				}
			}
		}
		if text, ok := n["text"].(string); ok {
			b.WriteString(text)
		}
		if content, ok := n["content"].([]any); ok {
			for _, child := range content {
				walkADF(child, b)
			}
		}
		if blockNodes[nodeType] {
			b.WriteString("\n\n")
		}
	}
}

// collapseBlankLines squeezes runs of 3+ newlines to a paragraph break so
// nested block nodes (a listItem holding a paragraph) do not stack blank
// lines.
func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// Document wraps plain text in a minimal ADF document suitable for the
// create/edit description and comment bodies, which the API requires as ADF
// rather than a string. Blank-line-separated blocks become separate
// paragraphs; a single newline within a block becomes a hardBreak so line
// structure survives the round trip. Empty text still yields a valid empty
// paragraph so the caller always sends well-formed ADF.
func Document(text string) map[string]any {
	paragraphs := strings.Split(text, "\n\n")
	content := make([]any, 0, len(paragraphs))
	for _, para := range paragraphs {
		content = append(content, paragraph(para))
	}
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}

// paragraph builds one ADF paragraph node, splitting embedded single
// newlines into text runs joined by hardBreak nodes.
func paragraph(text string) map[string]any {
	lines := strings.Split(text, "\n")
	inline := make([]any, 0, len(lines))
	for i, line := range lines {
		if i > 0 {
			inline = append(inline, map[string]any{"type": "hardBreak"})
		}
		if line != "" {
			inline = append(inline, map[string]any{"type": "text", "text": line})
		}
	}
	return map[string]any{"type": "paragraph", "content": inline}
}
