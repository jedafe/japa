package seo

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// sitemapCacheTTL is the Valkey cache TTL for rendered sitemap XML (1h,
// matching the HTTP Cache-Control max-age). The sitemap:generate Asynq task
// invalidates these keys after pre-generating fresh sitemaps.
// Ref: japaconnect-engineering/sections/26-seo-performance.md.
const sitemapCacheTTL = time.Hour

// sitemapSettingsReader is the subset of config.Settings the sitemap handlers
// need to read the Section 43 quality-gate thresholds (seo.sitemap.*). Declared
// as a local interface so the seo package does not import config (avoids an
// import cycle); *config.Settings satisfies it structurally.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
type sitemapSettingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// SitemapHandler handles sitemap generation requests.
type SitemapHandler struct {
	queries         *gen.Queries
	reader          *pgxpool.Pool
	settings        sitemapSettingsReader
	vkClient        *valkey.Client
	siteURL         string // JC_SITE_URL — configured base URL (Rule 3)
	imgproxyBaseURL string // JC_IMGPROXY_BASE_URL — imgproxy origin for absolute image URLs (Rule 3)
}

// NewSitemapHandler creates a new sitemap handler. settings may be nil —
// handlers fall back to bots.DefaultSitemapQualityConfig() defaults. vkClient
// may be nil — handlers fall back to rendering without cache. siteURL is the
// configured public base URL (JC_SITE_URL); when empty the handler derives
// the base URL from the request host (fallback only). imgproxyBaseURL is the
// configured imgproxy origin (JC_IMGPROXY_BASE_URL); when empty, image
// sitemap URLs are built against the site base URL via the local media serve
// endpoint (Rule 3 — no os.Getenv inside the package).
func NewSitemapHandler(queries *gen.Queries, reader *pgxpool.Pool, settings sitemapSettingsReader, vkClient *valkey.Client, siteURL, imgproxyBaseURL string) *SitemapHandler {
	return &SitemapHandler{queries: queries, reader: reader, settings: settings, vkClient: vkClient, siteURL: siteURL, imgproxyBaseURL: imgproxyBaseURL}
}

// sitemapCacheKey builds the tenant-scoped Valkey cache key for a sitemap
// sub-file. Pattern: jc:cache:sitemap:{tenant_id}:{type}
// (valkey-keys-catalog.md). type is "content", "spaces", "tags", "users",
// "images", or "index".
func sitemapCacheKey(tenantID pgtype.UUID, sitemapType string) string {
	return "jc:cache:sitemap:" + tenantID.String() + ":" + sitemapType
}

// cachedSitemapXML tries to serve a cached sitemap from Valkey. Returns true
// if a cache HIT was served (caller should return immediately). On miss or
// error, returns false so the caller renders fresh XML.
func (h *SitemapHandler) cachedSitemapXML(w http.ResponseWriter, r *http.Request, tenantID pgtype.UUID, sitemapType string) bool {
	if h.vkClient == nil {
		return false
	}

	cached, err := h.vkClient.Redis().Get(r.Context(), sitemapCacheKey(tenantID, sitemapType)).Bytes()
	if err != nil || len(cached) == 0 {
		return false
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cached)

	return true
}

// cacheSitemapXML stores rendered sitemap XML in Valkey. Best-effort — a
// cache write failure is logged but does not fail the request.
func (h *SitemapHandler) cacheSitemapXML(ctx context.Context, tenantID pgtype.UUID, sitemapType string, data []byte) {
	if h.vkClient == nil {
		return
	}

	if err := h.vkClient.Redis().Set(ctx, sitemapCacheKey(tenantID, sitemapType), data, sitemapCacheTTL).Err(); err != nil {
		slog.Warn("sitemap: failed to cache", "type", sitemapType, "error", err)
	}
}

// sitemapCacheInvalidator is the subset of the Valkey/Redis API that
// InvalidateSitemapCache needs. Both *valkey.Client (via .Redis()) and
// valkeycompat.Cmdable satisfy this structurally.
type sitemapCacheInvalidator interface {
	Del(ctx context.Context, keys ...string) *valkeycompat.IntCmd
}

// InvalidateSitemapCache deletes all cached sitemap XML for a tenant. Called
// by the sitemap:generate Asynq task after pre-generating fresh sitemaps so
// the on-demand handlers serve the new content on next request.
// Ref: japaconnect-engineering/sections/26-seo-performance.md.
func InvalidateSitemapCache(ctx context.Context, rdb sitemapCacheInvalidator, tenantID pgtype.UUID) {
	if rdb == nil {
		return
	}

	types := []string{"index", "content", "spaces", "tags", "users", "images"}
	for _, t := range types {
		if err := rdb.Del(ctx, sitemapCacheKey(tenantID, t)).Err(); err != nil {
			slog.Warn("sitemap: failed to invalidate cache", "type", t, "error", err)
		}
	}
}

// baseURL returns the configured site URL, falling back to https://r.Host when
// unset (Rule 3 — no hardcoded config; the fallback is a last resort for
// misconfigured deployments).
func (h *SitemapHandler) baseURL(r *http.Request) string {
	if h.siteURL != "" {
		return strings.TrimRight(h.siteURL, "/")
	}

	return "https://" + r.Host
}

// sitemapQualityConfig reads the seo.sitemap.* thresholds from system_settings
// for the given tenant, falling back to spec defaults when the settings reader
// is unavailable or a key is unset.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func (h *SitemapHandler) sitemapQualityConfig(ctx context.Context, tenantID pgtype.UUID) bots.SitemapQualityConfig {
	cfg := bots.DefaultSitemapQualityConfig()
	if h.settings == nil {
		return cfg
	}

	cfg.MinMembers = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.space_min_members", cfg.MinMembers)
	cfg.MinPosts = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.space_min_posts", cfg.MinPosts)
	cfg.MinForums = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.space_min_forums", cfg.MinForums)
	cfg.MinQuestions = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.space_min_questions", cfg.MinQuestions)
	cfg.MinWords = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.content_min_words", cfg.MinWords)
	cfg.BlogMinWords = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.blog_min_words", cfg.BlogMinWords)
	cfg.ForumMinWords = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.forum_min_words", cfg.ForumMinWords)
	cfg.DiscussionMinWords = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.discussion_min_words", cfg.DiscussionMinWords)
	cfg.GuideMinWords = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.guide_min_words", cfg.GuideMinWords)
	cfg.ComparisonMinWords = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.comparison_min_words", cfg.ComparisonMinWords)
	cfg.BlogMinEngagement = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.blog_min_engagement", cfg.BlogMinEngagement)
	cfg.ForumMinReplies = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.forum_min_replies", cfg.ForumMinReplies)
	cfg.QuestionMinAnswers = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.question_min_answers", cfg.QuestionMinAnswers)
	cfg.DiscussionMinComments = h.settings.GetIntOr(ctx, tenantID, "seo.sitemap.discussion_min_comments", cfg.DiscussionMinComments)

	return cfg
}

// urlset is the root XML element for sitemaps.
type urlset struct {
	XMLName xml.Name   `xml:"urlset"`
	Xmlns   string     `xml:"xmlns,attr"`
	URLs    []urlEntry `xml:"url"`
}

// urlentry represents a single URL in the sitemap.
type urlEntry struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

// sitemapindex is the root XML element for a sitemap index (Section 26 —
// root /sitemap.xml links to sub-sitemaps, not a flat urlset).
type sitemapindex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Xmlns    string         `xml:"xmlns,attr"`
	Sitemaps []sitemapEntry `xml:"sitemap"`
}

// sitemapEntry represents a single sub-sitemap in a sitemap index.
type sitemapEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// ServeSitemap serves the root sitemap index (Section 26). Per the spec, the
// root /sitemap.xml is a <sitemapindex> linking to the sub-sitemaps
// (content, spaces, tags, users, images), NOT a flat <urlset>. The lastmod
// for each sub-sitemap is the most recent updated_at among the entries it
// would contain, so crawlers can skip unchanged chunks. The rendered XML is
// cached in Valkey (jc:cache:sitemap:{tenant_id}:index, TTL 1h) and
// invalidated by the sitemap:generate Asynq task.
// Ref: japaconnect-engineering/sections/26-seo-performance.md.
func (h *SitemapHandler) ServeSitemap(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if h.cachedSitemapXML(w, r, t.ID, "index") {
		return
	}

	base := h.baseURL(r)

	// Compute the lastmod for each sub-sitemap from the most recent row.
	contentLastMod := h.latestContentLastMod(r.Context(), t.ID)
	spacesLastMod := h.latestSpacesLastMod(r.Context(), t.ID)
	usersLastMod := h.latestUsersLastMod(r.Context(), t.ID)
	imagesLastMod := h.latestImagesLastMod(r.Context(), t.ID)

	entries := []sitemapEntry{
		{Loc: base + "/sitemap-content.xml", LastMod: contentLastMod},
		{Loc: base + "/sitemap-spaces.xml", LastMod: spacesLastMod},
		{Loc: base + "/sitemap-tags.xml"},
		{Loc: base + "/sitemap-users.xml", LastMod: usersLastMod},
		{Loc: base + "/sitemap-images.xml", LastMod: imagesLastMod},
	}

	var buf strings.Builder

	buf.WriteString(xml.Header)
	_ = xml.NewEncoder(&buf).Encode(sitemapindex{
		Xmlns:    "http://www.sitemaps.org/schemas/sitemap/0.9",
		Sitemaps: entries,
	})
	data := []byte(buf.String())
	h.cacheSitemapXML(r.Context(), t.ID, "index", data)

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// latestContentLastMod returns the most recent content updated_at as RFC3339,
// or empty string when no published content exists.
func (h *SitemapHandler) latestContentLastMod(ctx context.Context, tenantID pgtype.UUID) string {
	rows, err := h.queries.ListPublishedSlugsForSitemap(ctx)
	if err != nil || len(rows) == 0 {
		return ""
	}

	return rows[0].UpdatedAt.Time.Format(time.RFC3339)
}

// latestSpacesLastMod returns the most recent active space updated_at, or empty.
func (h *SitemapHandler) latestSpacesLastMod(ctx context.Context, tenantID pgtype.UUID) string {
	var ts time.Time

	err := h.reader.QueryRow(ctx, `SELECT updated_at FROM spaces WHERE status = 'active' AND deleted_at IS NULL ORDER BY updated_at DESC LIMIT 1`).Scan(&ts)
	if err != nil {
		return ""
	}

	return ts.Format(time.RFC3339)
}

// latestUsersLastMod returns the most recent active user updated_at, or empty.
func (h *SitemapHandler) latestUsersLastMod(ctx context.Context, tenantID pgtype.UUID) string {
	var ts time.Time

	err := h.reader.QueryRow(ctx, `SELECT u.updated_at FROM users u JOIN user_groups ug ON u.user_group_id = ug.id WHERE u.tenant_id = $1 AND u.status = 'active' AND COALESCE(ug.allow_search_indexing, true) ORDER BY u.updated_at DESC LIMIT 1`, tenantID).Scan(&ts)
	if err != nil {
		return ""
	}

	return ts.Format(time.RFC3339)
}

// latestImagesLastMod returns the most recent updated_at among content rows
// that have a featured image and pass the Section 43 quality gate, or empty.
// Used as the lastmod for /sitemap-images.xml in the sitemap index.
func (h *SitemapHandler) latestImagesLastMod(ctx context.Context, tenantID pgtype.UUID) string {
	var ts time.Time

	err := h.reader.QueryRow(ctx, `SELECT c.updated_at FROM contents c
		 WHERE c.tenant_id = $1 AND c.status = 'published'
		   AND c.visibility = 'public' AND c.seo_indexable = true
		   AND c.featured_image_id IS NOT NULL
		 ORDER BY c.updated_at DESC LIMIT 1`, tenantID).Scan(&ts)
	if err != nil {
		return ""
	}

	return ts.Format(time.RFC3339)
}

// ServeContentSitemap serves content-only sitemap entries (P5.6 — Task 31),
// filtered through the Section 43 per-content-type quality gates
// (bots.IsContentIndexableByType) using the seo.sitemap.* thresholds from
// system_settings. This matches the Asynq sitemap:generate worker's gating
// so the on-demand and pre-generated sitemaps agree.
// Ref: japaconnect-engineering/sections/26-seo-performance.md, sections/43-bot-crawler-management.md.
func (h *SitemapHandler) ServeContentSitemap(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if h.cachedSitemapXML(w, r, t.ID, "content") {
		return
	}

	rows, err := h.queries.ListPublishedSlugsForSitemap(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	cfg := h.sitemapQualityConfig(r.Context(), t.ID)
	base := h.baseURL(r)
	entries := make([]urlEntry, 0, len(rows))

	for _, c := range rows {
		isQuestion := c.IsQuestion != nil && *c.IsQuestion
		if !bots.IsContentIndexableByType(bots.ContentIndexableInput{
			ContentType:   c.ContentType,
			Status:        "published",
			Visibility:    c.Visibility,
			WordCount:     int(c.WordCount),
			CommentCount:  int(c.CommentCount),
			ShareCount:    int(c.ShareCount),
			ReactionCount: int(c.ReactionCount),
			ReplyCount:    int(c.ReplyCount),
			IsQuestion:    isQuestion,
			Solved:        c.Solved,
			SeoIndexable:  c.SeoIndexable,
		}, cfg) {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/content/%s", base, c.Slug),
			LastMod:    c.UpdatedAt.Time.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.8",
		})
	}

	h.serveSitemapBytes(w, r, t.ID, "content", renderURLSetXML(entries))
}

// ServeSpacesSitemap serves space sitemap entries (P5.6 — Task 31), filtered
// through the Section 43 space quality gates (bots.IsSpaceIndexable) using the
// seo.sitemap.space_min_* thresholds from system_settings. A space must meet
// minimum member, post, forum-thread, and solved-question counts to be
// included. Forum and question counts are computed via subqueries against
// forum_threads joined to the space's content.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func (h *SitemapHandler) ServeSpacesSitemap(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if h.cachedSitemapXML(w, r, t.ID, "spaces") {
		return
	}

	cfg := h.sitemapQualityConfig(r.Context(), t.ID)

	rows, err := h.reader.Query(r.Context(), `SELECT s.slug, s.updated_at, s.member_count, s.post_count,
		        COALESCE(fc.forum_count, 0), COALESCE(qc.question_count, 0)
		 FROM spaces s
		 LEFT JOIN (
		     SELECT c.space_id, count(*) AS forum_count
		     FROM contents c JOIN forum_threads ft ON ft.content_id = c.id
		     WHERE c.status = 'published' AND c.deleted_at IS NULL AND c.visibility = 'public'
		     GROUP BY c.space_id
		 ) fc ON fc.space_id = s.id
		 LEFT JOIN (
		     SELECT c.space_id, count(*) AS question_count
		     FROM contents c JOIN forum_threads ft ON ft.content_id = c.id
		     WHERE c.status = 'published' AND c.deleted_at IS NULL AND c.visibility = 'public'
		       AND ft.solved = true
		     GROUP BY c.space_id
		 ) qc ON qc.space_id = s.id
		 WHERE s.tenant_id = $1 AND s.status = 'active' AND s.deleted_at IS NULL
		 ORDER BY s.updated_at DESC`, t.ID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for rows.Next() {
		var slug string

		var updatedAt time.Time

		var memberCount, postCount, forumCount, questionCount int
		if err := rows.Scan(&slug, &updatedAt, &memberCount, &postCount, &forumCount, &questionCount); err != nil {
			continue
		}

		if !bots.IsSpaceIndexable(memberCount, postCount, forumCount, questionCount,
			cfg.MinMembers, cfg.MinPosts, cfg.MinForums, cfg.MinQuestions) {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/spaces/%s", base, slug),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.7",
		})
	}

	h.serveSitemapBytes(w, r, t.ID, "spaces", renderURLSetXML(entries))
}

// ServeTagsSitemap serves tag sitemap entries (P5.6 — Task 31).
func (h *SitemapHandler) ServeTagsSitemap(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if h.cachedSitemapXML(w, r, t.ID, "tags") {
		return
	}

	rows, err := h.reader.Query(r.Context(), `SELECT slug FROM tags  ORDER BY name`)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/tags/%s", base, slug),
			ChangeFreq: "monthly",
			Priority:   "0.5",
		})
	}

	h.serveSitemapBytes(w, r, t.ID, "tags", renderURLSetXML(entries))
}

// ServeUsersSitemap serves user profile sitemap entries (P5.6 — Task 31),
// filtered by the UserGroup-level allow_search_indexing gate (Section 26:
// "admin sets allow_search_indexing per UserGroup. New/unverified members'
// posts excluded from sitemap"). Users whose UserGroup has
// allow_search_indexing = false are excluded.
// Ref: japaconnect-engineering/sections/26-seo-performance.md.
func (h *SitemapHandler) ServeUsersSitemap(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if h.cachedSitemapXML(w, r, t.ID, "users") {
		return
	}

	rows, err := h.reader.Query(r.Context(), `SELECT u.username, u.updated_at FROM users u
		 JOIN user_groups ug ON u.user_group_id = ug.id
		 WHERE u.tenant_id = $1 AND u.status = 'active'
		   AND COALESCE(ug.allow_search_indexing, true) = true
		 ORDER BY u.updated_at DESC`, t.ID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	defer rows.Close()

	base := h.baseURL(r)
	entries := make([]urlEntry, 0)

	for rows.Next() {
		var username string

		var updatedAt time.Time
		if err := rows.Scan(&username, &updatedAt); err != nil {
			continue
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/u/%s", base, username),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "monthly",
			Priority:   "0.6",
		})
	}

	h.serveSitemapBytes(w, r, t.ID, "users", renderURLSetXML(entries))
}

// imageURLSet is a urlset with the image namespace, for image sitemaps.
// Ref: japaconnect-engineering/sections/26-seo-performance.md (image sitemaps).
type imageURLSet struct {
	XMLName    xml.Name        `xml:"urlset"`
	Xmlns      string          `xml:"xmlns,attr"`
	XmlnsImage string          `xml:"xmlns:image,attr"`
	URLs       []imageURLEntry `xml:"url"`
}

// imageURLEntry is a url entry with optional image sub-elements.
type imageURLEntry struct {
	Loc     string      `xml:"loc"`
	LastMod string      `xml:"lastmod,omitempty"`
	Images  []imageItem `xml:"image,omitempty"`
}

// imageItem is a single <image:image> entry inside a url.
type imageItem struct {
	Loc     string `xml:"image:loc"`
	Title   string `xml:"image:title,omitempty"`
	Caption string `xml:"image:caption,omitempty"`
}

// ServeImagesSitemap serves an image sitemap (Section 26: "Image sitemaps
// included for content with featured images. Regenerated alongside main
// sitemap."). For each published, public, indexable content row that has a
// featured_image_id, emits a <url> with the content's canonical URL and an
// <image:image> sub-element pointing to the featured image's media URL.
// The media URL is resolved via the imgproxy/local-serve fallback
// (media.MediaURL) using the media row's storage_bucket/storage_path.
// Ref: japaconnect-engineering/sections/26-seo-performance.md.
func (h *SitemapHandler) ServeImagesSitemap(w http.ResponseWriter, r *http.Request) {
	t := &struct{ ID pgtype.UUID }{}
	if h.cachedSitemapXML(w, r, t.ID, "images") {
		return
	}

	base := h.baseURL(r)

	rows, err := h.reader.Query(r.Context(), `SELECT c.slug, c.updated_at, c.title,
		        m.storage_bucket, m.storage_path, COALESCE(m.alt_text, c.title)
		 FROM contents c
		 JOIN media m ON c.featured_image_id = m.id AND m.tenant_id = c.tenant_id
		 WHERE c.tenant_id = $1 AND c.status = 'published'
		   AND c.deleted_at IS NULL
		   AND c.visibility = 'public' AND c.seo_indexable = true
		   AND c.featured_image_id IS NOT NULL
		   AND m.deleted_at IS NULL AND m.confirmed = true
		 ORDER BY c.updated_at DESC`, t.ID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate image sitemap")
		return
	}

	defer rows.Close()

	entries := make([]imageURLEntry, 0)

	for rows.Next() {
		var slug, title, storageBucket, storagePath, altText string

		var updatedAt time.Time
		if err := rows.Scan(&slug, &updatedAt, &title, &storageBucket, &storagePath, &altText); err != nil {
			continue
		}

		imgURL := resolveMediaURL(h.imgproxyBaseURL, base, storageBucket, storagePath)
		entries = append(entries, imageURLEntry{
			Loc:     fmt.Sprintf("%s/content/%s", base, slug),
			LastMod: updatedAt.Format(time.RFC3339),
			Images: []imageItem{
				{Loc: imgURL, Title: title, Caption: altText},
			},
		})
	}

	var buf strings.Builder

	buf.WriteString(xml.Header)
	_ = xml.NewEncoder(&buf).Encode(imageURLSet{
		Xmlns:      "http://www.sitemaps.org/schemas/sitemap/0.9",
		XmlnsImage: "http://www.google.com/schemas/sitemap-image/1.1",
		URLs:       entries,
	})

	h.serveSitemapBytes(w, r, t.ID, "images", []byte(buf.String()))
}

// resolveMediaURL builds a public, absolute media URL from
// storage_bucket/storage_path. It mirrors media.MediaURL's fallback path
// (local serve endpoint) without importing the media package (avoids an
// import cycle). When imgproxy is configured (imgproxyBaseURL non-empty),
// the URL is absolute against the imgproxy origin; otherwise the local
// /api/v1/media/serve/ path is prefixed with the sitemap base URL so the
// emitted <image:loc> is always absolute (sitemaps.org protocol requires
// absolute URLs — relative URLs are rejected by Google Search Console).
// Ref: japaconnect-engineering/sections/26-seo-performance.md.
func resolveMediaURL(imgproxyBaseURL, siteBaseURL, storageBucket, storagePath string) string {
	if imgproxyBaseURL != "" {
		// Defer to the full imgproxy URL construction. We can't call
		// media.MediaURL here (import cycle), so we build the raw source
		// URL — imgproxy signs it, but for sitemap purposes the unsigned
		// source URL is sufficient because crawlers fetch via the site.
		return strings.TrimRight(imgproxyBaseURL, "/") + "/" + storageBucket + "/" + storagePath
	}

	return strings.TrimRight(siteBaseURL, "/") + "/api/v1/media/serve/" + storageBucket + "/" + storagePath
}

// renderURLSetXML renders a urlset to XML bytes (with the XML header) for
// caching and serving. Used by all sub-sitemap handlers.
func renderURLSetXML(entries []urlEntry) []byte {
	var buf strings.Builder

	buf.WriteString(xml.Header)
	_ = xml.NewEncoder(&buf).Encode(urlset{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  entries,
	})

	return []byte(buf.String())
}

// serveSitemapBytes writes pre-rendered sitemap XML bytes to the response,
// sets standard headers, and caches the bytes in Valkey under the given type.
func (h *SitemapHandler) serveSitemapBytes(w http.ResponseWriter, r *http.Request, tenantID pgtype.UUID, sitemapType string, data []byte) {
	h.cacheSitemapXML(r.Context(), tenantID, sitemapType, data)
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *SitemapHandler) writeXML(w http.ResponseWriter, entries []urlEntry) {
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_ = xml.NewEncoder(w).Encode(urlset{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  entries,
	})
}

// ValidateSEO handles GET /admin/seo/validate/:id — validates SEO metadata for
// a content item (Section 26: "validate JSON-LD output for a content item").
//
// Ref: japaconnect-engineering/sections/26-seo-performance.md
//
// The route validates a CONTENT item, not a pagebuilder page: the canonical
// SEO metadata (meta_title, meta_description, og_image_url, canonical_url,
// seo_indexable) lives on the `contents` table (migration 000165), and the
// JSON-LD template is keyed by content_type via content_type_jsonld_templates.
// The previous implementation queried the `pages` table and returned only
// {id, valid, issues}, which did not match the admin SPA contract
// (SeoValidationResult: type, title, slug, all meta fields, issues, score).
//
// Render-time resolution follows Section 26's ResolveMetadata:
//   - meta_title  -> coalesce(meta_title, title)
//   - og_title    -> coalesce(meta_title, title)   (no og_title column; falls back to title)
//   - meta_description / og_description -> meta_description (or body excerpt)
//   - og_image_url -> og_image_url (caller may resolve featured_image fallback)
//   - canonical_url -> canonical_url (self-referencing fallback handled by caller)
//   - robots_meta  -> "noindex,nofollow" for private visibility,
//     "noindex,follow" when seo_indexable = false,
//     "index,follow" otherwise
//   - json_ld_type -> @type extracted from the active content_type_jsonld_templates row
//
// Score: 100 minus 12 points per issue (floor 0). The admin SPA renders the
// score as a circular progress indicator and lists each issue.
func (h *SitemapHandler) ValidateSEO(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	t := &struct{ ID pgtype.UUID }{}
	if t == nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "tenant_unavailable", "No tenant available")
		return
	}

	// Query the contents row with render-time resolution (Section 26).
	// Uses raw pgxpool (the handler's existing pattern) until the
	// sidebar_widgets sqlc blocker is resolved; the equivalent sqlc query
	// is documented in internal/db/queries/seo.sql (GetContentSEOByID).
	const q = `
		SELECT c.id, c.title, c.slug, c.content_type, c.body, c.visibility, c.seo_indexable,
		       COALESCE(c.meta_title, '') AS meta_title,
		       COALESCE(c.meta_description, '') AS meta_description,
		       COALESCE(c.og_image_url, '') AS og_image_url,
		       COALESCE(c.canonical_url, '') AS canonical_url,
		       COALESCE(c.meta_title, c.title) AS og_title,
		       COALESCE(c.meta_description, '') AS og_description,
		       COALESCE(jt.template, '{}'::jsonb) AS jsonld_template
		FROM contents c
		LEFT JOIN content_type_jsonld_templates jt
		       ON jt.content_type = c.content_type
		      AND jt.tenant_id = c.tenant_id
		      AND jt.is_active = true
		WHERE c.id = $1 AND c.tenant_id = $2`

	var (
		contentID      string
		title          string
		slug           string
		contentType    string
		body           string
		visibility     string
		seoIndexable   bool
		metaTitle      string
		metaDesc       string
		ogImageURL     string
		canonicalURL   string
		ogTitle        string
		ogDescription  string
		jsonldTemplate []byte
	)

	err := h.reader.QueryRow(r.Context(), q, id, t.ID).Scan(
		&contentID, &title, &slug, &contentType, &body, &visibility, &seoIndexable,
		&metaTitle, &metaDesc, &ogImageURL, &canonicalURL, &ogTitle, &ogDescription, &jsonldTemplate,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
		return
	}

	// Resolve metadata per Section 26 ResolveMetadata.
	resolvedMetaTitle := metaTitle
	if resolvedMetaTitle == "" {
		resolvedMetaTitle = title
	}

	resolvedMetaDesc := metaDesc
	if resolvedMetaDesc == "" {
		resolvedMetaDesc = excerpt(body, 160)
	}

	resolvedOGTitle := ogTitle
	if resolvedOGTitle == "" {
		resolvedOGTitle = title
	}

	resolvedOGDesc := ogDescription
	if resolvedOGDesc == "" {
		resolvedOGDesc = resolvedMetaDesc
	}

	// robots_meta per Section 26: private -> noindex,nofollow;
	// seo_indexable false -> noindex,follow; otherwise index,follow.
	var robotsMeta string

	switch {
	case visibility == "private":
		robotsMeta = "noindex,nofollow"
	case !seoIndexable:
		robotsMeta = "noindex,follow"
	default:
		robotsMeta = "index,follow"
	}

	// json_ld_type: extract @type from the active template's top-level @type
	// field. Templates are admin-defined per content type (Section 14).
	jsonLDType := ""

	if len(jsonldTemplate) > 0 {
		var tpl struct {
			Type string `json:"@type"`
		}

		if json.Unmarshal(jsonldTemplate, &tpl) == nil {
			jsonLDType = tpl.Type
		}
	}
	// Fallback to the Section 26 schema mapping when no admin template is set.
	if jsonLDType == "" {
		switch contentType {
		case "blog_post":
			jsonLDType = "BlogPosting"
		case "discussion", "question":
			jsonLDType = "QAPage"
		default:
			jsonLDType = "Article"
		}
	}

	// Build the issues list. Each missing/empty SEO field is one issue,
	// and each present-but-too-short or too-long field is a separate issue.
	// Per Section 26 ResolveMetadata, canonical_url and og_image_url have
	// platform-level fallbacks (self-referencing URL and default banner),
	// so an empty value is NOT a "missing" issue — the platform auto-resolves
	// them. Only fields without fallbacks are flagged as missing.
	// Minimum/maximum lengths mirror the client-side computeSeoScore utility
	// (apps/main/src/lib/utils/seo-score.ts) per common SEO best practices.
	issues := []string{}
	if title == "" {
		issues = append(issues, "missing_title")
	}

	if slug == "" {
		issues = append(issues, "missing_slug")
	}

	if resolvedMetaTitle == "" {
		issues = append(issues, "missing_meta_title")
	}

	if resolvedMetaDesc == "" {
		issues = append(issues, "missing_meta_description")
	}

	if resolvedOGTitle == "" {
		issues = append(issues, "missing_og_title")
	}

	if resolvedOGDesc == "" {
		issues = append(issues, "missing_og_description")
	}

	if jsonLDType == "" {
		issues = append(issues, "missing_json_ld_type")
	}

	if robotsMeta == "noindex,nofollow" || robotsMeta == "noindex,follow" {
		issues = append(issues, "not_indexable")
	}

	// Minimum length checks (only for present, non-empty fields).
	if title != "" && len(title) < 10 {
		issues = append(issues, "short_title")
	}

	if slug != "" && len(slug) < 3 {
		issues = append(issues, "short_slug")
	}

	if resolvedMetaTitle != "" && len(resolvedMetaTitle) < 30 {
		issues = append(issues, "short_meta_title")
	}

	if resolvedMetaDesc != "" && len(resolvedMetaDesc) < 70 {
		issues = append(issues, "short_meta_description")
	}

	if resolvedOGTitle != "" && len(resolvedOGTitle) < 30 {
		issues = append(issues, "short_og_title")
	}

	if resolvedOGDesc != "" && len(resolvedOGDesc) < 70 {
		issues = append(issues, "short_og_description")
	}

	// Maximum length checks (over-limit fields).
	if resolvedMetaTitle != "" && len(resolvedMetaTitle) > 60 {
		issues = append(issues, "long_meta_title")
	}

	if resolvedMetaDesc != "" && len(resolvedMetaDesc) > 160 {
		issues = append(issues, "long_meta_description")
	}

	// Score: 100 - 12 per issue, floored at 0.
	score := 100 - len(issues)*12
	if score < 0 {
		score = 0
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"id":               contentID,
			"type":             contentType,
			"title":            title,
			"slug":             slug,
			"meta_title":       resolvedMetaTitle,
			"meta_description": resolvedMetaDesc,
			"og_title":         resolvedOGTitle,
			"og_description":   resolvedOGDesc,
			"og_image_url":     ogImageURL,
			"canonical_url":    canonicalURL,
			"json_ld_type":     jsonLDType,
			"robots_meta":      robotsMeta,
			"issues":           issues,
			"score":            score,
		},
	})
}

// excerpt returns the first n characters of body with HTML/markdown noise
// stripped, for meta_description fallback per Section 26 ResolveMetadata.
// Truncation is rune-safe (not byte-safe) so multi-byte UTF-8 characters
// (French accents, Nigerian names) are never split mid-codepoint.
func excerpt(body string, n int) string {
	if body == "" {
		return ""
	}
	// Strip common markdown markers and collapse whitespace. A full
	// strip_markdown lives in internal/content (SanitizeText /
	// ComputeContentHash); this is a lightweight fallback for the
	// validator only.
	s := body
	for _, m := range []string{"#", "*", "_", "`", ">", "- "} {
		s = strings.ReplaceAll(s, m, "")
	}

	s = strings.Join(strings.Fields(s), " ")
	// Rune-safe truncation: slice by character count, not byte count,
	// to avoid producing invalid UTF-8 (Section 7 i18n bug class).
	if r := []rune(s); len(r) > n {
		s = string(r[:n])
	}

	return s
}

// writeError is defined in ogimage.go and shared across the seo package.
