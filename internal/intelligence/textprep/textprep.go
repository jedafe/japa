// Package textprep prepares raw fetched document bodies for the LLM
// extraction prompt. Raw intel documents are stored verbatim (HTML with
// scripts, styles, and markup — up to hundreds of KB); feeding that into
// the extraction prompt wastes tokens and produces all-null results from
// weaker models. SourceText converts HTML bodies to visible text and caps
// the length; non-HTML bodies (RSS/Atom XML, JSON, plain text) pass through
// unchanged so the rule-based extractors and curators that need the raw
// markup still see it.
//
// IsSoft404 detects documents whose HTTP status was 200 but whose content
// is actually an error page ("404: Page Not Found" served with 200, CMS
// error pages, "an error has occurred" interstitials). The extract worker
// skips those documents instead of burning an LLM call that can only
// return nulls.
package textprep

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// DefaultMaxSourceChars caps the source text sent to the LLM. ~60k chars is
// roughly 15k tokens — enough to cover a full listing/detail page while
// staying well inside free-tier model context windows. This is an
// implementation constant, not an operator knob: truncation beyond this
// point only cuts boilerplate navigation/footer text.
const DefaultMaxSourceChars = 60000

// soft404Patterns match the visible title/heading text of error pages that
// sites serve with HTTP 200. Anchored patterns only — a scholarship page
// mentioning "404" in body copy must not trip the detector.
var soft404Patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*404\b`),
	regexp.MustCompile(`(?i)\b404\s*[:–-]\s*(page\s+not\s+found|error|not\s+found)`),
	regexp.MustCompile(`(?i)\bpage\s+not\s+found\b`),
	regexp.MustCompile(`(?i)\b(page|file|document)\s+(was\s+)?not\s+found\b`),
	regexp.MustCompile(`(?i)\bthis\s+page\s+(doesn'?t|does\s+not)\s+exist\b`),
	regexp.MustCompile(`(?i)^\s*(error|fehler|erreur|errore)\s*[:–-]?\s*404\b`),
	regexp.MustCompile(`(?i)^\s*ein\s+fehler\s+ist\s+aufgetreten\b`),
	regexp.MustCompile(`(?i)^\s*(une\s+erreur|page\s+introuvable)\b`),
}

// SourceText converts a raw document body into the text sent to the LLM
// extractor. HTML is parsed and reduced to visible text (script, style,
// noscript, template, svg, and iframe contents removed); everything else is
// returned unchanged apart from the length cap. maxChars <= 0 applies
// DefaultMaxSourceChars.
func SourceText(body, contentType string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = DefaultMaxSourceChars
	}

	if isHTML(body, contentType) {
		body = htmlToText(body)
	}

	if len(body) > maxChars {
		// Cut at a rune boundary so we never emit an invalid UTF-8 tail.
		runes := []rune(body)
		if len(runes) > maxChars {
			body = string(runes[:maxChars])
		}
	}

	return body
}

// IsSoft404 reports whether a fetched document is an error page despite a
// 2xx HTTP status. httpStatus >= 400 is a hard signal; otherwise the
// document's <title> and the first 500 chars of visible text are checked
// against the error-page patterns.
func IsSoft404(body, contentType string, httpStatus int) bool {
	if httpStatus >= 400 {
		return true
	}

	if !isHTML(body, contentType) {
		return false
	}

	title, text := htmlTitleAndText(body)
	if text == "" {
		return false
	}

	head := text
	if len([]rune(head)) > 500 {
		head = string([]rune(head)[:500])
	}

	for _, pat := range soft404Patterns {
		if pat.MatchString(title) || pat.MatchString(head) {
			return true
		}
	}

	return false
}

// HasExtractableText reports whether a document carries enough visible
// text for the LLM extractor to work on. HTML pages whose body reduces to
// under 20 chars of text are JS-shell SPAs or blank pages — extraction can
// only yield nulls from them, so the worker skips them instead of burning
// an LLM call.
func HasExtractableText(body, contentType string) bool {
	if !isHTML(body, contentType) {
		return strings.TrimSpace(body) != ""
	}

	_, text := htmlTitleAndText(body)

	return len([]rune(text)) >= 20
}

// isHTML reports whether the body should be treated as HTML — by declared
// content type first, then by sniffing for markup. RSS/Atom/XML, JSON, and
// plain-text bodies are left untouched for the rule-based extractors.
func isHTML(body, contentType string) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "html") {
		return true
	}

	if ct != "" && (strings.Contains(ct, "xml") || strings.Contains(ct, "json") || strings.Contains(ct, "calendar") || strings.Contains(ct, "csv")) {
		return false
	}

	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}

	if strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<rss") || strings.HasPrefix(trimmed, "<feed") {
		return false
	}

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return false
	}

	return strings.HasPrefix(trimmed, "<")
}

// htmlToText reduces an HTML document to its visible text: scripts, styles,
// and other non-content elements are removed, then remaining text nodes are
// joined with whitespace collapsed.
func htmlToText(body string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return collapseWS(body)
	}

	doc.Find("script, style, noscript, template, svg, iframe, head").Remove()

	sel := doc.Find("body")
	if sel.Length() == 0 {
		sel = doc.Selection
	}

	return collapseWS(sel.Text())
}

// htmlTitleAndText returns the document's <title> text and the cleaned
// visible text, for soft-404 pattern matching.
func htmlTitleAndText(body string) (title, text string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", collapseWS(body)
	}

	title = collapseWS(doc.Find("title").First().Text())

	doc.Find("script, style, noscript, template, svg, iframe, head").Remove()

	sel := doc.Find("body")
	if sel.Length() == 0 {
		sel = doc.Selection
	}

	return title, collapseWS(sel.Text())
}

func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
