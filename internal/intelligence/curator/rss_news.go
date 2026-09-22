package curator

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// RSSNews curates rss crawl-method channels: the raw doc body is the feed
// XML and every <item> (RSS 2.0) or <entry> (Atom) becomes one record.
// Fields map onto the shared news/article field set (title, summary,
// published_date, article_url, author, topics) — the record's entity type
// is the channel's declared entity_types[0] (news feeds → news, blogs →
// article), so a single curator serves both categories.
type RSSNews struct{}

// Handles returns the crawl method this curator serves.
func (RSSNews) Handles() string { return string(sources.CrawlMethodRSS) }

// rssFeedDoc / atomFeedDoc mirror the two feed shapes. The extractor's
// rule-based RSS parser is not reused: it only keeps the newest item for
// the single-record extract path, while curation needs every item plus the
// guid/content:encoded fields it doesn't model.
type rssFeedDoc struct {
	Items []rssItem `xml:"channel>item"`
}

type rssItem struct {
	Title          string   `xml:"title"`
	Link           string   `xml:"link"`
	Description    string   `xml:"description"`
	PubDate        string   `xml:"pubDate"`
	Author         string   `xml:"author"`
	GUID           string   `xml:"guid"`
	DCCreator      string   `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Categories     []string `xml:"category"`
	ContentEncoded string   `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

type atomFeedDoc struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title      string     `xml:"title"`
	ID         string     `xml:"id"`
	Updated    string     `xml:"updated"`
	Published  string     `xml:"published"`
	Summary    string     `xml:"summary"`
	Content    string     `xml:"content"`
	Author     atomAuthor `xml:"author"`
	Links      []atomLink `xml:"link"`
	Categories []atomCat  `xml:"category"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomCat struct {
	Term string `xml:"term,attr"`
}

// Curate parses the feed body and returns one record per item/entry. RSS
// 2.0 (<rss><channel><item>) is tried first; a body with no items falls
// through to the Atom (<feed><entry>) shape.
func (RSSNews) Curate(_ context.Context, body []byte, _ string, ch sources.Channel) ([]Record, error) {
	return curateFeedItems(body, ch)
}

// curateFeedItems is the shared feed→records pipeline: RSS items first,
// Atom entries on fallback. WebSub deliveries are the same shapes, so the
// webhook curator delegates here.
func curateFeedItems(body []byte, ch sources.Channel) ([]Record, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, errors.New("curator: rss: empty feed body")
	}

	var feed rssFeedDoc
	if err := xml.Unmarshal(body, &feed); err == nil && len(feed.Items) > 0 {
		entityType := resolveEntityType("", ch)
		out := make([]Record, 0, len(feed.Items))

		for _, it := range feed.Items {
			ext := rssItemToExtraction(it)
			out = append(out, Record{
				Extraction:  validate(ext, entityType),
				ExternalKey: rssItemKey(it),
				EntityType:  entityType,
			})
		}

		return out, nil
	}

	// Atom feeds carry a <feed> root instead of <rss><channel>.
	return curateAtom(body, ch)
}

// curateAtom handles Atom feeds (entry elements instead of item).
func curateAtom(body []byte, ch sources.Channel) ([]Record, error) {
	var feed atomFeedDoc
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("curator: rss: parse atom feed: %w", err)
	}
	if len(feed.Entries) == 0 {
		return nil, errors.New("curator: rss: atom feed has no entries")
	}

	entityType := resolveEntityType("", ch)
	out := make([]Record, 0, len(feed.Entries))

	for _, e := range feed.Entries {
		ext := make(extractor.Extraction)

		field(ext, "title", e.Title, e.Title)
		if summary := stripHTMLTags(firstNonEmpty(e.Summary, e.Content)); summary != "" {
			field(ext, "summary", summary, firstNonEmpty(e.Summary, e.Content))
		}
		if body := stripHTMLTags(e.Content); body != "" {
			field(ext, "body", body, e.Content)
		}
		if iso := parseFeedDate(firstNonEmpty(e.Published, e.Updated)); iso != "" {
			field(ext, "published_date", iso, firstNonEmpty(e.Published, e.Updated))
		}
		if link := atomEntryLink(e); link != "" {
			field(ext, "article_url", link, link)
		}
		if e.Author.Name != "" {
			field(ext, "author", e.Author.Name, e.Author.Name)
		}
		var topics []string
		for _, c := range e.Categories {
			if c.Term != "" {
				topics = append(topics, c.Term)
			}
		}
		if len(topics) > 0 {
			field(ext, "topics", topics, strings.Join(topics, ", "))
		}

		key := e.ID
		if key == "" {
			key = atomEntryLink(e)
		}
		if key == "" {
			key = "hash:" + hashKey(e.Title, e.Published, e.Updated)
		}

		out = append(out, Record{
			Extraction:  validate(ext, entityType),
			ExternalKey: key,
			EntityType:  entityType,
		})
	}

	return out, nil
}

// atomEntryLink returns the entry's rel=alternate (or first) link href.
func atomEntryLink(e atomEntry) string {
	fallback := ""
	for _, l := range e.Links {
		if l.Href == "" {
			continue
		}
		if l.Rel == "alternate" || l.Rel == "" {
			return l.Href
		}
		fallback = l.Href
	}

	return fallback
}

// rssItemToExtraction maps one RSS item onto the shared news/article field
// set. Mirrors the extractor's rule-based mapping but keeps every item and
// the content:encoded body for the article schema.
func rssItemToExtraction(it rssItem) extractor.Extraction {
	ext := make(extractor.Extraction)

	field(ext, "title", it.Title, it.Title)

	if summary := stripHTMLTags(it.Description); summary != "" {
		field(ext, "summary", summary, it.Description)
	}

	if body := stripHTMLTags(it.ContentEncoded); body != "" {
		field(ext, "body", body, it.ContentEncoded)
	}

	if iso := parseFeedDate(it.PubDate); iso != "" {
		field(ext, "published_date", iso, it.PubDate)
	}

	if it.Link != "" {
		field(ext, "article_url", it.Link, it.Link)
	}

	if author := extractAuthorName(firstNonEmpty(it.DCCreator, it.Author)); author != "" {
		field(ext, "author", author, firstNonEmpty(it.DCCreator, it.Author))
	}

	var topics []string
	for _, c := range it.Categories {
		if c = strings.TrimSpace(c); c != "" {
			topics = append(topics, c)
		}
	}
	if len(topics) > 0 {
		field(ext, "topics", topics, strings.Join(topics, ", "))
	}

	return ext
}

// rssItemKey derives the item's stable external key: guid first, then the
// link, then a content hash of title+pubDate so GUID-less feeds still
// dedupe across re-crawls.
func rssItemKey(it rssItem) string {
	if s := strings.TrimSpace(it.GUID); s != "" {
		return s
	}
	if s := strings.TrimSpace(it.Link); s != "" {
		return s
	}

	return "hash:" + hashKey(it.Title, it.PubDate)
}

// parseFeedDate normalizes common RSS/Atom date formats to ISO 8601
// (YYYY-MM-DD — the date coercer's accepted form).
func parseFeedDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

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
			return t.Format("2006-01-02")
		}
	}

	return ""
}

// stripHTMLTags removes HTML tags and collapses whitespace. Feed
// description/content fields often embed markup.
func stripHTMLTags(s string) string {
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

	return strings.TrimSpace(strings.Join(strings.Fields(result.String()), " "))
}

// extractAuthorName pulls the display name out of an RSS author field —
// the common shape is "email@example.com (Name)".
func extractAuthorName(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "("); idx >= 0 {
		if end := strings.Index(s, ")"); end > idx {
			return strings.TrimSpace(s[idx+1 : end])
		}
	}
	// A bare email carries no usable display name.
	if strings.Contains(s, "@") && !strings.Contains(s, " ") {
		return ""
	}

	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}

	return ""
}
