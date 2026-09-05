// Package rag — text_test.go tests the TipTap/HTML/markdown text extraction
// utility. These are pure-function tests with no database dependency.
package rag

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractPlainText_PlainText(t *testing.T) {
	input := "This is plain text with no markup."
	got := ExtractPlainText(input)

	if got != input {
		t.Errorf("ExtractPlainText(plain) = %q, want %q", got, input)
	}
}

func TestExtractPlainText_Empty(t *testing.T) {
	got := ExtractPlainText("")
	if got != "" {
		t.Errorf("ExtractPlainText(\"\") = %q, want \"\"", got)
	}
}

func TestExtractPlainText_TipTapJSON(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []map[string]any{
			{
				"type": "paragraph",
				"content": []map[string]any{
					{"type": "text", "text": "Hello "},
					{"type": "text", "text": "world"},
				},
			},
			{
				"type": "paragraph",
				"content": []map[string]any{
					{"type": "text", "text": "Second paragraph"},
				},
			},
		},
	}
	body, _ := json.Marshal(doc)
	got := ExtractPlainText(string(body))
	// Should contain both paragraphs' text.
	if !strings.Contains(got, "Hello world") {
		t.Errorf("ExtractPlainText(tiptap) missing 'Hello world': %q", got)
	}

	if !strings.Contains(got, "Second paragraph") {
		t.Errorf("ExtractPlainText(tiptap) missing 'Second paragraph': %q", got)
	}
}

func TestExtractPlainText_HTML(t *testing.T) {
	input := "<p>Hello <strong>world</strong></p><p>Second</p>"

	got := ExtractPlainText(input)
	if !strings.Contains(got, "Hello") {
		t.Errorf("ExtractPlainText(html) missing 'Hello': %q", got)
	}

	if !strings.Contains(got, "world") {
		t.Errorf("ExtractPlainText(html) missing 'world': %q", got)
	}

	if !strings.Contains(got, "Second") {
		t.Errorf("ExtractPlainText(html) missing 'Second': %q", got)
	}
}

func TestExtractPlainText_Markdown(t *testing.T) {
	input := "# Heading\n\nThis is **bold** text with a [link](https://example.com)."

	got := ExtractPlainText(input)
	if !strings.Contains(got, "Heading") {
		t.Errorf("ExtractPlainText(markdown) missing 'Heading': %q", got)
	}

	if !strings.Contains(got, "bold") {
		t.Errorf("ExtractPlainText(markdown) missing 'bold': %q", got)
	}

	if !strings.Contains(got, "link") {
		t.Errorf("ExtractPlainText(markdown) missing 'link': %q", got)
	}

	if strings.Contains(got, "**") {
		t.Errorf("ExtractPlainText(markdown) should strip **: %q", got)
	}

	if strings.Contains(got, "https://") {
		t.Errorf("ExtractPlainText(markdown) should strip URLs from links: %q", got)
	}
}

func TestStripHTMLTags_Entities(t *testing.T) {
	input := "A &amp; B &lt; C &gt; D &quot;E&quot; &#39;F&#39;"

	got := stripHTMLTags(input)
	if got != `A & B < C > D "E" 'F'` {
		t.Errorf("stripHTMLTags(entities) = %q, want %q", got, `A & B < C > D "E" 'F'`)
	}
}

func TestCollapseWhitespace(t *testing.T) {
	input := "  hello   world  \n\n  foo  "

	got := collapseWhitespace(input)
	if got != "hello world foo" {
		t.Errorf("collapseWhitespace = %q, want %q", got, "hello world foo")
	}
}
