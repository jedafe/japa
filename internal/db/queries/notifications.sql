-- Notification system queries (Section 35).
-- In-app, email, push delivery with batch deduplication.

-- =====================
-- Notifications CRUD
-- =====================

-- name: InsertNotification :one
-- BUG-005: changed from :exec to :one returning id so the notification
-- ID is available for the WebSocket broadcast payload (§35 L88, §10).
INSERT INTO notifications (recipient_id, notification_type, actor_type, actor_id, subject_type, subject_id, title, body, action_url) VALUES (@recipient_id, @notification_type, @actor_type, @actor_id, @subject_type, @subject_id, @title, @body, @action_url)

RETURNING id;

-- name: GetNotificationsByRecipient :many
SELECT n.id, n.recipient_id, n.notification_type, n.actor_type, n.actor_id,
 n.subject_type, n.subject_id, n.title, n.body, n.action_url, n.read_at, n.created_at,
 nt.label AS type_label
FROM notifications n
JOIN notification_types nt ON nt.key = n.notification_type
WHERE n.recipient_id = @recipient_id
 AND (@unread_only::bool = false OR n.read_at IS NULL)
ORDER BY n.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetUnreadNotificationCount :one
SELECT COUNT(*)::int AS count
FROM notifications
WHERE recipient_id = @recipient_id AND read_at IS NULL;

-- name: CountNotificationsByRecipient :one
-- Total matching the same unread_only filter as GetNotificationsByRecipient,
-- so the list endpoint can return { data, total, has_more } (§35).
SELECT COUNT(*)::int AS count
FROM notifications
WHERE recipient_id = @recipient_id
 AND (@unread_only::bool = false OR read_at IS NULL);

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = now() WHERE id = @id AND recipient_id = @recipient_id;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now()
WHERE recipient_id = @recipient_id AND read_at IS NULL;

-- name: DeleteNotification :execrows
DELETE FROM notifications WHERE id = @id AND recipient_id = @recipient_id;

-- name: HasUnreadNotificationForSubject :one
-- Repeat-event dedup for DM-style flows: one unread notification per
-- (recipient, type, subject) is enough — a second row for the same
-- conversation would spam the notification centre during a live chat.
SELECT EXISTS(
 SELECT 1 FROM notifications
 WHERE recipient_id = @recipient_id
  AND notification_type = @notification_type
  AND subject_type = @subject_type
  AND subject_id = @subject_id
  AND read_at IS NULL
) AS has_unread;

-- =====================
-- Notification Preferences
-- =====================

-- name: GetNotificationPreferences :many
SELECT np.user_id, np.notification_type, np.in_app, np.email, np.push,
 nt.label AS type_label, nt.category AS type_category
FROM notification_preferences np
JOIN notification_types nt ON nt.key = np.notification_type
WHERE np.user_id = @user_id;

-- name: UpsertNotificationPreference :exec
INSERT INTO notification_preferences (user_id, notification_type, in_app, email, push) VALUES (@user_id, @notification_type, @in_app, @email, @push)

ON CONFLICT (user_id, notification_type) DO UPDATE
SET in_app = EXCLUDED.in_app, email = EXCLUDED.email, push = EXCLUDED.push;

-- =====================
-- Batch Dedup Windows
-- =====================

-- name: GetOpenBatchWindow :one
SELECT id, recipient_id, notification_type, subject_type, subject_id,
 actor_ids, actor_count, last_actor_id, window_open_at, window_close_at, dispatched
FROM notification_batch_windows
WHERE recipient_id = @recipient_id
 AND notification_type = @notification_type
 AND subject_type = @subject_type
 AND subject_id = @subject_id
 AND dispatched = false
 AND window_close_at > now();

-- name: UpsertBatchWindow :one
-- BUG-002: UNIQUE(recipient_id, notification_type, subject_type, subject_id)
-- previously made a second batch impossible — after the first window
-- dispatched, every later event for the same subject hit a unique violation
-- and was dropped. On conflict we now:
--   * dispatched or expired window → reset it into a fresh batch
--     (actor list rebuilt, close_at re-scheduled, subject fields updated)
--   * still-open window (concurrent-create race) → extend it, counting each
--     actor once (BUG-008: array_append previously double-counted repeats)
INSERT INTO notification_batch_windows (recipient_id, notification_type, subject_type, subject_id, actor_ids, actor_count, last_actor_id, window_close_at, subject_title, action_url)
VALUES (@recipient_id, @notification_type, @subject_type, @subject_id, ARRAY[@actor_id::uuid], 1, @actor_id, now() + make_interval(mins => @window_minutes), @subject_title, @action_url)
ON CONFLICT (recipient_id, notification_type, subject_type, subject_id) DO UPDATE SET
 actor_ids = CASE
   WHEN notification_batch_windows.dispatched OR notification_batch_windows.window_close_at <= now() THEN ARRAY[EXCLUDED.last_actor_id]
   WHEN notification_batch_windows.actor_ids @> ARRAY[EXCLUDED.last_actor_id] THEN notification_batch_windows.actor_ids
   ELSE array_append(notification_batch_windows.actor_ids, EXCLUDED.last_actor_id)
 END,
 actor_count = CASE
   WHEN notification_batch_windows.dispatched OR notification_batch_windows.window_close_at <= now() THEN 1
   WHEN notification_batch_windows.actor_ids @> ARRAY[EXCLUDED.last_actor_id] THEN notification_batch_windows.actor_count
   ELSE notification_batch_windows.actor_count + 1
 END,
 last_actor_id = EXCLUDED.last_actor_id,
 subject_title = COALESCE(EXCLUDED.subject_title, notification_batch_windows.subject_title),
 action_url = COALESCE(EXCLUDED.action_url, notification_batch_windows.action_url),
 dispatched = CASE
   WHEN notification_batch_windows.dispatched OR notification_batch_windows.window_close_at <= now() THEN false
   ELSE notification_batch_windows.dispatched
 END,
 window_close_at = CASE
   WHEN notification_batch_windows.dispatched OR notification_batch_windows.window_close_at <= now() THEN EXCLUDED.window_close_at
   ELSE notification_batch_windows.window_close_at
 END
RETURNING id, window_close_at;

-- name: ExtendBatchWindow :exec
-- BUG-008: guard against counting the same actor twice — a repeat reaction
-- from one user must not read "Alice and 1 other reacted".
UPDATE notification_batch_windows
SET actor_ids = CASE WHEN @actor_id::uuid = ANY(actor_ids) THEN actor_ids ELSE array_append(actor_ids, @actor_id) END,
 actor_count = actor_count + CASE WHEN @actor_id::uuid = ANY(actor_ids) THEN 0 ELSE 1 END,
 last_actor_id = @actor_id
WHERE id = @id;

-- name: MarkBatchWindowDispatched :exec
UPDATE notification_batch_windows SET dispatched = true WHERE id = @id;

-- name: GetBatchWindowByID :one
SELECT id, recipient_id, notification_type, subject_type, subject_id,
 actor_ids, actor_count, last_actor_id, window_open_at, window_close_at, dispatched,
 subject_title, action_url
FROM notification_batch_windows
WHERE id = @id;

-- name: GetBatchWindowForUpdate :one
-- BUG-018: row lock for the transactional claim+insert in DispatchBatch, so a
-- retry can never double-insert the notification row.
SELECT id, recipient_id, notification_type, subject_type, subject_id,
 actor_ids, actor_count, last_actor_id, window_open_at, window_close_at, dispatched,
 subject_title, action_url
FROM notification_batch_windows
WHERE id = @id
FOR UPDATE;

-- =====================
-- Notification Types
-- =====================

-- name: GetNotificationTypes :many
SELECT key, label, description, category, enabled, default_in_app, default_email, default_push
FROM notification_types ORDER BY key;

-- name: GetEnabledNotificationTypes :many
-- BUG-015: the member-facing /me/notification-types endpoint must not leak
-- types an admin has disabled (the admin list intentionally returns all).
SELECT key, label, description, category, enabled, default_in_app, default_email, default_push
FROM notification_types WHERE enabled = true ORDER BY key;

-- name: GetNotificationTypeLabel :one
-- Label lookup for a single type — used to enrich WebSocket `notification`
-- payloads with type_label so clients don't fall back to a generic label.
SELECT label FROM notification_types WHERE key = @key;

-- name: AdminListNotificationTypes :many
-- Admin list with offset pagination + search + enabled filter + sort (Section 35).
SELECT key, label, description, category, enabled, default_in_app, default_email, default_push
FROM notification_types
WHERE (@q::text = '' OR key ILIKE '%' || @q || '%' OR label ILIKE '%' || @q || '%' OR description ILIKE '%' || @q || '%')
 AND (@enabled_filter::int = 0 OR enabled = (@enabled_filter::int = 1))
ORDER BY
 CASE WHEN @sort::text = 'key' AND @dir::text = 'asc' THEN key END ASC,
 CASE WHEN @sort::text = 'key' AND @dir::text = 'desc' THEN key END DESC,
 CASE WHEN @sort::text = 'label' AND @dir::text = 'asc' THEN label END ASC,
 CASE WHEN @sort::text = 'label' AND @dir::text = 'desc' THEN label END DESC,
 CASE WHEN @sort::text = 'enabled' AND @dir::text = 'asc' THEN enabled END ASC,
 CASE WHEN @sort::text = 'enabled' AND @dir::text = 'desc' THEN enabled END DESC,
 CASE WHEN @sort::text NOT IN ('key', 'label', 'enabled') THEN key END ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountNotificationTypes :one
SELECT COUNT(*)::int AS count
FROM notification_types
WHERE (@q::text = '' OR key ILIKE '%' || @q || '%' OR label ILIKE '%' || @q || '%' OR description ILIKE '%' || @q || '%')
 AND (@enabled_filter::int = 0 OR enabled = (@enabled_filter::int = 1));

-- name: CreateNotificationType :exec
-- Plain INSERT — POST /admin/notification-types is a create; an existing
-- key must surface SQLSTATE 23505 → 409 instead of silently overwriting
-- the type's labels/defaults (the PATCH path uses UpdateNotificationType).
INSERT INTO notification_types (key, label, description, category, enabled, default_in_app, default_email, default_push) VALUES (@key, @label, @description, @category, @enabled, @default_in_app, @default_email, @default_push)
;

-- name: UpdateNotificationType :execrows
UPDATE notification_types
SET label = @label, description = @description, category = @category, enabled = @enabled,
 default_in_app = @default_in_app, default_email = @default_email, default_push = @default_push
WHERE key = @key;

-- name: DeleteNotificationType :execrows
-- BUG-007: spec §35 defines DELETE as "disable type". A hard DELETE violates
-- the notifications.notification_type FK (SQLSTATE 23503) the moment any
-- notification references the key, so this now flips enabled=false.
UPDATE notification_types SET enabled = false WHERE key = @key;
