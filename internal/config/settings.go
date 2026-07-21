package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

const settingsCacheTTL = 5 * time.Minute

const settingsCachePrefix = "jc:settings:"

type Settings struct {
	db    *db.DB
	vk    valkeycompat.Cmdable
	mu    sync.RWMutex
	cache map[string]cachedSetting
}

type cachedSetting struct {
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

func NewSettings(database *db.DB, vkClient valkeycompat.Cmdable) *Settings {
	s := &Settings{
		db:    database,
		vk:    vkClient,
		cache: make(map[string]cachedSetting),
	}
	go s.cleanupLoop(60 * time.Second)
	return s
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

	var value string
	err := s.db.Reader.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE tenant_id = $1 AND key = $2`,
		tenantID, key,
	).Scan(&value)
	if err != nil {
		slog.Warn("settings: cache miss + DB miss", "key", key, "error", err)
		return "", fmt.Errorf("setting %q not found: %w", key, err)
	}

	entry := cachedSetting{Value: value, CreatedAt: time.Now()}
	s.mu.Lock()
	s.cache[cacheKey] = entry
	s.mu.Unlock()

	if s.vk != nil {
		data, _ := json.Marshal(entry)
		_ = s.vk.Set(ctx, settingsCachePrefix+cacheKey, data, settingsCacheTTL).Err()
	}

	return value, nil
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

// GetBoolOr returns the boolean value for key, or fallback when the key is
// unset or missing. It never returns an error.
func (s *Settings) GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool {
	v, err := s.GetSettingBool(ctx, tenantID, key)
	if err != nil {
		return fallback
	}
	return v
}

// GetPublicSettings returns all settings marked as public for the given tenant.
func (s *Settings) GetPublicSettings(ctx context.Context, tenantID pgtype.UUID) (map[string]any, error) {
	rows, err := s.db.Reader.Query(ctx,
		`SELECT key, value FROM system_settings WHERE tenant_id = $1 AND is_public = true`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying public settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]any)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scanning public setting: %w", err)
		}
		var parsed any
		if json.Unmarshal([]byte(value), &parsed) == nil {
			settings[key] = parsed
		} else {
			settings[key] = value
		}
	}

	return settings, rows.Err()
}

// cleanupLoop periodically evicts expired L1 cache entries to prevent memory leaks.
func (s *Settings) cleanupLoop(maxAge time.Duration) {
	ticker := time.NewTicker(maxAge)
	defer ticker.Stop()

	for range ticker.C {
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
