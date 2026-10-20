package nextactions

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// NotificationCreator is the §35 slice the notifier needs.
type NotificationCreator interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// NotifierAdapter dispatches materialized suggestions as
// next_action.suggested notifications via §35 (in-app + push + email per
// the recipient's channel preferences).
type NotifierAdapter struct {
	svc NotificationCreator
}

// NewNotifier wraps the notifications service.
func NewNotifier(svc NotificationCreator) *NotifierAdapter {
	return &NotifierAdapter{svc: svc}
}

// NotifySuggestion sends the rule's payload as a notification.
func (n *NotifierAdapter) NotifySuggestion(ctx context.Context, userID pgtype.UUID, rule Rule) error {
	return n.svc.CreateNotification(ctx, userID, "next_action.suggested",
		"system", pgtype.UUID{}, "next_action_rule", rule.ID,
		rule.Payload.Title, rule.Payload.Body, rule.Payload.CTAURL)
}
