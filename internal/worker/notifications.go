package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// NotificationBatchDispatcher provides the batch window dispatch capability.
// The notifications service implements this interface and registers with the
// worker so HandleNotificationDispatchBatch can delegate to it.
type NotificationBatchDispatcher interface {
	DispatchBatch(ctx context.Context, windowID pgtype.UUID) error
}

var notificationBatchDispatcher NotificationBatchDispatcher

// RegisterNotificationBatchDispatcher registers the notifications service
// for batch window dispatch tasks.
func RegisterNotificationBatchDispatcher(d NotificationBatchDispatcher) {
	notificationBatchDispatcher = d
}

// NotificationCreator creates in-app notification rows through the full
// §35 pipeline — effective-preference gate, WS event, unread-badge
// invalidation, and pref-gated push/email fan-out — rather than a bare
// InsertNotification. Implemented by notifications.Service.CreateNotification
// (BUG-005: the dm:notify_* handlers previously enqueued web push only, so
// the seeded dm.message/dm.request types never produced notification-centre
// rows and user preferences were bypassed. BUG-030: every other worker-side
// producer did the same class of bypass — a bare insert with no pref gate,
// no WS event, no badge invalidation, and no push/email fan-out — so they
// were moved onto this interface too).
type NotificationCreator interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
	// HasUnreadNotification powers the per-conversation dedup — a second DM
	// on an already-unread conversation gets push only, not another row.
	HasUnreadNotification(ctx context.Context, recipientID pgtype.UUID, nType, subjectType string, subjectID pgtype.UUID) (bool, error)
}

var notificationCreator NotificationCreator

// RegisterNotificationCreator registers the notifications service for
// worker-side notification creation (dm:notify_* plus the direct-insert
// producers converted in BUG-030).
func RegisterNotificationCreator(d NotificationCreator) {
	notificationCreator = d
}
