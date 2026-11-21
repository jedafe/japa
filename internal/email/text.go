// Package email — text.go derives the plain-text alternative part that
// accompanies every HTML message as multipart/alternative. HTML-only mail
// is a real spam-score flag (SpamAssassin MIME_HTML_ONLY and similar
// receiver-side heuristics), so providers never emit it: callers may set
// EmailRequest.TextBody explicitly, and when absent one is derived from
// the HTML body here.
package email

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// textSkipElements contribute no text to the derived alternative part —
// document metadata and non-rendering elements only.
var textSkipElements = map[atom.Atom]bool{
	atom.Head:     true,
	atom.Noscript: true,
	atom.Script:   true,
	atom.Style:    true,
	atom.Template: true,
	atom.Title:    true,
}

// textBlockElements mark a line boundary in the derived text — roughly
// the HTML elements that render as blocks in a mail client.
var textBlockElements = map[atom.Atom]bool{
	atom.Address: true, atom.Article: true, atom.Aside: true,
	atom.Blockquote: true, atom.Br: true, atom.Dd: true, atom.Div: true,
	atom.Dl: true, atom.Dt: true, atom.Fieldset: true, atom.Figcaption: true,
	atom.Figure: true, atom.Footer: true, atom.Form: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true,
	atom.H5: true, atom.H6: true, atom.Header: true, atom.Hr: true,
	atom.Li: true, atom.Main: true, atom.Nav: true, atom.Ol: true,
	atom.P: true, atom.Pre: true, atom.Section: true, atom.Table: true,
	atom.Tbody: true, atom.Td: true, atom.Tfoot: true, atom.Th: true,
	atom.Thead: true, atom.Tr: true, atom.Ul: true,
}

// TextAlternative returns the plain-text part to pair with an HTML Body.
// An explicit TextBody wins; otherwise it is derived from the HTML.
// Returns "" for non-HTML requests (Body already is the text part).
func (r EmailRequest) TextAlternative() string {
	if !r.IsHTML {
		return ""
	}

	if strings.TrimSpace(r.TextBody) != "" {
		return r.TextBody
	}

	return HTMLToText(r.Body)
}

// HTMLToText renders an HTML email body as readable plain text: script,
// style and head content is dropped, block elements become line breaks,
// list items get "- " bullets, images contribute their alt text, and
// links keep their text followed by the href in <angle brackets> when it
// differs from the link text (so buttons like "Verify Email" still expose
// their destination).
func HTMLToText(htmlBody string) string {
	if strings.TrimSpace(htmlBody) == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return ""
	}

	var b strings.Builder

	var walk func(*html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if textSkipElements[n.DataAtom] {
				return
			}

			if n.DataAtom == atom.A {
				writeAnchorText(&b, n)

				return
			}

			if textBlockElements[n.DataAtom] {
				b.WriteString("\n")
			}

			if n.DataAtom == atom.Li {
				b.WriteString("- ")
			}

			if n.DataAtom == atom.Img {
				if alt := elementAttr(n, "alt"); alt != "" {
					b.WriteString(alt)
				}
			}
		}

		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}

		if n.Type == html.ElementNode && textBlockElements[n.DataAtom] {
			b.WriteString("\n")
		}
	}

	walk(doc)

	return normalizePlainText(b.String())
}

// writeAnchorText emits an <a> element's link text followed by its href
// in <angle brackets> when the href is a remote URL distinct from the
// text — "Verify Email <https://…>" — while bare-URL links render once.
// The element's children are consumed here, so the caller must not walk
// them again.
func writeAnchorText(b *strings.Builder, n *html.Node) {
	var t strings.Builder

	var collect func(*html.Node)

	collect = func(x *html.Node) {
		if x.Type == html.TextNode {
			t.WriteString(x.Data)
		}

		for c := x.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}

	collect(n)

	linkText := strings.Join(strings.Fields(t.String()), " ")
	href := strings.TrimSpace(elementAttr(n, "href"))

	b.WriteString(linkText)

	if isRemoteURL(href) && !strings.EqualFold(linkText, href) {
		b.WriteString(" <" + href + ">")
	}
}

// elementAttr returns an element attribute value, or "" when absent.
func elementAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}

// isRemoteURL reports whether href is an http(s) link worth annotating —
// mailto/tel/anchors are not destinations the text part needs to surface.
func isRemoteURL(href string) bool {
	l := strings.ToLower(href)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// normalizePlainText trims each line, collapses intra-line whitespace
// runs, drops blank runs down to a single separating blank line, and
// trims leading/trailing blanks — the raw walk output is whitespace-noisy
// because every block element contributes a newline marker.
func normalizePlainText(s string) string {
	var out []string

	prevBlank := true

	for _, ln := range strings.Split(s, "\n") {
		ln = strings.Join(strings.Fields(ln), " ")
		if ln == "" {
			prevBlank = true
			continue
		}

		if prevBlank && len(out) > 0 {
			out = append(out, "")
		}

		out = append(out, ln)
		prevBlank = false
	}

	return strings.Join(out, "\n")
}
