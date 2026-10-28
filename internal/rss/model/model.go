// Package model provides the shared RSS 2.0 XML model and rendering
// helpers used by internal/rss (live HTTP handlers), internal/blog (blog
// feed), and internal/worker (rss:regenerate background task).
//
// This subpackage intentionally has NO dependency on chi, platform, net/http,
// or the internal/rss handlers package — only on the standard library,
// pgtype (for timestamp formatting), and slog (for error logging). This
// lets the background worker import it without pulling in the HTTP stack.
//
// Ref: japaconnect-engineering/sections/26-seo-performance.md
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Feed is the top-level RSS 2.0 envelope.
type Feed struct {
	XMLName   xml.Name `xml:"rss"`
	Version   string   `xml:"version,attr"`
	XMLNSAtom string   `xml:"xmlns:atom,attr"`
	XMLNSDC   string   `xml:"xmlns:dc,attr"`
	Channel   Channel  `xml:"channel"`
}

// AtomLink is the channel's self-referencing link — required by strict RSS
// validators and used by readers to re-resolve the feed URL.
type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

// guidXML carries the GUID value plus its isPermaLink attribute. RSS 2.0
// defaults isPermaLink to true — a urn:uuid: GUID rendered without the
// attribute makes readers treat the URN as a resolvable URL.
type guidXML struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

// NewGUID builds a guid element. URL GUIDs are permalinks; opaque strings
// like urn:uuid: are not — callers must say which.
func NewGUID(value string, isPermaLink bool) guidXML {
	return guidXML{IsPermaLink: isPermaLink, Value: value}
}

// Channel groups feed metadata and items.
type Channel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language"`
	LastBuildDate string    `xml:"lastBuildDate"`
	TTL           int       `xml:"ttl"`
	Docs          string    `xml:"docs"`
	Generator     string    `xml:"generator"`
	AtomLink      *AtomLink `xml:"atom:link"`
	Items         []Item    `xml:"item"`
}

// Item represents a single entry in the feed.
type Item struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        guidXML `xml:"guid"`
	Description string  `xml:"description"`
	PubDate     string  `xml:"pubDate"`
	Creator     string  `xml:"dc:creator,omitempty"`
	Category    string  `xml:"category,omitempty"`
}

// ExcerptMaxRunes is the maximum number of runes in an item description.
const ExcerptMaxRunes = 300

// htmlTagPattern matches HTML/XML tags for stripping to plain text.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// HTMLToTextExcerpt strips HTML tags from bodyHtml, collapses whitespace,
// and truncates to maxRunes characters, returning a clean plain-text excerpt
// suitable for an RSS <description> element.
func HTMLToTextExcerpt(bodyHtml string, maxRunes int) string {
	plain := htmlTagPattern.ReplaceAllString(bodyHtml, "")
	plain = strings.TrimSpace(plain)
	plain = strings.Join(strings.Fields(plain), " ")

	if len([]rune(plain)) <= maxRunes {
		return plain
	}

	runes := []rune(plain)

	return string(runes[:maxRunes]) + "…"
}

// ExcerptFromBody extracts a plain-text excerpt from raw markdown body
// (fallback when body_html is empty). Truncates to ExcerptMaxRunes runes.
func ExcerptFromBody(body string) string {
	plain := strings.TrimSpace(body)
	plain = strings.Join(strings.Fields(plain), " ")

	if len([]rune(plain)) <= ExcerptMaxRunes {
		return plain
	}

	runes := []rune(plain)

	return string(runes[:ExcerptMaxRunes]) + "…"
}

// FormatTime formats a pgtype.Timestamptz as RFC1123Z for an RSS pubDate.
// Returns an empty string when the timestamp is invalid (NULL/zero),
// so callers can fall back to another timestamp field by checking for "".
func FormatTime(t pgtype.Timestamptz) string {
	if t.Valid && !t.Time.IsZero() {
		return t.Time.Format(time.RFC1123Z)
	}

	return ""
}

// contentRoutePrefix maps a content_type to its canonical public detail
// route prefix (ADR-058 route families). Mirrors
// apps/main/src/lib/utils/contentLink.ts — unknown types fall back to the
// generic /content/ route, which 301s to the canonical URL.
var contentRoutePrefix = map[string]string{
	"blog_post":       "/blog/",
	"guide":           "/guide/",
	"discussion":      "/forum/",
	"forum_thread":    "/forum/", // legacy alias for 'discussion'
	"comparison":      "/comparisons/",
	"story":           "/story/",
	"achievement":     "/achievements/",
	"page":            "/pages/",
	"job_listing":     "/jobs/",
	"service_listing": "/services/",
	"video":           "/video/", // §106 video posts live at /video/{slug}
	"list":            "/list/",  // §14 list amendment
}

// LinkPathFor returns the canonical route prefix (with trailing slash) for a
// content_type so feed item <link>s emit canonical URLs instead of the
// redirect-shim /content/{slug} path.
func LinkPathFor(contentType string) string {
	if prefix, ok := contentRoutePrefix[contentType]; ok {
		return prefix
	}

	return "/content/"
}

// TypeFeed describes a per-content-type feed surface at /{prefix}/feed.xml.
type TypeFeed struct {
	Prefix      string // route prefix, e.g. "story" → /story/feed.xml
	ContentType string // contents.content_type filter
	Label       string // feed title suffix, e.g. "Stories" → "{site} — Stories"
}

// typeFeeds is the registry of per-type feeds — every ADR-058 route family
// that publishes long-form content gets one, alongside the universal, blog,
// and forum feeds. Blog and forum are excluded: they have dedicated feeds
// with their own queries (/blog/feed.xml, /forum/feed.xml). page and
// achievement are excluded as non-feed content types.
var typeFeeds = []TypeFeed{
	{Prefix: "story", ContentType: "story", Label: "Stories"},
	{Prefix: "guide", ContentType: "guide", Label: "Guides"},
	{Prefix: "comparisons", ContentType: "comparison", Label: "Comparisons"},
	{Prefix: "jobs", ContentType: "job_listing", Label: "Jobs"},
	{Prefix: "services", ContentType: "service_listing", Label: "Services"},
	{Prefix: "video", ContentType: "video", Label: "Videos"},
	{Prefix: "list", ContentType: "list", Label: "Lists"},
}

// TypeFeeds returns the per-type feed registry — handlers mount a route per
// entry and the rss:regenerate worker warms a cache key per entry.
func TypeFeeds() []TypeFeed {
	out := make([]TypeFeed, len(typeFeeds))
	copy(out, typeFeeds)

	return out
}

// TypeFeedForPrefix resolves a route prefix to its TypeFeed, or false.
func TypeFeedForPrefix(prefix string) (TypeFeed, bool) {
	for _, tf := range typeFeeds {
		if tf.Prefix == prefix {
			return tf, true
		}
	}

	return TypeFeed{}, false
}

// GUIDFromID returns a URN GUID string from a content UUID, or "" when
// the UUID is invalid.
func GUIDFromID(id pgtype.UUID) string {
	if id.Valid {
		return "urn:uuid:" + id.String()
	}

	return ""
}

// ItemFromRow builds an Item from the common RSS row fields shared by all
// ListRSS*FeedItems queries. PubDate prefers publishedAt, falling back to
// createdAt. Description strips HTML from bodyHtml, falling back to a
// plain-text excerpt from body.
func ItemFromRow(
	id pgtype.UUID,
	title, slug, bodyHtml, body, username string,
	publishedAt, createdAt pgtype.Timestamptz,
	siteURL, linkPath string,
) Item {
	pubDate := FormatTime(publishedAt)
	if pubDate == "" {
		pubDate = FormatTime(createdAt)
	}

	desc := HTMLToTextExcerpt(bodyHtml, ExcerptMaxRunes)
	if desc == "" && body != "" {
		desc = ExcerptFromBody(body)
	}

	return Item{
		Title:       title,
		Link:        siteURL + linkPath + slug,
		GUID:        NewGUID(GUIDFromID(id), false), // urn:uuid: is not a URL
		Description: desc,
		PubDate:     pubDate,
		Creator:     username,
	}
}

// ETag returns a weak ETag for a rendered feed body so readers can poll with
// If-None-Match and get a cheap 304 instead of the full payload. All feed
// handlers share this so a given body produces the same tag everywhere.
func ETag(body []byte) string {
	sum := sha256.Sum256(body)

	return `W/"` + hex.EncodeToString(sum[:])[:24] + `"`
}

// IfNoneMatch reports whether an If-None-Match header value satisfies the
// given entity tag. RFC 7232 mandates WEAK comparison here — W/"x" and "x"
// match — so the W/ prefix is stripped before comparing, alongside the
// wildcard (*) and comma-separated list forms.
func IfNoneMatch(header, etag string) bool {
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(header, ",") {
		c := strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if c == want || c == "*" {
			return true
		}
	}

	return false
}

// Render marshals a feed to indented XML bytes with the XML declaration.
// lastBuildDate defaults to the most recent item's PubDate, or now if there
// are no items. selfLink is the feed's own canonical URL, emitted as
// <atom:link rel="self"> (validator-required). Returns nil on marshal error
// (logged via slog).
func Render(title, link, description, language, selfLink string, items []Item) []byte {
	var lastBuild string
	if len(items) > 0 && items[0].PubDate != "" {
		lastBuild = items[0].PubDate
	} else {
		lastBuild = time.Now().UTC().Format(time.RFC1123Z)
	}

	if language == "" {
		language = "en"
	}

	feed := Feed{
		Version:   "2.0",
		XMLNSAtom: "http://www.w3.org/2005/Atom",
		XMLNSDC:   "http://purl.org/dc/elements/1.1/",
		Channel: Channel{
			Title:         title,
			Link:          link,
			Description:   description,
			Language:      language,
			LastBuildDate: lastBuild,
			TTL:           15, // minutes — matches Cache-Control max-age=900 (§26)
			Docs:          "https://cyber.harvard.edu/rss/rss.html",
			Generator:     "JapaConnect",
			AtomLink:      &AtomLink{Href: selfLink, Rel: "self", Type: "application/rss+xml"},
			Items:         items,
		},
	}

	xmlBytes, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		slog.Error("rss/model: marshal xml", "error", err)
		return nil
	}

	return append([]byte(xml.Header), xmlBytes...)
}
