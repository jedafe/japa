// Package interactions implements user-facing block and mute handlers
// for the social graph (Sections 41, 52).
package interactions

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// muteCacheTTL is the TTL for mute SET caches in Valkey (Section 52).
const muteCacheTTL = 10 * time.Minute

// Service provides block/mute operations with reader/writer pool separation.
type Service struct {
	db    *db.DB
	read  *gen.Queries
	write *gen.Queries
	vk    valkeycompat.Cmdable // nil disables caching (tests/development)
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		db:    database,
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// SetValkey wires the Valkey client for mute SET caching (Section 52 /
// VALKEY-001). Keys: jc:muted_users:{}:{user_id},
// jc:muted_spaces:{}:{user_id}.
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// mutedUsersKey returns the tenant-scoped Valkey key for a user's muted-users
// SET (BUG-003: previously jc:muted_users:{user_id} without tenant scope).
func mutedUsersKey(tenantID, userID pgtype.UUID) string {
	return valkey.MustKey("muted_users:%s:%s", tenantID.String(), userID.String())
}

// mutedSpacesKey is owned by internal/spaces (space mute actions are
// slug-resolved there). This package only caches muted-*user* SETs.

// invalidateMutedUsersCache deletes the muted-users SET cache for the user.
// Called after mute/unmute so the next read refetches from the DB.
func (s *Service) invalidateMutedUsersCache(ctx context.Context, tenantID, userID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	if err := s.vk.Del(ctx, mutedUsersKey(tenantID, userID)).Err(); err != nil {
		// Cache invalidation failure is non-fatal — the TTL will eventually
		// refresh; worst case is a stale mute list for up to 10 minutes.
		return
	}
}
