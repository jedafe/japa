// Package content — §45 FAQ accordion → FAQPage JSON-LD.
//
// The editor's faq block stores semantic <details>/<summary> markup in
// body_html (see packages/ui/src/editor/nodes/FaqNode.ts and the
// data-type whitelist in sanitize.go). That markup renders an accessible
// accordion but carries no structured data — this file extracts the Q/A
// pairs back out of the stored HTML and merges a schema.org FAQPage node
// into the resolved JSON-LD sidecar, the same way
// injectListItemListElements (list.go) post-processes ItemList documents.
package content

import (
	"bytes"
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// faqQA is one question/answer pair extracted from a faqItem block. The
// answer keeps its inner HTML — schema.org acceptedAnswer.text permits
// markup, and body_html is already sanitized so it is safe to re-emit.
type faqQA struct {
	question string
	answer   string
}

// InjectFaqPageJSONLD merges a FAQPage node — built from the §45 faq-item
// blocks in the content's stored markup — into the resolved JSON-LD
// document. bodyHtml is the canonical rendered body and is authoritative —
// when it exists, markup that did not survive rendering (goldmark drops
// raw HTML from markdown bodies, e.g. inside fenced code samples) is
// invisible to readers and must not reach structured data. The raw body is
// only parsed when body_html is empty — the page then renders the body
// itself, so any faq markup in it is genuinely visible. No faq items, or a
// resolved document that cannot be merged into, returns jsonLd unchanged.
func InjectFaqPageJSONLD(jsonLd []byte, bodyHtml, body string) []byte {
	items := extractFaqItems(bodyHtml)
	if len(items) == 0 && strings.TrimSpace(bodyHtml) == "" {
		items = extractFaqItems(body)
	}

	if len(items) == 0 {
		return jsonLd
	}

	entities := make([]map[string]any, 0, len(items))

	for _, qa := range items {
		entities = append(entities, map[string]any{
			"@type": "Question",
			"name":  qa.question,
			"acceptedAnswer": map[string]any{
				"@type": "Answer",
				"text":  qa.answer,
			},
		})
	}

	faqPage := map[string]any{
		"@type":      "FAQPage",
		"mainEntity": entities,
	}

	// No template resolved — emit a standalone FAQPage document.
	if len(bytes.TrimSpace(jsonLd)) == 0 {
		faqPage["@context"] = "https://schema.org"
		if out, err := json.Marshal(faqPage); err == nil {
			return out
		}

		return jsonLd
	}

	var doc map[string]any
	if err := json.Unmarshal(jsonLd, &doc); err != nil {
		return jsonLd
	}

	// A resolved "null"/"{}"/non-entity payload has nothing to merge into —
	// emit the standalone FAQPage instead of wrapping a null graph member.
	if len(doc) == 0 {
		faqPage["@context"] = "https://schema.org"
		if out, err := json.Marshal(faqPage); err == nil {
			return out
		}

		return jsonLd
	}

	// The resolved template already carries FAQPage — don't duplicate it.
	if jsonLDHasType(doc, "FAQPage") {
		return jsonLd
	}

	// An existing @graph (e.g. admin-authored templates) gets the FAQPage
	// appended; a plain single-entity document wraps into a new @graph
	// alongside it so both schemas reach the crawler in one script tag.
	if graph, ok := doc["@graph"].([]any); ok {
		doc["@graph"] = append(graph, faqPage)
	} else {
		contextVal := "https://schema.org"
		if c, ok := doc["@context"].(string); ok && c != "" {
			contextVal = c
		}

		// The member inherits the graph's @context — drop the inner copy
		// when it's identical so validators see canonical flat output
		// (a different context is a legal JSON-LD scoped override and is
		// kept so template semantics don't change).
		if doc["@context"] == contextVal {
			delete(doc, "@context")
		}

		doc = map[string]any{
			"@context": contextVal,
			"@graph":   []any{doc, faqPage},
		}
	}

	if out, err := json.Marshal(doc); err == nil {
		return out
	}

	return jsonLd
}

// jsonLDHasType reports whether a resolved JSON-LD document already carries
// a node of the given @type — at the top level or inside @graph.
func jsonLDHasType(doc map[string]any, typeName string) bool {
	if jsonLDTypeIs(doc, typeName) {
		return true
	}

	if graph, ok := doc["@graph"].([]any); ok {
		for _, node := range graph {
			if m, ok := node.(map[string]any); ok && jsonLDTypeIs(m, typeName) {
				return true
			}
		}
	}

	return false
}

// jsonLDTypeIs reports whether node's @type equals typeName. @type may be a
// string or an array of types (JSON-LD allows both spellings).
func jsonLDTypeIs(node map[string]any, typeName string) bool {
	switch t := node["@type"].(type) {
	case string:
		return t == typeName
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok && s == typeName {
				return true
			}
		}
	}

	return false
}

// extractFaqItems walks an HTML body and returns the ordered Q/A pairs of
// every <details data-type="faq-item"> accordion item. Items missing
// either side are skipped — a lone question renders a schema-invalid
// FAQPage Question (same rule as the deals faq form).
func extractFaqItems(body string) []faqQA {
	if !strings.Contains(body, "faq-item") {
		return nil
	}

	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	var items []faqQA

	var walk func(n *html.Node)

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "details" && attrValue(n, "data-type") == "faq-item" {
			if qa, ok := parseFaqItem(n); ok {
				items = append(items, qa)
			}

			return // faq items cannot nest — no descent needed
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)

	return items
}

// parseFaqItem extracts the question text and answer inner-HTML from a
// faq-item <details> element: <summary data-type="faq-question"> text and
// <div data-type="faq-answer"> children. Both are direct children per the
// node schema (faqItem = faqSummary + faqContent); descendants are scanned
// leniently so hand-authored markup with a wrapper still resolves.
func parseFaqItem(item *html.Node) (faqQA, bool) {
	var question, answer string

	var find func(n *html.Node)

	find = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}

			switch {
			case c.Data == "details":
				// Never descend into a nested details — its pair is its own item.
				continue
			case c.Data == "summary" && attrValue(c, "data-type") == "faq-question" && question == "":
				question = strings.Join(strings.Fields(textOf(c)), " ")
			case c.Data == "div" && attrValue(c, "data-type") == "faq-answer" && answer == "":
				inner := strings.TrimSpace(innerHTML(c))
				// An answer with no visible text and no media (e.g. the
				// editor's empty trailing <p></p>) is a missing answer —
				// emitting "text":"<p></p>" renders a schema-invalid
				// FAQPage Question.
				if strings.TrimSpace(textOf(c)) != "" || strings.Contains(inner, "<img") {
					// Re-sanitize: body_html inputs are already clean so
					// this is a no-op, but the markdown `body` fallback is
					// pre-sanitization user input — unsafe hrefs and tags
					// must not be re-emitted into structured data.
					answer = SanitizeHTML(inner)
				}
			default:
				find(c)
			}
		}
	}

	find(item)

	if question == "" || answer == "" {
		return faqQA{}, false
	}

	return faqQA{question: question, answer: answer}, true
}

// attrValue returns the lower-cased element attribute value (HTML
// attribute names are case-insensitive; the parser already lowercases).
func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}

// textOf returns the concatenated text content of a node subtree.
func textOf(n *html.Node) string {
	var buf bytes.Buffer

	extractText(&buf, n)

	return buf.String()
}

// innerHTML renders the children of n back to an HTML string.
func innerHTML(n *html.Node) string {
	var buf bytes.Buffer

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return ""
		}
	}

	return buf.String()
}
