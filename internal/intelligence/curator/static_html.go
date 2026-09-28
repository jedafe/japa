package curator

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// StaticHTML curates static/oneshot crawl-method channels: the raw doc
// body is a fetched HTML page. It is the deterministic-first stage of the
// hybrid pipeline — when a page carries enough structure to populate a
// record without a model call, no LLM spend happens at all.
//
// Extraction layers, highest-authority first:
//
//  1. provider_config.selectors — an operator-declared field→CSS-selector
//     map for sites with stable templates (e.g. a scholarship database).
//     Configured selectors count as the extraction contract; values are
//     stamped with curator confidence 1.0.
//  2. schema.org JSON-LD blocks embedded in the page — the same node
//     mapping the jsonld curator uses; a typed node (JobPosting, Event,
//     NewsArticle…) produces its own record.
//  3. Generic document metadata — OpenGraph/Twitter/meta tags plus a
//     small set of conservative text heuristics (deadline dates near
//     "deadline"/"closing date" phrasing). These carry reduced confidence
//     (0.7) so the review surface can tell guessed fields apart from
//     structured ones.
//
// Quality gate: a record needs a title plus at least one other populated
// field. Below the bar the curator returns ErrNoCurator — the worker then
// falls back to the LLM extract pipeline, which is the designed behaviour
// for unstructured pages. An empty record is never persisted.
//
// The channel provider_config contract is:
//
//	{
//	  "selectors":   {"title": "h1.program-title", "deadline": ".app-deadline"},
//	  "entity_type": "scholarship"
//	}
type StaticHTML struct {
	// method is the crawl method this registration serves — the same logic
	// covers static and oneshot bodies.
	method string
}

// Handles returns the crawl method this curator serves.
func (s StaticHTML) Handles() string { return s.method }

// staticConfig is the channel provider_config contract for static curation.
type staticConfig struct {
	// Selectors maps a schema field name to a CSS selector; the first
	// matching element's text becomes the field value.
	Selectors map[string]string `json:"selectors"`
	// EntityType overrides the record entity type (default: channel's
	// entity_types[0]).
	EntityType string `json:"entity_type"`
}

// heuristicFieldConfidence marks fields derived from loose text patterns —
// structured sources (ld+json, selectors) stay at 1.0.
const heuristicFieldConfidence = 0.7

// deadlineRe finds date-like strings immediately after deadline phrasing.
// Kept deliberately narrow: a bare date anywhere in body copy is not a
// deadline, so the pattern requires the label.
var deadlineRe = regexp.MustCompile(`(?i)\b(?:deadline|closing\s+date|apply\s+by|applications?\s+close[s]?(?:\s+on)?|due\s+date)\b\s*[:.\-–—]?\s*` +
	`(\d{4}-\d{2}-\d{2}|` + // 2025-12-31
	`[A-Za-z]{3,9}\s+\d{1,2}(?:st|nd|rd|th)?,?\s+\d{4}|` + // December 15, 2025
	`\d{1,2}(?:st|nd|rd|th)?\s+[A-Za-z]{3,9},?\s+\d{4}|` + // 15 December 2025
	`\d{1,2}/\d{1,2}/\d{4})`) // 15/12/2025

// Curate runs the layered static-HTML extraction described on StaticHTML.
func (s StaticHTML) Curate(_ context.Context, body []byte, contentType string, ch sources.Channel) ([]Record, error) {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return nil, errors.New("curator: static: empty body")
	}
	// A static channel that produced non-HTML (a JSON blob, an RSS feed that
	// drifted methods) is outside this curator's contract — defer to the
	// extract pipeline rather than error into a review stub.
	if !strings.HasPrefix(raw, "<") && !strings.Contains(strings.ToLower(contentType), "html") {
		return nil, ErrNoCurator
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return nil, errors.New("curator: static: unparseable HTML")
	}

	cfg := parseStaticConfig(ch.ProviderConfig)
	entityType := resolveEntityType(cfg.EntityType, ch)
	schema, _ := schemas.ForEntityType(entityType)

	// Layer 2 first: typed ld+json nodes are authoritative structured data —
	// one record per recognized node, exactly like the jsonld curator.
	if recs := ldjsonRecordsFromHTML(doc, ch); len(recs) > 0 {
		return recs, nil
	}

	// Page-level record: selectors + generic metadata + heuristics.
	ext := make(extractor.Extraction)

	// Layer 1: operator selectors (highest authority for page-level fields).
	for fieldName, sel := range cfg.Selectors {
		if sel == "" {
			continue
		}
		if text := collapseSpace(doc.Find(sel).First().Text()); text != "" {
			field(ext, fieldName, selectorCoerce(schema, fieldName, text), text)
		}
	}

	// Layer 3: generic metadata fills whatever selectors didn't.
	metaFill(ext, doc)

	// Heuristic deadline — only when no structured date was found.
	if !hasField(ext, "deadline") && !hasField(ext, "published_date") {
		if m := deadlineRe.FindStringSubmatch(visibleText(doc)); len(m) == 2 {
			if iso := parseHumanDate(m[1]); iso != "" {
				fieldConf(ext, "deadline", iso, m[0], heuristicFieldConfidence)
			}
		}
	}

	// Quality gate — title + one more field, else the LLM gets the page.
	if staticRecordTooThin(ext, cfg) {
		return nil, ErrNoCurator
	}

	if _, ok := ext["title"]; !ok {
		return nil, ErrNoCurator
	}

	return []Record{{
		Extraction:  validate(ext, entityType),
		ExternalKey: staticPageKey(ext, ch),
		EntityType:  entityType,
	}}, nil
}

// staticRecordTooThin reports whether the page-level extraction is below
// the quality bar: without configured selectors a title alone is not a
// record worth persisting (LLM fallback is designed for that case); with
// selectors configured the operator declared the mapping sufficient — the
// record ships even if only the title selector matched.
func staticRecordTooThin(ext extractor.Extraction, cfg staticConfig) bool {
	if _, ok := ext["title"]; !ok {
		return true
	}
	if len(cfg.Selectors) > 0 {
		return false
	}
	for name, f := range ext {
		if name != "title" && name != "source_url" && f.Value != nil {
			return false
		}
	}

	return true
}

// ldjsonRecordsFromHTML extracts application/ld+json blocks from the page
// and maps each recognized typed node to a record — reusing the jsonld
// curator's node mapping so static pages with embedded schema.org markup
// get the full deterministic treatment.
func ldjsonRecordsFromHTML(doc *goquery.Document, ch sources.Channel) []Record {
	// Each block is decoded as a complete JSON value — unlike the jsonld
	// fetcher body (newline-joined single-line blocks), in-page scripts can
	// hold multi-line JSON, so the line-splitting helper does not apply.
	var nodes []map[string]any
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, s *goquery.Selection) {
		var v any
		if err := json.Unmarshal([]byte(strings.TrimSpace(s.Text())), &v); err == nil {
			nodes = append(nodes, flattenLDJSON(v)...)
		}
	})
	if len(nodes) == 0 {
		return nil
	}

	var out []Record
	for _, node := range nodes {
		entityType := nodeEntityType(node)
		if entityType == "" {
			continue
		}
		if len(ch.EntityTypes) > 0 && !containsString(ch.EntityTypes, entityType) {
			continue
		}
		ext := nodeToExtraction(node, entityType)
		if ext == nil {
			continue
		}
		out = append(out, Record{
			Extraction:  validate(ext, entityType),
			ExternalKey: ldjsonKey(node, entityType),
			EntityType:  entityType,
		})
	}

	return out
}

// metaFill populates generic fields from OpenGraph/Twitter/meta tags and
// the document head — title, summary, canonical URL, dates, image.
func metaFill(ext extractor.Extraction, doc *goquery.Document) {
	meta := func(keys ...string) string {
		for _, k := range keys {
			var v string
			doc.Find("meta").EachWithBreak(func(_ int, s *goquery.Selection) bool {
				p, _ := s.Attr("property")
				n, _ := s.Attr("name")
				if p == k || n == k {
					v, _ = s.Attr("content")
					return false
				}
				return true
			})
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}

	if !hasField(ext, "title") {
		if t := meta("og:title", "twitter:title"); t != "" {
			field(ext, "title", t, t)
		} else if t := collapseSpace(doc.Find("title").First().Text()); t != "" {
			fieldConf(ext, "title", t, t, heuristicFieldConfidence)
		}
	}
	if !hasField(ext, "description_summary") && !hasField(ext, "summary") {
		if d := meta("og:description", "twitter:description", "description"); d != "" {
			field(ext, "description_summary", d, d)
		}
	}
	if !hasField(ext, "application_url") && !hasField(ext, "article_url") {
		if u := meta("og:url"); u != "" {
			field(ext, "application_url", u, u)
		} else if href, ok := doc.Find(`link[rel="canonical"]`).First().Attr("href"); ok && href != "" {
			field(ext, "application_url", href, href)
		}
	}
	if !hasField(ext, "published_date") && !hasField(ext, "deadline") {
		if d := meta("article:published_time", "date", "dcterms.date"); d != "" {
			if iso := ldjsonDate(d); iso != "" {
				field(ext, "published_date", iso, d)
			}
		}
	}
	if !hasField(ext, "image_url") {
		if i := meta("og:image", "twitter:image"); i != "" {
			field(ext, "image_url", i, i)
		}
	}
}

// selectorCoerce shapes a selector-derived value for its schema field
// type: array-typed fields take single-element slices and date fields are
// normalized to ISO 8601 — extractor.Validate's date coercer accepts only
// ISO input, so a raw "15 December 2026" selector hit must be converted
// here or it lands as null.
func selectorCoerce(schema schemas.EntitySchema, fieldName, value string) any {
	switch t := schemaFieldType(schema, fieldName); t {
	case "text_array", "country_array":
		return []string{value}
	case "date":
		if iso := parseHumanDate(value); iso != "" {
			return iso
		}
		if iso := ldjsonDate(value); iso != "" {
			return iso
		}
	}

	return value
}

// staticPageKey derives the page record's external key: the application/
// article URL when present, else a hash of title + entity-relevant values.
func staticPageKey(ext extractor.Extraction, ch sources.Channel) string {
	for _, name := range []string{"application_url", "article_url"} {
		if f, ok := ext[name]; ok {
			if s, ok := f.Value.(string); ok && s != "" {
				return s
			}
		}
	}
	if ch.EntryUrl != "" {
		return ch.EntryUrl
	}

	title, _ := ext["title"].Value.(string)

	return "hash:" + hashKey(resolveEntityType("", ch), title)
}

// visibleText returns the page's visible text with non-content elements
// removed — the corpus the text heuristics scan.
func visibleText(doc *goquery.Document) string {
	sel := doc.Find("body")
	if sel.Length() == 0 {
		sel = doc.Selection
	}
	// Clone before stripping — removing nodes mutates the shared tree, and
	// the caller's document is still needed for meta/ld+json extraction.
	c := sel.Clone()
	c.Find("script, style, noscript, template, svg, iframe, head").Remove()

	return collapseSpace(c.Text())
}

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func hasField(ext extractor.Extraction, name string) bool {
	f, ok := ext[name]

	return ok && f.Value != nil
}

// fieldConf is field() with an explicit confidence — heuristic fields are
// guesses, not verbatim structured data.
func fieldConf(ext extractor.Extraction, name string, value any, evidence string, conf float64) {
	if value == nil {
		return
	}
	if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
		return
	}
	ev := evidence
	ext[name] = extractor.FieldExtraction{Value: value, Confidence: conf, Evidence: &ev}
}

// parseHumanDate normalizes the date shapes deadlineRe captures to
// YYYY-MM-DD. Ordinal suffixes (15th) are stripped first.
func parseHumanDate(s string) string {
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`(?i)(\d)(st|nd|rd|th)\b`).ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, ",", "")

	for _, f := range []string{
		"2006-01-02",
		"January 2 2006", "Jan 2 2006",
		"2 January 2006", "2 Jan 2006",
		"01/02/2006",
	} {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02")
		}
	}

	return parseFeedDate(s)
}

func parseStaticConfig(raw []byte) staticConfig {
	var cfg staticConfig
	if len(raw) == 0 {
		return cfg
	}
	_ = json.Unmarshal(raw, &cfg)

	return cfg
}
