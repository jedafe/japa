package blog

import (
	"io"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rss/model"
)

// RSS 2.0 XML model — aliases re-exported from the shared internal/rss/model
// subpackage (RSS-011). Previously this file defined its own copies, which
// caused RSS-003 (the blog copy forgot the Valid check that the rss copy had).

// RSSFeed is the top-level XML structure for an RSS 2.0 feed.
type RSSFeed = model.Feed

// RSSChannel contains metadata and items for the feed.
type RSSChannel = model.Channel

// RSSItem represents a single entry in the feed.
type RSSItem = model.Item

// generateRSSXML writes an RSS 2.0 XML feed to the given writer.
func generateRSSXML(w io.Writer, posts []gen.ListPublishedPostsForRSSRow, siteURL string, siteName string) error {
	items := make([]model.Item, 0, len(posts))

	for _, p := range posts {
		// RSS-003: model.FormatTime checks Valid+IsZero — time.Time{}.
		// Format(RFC1123Z) is "Mon, 01 Jan 0001 ...", not empty. The old
		// `if pubDate == ""` check never fired, so a zero/NULL
		// published_at produced a bogus year-0001 pubDate.
		pubDate := model.FormatTime(p.PublishedAt)
		if pubDate == "" {
			pubDate = model.FormatTime(p.CreatedAt)
		}

		// M4: use a plain-text excerpt (HTML stripped, truncated to 300
		// runes) instead of raw body_html — avoids XSS-in-feed and
		// malformed-feeling feeds in RSS readers. Fall back to a body
		// excerpt for posts with no body_html (e.g. Tiptap JSON bodies
		// that haven't been rendered yet).
		desc := model.HTMLToTextExcerpt(p.BodyHtml, model.ExcerptMaxRunes)
		if desc == "" && p.Body != "" {
			desc = model.ExcerptFromBody(p.Body)
		}

		items = append(items, model.Item{
			Title:       p.Title,
			Link:        siteURL + "/blog/" + p.Slug,
			GUID:        model.GUIDFromID(p.ID),
			Description: desc,
			PubDate:     pubDate,
		})
	}

	xmlBytes := model.Render(
		siteName+" Blog",
		siteURL+"/blog",
		siteName+" Blog — latest posts",
		"en-us",
		items,
	)
	if xmlBytes == nil {
		return nil
	}

	// model.Render already includes the XML header; write directly.
	_, err := w.Write(xmlBytes)
	return err
}
