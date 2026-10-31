-- Moderation alert queries (audit A13).

-- name: CreateModerationAlert :one
INSERT INTO moderation_alerts (alert_type, severity, title, description, meta) VALUES (@alert_type, @severity, @title, sqlc.narg(description), sqlc.narg(meta))
RETURNING *;

-- name: ListModerationAlerts :many
SELECT * FROM moderation_alerts
WHERE (NULLIF(@status, '') IS NULL OR status = @status)
ORDER BY
 CASE WHEN status = 'open' THEN 0 ELSE 1 END,
 created_at DESC;

-- name: ListModerationAlertsFiltered :many
-- Admin alerts list — server-side severity/status/q filters + pagination so
-- the DataTable spec holds (was status-only, everything else ignored).
SELECT * FROM moderation_alerts
WHERE (NULLIF(@status, '') IS NULL OR status = @status)
 AND (NULLIF(@severity, '') IS NULL OR severity = @severity)
 AND (NULLIF(@q, '') IS NULL OR
   title ILIKE '%' || @q || '%'
   OR COALESCE(description, '') ILIKE '%' || @q || '%'
   OR alert_type ILIKE '%' || @q || '%')
ORDER BY
 CASE WHEN status = 'open' THEN 0 ELSE 1 END,
 created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountModerationAlertsFiltered :one
SELECT COUNT(*)::int FROM moderation_alerts
WHERE (NULLIF(@status, '') IS NULL OR status = @status)
 AND (NULLIF(@severity, '') IS NULL OR severity = @severity)
 AND (NULLIF(@q, '') IS NULL OR
   title ILIKE '%' || @q || '%'
   OR COALESCE(description, '') ILIKE '%' || @q || '%'
   OR alert_type ILIKE '%' || @q || '%');

-- name: GetModerationAlert :one
SELECT * FROM moderation_alerts
WHERE id = @id;

-- name: AcknowledgeModerationAlert :one
UPDATE moderation_alerts
SET status = 'acknowledged',
 acknowledged_at = now(),
 acknowledged_by = @acknowledged_by,
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: ResolveModerationAlert :one
UPDATE moderation_alerts
SET status = 'resolved',
 resolved_at = now(),
 resolved_by = @resolved_by,
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: CountPendingReportsByEntity :one
SELECT COUNT(*) FROM content_reports
WHERE entity_type = @entity_type AND entity_id = @entity_id
 AND status = 'pending';

-- name: ExpireModerationBans :exec
UPDATE user_moderation_actions
SET action = 'unban'
WHERE action = 'ban'
 AND expires_at IS NOT NULL
 AND expires_at < now();
