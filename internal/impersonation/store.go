package impersonation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Key prefix for impersonation state in Valkey.
const keyPrefix = "jc:impersonate:"

// DefaultImpersonationTTL is the duration an impersonation session lasts
// before automatically expiring. The 30-minute window is deliberately
// short to limit the blast radius of accidental or malicious use.
const DefaultImpersonationTTL = 30 * time.Minute

// Store provides Valkey-backed impersonation state.
type Store struct {
	rdb valkeycompat.Cmdable
}

// NewStore creates a Store backed by the given Valkey client.
func NewStore(rdb valkeycompat.Cmdable) *Store {
	return &Store{rdb: rdb}
}

// SetImpersonation stores the impersonation target for an admin.
// The key auto-expires after the given TTL.
func (s *Store) SetImpersonation(ctx context.Context, adminID, targetID pgtype.UUID, ttl time.Duration) error {
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
	if errors.Is(err, valkeycompat.Nil) {
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

// GetDelImpersonation atomically retrieves and removes the impersonation
// target for an admin. Returns empty string and nil error if no active
// impersonation. Used by EndImpersonation so the target_id is captured
// for the audit log before the key is removed (Section 46, Rule 10).
func (s *Store) GetDelImpersonation(ctx context.Context, adminID pgtype.UUID) (string, error) {
	if s.rdb == nil {
		return "", nil
	}

	key := keyPrefix + adminID.String()

	val, err := s.rdb.GetDel(ctx, key).Result()
	if errors.Is(err, valkeycompat.Nil) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("getdel impersonation: %w", err)
	}

	return val, nil
}

// TTLImpersonation returns the remaining TTL in seconds for an admin's
// impersonation session. Returns 0 if no active session or rdb is nil.
// Used by GetImpersonationStatus to compute expires_at (Section 46).
func (s *Store) TTLImpersonation(ctx context.Context, adminID pgtype.UUID) (int64, error) {
	if s.rdb == nil {
		return 0, nil
	}

	key := keyPrefix + adminID.String()

	ttl, err := s.rdb.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("ttl impersonation: %w", err)
	}

	return int64(ttl.Seconds()), nil
}
