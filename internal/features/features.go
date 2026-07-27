package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/tenant"
)

// Checker resolves whether a feature is enabled for a given tenant.
// It caches results with automatic TTL-based eviction to prevent stale flags.
type Checker struct {
	db    *db.DB
	cache *flagCache
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
}

// New creates a feature flag Checker backed by the given database.
// A background goroutine evicts cache entries older than 60 seconds.
func New(database *db.DB) *Checker {
	c := &Checker{
		db: database,
		cache: &flagCache{
			entries: make(map[string]map[string]*flagCacheEntry),
		},
	}
	go c.cache.cleanupLoop(60 * time.Second)

	return c
}

// IsEnabled checks if a feature is enabled for the tenant in context.
// If the feature_flags table has no row for the key, the registry default
// is used. The result is cached per-request.
func (c *Checker) IsEnabled(ctx context.Context, tenantID pgtype.UUID, key string) bool {
	tenantKey := tenantID.String()
	if cached := c.cacheGet(tenantKey, key); cached != nil {
		return *cached
	}

	enabled := c.queryEnabled(ctx, tenantID, key)
	c.cacheSet(tenantKey, key, enabled)

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
func (c *Checker) ClearCacheForTenant(tenantID string) {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	delete(c.cache.entries, tenantID)
}

// cleanupLoop periodically evicts cache entries older than maxAge.
func (fc *flagCache) cleanupLoop(maxAge time.Duration) {
	ticker := time.NewTicker(maxAge)
	defer ticker.Stop()

	for range ticker.C {
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
// If disabled, returns 404 with {"error":"feature_disabled"}.
// This MUST be used inside the tenant middleware group so tenant
// context is available.
func Gate(checker *Checker, key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := tenant.FromContext(r.Context())
			if t == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "feature_disabled"})

				return
			}

			if !checker.IsEnabled(r.Context(), t.ID, key) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "feature_disabled"})

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
