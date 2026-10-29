package notifications

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for notifications.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
	// writePool enables multi-statement transactions (transactional batch
	// dispatch, bulk preference upserts) — gen.Queries alone cannot BEGIN.
	writePool *pgxpool.Pool
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// setWritePool attaches the writer pool used for transactions. Called from
// NewServiceWithConfig where the pool is available.
func (s *Store) setWritePool(p *pgxpool.Pool) { s.writePool = p }

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
func (s *Store) getNotificationsByRecipient(ctx context.Context, recipientID pgtype.UUID, limit, offset int32, unreadOnly bool) ([]gen.GetNotificationsByRecipientRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.GetNotificationsByRecipient(ctx, gen.GetNotificationsByRecipientParams{
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

// countNotificationsByRecipient returns the total number of notifications
// matching the same unread_only filter as getNotificationsByRecipient, so
// the list endpoint can report { total, has_more } (§35).
func (s *Store) countNotificationsByRecipient(ctx context.Context, recipientID pgtype.UUID, unreadOnly bool) (int32, error) {
	count, err := s.read.CountNotificationsByRecipient(ctx, gen.CountNotificationsByRecipientParams{
		RecipientID: recipientID,
		UnreadOnly:  unreadOnly,
	})
	if err != nil {
		return 0, fmt.Errorf("counting notifications: %w", err)
	}

	return count, nil
}

// getNotificationTypeLabel returns the human-readable label for a
// notification type key (used to enrich WebSocket payloads).
func (s *Store) getNotificationTypeLabel(ctx context.Context, key string) (string, error) {
	label, err := s.read.GetNotificationTypeLabel(ctx, key)
	if err != nil {
		return "", fmt.Errorf("getting notification type label: %w", err)
	}

	return label, nil
}

// getUnreadCount returns the number of unread notifications.
func (s *Store) getUnreadCount(ctx context.Context, recipientID pgtype.UUID) (int32, error) {
	count, err := s.read.GetUnreadNotificationCount(ctx,

		recipientID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting unread notifications: %w", err)
	}

	return count, nil
}

// markNotificationRead marks a single notification as read.
// BUG-016: returns ErrNotificationNotFound when the id is unknown or owned
// by another user instead of silently succeeding on 0 affected rows.
func (s *Store) markNotificationRead(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	n, err := s.write.MarkNotificationRead(ctx, gen.MarkNotificationReadParams{
		ID:          notificationID,
		RecipientID: recipientID,
	})
	if err != nil {
		return fmt.Errorf("marking notification read: %w", err)
	}

	if n == 0 {
		return ErrNotificationNotFound
	}

	return nil
}

// markAllNotificationsRead marks all notifications as read for a user.
func (s *Store) markAllNotificationsRead(ctx context.Context, recipientID pgtype.UUID) error {
	if err := s.write.MarkAllNotificationsRead(ctx,

		recipientID,
	); err != nil {
		return fmt.Errorf("marking all notifications read: %w", err)
	}

	return nil
}

// deleteNotification deletes a single notification.
// BUG-016: returns ErrNotificationNotFound when the id is unknown or owned
// by another user instead of silently succeeding on 0 affected rows.
func (s *Store) deleteNotification(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	n, err := s.write.DeleteNotification(ctx, gen.DeleteNotificationParams{
		ID:          notificationID,
		RecipientID: recipientID,
	})
	if err != nil {
		return fmt.Errorf("deleting notification: %w", err)
	}

	if n == 0 {
		return ErrNotificationNotFound
	}

	return nil
}

// hasUnreadNotificationForSubject reports whether an unread notification
// already exists for (recipient, type, subject) — used by repeat-event
// flows (DMs) to avoid stacking a second in-app row on the same subject.
func (s *Store) hasUnreadNotificationForSubject(ctx context.Context, recipientID pgtype.UUID, nType, subjectType string, subjectID pgtype.UUID) (bool, error) {
	has, err := s.read.HasUnreadNotificationForSubject(ctx, gen.HasUnreadNotificationForSubjectParams{
		RecipientID:      recipientID,
		NotificationType: nType,
		SubjectType:      &subjectType,
		SubjectID:        subjectID,
	})
	if err != nil {
		return false, fmt.Errorf("checking unread notification: %w", err)
	}

	return has, nil
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

// upsertNotificationPreferencesTx applies a bulk preference update inside one
// transaction — a mid-list failure must not leave a partial save (BUG-014).
func (s *Store) upsertNotificationPreferencesTx(ctx context.Context, params []gen.UpsertNotificationPreferenceParams) error {
	if s.writePool == nil {
		return fmt.Errorf("upserting notification preferences: write pool not configured")
	}

	tx, err := s.writePool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("beginning preferences tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.write.WithTx(tx)
	for _, p := range params {
		if err := qtx.UpsertNotificationPreference(ctx, p); err != nil {
			return fmt.Errorf("upserting notification preference %q: %w", p.NotificationType, err)
		}
	}

	return tx.Commit(ctx)
}

// isBlocked reports whether either user has blocked the other (§41).
// Used to suppress notifications from blocked actors.
func (s *Store) isBlocked(ctx context.Context, userA, userB pgtype.UUID) (bool, error) {
	return s.read.IsBlocked(ctx, gen.IsBlockedParams{BlockerID: userA, BlockedID: userB})
}

// getOpenBatchWindow returns an open, non-dispatched batch window.
func (s *Store) getOpenBatchWindow(ctx context.Context, params gen.GetOpenBatchWindowParams) (gen.GetOpenBatchWindowRow, error) {
	window, err := s.read.GetOpenBatchWindow(ctx, params)
	if err != nil {
		return gen.GetOpenBatchWindowRow{}, fmt.Errorf("getting open batch window: %w", err)
	}

	return window, nil
}

// upsertBatchWindow creates a batch window or resets/extends an existing one.
// BUG-002: the UNIQUE constraint previously made post-dispatch re-batching
// impossible. The upsert resets dispatched/expired windows and extends open
// ones; returns the window id + close_at for the dispatch enqueue.
func (s *Store) upsertBatchWindow(ctx context.Context, params gen.UpsertBatchWindowParams) (gen.UpsertBatchWindowRow, error) {
	row, err := s.write.UpsertBatchWindow(ctx, params)
	if err != nil {
		return gen.UpsertBatchWindowRow{}, fmt.Errorf("upserting batch window: %w", err)
	}

	return row, nil
}

// dispatchWindowTx performs the batch dispatch inside a single transaction:
// row-lock the window, skip when already dispatched, insert the aggregated
// notification, mark dispatched, commit. BUG-018: previously insert and
// mark-dispatched were separate statements, so a failure between them left a
// window that would either retry-duplicate the notification or wedge.
// Returns the new notification id, the window row, and whether this call
// claimed the window (false when another worker already dispatched it).
func (s *Store) dispatchWindowTx(ctx context.Context, windowID pgtype.UUID, insert gen.InsertNotificationParams) (pgtype.UUID, gen.NotificationBatchWindow, bool, error) {
	if s.writePool == nil {
		return pgtype.UUID{}, gen.NotificationBatchWindow{}, false, fmt.Errorf("dispatching batch window: write pool not configured")
	}

	tx, err := s.writePool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return pgtype.UUID{}, gen.NotificationBatchWindow{}, false, fmt.Errorf("beginning dispatch tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.write.WithTx(tx)

	window, err := qtx.GetBatchWindowForUpdate(ctx, windowID)
	if err != nil {
		return pgtype.UUID{}, gen.NotificationBatchWindow{}, false, fmt.Errorf("locking batch window: %w", err)
	}

	if window.Dispatched {
		return pgtype.UUID{}, window, false, nil
	}

	notifID, err := qtx.InsertNotification(ctx, insert)
	if err != nil {
		return pgtype.UUID{}, window, false, fmt.Errorf("inserting batch notification: %w", err)
	}

	if err := qtx.MarkBatchWindowDispatched(ctx, windowID); err != nil {
		return pgtype.UUID{}, window, false, fmt.Errorf("marking batch window dispatched: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return pgtype.UUID{}, window, false, fmt.Errorf("committing dispatch tx: %w", err)
	}

	return notifID, window, true, nil
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
func (s *Store) getBatchWindowByID(ctx context.Context, windowID pgtype.UUID) (gen.NotificationBatchWindow, error) {
	window, err := s.read.GetBatchWindowByID(ctx, windowID)
	if err != nil {
		return gen.NotificationBatchWindow{}, fmt.Errorf("getting batch window by ID: %w", err)
	}

	return window, nil
}

// toNotificationType maps a query row to the table model shape.
func toNotificationType(r gen.GetNotificationTypesRow) gen.NotificationType {
	return gen.NotificationType{
		Key:          r.Key,
		Label:        r.Label,
		Description:  r.Description,
		Category:     r.Category,
		Enabled:      r.Enabled,
		DefaultInApp: r.DefaultInApp,
		DefaultEmail: r.DefaultEmail,
		DefaultPush:  r.DefaultPush,
	}
}

// getNotificationTypes returns all configured notification types.
func (s *Store) getNotificationTypes(ctx context.Context) ([]gen.NotificationType, error) {
	rows, err := s.read.GetNotificationTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting notification types: %w", err)
	}

	types := make([]gen.NotificationType, len(rows))
	for i, r := range rows {
		types[i] = toNotificationType(r)
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

	types := make([]gen.NotificationType, len(rows))
	for i, r := range rows {
		types[i] = toNotificationType(gen.GetNotificationTypesRow(r))
	}

	return types, nil
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

// createNotificationType inserts a notification type. A duplicate key
// surfaces as ErrNotificationTypeExists (SQLSTATE 23505 → 409 at the
// handler) instead of silently overwriting the row.
func (s *Store) createNotificationType(ctx context.Context, p gen.CreateNotificationTypeParams) error {
	if err := s.write.CreateNotificationType(ctx, p); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrNotificationTypeExists
		}

		return fmt.Errorf("creating notification type: %w", err)
	}

	return nil
}

// getEnabledNotificationTypes returns enabled types only — the member-facing
// /me/notification-types contract (BUG-015).
func (s *Store) getEnabledNotificationTypes(ctx context.Context) ([]gen.NotificationType, error) {
	rows, err := s.read.GetEnabledNotificationTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting enabled notification types: %w", err)
	}

	types := make([]gen.NotificationType, len(rows))
	for i, r := range rows {
		types[i] = toNotificationType(gen.GetNotificationTypesRow(r))
	}

	return types, nil
}

// updateNotificationType updates an existing notification type definition.
// BUG-019: returns ErrNotificationTypeNotFound when the key does not exist
// instead of silently succeeding on 0 affected rows.
func (s *Store) updateNotificationType(ctx context.Context, p gen.UpdateNotificationTypeParams) error {
	n, err := s.write.UpdateNotificationType(ctx, p)
	if err != nil {
		return fmt.Errorf("updating notification type: %w", err)
	}

	if n == 0 {
		return ErrNotificationTypeNotFound
	}

	return nil
}

// deleteNotificationType disables a notification type (§35: DELETE = disable).
// BUG-007/019: ErrNotificationTypeNotFound when the key does not exist.
func (s *Store) deleteNotificationType(ctx context.Context, key string) error {
	n, err := s.write.DeleteNotificationType(ctx, key)
	if err != nil {
		return fmt.Errorf("disabling notification type: %w", err)
	}

	if n == 0 {
		return ErrNotificationTypeNotFound
	}

	return nil
}

// getUserDisplayName returns a user's display_name (nil when unset).
// BUG-003: previously passed a zero UUID as , which caused
// GetUserProfile to return ErrNoRows for every user. Now requires the
// correct so the platform-scoped WHERE clause matches.
func (s *Store) getUserDisplayName(ctx context.Context, userID pgtype.UUID) (*string, error) {
	user, err := s.read.GetUserProfile(ctx,

		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting user profile: %w", err)
	}

	return user.DisplayName, nil
}

// getUserEmail returns the user's email address for notification email
// delivery (§35, BUG-003). Platform-scoped per §04.
func (s *Store) getUserEmail(ctx context.Context, userID pgtype.UUID) (*string, error) {
	email, err := s.read.GetUserEmail(ctx,

		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting user email: %w", err)
	}

	return &email, nil
}
