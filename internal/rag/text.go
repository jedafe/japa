// Package rag — text.go provides the text extraction utility that strips
// TipTap JSON block structure to plain text before chunking and embedding.
// ADR-069: "Chunked embedding of rich TipTap JSON requires stripping block
// structure to plain text before embedding — a text-extraction utility is
// needed." This is the utility, not inline in the task.
//
// Ref: japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package rag

import (
	"encoding/json"
	"strings"
	"unicode"
)

// ExtractPlainText extracts plain text from a content body that may be:
//   - TipTap JSON (starts with `{`)
//   - HTML (starts with `<`)
//   - Plain text / markdown (everything else)
//
// The extraction strips all block structure, attributes, and markup,
// producing a single text blob suitable for chunking and embedding.
func ExtractPlainText(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	// TipTap JSON — parse the ProseMirror doc tree and walk text nodes.
	if strings.HasPrefix(body, "{") {
		if text, ok := extractFromTipTap(body); ok {
			return text
		}
		// If JSON parsing fails, fall through to plain text.
	}

	// HTML — strip tags.
	if strings.HasPrefix(body, "<") {
		return stripHTMLTags(body)
	}

	// Markdown / plain text — strip markdown syntax lightly.
	return stripMarkdown(body)
}

// tiptapDoc is the minimal ProseMirror document structure needed for text
// extraction. We only care about the recursive content tree and text nodes.
type tiptapDoc struct {
	Type    string      `json:"type"`
	Content []tiptapDoc `json:"content,omitempty"`
	Text    string      `json:"text,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// extractFromTipTap walks a TipTap/ProseMirror JSON document tree and
// concatenates all text node values with appropriate spacing between blocks.
func extractFromTipTap(body string) (string, bool) {
	var doc tiptapDoc
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return "", false
	}

	var sb strings.Builder
	walkTipTap(&doc, &sb)
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return "", false
	}
	return text, true
}

// walkTipTap recursively traverses a ProseMirror node tree, appending text
// from text nodes and adding newlines between block-level children.
func walkTipTap(node *tiptapDoc, sb *strings.Builder) {
	if node.Text != "" {
		sb.WriteString(node.Text)
		return
	}

	for i, child := range node.Content {
		// Add a paragraph break between block-level children (not between
		// inline text nodes within the same paragraph).
		if i > 0 && isBlockNode(node.Type) && isBlockNode(child.Type) {
			sb.WriteString("\n\n")
		}
		walkTipTap(&child, sb)
	}
}

// isBlockNode returns true for ProseMirror node types that represent
// block-level content (paragraphs, headings, list items, etc.).
func isBlockNode(nodeType string) bool {
	switch nodeType {
	case "doc", "paragraph", "heading", "blockquote", "bulletList",
		"orderedList", "listItem", "codeBlock", "preformatted",
		"table", "tableRow", "tableCell", "tableHeader",
		"callout", "details", "image", "videoEmbed", "divider":
		return true
	default:
		return false
	}
}

// stripHTMLTags removes HTML tags and decodes common entities, producing
// plain text. This is a simple stripper — not a full HTML parser.
func stripHTMLTags(html string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			sb.WriteRune(' ')
		case !inTag:
			sb.WriteRune(r)
		}
	}

	text := sb.String()
	// Decode common HTML entities.
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	return strings.TrimSpace(collapseWhitespace(text))
}

// stripMarkdown removes common markdown syntax to produce plain text.
func stripMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var sb strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines but preserve paragraph breaks.
		if trimmed == "" {
			sb.WriteString("\n\n")
			continue
		}

		// Strip heading markers (# ## ###).
		trimmed = strings.TrimLeft(trimmed, "#")

		// Strip list markers (- * + 1.).
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
			trimmed = trimmed[2:]
		} else if len(trimmed) > 2 && unicode.IsDigit(rune(trimmed[0])) && trimmed[1] == '.' && trimmed[2] == ' ' {
			trimmed = trimmed[3:]
		}

		// Strip blockquote markers.
		trimmed = strings.TrimPrefix(trimmed, "> ")

		// Strip emphasis markers.
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "__", "")
		trimmed = strings.ReplaceAll(trimmed, "`", "")
		trimmed = strings.ReplaceAll(trimmed, "*", "")
		trimmed = strings.ReplaceAll(trimmed, "_", " ")

		// Strip link syntax: [text](url) → text.
		trimmed = stripLinks(trimmed)

		sb.WriteString(strings.TrimSpace(trimmed))
		sb.WriteString(" ")
	}

	return strings.TrimSpace(collapseWhitespace(sb.String()))
}

// stripLinks replaces [text](url) with just text.
func stripLinks(s string) string {
	for {
		idx := strings.Index(s, "](")
		if idx == -1 {
			break
		}
		// Find the opening bracket.
		openIdx := strings.LastIndex(s[:idx], "[")
		if openIdx == -1 {
			break
		}
		// Find the closing paren.
		closeIdx := strings.Index(s[idx+2:], ")")
		if closeIdx == -1 {
			break
		}
		closeIdx += idx + 2

		text := s[openIdx+1 : idx]
		s = s[:openIdx] + text + s[closeIdx+1:]
	}
	return s
}

// collapseWhitespace replaces runs of whitespace with a single space.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
