// Package websocket implements the Real-Time WebSocket system (Section 10).
// WebSocket hub with Valkey pub/sub bridge for multi-instance scaling.
// Seven event types: notification, presence, typing, space_activity,
// feed_update, dm_message, moderation_alert.
// Direct Messaging with conversation threads and message requests.
package websocket

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Event type constants for WebSocket messages.
const (
	EventNotification    = "notification"
	EventPresence        = "presence"
	EventTyping          = "typing"
	EventSpaceActivity   = "space_activity"
	EventFeedUpdate      = "feed_update"
	EventDMMessage       = "dm_message"
	EventModerationAlert = "moderation_alert"
)

// DM privacy settings.
const (
	DMPrivacyEveryone      = "everyone"
	DMPrivacyFollowersOnly = "followers_only"
	DMPrivacyNobody        = "nobody"
)

// DM message request statuses.
const (
	MessageRequestPending  = "pending"
	MessageRequestAccepted = "accepted"
	MessageRequestDeclined = "declined"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Service provides WebSocket and DM operations.
type Service struct {
	store *Store
	hub   *Hub
	db    *db.DB
}

// NewService creates a Service backed by the given database and hub.
func NewService(database *db.DB, hub *Hub) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		hub: hub,
		db:  database,
	}
}
