-- name: ListContentForSitemap :many
SELECT id, slug, updated_at, content_type, status, tenant_id
FROM contents
WHERE tenant_id = @tenant_id
  AND status = 'published'
ORDER BY updated_at DESC;

-- name: GetPageMetadataBySlug :one
SELECT c.id, c.tenant_id, c.title, COALESCE(c.title, '') AS description,
       c.slug, c.content_type, '' AS og_image,
       COALESCE(c.canonical_url, '') AS canonical_url,
       COALESCE(c.title, '') AS og_title,
       COALESCE(c.title, '') AS og_description,
       COALESCE(up.display_name, u.username, '') AS author,
       COALESCE(c.published_at, c.created_at) AS published_at,
       c.updated_at
FROM contents c
LEFT JOIN users u ON c.author_id = u.id
LEFT JOIN user_profiles up ON u.id = up.user_id AND u.tenant_id = up.tenant_id
WHERE c.slug = @slug AND c.tenant_id = @tenant_id AND c.status = 'published';