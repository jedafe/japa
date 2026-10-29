// Package notifications implements the Notification System (Section 35).
// Supports in-app, email, and push delivery with batch deduplication.
// Notification types are admin-configurable. Real-time delivery via WebSocket
// notification events (Section 10).
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// ErrPushSubscriptionNotOwned is returned when a user attempts to delete a
// push subscription that does not belong to them. Ref: BUG-005, §35.
var ErrPushSubscriptionNotOwned = errors.New("push subscription not owned by user")

// ErrNotificationNotFound is returned when a notification id is unknown or
// owned by a different user (BUG-016 — replaces silent no-op success).
var ErrNotificationNotFound = errors.New("notification not found")

// ErrNotificationTypeNotFound is returned when a notification_type key does
// not exist (BUG-011/019 — replaces FK-violation 500s and silent no-ops).
var ErrNotificationTypeNotFound = errors.New("notification type not found")

// ErrNotificationTypeExists is returned when POST /admin/notification-types
// receives an already-registered key (was silently overwritten before).
var ErrNotificationTypeExists = errors.New("notification type already exists")

// Notification type constants — must match the seeded rows in notification_types
// (migration 000099, Section 35 seed catalog).
const (
	TypeNewFollower        = "follow.received"
	TypeContentComment     = "content.comment"
	TypeCommentReply       = "comment.reply"
	TypeMentionContent     = "mention.content"
	TypeMentionComment     = "mention.comment"
	TypeContentReaction    = "content.reaction"
	TypeDMMessage          = "dm.message"
	TypeDMRequest          = "dm.request"
	TypeSystemAnnouncement = "broadcast"

	// Deprecated aliases for backward compatibility with older call-sites.
	// These map to the legacy 000013 seed keys. New code should use the
	// JEOS-spec constants above.
	TypePostReply = "post_reply"
	TypeComment   = "comment"
	TypeMention   = "mention"
	TypeReaction  = "reaction"
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

// WSHuber abstracts the WebSocket hub so the notification service can broadcast
// real-time `notification` events without importing the websocket package
// directly (avoids an import cycle). Implemented by *websocket.Hub.
// Ref: japaconnect-engineering/sections/35-notification-system.md (BUG-002).
type WSHuber interface {
	BroadcastToUser(userID string, event WSEventPayload)
}

// WSEventPayload is the WebSocket event envelope passed to WSHuber.
// Mirrors websocket.WSEvent without the import.
type WSEventPayload struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Service provides notification operations with reader/writer pool separation.
type Service struct {
	store       *Store
	client      *asynq.Client
	push        *PushDelivery
	pushStore   *PushStore
	settings    SettingsProvider
	wsHub       WSHuber
	valkeyCache valkeycompat.Cmdable
	lit         literalResolver // nil disables literal resolution (§14)
}

// SetLiteralResolver injects the template-literal catalog so notification
// titles/bodies in the real-time WebSocket payload render {{...}}
// placeholders — the live event has no later read-path resolution (§14).
func (s *Service) SetLiteralResolver(l literalResolver) { s.lit = l }

// SetValkey injects the Valkey client for unread-count caching.
// BUG-013: previously every unread-count badge poll hit the DB.
// Ref: japaconnect-engineering/sections/35-notification-system.md (jc:notif_unread).
func (s *Service) SetValkey(vk *valkey.Client) {
	if vk == nil {
		return
	}

	s.valkeyCache = vk.Redis()
}

// unreadCacheKey builds the Valkey key for a user's unread notification count.
// Format: jc:notif_unread:{user_id} (§35, valkey-keys-catalog).
func unreadCacheKey(userID pgtype.UUID) string {
	return valkey.MustKey("notif_unread:%s", userID.String())
}

// getCachedUnreadCount returns the cached unread count, or -1 on miss/error.
func (s *Service) getCachedUnreadCount(ctx context.Context, userID pgtype.UUID) int {
	if s.valkeyCache == nil {
		return -1
	}

	n, err := s.valkeyCache.Get(ctx, unreadCacheKey(userID)).Int()
	if err != nil {
		return -1
	}

	return n
}

// setCachedUnreadCount stores the unread count in Valkey with a 60s TTL.
func (s *Service) setCachedUnreadCount(ctx context.Context, userID pgtype.UUID, count int) {
	if s.valkeyCache == nil {
		return
	}

	if err := s.valkeyCache.Set(ctx, unreadCacheKey(userID), strconv.Itoa(count), 60*time.Second).Err(); err != nil {
		slog.Warn("notifications: failed to cache unread count", "error", err)
	}
}

// invalidateUnreadCount deletes the cached unread count.
// Called after mark-read, mark-all-read, delete, and new notification insert.
func (s *Service) invalidateUnreadCount(ctx context.Context, userID pgtype.UUID) {
	if s.valkeyCache == nil {
		return
	}

	if err := s.valkeyCache.Del(ctx, unreadCacheKey(userID)).Err(); err != nil {
		slog.Warn("notifications: failed to invalidate unread count cache", "error", err)
	}
}

// SettingsProvider abstracts system settings access for notification batching.
type SettingsProvider interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// NewService creates a Service backed by the given database pool.
// The push delivery client is built from JC_VAPID_* environment variables.
// Prefer NewServiceWithConfig when a config.Config is available (Rule 3).
func NewService(database *db.DB) *Service {
	return NewServiceWithConfig(database, PushConfig{
		PrivateKey: os.Getenv("JC_VAPID_PRIVATE_KEY"),
		PublicKey:  os.Getenv("JC_VAPID_PUBLIC_KEY"),
		Subject:    os.Getenv("JC_VAPID_SUBJECT"),
	})
}

// NewServiceWithConfig creates a Service backed by the given database pool
// and a PushConfig loaded by internal/config (Rule 3 — no ad-hoc os.Getenv).
// Pass notifications.PushConfig fields from a *config.Config.
func NewServiceWithConfig(database *db.DB, pushCfg PushConfig) *Service {
	store := &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
	store.setWritePool(database.Writer)

	return &Service{
		store:     store,
		push:      NewPushDeliveryFromConfig(pushCfg),
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

// SetWSHub injects the WebSocket hub for real-time notification delivery.
// BUG-002: without this, new notifications are never pushed to the client
// via the `notification` WS event — users must manually refresh.
// Ref: japaconnect-engineering/sections/35-notification-system.md.
func (s *Service) SetWSHub(h WSHuber) {
	s.wsHub = h
}

// broadcastNotification sends a `notification` WS event to the recipient.
// Called after every successful notification insert (batch or direct).
// BUG-002: previously missing — real-time delivery was broken.
func (s *Service) broadcastNotification(ctx context.Context, recipientID pgtype.UUID, payload interface{}) {
	if s.wsHub == nil {
		return
	}

	// §14: resolve {{...}} in the live payload — the WS event has no later
	// read-path resolution.
	if s.lit != nil {
		s.lit.ResolveData(ctx, payload, literals.NewContext(ctx, ""))
	}

	s.wsHub.BroadcastToUser(recipientID.String(), WSEventPayload{
		Type:    "notification",
		Payload: payload,
	})
}

// broadcastUnreadCount pushes the fresh unread count to the user's open
// sessions (§35 L121 — "also pushed via WebSocket"). BUG-017: previously
// the badge only moved on new `notification` events, so marking read on one
// device never synced the badge on another. Best-effort — DB errors degrade
// to no push; the 60s Valkey TTL self-heals the next poll.
func (s *Service) broadcastUnreadCount(ctx context.Context, recipientID pgtype.UUID) {
	if s.wsHub == nil {
		return
	}

	count, err := s.store.getUnreadCount(ctx, recipientID)
	if err != nil {
		return
	}

	s.wsHub.BroadcastToUser(recipientID.String(), WSEventPayload{
		Type:    "notification_unread_count",
		Payload: map[string]any{"count": int(count)},
	})
}

// PushDelivery returns the push delivery instance for subscription management.
func (s *Service) PushDelivery() *PushDelivery {
	return s.push
}

// enqueuePushDelivery enqueues a background push delivery task for a recipient.
// BUG-004: respects notifications.max_delivery_attempts from system_settings.
func (s *Service) enqueuePushDelivery(recipientID pgtype.UUID, title, body, actionURL, dedupID string) {
	s.enqueuePushDeliveryWithCtx(context.Background(), recipientID, title, body, actionURL, dedupID)
}

// enqueuePushDeliveryWithCtx is the context-aware variant of enqueuePushDelivery.
// BUG-006: previously enqueuePushDelivery used context.Background() and a zero
// UUID platform for the settings lookup, losing platform context. This variant
// accepts the request context so the settings provider can scope the lookup
// and the operation respects cancellation/deadline.
//
// dedupID rides inside the task payload so asynq.Unique collapses only true
// redeliveries of the same logical notification — BUG-012: identical text
// alone (two "Alice reacted" batches in an hour) previously deduped away a
// legitimate second push.
func (s *Service) enqueuePushDeliveryWithCtx(ctx context.Context, recipientID pgtype.UUID, title, body, actionURL, dedupID string) {
	if s.client == nil || s.push == nil || !s.push.IsEnabled() {
		return
	}

	payload, err := json.Marshal(PushDeliveryPayload{
		UserID:         recipientID.String(),
		Title:          title,
		Body:           body,
		ActionURL:      actionURL,
		NotificationID: dedupID,
	})
	if err != nil {
		slog.Error("notifications: failed to marshal push payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypePushDelivery, payload)
	opts := []asynq.Option{
		asynq.Queue(worker.QueueForTask(worker.TypePushDelivery)),
		asynq.Unique(1 * time.Hour),
	}
	// BUG-004: cap retries at notifications.max_delivery_attempts (default 5).
	// BUG-006: use the passed context instead of context.Background().
	if s.settings != nil {
		maxAttempts := s.settings.GetIntOr(ctx, "notifications.max_delivery_attempts", 5)
		if maxAttempts > 0 {
			opts = append(opts, asynq.MaxRetry(maxAttempts))
		}
	}

	info, err := s.client.Enqueue(task, opts...)
	if err != nil {
		slog.Error("notifications: failed to enqueue push delivery",
			"recipient_id", recipientID.String(), "error", err)
		return
	}

	slog.Info("notifications: push delivery enqueued",
		"recipient_id", recipientID.String(), "task_id", info.ID)
}

// checkAndEnqueuePush checks user preferences and enqueues push if enabled.
// BUG-002: previously only checked explicit user preferences, ignoring
// notification_type defaults. A type with default_push=true (e.g.
// mention.content, dm.message) would never send push if the user had no
// saved preference. Now resolves the effective preference (user override
// OR type default) before deciding.
func (s *Service) checkAndEnqueuePush(ctx context.Context, recipientID pgtype.UUID, nType, title, body, actionURL, dedupID string) {
	prefs, err := s.resolveEffectivePreferences(ctx, recipientID)
	if err != nil {
		return
	}

	if ep, ok := prefs[nType]; ok && ep.Push && !ep.Disabled {
		s.enqueuePushDeliveryWithCtx(ctx, recipientID, title, body, actionURL, dedupID)
	}
}

// enqueueEmailDelivery enqueues an email:send task for a notification.
// BUG-003: previously email delivery was never dispatched even when the
// user's preference had email=true. Ref: §35 L88.
func (s *Service) enqueueEmailDelivery(ctx context.Context, recipientID pgtype.UUID, nType, title, body, actionURL string) {
	if s.client == nil {
		return
	}

	// Resolve the recipient's email address. We use the store's user lookup;
	// if the user has no email, skip silently.
	email, err := s.store.getUserEmail(ctx, recipientID)
	if err != nil || email == nil || *email == "" {
		return
	}

	// BUG-023: subject-only notification emails had no CTA — append the deep
	// link so the message is actionable.
	if actionURL != "" {
		body = strings.TrimSpace(body + "\n\nView: " + actionURL)
	}

	payload, err := json.Marshal(EmailSendPayload{
		To:        *email,
		Subject:   title,
		Body:      body,
		Type:      nType,
		ActionURL: actionURL,
	})
	if err != nil {
		slog.Error("notifications: failed to marshal email payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypeEmailSend, payload)
	opts := []asynq.Option{
		asynq.Queue(worker.QueueForTask(worker.TypeEmailSend)),
	}
	// BUG-004: cap retries at notifications.max_delivery_attempts (default 5).
	if s.settings != nil {
		maxAttempts := s.settings.GetIntOr(ctx, "notifications.max_delivery_attempts", 5)
		if maxAttempts > 0 {
			opts = append(opts, asynq.MaxRetry(maxAttempts))
		}
	}

	if _, err := s.client.Enqueue(task, opts...); err != nil {
		slog.Error("notifications: failed to enqueue email delivery",
			"recipient_id", recipientID.String(), "error", err)
		return
	}

	slog.Info("notifications: email delivery enqueued",
		"recipient_id", recipientID.String(), "type", nType)
}

// checkAndEnqueueEmail checks user preferences and enqueues email if enabled.
// BUG-003: previously missing — email delivery was never dispatched.
// BUG-002: previously only checked explicit user preferences, ignoring
// notification_type defaults. A type with default_email=true (e.g.
// payment.failed, org.invite) would never send email if the user had no
// saved preference. Now resolves the effective preference (user override
// OR type default) before deciding.
func (s *Service) checkAndEnqueueEmail(ctx context.Context, recipientID pgtype.UUID, nType, title, body, actionURL string) {
	prefs, err := s.resolveEffectivePreferences(ctx, recipientID)
	if err != nil {
		return
	}

	if ep, ok := prefs[nType]; ok && ep.Email && !ep.Disabled {
		s.enqueueEmailDelivery(ctx, recipientID, nType, title, body, actionURL)
	}
}

// EmailSendPayload is the Asynq task payload for email:send.
type EmailSendPayload struct {
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Type      string `json:"type"`
	ActionURL string `json:"action_url,omitempty"`
}

// PushSubscription is the public view of a browser push subscription.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
// Ref: japaconnect-engineering/sections/35-notification-system.md
// Excludes p256dh/auth_key so the client never receives private VAPID keys.
type PushSubscription struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
}

// ListMyPushSubscriptions returns the user's web push subscriptions.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (Devices section).
func (s *Service) ListMyPushSubscriptions(ctx context.Context, userID pgtype.UUID) ([]PushSubscription, error) {
	rows, err := s.pushStore.ListPushSubscriptionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]PushSubscription, len(rows))
	for i, r := range rows {
		out[i] = PushSubscription{ID: r.ID.String(), Endpoint: r.Endpoint}
	}

	return out, nil
}

// SavePushSubscription stores a browser push subscription for a user.
// maxPushSubscriptionsPerUser caps stored web push subscriptions — without
// a cap a client could grow the table unboundedly. Oldest rows are evicted
// so the newest subscription always wins (API-safety invariant, not a
// configurable — mirrors the MaxPageSize clamp precedent).
const maxPushSubscriptionsPerUser = 20

func (s *Service) SavePushSubscription(ctx context.Context, userID pgtype.UUID, endpoint, p256dh, authKey string) error {
	if err := s.pushStore.SavePushSubscription(ctx, userID, endpoint, p256dh, authKey); err != nil {
		return err
	}

	// Evict-oldest trim over the cap. Rows are ordered created_at ASC so the
	// head of the list is the stale end.
	subs, err := s.pushStore.ListPushSubscriptionsByUser(ctx, userID)
	if err != nil || len(subs) <= maxPushSubscriptionsPerUser {
		return err
	}

	for _, old := range subs[:len(subs)-maxPushSubscriptionsPerUser] {
		if err := s.pushStore.DeletePushSubscriptionByID(ctx, old.ID); err != nil {
			slog.Warn("notifications: push subscription trim failed", "id", old.ID.String(), "error", err)
		}
	}

	return nil
}

// DeletePushSubscription removes a browser push subscription by endpoint.
// Kept for backward compatibility with callers that key by endpoint.
func (s *Service) DeletePushSubscription(ctx context.Context, userID pgtype.UUID, endpoint string) error {
	return s.pushStore.DeletePushSubscriptionByEndpoint(ctx, userID, endpoint)
}

// DeletePushSubscriptionByID removes a browser push subscription by its
// primary key. BUG-005: catalog route DELETE /me/push-tokens/{id} uses the
// path-param id. Performs an ownership check before deletion.
func (s *Service) DeletePushSubscriptionByID(ctx context.Context, userID, id pgtype.UUID) error {
	sub, err := s.pushStore.GetPushSubscriptionByID(ctx, id)
	if err != nil {
		return fmt.Errorf("looking up push subscription: %w", err)
	}
	// Ownership check: the subscription must belong to the authenticated user.
	if sub.UserID != userID {
		return ErrPushSubscriptionNotOwned
	}

	return s.pushStore.DeletePushSubscriptionByID(ctx, id)
}

// GetVAPIDPublicKey returns the VAPID public key for client-side subscription.
func (s *Service) GetVAPIDPublicKey() string {
	return s.push.PublicKey()
}
