// Package follows implements the universal polymorphic Follow System (Section 31).
// Users can follow other users, Spaces, Tags, Organizations, and Professional
// Profiles. A single follows table handles all entity types. Follower counts
// are denormalized on entity tables and maintained in the service layer.
package follows

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/wallet"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// Entity type constants for the polymorphic follows table.
const (
	EntityTypeUser         = "user"
	EntityTypeSpace        = "space"
	EntityTypeTag          = "tag"
	EntityTypeOrganization = "organization"
	EntityTypeSource       = "source"
)

// DefaultPageSize is the default number of items returned by list queries.
const DefaultPageSize = 20

// MaxPageSize is the maximum number of items a client can request.
const MaxPageSize = 100

// isFollowingCacheTTL is the Valkey TTL for cached is_following lookups
// (Section 31 / L3). Short TTL keeps the cache fresh while cutting DB hits
// on hot profile pages. Ref: conventions/valkey-keys-catalog.md.
const isFollowingCacheTTL = 60 * time.Second

// NotificationDispatcher dispatches a notification for a follow event.
// Implemented by notifications.Service (adapted in server.go). Dispatch is
// fire-and-forget per Rule 8 — the follow operation must not block on
// notification delivery. Ref: japaconnect-engineering/sections/35-notification-system.md.
type NotificationDispatcher interface {
	DispatchFollowNotification(ctx context.Context, tenantID, recipientID, actorID pgtype.UUID, entityType string, entityID pgtype.UUID)
}

// Service provides follow operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	notify   NotificationDispatcher
	vk       valkeycompat.Cmdable // nil disables caching (tests/development)
	activity wallet.ActivityEmitter
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db: database,
	}
}

// SetNotificationDispatcher injects the notification dispatcher used to send
// follow.received notifications (Section 35). Optional — when nil, no
// notification is dispatched (backward-compatible with tests).
func (s *Service) SetNotificationDispatcher(d NotificationDispatcher) {
	s.notify = d
}

// SetValkey wires the Valkey client for is_following caching (Section 31 /
// L3). Keys: jc:is_following:{follower_id}:{entity_type}:{entity_id}.
// Optional — when nil, caching is disabled (backward-compatible with tests).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// SetActivityEmitter injects the wallet activity emitter used to record
// follow.created events so §64 badge/challenge triggers (e.g.
// super_connector) can fire. Optional — nil-safe.
func (s *Service) SetActivityEmitter(e wallet.ActivityEmitter) { s.activity = e }

// emitActivity records a wallet activity event and enqueues its processing.
// Fire-and-forget (Rule 8): failures are logged, never propagated. A
// duplicate reference hits ON CONFLICT DO NOTHING and returns ErrNoRows —
// treated as "already recorded" so unfollow/refollow cycles cannot farm the
// activity count.
func (s *Service) emitActivity(ctx context.Context, tenantID, userID pgtype.UUID, key, reference string, payload map[string]interface{}) {
	if s.activity == nil || !userID.Valid {
		return
	}

	event, err := s.activity.EmitActivityEvent(ctx, tenantID, userID, wallet.EmitActivityEventInput{
		ActivityKey: key,
		Reference:   reference,
		Payload:     payload,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("follows: emit activity event failed", "activity_key", key, "error", err)
		}

		return
	}

	if err := s.activity.EnqueueProcessActivityEvent(ctx, event.ID, tenantID); err != nil {
		slog.Warn("follows: enqueue activity event failed", "activity_key", key, "error", err)
	}
}

// isFollowingKey returns the Valkey cache key for a follow-status lookup.
// Ref: conventions/valkey-keys-catalog.md (Follow System section).
func isFollowingKey(followerID pgtype.UUID, entityType string, entityID pgtype.UUID) string {
	return valkey.MustKey("is_following:%s:%s:%s", followerID.String(), entityType, entityID.String())
}
