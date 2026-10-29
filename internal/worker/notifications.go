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

// DMNotificationCreator creates in-app notification rows for DM events.
// Implemented by notifications.Service.CreateNotification (BUG-005: the
// dm:notify_* handlers previously enqueued web push only, so the seeded
// dm.message/dm.request types never produced notification-centre rows and
// user preferences were bypassed).
type DMNotificationCreator interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
	// HasUnreadNotification powers the per-conversation dedup — a second DM
	// on an already-unread conversation gets push only, not another row.
	HasUnreadNotification(ctx context.Context, recipientID pgtype.UUID, nType, subjectType string, subjectID pgtype.UUID) (bool, error)
}

var dmNotificationCreator DMNotificationCreator

// RegisterDMNotificationCreator registers the notifications service for
// dm:notify_* tasks.
func RegisterDMNotificationCreator(d DMNotificationCreator) {
	dmNotificationCreator = d
}
