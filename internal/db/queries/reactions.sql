-- Reaction types and polymorphic reactions (tenant-scoped).

-- =====================
-- Reaction Types
-- =====================

-- name: ListReactionTypes :many
SELECT tenant_id, key, emoji, label, enabled, position, created_at, updated_at
FROM reaction_types
WHERE tenant_id = @tenant_id
ORDER BY position ASC, key ASC;

-- name: GetReactionType :one
SELECT tenant_id, key, emoji, label, enabled, position, created_at, updated_at
FROM reaction_types
WHERE tenant_id = @tenant_id AND key = @key;

-- name: CreateReactionType :one
INSERT INTO reaction_types (tenant_id, key, emoji, label, enabled, position)
VALUES (@tenant_id, @key, @emoji, @label, @enabled, @position)
RETURNING tenant_id, key, emoji, label, enabled, position, created_at, updated_at;

-- name: UpdateReactionType :one
UPDATE reaction_types
SET emoji = @emoji, label = @label, enabled = @enabled, position = @position, updated_at = now()
WHERE tenant_id = @tenant_id AND key = @key
RETURNING tenant_id, key, emoji, label, enabled, position, created_at, updated_at;

-- name: DeleteReactionType :exec
DELETE FROM reaction_types WHERE tenant_id = @tenant_id AND key = @key;

-- =====================
-- Reactions
-- =====================

-- name: AddReaction :one
INSERT INTO reactions (tenant_id, entity_type, entity_id, user_id, reaction_key)
VALUES (@tenant_id, @entity_type, @entity_id, @user_id, @reaction_key)
ON CONFLICT (tenant_id, entity_type, entity_id, user_id, reaction_key) DO NOTHING
RETURNING tenant_id, entity_type, entity_id, user_id, reaction_key, created_at;

-- name: RemoveReaction :exec
DELETE FROM reactions
WHERE tenant_id = @tenant_id AND entity_type = @entity_type AND entity_id = @entity_id AND user_id = @user_id AND reaction_key = @reaction_key;

-- name: ListReactionsByEntity :many
SELECT tenant_id, entity_type, entity_id, user_id, reaction_key, created_at
FROM reactions
WHERE tenant_id = @tenant_id AND entity_type = @entity_type AND entity_id = @entity_id
ORDER BY created_at DESC;

-- name: CountReactionsByEntity :many
SELECT reaction_key, COUNT(*) AS count
FROM reactions
WHERE tenant_id = @tenant_id AND entity_type = @entity_type AND entity_id = @entity_id
GROUP BY reaction_key
ORDER BY count DESC;

-- name: GetUserReactions :many
SELECT tenant_id, entity_type, entity_id, user_id, reaction_key, created_at
FROM reactions
WHERE tenant_id = @tenant_id AND entity_type = @entity_type AND entity_id = @entity_id AND user_id = @user_id;
