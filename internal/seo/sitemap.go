package seo

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// SitemapHandler handles sitemap generation requests.
type SitemapHandler struct {
	queries *gen.Queries
	reader  *pgxpool.Pool
}

// NewSitemapHandler creates a new sitemap handler.
func NewSitemapHandler(queries *gen.Queries, reader *pgxpool.Pool) *SitemapHandler {
	return &SitemapHandler{queries: queries, reader: reader}
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

// ServeSitemap generates and serves an XML sitemap for the tenant.
func (h *SitemapHandler) ServeSitemap(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())

	contentList, err := h.queries.ListContentForSitemap(r.Context(), t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}

	baseURL := "https://" + r.Host
	entries := make([]urlEntry, 0, len(contentList)+1)

	// Homepage
	entries = append(entries, urlEntry{
		Loc:        baseURL,
		LastMod:    time.Now().UTC().Format(time.RFC3339),
		ChangeFreq: "daily",
		Priority:   "1.0",
	})

	// Content pages
	for _, content := range contentList {
		lastMod := time.Now().UTC().Format(time.RFC3339)
		if content.UpdatedAt.Valid {
			lastMod = content.UpdatedAt.Time.Format(time.RFC3339)
		}

		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/%s/%s", baseURL, content.ContentType, content.Slug),
			LastMod:    lastMod,
			ChangeFreq: "weekly",
			Priority:   "0.8",
		})
	}

	h.writeXML(w, entries)
}

// ServeContentSitemap serves content-only sitemap entries (P5.6 — Task 31).
func (h *SitemapHandler) ServeContentSitemap(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	contentList, err := h.queries.ListContentForSitemap(r.Context(), t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}
	baseURL := "https://" + r.Host
	entries := make([]urlEntry, 0, len(contentList))
	for _, c := range contentList {
		lastMod := time.Now().UTC().Format(time.RFC3339)
		if c.UpdatedAt.Valid {
			lastMod = c.UpdatedAt.Time.Format(time.RFC3339)
		}
		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/%s/%s", baseURL, c.ContentType, c.Slug),
			LastMod:    lastMod,
			ChangeFreq: "weekly",
			Priority:   "0.8",
		})
	}
	h.writeXML(w, entries)
}

// ServeSpacesSitemap serves space sitemap entries (P5.6 — Task 31).
func (h *SitemapHandler) ServeSpacesSitemap(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	rows, err := h.reader.Query(r.Context(),
		`SELECT slug, updated_at FROM spaces WHERE tenant_id = $1 AND status = 'active' ORDER BY updated_at DESC`, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}
	defer rows.Close()
	baseURL := "https://" + r.Host
	entries := make([]urlEntry, 0)
	for rows.Next() {
		var slug string
		var updatedAt time.Time
		if err := rows.Scan(&slug, &updatedAt); err != nil {
			continue
		}
		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/spaces/%s", baseURL, slug),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "weekly",
			Priority:   "0.7",
		})
	}
	h.writeXML(w, entries)
}

// ServeTagsSitemap serves tag sitemap entries (P5.6 — Task 31).
func (h *SitemapHandler) ServeTagsSitemap(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	rows, err := h.reader.Query(r.Context(),
		`SELECT slug FROM tags WHERE tenant_id = $1 ORDER BY name`, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}
	defer rows.Close()
	baseURL := "https://" + r.Host
	entries := make([]urlEntry, 0)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			continue
		}
		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/tags/%s", baseURL, slug),
			ChangeFreq: "monthly",
			Priority:   "0.5",
		})
	}
	h.writeXML(w, entries)
}

// ServeUsersSitemap serves user profile sitemap entries (P5.6 — Task 31).
func (h *SitemapHandler) ServeUsersSitemap(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	rows, err := h.reader.Query(r.Context(),
		`SELECT username, updated_at FROM users WHERE tenant_id = $1 AND status = 'active' ORDER BY updated_at DESC`, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to generate sitemap")
		return
	}
	defer rows.Close()
	baseURL := "https://" + r.Host
	entries := make([]urlEntry, 0)
	for rows.Next() {
		var username string
		var updatedAt time.Time
		if err := rows.Scan(&username, &updatedAt); err != nil {
			continue
		}
		entries = append(entries, urlEntry{
			Loc:        fmt.Sprintf("%s/users/%s", baseURL, username),
			LastMod:    updatedAt.Format(time.RFC3339),
			ChangeFreq: "monthly",
			Priority:   "0.6",
		})
	}
	h.writeXML(w, entries)
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
//                     "noindex,follow" when seo_indexable = false,
//                     "index,follow" otherwise
//   - json_ld_type -> @type extracted from the active content_type_jsonld_templates row
//
// Score: 100 minus 12 points per issue (floor 0). The admin SPA renders the
// score as a circular progress indicator and lists each issue.
func (h *SitemapHandler) ValidateSEO(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusInternalServerError, "tenant_unavailable", "No tenant available")
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
		writeError(w, http.StatusNotFound, "not_found", "Content not found")
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
			"id":              contentID,
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
	if len(s) > n {
		s = s[:n]
	}
	return s
}

// writeError is defined in ogimage.go and shared across the seo package.
