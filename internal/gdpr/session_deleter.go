package gdpr

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/valkey-io/valkey-go/valkeycompat"
)

// ValkeySessionDeleter implements SessionDeleter by deleting all user-scoped
// Valkey keys for a given user ID. It deletes known fixed-pattern keys
// directly and uses SCAN for wildcard-pattern keys.
type ValkeySessionDeleter struct {
	rdb valkeycompat.Cmdable
}

// NewValkeySessionDeleter creates a SessionDeleter backed by the Valkey client.
func NewValkeySessionDeleter(rdb valkeycompat.Cmdable) *ValkeySessionDeleter {
	return &ValkeySessionDeleter{rdb: rdb}
}

// DeleteSessions removes all Valkey keys scoped to the given user ID.
func (d *ValkeySessionDeleter) DeleteSessions(ctx context.Context, userID string) error {
	if d == nil || d.rdb == nil {
		return nil
	}

	// Known per-user key patterns — delete directly.
	patterns := []string{
		"jc:perm:" + userID,
		"jc:ban:" + userID,
		"jc:impersonate:" + userID,
	}
	for _, key := range patterns {
		if err := d.rdb.Del(ctx, key).Err(); err != nil {
			slog.Warn("gdpr: failed to delete valkey key during session cleanup",
				"key", key, "error", err)
		}
	}

	// Bruteforce keys are keyed by LoginIdentifier(email, ip), not userID, so
	// a userID-prefixed Del is a no-op. Bruteforce keys have short TTLs (~15 min),
	// so they expire naturally. 

	// Wildcard patterns requiring SCAN — space unread counters and rate limits.
	wildcardPatterns := []string{
		"jc:space_unread:" + userID + ":*",
	}
	for _, pattern := range wildcardPatterns {
		if err := d.scanAndDelete(ctx, pattern); err != nil {
			slog.Warn("gdpr: failed to scan+delete valkey keys during session cleanup",
				"pattern", pattern, "error", err)
		}
	}

	// Editor lock keys are keyed by {content_type}:{content_id} with the user
	// as the *value*, not part of the key name. SCAN for all locks and delete
	// those held by this user. 
	if err := d.scanAndDeleteByValue(ctx, "jc:editor_lock:*", userID); err != nil {
		slog.Warn("gdpr: failed to delete editor lock keys during session cleanup",
			"user_id", userID, "error", err)
	}

	return nil
}

// scanAndDeleteByValue scans keys matching the pattern, reads their values,
// and deletes those whose value equals the given userID.
func (d *ValkeySessionDeleter) scanAndDeleteByValue(ctx context.Context, pattern, userID string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := d.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scanning valkey keys (%s): %w", pattern, err)
		}
		for _, key := range keys {
			val, err := d.rdb.Get(ctx, key).Result()
			if err != nil {
				continue
			}
			if val == userID {
				d.rdb.Del(ctx, key)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// scanAndDelete performs SCAN with a pattern, then batches DEL on matched keys.
func (d *ValkeySessionDeleter) scanAndDelete(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := d.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scanning valkey keys (%s): %w", pattern, err)
		}
		if len(keys) > 0 {
			if err := d.rdb.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("deleting valkey keys (%s): %w", pattern, err)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
