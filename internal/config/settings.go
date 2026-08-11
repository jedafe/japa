package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/valkey-io/valkey-go/valkeycompat"
	"golang.org/x/sync/singleflight"
)

const settingsCacheTTL = 5 * time.Minute

const settingsCachePrefix = "jc:settings:"

type Settings struct {
	db      *db.DB
	q       *gen.Queries
	qWriter *gen.Queries
	vk      valkeycompat.Cmdable
	mu      sync.RWMutex
	cache   map[string]cachedSetting
	stopCh  chan struct{}
	sf      singleflight.Group
}

type cachedSetting struct {
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

func NewSettings(database *db.DB, vkClient valkeycompat.Cmdable) *Settings {
	s := &Settings{
		db:      database,
		q:       db.NewQueries(database.Reader),
		qWriter: db.NewQueries(database.Writer),
		vk:      vkClient,
		cache:   make(map[string]cachedSetting),
		stopCh:  make(chan struct{}),
	}
	go s.cleanupLoop(60 * time.Second)
	return s
}

// Close stops the background cleanup goroutine. Callers should call this
// during application shutdown to avoid leaking the cleanup goroutine. It is
// safe to call multiple times — subsequent calls are no-ops. It is also safe
// to call on a Settings instance created without NewSettings (stopCh == nil).
func (s *Settings) Close() {
	if s.stopCh == nil {
		return
	}
	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
}

func (s *Settings) GetSetting(ctx context.Context, tenantID pgtype.UUID, key string) (string, error) {
	cacheKey := tenantID.String() + ":" + key

	s.mu.RLock()
	if cached, ok := s.cache[cacheKey]; ok && time.Since(cached.CreatedAt) < settingsCacheTTL {
		s.mu.RUnlock()
		return cached.Value, nil
	}
	s.mu.RUnlock()

	if s.vk != nil {
		data, err := s.vk.Get(ctx, settingsCachePrefix+cacheKey).Bytes()
		if err == nil {
			var entry cachedSetting
			if json.Unmarshal(data, &entry) == nil {
				s.mu.Lock()
				s.cache[cacheKey] = entry
				s.mu.Unlock()
				return entry.Value, nil
			}
		}
	}

	// Coalesce concurrent misses for the same cache key so only one goroutine
	// hits the database (BUG-006 — cache stampede prevention).
	v, err, _ := s.sf.Do(cacheKey, func() (any, error) {
		// Re-check L1 under singleflight — a concurrent call may have already
		// populated the cache while we were waiting.
		s.mu.RLock()
		if cached, ok := s.cache[cacheKey]; ok && time.Since(cached.CreatedAt) < settingsCacheTTL {
			s.mu.RUnlock()
			return cached.Value, nil
		}
		s.mu.RUnlock()

		valueBytes, err := s.q.GetSettingValue(ctx, gen.GetSettingValueParams{
			TenantID: tenantID,
			Key:      key,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				slog.Debug("settings: setting not stored, using default", "key", key)
			} else {
				slog.Warn("settings: DB read failed", "key", key, "error", err)
			}
			return "", fmt.Errorf("setting %q not found: %w", key, err)
		}

		value := string(valueBytes)
		entry := cachedSetting{Value: value, CreatedAt: time.Now()}
		s.mu.Lock()
		s.cache[cacheKey] = entry
		s.mu.Unlock()

		if s.vk != nil {
			data, _ := json.Marshal(entry)
			_ = s.vk.Set(ctx, settingsCachePrefix+cacheKey, data, settingsCacheTTL).Err()
		}

		return value, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (s *Settings) GetSettingBool(ctx context.Context, tenantID pgtype.UUID, key string) (bool, error) {
	val, err := s.GetSetting(ctx, tenantID, key)
	if err != nil {
		return false, err
	}
	return parseBool(val), nil
}

// parseBool decodes a JSON-encoded system_setting value into a bool.
// Accepts JSON bool (true), JSON string ("true"/"1"), or bare token.
func parseBool(sv string) bool {
	sv = strings.TrimSpace(sv)
	var b bool
	if json.Unmarshal([]byte(sv), &b) == nil {
		return b
	}
	var str string
	if json.Unmarshal([]byte(sv), &str) == nil {
		str = strings.TrimSpace(str)
		return str == "true" || str == "1"
	}
	return sv == "true" || sv == "1"
}

func (s *Settings) GetSettingInt(ctx context.Context, tenantID pgtype.UUID, key string) (int, error) {
	val, err := s.GetSetting(ctx, tenantID, key)
	if err != nil {
		return 0, err
	}
	return parseInt(val)
}

// parseInt decodes a JSON-encoded system_setting value into an int.
// Accepts JSON number (5000), JSON string ("5000"), or bare token.
func parseInt(sv string) (int, error) {
	sv = strings.TrimSpace(sv)
	var n int
	if json.Unmarshal([]byte(sv), &n) == nil {
		return n, nil
	}
	var str string
	if json.Unmarshal([]byte(sv), &str) == nil {
		if m, convErr := strconv.Atoi(strings.TrimSpace(str)); convErr == nil {
			return m, nil
		}
	}
	if m, convErr := strconv.Atoi(sv); convErr == nil {
		return m, nil
	}
	return 0, fmt.Errorf("value is not an integer: %q", sv)
}

func (s *Settings) InvalidateSettings(ctx context.Context, tenantID pgtype.UUID) {
	tenantPrefix := tenantID.String() + ":"

	s.mu.Lock()
	for k := range s.cache {
		if strings.HasPrefix(k, tenantPrefix) {
			delete(s.cache, k)
		}
	}
	s.mu.Unlock()

	prefix := settingsCachePrefix + tenantID.String() + ":"

	if s.vk != nil {
		var cursor uint64
		for {
			keys, nextCursor, err := s.vk.Scan(ctx, cursor, prefix+"*", 100).Result()
			if err != nil {
				slog.Warn("settings: failed to scan valkey for invalidation", "error", err)
				return
			}
			if len(keys) > 0 {
				_ = s.vk.Del(ctx, keys...).Err()
			}
			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}
}

// GetIntOr returns the integer value for key, or fallback when the key is
// unset, missing, or not parseable as an integer. It never returns an error,
// so call sites read a hot-reloadable operational limit with a safe default
// and unchanged behaviour when the setting has not been configured.
func (s *Settings) GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int {
	v, err := s.GetSettingInt(ctx, tenantID, key)
	if err != nil {
		return fallback
	}
	return v
}

// GetStringOr returns the string value for key, or fallback when the key is
// unset or missing. It never returns an error. The value is unquoted if it
// is a JSON-encoded string (e.g. "\"https://...\"" → "https://...").
func (s *Settings) GetStringOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback string) string {
	val, err := s.GetSetting(ctx, tenantID, key)
	if err != nil || val == "" {
		return fallback
	}
	// system_settings values are stored as JSON-encoded strings —
	// unquote if the raw value is a JSON string (starts and ends with `"`).
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		var unquoted string
		if json.Unmarshal([]byte(val), &unquoted) == nil {
			return unquoted
		}
	}
	return val
}

// GetFloatOr returns the float64 value for key, or fallback when the key is
// unset, missing, or not parseable as a float. It never returns an error.
func (s *Settings) GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64 {
	val, err := s.GetSetting(ctx, tenantID, key)
	if err != nil {
		return fallback
	}
	f, err := parseFloat(val)
	if err != nil {
		return fallback
	}
	return f
}

// parseFloat decodes a JSON-encoded system_setting value into a float64.
// Accepts JSON number (1.8), JSON string ("1.8"), or bare token.
func parseFloat(sv string) (float64, error) {
	sv = strings.TrimSpace(sv)
	var f float64
	if json.Unmarshal([]byte(sv), &f) == nil {
		return f, nil
	}
	var str string
	if json.Unmarshal([]byte(sv), &str) == nil {
		if v, convErr := strconv.ParseFloat(strings.TrimSpace(str), 64); convErr == nil {
			return v, nil
		}
	}
	if v, convErr := strconv.ParseFloat(sv, 64); convErr == nil {
		return v, nil
	}
	return 0, fmt.Errorf("value is not a float: %q", sv)
}

// GetBoolOr returns the boolean value for key, or fallback when the key is
// unset or missing. It never returns an error.
func (s *Settings) GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool {
	v, err := s.GetSettingBool(ctx, tenantID, key)
	if err != nil {
		return fallback
	}
	return v
}

// SetSetting upserts a setting value for the given tenant and key.
// The is_public flag is preserved on conflict (not reset to false).
func (s *Settings) SetSetting(ctx context.Context, tenantID pgtype.UUID, key, value string) error {
	if err := s.qWriter.UpsertSystemSettingPreservePublic(ctx, gen.UpsertSystemSettingPreservePublicParams{
		TenantID: tenantID,
		Key:      key,
		Value:    []byte(value),
	}); err != nil {
		return fmt.Errorf("upserting setting %q: %w", key, err)
	}

	s.InvalidateSettings(ctx, tenantID)
	return nil
}

// UpdateSettingsBatch atomically upserts multiple settings for one tenant in a
// single transaction. Each entry in settings maps a full dot-notation key
// (e.g. "site.name") to its already-serialized JSON value. The cache is
// invalidated once after commit so callers do not need to invalidate per key.
// is_public is preserved on conflict (not overwritten).
func (s *Settings) UpdateSettingsBatch(ctx context.Context, tenantID pgtype.UUID, settings map[string][]byte) error {
	err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		for key, valueJSON := range settings {
			if err := q.UpsertSystemSettingPreservePublic(ctx, gen.UpsertSystemSettingPreservePublicParams{
				TenantID: tenantID,
				Key:      key,
				Value:    valueJSON,
			}); err != nil {
				return fmt.Errorf("upserting setting %q: %w", key, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.InvalidateSettings(ctx, tenantID)
	return nil
}

// UpdateSettingsBatchWithPublic is like UpdateSettingsBatch but also sets the
// is_public flag on each upserted row (used by the admin handler so appearance
// settings are exposed via GET /site/settings/public). On conflict, is_public
// is updated to the new value so toggling publicity takes effect immediately.
func (s *Settings) UpdateSettingsBatchWithPublic(ctx context.Context, tenantID pgtype.UUID, settings map[string][]byte, isPublic map[string]bool) error {
	err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		for key, valueJSON := range settings {
			pub := isPublic[key]
			if err := q.UpsertSystemSetting(ctx, gen.UpsertSystemSettingParams{
				TenantID: tenantID,
				Key:      key,
				Value:    valueJSON,
				IsPublic: pub,
			}); err != nil {
				return fmt.Errorf("upserting setting %q: %w", key, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.InvalidateSettings(ctx, tenantID)
	return nil
}

// GetPublicSettings returns all settings marked as public for the given tenant.
func (s *Settings) GetPublicSettings(ctx context.Context, tenantID pgtype.UUID) (map[string]any, error) {
	rows, err := s.q.GetPublicSettings(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying public settings: %w", err)
	}

	settings := make(map[string]any, len(rows))
	for _, row := range rows {
		var parsed any
		if json.Unmarshal(row.Value, &parsed) == nil {
			settings[row.Key] = parsed
		} else {
			settings[row.Key] = string(row.Value)
		}
	}

	return settings, nil
}

// cleanupLoop periodically evicts expired L1 cache entries to prevent memory leaks.
// It exits when Close() is called so the goroutine does not leak.
func (s *Settings) cleanupLoop(maxAge time.Duration) {
	ticker := time.NewTicker(maxAge)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for key, entry := range s.cache {
				if now.Sub(entry.CreatedAt) > settingsCacheTTL {
					delete(s.cache, key)
				}
			}
			s.mu.Unlock()
		}
	}
}
