package features

import (
	"context"
	"regexp"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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

// flagAllCacheKey is the Valkey/L1 key for the aggregate snapshot of every
// feature_flags row (JSON array of FlagRow). It lives under the jc:flags:
// prefix so the SCAN-based flush in valkeyFlushAll invalidates it alongside
// per-key entries. Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §5.3.
const flagAllCacheKey = "_all"

// FeatureChecker is the interface for checking whether a feature flag is
// enabled for a given platform. Services that need to verify their own feature
// flag (e.g. story.Service.Enabled) should accept this interface rather than
// reading system_settings directly, so that feature_flags remains the single
// source of truth for feature reachability.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Checker resolves whether a feature is enabled for a given platform.
// It uses a two-tier cache: L1 (in-memory, 60s TTL for fast eviction) and
// L2 (Valkey, 5min TTL for cross-instance consistency). When Valkey is
// unavailable, it gracefully degrades to L1-only operation.
type Checker struct {
	db        *db.DB
	cache     *flagCache
	vk        valkeycompat.Cmdable // nil when Valkey is unavailable (graceful degradation)
	closeOnce sync.Once
	// audienceResolver supplies caller group/permission data for EvalFlag.
	// Optional — set once at startup via SetAudienceResolver before serving
	// traffic; nil means every caller evaluates as anonymous (audience-scoped
	// flags are off for them).
	audienceResolver AudienceResolver
	// onConfigChanged fires after every successful flag write (SetEnabled,
	// PatchFlag, SetRolloutPercentage, DeleteFlag) with the flag's post-write
	// config. The hook lives on Checker — not Handlers — because the
	// /control/extensions write paths call SetEnabled/SetRolloutPercentage
	// directly and would silently skip a handler-level hook. Optional — set
	// once at startup via SetOnConfigChanged.
	onConfigChanged func(ctx context.Context, key string, cfg FlagConfig)

	// platformID is this install's stable identity (pg_control_system
	// system_identifier), lazily loaded once. It salts rollout bucketing so
	// the same flag at the same percentage lands in different cohorts on
	// different installs — rollout_percentage is only meaningful when the
	// hash input varies across platforms.
	platformID     string
	platformIDOnce sync.Once
}

// flagCacheEntry wraps a flag's resolved enabled state and raw config blob
// with its creation time for TTL eviction. Config is cached alongside the
// bool because scoped evaluation (EvalFlag) needs the per-surface rules —
// without it every gated request would hit Postgres.
type flagCacheEntry struct {
	enabled   bool
	config    []byte
	createdAt time.Time
}

// flagAllEntry snapshots the full feature_flags table for EvalAll and
// ListExtensions so neither runs ListFlagRows per request.
type flagAllEntry struct {
	rows      []FlagRow
	createdAt time.Time
}

// flagCache provides caching of feature flag lookups with TTL-based eviction.
type flagCache struct {
	mu      sync.RWMutex
	entries map[string]*flagCacheEntry // flag key -> entry
	all     *flagAllEntry              // aggregate row snapshot (jc:flags:_all)
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

// SetOnConfigChanged registers a callback fired after every successful flag
// write — SetEnabled, PatchFlag, SetRolloutPercentage, DeleteFlag — with the
// flag's post-write config. DeleteFlag fires with an empty FlagConfig{} so
// the consumer (roadmap sync) unlinks the flag's item per spec §4.3. Wired
// in server.go to roadmap.Service.SyncFlagPhase.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §4.4.
func (c *Checker) SetOnConfigChanged(fn func(ctx context.Context, key string, cfg FlagConfig)) {
	c.onConfigChanged = fn
}

// fireConfigChanged invokes the registered config-change hook fire-and-forget
// (Rule 8): the goroutine detaches from the request context via
// context.WithoutCancel so a slow consumer (roadmap sync, cache warmers)
// never delays the write response.
func (c *Checker) fireConfigChanged(ctx context.Context, key string, cfg FlagConfig) {
	if c.onConfigChanged == nil {
		return
	}

	go c.onConfigChanged(context.WithoutCancel(ctx), key, cfg)
}

// IsEnabled checks if a feature is enabled for the platform in context.
// If the feature_flags table has no row for the key, the registry default
// is used. The result is cached in L1 (in-memory) and L2 (Valkey).
// Lookup order: L1 → L2 → DB. Results are cached at both layers.
// On transient DB errors the default is returned for the current request
// but NOT cached — the next request retries the DB.
func (c *Checker) IsEnabled(ctx context.Context, key string) bool {
	// Ref: .jeos/sections/42-feature-flags.md (removal in progress).
	// Global resolution only — surface/geo/audience overlays live in EvalFlag.
	enabled, _ := c.resolveFlag(ctx, key)

	return enabled
}

// resolveFlag is the shared L1 → L2 → DB lookup behind IsEnabled and EvalFlag.
// It returns the rollout-effective enabled bool plus the raw config blob the
// scoped evaluator needs for surface/geo/audience rules. On transient DB
// errors the registry default is returned but NOT cached — the next request
// retries the DB.
func (c *Checker) resolveFlag(ctx context.Context, key string) (bool, []byte) {
	// L1 — in-memory (hot path)
	if cached := c.cacheGet(key); cached != nil {
		return cached.enabled, cached.config
	}

	// L2 — Valkey (shared across server instances)
	if cached := c.valkeyGet(ctx, key); cached != nil {
		c.cacheSet(key, cached.enabled, cached.config)
		return cached.enabled, cached.config
	}

	// DB — source of truth
	if c.db == nil {
		return DefaultFor(key), nil
	}

	enabled, config, err := c.queryFlagRow(ctx, key)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		// Transient DB error (connection failure, context cancellation, etc.).
		// Return the registry default for this request but do NOT cache it —
		// caching a default from a failed query would poison the cache for
		// up to 5 minutes (L2), silently flipping features on or off.
		return enabled, nil
	}

	// Success or ErrNoRows (legitimate "flag not set" → use default).
	// The cached enabled value is rollout-effective; config stays raw so
	// EvalFlag can apply surface/geo/audience rules on top.
	effective := effectiveEnabled(c.rolloutID(ctx), key, enabled, config)
	c.cacheSet(key, effective, config)
	c.valkeySet(ctx, key, effective, config)

	return effective, config
}

// QueryEnabled reads the flag's effective enabled state straight from the DB,
// bypassing the L1/L2 caches. Write handlers use it for the pre-write
// snapshot that drives off→on transition detection — a cached value could be
// stale (written by another instance within the cache TTL) and would skip the
// activation hook. Falls back to IsEnabled on transient DB errors.
func (c *Checker) QueryEnabled(ctx context.Context, key string) bool {
	enabled, err := c.queryEnabled(ctx, key)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return c.IsEnabled(ctx, key)
	}

	return enabled
}

// queryEnabled queries the database for the feature flag. When the flag is
// enabled and its config carries a rollout_percentage between 0 and 100
// (exclusive), a deterministic hash of (ID, key) decides whether this
// platform falls inside the rollout cohort (BL-013 / Section 42). This gives a
// stable per-platform-per-flag result: the same platform always sees the same
// outcome for a given flag until the percentage or enabled state changes.
// Returns (default, ErrNoRows) when the flag is not set so the caller can
// distinguish "not set" from a real DB error.
func (c *Checker) queryEnabled(ctx context.Context, key string) (bool, error) {
	enabled, config, err := c.queryFlagRow(ctx, key)
	if err != nil {
		return enabled, err
	}

	return effectiveEnabled(c.rolloutID(ctx), key, enabled, config), nil
}

// queryFlagRow reads the raw feature_flags row for a key. Returns
// (DefaultFor(key), nil, err) on failure so callers can distinguish
// "not set" (ErrNoRows → registry default) from a real DB error.
func (c *Checker) queryFlagRow(ctx context.Context, key string) (bool, []byte, error) {
	if c.db == nil {
		return DefaultFor(key), nil, pgx.ErrNoRows
	}

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

		return DefaultFor(key), nil, err
	}

	return enabled, configBytes, nil
}

// effectiveEnabled applies the BL-013 / Section 42 gradual-rollout rule to a
// raw (enabled, config) pair: when enabled and rollout_percentage is between
// 0 and 100 (exclusive), a deterministic hash decides whether this platform
// falls inside the rollout cohort. The same flag always sees the same outcome
// until the percentage or enabled state changes.
func effectiveEnabled(id, key string, enabled bool, config []byte) bool {
	if !enabled {
		return false
	}

	var cfg FlagConfig
	if len(config) > 0 {
		if jsonErr := json.Unmarshal(config, &cfg); jsonErr == nil {
			if cfg.RolloutPercentage > 0 && cfg.RolloutPercentage < 100 {
				return hashInRollout(id, key, cfg.RolloutPercentage)
			}
		}
	}

	return true
}

// hashInRollout deterministically decides whether a (ID, key) pair falls
// inside the rollout cohort. It hashes the pair into a 0–99 bucket and returns
// true when bucket < percentage. The result is stable for a given pair.
func hashInRollout(id, key string, percentage int) bool {
	h := fnvHash64(id + ":" + key)
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

// rolloutID returns the install's stable platform identifier used to salt
// rollout bucketing — Postgres's own system_identifier, read once via
// pg_control_system() and memoized. On any failure it returns "" so the
// hash degrades to the historical key-only behavior rather than breaking
// flag evaluation.
func (c *Checker) rolloutID(ctx context.Context) string {
	c.platformIDOnce.Do(func() {
		if c.db == nil {
			return
		}
		if err := c.db.Reader.QueryRow(ctx,
			`SELECT system_identifier FROM pg_control_system()`).Scan(&c.platformID); err != nil {
			slog.Warn("features: platform id unavailable, rollout bucketing unsalted", "error", err)
		}
	})

	return c.platformID
}

// cacheGet retrieves a cached flag entry (enabled + config) if still valid.
func (c *Checker) cacheGet(flagKey string) *flagCacheEntry {
	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()

	if entry, ok := c.cache.entries[flagKey]; ok {
		return entry
	}

	return nil
}

// cacheSet stores a flag value in the cache with current timestamp.
func (c *Checker) cacheSet(flagKey string, enabled bool, config []byte) {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()

	c.cache.entries[flagKey] = &flagCacheEntry{
		enabled:   enabled,
		config:    config,
		createdAt: time.Now(),
	}
}

// cacheAllSet stores the aggregate feature_flags snapshot in L1.
func (c *Checker) cacheAllSet(rows []FlagRow) {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()

	c.cache.all = &flagAllEntry{rows: rows, createdAt: time.Now()}
}

// ClearCache clears all cached flag values (in-memory and Valkey), including
// the jc:flags:_all aggregate — the SCAN pattern covers it.
func (c *Checker) ClearCache() {
	c.cache.mu.Lock()
	c.cache.entries = make(map[string]*flagCacheEntry)
	c.cache.all = nil
	c.cache.mu.Unlock()
	c.valkeyFlushAll(context.Background())
}

// InvalidateFlag drops a single flag's cached resolution plus the shared
// aggregate snapshot (which contains that flag's row). Write paths that know
// the affected key should prefer this over ClearCache.
func (c *Checker) InvalidateFlag(ctx context.Context, flagKey string) {
	c.cache.mu.Lock()
	delete(c.cache.entries, flagKey)
	c.cache.all = nil
	c.cache.mu.Unlock()

	if c.vk == nil {
		return
	}

	if err := c.vk.Del(ctx, flagCacheKeyPrefix+flagKey, flagCacheKeyPrefix+flagAllCacheKey).Err(); err != nil {
		slog.Warn("features: failed to invalidate valkey cache", "key", flagKey, "error", err)
	}
}

// flagCacheValue is the L2 wire format stored at jc:flags:<key>. The raw
// config rides alongside the resolved bool so scoped evaluation never needs
// a Postgres round-trip on a cache hit.
type flagCacheValue struct {
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}

// valkeyGet retrieves a cached flag entry from Valkey (L2).
// Returns nil on cache miss, decode failure, or when Valkey is unavailable.
// Accepts the current JSON {enabled, config} value and the legacy bare
// "0"/"1"/"true"/"false" forms (decoded with an empty config).
func (c *Checker) valkeyGet(ctx context.Context, flagKey string) *flagCacheEntry {
	if c.vk == nil {
		return nil
	}

	val, err := c.vk.Get(ctx, flagCacheKeyPrefix+flagKey).Result()
	if err != nil {
		return nil
	}

	switch val {
	case "1", "true":
		return &flagCacheEntry{enabled: true}
	case "0", "false":
		return &flagCacheEntry{enabled: false}
	}

	var v flagCacheValue
	if err := json.Unmarshal([]byte(val), &v); err != nil {
		return nil
	}

	return &flagCacheEntry{enabled: v.Enabled, config: []byte(v.Config)}
}

// valkeySet stores a flag value in Valkey with a 5-minute TTL.
// Failures are logged but never propagated — the L1 cache + DB remain
// authoritative.
func (c *Checker) valkeySet(ctx context.Context, flagKey string, enabled bool, config []byte) {
	if c.vk == nil {
		return
	}

	payload := flagCacheValue{Enabled: enabled, Config: json.RawMessage(config)}
	if len(payload.Config) == 0 {
		payload.Config = json.RawMessage(`{}`)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("features: failed to marshal valkey cache value", "key", flagKey, "error", err)
		return
	}

	if err := c.vk.Set(ctx, flagCacheKeyPrefix+flagKey, string(raw), flagCacheTTL).Err(); err != nil {
		slog.Warn("features: failed to write valkey cache", "key", flagKey, "error", err)
	}
}

// AllFlagRows returns every feature_flags row through the L1 → L2 → DB
// cascade backed by the jc:flags:_all aggregate. EvalAll and the control
// panel's ListExtensions both need every row's config; this keeps them off
// ListFlagRows' per-request Postgres query. On DB error it returns nil so
// callers can fall back to registry defaults — failures are never cached.
func (c *Checker) AllFlagRows(ctx context.Context) []FlagRow {
	c.cache.mu.RLock()
	cached := c.cache.all
	c.cache.mu.RUnlock()

	if cached != nil {
		return cached.rows
	}

	// L2 — Valkey aggregate (JSON array of FlagRow)
	if c.vk != nil {
		if val, err := c.vk.Get(ctx, flagCacheKeyPrefix+flagAllCacheKey).Result(); err == nil && val != "" {
			var rows []FlagRow
			if jsonErr := json.Unmarshal([]byte(val), &rows); jsonErr == nil {
				c.cacheAllSet(rows)
				return rows
			}
		}
	}

	// DB — source of truth
	if c.db == nil {
		return nil
	}

	rows, err := c.ListFlagRows(ctx)
	if err != nil {
		slog.Warn("features: aggregate row query failed", "error", err)
		return nil
	}

	c.cacheAllSet(rows)
	c.valkeySetAll(ctx, rows)

	return rows
}

// valkeySetAll stores the aggregate FlagRow snapshot in Valkey with a
// 5-minute TTL. Failures are logged but never propagated.
func (c *Checker) valkeySetAll(ctx context.Context, rows []FlagRow) {
	if c.vk == nil {
		return
	}

	raw, err := json.Marshal(rows)
	if err != nil {
		slog.Warn("features: failed to marshal flag rows aggregate", "error", err)
		return
	}

	if err := c.vk.Set(ctx, flagCacheKeyPrefix+flagAllCacheKey, string(raw), flagCacheTTL).Err(); err != nil {
		slog.Warn("features: failed to write valkey aggregate cache", "error", err)
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

			if fc.all != nil && now.Sub(fc.all.createdAt) > maxAge {
				fc.all = nil
			}
			fc.mu.Unlock()
		}
	}
}

// GetAllEnabled returns a map of all feature keys to their enabled state
// for the given platform. Used by the frontend layout to get all flags at once.
// Fetches all platform flag rows in a single DB query instead of issuing one
// query per registry key (eliminates the N+1 pattern on cold cache).
func (c *Checker) GetAllEnabled(ctx context.Context) map[string]bool {
	result := make(map[string]bool)

	// Start with registry defaults.
	for _, f := range Registry {
		result[f.Key] = f.Default
	}

	// Overlay stored rows via the shared aggregate cache (no per-request
	// Postgres hit). On DB error AllFlagRows returns nil and the defaults
	// above stand — same fallback as the old inline batch query.
	for _, row := range c.AllFlagRows(ctx) {
		result[row.Key] = effectiveEnabled(c.rolloutID(ctx), row.Key, row.Enabled, row.Config)
	}

	return result
}

// SetEnabled updates a feature flag's enabled state in the database,
// preserving any stored config. RETURNING feeds the post-write config
// straight to the change hook — no second read that could hit a lagged
// replica and hand the roadmap sync a stale blob.
func (c *Checker) SetEnabled(ctx context.Context, key string, enabled bool, updatedBy pgtype.UUID) error {
	if c.db == nil {
		return errors.New("features: SetEnabled called with no database")
	}

	var configBytes []byte

	err := c.db.Writer.QueryRow(ctx, `
		INSERT INTO feature_flags (key, enabled, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET enabled = $2, updated_by = $3, updated_at = now()
		RETURNING config
	`, key, enabled, updatedBy).Scan(&configBytes)
	if err != nil {
		return fmt.Errorf("updating feature flag %s: %w", key, err)
	}

	var cfg FlagConfig
	if len(configBytes) > 0 {
		if err := json.Unmarshal(configBytes, &cfg); err != nil {
			slog.Warn("features: post-write config failed to decode", "key", key, "error", err)
		}
	}

	c.InvalidateFlag(ctx, key)
	c.fireConfigChanged(ctx, key, cfg)

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
	// Phase is the rollout phase (one of Phases). Empty/absent is treated as
	// "shipped" for enabled flags and "planned" for disabled ones at eval.
	Phase string `json:"phase"`
	// ShowOnRoadmap opts the flag into the public roadmap projection; the
	// roadmap sync hook derives the linked item's status from Phase.
	ShowOnRoadmap bool `json:"showOnRoadmap"`
	// Surfaces maps a surface name (one of Surfaces) to its visibility rule.
	// A surface absent from the map behaves as {state:"visible"} — everyone,
	// all countries. A nil map means all surfaces visible.
	Surfaces map[string]SurfaceRule `json:"surfaces"`
}

// SurfaceRule is the per-surface visibility, audience, and geo policy for a
// flag, stored inside FlagConfig.Surfaces.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §3.1.
type SurfaceRule struct {
	State    string   `json:"state"`               // visible|hidden|disabled
	Audience []string `json:"audience,omitempty"`  // group:<slug>|perm:<node>; empty = everyone
	GeoAllow []string `json:"geo_allow,omitempty"` // ISO 3166-1 alpha-2, uppercase; empty = all countries
	GeoDeny  []string `json:"geo_deny,omitempty"`  // ISO alpha-2 denied even when also in GeoAllow
}

// Phases enumerates the rollout phases a flag's config may carry. Phases are
// operational metadata — transitions are free (no state machine) and the
// write path audit-logs every change.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §4.1.
var Phases = []string{"planned", "in_progress", "testing", "shipped", "archived"}

// Surfaces enumerates the client surfaces a flag may carry per-surface rules
// for. "extension" is the browser-extension surface, not the control-panel
// extensions catalog.
var Surfaces = []string{"main", "tools", "academy", "docs", "mobile", "admin", "extension", "desktop"}

// ValidPhase reports whether p is a known rollout phase.
func ValidPhase(p string) bool {
	for _, phase := range Phases {
		if p == phase {
			return true
		}
	}

	return false
}

// ValidSurface reports whether s is a known surface name.
func ValidSurface(s string) bool {
	for _, surface := range Surfaces {
		if s == surface {
			return true
		}
	}

	return false
}

// ConfigError is a FlagConfig validation failure. Code is the
// machine-readable wire code emitted in the error envelope; Message is the
// human-readable detail. It matches errpkg.ErrInvalid via Is so generic
// handling still maps it to 400 when the specific code isn't used.
type ConfigError struct {
	Code    string
	Message string
}

// Error returns "code: message" so the wire code stays visible in logs and
// error strings.
func (e *ConfigError) Error() string {
	return e.Code + ": " + e.Message
}

// Is matches errpkg.ErrInvalid so errors.Is(err, errpkg.ErrInvalid) checks —
// including HandleServiceError's generic 400 mapping — keep working for
// callers that don't extract the specific code.
func (e *ConfigError) Is(target error) bool {
	return target == errpkg.ErrInvalid
}

// ValidateConfig checks a FlagConfig against the unified-rollout rules:
// phase vocabulary, known surface keys, per-surface state vocabulary,
// audience entry format, and uppercase ISO alpha-2 geo codes. An empty Phase
// is allowed (eval derives it from the enabled state). A surface key present
// in Surfaces must carry an explicit state. A country code appearing in both
// GeoAllow and GeoDeny is valid — deny wins at evaluation. Failures return a
// typed *ConfigError whose Code (invalid_phase, invalid_surface,
// invalid_surface_state, invalid_audience, invalid_geo) is the
// machine-readable wire code; the error still matches errpkg.ErrInvalid via
// errors.Is for generic handling. HTTP write paths should respond via
// WriteConfigError so the specific code reaches the error envelope.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §3.1.
func ValidateConfig(cfg FlagConfig) error {
	if cfg.Phase != "" && !ValidPhase(cfg.Phase) {
		return &ConfigError{
			Code:    "invalid_phase",
			Message: fmt.Sprintf("phase %q is not a valid rollout phase (one of: %s)", cfg.Phase, strings.Join(Phases, ", ")),
		}
	}

	for surface, rule := range cfg.Surfaces {
		if !ValidSurface(surface) {
			return &ConfigError{
				Code:    "invalid_surface",
				Message: fmt.Sprintf("surface %q is not a known client surface (one of: %s)", surface, strings.Join(Surfaces, ", ")),
			}
		}

		switch rule.State {
		case "visible", "hidden", "disabled":
		default:
			return &ConfigError{
				Code:    "invalid_surface_state",
				Message: fmt.Sprintf("state %q for surface %q is not a valid surface state (visible|hidden|disabled)", rule.State, surface),
			}
		}

		for _, entry := range rule.Audience {
			if !strings.HasPrefix(entry, "group:") && !strings.HasPrefix(entry, "perm:") {
				return &ConfigError{
					Code:    "invalid_audience",
					Message: fmt.Sprintf("audience entry %q for surface %q must be group:<slug> or perm:<node>", entry, surface),
				}
			}
		}

		for _, code := range rule.GeoAllow {
			if !isISOAlpha2(code) {
				return &ConfigError{
					Code:    "invalid_geo",
					Message: fmt.Sprintf("geo_allow code %q for surface %q must be an uppercase ISO alpha-2 country code", code, surface),
				}
			}
		}

		for _, code := range rule.GeoDeny {
			if !isISOAlpha2(code) {
				return &ConfigError{
					Code:    "invalid_geo",
					Message: fmt.Sprintf("geo_deny code %q for surface %q must be an uppercase ISO alpha-2 country code", code, surface),
				}
			}
		}
	}

	return nil
}

// WriteConfigError writes a validation failure using the ConfigError's
// machine-readable code instead of the generic validation_error code.
// Non-ConfigError values fall back to errpkg.HandleServiceError.
func WriteConfigError(w http.ResponseWriter, err error) {
	var ce *ConfigError
	if errors.As(err, &ce) {
		errpkg.WriteError(w, http.StatusBadRequest, ce.Code, ce.Message)
		return
	}

	errpkg.HandleServiceError(w, err)
}

// flagKeyRe enforces the flag-key vocabulary: lowercase segments joined by
// dots — {domain}.{feature} or feature.{name} (feature-flag-naming.md).
// Rejects junk keys (uppercase, spaces, empty segments) before they can
// become permanent "Custom" extensions on the control panel catalog.
var flagKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

// ValidFlagKey reports whether key is a well-formed feature flag key.
func ValidFlagKey(key string) bool {
	return len(key) <= 120 && flagKeyRe.MatchString(key)
}

// isISOAlpha2 reports whether code is exactly two uppercase ASCII letters —
// the stored form for ISO 3166-1 alpha-2 country codes. Lowercase codes are
// rejected rather than normalized so malformed input never silently persists.
func isISOAlpha2(code string) bool {
	if len(code) != 2 {
		return false
	}

	return code[0] >= 'A' && code[0] <= 'Z' && code[1] >= 'A' && code[1] <= 'Z'
}

// PatchFlag upserts a feature flag row by atomically merging a JSON-object
// patch into its config inside the INSERT/ON CONFLICT — no read-modify-write
// window, so concurrent writes can't lose each other's fields. patch keys are
// FlagConfig field names; top-level keys replace wholesale (surfaces is a
// whole-map replace, matching the PATCH contract). enabled == nil leaves the
// enabled column unchanged; on insert the row starts at the registry Default
// for the key. Returns the persisted (enabled, config) so callers can audit
// and respond with post-merge state rather than what they intended to write.
// Callers validate the patch fields with ValidateConfig first — invalid
// configs return a *ConfigError mapped to 400 via WriteConfigError.
func (c *Checker) PatchFlag(ctx context.Context, key string, enabled *bool, patch []byte, updatedBy pgtype.UUID) (bool, FlagConfig, error) {
	if c.db == nil {
		return false, FlagConfig{}, errors.New("features: PatchFlag called with no database")
	}

	if len(patch) == 0 {
		patch = []byte("{}")
	}

	var enabledParam pgtype.Bool
	if enabled != nil {
		enabledParam = pgtype.Bool{Bool: *enabled, Valid: true}
	}

	var rowEnabled bool

	var configBytes []byte

	err := c.db.Writer.QueryRow(ctx, `
		INSERT INTO feature_flags (key, enabled, config, updated_by)
		VALUES ($1, COALESCE($2, $5), $3::jsonb, $4)
		ON CONFLICT (key) DO UPDATE SET
			enabled    = COALESCE($2, feature_flags.enabled),
			config     = COALESCE(feature_flags.config, '{}'::jsonb) || $3::jsonb,
			updated_by = $4,
			updated_at = now()
		RETURNING enabled, config
	`, key, enabledParam, string(patch), updatedBy, DefaultFor(key)).Scan(&rowEnabled, &configBytes)
	if err != nil {
		return false, FlagConfig{}, fmt.Errorf("patching feature flag %s: %w", key, err)
	}

	var cfg FlagConfig
	if len(configBytes) > 0 {
		if err := json.Unmarshal(configBytes, &cfg); err != nil {
			slog.Warn("features: patched config failed to decode", "key", key, "error", err)
		}
	}

	c.InvalidateFlag(ctx, key)
	c.fireConfigChanged(ctx, key, cfg)

	return rowEnabled, cfg, nil
}

// DeleteFlag removes a feature flag row. After deletion the
// flag falls back to its registry Default, exactly as if it had never been
// set. The registry key itself is never removed.
func (c *Checker) DeleteFlag(ctx context.Context, key string) error {
	if c.db == nil {
		return errors.New("features: DeleteFlag called with no database")
	}

	_, err := c.db.Writer.Exec(ctx, `
		DELETE FROM feature_flags
		WHERE key = $1
	`, key)
	if err != nil {
		return fmt.Errorf("deleting feature flag %s: %w", key, err)
	}

	c.InvalidateFlag(ctx, key)
	// Empty config signals "flag row gone" — the roadmap sync unlinks the
	// flag's item (clears feature_flag_key, returns it to manual lifecycle)
	// per spec §4.3.
	c.fireConfigChanged(ctx, key, FlagConfig{})

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

// FlagRow is a raw feature_flags table row for a platform. JSON tags are the
// wire form for the jc:flags:_all aggregate snapshot (Config is the raw
// jsonb blob, base64-encoded by encoding/json).
type FlagRow struct {
	Key       string    `json:"key"`
	Enabled   bool      `json:"enabled"`
	Config    []byte    `json:"config,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetFlagConfig returns the stored FlagConfig for a feature flag, or a
// zero-value FlagConfig when the flag has no row or no config blob.
func (c *Checker) GetFlagConfig(ctx context.Context, key string) (FlagConfig, error) {
	if c.db == nil {
		return FlagConfig{}, nil
	}

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
	// Routes through PatchFlag so the percentage merge runs inside the
	// upsert — a read-modify-write here would let a concurrent PATCH's
	// fields get clobbered. PatchFlag's INSERT path already seeds enabled
	// from the registry Default (a rollout write never enables a flag).
	patch := fmt.Sprintf(`{"rolloutPercentage":%d}`, percentage)

	_, _, err := c.PatchFlag(ctx, key, nil, []byte(patch), updatedBy)

	return err
}

// Gate is HTTP middleware that checks if a feature is enabled for the
// caller. It evaluates the caller's full scope — X-JC-Surface header, geo
// context, and resolved audience — via EvalFlag, so a flag disabled on the
// caller's surface, denied by geo rules, or outside the caller's audience
// reads as disabled. If disabled, returns 404 with
// {"error":{"code":"feature_disabled","message":"..."}} (ADR-042 — 404, not
// 403, so a disabled feature's existence is not revealed).
// This MUST be used inside the platform middleware group so platform
// context is available.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §5.4.
func Gate(checker *Checker, key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sc := checker.BuildScope(r, SurfaceFrom(r))
			if !checker.EvalFlag(r.Context(), key, sc).Enabled {
				errpkg.WriteError(w, http.StatusNotFound, "feature_disabled", "This feature is not available")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GateAny is HTTP middleware that passes when ANY of the given feature
// keys is enabled for the caller — the composite gate for unified routes
// that serve more than one feature (e.g. the §111/000824 services +
// mentorship surfaces, reachable while service_directory.enabled OR
// mentorship.enabled is on so mentorship-only deployments keep their
// unified inbox/listing routes). Each key is evaluated with the caller's
// full scope exactly like Gate; only when EVERY key is disabled does the
// middleware return the 404 feature_disabled contract (ADR-042). An empty
// keys list always 404s.
// This MUST be used inside the platform middleware group so platform
// context is available.
// Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §5.4.
func GateAny(checker *Checker, keys ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sc := checker.BuildScope(r, SurfaceFrom(r))
			for _, key := range keys {
				if checker.EvalFlag(r.Context(), key, sc).Enabled {
					next.ServeHTTP(w, r)
					return
				}
			}

			errpkg.WriteError(w, http.StatusNotFound, "feature_disabled", "This feature is not available")
		})
	}
}
