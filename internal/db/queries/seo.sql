-- name: ListContentForSitemap :many
SELECT id, slug, updated_at, content_type, status, tenant_id
FROM contents
WHERE tenant_id = @tenant_id
  AND status = 'published'
ORDER BY updated_at DESC;

-- name: GetPageMetadataBySlug :one
-- Render-time resolution (ADR-052): per-content SEO overrides take priority,
-- then the content title/body, then empty defaults. og_image_url falls back
-- to the featured image's media URL when blank — resolved by the caller via
-- the media join, not here (this query stays single-row).
SELECT c.id, c.tenant_id,
       COALESCE(c.meta_title, c.title) AS title,
       COALESCE(c.meta_description, '') AS description,
       c.slug, c.content_type,
       COALESCE(c.og_image_url, '') AS og_image,
       COALESCE(c.canonical_url, '') AS canonical_url,
       COALESCE(c.meta_title, c.title) AS og_title,
       COALESCE(c.meta_description, c.title) AS og_description,
       c.seo_indexable,
       COALESCE(up.display_name, u.username, '') AS author,
       COALESCE(c.published_at, c.created_at) AS published_at,
       c.updated_at
FROM contents c
LEFT JOIN users u ON c.author_id = u.id
LEFT JOIN user_profiles up ON u.id = up.user_id AND u.tenant_id = up.tenant_id
WHERE c.slug = @slug AND c.tenant_id = @tenant_id AND c.status = 'published';