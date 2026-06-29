package seo

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handlers holds dependencies for SEO HTTP handlers.
type Handlers struct{}

// NewHandlers creates a new SEO handler set.
func NewHandlers() *Handlers {
	return &Handlers{}
}

// Route mounts SEO-related routes onto the given router.
func (h *Handlers) Route(r chi.Router) {
	r.Get("/og-image", h.ServeOGImage)
}

// ServeOGImage returns a 1x1 transparent PNG placeholder.
// TODO: Replace with golang.org/x/image-based dynamic OG image generation
// that renders title, subtitle, and brand logo onto a 1200×630 canvas.
func (h *Handlers) ServeOGImage(w http.ResponseWriter, r *http.Request) {
	// 1x1 transparent PNG (minimal valid image).
	// See ogimage.go generateOGImage for the full implementation target.
	const oneByOnePNG = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n\xb4\x00\x00\x00\x00IEND\xaeB`\x82"

	title := r.URL.Query().Get("title")
	_ = title // reserved for future dynamic rendering

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(oneByOnePNG)); err != nil {
		slog.Warn("og-image write error", "error", err)
	}
}
