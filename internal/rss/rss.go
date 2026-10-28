// Package rss provides RSS feed handlers for JapaConnect content.
// // The handlers query published content from the database and render
// standard RSS 2.0 XML. All feeds are public (no auth required).
// // The shared RSS 2.0 XML model lives in internal/rss/model — imported by
// this package, internal/blog, internal/roadmap, and internal/worker so all
// four render feeds identically (RSS-011).
// // Ref: japaconnect-engineering/sections/26-seo-performance.md
package rss

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
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
// writing cached RSS feeds (jc:cache:feed:rss*). Space/tag/user/forum feeds
// cache their rendered XML inline (write-on-miss) so the
// Cache-Control: max-age=900 header is backed by an actual cache.
type CacheClient interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *valkeycompat.StatusCmd
}

// feedCacheTTL is the TTL for inline-cached space/tag/user/forum feeds.
// Matches the universal feed's worker-generated cache (1hr) — comfortably
// longer than the Cache-Control max-age=900s so the 15min browser TTL stays
// valid between hourly regenerations.
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

// serveFeedXML writes a rendered feed body with the standard RSS headers
// (§26: Cache-Control max-age=900). Honors If-None-Match against a weak ETag
// of the body — a matching request gets 304 and no payload. A nil body means
// the render failed upstream and becomes a 500, not an empty feed.
func serveFeedXML(w http.ResponseWriter, r *http.Request, body []byte) {
	if body == nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to render feed")
		return
	}

	etag := model.ETag(body)
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age="+itoa(cacheMaxAge))
	w.Header().Set("ETag", etag)

	if model.IfNoneMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	// #nosec G705 // feed is server-rendered XML; user text is escaped by xml.Marshal.
	if _, err := w.Write(body); err != nil {
		slog.Error("rss: write xml", "error", err)
	}
}

// HandleFeed serves the universal feed at /feed.xml.
// The feed returns all published content (excluding soft-deleted and
// auto-hidden items). Reads from the Valkey cache populated by the
// rss:regenerate worker task when available; on a miss it renders live and
// writes the body back so subsequent requests are cache hits.
func (h *Handlers) HandleFeed(w http.ResponseWriter, r *http.Request) {
	siteURL := resolveSiteURL(r, h.siteURL)

	// Try the Valkey cache first (jc:cache:feed:rss).
	cacheKey := valkey.MustKey("cache:feed:rss")
	if h.redis != nil {
		if cached, err := h.redis.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			serveFeedXML(w, r, []byte(cached))

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

	// The channel <link> is the site root, not the feed URL itself (RSS 2.0:
	// link is "the URL to the HTML website corresponding to the channel").
	xmlBytes := renderFeedXML(h.siteName, siteURL, h.siteName+" — latest content", items)

	if h.redis != nil && xmlBytes != nil {
		if err := h.redis.Set(r.Context(), cacheKey, string(xmlBytes), feedCacheTTL).Err(); err != nil {
			slog.Warn("rss: failed to cache feed", "cache_key", cacheKey, "error", err)
		}
	}

	serveFeedXML(w, r, xmlBytes)
}

// serveCachedOrLive handles the space/tag/user/forum feed pattern: try the
// Valkey cache for the given key, serve on hit; on miss, run the query,
// render, write the rendered XML back to the cache, and serve.
// // The render callback is only invoked on a cache miss and returns the
// items or an error (in which case a 500 is written and no caching occurs).
func (h *Handlers) serveCachedOrLive(
	w http.ResponseWriter,
	r *http.Request,
	cacheKey string,
	render func() ([]RSSItem, error),
	title, link, description string,
) {
	// Try the inline cache first.
	if h.redis != nil {
		if cached, err := h.redis.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			serveFeedXML(w, r, []byte(cached))

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

	serveFeedXML(w, r, xmlBytes)
}

// HandleSpaceFeed serves a space-specific RSS feed at /spaces/{slug}/feed.xml.
// Nonexistent or non-public spaces 404 rather than serving an empty phantom
// channel — an uncached 404 also keeps arbitrary slugs out of the cache.
func (h *Handlers) HandleSpaceFeed(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "Space slug is required")
		return
	}

	// Existence + visibility gate: only public, active spaces get a feed.
	if _, err := h.q.GetPublicSpaceBySlug(r.Context(), slug); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
			return
		}

		slog.Error("rss: lookup space for feed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load feed")

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
// Nonexistent tags 404 rather than serving an empty phantom channel.
func (h *Handlers) HandleTagFeed(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "Tag slug is required")
		return
	}

	// Existence gate: the list query alone can't tell "no posts" from "no tag".
	if _, err := h.q.GetTagBySlug(r.Context(), slug); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "tag_not_found", "Tag not found")
			return
		}

		slog.Error("rss: lookup tag for feed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load feed")

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
// The channel <link> points at the canonical public profile route /u/{username}
// (the frontend proxies /u/{username}/feed.xml to this handler).
func (h *Handlers) HandleUserFeed(w http.ResponseWriter, r *http.Request) {
	username := httpx.URLParamDecoded(r, "username")
	if username == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "Username is required")
		return
	}

	// A feed for a nonexistent user must 404, not serve an empty phantom
	// channel. GetPublicProfileByUsername resolves the canonical username OR
	// the public_username alias (and requires an active account) — the same
	// semantics as ListRSSUserFeedItems and the /u/{username} profile page.
	if _, err := h.q.GetPublicProfileByUsername(r.Context(), username); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}

		slog.Error("rss: lookup user for feed", "username", username, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load feed")

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
		siteURL+"/u/"+username,
		"Posts by "+username+" on "+h.siteName,
	)
}

// HandleForumFeed serves the forum-wide RSS feed at /forum/feed.xml.
// Items are published threads from public spaces — members/private-space
// threads and anonymous authors never leak (see ListRSSForumFeedItems).
func (h *Handlers) HandleForumFeed(w http.ResponseWriter, r *http.Request) {
	siteURL := resolveSiteURL(r, h.siteURL)
	cacheKey := valkey.MustKey("cache:feed:rss:forum")

	h.serveCachedOrLive(
		w, r, cacheKey,
		func() ([]RSSItem, error) {
			rows, err := h.q.ListRSSForumFeedItems(r.Context(), h.feedLimit)
			if err != nil {
				slog.Error("rss: query forum feed", "error", err)
				return nil, err
			}

			return feedItemsFromForumRows(rows, siteURL), nil
		},
		h.siteName+" — Forum",
		siteURL+"/forum",
		"Latest discussions from the "+h.siteName+" community forum.",
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

// HandleTypeFeed returns a handler for /{prefix}/feed.xml — a per-type feed
// for one ADR-058 content route family (/story/feed.xml, /guide/feed.xml,
// …). Cached write-on-miss under jc:cache:feed:rss:type:{content_type};
// the rss:regenerate worker also warms these keys. Unknown prefixes 404 —
// see model.TypeFeedForPrefix.
func (h *Handlers) HandleTypeFeed(prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tf, ok := model.TypeFeedForPrefix(prefix)
		if !ok {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Feed not found")
			return
		}

		siteURL := resolveSiteURL(r, h.siteURL)
		cacheKey := valkey.MustKey("cache:feed:rss:type:%s", tf.ContentType)

		h.serveCachedOrLive(w, r, cacheKey, func() ([]RSSItem, error) {
			rows, err := h.q.ListRSSFeedItemsByContentType(r.Context(), gen.ListRSSFeedItemsByContentTypeParams{
				ContentType: tf.ContentType,
				PageLimit:   defaultFeedLimit,
			})
			if err != nil {
				return nil, err
			}

			return feedItemsFromTypeRows(rows, siteURL), nil
		}, h.siteName+" — "+tf.Label, siteURL+"/"+prefix, "Latest "+strings.ToLower(tf.Label)+" on "+h.siteName+".")
	}
}

// feedItemsFromAllRows converts ListRSSFeedItemsRow to RSS items.
func feedItemsFromAllRows(rows []gen.ListRSSFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.ItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, model.LinkPathFor(row.ContentType),
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
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, model.LinkPathFor(row.ContentType),
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
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, model.LinkPathFor(row.ContentType),
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
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, model.LinkPathFor(row.ContentType),
		))
	}

	return items
}

// feedItemsFromForumRows converts ListRSSForumFeedItemsRow to RSS items —
// thread links use the canonical /forum/{slug} route, not /content/.
func feedItemsFromForumRows(rows []gen.ListRSSForumFeedItemsRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.ItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, "/forum/",
		))
	}

	return items
}

// feedItemsFromTypeRows converts ListRSSFeedItemsByContentTypeRow to RSS
// items — canonical links still resolve per content_type via LinkPathFor.
func feedItemsFromTypeRows(rows []gen.ListRSSFeedItemsByContentTypeRow, siteURL string) []RSSItem {
	items := make([]RSSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.ItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, siteURL, model.LinkPathFor(row.ContentType),
		))
	}

	return items
}

// renderFeedXML marshals the feed to XML bytes (with the XML declaration).
// Delegates to the shared model.Render (RSS-011).
func renderFeedXML(title, link, description string, items []RSSItem) []byte {
	return model.Render(title, link, description, "en", link+"/feed.xml", items)
}

// itoa converts an int to its decimal string representation.
func itoa(n int) string {
	return strconv.Itoa(n)
}
