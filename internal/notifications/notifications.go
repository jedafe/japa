// Package notifications implements the Notification System (Section 35).
// Supports in-app, email, and push delivery with batch deduplication.
// Notification types are admin-configurable. Real-time delivery via WebSocket
// notification events (Section 10).
package notifications

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
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

// BatchWindowMinutes is the batch window duration in minutes for deduplication.
const BatchWindowMinutes = 5

// DefaultPageSize is the default number of notifications returned per page.
const DefaultPageSize = 20

// MaxPageSize is the maximum number of notifications a client can request.
const MaxPageSize = 100

// Service provides notification operations with reader/writer pool separation.
type Service struct {
	store     *Store
	client    *asynq.Client
	push      *PushDelivery
	pushStore *PushStore
	settings  SettingsProvider
}

// SettingsProvider abstracts system settings access for notification batching.
type SettingsProvider interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// NewService creates a Service backed by the given database pool.
// All data access goes through the store, so the raw pool is not retained.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		push:      NewPushDelivery(),
		pushStore: NewPushStore(database.Reader, database.Writer),
	}
}

// SetAsynqClient injects the Asynq client for enqueuing push delivery tasks.
func (s *Service) SetAsynqClient(client *asynq.Client) {
	s.client = client
}

// SetSettings injects the system settings provider for configurable batch windows.
func (s *Service) SetSettings(sp SettingsProvider) {
	s.settings = sp
}

// PushDelivery returns the push delivery instance for subscription management.
func (s *Service) PushDelivery() *PushDelivery {
	return s.push
}

// enqueuePushDelivery enqueues a background push delivery task for a recipient.
func (s *Service) enqueuePushDelivery(recipientID pgtype.UUID, title, body, actionURL string) {
	if s.client == nil || s.push == nil || !s.push.IsEnabled() {
		return
	}

	payload, err := json.Marshal(PushDeliveryPayload{
		UserID:    recipientID.String(),
		Title:     title,
		Body:      body,
		ActionURL: actionURL,
	})
	if err != nil {
		slog.Error("notifications: failed to marshal push payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypePushDelivery, payload)

	info, err := s.client.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypePushDelivery)), asynq.Unique(1*time.Hour))
	if err != nil {
		slog.Error("notifications: failed to enqueue push delivery",
			"recipient_id", recipientID.String(), "error", err)
		return
	}

	slog.Info("notifications: push delivery enqueued",
		"recipient_id", recipientID.String(), "task_id", info.ID)
}

// checkAndEnqueuePush checks user preferences and enqueues push if enabled.
func (s *Service) checkAndEnqueuePush(ctx context.Context, recipientID pgtype.UUID, nType, title, body, actionURL string) {
	pref, err := s.store.getNotificationPreferences(ctx, recipientID)
	if err != nil {
		return
	}

	for _, p := range pref {
		if p.NotificationType == nType && p.Push {
			s.enqueuePushDelivery(recipientID, title, body, actionURL)
			return
		}
	}
}

// SavePushSubscription stores a browser push subscription for a user.
func (s *Service) SavePushSubscription(ctx context.Context, userID pgtype.UUID, endpoint, p256dh, authKey string) error {
	return s.pushStore.SavePushSubscription(ctx, userID, endpoint, p256dh, authKey)
}

// DeletePushSubscription removes a browser push subscription for a user.
func (s *Service) DeletePushSubscription(ctx context.Context, userID pgtype.UUID, endpoint string) error {
	return s.pushStore.DeletePushSubscription(ctx, userID, endpoint)
}

// GetVAPIDPublicKey returns the VAPID public key for client-side subscription.
func (s *Service) GetVAPIDPublicKey() string {
	return s.push.PublicKey()
}
