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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/tenant"
)

// ---------------------------------------------------------------------------
// XML model
// ---------------------------------------------------------------------------

// RSSFeed is the top-level RSS 2.0 envelope.
type RSSFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel RSSChannel `xml:"channel"`
}

// RSSChannel groups feed metadata and items.
type RSSChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language,omitempty"`
	Items       []RSSItem `xml:"item"`
}

// RSSItem represents a single entry in the feed.
type RSSItem struct {
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
func HandleFeed(pool *pgxpool.Pool, feedType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenant.IDFromContext(r.Context())
		if tenantID == (pgtype.UUID{}) {
			http.Error(w, "tenant unavailable", http.StatusServiceUnavailable)
			return
		}

		var contentFilter string
		switch feedType {
		case "blog":
			contentFilter = "AND c.content_type = 'blog_post'"
		default:
			contentFilter = ""
		}

		items, err := queryFeedItems(r, pool, tenantID, contentFilter)
		if err != nil {
			slog.Error("rss: query feed", "feed_type", feedType, "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		serveRSSFeed(w, "JapaConnect", "https://japaconnect.com",
			"Community platform for the Nigerian diaspora", items)
	}
}

// HandleSpaceFeed serves a space-specific RSS feed at /spaces/{slug}/feed.xml.
func HandleSpaceFeed(pool *pgxpool.Pool) http.HandlerFunc {
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

		rows, err := pool.Query(r.Context(), `
			SELECT c.title, c.slug, LEFT(c.body, 200), c.created_at, u.username
			FROM contents c
			JOIN users u ON c.author_user_id = u.id
			JOIN spaces s ON c.space_id = s.id
			WHERE c.tenant_id = $1 AND c.status = 'published' AND s.slug = $2
			ORDER BY c.created_at DESC
			LIMIT 20`, tenantID, slug)
		if err != nil {
			slog.Error("rss: query space feed", "slug", slug, "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := scanFeedItems(rows)
		serveRSSFeed(w, "JapaConnect — "+slug, "https://japaconnect.com/spaces/"+slug,
			"Posts from the "+slug+" space", items)
	}
}

// HandleTagFeed serves a tag-specific RSS feed at /tags/{slug}/feed.xml.
func HandleTagFeed(pool *pgxpool.Pool) http.HandlerFunc {
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

		rows, err := pool.Query(r.Context(), `
			SELECT c.title, c.slug, LEFT(c.body, 200), c.created_at, u.username
			FROM contents c
			JOIN users u ON c.author_user_id = u.id
			JOIN content_tags ct ON ct.content_id = c.id
			JOIN tags t ON ct.tag_id = t.id
			WHERE c.tenant_id = $1 AND c.status = 'published' AND t.slug = $2
			ORDER BY c.created_at DESC
			LIMIT 20`, tenantID, slug)
		if err != nil {
			slog.Error("rss: query tag feed", "slug", slug, "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := scanFeedItems(rows)
		serveRSSFeed(w, "JapaConnect — #"+slug, "https://japaconnect.com/tags/"+slug,
			"Posts tagged #"+slug, items)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// queryFeedItems runs the common feed query with an optional content type filter.
func queryFeedItems(r *http.Request, pool *pgxpool.Pool, tenantID pgtype.UUID, contentFilter string) ([]RSSItem, error) {
	rows, err := pool.Query(r.Context(), `
		SELECT c.title, c.slug, LEFT(c.body, 200), c.created_at, u.username
		FROM contents c
		JOIN users u ON c.author_user_id = u.id
		WHERE c.tenant_id = $1 AND c.status = 'published' `+contentFilter+`
		ORDER BY c.created_at DESC
		LIMIT 20`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanFeedItems(rows), nil
}

// scanFeedItems iterates rows and builds RSS items, skipping any scan errors.
func scanFeedItems(rows pgx.Rows) []RSSItem {
	items := make([]RSSItem, 0, 20)
	for rows.Next() {
		var item RSSItem
		var createdAt time.Time
		if err := rows.Scan(&item.Title, &item.Link, &item.Description, &createdAt, &item.Author); err != nil {
			slog.Warn("rss: scan row", "error", err)
			continue
		}
		item.Link = "https://japaconnect.com/content/" + item.Link
		item.PubDate = createdAt.Format(time.RFC1123Z)
		items = append(items, item)
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
