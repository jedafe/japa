package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	return val == "true" || val == "1", nil
}

func (s *Settings) GetSettingInt(ctx context.Context, tenantID pgtype.UUID, key string) (int, error) {
	val, err := s.GetSetting(ctx, tenantID, key)
	if err != nil {
		return 0, err
	}
	var n int
	if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
		return 0, fmt.Errorf("setting %q is not an integer: %w", key, err)
	}
	return n, nil
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
