package auth

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jedafe/japa/internal/valkey"
)

// bruteForceKeyPrefix namespaces all brute-force counters in Valkey.
const bruteForceKeyPrefix = "jc:bruteforce:"

// ValkeyBruteForceStore implements BruteForceStore backed by Valkey.
// Keys are namespaced with jc:bruteforce: and expire after FailWindow
// so stale counters are automatically cleaned up.
type ValkeyBruteForceStore struct {
	vk *valkey.Client
}

// NewValkeyBruteForceStore creates a Valkey-backed brute-force store.
func NewValkeyBruteForceStore(vk *valkey.Client) *ValkeyBruteForceStore {
	return &ValkeyBruteForceStore{vk: vk}
}

// fullKey namespaces the caller-provided key with the brute-force prefix.
func fullKey(key string) string {
	return bruteForceKeyPrefix + key
}

// GetFailCount returns the current failure count for the given key.
// Returns 0 when the key does not exist (no prior failures).
func (s *ValkeyBruteForceStore) GetFailCount(ctx context.Context, key string) (int, error) {
	val, err := s.vk.Redis().Get(ctx, fullKey(key)).Result()
	if err != nil {
		// A missing key is not an error — it means zero prior failures.
		if err.Error() == "redis: nil" {
			return 0, nil
		}
		return 0, fmt.Errorf("bruteforce get count: %w", err)
	}

	count, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("bruteforce parse count %q: %w", val, err)
	}
	return count, nil
}

// IncrFailCount atomically increments the failure counter and sets (or
// refreshes) the TTL to FailWindow. Returns the new count after
// incrementing.
func (s *ValkeyBruteForceStore) IncrFailCount(ctx context.Context, key string) error {
	rdb := s.vk.Redis()
	fk := fullKey(key)

	// Pipeline INCR + EXPIRE for atomicity — both execute in a single
	// round-trip and the EXPIRE resets on every failure so the window
	// slides forward.
	pipe := rdb.Pipeline()
	pipe.Incr(ctx, fk)
	pipe.Expire(ctx, fk, FailWindow)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("bruteforce incr: %w", err)
	}
	return nil
}

// ResetFailCount deletes the failure counter for the given key.
// It is not an error to reset a key that does not exist.
func (s *ValkeyBruteForceStore) ResetFailCount(ctx context.Context, key string) error {
	if err := s.vk.Redis().Del(ctx, fullKey(key)).Err(); err != nil {
		return fmt.Errorf("bruteforce reset: %w", err)
	}
	return nil
}
