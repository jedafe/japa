package seo

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"

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
	// V1: placeholder colored background
	// TODO: implement real text rendering with golang.org/x/image/font
	img := image.NewRGBA(image.Rect(0, 0, 1200, 630))

	// Fill with brand color
	for y := 0; y < 630; y++ {
		for x := 0; x < 1200; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 41, B: 59, A: 255}) // slate-800
		}
	}

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