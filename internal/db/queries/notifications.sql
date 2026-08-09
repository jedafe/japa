-- Notification system queries (Section 35).
-- In-app, email, push delivery with batch deduplication.

-- =====================
-- Notifications CRUD
-- =====================

-- name: InsertNotification :exec
INSERT INTO notifications (tenant_id, recipient_id, notification_type, actor_type, actor_id, subject_type, subject_id, title, body, action_url)
VALUES (@tenant_id, @recipient_id, @notification_type, @actor_type, @actor_id, @subject_type, @subject_id, @title, @body, @action_url);

-- name: GetNotificationsByRecipient :many
SELECT n.id, n.tenant_id, n.recipient_id, n.notification_type, n.actor_type, n.actor_id,
       n.subject_type, n.subject_id, n.title, n.body, n.action_url, n.read_at, n.created_at,
       nt.label AS type_label
FROM notifications n
JOIN notification_types nt ON nt.key = n.notification_type
WHERE n.tenant_id = @tenant_id AND n.recipient_id = @recipient_id
ORDER BY n.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetUnreadNotificationCount :one
SELECT COUNT(*)::int AS count
FROM notifications
WHERE tenant_id = @tenant_id AND recipient_id = @recipient_id AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = now() WHERE id = @id AND recipient_id = @recipient_id;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now()
WHERE tenant_id = @tenant_id AND recipient_id = @recipient_id AND read_at IS NULL;

-- name: DeleteNotification :exec
DELETE FROM notifications WHERE id = @id AND recipient_id = @recipient_id;

-- =====================
-- Notification Preferences
-- =====================

-- name: GetNotificationPreferences :many
SELECT np.user_id, np.notification_type, np.in_app, np.email, np.push,
       nt.label AS type_label
FROM notification_preferences np
JOIN notification_types nt ON nt.key = np.notification_type
WHERE np.user_id = @user_id;

-- name: UpsertNotificationPreference :exec
INSERT INTO notification_preferences (user_id, notification_type, in_app, email, push)
VALUES (@user_id, @notification_type, @in_app, @email, @push)
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

-- name: CreateBatchWindow :exec
INSERT INTO notification_batch_windows (recipient_id, notification_type, subject_type, subject_id, actor_ids, actor_count, last_actor_id, window_close_at)
VALUES (@recipient_id, @notification_type, @subject_type, @subject_id, ARRAY[@actor_id::uuid], 1, @actor_id, now() + make_interval(mins => @window_minutes));

-- name: ExtendBatchWindow :exec
UPDATE notification_batch_windows
SET actor_ids = array_append(actor_ids, @actor_id),
    actor_count = actor_count + 1,
    last_actor_id = @actor_id
WHERE id = @id;

-- name: MarkBatchWindowDispatched :exec
UPDATE notification_batch_windows SET dispatched = true WHERE id = @id;

-- name: GetBatchWindowByID :one
SELECT id, recipient_id, notification_type, subject_type, subject_id,
       actor_ids, actor_count, last_actor_id, window_open_at, window_close_at, dispatched
FROM notification_batch_windows
WHERE id = @id;

-- =====================
-- Notification Types
-- =====================

-- name: GetNotificationTypes :many
SELECT key, label, description, enabled, default_in_app, default_email, default_push
FROM notification_types ORDER BY key;

-- name: AdminListNotificationTypes :many
-- Admin list with offset pagination + search + enabled filter + sort (Section 35).
SELECT key, label, description, enabled, default_in_app, default_email, default_push
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

-- name: UpsertNotificationType :exec
INSERT INTO notification_types (key, label, description, enabled, default_in_app, default_email, default_push)
VALUES (@key, @label, @description, @enabled, @default_in_app, @default_email, @default_push)
ON CONFLICT (key) DO UPDATE SET
  label = EXCLUDED.label,
  description = EXCLUDED.description,
  enabled = EXCLUDED.enabled,
  default_in_app = EXCLUDED.default_in_app,
  default_email = EXCLUDED.default_email,
  default_push = EXCLUDED.default_push;

-- name: UpdateNotificationType :exec
UPDATE notification_types
SET label = @label, description = @description, enabled = @enabled,
    default_in_app = @default_in_app, default_email = @default_email, default_push = @default_push
WHERE key = @key;

-- name: DeleteNotificationType :exec
DELETE FROM notification_types WHERE key = @key;
