package features

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// flagCacheTTL is the TTL for both the in-memory L1 cache and the Valkey L2
// cache. Per Section 42, feature flag caches use a 5-minute TTL.
const flagCacheTTL = 5 * time.Minute

// flagCacheKeyPrefix namespaces all feature flag cache keys in Valkey.
const flagCacheKeyPrefix = "jc:flags:"

// Checker resolves whether a feature is enabled for a given tenant.
// It uses a two-tier cache: L1 (in-memory, 60s TTL for fast eviction) and
// L2 (Valkey, 5min TTL for cross-instance consistency). When Valkey is
// unavailable, it gracefully degrades to L1-only operation.
type Checker struct {
	db    *db.DB
	cache *flagCache
	vk    valkeycompat.Cmdable // nil when Valkey is unavailable (graceful degradation)
}

// flagCacheEntry wraps a flag value with its creation time for TTL eviction.
type flagCacheEntry struct {
	enabled   bool
	createdAt time.Time
}

// flagCache provides caching of feature flag lookups with TTL-based eviction.
type flagCache struct {
	mu      sync.RWMutex
	entries map[string]map[string]*flagCacheEntry // tenant_id -> key -> entry
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
			entries: make(map[string]map[string]*flagCacheEntry),
			done:    make(chan struct{}),
		},
		vk: vkClient,
	}
	go c.cache.cleanupLoop(60 * time.Second)

	return c
}

// Close stops the background cleanup goroutine and releases resources.
func (c *Checker) Close() {
	if c.cache.done != nil {
		select {
		case <-c.cache.done:
			// already closed
		default:
			close(c.cache.done)
		}
	}
}

// IsEnabled checks if a feature is enabled for the tenant in context.
// If the feature_flags table has no row for the key, the registry default
// is used. The result is cached in L1 (in-memory) and L2 (Valkey).
// Lookup order: L1 → L2 → DB. Results are cached at both layers.
func (c *Checker) IsEnabled(ctx context.Context, tenantID pgtype.UUID, key string) bool {
	tenantKey := tenantID.String()
	if cached := c.cacheGet(tenantKey, key); cached != nil {
		return *cached
	}

	// L2 — Valkey (shared across server instances)
	if cached := c.valkeyGet(ctx, tenantKey, key); cached != nil {
		c.cacheSet(tenantKey, key, *cached)
		return *cached
	}

	// DB — source of truth
	enabled := c.queryEnabled(ctx, tenantID, key)
	c.cacheSet(tenantKey, key, enabled)
	c.valkeySet(ctx, tenantKey, key, enabled)

	return enabled
}

// queryEnabled queries the database for the feature flag. When the flag is
// enabled and its config carries a rollout_percentage between 0 and 100
// (exclusive), a deterministic hash of (tenantID, key) decides whether this
// tenant falls inside the rollout cohort (BL-013 / Section 42). This gives a
// stable per-tenant-per-flag result: the same tenant always sees the same
// outcome for a given flag until the percentage or enabled state changes.
func (c *Checker) queryEnabled(ctx context.Context, tenantID pgtype.UUID, key string) bool {
	var enabled bool
	var configBytes []byte

	err := c.db.Reader.QueryRow(ctx, `
		SELECT enabled, config FROM feature_flags
		WHERE key = $1 AND tenant_id = $2
	`, key, tenantID).Scan(&enabled, &configBytes)
	if err != nil {
		return DefaultFor(key)
	}

	if !enabled {
		return false
	}

	// BL-013 / Section 42: gradual rollout. Parse the config blob for a
	// rollout_percentage. 0 or 100 means "off" or "fully on" — no hash needed.
	var cfg FlagConfig
	if len(configBytes) > 0 {
		if jsonErr := json.Unmarshal(configBytes, &cfg); jsonErr == nil {
			if cfg.RolloutPercentage > 0 && cfg.RolloutPercentage < 100 {
				return hashInRollout(tenantID, key, cfg.RolloutPercentage)
			}
		}
	}

	return true
}

// hashInRollout deterministically decides whether a (tenantID, key) pair falls
// inside the rollout cohort. It hashes the pair into a 0–99 bucket and returns
// true when bucket < percentage. The result is stable for a given pair.
func hashInRollout(tenantID pgtype.UUID, key string, percentage int) bool {
	h := fnvHash64(tenantID.String() + ":" + key)
	bucket := int(h % 100)
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
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

// cacheGet retrieves a cached flag value if still valid.
func (c *Checker) cacheGet(tenantKey, flagKey string) *bool {
	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()

	if tenantFlags, ok := c.cache.entries[tenantKey]; ok {
		if entry, ok := tenantFlags[flagKey]; ok {
			return &entry.enabled
		}
	}

	return nil
}

// cacheSet stores a flag value in the cache with current timestamp.
func (c *Checker) cacheSet(tenantKey, flagKey string, enabled bool) {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()

	if _, ok := c.cache.entries[tenantKey]; !ok {
		c.cache.entries[tenantKey] = make(map[string]*flagCacheEntry)
	}

	c.cache.entries[tenantKey][flagKey] = &flagCacheEntry{
		enabled:   enabled,
		createdAt: time.Now(),
	}
}

// ClearCache clears all cached flag values.
func (c *Checker) ClearCache() {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	c.cache.entries = make(map[string]map[string]*flagCacheEntry)
}

// ClearCacheForTenant clears cached flag values for a single tenant only
// (STD-003 — avoids clearing all tenants' caches on a per-tenant flag update).
// Both L1 (in-memory) and L2 (Valkey) caches are invalidated.
func (c *Checker) ClearCacheForTenant(tenantID string) {
	// Evict from L1.
	c.cache.mu.Lock()
	delete(c.cache.entries, tenantID)
	c.cache.mu.Unlock()

	// Evict from L2 — SCAN and DELETE all jc:flags:{tenant_id}:* keys.
	c.valkeyFlushTenant(context.Background(), tenantID)
}

// valkeyGet retrieves a cached flag value from Valkey (L2).
// Returns nil on cache miss or when Valkey is unavailable.
func (c *Checker) valkeyGet(ctx context.Context, tenantKey, flagKey string) *bool {
	if c.vk == nil {
		return nil
	}

	val, err := c.vk.Get(ctx, flagCacheKeyPrefix+tenantKey+":"+flagKey).Result()
	if err != nil {
		return nil
	}

	enabled := val == "1"
	return &enabled
}

// valkeySet stores a flag value in Valkey with a 5-minute TTL.
// Failures are logged but never propagated — the L1 cache + DB remain
// authoritative.
func (c *Checker) valkeySet(ctx context.Context, tenantKey, flagKey string, enabled bool) {
	if c.vk == nil {
		return
	}

	val := "0"
	if enabled {
		val = "1"
	}

	if err := c.vk.Set(ctx, flagCacheKeyPrefix+tenantKey+":"+flagKey, val, flagCacheTTL).Err(); err != nil {
		slog.Warn("features: failed to write valkey cache", "key", flagKey, "error", err)
	}
}

// valkeyFlushTenant scans and deletes all jc:flags:{tenant_id}:* keys from
// Valkey for the given tenant. Failures are logged but not propagated.
func (c *Checker) valkeyFlushTenant(ctx context.Context, tenantID string) {
	if c.vk == nil {
		return
	}

	pattern := flagCacheKeyPrefix + tenantID + ":*"
	var cursor uint64

	for {
		keys, nextCursor, err := c.vk.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("features: failed to scan valkey for cache flush", "tenant", tenantID, "error", err)
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

			for tenantKey, tenantFlags := range fc.entries {
				for key, entry := range tenantFlags {
					if now.Sub(entry.createdAt) > maxAge {
						delete(tenantFlags, key)
					}
				}

				if len(tenantFlags) == 0 {
					delete(fc.entries, tenantKey)
				}
			}
			fc.mu.Unlock()
		}
	}
}

// GetAllEnabled returns a map of all feature keys to their enabled state
// for the given tenant. Used by the frontend layout to get all flags at once.
func (c *Checker) GetAllEnabled(ctx context.Context, tenantID pgtype.UUID) map[string]bool {
	result := make(map[string]bool)
	for _, f := range Registry {
		result[f.Key] = c.IsEnabled(ctx, tenantID, f.Key)
	}

	return result
}

// SetEnabled updates a feature flag in the database.
func (c *Checker) SetEnabled(ctx context.Context, tenantID pgtype.UUID, key string, enabled bool, updatedBy pgtype.UUID) error {
	_, err := c.db.Writer.Exec(ctx, `
		INSERT INTO feature_flags (key, tenant_id, enabled, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key, tenant_id)
		DO UPDATE SET enabled = $3, updated_by = $4, updated_at = now()
	`, key, tenantID, enabled, updatedBy)
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
func (c *Checker) UpdateFlag(ctx context.Context, tenantID pgtype.UUID, key string, enabled bool, cfg FlagConfig, updatedBy pgtype.UUID) error {
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling feature flag config %s: %w", key, err)
	}

	_, err = c.db.Writer.Exec(ctx, `
		INSERT INTO feature_flags (key, tenant_id, enabled, config, updated_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (key, tenant_id)
		DO UPDATE SET enabled = $3, config = $4, updated_by = $5, updated_at = now()
	`, key, tenantID, enabled, configJSON, updatedBy)
	if err != nil {
		return fmt.Errorf("upserting feature flag %s: %w", key, err)
	}

	return nil
}

// DeleteFlag removes a feature flag row for the tenant. After deletion the
// flag falls back to its registry Default, exactly as if it had never been
// set. The registry key itself is never removed — only the tenant override.
func (c *Checker) DeleteFlag(ctx context.Context, tenantID pgtype.UUID, key string) error {
	_, err := c.db.Writer.Exec(ctx, `
		DELETE FROM feature_flags
		WHERE key = $1 AND tenant_id = $2
	`, key, tenantID)
	if err != nil {
		return fmt.Errorf("deleting feature flag %s: %w", key, err)
	}

	return nil
}

// ListFlagRows returns every feature_flags row for the tenant, registry key or
// not. Used by ListFeatureFlags so admin-created keys (not in the static
// Registry) still appear in the admin UI.
func (c *Checker) ListFlagRows(ctx context.Context, tenantID pgtype.UUID) ([]FlagRow, error) {
	rows, err := c.db.Reader.Query(ctx, `
		SELECT key, enabled, config, updated_at
		FROM feature_flags
		WHERE tenant_id = $1
	`, tenantID)
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

// Gate is HTTP middleware that checks if a feature is enabled.
// If disabled, returns 404 with {"error":{"code":"feature_disabled","message":"..."}}.
// This MUST be used inside the tenant middleware group so tenant
// context is available.
func Gate(checker *Checker, key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := tenant.FromContext(r.Context())
			if t == nil {
				writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
				return
			}

			if !checker.IsEnabled(r.Context(), t.ID, key) {
				writeError(w, http.StatusNotFound, "feature_disabled", "This feature is not available")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
