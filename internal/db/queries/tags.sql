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
