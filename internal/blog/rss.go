package blog

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/db/gen"
)

// RSSFeed is the top-level XML structure for an RSS 2.0 feed.
type RSSFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel RSSChannel `xml:"channel"`
}

// RSSChannel contains metadata and items for the feed.
type RSSChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []RSSItem `xml:"item"`
}

// RSSItem represents a single entry in the feed.
type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author,omitempty"`
}

// htmlTagPattern matches HTML/XML tags for stripping to plain text.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// htmlToTextExcerpt strips HTML tags from bodyHtml, collapses whitespace,
// and truncates to maxRunes characters, returning a clean plain-text excerpt
// suitable for an RSS <description> element (M4).
func htmlToTextExcerpt(bodyHtml string, maxRunes int) string {
	plain := htmlTagPattern.ReplaceAllString(bodyHtml, "")
	plain = strings.TrimSpace(plain)
	// Collapse runs of whitespace into single spaces.
	plain = strings.Join(strings.Fields(plain), " ")
	if len([]rune(plain)) <= maxRunes {
		return plain
	}
	runes := []rune(plain)
	return string(runes[:maxRunes]) + "…"
}

// generateRSSXML writes an RSS 2.0 XML feed to the given writer.
func generateRSSXML(w io.Writer, posts []gen.ListPublishedPostsForRSSRow, siteURL string, siteName string) error {
	// Use the most recent post's published_at as lastBuildDate.
	var lastBuild string
	if len(posts) > 0 {
		t := posts[0].PublishedAt.Time
		if t.IsZero() {
			t = posts[0].CreatedAt.Time
		}
		if !t.IsZero() {
			lastBuild = t.Format(time.RFC1123Z)
		}
	}

	channel := RSSChannel{
		Title:         siteName + " Blog",
		Link:          siteURL + "/blog",
		Description:   "Insights and stories from the Nigerian diaspora community",
		Language:      "en-us",
		LastBuildDate: lastBuild,
		Items:         make([]RSSItem, 0, len(posts)),
	}

	for _, p := range posts {
		pubDate := p.PublishedAt.Time.Format(time.RFC1123Z)
		if pubDate == "" {
			pubDate = p.CreatedAt.Time.Format(time.RFC1123Z)
		}

		guid := "urn:uuid:" + p.ID.String()

		// M4: use a plain-text excerpt (HTML stripped, truncated to 300
		// runes) instead of raw body_html — avoids XSS-in-feed and
		// malformed-feeling feeds in RSS readers.
		// Fall back to excerptFromBody for posts with no body_html (e.g.
		// Tiptap JSON bodies that haven't been rendered yet).
		desc := htmlToTextExcerpt(p.BodyHtml, 300)
		if desc == "" && p.Body != "" {
			desc = excerptFromBody(p.Body)
		}

		item := RSSItem{
			Title:       p.Title,
			Link:        siteURL + "/blog/" + p.Slug,
			GUID:        guid,
			Description: desc,
			PubDate:     pubDate,
		}
		channel.Items = append(channel.Items, item)
	}

	feed := RSSFeed{
		Version: "2.0",
		Channel: channel,
	}

	_, err := fmt.Fprintf(w, xml.Header)
	if err != nil {
		return fmt.Errorf("writing XML header: %w", err)
	}

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")

	if err := enc.Encode(feed); err != nil {
		return fmt.Errorf("encoding RSS XML: %w", err)
	}

	return nil
}
