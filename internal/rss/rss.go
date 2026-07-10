// Package rss provides RSS feed handlers for JapaConnect content.
//
// The handlers query published content from the database and render
// standard RSS 2.0 XML. All feeds are public (no auth required) and
// tenant-scoped via the tenant middleware.
package rss

import (
	"encoding/xml"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// ---------------------------------------------------------------------------
// XML model
// ---------------------------------------------------------------------------

// RSSFeed is the top-level RSS 2.0 envelope.
type RSSFeed struct { //nolint:revive // stuttering type name preserved for API compatibility
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel RSSChannel `xml:"channel"`
}

// RSSChannel groups feed metadata and items.
type RSSChannel struct { //nolint:revive // stuttering type name preserved for API compatibility
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language,omitempty"`
	Items       []RSSItem `xml:"item"`
}

// RSSItem represents a single entry in the feed.
type RSSItem struct { //nolint:revive // stuttering type name preserved for API compatibility
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author,omitempty"`
	GUID        string `xml:"guid,omitempty"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// HandleFeed serves the universal feed (/feed.xml) and blog-only feed
// (/blog/feed.xml). The feedType parameter selects the content filter:
// "all" returns all published content, "blog" returns only blog posts.
func HandleFeed(q *gen.Queries, feedType string, siteURL string, siteName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant.IDFromContext(r.Context())
		if tenantID == (pgtype.UUID{}) {
			http.Error(w, "tenant unavailable", http.StatusServiceUnavailable)
			return
		}

		var items []RSSItem
		var err error

		switch feedType {
		case "blog":
			rows, qErr := q.ListRSSFeedItemsByContentType(r.Context(), gen.ListRSSFeedItemsByContentTypeParams{
				TenantID:    tenantID,
				ContentType: "blog_post",
			})
			if qErr != nil {
				err = qErr
			} else {
				items = feedItemsFromContentTypeRows(rows, siteURL)
			}
		default:
			rows, qErr := q.ListRSSFeedItems(r.Context(), tenantID)
			if qErr != nil {
				err = qErr
			} else {
				items = feedItemsFromAllRows(rows, siteURL)
			}
		}

		if err != nil {
			slog.Error("rss: query feed", "feed_type", feedType, "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)

			return
		}

		serveRSSFeed(w, siteName, siteURL,
			"Community platform for the Nigerian diaspora", items)
	}
}

// HandleSpaceFeed serves a space-specific RSS feed at /spaces/{slug}/feed.xml.
func HandleSpaceFeed(q *gen.Queries, siteURL string, siteName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		if slug == "" {
			http.Error(w, "missing space slug", http.StatusBadRequest)
			return
		}

		tenantID := tenant.IDFromContext(r.Context())
		if tenantID == (pgtype.UUID{}) {
			http.Error(w, "tenant unavailable", http.StatusServiceUnavailable)
			return
		}

		rows, err := q.ListRSSSpaceFeedItems(r.Context(), gen.ListRSSSpaceFeedItemsParams{
			TenantID: tenantID,
			Slug:     slug,
		})
		if err != nil {
			slog.Error("rss: query space feed", "slug", slug, "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)

			return
		}

		items := feedItemsFromSpaceRows(rows, siteURL)
		serveRSSFeed(w, siteName+" — "+slug, siteURL+"/spaces/"+slug,
			"Posts from the "+slug+" space", items)
	}
}

// HandleTagFeed serves a tag-specific RSS feed at /tags/{slug}/feed.xml.
func HandleTagFeed(q *gen.Queries, siteURL string, siteName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		if slug == "" {
			http.Error(w, "missing tag slug", http.StatusBadRequest)
			return
		}

		tenantID := tenant.IDFromContext(r.Context())
		if tenantID == (pgtype.UUID{}) {
			http.Error(w, "tenant unavailable", http.StatusServiceUnavailable)
			return
		}

		rows, err := q.ListRSSTagFeedItems(r.Context(), gen.ListRSSTagFeedItemsParams{
			TenantID: tenantID,
			Slug:     slug,
		})
		if err != nil {
			slog.Error("rss: query tag feed", "slug", slug, "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)

			return
		}

		items := feedItemsFromTagRows(rows, siteURL)
		serveRSSFeed(w, siteName+" — #"+slug, siteURL+"/tags/"+slug,
			"Posts tagged #"+slug, items)
	}
}

// HandleUserFeed serves a user-specific RSS feed at /users/{username}/feed.xml.
func HandleUserFeed(q *gen.Queries, siteURL string, siteName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := chi.URLParam(r, "username")
		if username == "" {
			http.Error(w, "missing username", http.StatusBadRequest)
			return
		}

		tenantID := tenant.IDFromContext(r.Context())
		if tenantID == (pgtype.UUID{}) {
			http.Error(w, "tenant unavailable", http.StatusServiceUnavailable)
			return
		}

		rows, err := q.ListRSSUserFeedItems(r.Context(), gen.ListRSSUserFeedItemsParams{
			TenantID: tenantID,
			Username: username,
		})
		if err != nil {
			slog.Error("rss: query user feed", "username", username, "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)

			return
		}

		items := feedItemsFromUserRows(rows, siteURL)
		serveRSSFeed(w, siteName+" — "+username, siteURL+"/users/"+username,
			"Posts by "+username, items)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// formatTime formats a pgtype.Timestamptz as RFC1123Z.
func formatTime(t pgtype.Timestamptz) string {
	if t.Valid {
		return t.Time.Format(time.RFC1123Z)
	}

	return ""
}

// feedItemsFromContentTypeRows converts ListRSSFeedItemsByContentTypeRow to RSS items.
func feedItemsFromContentTypeRows(rows []gen.ListRSSFeedItemsByContentTypeRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RSSItem{
			Title:       row.Title,
			Link:        siteURL + "/content/" + row.Slug,
			Description: row.Left,
			PubDate:     formatTime(row.CreatedAt),
			Author:      row.Username,
		})
	}

	return items
}

// feedItemsFromAllRows converts ListRSSFeedItemsRow to RSS items.
func feedItemsFromAllRows(rows []gen.ListRSSFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RSSItem{
			Title:       row.Title,
			Link:        siteURL + "/content/" + row.Slug,
			Description: row.Left,
			PubDate:     formatTime(row.CreatedAt),
			Author:      row.Username,
		})
	}

	return items
}

// feedItemsFromSpaceRows converts ListRSSSpaceFeedItemsRow to RSS items.
func feedItemsFromSpaceRows(rows []gen.ListRSSSpaceFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RSSItem{
			Title:       row.Title,
			Link:        siteURL + "/content/" + row.Slug,
			Description: row.Left,
			PubDate:     formatTime(row.CreatedAt),
			Author:      row.Username,
		})
	}

	return items
}

// feedItemsFromTagRows converts ListRSSTagFeedItemsRow to RSS items.
func feedItemsFromTagRows(rows []gen.ListRSSTagFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RSSItem{
			Title:       row.Title,
			Link:        siteURL + "/content/" + row.Slug,
			Description: row.Left,
			PubDate:     formatTime(row.CreatedAt),
			Author:      row.Username,
		})
	}

	return items
}

// feedItemsFromUserRows converts ListRSSUserFeedItemsRow to RSS items.
func feedItemsFromUserRows(rows []gen.ListRSSUserFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RSSItem{
			Title:       row.Title,
			Link:        siteURL + "/content/" + row.Slug,
			Description: row.Left,
			PubDate:     formatTime(row.CreatedAt),
			Author:      row.Username,
		})
	}

	return items
}

// serveRSSFeed marshals the feed to XML and writes the response.
func serveRSSFeed(w http.ResponseWriter, title, link, description string, items []RSSItem) {
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	feed := RSSFeed{
		Version: "2.0",
		Channel: RSSChannel{
			Title:       title,
			Link:        link,
			Description: description,
			Language:    "en",
			Items:       items,
		},
	}

	w.WriteHeader(http.StatusOK)

	if err := xml.NewEncoder(w).Encode(feed); err != nil {
		slog.Error("rss: encode xml", "error", err)
	}
}
