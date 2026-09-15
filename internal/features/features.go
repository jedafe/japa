package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// flagCacheTTL is the TTL for both the in-memory L1 cache and the Valkey L2
// cache. Per Section 42, feature flag caches use a 5-minute TTL.
const flagCacheTTL = 5 * time.Minute

// flagCacheKeyPrefix namespaces all feature flag cache keys in Valkey.
const flagCacheKeyPrefix = "jc:flags:"

// FeatureChecker is the interface for checking whether a feature flag is
// enabled for a given tenant. Services that need to verify their own feature
// flag (e.g. story.Service.Enabled) should accept this interface rather than
// reading system_settings directly, so that feature_flags remains the single
// source of truth for feature reachability.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Checker resolves whether a feature is enabled for a given tenant.
// It uses a two-tier cache: L1 (in-memory, 60s TTL for fast eviction) and
// L2 (Valkey, 5min TTL for cross-instance consistency). When Valkey is
// unavailable, it gracefully degrades to L1-only operation.
type Checker struct {
	db        *db.DB
	cache     *flagCache
	vk        valkeycompat.Cmdable // nil when Valkey is unavailable (graceful degradation)
	closeOnce sync.Once
}

// flagCacheEntry wraps a flag value with its creation time for TTL eviction.
type flagCacheEntry struct {
	enabled   bool
	createdAt time.Time
}

// flagCache provides caching of feature flag lookups with TTL-based eviction.
type flagCache struct {
	mu      sync.RWMutex
	entries map[string]*flagCacheEntry // flag key -> entry
	done    chan struct{}
}

// New creates a feature flag Checker backed by the given database and
// optional Valkey client. When vkClient is non-nil, a two-tier L1+L2 cache
// is used: L1 (in-memory, 60s TTL) for hot-path lookups and L2 (Valkey,
// 5min TTL) for cross-instance consistency. When vkClient is nil, only
// the in-memory L1 cache is used (graceful degradation for single-server
// or Valkey-unavailable deployments).
// A background goroutine evicts L1 cache entries older than 60 seconds.
func New(database *db.DB, vkClient valkeycompat.Cmdable) *Checker {
	c := &Checker{
		db: database,
		cache: &flagCache{
			entries: make(map[string]*flagCacheEntry),
			done:    make(chan struct{}),
		},
		vk: vkClient,
	}
	go c.cache.cleanupLoop(60 * time.Second)

	return c
}

// Close stops the background cleanup goroutine and releases resources.
// Safe to call concurrently — sync.Once prevents double-close panics.
func (c *Checker) Close() {
	c.closeOnce.Do(func() {
		if c.cache.done != nil {
			close(c.cache.done)
		}
	})
}

// IsEnabled checks if a feature is enabled for the tenant in context.
// If the feature_flags table has no row for the key, the registry default
// is used. The result is cached in L1 (in-memory) and L2 (Valkey).
// Lookup order: L1 → L2 → DB. Results are cached at both layers.
// On transient DB errors the default is returned for the current request
// but NOT cached — the next request retries the DB.
func (c *Checker) IsEnabled(ctx context.Context, key string) bool {
	// Ref: .jeos/sections/42-feature-flags.md (removal in progress).
	// ID is kept in the public signature for Phase 1 compatibility and is ignored.
	if cached := c.cacheGet(key); cached != nil {
		return *cached
	}

	// L2 — Valkey (shared across server instances)
	if cached := c.valkeyGet(ctx, key); cached != nil {
		c.cacheSet(key, *cached)
		return *cached
	}

	// DB — source of truth
	enabled, err := c.queryEnabled(ctx, key)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		// Transient DB error (connection failure, context cancellation, etc.).
		// Return the registry default for this request but do NOT cache it —
		// caching a default from a failed query would poison the cache for
		// up to 5 minutes (L2), silently flipping features on or off.
		return enabled
	}

	// Success or ErrNoRows (legitimate "flag not set" → use default).
	// Safe to cache at both layers.
	c.cacheSet(key, enabled)
	c.valkeySet(ctx, key, enabled)

	return enabled
}

// queryEnabled queries the database for the feature flag. When the flag is
// enabled and its config carries a rollout_percentage between 0 and 100
// (exclusive), a deterministic hash of (ID, key) decides whether this
// tenant falls inside the rollout cohort (BL-013 / Section 42). This gives a
// stable per-tenant-per-flag result: the same tenant always sees the same
// outcome for a given flag until the percentage or enabled state changes.
// Returns (default, ErrNoRows) when the flag is not set so the caller can
// distinguish "not set" from a real DB error.
func (c *Checker) queryEnabled(ctx context.Context, key string) (bool, error) {
	var enabled bool

	var configBytes []byte

	err := c.db.Reader.QueryRow(ctx, `
		SELECT enabled, config FROM feature_flags
		WHERE key = $1
	`, key).Scan(&enabled, &configBytes)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("features: query failed, using registry default", "key", key, "error", err)
		}

		return DefaultFor(key), err
	}

	if !enabled {
		return false, nil
	}

	// BL-013 / Section 42: gradual rollout. Parse the config blob for a
	// rollout_percentage. 0 or 100 means "off" or "fully on" — no hash needed.
	var cfg FlagConfig
	if len(configBytes) > 0 {
		if jsonErr := json.Unmarshal(configBytes, &cfg); jsonErr == nil {
			if cfg.RolloutPercentage > 0 && cfg.RolloutPercentage < 100 {
				return hashInRollout(key, cfg.RolloutPercentage), nil
			}
		}
	}

	return true, nil
}

// hashInRollout deterministically decides whether a (ID, key) pair falls
// inside the rollout cohort. It hashes the pair into a 0–99 bucket and returns
// true when bucket < percentage. The result is stable for a given pair.
func hashInRollout(key string, percentage int) bool {
	h := fnvHash64("" + ":" + key)
	bucket := int(h % 100) // #nosec

	return bucket < percentage
}

// fnvHash64 is a small FNV-1a 64-bit implementation used for deterministic
// rollout bucketing (BL-013). Avoids importing hash/fnv to keep the import
// list lean; the algorithm is public domain and stable.
func fnvHash64(s string) uint64 {
	const (
		offset64 uint64 = 14695981039346656037
		prime64  uint64 = 1099511628211
	)

	h := offset64
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= prime64
	}

	return h
}

// cacheGet retrieves a cached flag value if still valid.
func (c *Checker) cacheGet(flagKey string) *bool {
	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()

	if entry, ok := c.cache.entries[flagKey]; ok {
		return &entry.enabled
	}

	return nil
}

// cacheSet stores a flag value in the cache with current timestamp.
func (c *Checker) cacheSet(flagKey string, enabled bool) {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()

	c.cache.entries[flagKey] = &flagCacheEntry{
		enabled:   enabled,
		createdAt: time.Now(),
	}
}

// ClearCache clears all cached flag values (in-memory and Valkey).
func (c *Checker) ClearCache() {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	c.cache.entries = make(map[string]*flagCacheEntry)
	c.valkeyFlushAll(context.Background())
}

// valkeyGet retrieves a cached flag value from Valkey (L2).
// Returns nil on cache miss or when Valkey is unavailable.
func (c *Checker) valkeyGet(ctx context.Context, flagKey string) *bool {
	if c.vk == nil {
		return nil
	}

	val, err := c.vk.Get(ctx, flagCacheKeyPrefix+flagKey).Result()
	if err != nil {
		return nil
	}

	enabled := val == "1"

	return &enabled
}

// valkeySet stores a flag value in Valkey with a 5-minute TTL.
// Failures are logged but never propagated — the L1 cache + DB remain
// authoritative.
func (c *Checker) valkeySet(ctx context.Context, flagKey string, enabled bool) {
	if c.vk == nil {
		return
	}

	val := "0"
	if enabled {
		val = "1"
	}

	if err := c.vk.Set(ctx, flagCacheKeyPrefix+flagKey, val, flagCacheTTL).Err(); err != nil {
		slog.Warn("features: failed to write valkey cache", "key", flagKey, "error", err)
	}
}

// valkeyFlushAll scans and deletes all jc:flags:* keys from Valkey.
// Failures are logged but not propagated.
func (c *Checker) valkeyFlushAll(ctx context.Context) {
	if c.vk == nil {
		return
	}

	pattern := flagCacheKeyPrefix + "*"

	var cursor uint64

	for {
		keys, nextCursor, err := c.vk.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("features: failed to scan valkey for cache flush", "error", err)
			return
		}

		if len(keys) > 0 {
			if err := c.vk.Del(ctx, keys...).Err(); err != nil {
				slog.Warn("features: failed to delete valkey cache keys", "count", len(keys), "error", err)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// cleanupLoop periodically evicts cache entries older than maxAge.
// Stops when the done channel is closed (graceful shutdown).
func (fc *flagCache) cleanupLoop(maxAge time.Duration) {
	ticker := time.NewTicker(maxAge)
	defer ticker.Stop()

	for {
		select {
		case <-fc.done:
			return
		case <-ticker.C:
			fc.mu.Lock()
			now := time.Now()

			for key, entry := range fc.entries {
				if now.Sub(entry.createdAt) > maxAge {
					delete(fc.entries, key)
				}
			}
			fc.mu.Unlock()
		}
	}
}

// GetAllEnabled returns a map of all feature keys to their enabled state
// for the given tenant. Used by the frontend layout to get all flags at once.
// Fetches all tenant flag rows in a single DB query instead of issuing one
// query per registry key (eliminates the N+1 pattern on cold cache).
func (c *Checker) GetAllEnabled(ctx context.Context) map[string]bool {
	result := make(map[string]bool)

	// Start with registry defaults.
	for _, f := range Registry {
		result[f.Key] = f.Default
	}

	// Batch-fetch all overrides in one round-trip, then overlay.
	rows, err := c.db.Reader.Query(ctx, `
		SELECT key, enabled, config FROM feature_flags
	`)
	if err != nil {
		// On DB error, fall back to defaults (already set above).
		slog.Warn("features: batch query failed, using registry defaults", "error", err)
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var r struct {
			Key     string
			Enabled bool
			Config  []byte
		}

		if err := rows.Scan(&r.Key, &r.Enabled, &r.Config); err != nil {
			slog.Warn("features: scanning batch row", "error", err)
			continue
		}

		if !r.Enabled {
			result[r.Key] = false
			continue
		}

		// Check rollout percentage in the config blob.
		var cfg FlagConfig
		if len(r.Config) > 0 {
			if jsonErr := json.Unmarshal(r.Config, &cfg); jsonErr == nil {
				if cfg.RolloutPercentage > 0 && cfg.RolloutPercentage < 100 {
					result[r.Key] = hashInRollout(r.Key, cfg.RolloutPercentage)
					continue
				}
			}
		}

		result[r.Key] = true
	}

	return result
}

// SetEnabled updates a feature flag in the database.
func (c *Checker) SetEnabled(ctx context.Context, key string, enabled bool, updatedBy pgtype.UUID) error {
	_, err := c.db.Writer.Exec(ctx, `
		INSERT INTO feature_flags (key, enabled, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET enabled = $2, updated_by = $3, updated_at = now()
	`, key, enabled, updatedBy)
	if err != nil {
		return fmt.Errorf("updating feature flag %s: %w", key, err)
	}

	return nil
}

// FlagConfig is the richer admin-facing metadata stored alongside a flag's
// enabled state in the feature_flags.config jsonb column. Storage only
// persists the key → enabled mapping plus this config blob; everything the
// admin UI shows beyond that (name, description, environment, rollout) lives
// here so it can be edited without changing the registry.
type FlagConfig struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	Environment       string `json:"environment"`
	RolloutPercentage int    `json:"rolloutPercentage"`
}

// UpdateFlag upserts a feature flag row, persisting both its enabled state and
// the richer admin metadata. Unlike SetEnabled, it accepts the full config so
// the admin UI can edit name/description/environment/rollout in one call.
func (c *Checker) UpdateFlag(ctx context.Context, key string, enabled bool, cfg FlagConfig, updatedBy pgtype.UUID) error {
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling feature flag config %s: %w", key, err)
	}

	_, err = c.db.Writer.Exec(ctx, `
		INSERT INTO feature_flags (key, enabled, config, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key)
		DO UPDATE SET enabled = $2, config = $3, updated_by = $4, updated_at = now()
	`, key, enabled, configJSON, updatedBy)
	if err != nil {
		return fmt.Errorf("upserting feature flag %s: %w", key, err)
	}

	return nil
}

// DeleteFlag removes a feature flag row. After deletion the
// flag falls back to its registry Default, exactly as if it had never been
// set. The registry key itself is never removed.
func (c *Checker) DeleteFlag(ctx context.Context, key string) error {
	_, err := c.db.Writer.Exec(ctx, `
		DELETE FROM feature_flags
		WHERE key = $1
	`, key)
	if err != nil {
		return fmt.Errorf("deleting feature flag %s: %w", key, err)
	}

	return nil
}

// ListFlagRows returns every feature_flags row, registry key or not. Used by
// ListFeatureFlags so admin-created keys (not in the static Registry) still
// appear in the admin UI.
func (c *Checker) ListFlagRows(ctx context.Context) ([]FlagRow, error) {
	rows, err := c.db.Reader.Query(ctx, `
		SELECT key, enabled, config, updated_at
		FROM feature_flags
	`)
	if err != nil {
		return nil, fmt.Errorf("listing feature flags: %w", err)
	}
	defer rows.Close()

	out := make([]FlagRow, 0)

	for rows.Next() {
		var r FlagRow
		if err := rows.Scan(&r.Key, &r.Enabled, &r.Config, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning feature flag row: %w", err)
		}

		out = append(out, r)
	}

	return out, rows.Err()
}

// FlagRow is a raw feature_flags table row for a tenant.
type FlagRow struct {
	Key       string
	Enabled   bool
	Config    []byte
	UpdatedAt time.Time
}

// GetFlagConfig returns the stored FlagConfig for a feature flag, or a
// zero-value FlagConfig when the flag has no row or no config blob.
func (c *Checker) GetFlagConfig(ctx context.Context, key string) (FlagConfig, error) {
	var configBytes []byte

	err := c.db.Reader.QueryRow(ctx, `
		SELECT config FROM feature_flags
		WHERE key = $1
	`, key).Scan(&configBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FlagConfig{}, nil
		}

		return FlagConfig{}, fmt.Errorf("querying feature flag config %s: %w", key, err)
	}

	if len(configBytes) == 0 {
		return FlagConfig{}, nil
	}

	var cfg FlagConfig
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return FlagConfig{}, nil
	}

	return cfg, nil
}

// SetRolloutPercentage updates the rollout_percentage in a feature flag's
// config blob without changing its enabled state. Used by the Control Panel
// extensions page to manage gradual rollout. A percentage of 0 or 100 means
// "off" or "fully on" (no hash bucketing).
func (c *Checker) SetRolloutPercentage(ctx context.Context, key string, percentage int, updatedBy pgtype.UUID) error {
	cfg, err := c.GetFlagConfig(ctx, key)
	if err != nil {
		return fmt.Errorf("reading config for rollout update %s: %w", key, err)
	}

	cfg.RolloutPercentage = percentage

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config for %s: %w", key, err)
	}

	// Use the registry default for the INSERT case (new row) so we don't
	// accidentally enable a disabled feature just by setting rollout. The
	// ON CONFLICT clause only updates config, preserving the existing
	// enabled state.
	defaultEnabled := DefaultFor(key)

	_, err = c.db.Writer.Exec(ctx, `
		INSERT INTO feature_flags (key, enabled, config, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key)
		DO UPDATE SET config = $3, updated_by = $4, updated_at = now()
	`, key, defaultEnabled, configJSON, updatedBy)
	if err != nil {
		return fmt.Errorf("updating rollout for %s: %w", key, err)
	}

	c.ClearCache()

	return nil
}

// Gate is HTTP middleware that checks if a feature is enabled.
// If disabled, returns 404 with {"error":{"code":"feature_disabled","message":"..."}}.
// This MUST be used inside the tenant middleware group so tenant
// context is available.
func Gate(checker *Checker, key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {


			if !checker.IsEnabled(r.Context(), key) {
				errpkg.WriteError(w, http.StatusNotFound, "feature_disabled", "This feature is not available")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
