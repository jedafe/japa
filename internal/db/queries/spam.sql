-- Spam Prevention & Content Safety (Section 41)
-- IP blocks, user blocks, censored words, and content hash deduplication.

-- =====================
-- IP Blocks
-- =====================

-- name: CreateIPBlock :one
INSERT INTO ip_blocks (tenant_id, cidr, block_type, reason, blocked_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, tenant_id, cidr, block_type, reason, blocked_by, expires_at, created_at;

-- name: GetIPBlockByCIDR :one
SELECT id, tenant_id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE tenant_id = $1
  AND cidr = $2
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListIPBlocks :many
SELECT id, tenant_id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE tenant_id = $1
  AND (created_at < $3 OR ($3 IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: DeleteIPBlock :exec
DELETE FROM ip_blocks
WHERE id = $1 AND tenant_id = $2;

-- name: GetIPBlockByID :one
SELECT id, tenant_id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE id = $1 AND tenant_id = $2;

-- name: UpdateIPBlock :one
UPDATE ip_blocks
SET cidr = @cidr, block_type = @block_type, reason = sqlc.narg(reason),
    expires_at = sqlc.narg(expires_at), blocked_by = @blocked_by
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, cidr, block_type, reason, blocked_by, expires_at, created_at;

-- name: GetActiveIPBlocks :many
SELECT id, tenant_id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE tenant_id = $1
  AND (expires_at IS NULL OR expires_at > now());

-- name: IsIPBlocked :one
SELECT EXISTS(
    SELECT 1 FROM ip_blocks
    WHERE tenant_id = @tenant_id
      AND @ip::inet <<= cidr
      AND (expires_at IS NULL OR expires_at > now())
) AS blocked;

-- =====================
-- User Blocks
-- =====================

-- name: CreateUserBlock :exec
INSERT INTO user_blocks (blocker_id, blocked_id)
VALUES ($1, $2)
ON CONFLICT (blocker_id, blocked_id) DO NOTHING;

-- name: DeleteUserBlock :exec
DELETE FROM user_blocks
WHERE blocker_id = $1 AND blocked_id = $2;

-- name: GetUserBlocks :many
SELECT blocker_id, blocked_id, created_at
FROM user_blocks
WHERE blocker_id = $1
ORDER BY created_at DESC;

-- name: ListAllUserBlocks :many
SELECT ub.blocker_id, ub.blocked_id, ub.created_at,
       bu.username AS blocker_username, bp.display_name AS blocker_display_name,
       buu.username AS blocked_username, bpp.display_name AS blocked_display_name
FROM user_blocks ub
LEFT JOIN users bu ON bu.id = ub.blocker_id
LEFT JOIN user_profiles bp ON bp.user_id = ub.blocker_id AND bp.tenant_id = bu.tenant_id
LEFT JOIN users buu ON buu.id = ub.blocked_id
LEFT JOIN user_profiles bpp ON bpp.user_id = ub.blocked_id AND bpp.tenant_id = buu.tenant_id
ORDER BY ub.created_at DESC
LIMIT $1;

-- name: IsBlocked :one
SELECT EXISTS(
    SELECT 1 FROM user_blocks
    WHERE (blocker_id = $1 AND blocked_id = $2)
       OR (blocker_id = $2 AND blocked_id = $1)
) AS blocked;

-- =====================
-- Censored Words
-- =====================

-- name: CreateCensoredWord :one
INSERT INTO censored_words (tenant_id, pattern, action, replacement)
VALUES ($1, $2, $3, $4)
RETURNING id, tenant_id, pattern, action, replacement, created_at;

-- name: ListCensoredWords :many
SELECT id, tenant_id, pattern, action, replacement, created_at
FROM censored_words
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: DeleteCensoredWord :exec
DELETE FROM censored_words
WHERE id = $1 AND tenant_id = $2;

-- name: GetCensoredWordByID :one
SELECT id, tenant_id, pattern, action, replacement, created_at
FROM censored_words
WHERE id = $1 AND tenant_id = $2;

-- name: UpdateCensoredWord :one
UPDATE censored_words
SET pattern = @pattern, action = @action, replacement = sqlc.narg(replacement)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, pattern, action, replacement, created_at;

-- name: GetCensoredWordsByTenant :many
SELECT id, tenant_id, pattern, action, replacement, created_at
FROM censored_words
WHERE tenant_id = $1;

-- =====================
-- Content Hash Deduplication
-- =====================

-- name: UpdateContentHash :exec
UPDATE contents
SET content_hash = $2
WHERE id = $1 AND tenant_id = $3;

-- name: CheckDuplicateHash :one
SELECT EXISTS(
    SELECT 1 FROM contents
    WHERE tenant_id = $1
      AND content_hash = $2
      AND content_hash IS NOT NULL
) AS duplicate;
