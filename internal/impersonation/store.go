package impersonation

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

// Key prefix for impersonation state in Valkey.
const keyPrefix = "jc:impersonate:"

// DefaultImpersonationTTL is the duration an impersonation session lasts
// before automatically expiring. The 30-minute window is deliberately
// short to limit the blast radius of accidental or malicious use.
const DefaultImpersonationTTL = 30 * time.Minute

// Store provides Valkey-backed impersonation state.
type Store struct {
	rdb *redis.Client
}

// NewStore creates a Store backed by the given Valkey client.
func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

// SetImpersonation stores the impersonation target for an admin.
// The key auto-expires after the given TTL.
func (s *Store) SetImpersonation(ctx context.Context, adminID pgtype.UUID, targetID pgtype.UUID, ttl time.Duration) error {
	if s.rdb == nil {
		return nil // graceful degradation
	}
	key := keyPrefix + adminID.String()
	return s.rdb.Set(ctx, key, targetID.String(), ttl).Err()
}

// GetImpersonation retrieves the impersonated user ID for an admin.
// Returns empty string and nil error if no active impersonation.
func (s *Store) GetImpersonation(ctx context.Context, adminID pgtype.UUID) (string, error) {
	if s.rdb == nil {
		return "", nil
	}
	key := keyPrefix + adminID.String()
	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get impersonation: %w", err)
	}
	return val, nil
}

// ClearImpersonation removes the impersonation state for an admin.
func (s *Store) ClearImpersonation(ctx context.Context, adminID pgtype.UUID) error {
	if s.rdb == nil {
		return nil
	}
	key := keyPrefix + adminID.String()
	return s.rdb.Del(ctx, key).Err()
}
