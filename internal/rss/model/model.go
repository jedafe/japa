// Package model provides the shared RSS 2.0 XML model and rendering
// helpers used by internal/rss (live HTTP handlers), internal/blog (blog
// feed), and internal/worker (rss:regenerate background task).
//
// This subpackage intentionally has NO dependency on chi, tenant, net/http,
// or the internal/rss handlers package — only on the standard library,
// pgtype (for timestamp formatting), and slog (for error logging). This
// lets the background worker import it without pulling in the HTTP stack.
//
// Ref: japaconnect-engineering/sections/26-seo-performance.md
package model

import (
	"encoding/xml"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Feed is the top-level RSS 2.0 envelope.
type Feed struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

// Channel groups feed metadata and items.
type Channel struct {
	Title         string `xml:"title"`
	Link          string `xml:"link"`
	Description   string `xml:"description"`
	Language      string `xml:"language"`
	LastBuildDate string `xml:"lastBuildDate"`
	Items         []Item `xml:"item"`
}

// Item represents a single entry in the feed.
type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author,omitempty"`
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
		GUID:        GUIDFromID(id),
		Description: desc,
		PubDate:     pubDate,
		Author:      username,
	}
}

// Render marshals a feed to indented XML bytes with the XML declaration.
// lastBuildDate defaults to the most recent item's PubDate, or now if there
// are no items. Returns nil on marshal error (logged via slog).
func Render(title, link, description, language string, items []Item) []byte {
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
		Version: "2.0",
		Channel: Channel{
			Title:         title,
			Link:          link,
			Description:   description,
			Language:      language,
			LastBuildDate: lastBuild,
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
