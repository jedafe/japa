-- name: CreateOperationBan :exec
INSERT INTO user_operation_bans (user_id, operation, reason, expires_at, issued_by) VALUES ($1, $2, $3, CASE WHEN $4::int > 0 THEN now() + ($4 || ' seconds')::interval ELSE NULL END, $5)
;

-- name: SoftDeleteActiveOperationBan :exec
UPDATE user_operation_bans SET deleted_at = now()
WHERE user_id = $1 AND operation = $2 AND deleted_at IS NULL;

-- name: LiftOperationBan :exec
UPDATE user_operation_bans SET deleted_at = now()
WHERE user_id = $1 AND operation = $2 AND deleted_at IS NULL;

-- name: LiftAllOperationBans :exec
UPDATE user_operation_bans SET deleted_at = now()
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: CountActiveOperationBans :one
SELECT COUNT(*)::int FROM user_operation_bans
WHERE user_id = $1 AND deleted_at IS NULL
 AND (expires_at IS NULL OR expires_at > now());

-- name: ListActiveOperationBans :many
SELECT id, operation::text, reason, expires_at, created_at
FROM user_operation_bans
WHERE user_id = $1 AND deleted_at IS NULL
 AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC;

-- name: GetUserOperationBansForCache :many
SELECT operation::text, expires_at
FROM user_operation_bans
WHERE user_id = $1 AND deleted_at IS NULL
 AND (expires_at IS NULL OR expires_at > now());

-- name: CheckUserOperationBanExists :one
-- AUDIT-BE-DB-003: replaces raw SQL existence check in operation ban enforcement middleware.
-- Returns true when an active (non-expired, non-deleted) operation ban exists.
SELECT EXISTS(
 SELECT 1 FROM user_operation_bans
 WHERE user_id = $1 AND operation = $2 AND deleted_at IS NULL
 AND (expires_at IS NULL OR expires_at > now())
);

-- name: GetUserOperationBanExpiration :one
-- AUDIT-BE-DB-004: replaces raw SQL expiration lookup in operation ban enforcement middleware.
-- Returns the expires_at of the most recent active operation ban (may be NULL = permanent).
SELECT expires_at FROM user_operation_bans
WHERE user_id = $1 AND operation = $2 AND deleted_at IS NULL
AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC LIMIT 1;

-- name: CreateUserSuspension :exec
INSERT INTO user_moderation_actions (user_id, action, reason) VALUES ($1, 'suspension', $2)
;

-- name: CreateUserTypedWarning :exec
INSERT INTO user_moderation_actions (user_id, action, reason, violation_type) VALUES ($1, 'warning', $2, $3)
;

-- name: CountUserWarningsByType :one
SELECT COUNT(*)::int FROM user_moderation_actions
WHERE user_id = $1 AND action = 'warning' AND violation_type = $2 AND deleted_at IS NULL;

-- name: ListUserWarningsTyped :many
SELECT id, reason, violation_type, created_at
FROM user_moderation_actions
WHERE user_id = $1 AND action = 'warning' AND deleted_at IS NULL
ORDER BY created_at DESC;
