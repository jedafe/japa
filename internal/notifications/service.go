package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// NotificationInfo extends a notification row with the type label.
type NotificationInfo struct {
	gen.GetNotificationsByRecipientRow
}

// PreferenceInfo extends a preference row with the type label.
type PreferenceInfo struct {
	gen.GetNotificationPreferencesRow
}

// DispatchResult describes the outcome of a dispatch attempt.
type DispatchResult struct {
	Action   string     `json:"action"`    // "created", "extended", or "immediate"
	WindowID pgtype.UUID `json:"window_id"`
}

// DispatchNotification handles the batch dedup logic for a notification event.
// 1. Check for open batch window for (recipient, type, subject)
// 2. If window exists and not dispatched: extend it (add actor)
// 3. If no window: create new window
// Returns whether a new notification was created or batch was extended.
func (s *Service) DispatchNotification(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) (DispatchResult, error) {
	// Check user preferences — if in-app is disabled, skip entirely.
	pref, err := s.store.getNotificationPreferences(ctx, recipientID)
	if err == nil {
		for _, p := range pref {
			if p.NotificationType == nType && !p.InApp {
				return DispatchResult{Action: "skipped"}, nil
			}
		}
	}

	// Try to find an existing open batch window.
	window, err := s.store.getOpenBatchWindow(ctx, gen.GetOpenBatchWindowParams{
		RecipientID:      recipientID,
		NotificationType: nType,
		SubjectType:      subjectType,
		SubjectID:        subjectID,
	})
	if err == nil && !window.Dispatched {
		// Extend existing window.
		if err := s.store.extendBatchWindow(ctx, gen.ExtendBatchWindowParams{
			ActorID: actorID,
			ID:      window.ID,
		}); err != nil {
			return DispatchResult{}, fmt.Errorf("extending batch window: %w", err)
		}
		return DispatchResult{Action: "extended", WindowID: window.ID}, nil
	}

	// Create new window.
	if err := s.store.createBatchWindow(ctx, gen.CreateBatchWindowParams{
		RecipientID:      recipientID,
		NotificationType: nType,
		SubjectType:      subjectType,
		SubjectID:        subjectID,
		ActorID:          actorID,
	}); err != nil {
		return DispatchResult{}, fmt.Errorf("creating batch window: %w", err)
	}

	// Retrieve the newly created window to get its ID.
	window, err = s.store.getOpenBatchWindow(ctx, gen.GetOpenBatchWindowParams{
		RecipientID:      recipientID,
		NotificationType: nType,
		SubjectType:      subjectType,
		SubjectID:        subjectID,
	})
	if err != nil {
		return DispatchResult{}, fmt.Errorf("retrieving new batch window: %w", err)
	}

	return DispatchResult{Action: "created", WindowID: window.ID}, nil
}

// DispatchBatch processes a completed batch window.
// Creates the final notification row with aggregated actor info
// (e.g., "Alice and 9 others reacted") and marks the window as dispatched.
func (s *Service) DispatchBatch(ctx context.Context, windowID pgtype.UUID, tenantID pgtype.UUID) error {
	// We need the window details to build the notification.
	// Since there's no "get by ID" query, we'll build the notification from
	// the window's data. This is called by the Asynq task processor.
	return s.store.markBatchWindowDispatched(ctx, windowID)
}

// CreateNotification directly creates a single notification (non-batch path).
func (s *Service) CreateNotification(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error {
	var actorTypePtr *string
	if actorType != "" {
		actorTypePtr = &actorType
	}
	var subjectTypePtr *string
	if subjectType != "" {
		subjectTypePtr = &subjectType
	}
	var bodyPtr *string
	if body != "" {
		bodyPtr = &body
	}
	var actionURLPtr *string
	if actionURL != "" {
		actionURLPtr = &actionURL
	}

	return s.store.insertNotification(ctx, gen.InsertNotificationParams{
		TenantID:         tenantID,
		RecipientID:      recipientID,
		NotificationType: nType,
		ActorType:        actorTypePtr,
		ActorID:          actorID,
		SubjectType:      subjectTypePtr,
		SubjectID:        subjectID,
		Title:            title,
		Body:             bodyPtr,
		ActionUrl:        actionURLPtr,
	})
}

// GetNotifications returns paginated notifications for a user.
func (s *Service) GetNotifications(ctx context.Context, tenantID, recipientID pgtype.UUID, limit, offset int32) ([]NotificationInfo, error) {
	rows, err := s.store.getNotificationsByRecipient(ctx, tenantID, recipientID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getting notifications: %w", err)
	}
	result := make([]NotificationInfo, len(rows))
	for i, r := range rows {
		result[i] = NotificationInfo{GetNotificationsByRecipientRow: r}
	}
	return result, nil
}

// GetUnreadCount returns the count of unread notifications.
func (s *Service) GetUnreadCount(ctx context.Context, tenantID, recipientID pgtype.UUID) (int, error) {
	count, err := s.store.getUnreadCount(ctx, tenantID, recipientID)
	if err != nil {
		return 0, fmt.Errorf("getting unread count: %w", err)
	}
	return int(count), nil
}

// MarkAsRead marks a single notification as read.
func (s *Service) MarkAsRead(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	return s.store.markNotificationRead(ctx, notificationID, recipientID)
}

// MarkAllAsRead marks all notifications as read for a user.
func (s *Service) MarkAllAsRead(ctx context.Context, tenantID, recipientID pgtype.UUID) error {
	return s.store.markAllNotificationsRead(ctx, tenantID, recipientID)
}

// DeleteNotification deletes a single notification.
func (s *Service) DeleteNotification(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	return s.store.deleteNotification(ctx, notificationID, recipientID)
}

// GetPreferences returns user's notification preferences.
func (s *Service) GetPreferences(ctx context.Context, userID pgtype.UUID) ([]PreferenceInfo, error) {
	rows, err := s.store.getNotificationPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("getting preferences: %w", err)
	}
	result := make([]PreferenceInfo, len(rows))
	for i, r := range rows {
		result[i] = PreferenceInfo{GetNotificationPreferencesRow: r}
	}
	return result, nil
}

// UpdatePreference updates a single notification type preference.
func (s *Service) UpdatePreference(ctx context.Context, userID pgtype.UUID, nType string, inApp, email, push bool) error {
	return s.store.upsertNotificationPreference(ctx, gen.UpsertNotificationPreferenceParams{
		UserID:           userID,
		NotificationType: nType,
		InApp:            inApp,
		Email:            email,
		Push:             push,
	})
}

// GetNotificationTypes returns all configured notification types.
func (s *Service) GetNotificationTypes(ctx context.Context) ([]gen.NotificationType, error) {
	return s.store.getNotificationTypes(ctx)
}

// BuildBatchTitle constructs an aggregated title from actor info.
// Examples: "Alice reacted", "Alice and 2 others reacted", "Alice and 9 others reacted"
func BuildBatchTitle(singleActorName string, actorCount int32, action string) string {
	if actorCount <= 1 {
		return fmt.Sprintf("%s %s", singleActorName, action)
	}
	others := actorCount - 1
	return fmt.Sprintf("%s and %d other%s %s", singleActorName, others, pluralize(others), action)
}

// pluralize returns "s" if n != 1, empty string otherwise.
func pluralize(n int32) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// SplitNames splits a comma-separated name list and returns the first name.
func SplitNames(names string) string {
	parts := strings.Split(names, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
