package stock

import "context"

// StockProvider implements search and download for a stock photo service.
type StockProvider interface {
	// Name returns the provider identifier (e.g., "unsplash", "pexels").
	Name() string

	// Search queries the provider for photos matching the given terms.
	Search(ctx context.Context, query string, page, perPage int) (*SearchResult, error)

	// GetByID fetches metadata for a single photo.
	GetByID(ctx context.Context, photoID string) (*StockPhoto, error)

	// Download fetches full-resolution photo bytes plus attribution metadata.
	Download(ctx context.Context, photoID string) (*DownloadedPhoto, error)
}
