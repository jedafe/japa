-- name: CreateOperationBan :exec
INSERT INTO user_operation_bans (user_id, tenant_id, operation, reason, expires_at, issued_by)
VALUES ($1, $2, $3, $4,
        CASE WHEN $5::int > 0 THEN now() + ($5 || ' seconds')::interval ELSE NULL END,
        $6);

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

-- name: CreateUserSuspension :exec
INSERT INTO user_moderation_actions (user_id, tenant_id, action, reason)
VALUES ($1, $2, 'suspension', $3);

-- name: CreateUserTypedWarning :exec
INSERT INTO user_moderation_actions (user_id, tenant_id, action, reason, violation_type)
VALUES ($1, $2, 'warning', $3, $4);

-- name: CountUserWarningsByType :one
SELECT COUNT(*)::int FROM user_moderation_actions
WHERE user_id = $1 AND action = 'warning' AND violation_type = $2 AND deleted_at IS NULL;

-- name: ListUserWarningsTyped :many
SELECT id, reason, violation_type, created_at
FROM user_moderation_actions
WHERE user_id = $1 AND action = 'warning' AND deleted_at IS NULL
ORDER BY created_at DESC;
