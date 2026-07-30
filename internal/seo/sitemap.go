package seo

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
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

// ValidateSEO handles GET /admin/seo/validate/:id — validates SEO metadata for a page (P5.6).
func (h *SitemapHandler) ValidateSEO(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusInternalServerError, "tenant_unavailable", "No tenant available")
		return
	}
	var title, slug string
	var canonicalURL *string
	err := h.reader.QueryRow(r.Context(),
		`SELECT title, slug, canonical_url FROM contents WHERE id = $1 AND tenant_id = $2`, id, t.ID).
		Scan(&title, &slug, &canonicalURL)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Page not found")
		return
	}
	issues := []string{}
	if title == "" {
		issues = append(issues, "missing_title")
	}
	if slug == "" {
		issues = append(issues, "missing_slug")
	}
	if canonicalURL == nil || *canonicalURL == "" {
		issues = append(issues, "missing_canonical_url")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"id":     id,
			"valid":  len(issues) == 0,
			"issues": issues,
		},
	})
}

// writeError is defined in ogimage.go and shared across the seo package.
