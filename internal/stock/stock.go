// Package stock provides stock photo search and import from Unsplash, Pexels,
// Pixabay, and Openverse.
package stock

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/crypto"
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
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	ThumbnailURL    string `json:"thumbnail_url"`
	FullURL         string `json:"full_url"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	Photographer    string `json:"photographer"`
	PhotographerURL string `json:"photographer_url"`
	License         string `json:"license,omitempty"`
	LicenseURL      string `json:"license_url,omitempty"`
	Description     string `json:"description,omitempty"`
}

// DownloadedPhoto is the result of downloading a full-resolution photo.
type DownloadedPhoto struct {
	Data            []byte
	Filename        string
	Photographer    string
	PhotographerURL string
	License         string
	LicenseURL      string
	ProviderPhotoID string
}

// cacheEntry holds cached search results with expiry.
type cacheEntry struct {
	Result    *SearchResult
	ExpiresAt time.Time
}

// DefaultCacheTTL is how long search results stay in the LRU cache.
const DefaultCacheTTL = 5 * time.Minute

// MaxCacheEntries is the maximum number of cached search result pages.
const MaxCacheEntries = 1000

// Service orchestrates stock photo search and import across providers.
type Service struct {
	providers map[string]StockProvider
	store     *Store
	media     *media.Service
	cache     map[string]*cacheEntry
	mu        sync.RWMutex
	cacheMu   sync.RWMutex
	cfg       *config.Config
	db        *db.DB
}

// NewService creates a Service. Call ReloadProviders after construction
// to populate providers from DB config.
func NewService(database *db.DB, mediaSvc *media.Service, cfg *config.Config) *Service {
	return &Service{
		providers: make(map[string]StockProvider),
		store:     NewStore(database),
		media:     mediaSvc,
		cache:     make(map[string]*cacheEntry),
		cfg:       cfg,
		db:        database,
	}
}

// ReloadProviders reads stock_providers from the DB, decrypts API keys,
// and constructs/destroys provider instances to match the current config.
func (s *Service) ReloadProviders(ctx context.Context) error {
	// Seed any providers not yet in DB from env vars.
	s.seedFromEnv(ctx)

	rows, err := s.store.ListProviders(ctx)
	if err != nil {
		return fmt.Errorf("listing providers: %w", err)
	}

	for _, row := range rows {
		s.mu.Lock()
		if !row.Enabled {
			delete(s.providers, row.ProviderName)
			s.mu.Unlock()
			continue
		}

		// Skip if provider is already loaded.
		if _, exists := s.providers[row.ProviderName]; exists {
			s.mu.Unlock()
			continue
		}

		var apiKey string
		if row.APIKeyEncrypted != "" {
			apiKey, err = crypto.Decrypt(row.APIKeyEncrypted)
			if err != nil {
				slog.Warn("stock: failed to decrypt API key", "provider", row.ProviderName, "error", err)
				s.mu.Unlock()
				continue
			}
		}

		switch row.ProviderName {
		case "unsplash":
			s.providers["unsplash"] = NewUnsplashProvider(apiKey)
		case "pexels":
			s.providers["pexels"] = NewPexelsProvider(apiKey)
		case "pixabay":
			s.providers["pixabay"] = NewPixabayProvider(apiKey)
		case "openverse":
			s.providers["openverse"] = NewOpenverseProvider()
		}
		s.mu.Unlock()
	}

	// Evict cache since providers changed.
	s.cacheMu.Lock()
	s.cache = make(map[string]*cacheEntry)
	s.cacheMu.Unlock()

	return nil
}

// seedFromEnv ensures provider rows exist and seeds API keys from env vars
// the first time a provider has no encrypted key.
func (s *Service) seedFromEnv(ctx context.Context) {
	envKeys := map[string]string{
		"unsplash": s.cfg.UnsplashAccessKey,
		"pexels":   s.cfg.PexelsAPIKey,
		"pixabay":  s.cfg.PixabayAPIKey,
	}
	for name, rawKey := range envKeys {
		encrypted := ""
		if rawKey != "" {
			var err error
			encrypted, err = crypto.Encrypt(rawKey)
			if err != nil {
				slog.Warn("stock: failed to encrypt env API key", "provider", name, "error", err)
				continue
			}
		}
		if err := s.store.SeedProvider(ctx, name, encrypted); err != nil {
			slog.Warn("stock: failed to seed provider", "provider", name, "error", err)
		}
	}
	// Openverse always gets seeded (no API key needed).
	_ = s.store.SeedProvider(ctx, "openverse", "")
}

// AvailableProviders returns provider names available to a user with the given
// permission nodes. Openverse is available with media.stock.search; the other
// three require media.stock.providers.
func (s *Service) AvailableProviders(ctx context.Context, userPerms []string) []string {
	hasSearch := false
	hasProviders := false
	for _, p := range userPerms {
		if p == "media.stock.search" {
			hasSearch = true
		}
		if p == "media.stock.providers" {
			hasProviders = true
		}
	}

	if !hasSearch {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var names []string
	for name, prov := range s.providers {
		if name == "openverse" {
			names = append(names, name)
			continue
		}
		if hasProviders {
			_ = prov // provider exists, include it
			names = append(names, name)
		}
	}
	return names
}

// Search queries stock providers with caching.
func (s *Service) Search(ctx context.Context, query, providerFilter string, page, perPage int) (*SearchResult, error) {
	if perPage <= 0 || perPage > 80 {
		perPage = 20
	}
	if page <= 0 {
		page = 1
	}

	// Determine which providers to search.
	s.mu.RLock()
	var searchProviders []StockProvider
	if providerFilter != "" {
		if p, ok := s.providers[providerFilter]; ok {
			searchProviders = append(searchProviders, p)
		}
	} else {
		for _, p := range s.providers {
			searchProviders = append(searchProviders, p)
		}
	}
	s.mu.RUnlock()

	if len(searchProviders) == 0 {
		return &SearchResult{Photos: []StockPhoto{}, Page: page, PerPage: perPage}, nil
	}

	// Search single provider — check cache first.
	if providerFilter != "" {
		return s.searchSingle(ctx, searchProviders[0], query, page, perPage)
	}

	// Fan out to all providers and merge.
	merged := &SearchResult{Photos: []StockPhoto{}, Page: page, PerPage: perPage}
	for _, p := range searchProviders {
		result, err := s.searchSingle(ctx, p, query, page, perPage)
		if err != nil {
			slog.Warn("stock: provider search failed", "provider", p.Name(), "query", query, "error", err)
			continue
		}
		merged.Photos = append(merged.Photos, result.Photos...)
		merged.Total += result.Total
		merged.HasNextPage = merged.HasNextPage || result.HasNextPage
	}
	return merged, nil
}

func (s *Service) searchSingle(ctx context.Context, p StockProvider, query string, page, perPage int) (*SearchResult, error) {
	key := cacheKey(p.Name(), query, page, perPage)

	s.cacheMu.RLock()
	if entry, ok := s.cache[key]; ok && time.Now().Before(entry.ExpiresAt) {
		s.cacheMu.RUnlock()
		return entry.Result, nil
	}
	s.cacheMu.RUnlock()

	result, err := p.Search(ctx, query, page, perPage)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	// Simple LRU eviction: if over max, clear the oldest half.
	if len(s.cache) >= MaxCacheEntries {
		for k := range s.cache {
			delete(s.cache, k)
			if len(s.cache) < MaxCacheEntries/2 {
				break
			}
		}
	}
	s.cache[key] = &cacheEntry{Result: result, ExpiresAt: time.Now().Add(DefaultCacheTTL)}
	s.cacheMu.Unlock()

	return result, nil
}

// GetPhoto fetches metadata for a single stock photo.
func (s *Service) GetPhoto(ctx context.Context, providerName, photoID string) (*StockPhoto, error) {
	s.mu.RLock()
	p, ok := s.providers[providerName]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider %s is not available", providerName)
	}

	val, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, fmt.Errorf("getting photo %s from %s: %w", photoID, providerName, err)
	}
	return val, nil
}

// ImportPhoto downloads a stock photo and stores it in the media library.
func (s *Service) ImportPhoto(ctx context.Context, tenantID, uploaderID pgtype.UUID, providerName, photoID string) (pgtype.UUID, error) {
	s.mu.RLock()
	p, ok := s.providers[providerName]
	s.mu.RUnlock()
	if !ok {
		return pgtype.UUID{}, fmt.Errorf("provider %s is not available", providerName)
	}

	downloaded, err := p.Download(ctx, photoID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("downloading from %s: %w", providerName, err)
	}

	attr := &media.MediaAttribution{
		SourcePlatform:        providerName,
		SourcePhotographer:    downloaded.Photographer,
		SourcePhotographerURL: downloaded.PhotographerURL,
		SourceLicense:         downloaded.License,
		SourceLicenseURL:      downloaded.LicenseURL,
		SourcePhotoID:         downloaded.ProviderPhotoID,
	}

	mediaID, err := s.media.StoreFromBytes(ctx, tenantID, uploaderID, downloaded.Data, downloaded.Filename, attr)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("storing imported photo: %w", err)
	}

	return mediaID, nil
}

func cacheKey(provider, query string, page, perPage int) string {
	return fmt.Sprintf("%s:%s:%d:%d", provider, query, page, perPage)
}
