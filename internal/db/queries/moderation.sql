-- Moderation alert queries (audit A13).

-- name: CreateModerationAlert :one
INSERT INTO moderation_alerts (
    tenant_id, alert_type, severity, title, description, meta
) VALUES (
    @tenant_id, @alert_type, @severity, @title, sqlc.narg(description), sqlc.narg(meta)
)
RETURNING *;

-- name: ListModerationAlerts :many
SELECT * FROM moderation_alerts
WHERE tenant_id = @tenant_id
  AND (NULLIF(@status, '') IS NULL OR status = @status)
ORDER BY
    CASE WHEN status = 'open' THEN 0 ELSE 1 END,
    created_at DESC;

-- name: GetModerationAlert :one
SELECT * FROM moderation_alerts
WHERE id = @id AND tenant_id = @tenant_id;

-- name: AcknowledgeModerationAlert :one
UPDATE moderation_alerts
SET status = 'acknowledged',
    acknowledged_at = now(),
    acknowledged_by = @acknowledged_by,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: ResolveModerationAlert :one
UPDATE moderation_alerts
SET status = 'resolved',
    resolved_at = now(),
    resolved_by = @resolved_by,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;
