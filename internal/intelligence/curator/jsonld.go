package curator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// JSONLD curates jsonld crawl-method channels: the raw doc body is the
// page's <script type="application/ld+json"> blocks, newline-joined (the
// jsonld fetcher stores exactly that). Each block is a JSON object or
// array; objects may nest their nodes under @graph. Every node whose @type
// maps to a registered entity type becomes one record — the @type mapping
// is the per-curator entity-type override the channel's entity_types
// cannot express on a mixed-type page.
type JSONLD struct{}

// Handles returns the crawl method this curator serves.
func (JSONLD) Handles() string { return string(sources.CrawlMethodJSONLD) }

// jsonldEntityTypes maps schema.org @type → intel entity_type. The worker
// uses a record's EntityType over the channel default when it is set.
var jsonldEntityTypes = map[string]string{
	"JobPosting":           schemas.EntityTypeJob,
	"NewsArticle":          schemas.EntityTypeNews,
	"Article":              schemas.EntityTypeArticle,
	"BlogPosting":          schemas.EntityTypeArticle,
	"Report":               schemas.EntityTypeArticle,
	"Event":                schemas.EntityTypeEvent,
	"JobPostingOccupation": schemas.EntityTypeJob,
	"FAQPage":              schemas.EntityTypeArticle,
}

// Curate parses the newline-joined ld+json blocks and returns one record
// per recognized typed node.
func (JSONLD) Curate(_ context.Context, body []byte, _ string, ch sources.Channel) ([]Record, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, errors.New("curator: jsonld: empty body")
	}

	nodes, err := ldjsonNodes(body)
	if err != nil {
		return nil, err
	}

	var out []Record

	for _, node := range nodes {
		entityType := nodeEntityType(node)
		if entityType == "" {
			continue
		}
		// Channel entity_types is the declared contract — a node type the
		// channel doesn't declare is skipped (e.g. a stray BreadcrumbList or
		// a JobPosting on a news channel).
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

	if len(out) == 0 {
		return nil, errors.New("curator: jsonld: no supported @type nodes found")
	}

	return out, nil
}

// ldjsonNodes decodes every newline-joined ld+json block and flattens
// top-level arrays and @graph containers into a flat node list.
func ldjsonNodes(body []byte) ([]map[string]any, error) {
	var nodes []map[string]any

	decoded := 0

	for _, block := range strings.Split(string(body), "\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		var v any
		if err := json.Unmarshal([]byte(block), &v); err != nil {
			// A single malformed block must not sink the whole document —
			// pages routinely embed invalid JSON-LD.
			continue
		}

		decoded++
		nodes = append(nodes, flattenLDJSON(v)...)
	}

	if decoded == 0 {
		return nil, errors.New("curator: jsonld: no parseable ld+json blocks")
	}

	return nodes, nil
}

// flattenLDJSON expands a decoded ld+json value into typed nodes: objects
// contribute themselves plus their @graph members; arrays contribute each
// element.
func flattenLDJSON(v any) []map[string]any {
	var out []map[string]any

	switch t := v.(type) {
	case map[string]any:
		out = append(out, t)
		if graph, ok := t["@graph"].([]any); ok {
			out = append(out, flattenLDJSON(graph)...)
		}
	case []any:
		for _, el := range t {
			out = append(out, flattenLDJSON(el)...)
		}
	}

	return out
}

// nodeEntityType returns the intel entity_type for a node's @type, or ""
// when the type is unsupported. @type may be a string or a string array.
func nodeEntityType(node map[string]any) string {
	types := ldjsonStrings(node["@type"])
	for _, ty := range types {
		if et, ok := jsonldEntityTypes[ty]; ok {
			return et
		}
	}

	return ""
}

// nodeToExtraction maps a typed ld+json node onto the target entity
// schema's field set. Returns nil when the node carries no title/name —
// the one field every curated schema requires.
func nodeToExtraction(node map[string]any, entityType string) extractor.Extraction {
	ext := make(extractor.Extraction)

	switch entityType {
	case schemas.EntityTypeJob:
		jobNodeToExtraction(node, ext)
	case schemas.EntityTypeEvent:
		eventNodeToExtraction(node, ext)
	default:
		// news + article share the article field set.
		articleNodeToExtraction(node, ext)
	}

	if _, ok := ext["title"]; !ok {
		return nil
	}

	return ext
}

// articleNodeToExtraction maps NewsArticle/Article/BlogPosting/FAQPage
// nodes onto the shared article/news field set.
func articleNodeToExtraction(node map[string]any, ext extractor.Extraction) {
	title := ldjsonString(node, "headline", "name")
	field(ext, "title", title, title)

	if s := ldjsonString(node, "description"); s != "" {
		field(ext, "summary", stripHTMLTags(s), s)
	}

	if s := ldjsonString(node, "articleBody"); s != "" {
		field(ext, "body", s, s)
	}

	if d := ldjsonDate(ldjsonString(node, "datePublished", "dateCreated")); d != "" {
		field(ext, "published_date", d, ldjsonString(node, "datePublished", "dateCreated"))
	}

	if u := ldjsonURL(node); u != "" {
		field(ext, "article_url", u, u)
	}

	if author := ldjsonAuthor(node["author"]); author != "" {
		field(ext, "author", author, author)
	}

	if topics := ldjsonStrings(node["keywords"]); len(topics) > 0 {
		field(ext, "topics", topics, strings.Join(topics, ", "))
	} else if s := ldjsonString(node, "articleSection"); s != "" {
		field(ext, "topics", []string{s}, s)
	}

	if img := ldjsonImage(node["image"]); img != "" {
		field(ext, "image_url", img, img)
	}

	// FAQPage: the Q&A pairs become the article body so the record carries
	// the actual content, not just the page title.
	if qa := ldjsonFAQBody(node["mainEntity"]); qa != "" {
		if _, hasBody := ext["body"]; !hasBody {
			field(ext, "body", qa, qa)
		}
	}
}

// jobNodeToExtraction maps a JobPosting node onto the job schema.
func jobNodeToExtraction(node map[string]any, ext extractor.Extraction) {
	title := ldjsonString(node, "title", "name")
	field(ext, "title", title, title)

	if s := ldjsonString(node, "description"); s != "" {
		field(ext, "description_summary", stripHTMLTags(s), s)
	}

	if org := ldjsonNestedName(node["hiringOrganization"]); org != "" {
		field(ext, "employer", org, org)
	}

	if loc, ok := node["jobLocation"].(map[string]any); ok {
		if addr, ok := loc["address"].(map[string]any); ok {
			if city := ldjsonString(addr, "addressLocality"); city != "" {
				field(ext, "city", city, city)
			}
			if country := ldjsonString(addr, "addressCountry", "addressCountry.name"); country != "" {
				field(ext, "country", []string{country}, country)
			}
		} else if s := ldjsonString(loc, "name"); s != "" {
			field(ext, "city", s, s)
		}
	}

	if jt := ldjsonEmploymentType(node["employmentType"]); jt != "" {
		field(ext, "job_type", jt, jt)
	}

	if salary, ok := node["baseSalary"].(map[string]any); ok {
		if v, ok := salary["value"].(map[string]any); ok {
			if n, ok := v["minValue"].(float64); ok {
				field(ext, "salary_min", n, fmt.Sprint(n))
			}
			if n, ok := v["maxValue"].(float64); ok {
				field(ext, "salary_max", n, fmt.Sprint(n))
			}
			if n, ok := v["value"].(float64); ok {
				field(ext, "salary_min", n, fmt.Sprint(n))
			}
		} else if n, ok := salary["value"].(float64); ok {
			field(ext, "salary_min", n, fmt.Sprint(n))
		}
		if c := ldjsonString(salary, "currency"); c != "" {
			field(ext, "currency", c, c)
		}
	}

	if d := ldjsonDate(ldjsonString(node, "validThrough")); d != "" {
		field(ext, "deadline", d, ldjsonString(node, "validThrough"))
	}

	if u := ldjsonURL(node); u != "" {
		field(ext, "application_url", u, u)
	}

	// jobLocationType "TELECOMMUTE" (or applicantLocationRequirements) marks
	// the job remote; the schema models remote as a bool-as-enum string.
	if lt, _ := node["jobLocationType"].(string); strings.EqualFold(lt, "TELECOMMUTE") {
		field(ext, "remote", "true", lt)
	}
}

// eventNodeToExtraction maps an Event node onto the event schema.
func eventNodeToExtraction(node map[string]any, ext extractor.Extraction) {
	title := ldjsonString(node, "name", "headline")
	field(ext, "title", title, title)

	if s := ldjsonString(node, "description"); s != "" {
		field(ext, "description", stripHTMLTags(s), s)
	}

	if s := ldjsonString(node, "startDate"); s != "" {
		field(ext, "starts_at", s, s)
	}
	if s := ldjsonString(node, "endDate"); s != "" {
		field(ext, "ends_at", s, s)
	}

	if loc := ldjsonLocation(node["location"]); loc != "" {
		field(ext, "location", loc, loc)
	}

	if u := ldjsonURL(node); u != "" {
		field(ext, "url", u, u)
	}

	if org := ldjsonNestedName(node["organizer"]); org != "" {
		field(ext, "organizer", org, org)
	}
}

// ldjsonKey derives the node's stable external key: identifier (string,
// PropertyValue.value, or array member) → url / mainEntityOfPage → @id →
// a content hash of @type + name + start/publish date.
func ldjsonKey(node map[string]any, entityType string) string {
	if id := ldjsonIdentifier(node["identifier"]); id != "" {
		return entityType + ":" + id
	}
	if u := ldjsonURL(node); u != "" {
		return u
	}
	if id := ldjsonString(node, "@id"); id != "" {
		return id
	}

	return "hash:" + hashKey(
		entityType,
		ldjsonString(node, "name", "headline", "title"),
		ldjsonString(node, "startDate", "datePublished"),
	)
}

// ---------------------------------------------------------------------------
// ld+json field accessors — schema.org values are loosely typed (string or
// object or array), so every accessor tolerates all three shapes.
// ---------------------------------------------------------------------------

// ldjsonStrings flattens a string-or-[]string JSON value.
func ldjsonStrings(v any) []string {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) != "" {
			return []string{t}
		}
	case []any:
		out := make([]string, 0, len(t))
		for _, el := range t {
			if s, ok := el.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}

		return out
	}

	return nil
}

// ldjsonString returns the first non-empty string among the named keys.
// A dotted key ("a.b") walks one level into a nested object.
func ldjsonString(node map[string]any, keys ...string) string {
	for _, k := range keys {
		if name, sub, ok := strings.Cut(k, "."); ok {
			if nested, ok := node[name].(map[string]any); ok {
				if s, _ := nested[sub].(string); strings.TrimSpace(s) != "" {
					return s
				}
			}
			continue
		}
		if s, _ := node[k].(string); strings.TrimSpace(s) != "" {
			return s
		}
	}

	return ""
}

// ldjsonNestedName returns the .name of an object (or the first array
// member's), tolerating a bare string value.
func ldjsonNestedName(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if s, ok := t["name"].(string); ok {
			return s
		}
	case []any:
		for _, el := range t {
			if s := ldjsonNestedName(el); s != "" {
				return s
			}
		}
	}

	return ""
}

// ldjsonAuthor flattens author (string | Person object | array) to a name.
func ldjsonAuthor(v any) string { return ldjsonNestedName(v) }

// ldjsonLocation renders an Event location (string | Place object) as a
// single display string.
func ldjsonLocation(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		parts := []string{}
		if s := ldjsonNestedName(t); s != "" {
			parts = append(parts, s)
		}
		if addr, ok := t["address"].(map[string]any); ok {
			city := ldjsonString(addr, "addressLocality")
			country := ldjsonString(addr, "addressCountry", "addressCountry.name")
			parts = append(parts, firstNonEmpty(city, country))
		} else if s, _ := t["address"].(string); s != "" {
			parts = append(parts, s)
		}

		return strings.Join(parts, ", ")
	}

	return ""
}

// ldjsonURL returns the node's canonical URL: url → mainEntityOfPage
// (string or @id object).
func ldjsonURL(node map[string]any) string {
	if s := ldjsonString(node, "url"); s != "" {
		return s
	}

	switch t := node["mainEntityOfPage"].(type) {
	case string:
		return t
	case map[string]any:
		return ldjsonString(t, "@id")
	}

	return ""
}

// ldjsonIdentifier flattens an identifier (string | PropertyValue object |
// array) to a usable string.
func ldjsonIdentifier(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		return ldjsonString(t, "value", "propertyID", "@id")
	case []any:
		for _, el := range t {
			if s := ldjsonIdentifier(el); s != "" {
				return s
			}
		}
	}

	return ""
}

// ldjsonImage flattens image (string | ImageObject | array) to a URL.
func ldjsonImage(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		return ldjsonString(t, "url", "@id")
	case []any:
		for _, el := range t {
			if s := ldjsonImage(el); s != "" {
				return s
			}
		}
	}

	return ""
}

// ldjsonEmploymentType maps schema.org employmentType values
// (FULL_TIME, PART_TIME, CONTRACTOR, INTERN, TEMPORARY — string or array)
// onto the job schema's job_type enum.
func ldjsonEmploymentType(v any) string {
	for _, s := range ldjsonStrings(v) {
		switch strings.ToUpper(s) {
		case "FULL_TIME":
			return "full_time"
		case "PART_TIME":
			return "part_time"
		case "CONTRACT", "CONTRACTOR":
			return "contract"
		case "INTERN", "INTERNSHIP":
			return "internship"
		case "TEMPORARY":
			return "temporary"
		case "REMOTE", "TELECOMMUTE":
			return "remote"
		}
	}

	return ""
}

// ldjsonFAQBody renders a FAQPage mainEntity (Question objects with
// acceptedAnswer.text) as a plain-text Q&A body.
func ldjsonFAQBody(v any) string {
	var b strings.Builder

	walk := func(node map[string]any) {
		q := ldjsonString(node, "name")
		var a string
		if ans, ok := node["acceptedAnswer"].(map[string]any); ok {
			a = ldjsonString(ans, "text")
		}
		if q != "" && a != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			fmt.Fprintf(&b, "Q: %s\nA: %s", q, stripHTMLTags(a))
		}
	}

	switch t := v.(type) {
	case map[string]any:
		walk(t)
	case []any:
		for _, el := range t {
			if m, ok := el.(map[string]any); ok {
				walk(m)
			}
		}
	}

	return b.String()
}

// ldjsonDate normalizes a schema.org date/datetime string to YYYY-MM-DD
// for the "date" field coercer. Unparseable values return "".
func ldjsonDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return t.Format("2006-01-02")
		}
	}

	return parseFeedDate(s)
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}

	return false
}
