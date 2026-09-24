// Package rss provides RSS feed handlers for JapaConnect content.
// // The handlers query published content from the database and render
// standard RSS 2.0 XML. All feeds are public (no auth required) and
// platform-scoped via the platform middleware.
// // The shared RSS 2.0 XML model lives in internal/rss/model — imported by
// this package, internal/blog, and internal/worker so all three render
// feeds identically (RSS-011).
// // Ref: japaconnect-engineering/sections/26-seo-performance.md
package rss

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/rss/model"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// defaultFeedLimit is the number of items returned per feed.
// Ref: japaconnect-engineering/sections/26-seo-performance.md.
const defaultFeedLimit = 20

// cacheMaxAge is the Cache-Control max-age in seconds (JEOS §26: 900s = 15min).
const cacheMaxAge = 900

// ---------------------------------------------------------------------------
// XML model — aliases re-exported from the shared internal/rss/model
// subpackage (RSS-011). Aliases preserve backward compatibility for tests
// and any external references.
// ---------------------------------------------------------------------------

// RSSFeed is the top-level RSS 2.0 envelope.
type RSSFeed = model.Feed //nolint:revive // stuttering alias preserved for API compatibility

// RSSChannel groups feed metadata and items.
type RSSChannel = model.Channel //nolint:revive // stuttering alias preserved for API compatibility

// RSSItem represents a single entry in the feed.
type RSSItem = model.Item //nolint:revive // stuttering alias preserved for API compatibility

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// CacheClient is the subset of valkeycompat.Cmdable used for reading and
// writing cached RSS feeds (jc:cache:feed:rss:*). RSS-005: space/tag/user
// feeds now cache their rendered XML inline (write-on-miss) so the
// Cache-Control: max-age=900 header is backed by an actual cache.
type CacheClient interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *valkeycompat.StatusCmd
}

// feedCacheTTL is the TTL for inline-cached space/tag/user feeds. Matches
// the universal feed's worker-generated cache (1hr) — comfortably longer
// than the Cache-Control max-age=900s so the 15min browser TTL stays valid
// between hourly regenerations.
const feedCacheTTL = time.Hour

// Handlers holds dependencies for RSS HTTP handlers.
type Handlers struct {
	q         *gen.Queries
	siteURL   string
	siteName  string
	redis     CacheClient // nil disables cache reads/writes
	feedLimit int32
	cat       literalCatalog // nil disables literal resolution
}

// literalCatalog resolves {{...}} template literals on feed item fields
// (§14 Global Template Literals). Implemented by *literals.Catalog.
type literalCatalog interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// NewHandlers creates Handlers with the given queries and site config.
func NewHandlers(q *gen.Queries, siteURL, siteName string) *Handlers {
	return &Handlers{
		q:         q,
		siteURL:   siteURL,
		siteName:  siteName,
		feedLimit: defaultFeedLimit,
	}
}

// SetRedis injects the Valkey/Redis client for reading and writing cached
// feeds.
func (h *Handlers) SetRedis(r CacheClient) {
	h.redis = r
}

// SetLiteralCatalog injects the template-literal catalog so feed item
// titles and descriptions render {{...}} placeholders (§14). Resolution
// runs on the cache-miss render path so cached XML is already resolved.
func (h *Handlers) SetLiteralCatalog(c literalCatalog) {
	h.cat = c
}

// resolveItems resolves {{...}} placeholders on feed items using a
// request-independent context — feeds are cached and shared, so geo tokens
// fall back to each item's own geo metadata or the literal fallback.
func (h *Handlers) resolveItems(ctx context.Context, items []RSSItem) {
	if h.cat == nil {
		return
	}

	h.cat.ResolveFields(ctx, &items, literals.NewContext(ctx, ""))
}

// HandleFeed serves the universal feed at /feed.xml.
// The feed returns all published content (excluding soft-deleted and
// auto-hidden items). Reads from the Valkey cache populated by the
// rss:regenerate worker task when available; falls back to live DB.
func (h *Handlers) HandleFeed(w http.ResponseWriter, r *http.Request) {

	siteURL := resolveSiteURL(r, h.siteURL)

	// Try the Valkey cache first (jc:cache:feed:rss:{}).
	cacheKey := valkey.MustKey("cache:feed:rss")
	if h.redis != nil {
		if cached, err := h.redis.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age="+itoa(cacheMaxAge))
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			// #nosec G705 // feed is server-rendered XML; user text is escaped by xml.Marshal.
			_, _ = w.Write([]byte(cached))

			return
		}
	}

	// Cache miss — generate live from the DB.
	rows, err := h.q.ListRSSFeedItems(r.Context(),

		h.feedLimit,
	)
	if err != nil {
		slog.Error("rss: query feed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Internal server error")

		return
	}

	items := feedItemsFromAllRows(rows, siteURL)
	h.resolveItems(r.Context(), items)
	serveRSSFeed(w, h.siteName, siteURL+"/feed.xml", h.siteName+" — latest content", items)
}

// serveCachedOrLive handles the space/tag/user feed pattern: try the
// Valkey cache for the given key, serve on hit; on miss, run the query,
// render, write the rendered XML back to the cache, and serve. RSS-005.
// // The render callback is only invoked on a cache miss and returns the
// items or an error (in which case a 500 is written and no caching occurs).
func (h *Handlers) serveCachedOrLive(
	w http.ResponseWriter,
	r *http.Request,
	cacheKey string,
	render func() ([]RSSItem, error),
	title, link, description string,
) {
	// RSS-005: try the inline cache first.
	if h.redis != nil {
		if cached, err := h.redis.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age="+itoa(cacheMaxAge))
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			// #nosec G705 // feed is server-rendered XML; user text is escaped by xml.Marshal.
			_, _ = w.Write([]byte(cached))

			return
		}
	}

	items, err := render()
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	// §14: resolve literals before XML render so cached and live feeds
	// alike never contain raw {{...}} placeholders.
	h.resolveItems(r.Context(), items)

	// Render to bytes so we can both write it to the cache and serve it.
	xmlBytes := renderFeedXML(title, link, description, items)

	if h.redis != nil && xmlBytes != nil {
		if err := h.redis.Set(r.Context(), cacheKey, string(xmlBytes), feedCacheTTL).Err(); err != nil {
			slog.Warn("rss: failed to cache feed", "cache_key", cacheKey, "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age="+itoa(cacheMaxAge))
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 // feed is server-rendered XML; user text is escaped by xml.Marshal.
	_, _ = w.Write(xmlBytes)
}

// HandleSpaceFeed serves a space-specific RSS feed at /spaces/{slug}/feed.xml.
func (h *Handlers) HandleSpaceFeed(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "Space slug is required")
		return
	}

	siteURL := resolveSiteURL(r, h.siteURL)
	cacheKey := valkey.MustKey("cache:feed:rss:space:%s", slug)

	h.serveCachedOrLive(
		w, r, cacheKey,
		func() ([]RSSItem, error) {
			rows, err := h.q.ListRSSSpaceFeedItems(r.Context(), gen.ListRSSSpaceFeedItemsParams{

				Slug:      slug,
				PageLimit: h.feedLimit,
			})
			if err != nil {
				slog.Error("rss: query space feed", "slug", slug, "error", err)
				return nil, err
			}

			return feedItemsFromSpaceRows(rows, siteURL), nil
		},
		h.siteName+" — "+slug,
		siteURL+"/spaces/"+slug,
		"Posts from the "+slug+" space on "+h.siteName,
	)
}

// HandleTagFeed serves a tag-specific RSS feed at /tags/{slug}/feed.xml.
func (h *Handlers) HandleTagFeed(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "Tag slug is required")
		return
	}

	siteURL := resolveSiteURL(r, h.siteURL)
	cacheKey := valkey.MustKey("cache:feed:rss:tag:%s", slug)

	h.serveCachedOrLive(
		w, r, cacheKey,
		func() ([]RSSItem, error) {
			rows, err := h.q.ListRSSTagFeedItems(r.Context(), gen.ListRSSTagFeedItemsParams{

				Slug:      slug,
				PageLimit: h.feedLimit,
			})
			if err != nil {
				slog.Error("rss: query tag feed", "slug", slug, "error", err)
				return nil, err
			}

			return feedItemsFromTagRows(rows, siteURL), nil
		},
		h.siteName+" — #"+slug,
		siteURL+"/tags/"+slug,
		"Posts tagged #"+slug+" on "+h.siteName,
	)
}

// HandleUserFeed serves a user-specific RSS feed at /users/{username}/feed.xml.
func (h *Handlers) HandleUserFeed(w http.ResponseWriter, r *http.Request) {
	username := httpx.URLParamDecoded(r, "username")
	if username == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "Username is required")
		return
	}

	siteURL := resolveSiteURL(r, h.siteURL)
	cacheKey := valkey.MustKey("cache:feed:rss:user:%s", username)

	h.serveCachedOrLive(
		w, r, cacheKey,
		func() ([]RSSItem, error) {
			rows, err := h.q.ListRSSUserFeedItems(r.Context(), gen.ListRSSUserFeedItemsParams{

				Username:  username,
				PageLimit: h.feedLimit,
			})
			if err != nil {
				slog.Error("rss: query user feed", "username", username, "error", err)
				return nil, err
			}

			return feedItemsFromUserRows(rows, siteURL), nil
		},
		h.siteName+" — "+username,
		siteURL+"/users/"+username,
		"Posts by "+username+" on "+h.siteName,
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveSiteURL returns the public-facing site URL. X-Forwarded-Host/Proto
// are honoured only when the immediate peer is a trusted proxy
// (JC_TRUSTED_PROXIES) — otherwise an attacker could spoof
// X-Forwarded-Host: evil.com and inject their domain into RSS feed links
// (host-header injection). Falls back to the configured JC_SITE_URL when the
// peer is untrusted or no forwarding headers are present.
func resolveSiteURL(r *http.Request, fallback string) string {
	return clientip.ForwardedBaseURL(r, fallback)
}

// feedItemsFromAllRows converts ListRSSFeedItemsRow to RSS items.
func feedItemsFromAllRows(rows []gen.ListRSSFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.ItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, "/content/",
		))
	}

	return items
}

// feedItemsFromSpaceRows converts ListRSSSpaceFeedItemsRow to RSS items.
func feedItemsFromSpaceRows(rows []gen.ListRSSSpaceFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.ItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, "/content/",
		))
	}

	return items
}

// feedItemsFromTagRows converts ListRSSTagFeedItemsRow to RSS items.
func feedItemsFromTagRows(rows []gen.ListRSSTagFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.ItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, "/content/",
		))
	}

	return items
}

// feedItemsFromUserRows converts ListRSSUserFeedItemsRow to RSS items.
func feedItemsFromUserRows(rows []gen.ListRSSUserFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.ItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, "/content/",
		))
	}

	return items
}

// renderFeedXML marshals the feed to XML bytes (with the XML declaration).
// Delegates to the shared model.Render (RSS-011).
func renderFeedXML(title, link, description string, items []RSSItem) []byte {
	return model.Render(title, link, description, "en", items)
}

// serveRSSFeed marshals the feed to XML and writes the response.
func serveRSSFeed(w http.ResponseWriter, title, link, description string, items []RSSItem) {
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age="+itoa(cacheMaxAge))
	// RSS-006: signal cache miss for observability (HIT is set by callers
	// that serve from cache).
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)

	xmlBytes := renderFeedXML(title, link, description, items)
	if xmlBytes == nil {
		return
	}

	// #nosec G705 // feed is server-rendered XML; user text is escaped by xml.Marshal.
	if _, err := w.Write(xmlBytes); err != nil {
		slog.Error("rss: write xml", "error", err)
	}
}

// itoa converts an int to its decimal string representation.
func itoa(n int) string {
	return strconv.Itoa(n)
}
