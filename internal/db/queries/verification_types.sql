-- verification_types — managed badge-type taxonomy (§07 amendment, §98;
-- migration 001013). Validation reads active values per scope; the admin
-- taxonomy manager lists all rows incl. inactive.

-- name: ListVerificationTypes :many
SELECT vt.id, vt.scope, vt.value, vt.label, vt.description, vt.badge_media_id,
       vt.position, vt.is_active, vt.is_protected, vt.created_at, vt.updated_at
FROM verification_types vt
WHERE (COALESCE(sqlc.narg('scope')::text, '') = '' OR vt.scope = sqlc.narg('scope'))
ORDER BY vt.scope, vt.position, vt.value;

-- name: ListActiveVerificationTypeValues :many
SELECT value
FROM verification_types
WHERE scope = @scope AND is_active
ORDER BY position, value;

-- name: VerificationTypeExists :one
SELECT EXISTS(
    SELECT 1 FROM verification_types
    WHERE scope = @scope AND value = @value AND is_active
);

-- name: GetVerificationTypeByID :one
SELECT id, scope, value, label, description, badge_media_id, position,
       is_active, is_protected, created_at, updated_at
FROM verification_types
WHERE id = @id;

-- name: CreateVerificationType :one
INSERT INTO verification_types (scope, value, label, description,
    badge_media_id, position, is_active)
VALUES (@scope, @value, @label, sqlc.narg('description'),
    sqlc.narg('badge_media_id'), COALESCE(sqlc.narg('position'), 0),
    COALESCE(sqlc.narg('is_active'), true))
RETURNING id, scope, value, label, description, badge_media_id, position,
          is_active, is_protected, created_at, updated_at;

-- name: UpdateVerificationType :one
UPDATE verification_types
SET label         = @label,
    description   = sqlc.narg('description'),
    badge_media_id = sqlc.narg('badge_media_id'),
    position      = COALESCE(sqlc.narg('position'), position),
    is_active     = COALESCE(sqlc.narg('is_active'), is_active),
    updated_at    = now()
WHERE id = @id
RETURNING id, scope, value, label, description, badge_media_id, position,
          is_active, is_protected, created_at, updated_at;

-- name: DeleteVerificationType :execrows
DELETE FROM verification_types WHERE id = @id;
