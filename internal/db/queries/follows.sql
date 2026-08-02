-- Follow system queries (Section 31).
-- Polymorphic follows for users, spaces, tags, organizations, professional_profiles.

-- =====================
-- Follow CRUD
-- =====================

-- name: InsertFollow :exec
INSERT INTO follows (tenant_id, follower_id, entity_type, entity_id)
VALUES (@tenant_id, @follower_id, @entity_type, @entity_id)
ON CONFLICT (follower_id, entity_type, entity_id) DO NOTHING;

-- name: DeleteFollowByFollowerEntity :exec
DELETE FROM follows
WHERE follower_id = @follower_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: DeleteFollowByID :exec
DELETE FROM follows WHERE id = @id;

-- name: GetFollow :one
SELECT id, tenant_id, follower_id, entity_type, entity_id, created_at
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
SELECT f.id, f.tenant_id, f.follower_id, f.entity_type, f.entity_id, f.created_at,
       u.username,
       COALESCE(up.display_name, '') AS display_name,
       COALESCE(up.avatar_url, '') AS avatar_url
FROM follows f
JOIN users u ON u.id = f.follower_id
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = f.tenant_id
WHERE f.tenant_id = @tenant_id AND f.entity_type = @entity_type AND f.entity_id = @entity_id
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetFollowingByUser :many
SELECT f.id, f.tenant_id, f.follower_id, f.entity_type, f.entity_id, f.created_at,
    CASE f.entity_type
        WHEN 'user' THEN (SELECT username FROM users WHERE id = f.entity_id)
        WHEN 'space' THEN (SELECT name FROM spaces WHERE id = f.entity_id)
        WHEN 'tag' THEN (SELECT name FROM tags WHERE id = f.entity_id)
        WHEN 'organization' THEN (SELECT name FROM organizations WHERE id = f.entity_id)
        WHEN 'professional_profile' THEN (SELECT name FROM professional_profiles WHERE id = f.entity_id)
        ELSE ''
    END AS entity_name,
    CASE f.entity_type
        WHEN 'user' THEN (SELECT username FROM users WHERE id = f.entity_id)
        WHEN 'space' THEN (SELECT slug FROM spaces WHERE id = f.entity_id)
        WHEN 'tag' THEN (SELECT slug FROM tags WHERE id = f.entity_id)
        WHEN 'organization' THEN (SELECT slug FROM organizations WHERE id = f.entity_id)
        ELSE ''
    END AS entity_slug
FROM follows f
WHERE f.tenant_id = @tenant_id AND f.follower_id = @follower_id
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetFollowingByUserAndType :many
SELECT f.id, f.tenant_id, f.follower_id, f.entity_type, f.entity_id, f.created_at,
    CASE f.entity_type
        WHEN 'user' THEN (SELECT username FROM users WHERE id = f.entity_id)
        WHEN 'space' THEN (SELECT name FROM spaces WHERE id = f.entity_id)
        WHEN 'tag' THEN (SELECT name FROM tags WHERE id = f.entity_id)
        WHEN 'organization' THEN (SELECT name FROM organizations WHERE id = f.entity_id)
        WHEN 'professional_profile' THEN (SELECT name FROM professional_profiles WHERE id = f.entity_id)
        ELSE ''
    END AS entity_name,
    CASE f.entity_type
        WHEN 'user' THEN (SELECT username FROM users WHERE id = f.entity_id)
        WHEN 'space' THEN (SELECT slug FROM spaces WHERE id = f.entity_id)
        WHEN 'tag' THEN (SELECT slug FROM tags WHERE id = f.entity_id)
        WHEN 'organization' THEN (SELECT slug FROM organizations WHERE id = f.entity_id)
        ELSE ''
    END AS entity_slug
FROM follows f
WHERE f.tenant_id = @tenant_id AND f.follower_id = @follower_id AND f.entity_type = @entity_type
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Counts
-- =====================

-- name: CountFollowersByEntity :one
SELECT COUNT(*)::int AS count FROM follows
WHERE tenant_id = @tenant_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: CountFollowingByUser :one
SELECT COUNT(*)::int AS count FROM follows
WHERE tenant_id = @tenant_id AND follower_id = @follower_id;

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
        WHERE sub.tenant_id = @tenant_id
          AND sub.follower_id = @follower_id
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
