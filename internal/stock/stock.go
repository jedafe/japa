// Package stock provides stock photo search and import from Unsplash, Pexels,
// Pixabay, and Openverse.
package stock

import (
	"fmt"
	"sync"
	"time"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/media"
)

// SearchResult holds a page of search results from a stock provider.
type SearchResult struct {
	Photos      []StockPhoto `json:"photos"`
	Total       int          `json:"total"`
	Page        int          `json:"page"`
	PerPage     int          `json:"per_page"`
	HasNextPage bool         `json:"has_next_page"`
}

// StockPhoto represents a single photo from a stock provider.
type StockPhoto struct {
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	ThumbnailURL   string `json:"thumbnail_url"`
	FullURL        string `json:"full_url"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Photographer   string `json:"photographer"`
	PhotographerURL string `json:"photographer_url"`
	License        string `json:"license,omitempty"`
	LicenseURL     string `json:"license_url,omitempty"`
	Description    string `json:"description,omitempty"`
}

// DownloadedPhoto is the result of downloading a full-resolution photo.
type DownloadedPhoto struct {
	Data             []byte
	Filename         string
	Photographer     string
	PhotographerURL  string
	License          string
	LicenseURL       string
	ProviderPhotoID  string
}

// cacheEntry holds cached search results with expiry.
type cacheEntry struct {
	Result    *SearchResult
	ExpiresAt time.Time
}

// Service orchestrates stock photo search and import across providers.
type Service struct {
	providers map[string]StockProvider
	store     *Store
	media     *media.Service
	cache     map[string]*cacheEntry
	cacheMu   sync.RWMutex
	cfg       *config.Config
	db        *db.DB
}

// NewService creates a Service, constructing providers that have API keys available.
func NewService(database *db.DB, mediaSvc *media.Service, cfg *config.Config) *Service {
	s := &Service{
		providers: make(map[string]StockProvider),
		store:     NewStore(database),
		media:     mediaSvc,
		cache:     make(map[string]*cacheEntry),
		cacheMu:   sync.RWMutex{},
		cfg:       cfg,
		db:        database,
	}
	// Providers are loaded lazily on first request or on startup via s.ReloadProviders().
	return s
}

// cacheKey builds a cache key for search results.
func cacheKey(provider, query string, page, perPage int) string {
	return provider + ":" + query + ":" + itoa(page) + ":" + itoa(perPage)
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
