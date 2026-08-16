package extractor

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
)

// rssFeedForRuleBased mirrors the RSS struct used by the fetcher but is
// kept local to avoid an import cycle (fetcher imports sources, which
// would pull in more deps). We only need the item-level fields for
// rule-based extraction.
type rssFeedForRuleBased struct {
	Items []rssItemForRuleBased `xml:"channel>item"`
}

type rssItemForRuleBased struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author"`
	DCCreator   string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Category    string `xml:"category"`
}

// RuleBasedExtract attempts to extract structured data from source text
// without an LLM call. Currently supports RSS feeds for the news entity
// type: it parses the XML, takes the first (most recent) feed item, and
// maps the RSS fields to the news schema fields.
//
// This serves as a fallback when no AI provider is configured (or the AI
// provider returns errors), ensuring that RSS-sourced news records still
// get structured data in the moderation queue instead of empty {} blobs.
//
// Returns (ExtractResult, true) when rule-based extraction succeeds,
// (ExtractResult{}, false) when the source text is not suitable for
// rule-based extraction (not RSS, or no items found).
func RuleBasedExtract(schema schemas.EntitySchema, sourceURL, sourceType, sourceText string) (ExtractResult, bool) {
	if schema.EntityType != schemas.EntityTypeNews {
		return ExtractResult{}, false
	}

	// Quick check: is this RSS/XML content?
	trimmed := strings.TrimSpace(sourceText)
	if !strings.HasPrefix(trimmed, "<?xml") && !strings.HasPrefix(trimmed, "<rss") && !strings.Contains(trimmed, "<rss") {
		return ExtractResult{}, false
	}

	var feed rssFeedForRuleBased
	if err := xml.Unmarshal([]byte(sourceText), &feed); err != nil {
		return ExtractResult{}, false
	}
	if len(feed.Items) == 0 {
		return ExtractResult{}, false
	}

	// Use the first (most recent) item — RSS feeds list newest first.
	item := feed.Items[0]
	ext := make(Extraction)

	// title (required field)
	addField(ext, "title", item.Title, item.Title)

	// summary from description (strip HTML tags)
	summary := stripHTMLTags(item.Description)
	if summary != "" {
		addField(ext, "summary", summary, item.Description)
	}

	// published_date — parse RSS date format to ISO 8601
	if item.PubDate != "" {
		if isoDate, err := parseRSSDate(item.PubDate); err == nil {
			addField(ext, "published_date", isoDate, item.PubDate)
		}
	}

	// article_url
	if item.Link != "" {
		addField(ext, "article_url", item.Link, item.Link)
	}

	// author — try dc:creator first, then author
	author := item.DCCreator
	if author == "" {
		author = item.Author
	}
	// RSS author often contains email in parens: "email (Name)" — extract name
	author = extractAuthorName(author)
	if author != "" {
		addField(ext, "author", author, author)
	}

	// topics from category
	if item.Category != "" {
		addField(ext, "topics", []string{item.Category}, item.Category)
	}

	// key_points — derive from summary (first sentence or two)
	if summary != "" {
		points := deriveKeyPoints(summary)
		if len(points) > 0 {
			addField(ext, "key_points", points, summary)
		}
	}

	// countries — not available from RSS metadata; leave for LLM or
	// manual review. The field is optional in the schema.

	// Validate against the schema (hallucination guard, type coercion).
	cleaned, verrs := Validate(ext, schema)
	return ExtractResult{Extraction: cleaned, Errors: verrs}, true
}

// addField adds a FieldExtraction triple to the extraction map with a
// high confidence (0.95) since rule-based extraction from structured XML
// is highly reliable. The evidence is the verbatim source text snippet.
func addField(ext Extraction, name string, value any, evidence string) {
	ev := evidence
	ext[name] = FieldExtraction{
		Value:      value,
		Confidence: 0.95,
		Evidence:   &ev,
	}
}

// stripHTMLTags removes HTML tags from a string and collapses whitespace.
// RSS <description> fields often contain HTML.
func stripHTMLTags(s string) string {
	// Remove HTML tags
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			result.WriteRune(' ')
		case !inTag:
			result.WriteRune(r)
		}
	}
	// Collapse whitespace
	return strings.TrimSpace(strings.Join(strings.Fields(result.String()), " "))
}

// parseRSSDate parses common RSS date formats and returns ISO 8601 (YYYY-MM-DD).
func parseRSSDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	// RFC 1123Z (most common RSS date format): "Mon, 02 Jan 2006 15:04:05 MST"
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 2 Jan 2006 15:04:05 MST",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("could not parse date: %s", s)
}

// extractAuthorName extracts the author name from an RSS author field.
// RSS author often contains "email (Name)" — we want just "Name".
func extractAuthorName(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "("); idx >= 0 {
		end := strings.Index(s, ")")
		if end > idx {
			return strings.TrimSpace(s[idx+1 : end])
		}
	}
	// If it looks like an email, return empty (no usable name)
	if strings.Contains(s, "@") && !strings.Contains(s, " ") {
		return ""
	}
	return s
}

// deriveKeyPoints splits a summary into 1-3 concise points.
func deriveKeyPoints(summary string) []string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	// Split on sentence boundaries
	sentences := splitSentences(summary)
	if len(sentences) > 3 {
		sentences = sentences[:3]
	}
	// Trim each to 200 chars
	var points []string
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) > 200 {
			s = s[:197] + "..."
		}
		if s != "" {
			points = append(points, s)
		}
	}
	return points
}

// splitSentences splits text on common sentence boundaries.
func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "!", ".\n")
	text = strings.ReplaceAll(text, "?", ".\n")
	parts := strings.Split(text, ".")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
