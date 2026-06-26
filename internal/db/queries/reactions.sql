-- Reaction types and polymorphic reactions.

-- =====================
-- Reaction Types
-- =====================

-- name: ListReactionTypes :many
SELECT key, emoji, label, enabled, position, created_at, updated_at
FROM reaction_types
ORDER BY position ASC, key ASC;

-- name: GetReactionType :one
SELECT key, emoji, label, enabled, position, created_at, updated_at
FROM reaction_types
WHERE key = @key;

-- name: CreateReactionType :one
INSERT INTO reaction_types (key, emoji, label, enabled, position)
VALUES (@key, @emoji, @label, @enabled, @position)
RETURNING key, emoji, label, enabled, position, created_at, updated_at;

-- name: UpdateReactionType :one
UPDATE reaction_types
SET emoji = @emoji, label = @label, enabled = @enabled, position = @position, updated_at = now()
WHERE key = @key
RETURNING key, emoji, label, enabled, position, created_at, updated_at;

-- name: DeleteReactionType :exec
DELETE FROM reaction_types WHERE key = @key;

-- =====================
-- Reactions
-- =====================

-- name: AddReaction :one
INSERT INTO reactions (entity_type, entity_id, user_id, reaction_key)
VALUES (@entity_type, @entity_id, @user_id, @reaction_key)
ON CONFLICT (entity_type, entity_id, user_id, reaction_key) DO NOTHING
RETURNING entity_type, entity_id, user_id, reaction_key, created_at;

-- name: RemoveReaction :exec
DELETE FROM reactions
WHERE entity_type = @entity_type AND entity_id = @entity_id AND user_id = @user_id AND reaction_key = @reaction_key;

-- name: ListReactionsByEntity :many
SELECT entity_type, entity_id, user_id, reaction_key, created_at
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
SELECT entity_type, entity_id, user_id, reaction_key, created_at
FROM reactions
WHERE entity_type = @entity_type AND entity_id = @entity_id AND user_id = @user_id;
