package notifications

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for notifications.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// insertNotification creates a new notification row and returns its ID.
// BUG-005: now returns the notification ID so callers can include it in
// the WebSocket broadcast payload (§35 L88, §10).
func (s *Store) insertNotification(ctx context.Context, params gen.InsertNotificationParams) (pgtype.UUID, error) {
	id, err := s.write.InsertNotification(ctx, params)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("inserting notification: %w", err)
	}

	return id, nil
}

// getNotificationsByRecipient returns paginated notifications for a user.
// BUG-007: supports the unread_only filter (§35 L107).
func (s *Store) getNotificationsByRecipient(ctx context.Context, tenantID, recipientID pgtype.UUID, limit, offset int32, unreadOnly bool) ([]gen.GetNotificationsByRecipientRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetNotificationsByRecipient(ctx, gen.GetNotificationsByRecipientParams{
		TenantID:    tenantID,
		RecipientID: recipientID,
		PageLimit:   limit,
		PageOffset:  offset,
		UnreadOnly:  unreadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("getting notifications: %w", err)
	}

	return rows, nil
}

// getUnreadCount returns the number of unread notifications.
func (s *Store) getUnreadCount(ctx context.Context, tenantID, recipientID pgtype.UUID) (int32, error) {
	count, err := s.read.GetUnreadNotificationCount(ctx, gen.GetUnreadNotificationCountParams{
		TenantID:    tenantID,
		RecipientID: recipientID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting unread notifications: %w", err)
	}

	return count, nil
}

// markNotificationRead marks a single notification as read.
func (s *Store) markNotificationRead(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	if err := s.write.MarkNotificationRead(ctx, gen.MarkNotificationReadParams{
		ID:          notificationID,
		RecipientID: recipientID,
	}); err != nil {
		return fmt.Errorf("marking notification read: %w", err)
	}

	return nil
}

// markAllNotificationsRead marks all notifications as read for a user.
func (s *Store) markAllNotificationsRead(ctx context.Context, tenantID, recipientID pgtype.UUID) error {
	if err := s.write.MarkAllNotificationsRead(ctx, gen.MarkAllNotificationsReadParams{
		TenantID:    tenantID,
		RecipientID: recipientID,
	}); err != nil {
		return fmt.Errorf("marking all notifications read: %w", err)
	}

	return nil
}

// deleteNotification deletes a single notification.
func (s *Store) deleteNotification(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	if err := s.write.DeleteNotification(ctx, gen.DeleteNotificationParams{
		ID:          notificationID,
		RecipientID: recipientID,
	}); err != nil {
		return fmt.Errorf("deleting notification: %w", err)
	}

	return nil
}

// getNotificationPreferences returns user's notification preferences.
func (s *Store) getNotificationPreferences(ctx context.Context, userID pgtype.UUID) ([]gen.GetNotificationPreferencesRow, error) {
	rows, err := s.read.GetNotificationPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("getting notification preferences: %w", err)
	}

	return rows, nil
}

// upsertNotificationPreference creates or updates a notification preference.
func (s *Store) upsertNotificationPreference(ctx context.Context, params gen.UpsertNotificationPreferenceParams) error {
	if err := s.write.UpsertNotificationPreference(ctx, params); err != nil {
		return fmt.Errorf("upserting notification preference: %w", err)
	}

	return nil
}

// getOpenBatchWindow returns an open, non-dispatched batch window.
func (s *Store) getOpenBatchWindow(ctx context.Context, params gen.GetOpenBatchWindowParams) (gen.GetOpenBatchWindowRow, error) {
	window, err := s.read.GetOpenBatchWindow(ctx, params)
	if err != nil {
		return gen.GetOpenBatchWindowRow{}, fmt.Errorf("getting open batch window: %w", err)
	}

	return window, nil
}

// createBatchWindow creates a new batch dedup window.
func (s *Store) createBatchWindow(ctx context.Context, params gen.CreateBatchWindowParams) error {
	if err := s.write.CreateBatchWindow(ctx, params); err != nil {
		return fmt.Errorf("creating batch window: %w", err)
	}

	return nil
}

// extendBatchWindow adds an actor to an existing batch window.
func (s *Store) extendBatchWindow(ctx context.Context, params gen.ExtendBatchWindowParams) error {
	if err := s.write.ExtendBatchWindow(ctx, params); err != nil {
		return fmt.Errorf("extending batch window: %w", err)
	}

	return nil
}

// markBatchWindowDispatched marks a batch window as dispatched.
func (s *Store) markBatchWindowDispatched(ctx context.Context, windowID pgtype.UUID) error {
	if err := s.write.MarkBatchWindowDispatched(ctx, windowID); err != nil {
		return fmt.Errorf("marking batch window dispatched: %w", err)
	}

	return nil
}

// getBatchWindowByID returns a batch window by its ID.
func (s *Store) getBatchWindowByID(ctx context.Context, windowID pgtype.UUID) (gen.GetBatchWindowByIDRow, error) {
	window, err := s.read.GetBatchWindowByID(ctx, windowID)
	if err != nil {
		return gen.GetBatchWindowByIDRow{}, fmt.Errorf("getting batch window by ID: %w", err)
	}

	return window, nil
}

// getNotificationTypes returns all configured notification types.
func (s *Store) getNotificationTypes(ctx context.Context) ([]gen.NotificationType, error) {
	types, err := s.read.GetNotificationTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting notification types: %w", err)
	}

	return types, nil
}

// adminListNotificationTypes returns a filtered, sorted, paginated slice of
// notification types for the admin DataTable (Section 35).
func (s *Store) adminListNotificationTypes(ctx context.Context, p gen.AdminListNotificationTypesParams) ([]gen.NotificationType, error) {
	rows, err := s.read.AdminListNotificationTypes(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("listing notification types: %w", err)
	}
	return rows, nil
}

// adminCountNotificationTypes returns the total count of notification types
// matching the given filter (Section 35).
func (s *Store) adminCountNotificationTypes(ctx context.Context, p gen.AdminCountNotificationTypesParams) (int32, error) {
	count, err := s.read.AdminCountNotificationTypes(ctx, p)
	if err != nil {
		return 0, fmt.Errorf("counting notification types: %w", err)
	}
	return count, nil
}

// upsertNotificationType inserts a notification type, updating label/description on conflict.
func (s *Store) upsertNotificationType(ctx context.Context, p gen.UpsertNotificationTypeParams) error {
	if err := s.write.UpsertNotificationType(ctx, p); err != nil {
		return fmt.Errorf("upserting notification type: %w", err)
	}

	return nil
}

// updateNotificationType updates an existing notification type definition.
func (s *Store) updateNotificationType(ctx context.Context, p gen.UpdateNotificationTypeParams) error {
	if err := s.write.UpdateNotificationType(ctx, p); err != nil {
		return fmt.Errorf("updating notification type: %w", err)
	}

	return nil
}

// deleteNotificationType removes a notification type definition.
func (s *Store) deleteNotificationType(ctx context.Context, key string) error {
	if err := s.write.DeleteNotificationType(ctx, key); err != nil {
		return fmt.Errorf("deleting notification type: %w", err)
	}

	return nil
}

// getUserDisplayName returns a user's display_name (nil when unset).
// BUG-003: previously passed a zero UUID as tenant_id, which caused
// GetUserProfile to return ErrNoRows for every user. Now requires the
// correct tenant_id so the tenant-scoped WHERE clause matches.
func (s *Store) getUserDisplayName(ctx context.Context, tenantID, userID pgtype.UUID) (*string, error) {
	user, err := s.read.GetUserProfile(ctx, gen.GetUserProfileParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting user profile: %w", err)
	}

	return user.DisplayName, nil
}

// getUserEmail returns the user's email address for notification email
// delivery (§35, BUG-003). Tenant-scoped per §04.
func (s *Store) getUserEmail(ctx context.Context, tenantID, userID pgtype.UUID) (*string, error) {
	email, err := s.read.GetUserEmail(ctx, gen.GetUserEmailParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting user email: %w", err)
	}
	return &email, nil
}
