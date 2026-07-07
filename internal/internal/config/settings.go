package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
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
	cache map[string]cachedSetting
}

type cachedSetting struct {
	value     string
	createdAt time.Time
}

func NewSettings(database *db.DB, vkClient valkeycompat.Cmdable) *Settings {
	return &Settings{
		db:    database,
		vk:    vkClient,
		cache: make(map[string]cachedSetting),
	}
}

func (s *Settings) GetSetting(ctx context.Context, tenantID pgtype.UUID, key string) (string, error) {
	cacheKey := tenantID.String() + ":" + key

	if cached, ok := s.cache[cacheKey]; ok && time.Since(cached.createdAt) < settingsCacheTTL {
		return cached.value, nil
	}

	if s.vk != nil {
		data, err := s.vk.Get(ctx, settingsCachePrefix+cacheKey).Bytes()
		if err == nil {
			var entry cachedSetting
			if json.Unmarshal(data, &entry) == nil {
				s.cache[cacheKey] = entry
				return entry.value, nil
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

	entry := cachedSetting{value: value, createdAt: time.Now()}
	s.cache[cacheKey] = entry

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
	// Values are stored JSON-encoded in a JSONB column. Accept a JSON bool
	// (true), a JSON string ("true"/"1"), or a bare token for resilience.
	sv := strings.TrimSpace(val)
	var b bool
	if json.Unmarshal([]byte(sv), &b) == nil {
		return b, nil
	}
	var str string
	if json.Unmarshal([]byte(sv), &str) == nil {
		str = strings.TrimSpace(str)
		return str == "true" || str == "1", nil
	}
	return sv == "true" || sv == "1", nil
}

func (s *Settings) GetSettingInt(ctx context.Context, tenantID pgtype.UUID, key string) (int, error) {
	val, err := s.GetSetting(ctx, tenantID, key)
	if err != nil {
		return 0, err
	}
	// Values are stored JSON-encoded in a JSONB column. Accept a JSON number
	// (5000), a JSON string ("5000"), or a bare integer for resilience.
	sv := strings.TrimSpace(val)
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
	return 0, fmt.Errorf("setting %q is not an integer: %q", key, val)
}

func (s *Settings) InvalidateSettings(ctx context.Context, tenantID pgtype.UUID) {
	prefix := settingsCachePrefix + tenantID.String() + ":"

	for k := range s.cache {
		if len(k) >= len(tenantID.String()) && k[:len(tenantID.String())] == tenantID.String() {
			delete(s.cache, k)
		}
	}

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
