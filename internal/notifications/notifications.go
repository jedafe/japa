// Package notifications implements the Notification System (Section 35).
// Supports in-app, email, and push delivery with batch deduplication.
// Notification types are admin-configurable. Real-time delivery via WebSocket
// notification events (Section 10).
package notifications

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Notification type constants — must match the seeded rows in notification_types.
const (
	TypeNewFollower        = "new_follower"
	TypePostReply          = "post_reply"
	TypeComment            = "comment"
	TypeMention            = "mention"
	TypeReaction           = "reaction"
	TypeDMMessage          = "dm_message"
	TypeDMRequest          = "dm_request"
	TypeSystemAnnouncement = "system_announcement"
)

// Actor type constants.
const (
	ActorTypeUser   = "user"
	ActorTypeSystem = "system"
)

// Batch window duration in minutes for deduplication.
const BatchWindowMinutes = 5

// DefaultPageSize is the default number of notifications returned per page.
const DefaultPageSize = 20

// MaxPageSize is the maximum number of notifications a client can request.
const MaxPageSize = 100

// Service provides notification operations with reader/writer pool separation.
type Service struct {
	store *Store
	db    *db.DB
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
