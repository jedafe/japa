// Package rss provides RSS feed handlers for JapaConnect content.
//
// The handlers query published content from the database and render
// standard RSS 2.0 XML. All feeds are public (no auth required) and
// tenant-scoped via the tenant middleware.
//
// Ref: japaconnect-engineering/sections/26-seo-performance.md
package rss

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// defaultFeedLimit is the number of items returned per feed.
// Ref: japaconnect-engineering/sections/26-seo-performance.md
const defaultFeedLimit = 20

// cacheMaxAge is the Cache-Control max-age in seconds (JEOS §26: 900s = 15min).
const cacheMaxAge = 900

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
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []RSSItem `xml:"item"`
}

// RSSItem represents a single entry in the feed.
type RSSItem struct { //nolint:revive // stuttering type name preserved for API compatibility
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author,omitempty"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// CacheReader is the subset of valkeycompat.Cmdable used for reading
// cached RSS feeds (jc:cache:feed:rss:{tenant_id}).
type CacheReader interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
}

// Handlers holds dependencies for RSS HTTP handlers.
type Handlers struct {
	q         *gen.Queries
	siteURL   string
	siteName  string
	redis     CacheReader // nil disables cache reads
	feedLimit int32
}

// NewHandlers creates Handlers with the given queries and site config.
func NewHandlers(q *gen.Queries, siteURL string, siteName string) *Handlers {
	return &Handlers{
		q:         q,
		siteURL:   siteURL,
		siteName:  siteName,
		feedLimit: defaultFeedLimit,
	}
}

// SetRedis injects the Valkey/Redis client for reading cached feeds.
func (h *Handlers) SetRedis(r CacheReader) {
	h.redis = r
}

// HandleFeed serves the universal feed at /feed.xml.
// The feed returns all published content (excluding soft-deleted and
// auto-hidden items). Reads from the Valkey cache populated by the
// rss:regenerate worker task when available; falls back to live DB.
func (h *Handlers) HandleFeed(w http.ResponseWriter, r *http.Request) {
	tenantID := tenant.IDFromContext(r.Context())
	if tenantID == (pgtype.UUID{}) {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	siteURL := resolveSiteURL(r, h.siteURL)

	// Try the Valkey cache first (jc:cache:feed:rss:{tenant_id}).
	cacheKey := "jc:cache:feed:rss:" + tenantID.String()
	if h.redis != nil {
		if cached, err := h.redis.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age="+itoa(cacheMaxAge))
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(cached))
			return
		}
	}

	// Cache miss — generate live from the DB.
	rows, err := h.q.ListRSSFeedItems(r.Context(), gen.ListRSSFeedItemsParams{
		TenantID:  tenantID,
		PageLimit: h.feedLimit,
	})
	if err != nil {
		slog.Error("rss: query feed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	items := feedItemsFromAllRows(rows, siteURL)
	serveRSSFeed(w, h.siteName, siteURL+"/feed.xml", h.siteName+" — latest content", items)
}

// HandleSpaceFeed serves a space-specific RSS feed at /spaces/{slug}/feed.xml.
func (h *Handlers) HandleSpaceFeed(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "Space slug is required")
		return
	}

	tenantID := tenant.IDFromContext(r.Context())
	if tenantID == (pgtype.UUID{}) {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	siteURL := resolveSiteURL(r, h.siteURL)

	rows, err := h.q.ListRSSSpaceFeedItems(r.Context(), gen.ListRSSSpaceFeedItemsParams{
		TenantID:  tenantID,
		Slug:      slug,
		PageLimit: h.feedLimit,
	})
	if err != nil {
		slog.Error("rss: query space feed", "slug", slug, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	items := feedItemsFromSpaceRows(rows, siteURL)
	serveRSSFeed(w, h.siteName+" — "+slug, siteURL+"/spaces/"+slug,
		"Posts from the "+slug+" space on "+h.siteName, items)
}

// HandleTagFeed serves a tag-specific RSS feed at /tags/{slug}/feed.xml.
func (h *Handlers) HandleTagFeed(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "Tag slug is required")
		return
	}

	tenantID := tenant.IDFromContext(r.Context())
	if tenantID == (pgtype.UUID{}) {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	siteURL := resolveSiteURL(r, h.siteURL)

	rows, err := h.q.ListRSSTagFeedItems(r.Context(), gen.ListRSSTagFeedItemsParams{
		TenantID:  tenantID,
		Slug:      slug,
		PageLimit: h.feedLimit,
	})
	if err != nil {
		slog.Error("rss: query tag feed", "slug", slug, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	items := feedItemsFromTagRows(rows, siteURL)
	serveRSSFeed(w, h.siteName+" — #"+slug, siteURL+"/tags/"+slug,
		"Posts tagged #"+slug+" on "+h.siteName, items)
}

// HandleUserFeed serves a user-specific RSS feed at /users/{username}/feed.xml.
func (h *Handlers) HandleUserFeed(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		writeError(w, http.StatusBadRequest, "missing_field", "Username is required")
		return
	}

	tenantID := tenant.IDFromContext(r.Context())
	if tenantID == (pgtype.UUID{}) {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	siteURL := resolveSiteURL(r, h.siteURL)

	rows, err := h.q.ListRSSUserFeedItems(r.Context(), gen.ListRSSUserFeedItemsParams{
		TenantID:  tenantID,
		Username:  username,
		PageLimit: h.feedLimit,
	})
	if err != nil {
		slog.Error("rss: query user feed", "username", username, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	items := feedItemsFromUserRows(rows, siteURL)
	serveRSSFeed(w, h.siteName+" — "+username, siteURL+"/users/"+username,
		"Posts by "+username+" on "+h.siteName, items)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeError writes a JSON error response in the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// resolveSiteURL returns the public-facing site URL. It prefers
// X-Forwarded-Host (set by the frontend proxy) over the configured
// JC_SITE_URL so RSS links point to the frontend, not the internal
// backend port. BUG-006 equivalent fix for universal feeds.
func resolveSiteURL(r *http.Request, fallback string) string {
	if fwdHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); fwdHost != "" {
		scheme := "https"
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else if r.TLS == nil {
			scheme = "http"
		}
		return scheme + "://" + fwdHost
	}
	return fallback
}

// htmlTagPattern matches HTML/XML tags for stripping to plain text.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// htmlToTextExcerpt strips HTML tags from bodyHtml, collapses whitespace,
// and truncates to maxRunes characters, returning a clean plain-text excerpt
// suitable for an RSS <description> element.
func htmlToTextExcerpt(bodyHtml string, maxRunes int) string {
	plain := htmlTagPattern.ReplaceAllString(bodyHtml, "")
	plain = strings.TrimSpace(plain)
	plain = strings.Join(strings.Fields(plain), " ")
	if len([]rune(plain)) <= maxRunes {
		return plain
	}
	runes := []rune(plain)
	return string(runes[:maxRunes]) + "…"
}

// excerptFromBody extracts a plain-text excerpt from raw markdown body
// (fallback when body_html is empty).
func excerptFromBody(body string) string {
	plain := strings.TrimSpace(body)
	plain = strings.Join(strings.Fields(plain), " ")
	if len([]rune(plain)) <= 300 {
		return plain
	}
	runes := []rune(plain)
	return string(runes[:300]) + "…"
}

// formatTime formats a pgtype.Timestamptz as RFC1123Z for RSS pubDate.
func formatTime(t pgtype.Timestamptz) string {
	if t.Valid {
		return t.Time.Format(time.RFC1123Z)
	}
	return ""
}

// feedItemFromRow converts a generic RSS row to an RSSItem.
// Uses published_at for PubDate (falling back to created_at), strips HTML
// for the description, and sets a GUID from the content ID.
func feedItemFromRow(id pgtype.UUID, title, slug, bodyHtml, body, username string, publishedAt, createdAt pgtype.Timestamptz, siteURL string) RSSItem {
	// PubDate: prefer published_at, fall back to created_at.
	pubDate := formatTime(publishedAt)
	if pubDate == "" {
		pubDate = formatTime(createdAt)
	}

	// Description: strip HTML from body_html, fall back to body excerpt.
	desc := htmlToTextExcerpt(bodyHtml, 300)
	if desc == "" && body != "" {
		desc = excerptFromBody(body)
	}

	// GUID: use content UUID as a URN.
	guid := ""
	if id.Valid {
		guid = "urn:uuid:" + id.String()
	}

	return RSSItem{
		Title:       title,
		Link:        siteURL + "/content/" + slug,
		GUID:        guid,
		Description: desc,
		PubDate:     pubDate,
		Author:      username,
	}
}

// feedItemsFromAllRows converts ListRSSFeedItemsRow to RSS items.
func feedItemsFromAllRows(rows []gen.ListRSSFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, feedItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL,
		))
	}
	return items
}

// feedItemsFromSpaceRows converts ListRSSSpaceFeedItemsRow to RSS items.
func feedItemsFromSpaceRows(rows []gen.ListRSSSpaceFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, feedItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL,
		))
	}
	return items
}

// feedItemsFromTagRows converts ListRSSTagFeedItemsRow to RSS items.
func feedItemsFromTagRows(rows []gen.ListRSSTagFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, feedItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL,
		))
	}
	return items
}

// feedItemsFromUserRows converts ListRSSUserFeedItemsRow to RSS items.
func feedItemsFromUserRows(rows []gen.ListRSSUserFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, feedItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL,
		))
	}
	return items
}

// serveRSSFeed marshals the feed to XML and writes the response.
func serveRSSFeed(w http.ResponseWriter, title, link, description string, items []RSSItem) {
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age="+itoa(cacheMaxAge))

	// lastBuildDate: the most recent item's pubDate, or now.
	var lastBuild string
	if len(items) > 0 && items[0].PubDate != "" {
		lastBuild = items[0].PubDate
	} else {
		lastBuild = time.Now().UTC().Format(time.RFC1123Z)
	}

	feed := RSSFeed{
		Version: "2.0",
		Channel: RSSChannel{
			Title:         title,
			Link:          link,
			Description:   description,
			Language:      "en",
			LastBuildDate: lastBuild,
			Items:         items,
		},
	}

	w.WriteHeader(http.StatusOK)

	// Write the XML declaration before encoding (required by some RSS readers).
	if _, err := io.WriteString(w, xml.Header); err != nil {
		slog.Error("rss: write xml header", "error", err)
		return
	}

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		slog.Error("rss: encode xml", "error", err)
	}
}

// itoa converts an int to its decimal string representation.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
