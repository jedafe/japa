package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/valkey-io/valkey-go/valkeycompat"
)

// bruteForceKeyPrefix namespaces all brute-force counters in Valkey.
const bruteForceKeyPrefix = "jc:bruteforce:"

// ValkeyBruteForceStore implements BruteForceStore backed by Valkey.
// Keys are namespaced with jc:bruteforce: and expire after FailWindow
// so stale counters are automatically cleaned up.
type ValkeyBruteForceStore struct {
	rdb valkeycompat.Cmdable
}

// NewValkeyBruteForceStore creates a Valkey-backed brute-force store.
func NewValkeyBruteForceStore(rdb valkeycompat.Cmdable) *ValkeyBruteForceStore {
	return &ValkeyBruteForceStore{rdb: rdb}
}

// fullKey namespaces the caller-provided key with the brute-force prefix.
func fullKey(key string) string {
	return bruteForceKeyPrefix + key
}

// GetFailCount returns the current failure count for the given key.
// Returns 0 when the key does not exist (no prior failures).
// Returns 0 when Valkey is unreachable (graceful degradation).
func (s *ValkeyBruteForceStore) GetFailCount(ctx context.Context, key string) (int, error) {
	if s.rdb == nil {
		return 0, nil
	}

	val, err := s.rdb.Get(ctx, fullKey(key)).Result()
	if err != nil {
		// A missing key is not an error — it means zero prior failures.
		if errors.Is(err, valkeycompat.Nil) {
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
// incrementing. No-op when Valkey is unreachable (graceful degradation).
func (s *ValkeyBruteForceStore) IncrFailCount(ctx context.Context, key string) error {
	if s.rdb == nil {
		return nil
	}

	fk := fullKey(key)

	// Pipeline INCR + EXPIRE for atomicity — both execute in a single
	// round-trip and the EXPIRE resets on every failure so the window
	// slides forward.
	pipe := s.rdb.Pipeline()
	pipe.Incr(ctx, fk)
	pipe.Expire(ctx, fk, FailWindow)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("bruteforce incr: %w", err)
	}

	return nil
}

// ResetFailCount deletes the failure counter for the given key.
// It is not an error to reset a key that does not exist.
// No-op when Valkey is unreachable (graceful degradation).
func (s *ValkeyBruteForceStore) ResetFailCount(ctx context.Context, key string) error {
	if s.rdb == nil {
		return nil
	}

	if err := s.rdb.Del(ctx, fullKey(key)).Err(); err != nil {
		return fmt.Errorf("bruteforce reset: %w", err)
	}

	return nil
}
