package seo

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OGImageHandler handles OG image generation requests.
type OGImageHandler struct {
	reader *pgxpool.Pool
}

// NewOGImageHandler creates a new OG image handler.
func NewOGImageHandler(reader *pgxpool.Pool) *OGImageHandler {
	return &OGImageHandler{reader: reader}
}

// Route mounts the OG image routes.
func (h *OGImageHandler) Route(r chi.Router) {
	r.Get("/og/{id}.png", h.ServeOGImage)
}

// ServeOGImage generates and serves an OG image for the given content ID.
func (h *OGImageHandler) ServeOGImage(w http.ResponseWriter, r *http.Request) {
	// Generate OG image with brand styling.
	// The image is 1200x630 (standard OG dimensions) with:
	// - Slate-800 background
	// - White text for title
	// - Brand accent stripe at bottom
	img := generateOGImage("JapaConnect", "Community Platform")

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		http.Error(w, "Failed to generate image", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400") // 24h cache
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// generateOGImage creates an OG image with text rendering.
// Uses the standard library's image/draw for text placement.
// Font rendering uses a simple bitmap approach for now — in production
// this would use golang.org/x/image/font with a TTF font.
func generateOGImage(title, subtitle string) *image.RGBA {
	const (
		width  = 1200
		height = 630
	)

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with brand color (slate-800)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 41, B: 59, A: 255})
		}
	}

	// Draw accent stripe at bottom
	stripeHeight := 8
	for y := height - stripeHeight; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 99, G: 102, B: 241, A: 255}) // indigo-500
		}
	}

	// Draw title text (simplified bitmap rendering)
	// In production, use golang.org/x/image/font with a TTF font
	drawText(img, 80, 200, title, color.White, 3.0)
	drawText(img, 80, 300, subtitle, color.RGBA{R: 148, G: 163, B: 184, A: 255}, 1.5) // slate-400

	return img
}

// drawText draws text on the image using a simple bitmap font.
// This is a placeholder — in production, use golang.org/x/image/font.
func drawText(img *image.RGBA, x, y int, text string, clr color.Color, scale float64) {
	// Simple text rendering using filled rectangles for each character
	// This creates a readable but basic text representation
	charWidth := int(12 * scale)
	charHeight := int(20 * scale)
	spacing := int(4 * scale)

	for i, ch := range text {
		if ch == ' ' {
			continue
		}
		drawChar(img, x+i*(charWidth+spacing), y, charWidth, charHeight, clr)
	}
}

// drawChar draws a single character placeholder (filled rectangle).
func drawChar(img *image.RGBA, x, y, width, height int, clr color.Color) {
	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			img.Set(x+dx, y+dy, clr)
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
	// Remove newlines and extra spaces
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")
	title = strings.Join(strings.Fields(title), " ")
	return truncateString(title, 60)
}
