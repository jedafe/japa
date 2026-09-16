// Package i18n — Service layer with in-memory + distributed Valkey cache for
// translation lookups.
// // The service maintains a per-platform, per-locale cache that is lazily loaded
// on first access and refreshed on mutation. The cache stores key→value maps
// for fast O(1) lookups from the frontend public API endpoint.
// // Two cache layers are used (Section 18 / VALKEY-002):
// 1. In-memory L1 cache (per-instance, 5min TTL) — fastest lookups
// 2. Distributed Valkey L2 cache (jc:cache:i18n:{}:{locale}, 1h TTL)
// — shared across instances, eliminates DB refetch when a different
// instance has already loaded the same locale.
// // On mutation, both layers are invalidated. The L2 DEL is best-effort; a
// failed DEL is non-fatal because the 1h TTL bounds staleness.
package i18n

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// cacheEntry holds a locale's translations and when they were last loaded.
type cacheEntry struct {
	data     map[string]string
	loadedAt time.Time
}

// cacheKey is a composite key of platform + locale.
type cacheKey struct {
	locale string
}

// l1CacheTTL is the in-memory L1 cache TTL (5 minutes).
const l1CacheTTL = 5 * time.Minute

// l2CacheTTL is the distributed Valkey L2 cache TTL (1 hour, per Section 18).
const l2CacheTTL = 1 * time.Hour

// Service provides translation operations with an in-memory cache layer.
type Service struct {
	store *Store
	db    *db.DB

	mu    sync.RWMutex
	cache map[cacheKey]*cacheEntry

	vk valkeycompat.Cmdable // nil disables L2 cache (tests/development)
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

// SetValkey wires the Valkey client for the distributed L2 cache (Section 18 /
// VALKEY-002). Keys: jc:cache:i18n:{}:{locale}.
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// l2Key returns the Valkey L2 cache key for a platform+locale.
func l2Key(locale string) string {
	return valkey.MustKey("cache:i18n::%s", locale)
}

// ---------------------------------------------------------------------------
// Public read methods (cached)
// ---------------------------------------------------------------------------

// GetTranslationsByLocale returns all translations for a locale as a key→value map.
// Results are served from the L1 in-memory cache when available; otherwise the
// L2 Valkey cache is checked; otherwise the cache is populated from the
// database on first access (Section 18 / VALKEY-002).
func (s *Service) GetTranslationsByLocale(ctx context.Context, locale string) (map[string]string, error) {
	// L1: in-memory cache.
	if m, ok := s.cacheGet(locale); ok {
		return m, nil
	}

	// L2: distributed Valkey cache.
	if s.vk != nil {
		if cached, err := s.vk.Get(ctx, l2Key(locale)).Result(); err == nil && cached != "" {
			var m map[string]string
			if err := json.Unmarshal([]byte(cached), &m); err == nil && m != nil {
				// Populate L1 from L2 so subsequent reads skip the network.
				s.cacheSet(locale, m)
				// Return a copy to prevent mutation of cached data.
				out := make(map[string]string, len(m))
				for k, v := range m {
					out[k] = v
				}

				return out, nil
			}
		}
	}

	// Miss: load from DB.
	rows, err := s.store.ListByLocale(ctx, locale)
	if err != nil {
		return nil, err
	}

	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}

	s.cacheSet(locale, m)

	return m, nil
}

// Translate returns the value for a single key, falling back to defaults if
// the key is not found. Pass an empty defaults map if no fallback is needed.
func (s *Service) Translate(ctx context.Context, locale, key string, defaults map[string]string) (string, error) {
	m, err := s.GetTranslationsByLocale(ctx, locale)
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
func (s *Service) TranslateMany(ctx context.Context, locale string, keys []string) (map[string]string, error) {
	m, err := s.GetTranslationsByLocale(ctx, locale)
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
func (s *Service) Upsert(ctx context.Context, locale, key, value string, ctxPtr *string) error {
	if err := s.store.Upsert(ctx, gen.UpsertTranslationParams{

		Locale:  locale,
		Key:     key,
		Value:   value,
		Context: ctxPtr,
	}); err != nil {
		return err
	}

	s.cacheInvalidate(locale)

	return nil
}

// UpsertBatch inserts or updates multiple translations at once.
func (s *Service) UpsertBatch(ctx context.Context, translations []UpsertBatchItem) error {
	affectedLocales := make(map[string]struct{})

	for _, t := range translations {
		if err := s.store.Upsert(ctx, gen.UpsertTranslationParams{

			Locale:  t.Locale,
			Key:     t.Key,
			Value:   t.Value,
			Context: t.Context,
		}); err != nil {
			return fmt.Errorf("batch upsert (key=%s): %w", t.Key, err)
		}

		affectedLocales[t.Locale] = struct{}{}
	}

	for locale := range affectedLocales {
		s.cacheInvalidate(locale)
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
func (s *Service) Delete(ctx context.Context, id pgtype.UUID, locale string) error {
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}

	s.cacheInvalidate(locale)

	return nil
}

// DeleteByLocale removes all translations for a locale and invalidates the cache.
func (s *Service) DeleteByLocale(ctx context.Context, locale string) error {
	if err := s.store.DeleteByLocale(ctx, locale); err != nil {
		return err
	}

	s.cacheInvalidate(locale)

	return nil
}

// GetByID returns a single translation by its primary key.
func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (gen.Translation, error) {
	return s.store.GetByID(ctx, id)
}

// ListLocales returns all distinct locales for a platform.
func (s *Service) ListLocales(ctx context.Context) ([]string, error) {
	return s.store.ListLocales(ctx)
}

// Counts returns translation counts grouped by locale.
func (s *Service) Counts(ctx context.Context) ([]gen.CountTranslationsByLocaleRow, error) {
	return s.store.CountByLocale(ctx)
}

// RefreshCache forces a full reload of all translations for a platform into
// the in-memory cache. Call after bulk imports or migration events.
func (s *Service) RefreshCache(ctx context.Context) error {
	locales, err := s.store.ListLocales(ctx)
	if err != nil {
		return fmt.Errorf("refresh cache: listing locales: %w", err)
	}

	for _, locale := range locales {
		rows, err := s.store.ListByLocale(ctx, locale)
		if err != nil {
			return fmt.Errorf("refresh cache (locale=%s): %w", locale, err)
		}

		m := make(map[string]string, len(rows))
		for _, r := range rows {
			m[r.Key] = r.Value
		}

		s.cacheSet(locale, m)
	}

	slog.Info("i18n cache refreshed", "locales", len(locales))

	return nil
}

// ---------------------------------------------------------------------------
// Cache internals
// ---------------------------------------------------------------------------

// cacheGet returns the cached map if present and not older than l1CacheTTL.
func (s *Service) cacheGet(locale string) (map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.cache[cacheKey{locale: locale}]
	if !ok {
		return nil, false
	}

	if time.Since(entry.loadedAt) > l1CacheTTL {
		return nil, false
	}
	// Return a copy to prevent mutation of cached data.
	m := make(map[string]string, len(entry.data))
	for k, v := range entry.data {
		m[k] = v
	}

	return m, true
}

// cacheSet writes a map into the L1 in-memory cache and, if Valkey is wired,
// into the L2 distributed cache (Section 18 / VALKEY-002). L2 write is
// best-effort — a failure logs and continues; the L1 cache is still populated.
func (s *Service) cacheSet(locale string, data map[string]string) {
	s.mu.Lock()
	s.cache[cacheKey{locale: locale}] = &cacheEntry{
		data:     data,
		loadedAt: time.Now(),
	}
	s.mu.Unlock()

	if s.vk == nil {
		return
	}
	// L2: serialize and write to Valkey with the 1h TTL.
	if encoded, err := json.Marshal(data); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := s.vk.Set(ctx, l2Key(locale), encoded, l2CacheTTL).Err(); err != nil {
			slog.Warn("i18n: failed to write L2 cache", "locale", locale, "error", err)
		}
	}
}

// cacheInvalidate removes a single locale from both the L1 in-memory cache
// and the L2 distributed Valkey cache (Section 18 / VALKEY-002). L2 DEL is
// best-effort — a failure logs and continues; the 1h TTL bounds staleness.
func (s *Service) cacheInvalidate(locale string) {
	s.mu.Lock()
	delete(s.cache, cacheKey{locale: locale})
	s.mu.Unlock()

	if s.vk == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.vk.Del(ctx, l2Key(locale)).Err(); err != nil {
		slog.Warn("i18n: failed to invalidate L2 cache", "locale", locale, "error", err)
	}
}
