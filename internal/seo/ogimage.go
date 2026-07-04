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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/jedafe/japa/internal/tenant"
)

// OGImageHandler handles OG image generation requests.
type OGImageHandler struct {
	reader *pgxpool.Pool
	face   font.Face
}

// NewOGImageHandler creates a new OG image handler.
// The font face is loaded once at construction from the embedded Go Regular
// font (golang.org/x/image/font/gofont/goregular). If font loading fails
// the handler still works — text rendering degrades gracefully to a
// fallback bitmap approach.
func NewOGImageHandler(reader *pgxpool.Pool) *OGImageHandler {
	face := loadFontFace(48)
	return &OGImageHandler{reader: reader, face: face}
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

// Route mounts the OG image routes.
func (h *OGImageHandler) Route(r chi.Router) {
	r.Get("/og/{id}.png", h.ServeOGImage)
}

// contentMeta holds the minimal data needed to render an OG image.
type contentMeta struct {
	Title       string
	ContentType string
}

// lookupContent queries the contents table for the title and content_type
// of the given content ID within the tenant. Returns nil if the ID is
// invalid or the content does not exist.
func (h *OGImageHandler) lookupContent(ctx context.Context, idStr string, tenantID pgtype.UUID) *contentMeta {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil
	}

	var pgID pgtype.UUID
	if err := pgID.Scan(id.String()); err != nil {
		return nil
	}

	var meta contentMeta

	err = h.reader.QueryRow(
		ctx,
		`SELECT title, content_type FROM contents WHERE id = $1 AND tenant_id = $2`,
		pgID, tenantID,
	).Scan(&meta.Title, &meta.ContentType)
	if err != nil {
		return nil
	}

	return &meta
}

// ServeOGImage generates and serves an OG image for the given content ID.
// If the content is found, its title is rendered as the main text.
// Otherwise, a default "JapaConnect" branding image is returned.
func (h *OGImageHandler) ServeOGImage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	t := tenant.FromContext(r.Context())

	var title, subtitle string

	if t != nil {
		if meta := h.lookupContent(r.Context(), idStr, t.ID); meta != nil {
			title = cleanTitle(meta.Title)
			subtitle = humanizeContentType(meta.ContentType)
		}
	}

	// Fallback to brand defaults when no content matched.
	if title == "" {
		title = "JapaConnect"
		subtitle = "Community Platform"
	}

	img := generateOGImage(title, subtitle, h.face)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		slog.Error("og-image: png encode failed", "error", err)
		http.Error(w, "Failed to generate image", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400") // 24h cache
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

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen-3] + "..."
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
	default:
		if ct == "" {
			return "Content"
		}

		return strings.ToUpper(ct[:1]) + ct[1:]
	}
}
