// Package i18n — Service layer with in-memory cache for translation lookups.
//
// The service maintains a per-tenant, per-locale cache that is lazily loaded
// on first access and refreshed on mutation. The cache stores key→value maps
// for fast O(1) lookups from the frontend public API endpoint.
package i18n

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// cacheEntry holds a locale's translations and when they were last loaded.
type cacheEntry struct {
	data      map[string]string
	loadedAt  time.Time
}

// cacheKey is a composite key of tenant + locale.
type cacheKey struct {
	tenantID pgtype.UUID
	locale   string
}

// Service provides translation operations with an in-memory cache layer.
type Service struct {
	store *Store
	db    *db.DB

	mu    sync.RWMutex
	cache map[cacheKey]*cacheEntry
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:    database,
		cache: make(map[cacheKey]*cacheEntry),
	}
}

// ---------------------------------------------------------------------------
// Public read methods (cached)
// ---------------------------------------------------------------------------

// GetTranslationsByLocale returns all translations for a locale as a key→value map.
// Results are served from cache when available; otherwise the cache is populated
// from the database on first access.
func (s *Service) GetTranslationsByLocale(ctx context.Context, tenantID pgtype.UUID, locale string) (map[string]string, error) {
	if m, ok := s.cacheGet(tenantID, locale); ok {
		return m, nil
	}

	rows, err := s.store.ListByLocale(ctx, tenantID, locale)
	if err != nil {
		return nil, err
	}

	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}

	s.cacheSet(tenantID, locale, m)
	return m, nil
}

// Translate returns the value for a single key, falling back to defaults if
// the key is not found. Pass an empty defaults map if no fallback is needed.
func (s *Service) Translate(ctx context.Context, tenantID pgtype.UUID, locale, key string, defaults map[string]string) (string, error) {
	m, err := s.GetTranslationsByLocale(ctx, tenantID, locale)
	if err != nil {
		return "", err
	}

	if v, ok := m[key]; ok {
		return v, nil
	}
	if defaults != nil {
		if v, ok := defaults[key]; ok {
			return v, nil
		}
	}
	return key, nil // return the key itself as final fallback
}

// TranslateMany returns values for multiple keys in one call.
func (s *Service) TranslateMany(ctx context.Context, tenantID pgtype.UUID, locale string, keys []string) (map[string]string, error) {
	m, err := s.GetTranslationsByLocale(ctx, tenantID, locale)
	if err != nil {
		return nil, err
	}

	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		} else {
			out[k] = k
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Write methods (invalidate cache on mutation)
// ---------------------------------------------------------------------------

// Upsert inserts or updates a single translation and invalidates the cache.
func (s *Service) Upsert(ctx context.Context, tenantID pgtype.UUID, locale, key, value string, ctxPtr *string) error {
	if err := s.store.Upsert(ctx, gen.UpsertTranslationParams{
		TenantID: tenantID,
		Locale:   locale,
		Key:      key,
		Value:    value,
		Context:  ctxPtr,
	}); err != nil {
		return err
	}
	s.cacheInvalidate(tenantID, locale)
	return nil
}

// UpsertBatch inserts or updates multiple translations at once.
func (s *Service) UpsertBatch(ctx context.Context, tenantID pgtype.UUID, translations []UpsertBatchItem) error {
	affectedLocales := make(map[string]struct{})

	for _, t := range translations {
		if err := s.store.Upsert(ctx, gen.UpsertTranslationParams{
			TenantID: tenantID,
			Locale:   t.Locale,
			Key:      t.Key,
			Value:    t.Value,
			Context:  t.Context,
		}); err != nil {
			return fmt.Errorf("batch upsert (key=%s): %w", t.Key, err)
		}
		affectedLocales[t.Locale] = struct{}{}
	}

	for locale := range affectedLocales {
		s.cacheInvalidate(tenantID, locale)
	}
	return nil
}

// UpsertBatchItem is a single item in a batch upsert operation.
type UpsertBatchItem struct {
	Locale  string
	Key     string
	Value   string
	Context *string
}

// Delete removes a translation and invalidates the cache.
func (s *Service) Delete(ctx context.Context, tenantID, id pgtype.UUID, locale string) error {
	if err := s.store.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.cacheInvalidate(tenantID, locale)
	return nil
}

// ListLocales returns all distinct locales for a tenant.
func (s *Service) ListLocales(ctx context.Context, tenantID pgtype.UUID) ([]string, error) {
	return s.store.ListLocales(ctx, tenantID)
}

// Counts returns translation counts grouped by locale.
func (s *Service) Counts(ctx context.Context, tenantID pgtype.UUID) ([]gen.CountTranslationsByLocaleRow, error) {
	return s.store.CountByLocale(ctx, tenantID)
}

// RefreshCache forces a full reload of all translations for a tenant into
// the in-memory cache. Call after bulk imports or migration events.
func (s *Service) RefreshCache(ctx context.Context, tenantID pgtype.UUID) error {
	locales, err := s.store.ListLocales(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("refresh cache: listing locales: %w", err)
	}

	for _, locale := range locales {
		rows, err := s.store.ListByLocale(ctx, tenantID, locale)
		if err != nil {
			return fmt.Errorf("refresh cache (locale=%s): %w", locale, err)
		}
		m := make(map[string]string, len(rows))
		for _, r := range rows {
			m[r.Key] = r.Value
		}
		s.cacheSet(tenantID, locale, m)
	}

	slog.Info("i18n cache refreshed", "tenant", tenantID, "locales", len(locales))
	return nil
}

// ---------------------------------------------------------------------------
// Cache internals
// ---------------------------------------------------------------------------

// cacheGet returns the cached map if present and not older than 5 minutes.
func (s *Service) cacheGet(tenantID pgtype.UUID, locale string) (map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.cache[cacheKey{tenantID: tenantID, locale: locale}]
	if !ok {
		return nil, false
	}
	if time.Since(entry.loadedAt) > 5*time.Minute {
		return nil, false
	}
	// Return a copy to prevent mutation of cached data.
	m := make(map[string]string, len(entry.data))
	for k, v := range entry.data {
		m[k] = v
	}
	return m, true
}

// cacheSet writes a map into the cache.
func (s *Service) cacheSet(tenantID pgtype.UUID, locale string, data map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[cacheKey{tenantID: tenantID, locale: locale}] = &cacheEntry{
		data:     data,
		loadedAt: time.Now(),
	}
}

// cacheInvalidate removes a single locale from the cache.
func (s *Service) cacheInvalidate(tenantID pgtype.UUID, locale string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, cacheKey{tenantID: tenantID, locale: locale})
}
