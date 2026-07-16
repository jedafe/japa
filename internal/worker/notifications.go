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
