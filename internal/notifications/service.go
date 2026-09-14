package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// NotificationInfo extends a notification row with the type label.
type NotificationInfo struct {
	gen.GetNotificationsByRecipientRow
}

// PreferenceInfo extends a preference row with the type label.
type PreferenceInfo struct {
	gen.GetNotificationPreferencesRow
}

// PreferenceOutput is the API response shape for a single notification
// preference. It merges the notification_type defaults with the user's
// overrides so the frontend always sees every available type — even when
// the user has never saved a preference for it (Section 35).
type PreferenceOutput struct {
	Type        string  `json:"type"`
	Label       string  `json:"label"`
	Category    string  `json:"category"`
	Description *string `json:"description"`
	InApp       bool    `json:"in_app"`
	Email       bool    `json:"email"`
	Push        bool    `json:"push"`
}

// DispatchResult describes the outcome of a dispatch attempt.
type DispatchResult struct {
	Action   string      `json:"action"` // "created", "extended", or "immediate"
	WindowID pgtype.UUID `json:"window_id"`
}

// DispatchNotification handles the batch dedup logic for a notification event.
// 1. Check for open batch window for (recipient, type, subject)
// 2. If window exists and not dispatched: extend it (add actor)
// 3. If no window: create new window
// Returns whether a new notification was created or batch was extended.
func (s *Service) DispatchNotification(ctx context.Context, recipientID pgtype.UUID, nType, _ string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, _, _, _ string) (DispatchResult, error) {
	// Check effective preferences (user override OR type default).
	// BUG-001: previously only checked explicit user preferences, ignoring
	// notification_type defaults. A type with default_in_app=false would
	// still create a notification if the user had no saved preference.
	prefs, err := s.resolveEffectivePreferences(ctx, recipientID)
	if err == nil {
		if ep, ok := prefs[nType]; ok && !ep.InApp {
			return DispatchResult{Action: "skipped"}, nil
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
	// AUDIT-FRESH-SETTING-007: batch window is configurable via system_settings.
	batchWindowMinutes := 5
	if s.settings != nil {
		batchWindowMinutes = s.settings.GetIntOr(ctx, "notifications.batch_window_minutes", batchWindowMinutes)
	}
	// BUG-001: pass so the batch window carries tenant context
	// through to DispatchBatch, which needs it for the notifications insert.
	if err := s.store.createBatchWindow(ctx, gen.CreateBatchWindowParams{

		RecipientID:      recipientID,
		NotificationType: nType,
		SubjectType:      subjectType,
		SubjectID:        subjectID,
		ActorID:          actorID,
		WindowMinutes:    int32(batchWindowMinutes), // #nosec
	}); err != nil {
		return DispatchResult{}, fmt.Errorf("creating batch window: %w", err)
	}

	// Retrieve the newly created window to get its ID and close_at.
	window, err = s.store.getOpenBatchWindow(ctx, gen.GetOpenBatchWindowParams{
		RecipientID:      recipientID,
		NotificationType: nType,
		SubjectType:      subjectType,
		SubjectID:        subjectID,
	})
	if err != nil {
		return DispatchResult{}, fmt.Errorf("retrieving new batch window: %w", err)
	}

	// Enqueue the dispatch batch task to fire at window_close_at.
	s.enqueueBatchDispatch(window)

	return DispatchResult{Action: "created", WindowID: window.ID}, nil
}

// enqueueBatchDispatch schedules a notification:dispatch_batch Asynq task
// to fire when the batch window closes.
func (s *Service) enqueueBatchDispatch(window gen.NotificationBatchWindow) {
	if s.client == nil {
		return
	}

	payload, err := json.Marshal(NotificationDispatchPayload{
		WindowID: window.ID.String(),
	})
	if err != nil {
		slog.Error("notifications: failed to marshal dispatch_batch payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypeNotificationDispatchBatch, payload)
	opts := []asynq.Option{
		asynq.Queue(worker.QueueForTask(worker.TypeNotificationDispatchBatch)),
		asynq.ProcessAt(window.WindowCloseAt.Time),
	}

	if _, err := s.client.Enqueue(task, opts...); err != nil {
		slog.Error("notifications: failed to enqueue dispatch_batch",
			"window_id", window.ID.String(), "error", err)
	}
}

// DispatchBatch processes a completed batch window.
// Reads the window, resolves actor display names, builds the aggregated notification
// (e.g. "Alice and 9 others reacted"), inserts the row, marks the window dispatched,
// and enqueues push/email delivery if the recipient's preferences allow.
func (s *Service) DispatchBatch(ctx context.Context, windowID pgtype.UUID) error {
	window, err := s.store.getBatchWindowByID(ctx, windowID)
	if err != nil {
		return fmt.Errorf("reading batch window: %w", err)
	}

	if window.Dispatched {
		return nil
	}

	action := notificationAction(window.NotificationType)

	// Get the last actor's display name for the batch title.
	// BUG-003: pass the batch window's so the profile lookup
	// resolves correctly (previously always returned "Someone").
	actorName := s.resolveActorDisplayName(ctx, window.LastActorID)

	title := BuildBatchTitle(actorName, window.ActorCount, action)

	// BUG-004: re-check the effective in-app preference before inserting.
	// The user may have disabled in-app during the 5-minute batch window.
	// If in-app is off, skip the insert but still mark the window dispatched
	// so the Asynq task does not retry. Push/email preferences are still
	// honoured below if the user has them enabled.
	prefs, prefErr := s.resolveEffectivePreferences(ctx, window.RecipientID)
	if prefErr == nil {
		if ep, ok := prefs[window.NotificationType]; ok && !ep.InApp {
			if err := s.store.markBatchWindowDispatched(ctx, windowID); err != nil {
				return fmt.Errorf("marking batch window dispatched: %w", err)
			}
			// Still honour push/email if the user enabled them.
			if ep.Push {
				s.enqueuePushDeliveryWithCtx(ctx, window.RecipientID, title, "", "")
			}

			if ep.Email {
				s.enqueueEmailDelivery(ctx, window.RecipientID, window.NotificationType, title, "", "")
			}

			return nil
		}
	}

	// Insert the aggregated notification.
	// BUG-001: use the batch window's (previously zero UUID, which
	// violated the NOT NULL FK constraint on notifications.).
	// BUG-005: capture the returned ID for the WebSocket broadcast.
	var bodyPtr *string

	var actionURLPtr *string

	notifID, err := s.store.insertNotification(ctx, gen.InsertNotificationParams{

		RecipientID:      window.RecipientID,
		NotificationType: window.NotificationType,
		ActorType:        stringPtr(ActorTypeUser),
		ActorID:          window.LastActorID,
		SubjectType:      stringPtr(window.SubjectType),
		SubjectID:        window.SubjectID,
		Title:            title,
		Body:             bodyPtr,
		ActionUrl:        actionURLPtr,
	})
	if err != nil {
		return fmt.Errorf("inserting batch notification: %w", err)
	}

	// Mark window as dispatched.
	if err := s.store.markBatchWindowDispatched(ctx, windowID); err != nil {
		return fmt.Errorf("marking batch window dispatched: %w", err)
	}

	// BUG-002: broadcast the new notification via WebSocket so the client
	// sees it without a manual refresh. Ref: §35 L88, §11.
	// BUG-005: include the real notification ID so the client can mark-as-read
	// directly from the realtime push without a refetch.
	s.broadcastNotification(window.RecipientID, map[string]any{
		"id":                notifID,
		"notification_type": window.NotificationType,
		"actor_type":        ActorTypeUser,
		"actor_id":          window.LastActorID,
		"subject_type":      window.SubjectType,
		"subject_id":        window.SubjectID,
		"title":             title,
		"body":              "",
		"action_url":        "",
		"created_at":        time.Now().UTC().Format(time.RFC3339),
	})

	// BUG-013: invalidate the unread count cache so the badge updates.
	s.invalidateUnreadCount(ctx, window.RecipientID)

	// BUG-007: use the already-fetched effective preferences to check
	// push/email instead of re-querying the DB in checkAndEnqueuePush/Email.
	if prefErr == nil {
		if ep, ok := prefs[window.NotificationType]; ok {
			if ep.Push {
				s.enqueuePushDeliveryWithCtx(ctx, window.RecipientID, title, "", "")
			}

			if ep.Email {
				s.enqueueEmailDelivery(ctx, window.RecipientID, window.NotificationType, title, "", "")
			}

			return nil
		}
	}

	// Fallback if preference resolution failed — use the old per-call checks.
	s.checkAndEnqueuePush(ctx, window.RecipientID, window.NotificationType, title, "", "")
	s.checkAndEnqueueEmail(ctx, window.RecipientID, window.NotificationType, title, "", "")

	return nil
}

// notificationAction maps notification types to human-readable action verbs.
func notificationAction(ntype string) string {
	switch ntype {
	case TypeContentReaction:
		return "reacted"
	case TypeContentComment:
		return "commented"
	case TypeMentionContent, TypeMentionComment:
		return "mentioned you"
	case TypeNewFollower:
		return "followed you"
	case TypeCommentReply:
		return "replied"
	case TypeDMMessage:
		return "sent a message"
	default:
		return "interacted"
	}
}

// resolveActorDisplayName returns the display_name of a user, or "Someone" on
// error. Routed through the store rather than constructing gen.Queries inline.
// BUG-003: previously passed a zero UUID as , which caused
// GetUserProfile to return ErrNoRows for every user — batch notification
// titles were always "Someone and N others reacted".
func (s *Service) resolveActorDisplayName(ctx context.Context, userID pgtype.UUID) string {
	name, err := s.store.getUserDisplayName(ctx, userID)
	if err != nil || name == nil || *name == "" {
		return "Someone"
	}

	return *name
}

// stringPtr returns a pointer to s, or nil if s is empty.
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// CreateNotification directly creates a single notification (non-batch path).
// If the recipient has push enabled for this type, a push delivery task is enqueued.
func (s *Service) CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error {
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

	// BUG-005: capture the returned ID for the WebSocket broadcast.
	notifID, err := s.store.insertNotification(ctx, gen.InsertNotificationParams{

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
	if err != nil {
		return fmt.Errorf("inserting notification: %w", err)
	}

	// BUG-002: broadcast the new notification via WebSocket so the client
	// sees it without a manual refresh. Ref: §35 L88, §11.
	// BUG-005: include the real notification ID so the client can mark-as-read
	// directly from the realtime push without a refetch.
	s.broadcastNotification(recipientID, map[string]any{
		"id":                notifID,
		"notification_type": nType,
		"actor_type":        actorType,
		"actor_id":          actorID,
		"subject_type":      subjectType,
		"subject_id":        subjectID,
		"title":             title,
		"body":              body,
		"action_url":        actionURL,
		"created_at":        time.Now().UTC().Format(time.RFC3339),
	})

	// BUG-013: invalidate the unread count cache so the badge updates.
	s.invalidateUnreadCount(ctx, recipientID)

	// BUG-007: resolve effective preferences once and use them for both
	// push and email checks, instead of two separate DB queries.
	prefs, prefErr := s.resolveEffectivePreferences(ctx, recipientID)
	if prefErr == nil {
		if ep, ok := prefs[nType]; ok {
			if ep.Push {
				s.enqueuePushDeliveryWithCtx(ctx, recipientID, title, body, actionURL)
			}

			if ep.Email {
				s.enqueueEmailDelivery(ctx, recipientID, nType, title, body, actionURL)
			}

			return nil
		}
	}

	// Fallback if preference resolution failed — use the old per-call checks.
	s.checkAndEnqueuePush(ctx, recipientID, nType, title, body, actionURL)
	s.checkAndEnqueueEmail(ctx, recipientID, nType, title, body, actionURL)

	return nil
}

// GetNotifications returns paginated notifications for a user.
// BUG-007: supports the ?unread_only=true filter (§35 L107).
func (s *Service) GetNotifications(ctx context.Context, recipientID pgtype.UUID, limit, offset int32, unreadOnly bool) ([]NotificationInfo, error) {
	rows, err := s.store.getNotificationsByRecipient(ctx, recipientID, limit, offset, unreadOnly)
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
// BUG-013: caches the count in Valkey (jc:notif_unread:{user_id}) with a
// 60s TTL to avoid hitting the DB on every badge poll.
func (s *Service) GetUnreadCount(ctx context.Context, recipientID pgtype.UUID) (int, error) {
	// Try cache first.
	if cached := s.getCachedUnreadCount(ctx, recipientID); cached >= 0 {
		return cached, nil
	}

	count, err := s.store.getUnreadCount(ctx, recipientID)
	if err != nil {
		return 0, fmt.Errorf("getting unread count: %w", err)
	}

	s.setCachedUnreadCount(ctx, recipientID, int(count))

	return int(count), nil
}

// MarkAsRead marks a single notification as read.
func (s *Service) MarkAsRead(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	if err := s.store.markNotificationRead(ctx, notificationID, recipientID); err != nil {
		return err
	}

	s.invalidateUnreadCount(ctx, recipientID)

	return nil
}

// MarkAllAsRead marks all notifications as read for a user.
func (s *Service) MarkAllAsRead(ctx context.Context, recipientID pgtype.UUID) error {
	if err := s.store.markAllNotificationsRead(ctx, recipientID); err != nil {
		return err
	}

	s.invalidateUnreadCount(ctx, recipientID)

	return nil
}

// DeleteNotification deletes a single notification.
func (s *Service) DeleteNotification(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	if err := s.store.deleteNotification(ctx, notificationID, recipientID); err != nil {
		return err
	}

	s.invalidateUnreadCount(ctx, recipientID)

	return nil
}

// GetPreferences returns all notification types merged with the user's
// overrides. Types the user has never configured fall back to the
// notification_types defaults (Section 35). This ensures the frontend
// always shows every available type — not just the ones the user has
// explicitly saved.
func (s *Service) GetPreferences(ctx context.Context, userID pgtype.UUID) ([]PreferenceOutput, error) {
	types, err := s.store.getNotificationTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting notification types: %w", err)
	}

	userPrefs, err := s.store.getNotificationPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("getting user preferences: %w", err)
	}

	// Index user overrides by type for O(1) lookup.
	overrides := make(map[string]gen.GetNotificationPreferencesRow, len(userPrefs))
	for _, p := range userPrefs {
		overrides[p.NotificationType] = p
	}

	result := make([]PreferenceOutput, 0, len(types))

	for _, nt := range types {
		if !nt.Enabled {
			continue
		}

		pref := PreferenceOutput{
			Type:        nt.Key,
			Label:       nt.Label,
			Category:    nt.Category,
			Description: nt.Description,
			InApp:       nt.DefaultInApp,
			Email:       nt.DefaultEmail,
			Push:        nt.DefaultPush,
		}
		if o, ok := overrides[nt.Key]; ok {
			pref.InApp = o.InApp
			pref.Email = o.Email
			pref.Push = o.Push
		}

		result = append(result, pref)
	}

	return result, nil
}

// effectivePref is the resolved preference for a single notification type,
// merging the user's override (if any) with the notification_type defaults.
type effectivePref struct {
	InApp bool
	Email bool
	Push  bool
}

// resolveEffectivePreferences returns a map of notification_type → effective
// preference, merging notification_types defaults with the user's overrides
// (Section 35). Types the user has never configured fall back to the type
// defaults. This is the single source of truth for dispatch/push/email
// preference checks — fixes BUG-001, BUG-002, BUG-004, BUG-007.
func (s *Service) resolveEffectivePreferences(ctx context.Context, recipientID pgtype.UUID) (map[string]effectivePref, error) {
	types, err := s.store.getNotificationTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting notification types: %w", err)
	}

	userPrefs, err := s.store.getNotificationPreferences(ctx, recipientID)
	if err != nil {
		return nil, fmt.Errorf("getting user preferences: %w", err)
	}

	overrides := make(map[string]gen.GetNotificationPreferencesRow, len(userPrefs))
	for _, p := range userPrefs {
		overrides[p.NotificationType] = p
	}

	result := make(map[string]effectivePref, len(types))

	for _, nt := range types {
		if !nt.Enabled {
			continue
		}

		ep := effectivePref{
			InApp: nt.DefaultInApp,
			Email: nt.DefaultEmail,
			Push:  nt.DefaultPush,
		}
		if o, ok := overrides[nt.Key]; ok {
			ep.InApp = o.InApp
			ep.Email = o.Email
			ep.Push = o.Push
		}

		result[nt.Key] = ep
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

// AdminListNotificationTypes returns a filtered, sorted, paginated slice of
// notification types for the admin DataTable (Section 35).
func (s *Service) AdminListNotificationTypes(ctx context.Context, p gen.AdminListNotificationTypesParams) ([]gen.NotificationType, error) {
	return s.store.adminListNotificationTypes(ctx, p)
}

// AdminCountNotificationTypes returns the total count of notification types
// matching the given filter (Section 35).
func (s *Service) AdminCountNotificationTypes(ctx context.Context, p gen.AdminCountNotificationTypesParams) (int32, error) {
	return s.store.adminCountNotificationTypes(ctx, p)
}

// BuildBatchTitle constructs an aggregated title from actor info.
// Examples: "Alice reacted", "Alice and 2 others reacted", "Alice and 9 others reacted".
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

// CreateNotificationType inserts a new notification type definition (P5.6).
// Hand-written SQL consolidated into sqlc (notifications.sql).
// Ref: japaconnect-engineering/sections/35-notification-system.md.
func (s *Service) CreateNotificationType(ctx context.Context, key, label, category string, description *string, enabled, defaultInApp, defaultEmail, defaultPush bool) error {
	return s.store.upsertNotificationType(ctx, gen.UpsertNotificationTypeParams{
		Key: key, Label: label, Category: category, Description: description, Enabled: enabled,
		DefaultInApp: defaultInApp, DefaultEmail: defaultEmail, DefaultPush: defaultPush,
	})
}

// UpdateNotificationType updates an existing notification type (P5.6).
// Ref: japaconnect-engineering/sections/35-notification-system.md.
func (s *Service) UpdateNotificationType(ctx context.Context, key, label, category string, description *string, enabled, defaultInApp, defaultEmail, defaultPush bool) error {
	return s.store.updateNotificationType(ctx, gen.UpdateNotificationTypeParams{
		Key: key, Label: label, Category: category, Description: description, Enabled: enabled,
		DefaultInApp: defaultInApp, DefaultEmail: defaultEmail, DefaultPush: defaultPush,
	})
}

// DeleteNotificationType removes a notification type (P5.6).
func (s *Service) DeleteNotificationType(ctx context.Context, key string) error {
	return s.store.deleteNotificationType(ctx, key)
}
