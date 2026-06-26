package blog

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"

	"github.com/jedafe/japa/internal/db/gen"
)

// RSSFeed is the top-level XML structure for an RSS 2.0 feed.
type RSSFeed struct {
	XMLName      xml.Name  `xml:"rss"`
	Version      string    `xml:"version,attr"`
	Channel      RSSChannel `xml:"channel"`
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

// generateRSSXML writes an RSS 2.0 XML feed to the given writer.
func generateRSSXML(w io.Writer, posts []gen.ListPublishedPostsForRSSRow) error {
	channel := RSSChannel{
		Title:       "JapaConnect Blog",
		Link:        "https://japaconnect.com/blog",
		Description: "Insights and stories from the Nigerian diaspora community",
		Language:    "en-us",
		Items:       make([]RSSItem, 0, len(posts)),
	}

	for _, p := range posts {
		pubDate := p.PublishedAt.Time.Format(time.RFC1123Z)
		if pubDate == "" {
			pubDate = p.CreatedAt.Time.Format(time.RFC1123Z)
		}

		guid := fmt.Sprintf("urn:uuid:%s", p.ID.String())

		item := RSSItem{
			Title:       p.Title,
			Link:        fmt.Sprintf("https://japaconnect.com/blog/%s", p.Slug),
			GUID:        guid,
			Description: p.BodyHtml,
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
