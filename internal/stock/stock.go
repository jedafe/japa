// Package stock provides stock photo search and import from Unsplash, Pexels,
// Pixabay, and Openverse.
package stock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/media"
	"golang.org/x/sync/errgroup"
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
//
//nolint:revive // exported: stutter naming is intentional
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

// ErrProviderUnavailable is returned when a provider is not loaded or enabled.
var ErrProviderUnavailable = errors.New("provider is not available")

// cacheEntry holds cached search results with expiry and last-access time
// for proper LRU eviction.
type cacheEntry struct {
	Result     *SearchResult
	ExpiresAt  time.Time
	LastAccess time.Time
}

// Service orchestrates stock photo search and import across providers.
type Service struct {
	providers     map[string]StockProvider
	store         *Store
	media         *media.Service
	cache         map[string]*cacheEntry
	mu            sync.RWMutex
	cacheMu       sync.RWMutex
	cfg           *config.Config
	cacheTTL      time.Duration
	maxCacheItems int
}

// NewService creates a Service. Call ReloadProviders after construction
// to populate providers from DB config.
func NewService(database *db.DB, mediaSvc *media.Service, cfg *config.Config) *Service {
	ttlSec := cfg.StockCacheTTLSec
	if ttlSec <= 0 {
		ttlSec = 300
	}

	maxEntries := cfg.StockCacheMaxEntries
	if maxEntries <= 0 {
		maxEntries = 1000
	}

	return &Service{
		providers:     make(map[string]StockProvider),
		store:         NewStore(database),
		media:         mediaSvc,
		cache:         make(map[string]*cacheEntry),
		cfg:           cfg,
		cacheTTL:      time.Duration(ttlSec) * time.Second,
		maxCacheItems: maxEntries,
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
			s.providers["unsplash"] = NewUnsplashProvider(apiKey, s.httpTimeout())
		case "pexels":
			s.providers["pexels"] = NewPexelsProvider(apiKey, s.httpTimeout())
		case "pixabay":
			s.providers["pixabay"] = NewPixabayProvider(apiKey, s.httpTimeout())
		case "openverse":
			s.providers["openverse"] = NewOpenverseProvider(s.httpTimeout())
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

// httpTimeout returns the configured provider HTTP client timeout, defaulting
// to 15 seconds when JC_STOCK_HTTP_TIMEOUT_SECONDS is unset or invalid.
func (s *Service) httpTimeout() time.Duration {
	sec := s.cfg.StockHTTPTimeoutSec
	if sec <= 0 {
		sec = 15
	}

	return time.Duration(sec) * time.Second
}

// AvailableProvidersForPerms returns provider names available to a user with
// the given resolved permission flags. This uses the RBAC resolver's
// HasPermission result (which honours the "*" wildcard) rather than raw
// permission keys, so super-admins correctly see all providers.
func (s *Service) AvailableProvidersForPerms(hasSearch, hasProviders bool) []string {
	if !hasSearch {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var names []string

	for name := range s.providers {
		if name == "openverse" {
			names = append(names, name)
			continue
		}

		if hasProviders {
			names = append(names, name)
		}
	}

	return names
}

// ListAllProviders returns all configured providers with their enabled/disabled
// status and API key presence. Used by the admin handler.
func (s *Service) ListAllProviders(ctx context.Context) ([]ProviderConfigRow, error) {
	return s.store.ListProviders(ctx)
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

	// Fan out to all providers concurrently and merge.
	type providerResult struct {
		photos  []StockPhoto
		total   int
		hasMore bool
	}

	results := make([]providerResult, len(searchProviders))
	g, gctx := errgroup.WithContext(ctx)

	for i, p := range searchProviders {
		i, p := i, p

		g.Go(func() error {
			result, err := s.searchSingle(gctx, p, query, page, perPage)
			if err != nil {
				slog.Warn("stock: provider search failed", "provider", p.Name(), "query", query, "error", err)
				return nil // don't fail the whole search on one provider error
			}

			results[i] = providerResult{
				photos:  result.Photos,
				total:   result.Total,
				hasMore: result.HasNextPage,
			}

			return nil
		})
	}

	_ = g.Wait()

	merged := &SearchResult{Photos: []StockPhoto{}, Page: page, PerPage: perPage}
	for _, r := range results {
		merged.Photos = append(merged.Photos, r.photos...)
		merged.Total += r.total
		merged.HasNextPage = merged.HasNextPage || r.hasMore
	}

	return merged, nil
}

func (s *Service) searchSingle(ctx context.Context, p StockProvider, query string, page, perPage int) (*SearchResult, error) {
	key := cacheKey(p.Name(), query, page, perPage)
	now := time.Now()

	s.cacheMu.RLock()
	if entry, ok := s.cache[key]; ok && now.Before(entry.ExpiresAt) {
		s.cacheMu.RUnlock()
		// Update last-access time under the write lock for LRU tracking.
		s.cacheMu.Lock()
		entry.LastAccess = now
		s.cacheMu.Unlock()

		return entry.Result, nil
	}
	s.cacheMu.RUnlock()

	result, err := p.Search(ctx, query, page, perPage)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	// LRU eviction: if over max, sort by last-access time and evict the
	// oldest half. Unlike random map iteration, this preserves hot entries.
	if len(s.cache) >= s.maxCacheItems {
		type kv struct {
			key string
			la  time.Time
		}

		entries := make([]kv, 0, len(s.cache))
		for k, e := range s.cache {
			entries = append(entries, kv{k, e.LastAccess})
		}

		sort.Slice(entries, func(i, j int) bool { return entries[i].la.Before(entries[j].la) })

		evictCount := len(s.cache) - s.maxCacheItems/2
		if evictCount < 0 {
			evictCount = len(s.cache) / 2
		}

		for i := 0; i < evictCount && i < len(entries); i++ {
			delete(s.cache, entries[i].key)
		}
	}

	s.cache[key] = &cacheEntry{Result: result, ExpiresAt: now.Add(s.cacheTTL), LastAccess: now}
	s.cacheMu.Unlock()

	return result, nil
}

// GetPhoto fetches metadata for a single stock photo.
func (s *Service) GetPhoto(ctx context.Context, providerName, photoID string) (*StockPhoto, error) {
	s.mu.RLock()
	p, ok := s.providers[providerName]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderUnavailable, providerName)
	}

	val, err := p.GetByID(ctx, photoID)
	if err != nil {
		return nil, fmt.Errorf("getting photo %s from %s: %w", photoID, providerName, err)
	}

	return val, nil
}

// ImportPhoto downloads a stock photo and stores it in the media library.
func (s *Service) ImportPhoto(ctx context.Context, uploaderID pgtype.UUID, providerName, photoID string) (pgtype.UUID, error) {
	s.mu.RLock()
	p, ok := s.providers[providerName]
	s.mu.RUnlock()

	if !ok {
		return pgtype.UUID{}, fmt.Errorf("%w: %s", ErrProviderUnavailable, providerName)
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

	mediaID, err := s.media.StoreFromBytes(ctx, uploaderID, downloaded.Data, downloaded.Filename, attr)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("storing imported photo: %w", err)
	}

	return mediaID, nil
}

func cacheKey(provider, query string, page, perPage int) string {
	return fmt.Sprintf("%s:%s:%d:%d", provider, query, page, perPage)
}

// validateDownloadURL constrains upstream-supplied photo URLs before the
// Download fetch: https only, no userinfo, no private-literal hosts. A
// compromised upstream returning "full_url": "http://169.254.169.254/x"
// must not turn the importer into an SSRF sink. Hostnames are trusted
// past the literal check (upstream CDNs resolve publicly); the 30s client
// timeout bounds the residual.
func validateDownloadURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || u.Scheme != "https" {
		return fmt.Errorf("stock: download URL must be https: %q", rawURL)
	}

	if u.User != nil {
		return errors.New("stock: download URL must not embed credentials")
	}

	if ip := net.ParseIP(u.Hostname()); ip != nil && isReservedIP(ip) {
		return fmt.Errorf("stock: download URL targets a private address: %q", rawURL)
	}

	return nil
}

// isReservedIP mirrors the webhook/ai/fx SSRF literal blocklist
// (stdlib-only, no cross-package import).
func isReservedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}

		if v4[0] == 0 || v4[0] >= 240 {
			return true
		}

		if (v4[0] == 192 && v4[1] == 0 && (v4[2] == 0 || v4[2] == 2)) ||
			(v4[0] == 198 && v4[1] == 51 && v4[2] == 100) ||
			(v4[0] == 203 && v4[1] == 0 && v4[2] == 113) ||
			(v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) {
			return true
		}
	}

	return false
}
