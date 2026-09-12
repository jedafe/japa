-- name: ListContentForSitemap :many
SELECT id, slug, updated_at, content_type, status
FROM contents
WHERE status = 'published'
ORDER BY updated_at DESC;

-- name: GetPageMetadataBySlug :one
-- Render-time resolution (ADR-052): per-content SEO overrides take priority,
-- then the content title/body, then empty defaults. og_image_url falls back
-- to the featured image's media URL when blank — resolved by the caller via
-- the media join, not here (this query stays single-row).
-- og_description falls back to '' (not the title) per Section 26
-- ResolveMetadata: the caller resolves the body-excerpt fallback via
-- excerpt(body, 160) when og_description is empty. Using the title as the
-- OG description is wrong — it duplicates og_title and is useless for
-- social sharing.
SELECT c.id, COALESCE(c.meta_title, c.title) AS title,
 COALESCE(c.meta_description, '') AS description,
 c.slug, c.content_type,
 COALESCE(c.og_image_url, '') AS og_image,
 COALESCE(c.canonical_url, '') AS canonical_url,
 COALESCE(c.meta_title, c.title) AS og_title,
 COALESCE(c.meta_description, '') AS og_description,
 c.seo_indexable,
 COALESCE(up.display_name, u.username, '') AS author,
 COALESCE(c.published_at, c.created_at) AS published_at,
 c.updated_at
FROM contents c
LEFT JOIN users u ON c.author_id = u.id
LEFT JOIN user_profiles up ON u.id = up.user_id
WHERE c.slug = @slug AND c.status = 'published';

-- name: GetContentSEOByID :one
-- Admin SEO validation (Section 26): fetches a single content row by id with
-- the same render-time resolution as GetPageMetadataBySlug, plus the raw
-- title/slug/body/visibility the validator needs to compute issues and score.
-- Unlike GetPageMetadataBySlug this does NOT filter by status — admins must be
-- able to validate drafts too.
SELECT c.id, c.title, c.slug, c.content_type, c.body, c.visibility, c.seo_indexable,
 COALESCE(c.meta_title, '') AS meta_title,
 COALESCE(c.meta_description, '') AS meta_description,
 COALESCE(c.og_image_url, '') AS og_image_url,
 COALESCE(c.canonical_url, '') AS canonical_url,
 COALESCE(c.meta_title, c.title) AS og_title,
 COALESCE(c.meta_description, '') AS og_description,
 COALESCE(jt.template, '{}'::jsonb) AS jsonld_template
FROM contents c
LEFT JOIN content_type_jsonld_templates jt
 ON jt.content_type = c.content_type
 AND jt.is_active = true
WHERE c.id = @content_id;