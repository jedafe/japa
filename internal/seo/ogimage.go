package seo

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tools"
	"github.com/jedafe/japa/internal/valkey"
)

// ogCacheTTL is the Valkey cache TTL for rendered OG images (Section 26: 24h).
const ogCacheTTL = 24 * time.Hour

// OGImageHandler handles OG image generation requests.
type OGImageHandler struct {
	reader   *pgxpool.Pool
	vkClient *valkey.Client
	face     font.Face
	siteName string
}

// NewOGImageHandler creates a new OG image handler. The vkClient enables
// Valkey caching of rendered images (Section 26: key jc:og:{key},
// TTL 24h) so repeated fetches don't re-render. The font face
// is loaded once at construction from the embedded Go Regular font
// (golang.org/x/image/font/gofont/goregular). If font loading fails the
// handler still works — text rendering degrades gracefully to a fallback
// bitmap approach.
func NewOGImageHandler(reader *pgxpool.Pool, vkClient *valkey.Client, siteName string) *OGImageHandler {
	face := loadFontFace(48)
	return &OGImageHandler{reader: reader, vkClient: vkClient, face: face, siteName: siteName}
}

// loadFontFace creates an opentype font face at the given size from the
// embedded Go Regular font data.
func loadFontFace(size float64) font.Face {
	ft, err := opentype.Parse(goregular.TTF)
	if err != nil {
		slog.Warn("og-image: failed to parse Go Regular font", "error", err)
		return nil
	}

	face, err := opentype.NewFace(ft, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		slog.Warn("og-image: failed to create font face", "error", err)
		return nil
	}

	return face
}

// contentMeta holds the minimal data needed to render an OG image.
type contentMeta struct {
	Title       string
	ContentType string
}

// lookup resolves an /og/{key}.png key to the title card it should render.
// Keys are namespaced so arbitrary path segments can never collide with
// content UUIDs:
//
//	tool:{slug}    indexable /tools/{slug} page — registry title card
//	space:{slug}   /wiki/{slug} + /universities/{slug} index — space-name card
//	user:{user}    /u/{username} profile — display-name card
//	{type}:{slug}  entity title card (product/tag/source/org/scholarship)
//	{type}:{uuid}  entity title card (calendar event, scam alert)
//	{uuid}         contents row, then wiki_pages row (space + entity pages)
//
// Every entity query gates on the same public/active predicate that
// entity's public page uses. Unknown or unresolvable keys return nil →
// the brand fallback card.
func (h *OGImageHandler) lookup(ctx context.Context, idStr string) *contentMeta {
	if slug, ok := strings.CutPrefix(idStr, "tool:"); ok {
		p, found := tools.Lookup(slug)
		if !found {
			return nil
		}
		return &contentMeta{Title: p.Title, ContentType: "tool"}
	}
	// Every remaining resolver hits the database.
	if h.reader == nil {
		return nil
	}
	if slug, ok := strings.CutPrefix(idStr, "space:"); ok {
		var name string
		err := h.reader.QueryRow(ctx,
			`SELECT name FROM spaces WHERE slug = $1 AND status = 'active' AND deleted_at IS NULL`, slug).Scan(&name)
		if err != nil || name == "" {
			return nil
		}
		return &contentMeta{Title: name, ContentType: "destination_wiki"}
	}
	if slug, ok := strings.CutPrefix(idStr, "user:"); ok {
		var username, displayName string
		err := h.reader.QueryRow(ctx,
			`SELECT u.username, COALESCE(up.display_name, '')
			 FROM users u LEFT JOIN user_profiles up ON up.user_id = u.id
			 WHERE u.username = $1 AND u.status = 'active'`, slug).Scan(&username, &displayName)
		if err != nil {
			return nil
		}
		if displayName == "" {
			displayName = "@" + username
		}
		return &contentMeta{Title: displayName, ContentType: "profile"}
	}
	if slug, ok := strings.CutPrefix(idStr, "scam-alert:"); ok {
		var scamType string
		err := h.reader.QueryRow(ctx,
			`SELECT scam_type FROM scam_reports WHERE id::text = $1 AND status = 'published' AND deleted_at IS NULL`, slug).Scan(&scamType)
		if err != nil || scamType == "" {
			return nil
		}
		return &contentMeta{Title: humanizeLabel(scamType), ContentType: "scam_alert"}
	}
	// Shared slug→title resolvers — each gates on its public page's
	// predicate. {prefix}:{param} → single title column.
	simple := []struct {
		prefix string
		query  string
		ctype  string
	}{
		{"product:", `SELECT name FROM products WHERE slug = $1 AND deleted_at IS NULL`, "product"},
		{"tag:", `SELECT name FROM tags WHERE slug = $1`, "tag"},
		{"source:", `SELECT name FROM intel_sources WHERE slug = $1 AND status = 'active'`, "source"},
		{"org:", `SELECT name FROM organizations WHERE slug = $1 AND status = 'active' AND deleted_at IS NULL`, "organization"},
		{"scholarship:", `SELECT title FROM scholarships WHERE slug = $1`, "scholarship"},
		{"event:", `SELECT title FROM calendar_entries WHERE id::text = $1 AND status = 'published' AND deleted_at IS NULL`, "event"},
	}
	for _, r := range simple {
		if param, ok := strings.CutPrefix(idStr, r.prefix); ok {
			var title string
			if err := h.reader.QueryRow(ctx, r.query, param).Scan(&title); err != nil || title == "" {
				return nil
			}
			return &contentMeta{Title: title, ContentType: r.ctype}
		}
	}
	if meta := h.lookupContent(ctx, idStr); meta != nil {
		return meta
	}
	return h.lookupWikiPage(ctx, idStr)
}

// humanizeLabel renders a snake_case slug/enum as a title ("fake_landlord"
// → "Fake Landlord") for OG card text.
func humanizeLabel(s string) string {
	parts := strings.Fields(strings.ReplaceAll(s, "_", " "))
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// lookupContent queries the contents table for the title and content_type
// of the given content ID within the platform. Returns nil if the ID is
// invalid, the content does not exist, or the content is private /
// non-indexable (Section 26: private content must not leak its title via
// a publicly served OG image).
func (h *OGImageHandler) lookupContent(ctx context.Context, idStr string) *contentMeta {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil
	}

	var pgID pgtype.UUID
	if err := pgID.Scan(id.String()); err != nil {
		return nil
	}

	var meta contentMeta

	var visibility string

	var seoIndexable bool

	err = h.reader.QueryRow(ctx, `SELECT title, content_type, visibility, seo_indexable
		 FROM contents WHERE id = $1`, pgID).Scan(&meta.Title, &meta.ContentType, &visibility, &seoIndexable)
	if err != nil {
		return nil
	}

	// Section 26: private or non-indexable content must not surface a
	// content-specific OG image. Return nil so the caller falls back to
	// the default brand image instead of leaking the content title.
	if visibility == "private" || !seoIndexable {
		return nil
	}

	return &meta
}

// lookupWikiPage resolves a wiki_pages UUID (space-bound and entity-bound
// wiki pages share the table) to its title card. Inactive/deleted pages
// return nil → brand fallback.
func (h *OGImageHandler) lookupWikiPage(ctx context.Context, idStr string) *contentMeta {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil
	}

	var pgID pgtype.UUID
	if err := pgID.Scan(id.String()); err != nil {
		return nil
	}

	var meta contentMeta
	err = h.reader.QueryRow(ctx,
		`SELECT title FROM wiki_pages WHERE id = $1 AND is_active = true AND deleted_at IS NULL`,
		pgID).Scan(&meta.Title)
	if err != nil {
		return nil
	}
	meta.ContentType = "wiki_page"
	return &meta
}

// ogCacheKey builds the Valkey cache key for an OG image.
// Pattern: jc:og:{key} — key is a namespaced slug (tool:{slug},
// space:{slug}) or a contents/wiki_pages UUID (valkey-keys-catalog).
func ogCacheKey(key string) string {
	return "jc:og:" + key
}

// ogBrandCacheKey is the fixed Valkey key for the brand-fallback OG image,
// used for invalid, private, and non-indexable content IDs.
const ogBrandCacheKey = "jc:og:default"

// ServeOGImage generates and serves an OG image for the given key.
// Rendered images are cached in Valkey (key jc:og:{key},
// TTL 24h per Section 26) so repeated fetches by social crawlers don't
// re-render. If the content is found and indexable, its title is rendered
// as the main text; otherwise a default brand image is served. Private or
// non-indexable content always gets the default brand image (no title leak).
func (h *OGImageHandler) ServeOGImage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")

	// Try Valkey cache first (Section 26).
	if h.vkClient != nil {
		cacheKey := ogCacheKey(idStr)
		if cached, err := h.vkClient.Redis().Get(r.Context(), cacheKey).Bytes(); err == nil && len(cached) > 0 {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=86400")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			// #nosec G705 // cached is a server-rendered PNG, not user-controlled HTML/JS.
			_, _ = w.Write(cached)

			return
		}
	}

	var title, subtitle string

	isBrandFallback := false

	if meta := h.lookup(r.Context(), idStr); meta != nil {
		title = cleanTitle(meta.Title)
		subtitle = humanizeContentType(meta.ContentType)
	}

	// Fallback to brand defaults when no content matched (or content is
	// private/non-indexable — lookupContent returns nil in that case).
	if title == "" {
		// Serve the cached brand image when present — re-rendering a PNG on
		// every request for invalid/private IDs is a CPU-DoS vector.
		if h.vkClient != nil {
			if cached, err := h.vkClient.Redis().Get(r.Context(), ogBrandCacheKey).Bytes(); err == nil && len(cached) > 0 {
				w.Header().Set("Content-Type", "image/png")
				w.Header().Set("Cache-Control", "public, max-age=86400")
				w.Header().Set("X-Cache", "HIT")
				w.WriteHeader(http.StatusOK)
				// #nosec G705 // cached is a server-rendered PNG.
				_, _ = w.Write(cached)

				return
			}
		}

		title = h.siteName
		subtitle = "Community Platform"
		isBrandFallback = true
	}

	img := generateOGImage(title, subtitle, h.face)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		slog.Error("og-image: png encode failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to generate image")

		return
	}

	// Cache the rendered PNG in Valkey (Section 26: TTL 24h). Content images
	// key by content id; the brand fallback keys under a fixed "default" key
	// so repeated misses on invalid IDs serve the cached image instead of
	// re-rendering.
	if h.vkClient != nil {
		cacheKey := ogBrandCacheKey
		if !isBrandFallback {
			cacheKey = ogCacheKey(idStr)
		}

		if err := h.vkClient.Redis().Set(r.Context(), cacheKey, buf.Bytes(), ogCacheTTL).Err(); err != nil {
			slog.Warn("og-image: failed to cache", "content_id", idStr, "error", err)
		}
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.Warn("og-image: write failed", "error", err)
	}
}

// generateOGImage creates a 1200x630 OG image with real font rendering.
func generateOGImage(title, subtitle string, face font.Face) *image.RGBA {
	const (
		width  = 1200
		height = 630
	)

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with brand color (slate-800).
	brandBg := color.RGBA{R: 30, G: 41, B: 59, A: 255}

	for y := range height {
		for x := range width {
			img.Set(x, y, brandBg)
		}
	}

	// Draw accent stripe at bottom (indigo-500).
	accent := color.RGBA{R: 99, G: 102, B: 241, A: 255}
	stripeHeight := 8

	for y := height - stripeHeight; y < height; y++ {
		for x := range width {
			img.Set(x, y, accent)
		}
	}

	// Render text with real font.
	titleColor := color.White
	subtitleColor := color.RGBA{R: 148, G: 163, B: 184, A: 255} // slate-400

	drawString(img, face, 80, 280, title, titleColor)
	drawString(img, face, 80, 340, subtitle, subtitleColor)

	return img
}

// drawString renders text onto the image using the provided font face.
// If face is nil, it falls back to bitmap rectangles per character.
func drawString(img *image.RGBA, face font.Face, x, y int, text string, clr color.Color) {
	if face != nil {
		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(clr),
			Face: face,
			Dot:  fixed.P(x, y),
		}
		d.DrawString(text)

		return
	}

	// Fallback: bitmap rectangles when font face is unavailable.
	const charW, charH, spacing = 10, 18, 4

	for i, ch := range text {
		if ch == ' ' {
			continue
		}

		cx := x + i*(charW+spacing)

		for dy := range charH {
			for dx := range charW {
				img.Set(cx+dx, y-charH+dy, clr)
			}
		}
	}
}

// truncateString truncates a string to maxLen runes — multi-byte UTF-8
// characters (accents, non-Latin names) are never split mid-codepoint.
func truncateString(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}

	return string(r[:maxLen-3]) + "..."
}

// cleanTitle removes special characters and truncates for display.
func cleanTitle(title string) string {
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")
	title = strings.Join(strings.Fields(title), " ")

	return truncateString(title, 60)
}

// humanizeContentType converts a content_type slug to a human-readable label.
func humanizeContentType(ct string) string {
	switch ct {
	case "blog_post":
		return "Blog Post"
	case "discussion":
		return "Discussion"
	case "question":
		return "Question"
	case "article":
		return "Article"
	case "guide":
		return "Guide"
	case "tool":
		return "Tool"
	case "wiki_page":
		return "Wiki Page"
	case "destination_wiki":
		return "Destination Wiki"
	case "profile":
		return "Member Profile"
	case "product":
		return "Product"
	case "tag":
		return "Tag"
	case "source":
		return "Source"
	case "organization":
		return "Organization"
	case "scholarship":
		return "Scholarship"
	case "event":
		return "Event"
	case "scam_alert":
		return "Scam Alert"
	default:
		if ct == "" {
			return "Content"
		}

		return strings.ToUpper(ct[:1]) + ct[1:]
	}
}
