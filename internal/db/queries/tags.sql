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
