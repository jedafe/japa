-- name: GetContentSEOByID :one
-- Admin SEO validation (Section 26): fetches a single content row by id with
-- render-time resolution. NOTE: the ValidateSEO handler currently uses an
-- equivalent inline query (see internal/seo/sitemap.go) because it also needs
-- engagement counts this query does not project. Keep the two in sync.
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
