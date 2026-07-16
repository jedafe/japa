package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jedafe/japa/internal/valkey"
)

// CacheFlusher provides cache invalidation for platform caches.
// All keys use the jc: namespace prefix. Flush operations are admin-only
// and should be audit-logged by the caller.
type CacheFlusher struct {
	vk *valkey.Client
}

// NewCacheFlusher creates a CacheFlusher backed by the given Valkey client.
func NewCacheFlusher(vk *valkey.Client) *CacheFlusher {
	return &CacheFlusher{vk: vk}
}

// FlushScope identifies which cache partition to flush.
type FlushScope string

const (
	// FlushAll deletes every jc: prefixed key — use with caution.
	FlushAll FlushScope = "all"
)

// FlushCache removes cached keys matching the given scope.
// FlushAll deletes every jc: prefixed key — use with caution.
func (f *CacheFlusher) FlushCache(ctx context.Context, scope FlushScope) (int64, error) {
	if f.vk == nil {
		slog.Warn("cache flush: valkey client unavailable")
		return 0, nil
	}

	pattern := cachePatternForScope(scope)
	if pattern == "" {
		return 0, fmt.Errorf("unknown cache scope: %s", scope)
	}

	rdb := f.vk.Redis()

	var cursor uint64

	var totalDeleted int64

	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return totalDeleted, fmt.Errorf("scanning cache keys: %w", err)
		}

		if len(keys) > 0 {
			deleted, err := rdb.Del(ctx, keys...).Result()
			if err != nil {
				return totalDeleted, fmt.Errorf("deleting cache keys: %w", err)
			}

			totalDeleted += deleted
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	slog.Info("cache flush completed", "scope", scope, "deleted", totalDeleted)

	return totalDeleted, nil
}

// FlushCacheByPrefix removes all keys matching a specific prefix under jc:.
func (f *CacheFlusher) FlushCacheByPrefix(ctx context.Context, prefix string) (int64, error) {
	if f.vk == nil {
		return 0, nil
	}

	if !strings.HasPrefix(prefix, "jc:") {
		prefix = "jc:" + prefix
	}

	return f.flushByPattern(ctx, prefix+"*")
}

func (f *CacheFlusher) flushByPattern(ctx context.Context, pattern string) (int64, error) {
	rdb := f.vk.Redis()

	var cursor uint64

	var totalDeleted int64

	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return totalDeleted, fmt.Errorf("scanning cache keys: %w", err)
		}

		if len(keys) > 0 {
			deleted, err := rdb.Del(ctx, keys...).Result()
			if err != nil {
				return totalDeleted, fmt.Errorf("deleting cache keys: %w", err)
			}

			totalDeleted += deleted
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return totalDeleted, nil
}

// cachePatternForScope maps a flush scope to the Valkey key pattern.
func cachePatternForScope(scope FlushScope) string {
	switch scope {
	case FlushAll:
		return "jc:*"
	default:
		return ""
	}
}
