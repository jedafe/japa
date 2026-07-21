-- General cross-domain tags (Section 08/49 catalog: GET /tags, /tags/:slug).
-- Distinct from forum_tags (forum-scoped) — this is the tags table content
-- is tagged against via content_tags, and the entity followed via follows
-- (entity_type='tag').

-- name: ListTagsByTenant :many
SELECT id, tenant_id, name, slug, description, color, follower_count, created_at, updated_at
FROM tags
WHERE tenant_id = @tenant_id
ORDER BY name;

-- name: GetTagBySlug :one
SELECT id, tenant_id, name, slug, description, color, follower_count, created_at, updated_at
FROM tags
WHERE tenant_id = @tenant_id AND slug = @slug;

-- Admin CRUD (catalog: POST/PATCH/DELETE /admin/tags).

-- name: CreateTag :one
INSERT INTO tags (tenant_id, name, slug, description, color)
VALUES (@tenant_id, @name, @slug, @description, @color)
RETURNING id, tenant_id, name, slug, description, color, follower_count, created_at, updated_at;

-- name: UpdateTag :one
UPDATE tags
SET name = @name, description = @description, color = @color, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, name, slug, description, color, follower_count, created_at, updated_at;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Tag moderators (Section 49)
-- =====================

-- name: ListTagModerators :many
-- Lists the moderators of a tag, joined to users for display.
SELECT tm.user_id, u.username, tm.created_at
FROM tag_moderators tm
JOIN users u ON u.id = tm.user_id AND u.tenant_id = tm.tenant_id
WHERE tm.tag_id = @tag_id AND tm.tenant_id = @tenant_id
ORDER BY tm.created_at ASC;

-- name: AddTagModerator :one
-- Idempotent: re-adding an existing moderator just refreshes created_at.
INSERT INTO tag_moderators (tenant_id, tag_id, user_id, assigned_by)
VALUES (@tenant_id, @tag_id, @user_id, @assigned_by)
ON CONFLICT (tenant_id, tag_id, user_id) DO UPDATE SET created_at = now()
RETURNING id, tenant_id, tag_id, user_id, assigned_by, assigned_by_user_id, created_at;

-- name: RemoveTagModerator :exec
DELETE FROM tag_moderators
WHERE tag_id = @tag_id AND user_id = @user_id AND tenant_id = @tenant_id;
