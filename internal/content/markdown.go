// Package content — markdown rendering for content bodies.
//
// Body text is stored as plain text / markdown. body_html is the rendered
// HTML form used by the frontend detail pages. This file provides the
// rendering pipeline; SanitizeHTML is then applied to the output so the
// stored HTML matches the whitelist in sanitize.go.
package content

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// bodyMarkdown is the shared goldmark renderer for content bodies.
// It enables GFM tables, linkify (auto-link bare URLs), and strikethrough,
// but the final HTML is still run through SanitizeHTML so only whitelist tags
// survive (e.g. <del> from strikethrough is dropped if not allowed).
var bodyMarkdown = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Strikethrough,
		extension.Linkify,
	),
)

// RenderBodyHTML converts a markdown/plain-text body to HTML.
// If rendering fails it falls back to the escaped plain text wrapped in a
// paragraph tag so the frontend never receives an empty body_html for a
// non-empty body.
func RenderBodyHTML(body string) string {
	if body == "" {
		return ""
	}

	var buf bytes.Buffer
	if err := bodyMarkdown.Convert([]byte(body), &buf); err != nil {
		return "<p>" + SanitizeText(body) + "</p>"
	}

	html := buf.String()
	if html == "" {
		html = "<p>" + SanitizeText(body) + "</p>"
	}

	return html
}
