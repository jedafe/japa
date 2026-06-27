package seo

import (
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

// Route mounts the sitemap routes.
func (h *SitemapHandler) Route(r chi.Router) {
	r.Get("/sitemap.xml", h.ServeSitemap)
}

// urlset is the root XML element for sitemaps.
type urlset struct {
	XMLName xml.Name    `xml:"urlset"`
	Xmlns   string      `xml:"xmlns,attr"`
	URLs    []urlEntry `xml:"url"`
}

// urlentry represents a single URL in the sitemap.
type urlEntry struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

// ServeSitemap generates and serves an XML sitemap for the tenant.
func (h *SitemapHandler) ServeSitemap(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())

	contentList, err := h.queries.ListContentForSitemap(r.Context(), t.ID)
	if err != nil {
		http.Error(w, "Failed to generate sitemap", http.StatusInternalServerError)
		return
	}

	baseURL := fmt.Sprintf("https://%s", r.Host)
	entries := make([]urlEntry, 0, len(contentList)+1)

	// Homepage
	entries = append(entries, urlEntry{
		Loc:        baseURL,
		LastMod:    time.Now().Format(time.RFC3339),
		ChangeFreq: "daily",
		Priority:   "1.0",
	})

	// Content pages
	for _, content := range contentList {
		lastMod := time.Now().Format(time.RFC3339)
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

	urlset := urlset{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  entries,
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Cache-Control", "public, max-age=3600") // 1 hour cache
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(urlset)
}