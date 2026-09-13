-- name: CreateIPBlock :one
INSERT INTO ip_blocks (cidr, block_type, reason, blocked_by, expires_at) VALUES ($1, $2, $3, $4, $5)

RETURNING id, cidr, block_type, reason, blocked_by, expires_at, created_at;

-- name: GetIPBlockByCIDR :one
SELECT id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE cidr = $1
 AND (expires_at IS NULL OR expires_at > now());

-- name: ListIPBlocks :many
SELECT id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE (created_at < $2 OR ($2 IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT $1;

-- name: ListIPBlocksPaginated :many
-- Offset-based paginated list for admin DataTable server-side pagination
-- (BUG-010). Supports optional block_type filter.
SELECT id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE (sqlc.narg(block_type)::text IS NULL OR block_type = sqlc.narg(block_type))
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: CountIPBlocks :one
-- Total count for admin DataTable pagination (BUG-010).
SELECT COUNT(*)::int AS count
FROM ip_blocks
WHERE (sqlc.narg(block_type)::text IS NULL OR block_type = sqlc.narg(block_type));

-- name: DeleteIPBlock :exec
DELETE FROM ip_blocks
WHERE id = $1;

-- name: GetIPBlockByID :one
SELECT id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE id = $1;

-- name: UpdateIPBlock :one
UPDATE ip_blocks
SET cidr = @cidr, block_type = @block_type, reason = sqlc.narg(reason),
 expires_at = sqlc.narg(expires_at), blocked_by = @blocked_by
WHERE id = @id
RETURNING id, cidr, block_type, reason, blocked_by, expires_at, created_at;

-- name: GetActiveIPBlocks :many
SELECT id, cidr, block_type, reason, blocked_by, expires_at, created_at
FROM ip_blocks
WHERE (expires_at IS NULL OR expires_at > now());

-- name: IsIPBlocked :one
SELECT EXISTS(
 SELECT 1 FROM ip_blocks
 WHERE @ip::inet <<= cidr
 AND (expires_at IS NULL OR expires_at > now())
) AS blocked;

-- name: GetIPBlockTypeForIP :one
-- Returns the most severe block_type for a matched IP. 'full' takes priority
-- over 'registration'/'posting'/'api' so a full block is enforced even if a
-- narrower block also matches. Per §41 IP Block Middleware, non-full block
-- types are scoped to specific route groups rather than blocking all requests.
SELECT COALESCE(
 (SELECT block_type::text FROM ip_blocks
 WHERE @ip::inet <<= cidr
 AND (expires_at IS NULL OR expires_at > now())
 ORDER BY CASE block_type
 WHEN 'full' THEN 0
 WHEN 'registration' THEN 1
 WHEN 'posting' THEN 2
 WHEN 'api' THEN 3
 ELSE 4
 END
 LIMIT 1),
 ''
)::text AS block_type;

-- =====================
-- User Blocks
-- =====================

-- name: CreateUserBlock :exec
-- Tenant-scoped per JEOS §9 (BUG-006).
INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)

ON CONFLICT (blocker_id, blocked_id) DO NOTHING;

-- name: DeleteUserBlock :exec
-- Tenant-scoped per JEOS §9 (BUG-006).
DELETE FROM user_blocks
WHERE blocker_id = $1 AND blocked_id = $2;

-- name: GetUserBlocks :many
SELECT blocker_id, blocked_id, created_at
FROM user_blocks
WHERE blocker_id = $1
ORDER BY created_at DESC;

-- name: ListBlockedUsers :many
-- Tenant-scoped blocked-users list with profile enrichment (BUG-003, BUG-004).
-- Ref: japaconnect-engineering/sections/41-spam-content-safety.md
SELECT ub.blocked_id, ub.created_at,
 u.username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM user_blocks ub
JOIN users u ON u.id = ub.blocked_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE ub.blocker_id = $1
ORDER BY ub.created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAllUserBlocks :many
-- Tenant-scoped per JEOS §9 (BUG-001). Admin list must not cross tenants.
SELECT ub.blocker_id, ub.blocked_id, ub.created_at,
 bu.username AS blocker_username, bp.display_name AS blocker_display_name,
 buu.username AS blocked_username, bpp.display_name AS blocked_display_name
FROM user_blocks ub
LEFT JOIN users bu ON bu.id = ub.blocker_id
LEFT JOIN user_profiles bp ON bp.user_id = ub.blocker_id
LEFT JOIN users buu ON buu.id = ub.blocked_id
LEFT JOIN user_profiles bpp ON bpp.user_id = ub.blocked_id

ORDER BY ub.created_at DESC
LIMIT $1;

-- name: ListAllUserBlocksPaginated :many
-- Offset-based paginated admin list for DataTable server-side pagination
-- (BUG-010). Tenant-scoped per JEOS §9.
SELECT ub.blocker_id, ub.blocked_id, ub.created_at,
 bu.username AS blocker_username, bp.display_name AS blocker_display_name,
 buu.username AS blocked_username, bpp.display_name AS blocked_display_name
FROM user_blocks ub
LEFT JOIN users bu ON bu.id = ub.blocker_id
LEFT JOIN user_profiles bp ON bp.user_id = ub.blocker_id
LEFT JOIN users buu ON buu.id = ub.blocked_id
LEFT JOIN user_profiles bpp ON bpp.user_id = ub.blocked_id

ORDER BY ub.created_at DESC, ub.blocker_id DESC
LIMIT $1 OFFSET $2;

-- name: CountAllUserBlocks :one
-- Total count for admin DataTable pagination (BUG-010). Tenant-scoped.
SELECT COUNT(*)::int AS count
FROM user_blocks
;

-- name: IsBlocked :one
-- Tenant-scoped per JEOS §9 (BUG-006). Checks both directions in one query.
SELECT EXISTS(
 SELECT 1 FROM user_blocks
 WHERE ((blocker_id = $1 AND blocked_id = $2)
 OR (blocker_id = $2 AND blocked_id = $1))
) AS blocked;

-- =====================
-- Block side-effects (BE-1 / Section 41 line 147)
-- Blocking a user also cancels any follow relationship (either direction) and
-- declines any pending DM request between the two users. These queries are
-- executed in the same transaction as CreateUserBlock.
-- =====================

-- name: DeleteFollowsBetweenUsers :exec
-- Removes follow rows in both directions between two users
-- (entity_type='user'). Tenant-scoped per JEOS §9.
DELETE FROM follows
WHERE entity_type = 'user'
 AND ((follower_id = $1 AND entity_id = $2)
 OR (follower_id = $2 AND entity_id = $1));

-- name: DeclinePendingDMRequestsBetweenUsers :exec
-- Marks pending DM requests between the two users as declined. Soft-updates
-- the status + resolved_at rather than hard-deleting (the request row is
-- needed for audit). Tenant-scoped per JEOS §9.
UPDATE dm_requests
SET status = 'declined', resolved_at = now()
WHERE status = 'pending'
 AND ((requester_id = $1 AND recipient_id = $2)
 OR (requester_id = $2 AND recipient_id = $1));

-- =====================
-- Censored Words
-- =====================

-- name: CreateCensoredWord :one
INSERT INTO censored_words (pattern, action, replacement) VALUES ($1, $2, $3)

RETURNING id, pattern, action, replacement, created_at;

-- name: ListCensoredWords :many
SELECT id, pattern, action, replacement, created_at
FROM censored_words

ORDER BY created_at DESC;

-- name: ListCensoredWordsPaginated :many
-- Offset-based paginated list for admin DataTable server-side pagination
-- (BUG-010). Supports optional action filter.
SELECT id, pattern, action, replacement, created_at
FROM censored_words
WHERE (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: CountCensoredWords :one
-- Total count for admin DataTable pagination (BUG-010).
SELECT COUNT(*)::int AS count
FROM censored_words
WHERE (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action));

-- name: DeleteCensoredWord :exec
DELETE FROM censored_words
WHERE id = $1;

-- name: GetCensoredWordByID :one
SELECT id, pattern, action, replacement, created_at
FROM censored_words
WHERE id = $1;

-- name: UpdateCensoredWord :one
UPDATE censored_words
SET pattern = @pattern, action = @action, replacement = sqlc.narg(replacement)
WHERE id = @id
RETURNING id, pattern, action, replacement, created_at;

-- name: GetCensoredWordsByTenant :many
SELECT id, pattern, action, replacement, created_at
FROM censored_words
;

-- =====================
-- Content Hash Deduplication
-- =====================

-- name: UpdateContentHash :exec
UPDATE contents
SET content_hash = $2
WHERE id = $1;

-- name: CheckDuplicateHash :one
SELECT EXISTS(
 SELECT 1 FROM contents
 WHERE content_hash = $1
 AND content_hash IS NOT NULL
) AS duplicate;
