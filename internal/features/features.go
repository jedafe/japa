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

// queryEnabled queries the database for the feature flag.
func (c *Checker) queryEnabled(ctx context.Context, tenantID pgtype.UUID, key string) bool {
	var enabled bool

	err := c.db.Reader.QueryRow(ctx, `
		SELECT enabled FROM feature_flags
		WHERE key = $1 AND tenant_id = $2
	`, key, tenantID).Scan(&enabled)
	if err != nil {
		return DefaultFor(key)
	}

	return enabled
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
