-- name: ListReactionTypes :many
SELECT tenant_id, key, emoji, label, enabled, position, created_at, updated_at
FROM reaction_types

ORDER BY position ASC, key ASC;

-- name: GetReactionType :one
SELECT tenant_id, key, emoji, label, enabled, position, created_at, updated_at
FROM reaction_types
WHERE key = @key;

-- name: CreateReactionType :one
INSERT INTO reaction_types (key, emoji, label, enabled, position) VALUES (@key, @emoji, @label, @enabled, @position)

RETURNING tenant_id, key, emoji, label, enabled, position, created_at, updated_at;

-- name: UpdateReactionType :one
UPDATE reaction_types
SET emoji = @emoji, label = @label, enabled = @enabled, position = @position, updated_at = now()
WHERE key = @key
RETURNING tenant_id, key, emoji, label, enabled, position, created_at, updated_at;

-- name: DeleteReactionType :exec
DELETE FROM reaction_types WHERE key = @key;

-- =====================
-- Reactions
-- =====================

-- name: AddReaction :one
INSERT INTO reactions (entity_type, entity_id, user_id, reaction_key) VALUES (@entity_type, @entity_id, @user_id, @reaction_key)

ON CONFLICT (entity_type, entity_id, user_id, reaction_key) DO NOTHING
RETURNING tenant_id, entity_type, entity_id, user_id, reaction_key, created_at;

-- name: RemoveReaction :exec
DELETE FROM reactions
WHERE entity_type = @entity_type AND entity_id = @entity_id AND user_id = @user_id AND reaction_key = @reaction_key;

-- name: ListReactionsByEntity :many
SELECT tenant_id, entity_type, entity_id, user_id, reaction_key, created_at
FROM reactions
WHERE entity_type = @entity_type AND entity_id = @entity_id
ORDER BY created_at DESC;

-- name: CountReactionsByEntity :many
SELECT reaction_key, COUNT(*) AS count
FROM reactions
WHERE entity_type = @entity_type AND entity_id = @entity_id
GROUP BY reaction_key
ORDER BY count DESC;

-- name: GetUserReactions :many
SELECT tenant_id, entity_type, entity_id, user_id, reaction_key, created_at
FROM reactions
WHERE entity_type = @entity_type AND entity_id = @entity_id AND user_id = @user_id;

-- name: ListContentLikedByUser :many
-- Public "Likes" tab: published+public content the user has reacted to (any
-- reaction key), one row per content (GROUP BY collapses multiple reaction
-- keys), newest reaction first. Public-safe: draft/private content excluded.
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
 c.created_at, c.view_count, c.comment_count,
 COALESCE(up.display_name, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 LEFT(c.body, 200) AS excerpt,
 MAX(r.created_at)::timestamptz AS reacted_at
FROM reactions r
JOIN contents c ON c.id = r.entity_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE r.user_id = @user_id
 AND r.entity_type = 'content'
 AND c.status = 'published'
 AND c.visibility = 'public'
GROUP BY c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
 c.created_at, c.view_count, c.comment_count,
 up.display_name, u.username, up.avatar_url, c.body
ORDER BY reacted_at DESC
LIMIT @page_limit OFFSET @page_offset;
