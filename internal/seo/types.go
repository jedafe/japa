package seo

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// PageMetadata holds all SEO-relevant metadata for a content page.
type PageMetadata struct {
	ID          pgtype.UUID `json:"id"`
	TenantID    pgtype.UUID 
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Slug        string      `json:"slug"`
	ContentType string      `json:"content_type"`
	OGImage     string      `json:"og_image"`
	Canonical   string      `json:"canonical_url"`
	Author      string      `json:"author"`
	PublishedAt time.Time   `json:"published_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// OGImageRequest describes a request for an Open Graph image.
type OGImageRequest struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	LogoPath string `json:"logo_path"` // optional, defaults to brand logo
}

// OGImageResult is the response from OG image generation.
type OGImageResult struct {
	ImageBytes  []byte `json:"image_bytes"`
	ContentType string `json:"content_type"`
}
