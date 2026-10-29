package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
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
// 3. If no window: upsert a window — the UNIQUE constraint resets dispatched
//    or expired windows into a fresh batch (BUG-002) instead of erroring.
// subjectTitle/subjectURL ride on the window so DispatchBatch can build a
// deep-linkable notification row (BUG-009).
// Returns whether a new notification was created or batch was extended.
func (s *Service) DispatchNotification(ctx context.Context, recipientID pgtype.UUID, nType, subjectTitle string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, subjectURL, _, _ string) (DispatchResult, error) {
	// §41 (BLK-008): a user block in either direction suppresses the
	// notification entirely — the blocked party's actions must not reach
	// the other side. Central check so every dispatch path (comments,
	// reactions, follows, shares) inherits it. Best-effort: on lookup
	// failure the notification proceeds rather than being dropped.
	if actorID.Valid {
		if blocked, err := s.store.isBlocked(ctx, recipientID, actorID); err == nil && blocked {
			return DispatchResult{Action: "skipped"}, nil
		}
	}

	// Check effective preferences (user override OR type default).
	// BUG-001: previously only checked explicit user preferences, ignoring
	// notification_type defaults. A type with default_in_app=false would
	// still create a notification if the user had no saved preference.
	prefs, err := s.resolveEffectivePreferences(ctx, recipientID)
	if err == nil {
		// Only skip when every channel is off — a user with in-app disabled
		// but push/email enabled must still get the batch so DispatchBatch
		// can deliver on those channels (ALG-006).
		// A disabled type suppresses delivery entirely (spec §35 "disable
		// type"); otherwise skip only when every channel is off.
		if ep, ok := prefs[nType]; ok && (ep.Disabled || (!ep.InApp && !ep.Push && !ep.Email)) {
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

	// Create or reset the window. The upsert handles the UNIQUE constraint:
	// a dispatched/expired row resets into a fresh batch; a still-open row
	// (concurrent-create race) extends instead (BUG-002/008).
	// AUDIT-FRESH-SETTING-007: batch window is configurable via system_settings.
	batchWindowMinutes := 5
	if s.settings != nil {
		batchWindowMinutes = s.settings.GetIntOr(ctx, "notifications.batch_window_minutes", batchWindowMinutes)
	}
	row, err := s.store.upsertBatchWindow(ctx, gen.UpsertBatchWindowParams{
		RecipientID:      recipientID,
		NotificationType: nType,
		SubjectType:      subjectType,
		SubjectID:        subjectID,
		ActorID:          actorID,
		WindowMinutes:    int32(batchWindowMinutes), // #nosec
		SubjectTitle:     stringPtr(subjectTitle),
		ActionUrl:        stringPtr(subjectURL),
	})
	if err != nil {
		return DispatchResult{}, fmt.Errorf("creating batch window: %w", err)
	}

	// Enqueue the dispatch batch task to fire at window_close_at.
	s.enqueueBatchDispatch(row.ID, row.WindowCloseAt)

	return DispatchResult{Action: "created", WindowID: row.ID}, nil
}

// enqueueBatchDispatch schedules a notification:dispatch_batch Asynq task
// to fire when the batch window closes.
func (s *Service) enqueueBatchDispatch(windowID pgtype.UUID, closeAt pgtype.Timestamptz) {
	if s.client == nil {
		return
	}

	payload, err := json.Marshal(NotificationDispatchPayload{
		WindowID: windowID.String(),
	})
	if err != nil {
		slog.Error("notifications: failed to marshal dispatch_batch payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypeNotificationDispatchBatch, payload)
	opts := []asynq.Option{
		asynq.Queue(worker.QueueForTask(worker.TypeNotificationDispatchBatch)),
		asynq.ProcessAt(closeAt.Time),
	}

	if _, err := s.client.Enqueue(task, opts...); err != nil {
		slog.Error("notifications: failed to enqueue dispatch_batch",
			"window_id", windowID.String(), "error", err)
	}
}

// DispatchBatch processes a completed batch window.
// Claims the window inside one transaction (row lock → insert → mark
// dispatched → commit — BUG-018), builds the aggregated notification
// (e.g. "Alice and 9 others reacted" — now carrying the subject title and
// deep link stored on the window, BUG-009), broadcasts the live WS event,
// and enqueues push/email delivery if the recipient's preferences allow.
func (s *Service) DispatchBatch(ctx context.Context, windowID pgtype.UUID) error {
	// Fast pre-check: already-dispatched windows exit before opening a tx.
	window, err := s.store.getBatchWindowByID(ctx, windowID)
	if err != nil {
		return fmt.Errorf("reading batch window: %w", err)
	}

	if window.Dispatched {
		return nil
	}

	// The window was reset into a fresh batch (BUG-002 upsert) while an older
	// queued task was still pending — a stale task must not dispatch the new
	// batch early. Re-enqueue for the real close time and exit.
	if window.WindowCloseAt.Valid && window.WindowCloseAt.Time.After(time.Now()) {
		s.enqueueBatchDispatch(windowID, window.WindowCloseAt)
		return nil
	}

	action := notificationAction(window.NotificationType)

	// Get the last actor's display name for the batch title.
	// BUG-003: pass the batch window's so the profile lookup
	// resolves correctly (previously always returned "Someone").
	actorName := s.resolveActorDisplayName(ctx, window.LastActorID)

	title := BuildBatchTitle(actorName, window.ActorCount, action)

	// BUG-009: the aggregated row carries the subject title as its body and
	// the stored deep link as action_url so the notification is clickable.
	body := ""
	if window.SubjectTitle != nil {
		body = *window.SubjectTitle
	}

	actionURL := ""
	if window.ActionUrl != nil {
		actionURL = *window.ActionUrl
	}

	// BUG-004: re-check the effective in-app preference before inserting.
	// The user may have disabled in-app during the batch window.
	prefs, prefErr := s.resolveEffectivePreferences(ctx, window.RecipientID)
	if prefErr == nil {
		if ep, ok := prefs[window.NotificationType]; ok && ep.Disabled {
			// Type disabled — suppress the whole batch, still mark the window.
			if err := s.store.markBatchWindowDispatched(ctx, windowID); err != nil {
				return fmt.Errorf("marking batch window dispatched: %w", err)
			}

			return nil
		}

		if ep, ok := prefs[window.NotificationType]; ok && !ep.InApp {
			if err := s.store.markBatchWindowDispatched(ctx, windowID); err != nil {
				return fmt.Errorf("marking batch window dispatched: %w", err)
			}
			// Still honour push/email if the user enabled them.
			if ep.Push {
				s.enqueuePushDeliveryWithCtx(ctx, window.RecipientID, title, body, actionURL, windowID.String())
			}

			if ep.Email {
				s.enqueueEmailDelivery(ctx, window.RecipientID, window.NotificationType, title, body, actionURL)
			}

			return nil
		}
	}

	// BUG-018: insert + mark-dispatched run inside one transaction — a
	// failure rolls both back so an Asynq retry can never double-insert.
	notifID, window, claimed, err := s.store.dispatchWindowTx(ctx, windowID, gen.InsertNotificationParams{
		RecipientID:      window.RecipientID,
		NotificationType: window.NotificationType,
		ActorType:        stringPtr(ActorTypeUser),
		ActorID:          window.LastActorID,
		SubjectType:      stringPtr(window.SubjectType),
		SubjectID:        window.SubjectID,
		Title:            title,
		Body:             stringPtr(body),
		ActionUrl:        stringPtr(actionURL),
	})
	if err != nil {
		return fmt.Errorf("dispatching batch window: %w", err)
	}

	if !claimed {
		return nil
	}

	// BUG-002: broadcast the new notification via WebSocket so the client
	// sees it without a manual refresh. Ref: §35 L88, §11.
	// BUG-005: include the real notification ID so the client can mark-as-read
	// directly from the realtime push without a refetch.
	payload := map[string]any{
		"id":                notifID,
		"notification_type": window.NotificationType,
		"actor_type":        ActorTypeUser,
		"actor_id":          window.LastActorID,
		"subject_type":      window.SubjectType,
		"subject_id":        window.SubjectID,
		"title":             title,
		"body":              body,
		"action_url":        actionURL,
		"created_at":        time.Now().UTC().Format(time.RFC3339),
	}
	if label := s.notificationTypeLabel(ctx, window.NotificationType); label != "" {
		payload["type_label"] = label
	}

	s.broadcastNotification(ctx, window.RecipientID, payload)

	// BUG-013: invalidate the unread count cache so the badge updates.
	s.invalidateUnreadCount(ctx, window.RecipientID)

	// BUG-007: use the already-fetched effective preferences to check
	// push/email instead of re-querying the DB in checkAndEnqueuePush/Email.
	if prefErr == nil {
		if ep, ok := prefs[window.NotificationType]; ok {
			if ep.Push {
				s.enqueuePushDeliveryWithCtx(ctx, window.RecipientID, title, body, actionURL, notifID.String())
			}

			if ep.Email {
				s.enqueueEmailDelivery(ctx, window.RecipientID, window.NotificationType, title, body, actionURL)
			}

			return nil
		}
	}

	// Fallback if preference resolution failed — use the old per-call checks.
	s.checkAndEnqueuePush(ctx, window.RecipientID, window.NotificationType, title, body, actionURL, notifID.String())
	s.checkAndEnqueueEmail(ctx, window.RecipientID, window.NotificationType, title, body, actionURL)

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
// The in-app row honours the recipient's in_app preference — a user who
// disabled in-app for this type gets push/email only, consistent with the
// batch path (BUG-005 class). Push delivery tasks carry the notification id
// so asynq.Unique only collapses true redeliveries (BUG-012).
func (s *Service) CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error {
	// §41 (BLK-008): same suppression as DispatchNotification on the
	// non-batch path — user-attributed notifications from a blocked actor
	// never reach the recipient.
	if actorID.Valid {
		if blocked, err := s.store.isBlocked(ctx, recipientID, actorID); err == nil && blocked {
			return nil
		}
	}

	// Resolve effective preferences once for the in-app gate + push/email.
	prefs, prefErr := s.resolveEffectivePreferences(ctx, recipientID)
	ep, havePref := prefs[nType]

	if prefErr == nil && havePref && ep.Disabled {
		// Type disabled — suppress all delivery.
		return nil
	}

	if prefErr == nil && havePref && !ep.InApp {
		// in-app off — no row, but push/email still honoured.
		if ep.Push {
			s.enqueuePushDeliveryWithCtx(ctx, recipientID, title, body, actionURL, "")
		}

		if ep.Email {
			s.enqueueEmailDelivery(ctx, recipientID, nType, title, body, actionURL)
		}

		return nil
	}

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
	payload := map[string]any{
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
	}
	if label := s.notificationTypeLabel(ctx, nType); label != "" {
		payload["type_label"] = label
	}

	s.broadcastNotification(ctx, recipientID, payload)

	// BUG-013: invalidate the unread count cache so the badge updates.
	s.invalidateUnreadCount(ctx, recipientID)

	// BUG-007: reuse the already-fetched effective preferences for both
	// push and email checks, instead of two separate DB queries.
	if prefErr == nil {
		if havePref {
			if ep.Push {
				s.enqueuePushDeliveryWithCtx(ctx, recipientID, title, body, actionURL, notifID.String())
			}

			if ep.Email {
				s.enqueueEmailDelivery(ctx, recipientID, nType, title, body, actionURL)
			}

			return nil
		}
	}

	// Fallback if preference resolution failed — use the old per-call checks.
	s.checkAndEnqueuePush(ctx, recipientID, nType, title, body, actionURL, notifID.String())
	s.checkAndEnqueueEmail(ctx, recipientID, nType, title, body, actionURL)

	return nil
}

// HasUnreadNotification reports whether an unread notification already
// exists for (recipient, type, subject). Repeat-event flows (dm.message,
// dm.request) use it to deliver push for every event while stacking at most
// one in-app row per subject until read.
func (s *Service) HasUnreadNotification(ctx context.Context, recipientID pgtype.UUID, nType, subjectType string, subjectID pgtype.UUID) (bool, error) {
	return s.store.hasUnreadNotificationForSubject(ctx, recipientID, nType, subjectType, subjectID)
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

// CountNotifications returns the total number of notifications matching the
// same unread_only filter as GetNotifications, so the list endpoint can
// report { total, has_more } instead of forcing clients to infer end-of-list
// from a short page.
func (s *Service) CountNotifications(ctx context.Context, recipientID pgtype.UUID, unreadOnly bool) (int32, error) {
	count, err := s.store.countNotificationsByRecipient(ctx, recipientID, unreadOnly)
	if err != nil {
		return 0, fmt.Errorf("counting notifications: %w", err)
	}

	return count, nil
}

// notificationTypeLabel returns the human-readable label for a notification
// type key, or "" if the type isn't registered. Best-effort — a missing
// label must never block delivery.
func (s *Service) notificationTypeLabel(ctx context.Context, nType string) string {
	label, err := s.store.getNotificationTypeLabel(ctx, nType)
	if err != nil {
		return ""
	}

	return label
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
// BUG-016: returns ErrNotificationNotFound for unknown/foreign ids.
// BUG-017: pushes the fresh unread count over WS so other sessions sync.
func (s *Service) MarkAsRead(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	if err := s.store.markNotificationRead(ctx, notificationID, recipientID); err != nil {
		return err
	}

	s.invalidateUnreadCount(ctx, recipientID)
	s.broadcastUnreadCount(ctx, recipientID)

	return nil
}

// MarkAllAsRead marks all notifications as read for a user.
func (s *Service) MarkAllAsRead(ctx context.Context, recipientID pgtype.UUID) error {
	if err := s.store.markAllNotificationsRead(ctx, recipientID); err != nil {
		return err
	}

	s.invalidateUnreadCount(ctx, recipientID)
	s.broadcastUnreadCount(ctx, recipientID)

	return nil
}

// DeleteNotification deletes a single notification.
// BUG-016: returns ErrNotificationNotFound for unknown/foreign ids.
func (s *Service) DeleteNotification(ctx context.Context, notificationID, recipientID pgtype.UUID) error {
	if err := s.store.deleteNotification(ctx, notificationID, recipientID); err != nil {
		return err
	}

	s.invalidateUnreadCount(ctx, recipientID)
	s.broadcastUnreadCount(ctx, recipientID)

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
// Disabled marks admin-disabled types (DELETE /admin/notification-types —
// spec §35 "disable type"): a disabled type must suppress ALL delivery, not
// merely lose its defaults — without this flag the disable was a no-op.
type effectivePref struct {
	InApp    bool
	Email    bool
	Push     bool
	Disabled bool
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
		// Disabled types stay in the map (flagged) so delivery paths can
		// suppress them outright; keeping their merged defaults preserves a
		// correct base for preference PATCHes while the type is off.
		ep := effectivePref{
			InApp:    nt.DefaultInApp,
			Email:    nt.DefaultEmail,
			Push:     nt.DefaultPush,
			Disabled: !nt.Enabled,
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

// UpdatePreferencePartial updates a single notification type preference,
// treating nil fields as "keep the current value". Without this, a PATCH
// body that omits a channel (e.g. {"in_app": true}) would silently reset
// the omitted channels to false — JSON zero-value decode can't distinguish
// "absent" from "explicit false".
//
// Base value resolution: the user's existing override row for the type wins;
// otherwise the notification_types effective defaults; otherwise the §35
// global defaults (in_app on, email/push off).
//
// BUG-011: an unknown notification_type now returns ErrNotificationTypeNotFound
// (404 at the handler) instead of surfacing the FK violation as a 500.
func (s *Service) UpdatePreferencePartial(ctx context.Context, userID pgtype.UUID, nType string, inApp, email, push *bool) error {
	base := effectivePref{InApp: true}

	// Existence check — an unknown key must not reach the FK-constrained
	// upsert and surface as a generic 500. Only a genuine miss maps to 404;
	// DB failures propagate as errors → 500 at the handler.
	if _, err := s.store.getNotificationTypeLabel(ctx, nType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotificationTypeNotFound
		}

		return err
	}

	// Effective prefs already merge the user's override with the type
	// defaults for enabled types.
	found := false

	if prefs, err := s.resolveEffectivePreferences(ctx, userID); err == nil {
		if ep, ok := prefs[nType]; ok {
			base = ep
			found = true
		}
	}

	// Disabled/unknown types are skipped by resolveEffectivePreferences —
	// preserve the user's existing override row verbatim in that case.
	if !found {
		if rows, err := s.store.getNotificationPreferences(ctx, userID); err == nil {
			for _, p := range rows {
				if p.NotificationType == nType {
					base = effectivePref{InApp: p.InApp, Email: p.Email, Push: p.Push}
					break
				}
			}
		}
	}

	if inApp != nil {
		base.InApp = *inApp
	}

	if email != nil {
		base.Email = *email
	}

	if push != nil {
		base.Push = *push
	}

	return s.UpdatePreference(ctx, userID, nType, base.InApp, base.Email, base.Push)
}

// PreferencePatch is one item of a bulk preference update — nil channels
// keep their current value (same partial-PATCH semantics as
// UpdatePreferencePartial).
type PreferencePatch struct {
	Type  string
	InApp *bool
	Email *bool
	Push  *bool
}

// UpdatePreferencesBulk applies a bulk preference update inside one
// transaction (BUG-014) after validating every type key up front
// (BUG-011 — the first unknown key returns ErrNotificationTypeNotFound and
// nothing is written).
func (s *Service) UpdatePreferencesBulk(ctx context.Context, userID pgtype.UUID, items []PreferencePatch) error {
	resolved := make([]gen.UpsertNotificationPreferenceParams, 0, len(items))

	// Hoist the effective-preference snapshot — one read for the whole batch
	// instead of N.
	prefs, _ := s.resolveEffectivePreferences(ctx, userID)

	for _, p := range items {
		if _, err := s.store.getNotificationTypeLabel(ctx, p.Type); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotificationTypeNotFound
			}

			return err
		}

		base := effectivePref{InApp: true}

		if ep, ok := prefs[p.Type]; ok {
			base = ep
		}

		if p.InApp != nil {
			base.InApp = *p.InApp
		}

		if p.Email != nil {
			base.Email = *p.Email
		}

		if p.Push != nil {
			base.Push = *p.Push
		}

		resolved = append(resolved, gen.UpsertNotificationPreferenceParams{
			UserID:           userID,
			NotificationType: p.Type,
			InApp:            base.InApp,
			Email:            base.Email,
			Push:             base.Push,
		})
	}

	return s.store.upsertNotificationPreferencesTx(ctx, resolved)
}

// GetNotificationTypes returns all configured notification types.
func (s *Service) GetNotificationTypes(ctx context.Context) ([]gen.NotificationType, error) {
	return s.store.getNotificationTypes(ctx)
}

// GetEnabledNotificationTypes returns enabled types only — the contract for
// the member-facing GET /me/notification-types (BUG-015).
func (s *Service) GetEnabledNotificationTypes(ctx context.Context) ([]gen.NotificationType, error) {
	return s.store.getEnabledNotificationTypes(ctx)
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
	return s.store.createNotificationType(ctx, gen.CreateNotificationTypeParams{
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
