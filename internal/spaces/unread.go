package spaces

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// UnreadCounter manages per-user per-space unread post counts using Valkey.
// Key format: jc:space_unread:{}:{user_id}:{space_id} -> integer
// Counters are incremented on new post fan-out and reset to 0 on visit.
// The segment ensures no cross-tenant collision if user IDs are
// not globally unique (Rule 4, §4.2).
type UnreadCounter struct {
	rdb valkeycompat.Cmdable
}

// NewUnreadCounter creates an UnreadCounter backed by the given Valkey client.
// If the client is nil, all operations are no-ops (graceful degradation).
func NewUnreadCounter(vk *valkey.Client) *UnreadCounter {
	if vk == nil {
		return &UnreadCounter{rdb: nil}
	}

	return &UnreadCounter{rdb: vk.Redis()}
}

// unreadKey builds the Valkey key for a tenant/user/space unread counter pair.
func unreadKey(userID, spaceID pgtype.UUID) string {
	return valkey.MustKey("space_unread::%s:%s", userID.String(), spaceID.String())
}

// Increment increments the unread count for a user in a space.
// It silently fails if Valkey is unreachable — unread counts are a
// nice-to-have, not critical path.
func (c *UnreadCounter) Increment(ctx context.Context, userID, spaceID pgtype.UUID) error {
	if c.rdb == nil {
		return nil
	}

	key := unreadKey(userID, spaceID)

	if err := c.rdb.Incr(ctx, key).Err(); err != nil {
		return fmt.Errorf("incrementing space unread count: %w", err)
	}

	// Attach a 24-hour TTL so stale counters are evicted automatically.
	if err := c.rdb.Expire(ctx, key, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("setting space unread TTL: %w", err)
	}

	return nil
}

// Reset sets the unread count for a user in a space to 0.
// Called when a user visits a space.
func (c *UnreadCounter) Reset(ctx context.Context, userID, spaceID pgtype.UUID) error {
	if c.rdb == nil {
		return nil
	}

	key := unreadKey(userID, spaceID)

	// Set to 0 with the 24-hour TTL in a single command so the key is
	// never observable without an expiry (avoids the Set+Expire race that
	// could leave a persistent zero key if Expire failed).
	if err := c.rdb.Set(ctx, key, 0, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("resetting space unread count: %w", err)
	}

	return nil
}

// Get returns the unread count for a user in a space.
// Returns 0 on error (including Valkey unreachable).
func (c *UnreadCounter) Get(ctx context.Context, userID, spaceID pgtype.UUID) (int, error) {
	if c.rdb == nil {
		return 0, nil
	}

	n, err := c.rdb.Get(ctx, unreadKey(userID, spaceID)).Int()
	if err != nil {
		// redis.Nil means the key does not exist yet; treat as zero.
		return 0, nil //nolint:nilerr // intentional: missing key = zero unread
	}

	return n, nil
}

// GetBatch returns unread counts for multiple spaces for a user.
// Returns a map of spaceID string -> count. Missing or errored entries are 0.
func (c *UnreadCounter) GetBatch(ctx context.Context, userID pgtype.UUID, spaceIDs []pgtype.UUID) (map[string]int, error) {
	out := make(map[string]int, len(spaceIDs))
	if c.rdb == nil {
		return out, nil
	}

	keys := make([]string, len(spaceIDs))
	for i, sid := range spaceIDs {
		keys[i] = unreadKey(userID, sid)
	}

	vals, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return out, nil //nolint:nilerr // intentional: degradable batch unread
	}

	for i, v := range vals {
		if v == nil {
			continue
		}

		if s, ok := v.(string); ok {
			n, _ := strconv.Atoi(s)
			out[spaceIDs[i].String()] = n
		}
	}

	return out, nil
}
