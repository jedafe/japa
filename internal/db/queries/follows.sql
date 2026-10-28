-- name: InsertFollow :execrows
-- Returns rows affected: 1 = inserted (newly following), 0 = conflict
-- (already following). Lets the service drive count deltas and side
-- effects off the actual state change, closing the check-then-insert race.
INSERT INTO follows (follower_id, entity_type, entity_id) VALUES (@follower_id, @entity_type, @entity_id)

ON CONFLICT (follower_id, entity_type, entity_id) DO NOTHING;

-- name: DeleteFollowByFollowerEntity :execrows
-- Returns rows affected: 1 = deleted (was following), 0 = no-op.
DELETE FROM follows
WHERE follower_id = @follower_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: DeleteFollowByID :exec
DELETE FROM follows WHERE id = @id;

-- name: GetFollow :one
SELECT id, follower_id, entity_type, entity_id, created_at
FROM follows
WHERE follower_id = @follower_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- =====================
-- Status Checks
-- =====================

-- name: IsFollowing :one
SELECT EXISTS (
 SELECT 1 FROM follows
 WHERE follower_id = @follower_id AND entity_type = @entity_type AND entity_id = @entity_id
) AS is_following;

-- name: IsMutualFollow :one
SELECT EXISTS (
 SELECT 1 FROM follows
 WHERE follower_id = @entity_id AND entity_type = 'user' AND entity_id = @follower_id
) AS is_mutual;

-- =====================
-- Lists
-- =====================

-- name: GetFollowersByEntity :many
SELECT f.id, f.follower_id, f.entity_type, f.entity_id, f.created_at,
 COALESCE(u.public_username, u.username, '') AS username,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url,
 u.profile_type,
 u.is_synthetic
FROM follows f
JOIN users u ON u.id = f.follower_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE f.entity_type = @entity_type AND f.entity_id = @entity_id
  AND u.status = 'active'
  AND (
    @entity_type <> 'user'
    OR sqlc.narg(viewer_id)::uuid IS NULL
    OR NOT EXISTS (
      SELECT 1 FROM user_blocks ub
      WHERE (ub.blocker_id = sqlc.narg(viewer_id)::uuid AND ub.blocked_id = f.follower_id)
         OR (ub.blocker_id = f.follower_id AND ub.blocked_id = sqlc.narg(viewer_id)::uuid)
    )
  )
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetFollowingByUser :many
SELECT f.id, f.follower_id, f.entity_type, f.entity_id, f.created_at,
 CASE f.entity_type
 WHEN 'user' THEN COALESCE((SELECT username FROM users WHERE id = f.entity_id), '')
 WHEN 'space' THEN COALESCE((SELECT name FROM spaces WHERE id = f.entity_id), '')
 WHEN 'tag' THEN COALESCE((SELECT name FROM tags WHERE id = f.entity_id), '')
 WHEN 'organization' THEN COALESCE((SELECT name FROM organizations WHERE id = f.entity_id), '')
 WHEN 'source' THEN COALESCE((SELECT name FROM intel_sources WHERE id = f.entity_id), '')
 ELSE ''
 END AS entity_name,
 CASE f.entity_type
 WHEN 'user' THEN COALESCE((SELECT username FROM users WHERE id = f.entity_id), '')
 WHEN 'space' THEN COALESCE((SELECT slug FROM spaces WHERE id = f.entity_id), '')
 WHEN 'tag' THEN COALESCE((SELECT slug FROM tags WHERE id = f.entity_id), '')
 WHEN 'organization' THEN COALESCE((SELECT slug FROM organizations WHERE id = f.entity_id), '')
 WHEN 'source' THEN COALESCE((SELECT slug FROM intel_sources WHERE id = f.entity_id), '')
 ELSE ''
 END AS entity_slug,
 CASE WHEN f.entity_type = 'user' THEN COALESCE((SELECT display_name FROM user_profiles WHERE user_id = f.entity_id), '') ELSE '' END::text AS entity_display_name,
 CASE f.entity_type
 WHEN 'source' THEN COALESCE((SELECT logo_url FROM intel_sources WHERE id = f.entity_id), '')
 ELSE COALESCE((SELECT avatar_url FROM user_profiles WHERE user_id = f.entity_id), '')
 END::text AS entity_avatar_url
FROM follows f
WHERE f.follower_id = @follower_id
  AND (
    (f.entity_type = 'user' AND EXISTS (SELECT 1 FROM users fu WHERE fu.id = f.entity_id AND fu.status = 'active'))
    OR (f.entity_type = 'space' AND EXISTS (SELECT 1 FROM spaces sp WHERE sp.id = f.entity_id AND sp.deleted_at IS NULL))
    OR (f.entity_type = 'tag' AND EXISTS (SELECT 1 FROM tags tg WHERE tg.id = f.entity_id))
    OR (f.entity_type = 'organization' AND EXISTS (SELECT 1 FROM organizations og WHERE og.id = f.entity_id AND og.deleted_at IS NULL))
    OR (f.entity_type = 'source' AND EXISTS (SELECT 1 FROM intel_sources src WHERE src.id = f.entity_id))
  )
  AND (f.entity_type <> 'user' OR sqlc.narg(viewer_id)::uuid IS NULL
    OR NOT EXISTS (
      SELECT 1 FROM user_blocks ub
      WHERE (ub.blocker_id = sqlc.narg(viewer_id)::uuid AND ub.blocked_id = f.entity_id)
         OR (ub.blocker_id = f.entity_id AND ub.blocked_id = sqlc.narg(viewer_id)::uuid)
    )
  )
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetFollowingByUserAndType :many
SELECT f.id, f.follower_id, f.entity_type, f.entity_id, f.created_at,
 CASE f.entity_type
 WHEN 'user' THEN COALESCE((SELECT username FROM users WHERE id = f.entity_id), '')
 WHEN 'space' THEN COALESCE((SELECT name FROM spaces WHERE id = f.entity_id), '')
 WHEN 'tag' THEN COALESCE((SELECT name FROM tags WHERE id = f.entity_id), '')
 WHEN 'organization' THEN COALESCE((SELECT name FROM organizations WHERE id = f.entity_id), '')
 WHEN 'source' THEN COALESCE((SELECT name FROM intel_sources WHERE id = f.entity_id), '')
 ELSE ''
 END AS entity_name,
 CASE f.entity_type
 WHEN 'user' THEN COALESCE((SELECT username FROM users WHERE id = f.entity_id), '')
 WHEN 'space' THEN COALESCE((SELECT slug FROM spaces WHERE id = f.entity_id), '')
 WHEN 'tag' THEN COALESCE((SELECT slug FROM tags WHERE id = f.entity_id), '')
 WHEN 'organization' THEN COALESCE((SELECT slug FROM organizations WHERE id = f.entity_id), '')
 WHEN 'source' THEN COALESCE((SELECT slug FROM intel_sources WHERE id = f.entity_id), '')
 ELSE ''
 END AS entity_slug,
 CASE WHEN f.entity_type = 'user' THEN COALESCE((SELECT display_name FROM user_profiles WHERE user_id = f.entity_id), '') ELSE '' END::text AS entity_display_name,
 CASE f.entity_type
 WHEN 'source' THEN COALESCE((SELECT logo_url FROM intel_sources WHERE id = f.entity_id), '')
 ELSE COALESCE((SELECT avatar_url FROM user_profiles WHERE user_id = f.entity_id), '')
 END::text AS entity_avatar_url
FROM follows f
WHERE f.follower_id = @follower_id AND f.entity_type = @entity_type
  AND (
    (f.entity_type = 'user' AND EXISTS (SELECT 1 FROM users fu WHERE fu.id = f.entity_id AND fu.status = 'active'))
    OR (f.entity_type = 'space' AND EXISTS (SELECT 1 FROM spaces sp WHERE sp.id = f.entity_id AND sp.deleted_at IS NULL))
    OR (f.entity_type = 'tag' AND EXISTS (SELECT 1 FROM tags tg WHERE tg.id = f.entity_id))
    OR (f.entity_type = 'organization' AND EXISTS (SELECT 1 FROM organizations og WHERE og.id = f.entity_id AND og.deleted_at IS NULL))
    OR (f.entity_type = 'source' AND EXISTS (SELECT 1 FROM intel_sources src WHERE src.id = f.entity_id))
  )
  AND (f.entity_type <> 'user' OR sqlc.narg(viewer_id)::uuid IS NULL
    OR NOT EXISTS (
      SELECT 1 FROM user_blocks ub
      WHERE (ub.blocker_id = sqlc.narg(viewer_id)::uuid AND ub.blocked_id = f.entity_id)
         OR (ub.blocker_id = f.entity_id AND ub.blocked_id = sqlc.narg(viewer_id)::uuid)
    )
  )
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Counts
-- =====================

-- name: CountFollowersByEntity :one
SELECT COUNT(*)::int AS count FROM follows
WHERE entity_type = @entity_type AND entity_id = @entity_id;

-- =====================
-- Bulk Operations
-- =====================

-- name: GetBulkFollowingStatus :many
-- Uses unnest so every input entity_id appears in the output with
-- is_following = false when no follow row exists for the current user.
-- Ref: japaconnect-engineering/sections/31-follow-system.md (bulk is_following)
SELECT @entity_type::text AS entity_type, eid::uuid AS entity_id,
 EXISTS (
 SELECT 1 FROM follows sub
 WHERE sub.follower_id = @follower_id
 AND sub.entity_type = @entity_type
 AND sub.entity_id = eid
 ) AS is_following
FROM unnest(@entity_ids::uuid[]) AS t(eid);

-- =====================
-- Cleanup
-- =====================

-- name: DeleteFollowsByFollower :exec
DELETE FROM follows WHERE follower_id = @follower_id;

-- name: DeleteFollowsByEntity :exec
DELETE FROM follows WHERE entity_type = @entity_type AND entity_id = @entity_id;
