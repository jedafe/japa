-- name: CreateContent :one
INSERT INTO contents (author_id, organization_id, space_id, content_type, title, slug, body, body_html, field_values, content_hash, geo_space_id, header_image_id, featured_image_id, canonical_url, meta_title, meta_description, og_image_url, seo_indexable, status, visibility, read_time_minutes, publish_at, published_at, guide_category, ai_generated, ai_model, ai_generated_at, hitl_reviewed_by, hitl_reviewed_at, journey_stage, visa_route, source_country, target_country, video_source_id, intel_source_id, intel_record_id) VALUES (@author_id, sqlc.narg(organization_id), sqlc.narg(space_id), @content_type, @title, @slug, @body, @body_html, @field_values, sqlc.narg(content_hash), sqlc.narg(geo_space_id), sqlc.narg(header_image_id), sqlc.narg(featured_image_id), sqlc.narg(canonical_url), sqlc.narg(meta_title), sqlc.narg(meta_description), sqlc.narg(og_image_url), @seo_indexable, @status, @visibility, @read_time_minutes, sqlc.narg(publish_at), sqlc.narg(published_at), sqlc.narg(guide_category), COALESCE(sqlc.narg(ai_generated), false), sqlc.narg(ai_model), sqlc.narg(ai_generated_at), sqlc.narg(hitl_reviewed_by), sqlc.narg(hitl_reviewed_at), sqlc.narg(journey_stage), sqlc.narg(visa_route), sqlc.narg(source_country), sqlc.narg(target_country), sqlc.narg(video_source_id), sqlc.narg(intel_source_id), sqlc.narg(intel_record_id))
RETURNING id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
 field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
 canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
 status, visibility, is_locked, is_pinned, read_time_minutes,
 view_count, comment_count, share_count, reshare_count, publish_at, published_at,
 created_at, updated_at, guide_category,
 ai_generated, ai_model, ai_generated_at, hitl_reviewed_by, hitl_reviewed_at,
 journey_stage, visa_route, source_country, target_country, video_source_id,
 intel_source_id, intel_record_id;

-- name: GetContentByID :one
SELECT id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
 field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
 canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
 status, visibility, is_locked, is_pinned, read_time_minutes,
 view_count, comment_count, share_count, reshare_count, publish_at, published_at,
 created_at, updated_at, guide_category,
 ai_generated, ai_model, ai_generated_at, hitl_reviewed_by, hitl_reviewed_at,
 journey_stage, visa_route, source_country, target_country, video_source_id,
 intel_source_id, intel_record_id, auto_hidden
FROM contents
WHERE id = @id AND deleted_at IS NULL;

-- name: GetContentByIntelRecord :one
SELECT c.id, c.title, c.slug, c.content_type, c.body, c.meta_description, c.canonical_url,
 COALESCE(cs.view_count, c.view_count, 0) AS view_count,
 COALESCE(cs.comment_count, c.comment_count, 0) AS comment_count,
 c.share_count, c.reshare_count,
 COALESCE(cs.vote_score, 0) AS vote_score,
 c.publish_at, c.published_at, c.created_at, c.updated_at, c.intel_source_id, c.intel_record_id
FROM contents c
LEFT JOIN content_scores cs ON cs.content_id = c.id
WHERE c.intel_record_id = @intel_record_id AND c.deleted_at IS NULL;

-- name: GetContentByHash :one
-- Duplicate-detection check (Rule 12 / BL-006). Returns a conflicting row
-- with the same (content_hash), excluding the row being updated.
SELECT id, author_id, content_hash
FROM contents
WHERE content_hash = @content_hash
 AND deleted_at IS NULL
 AND id <> @exclude_id;

-- name: GetContentBySlug :one
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count,
 (SELECT COUNT(*)
 FROM comments
 WHERE content_id = c.id
 AND deleted_at IS NULL
 AND status = 'published') AS comment_count,
 c.share_count,
 (SELECT COUNT(*) FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reaction_count,
 c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country, c.video_source_id
FROM contents c
WHERE c.slug = @slug AND c.deleted_at IS NULL;

-- name: GetContentDetailBySlug :one
-- Enriched single-content row for the public /content/[slug] reading page.
-- Mirrors GetBlogPostDetail (blog.sql) — author name/username/avatar, space
-- name/slug, tag slugs, featured + header image storage refs, and blog_series
-- navigation (prev/next) — but without the content_type restriction so every
-- CPT single (guide, page, comparison, discussion, …) gets the same fields.
SELECT c.id, c.author_id, c.space_id, c.organization_id,
 c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count,
 (SELECT COUNT(*)
 FROM comments
 WHERE content_id = c.id
 AND deleted_at IS NULL
 AND status = 'published') AS comment_count,
 c.share_count,
 (SELECT COUNT(*) FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reaction_count,
 (SELECT COUNT(*)::int FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reaction_count,
 c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country, c.video_source_id,
 c.intel_source_id, c.intel_record_id,
 src.id AS source_id, src.name AS source_name, src.slug AS source_slug, src.logo_url AS source_logo_url, src.source_url AS source_source_url, src.trust_level AS source_trust_level,
 COALESCE(up.display_name, u.public_username, u.username, '') AS author_name,
 COALESCE(u.public_username, u.username, '') AS author_username,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 m.storage_bucket AS featured_storage_bucket,
 m.storage_path AS featured_storage_path,
 hm.storage_bucket AS header_storage_bucket,
 hm.storage_path AS header_storage_path,
 COALESCE(string_agg(DISTINCT t.slug, ',' ORDER BY t.slug), '')::text AS tags_csv,
 bs.id AS series_id,
 bs.title AS series_title,
 bs.slug AS series_slug,
 bsp.position AS series_part,
 (SELECT COUNT(*) FROM blog_series_posts WHERE series_id = bs.id) AS series_total,
 COALESCE(prev_c.slug, '') AS series_prev_slug,
 COALESCE(next_c.slug, '') AS series_next_slug
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN intel_sources src ON src.id = c.intel_source_id
LEFT JOIN media m ON m.id = c.featured_image_id AND m.deleted_at IS NULL
LEFT JOIN media hm ON hm.id = c.header_image_id AND hm.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN blog_series_posts bsp ON bsp.content_id = c.id
LEFT JOIN blog_series bs ON bs.id = bsp.series_id AND bs.deleted_at IS NULL
LEFT JOIN LATERAL (
 SELECT c3.slug
 FROM blog_series_posts bsp4
 JOIN contents c3 ON c3.id = bsp4.content_id AND c3.deleted_at IS NULL
 WHERE bsp4.series_id = bs.id AND bsp4.position < bsp.position
 ORDER BY bsp4.position DESC
 LIMIT 1
) prev_c ON true
LEFT JOIN LATERAL (
 SELECT c2.slug
 FROM blog_series_posts bsp3
 JOIN contents c2 ON c2.id = bsp3.content_id AND c2.deleted_at IS NULL
 WHERE bsp3.series_id = bs.id AND bsp3.position > bsp.position
 ORDER BY bsp3.position ASC
 LIMIT 1
) next_c ON true
WHERE c.slug = @slug AND c.deleted_at IS NULL
GROUP BY c.id, up.display_name, up.avatar_url, u.username, u.public_username, s.name, s.slug,
 sm.storage_bucket, sm.storage_path, o.name, o.slug, om.storage_bucket, om.storage_path,
 m.storage_bucket, m.storage_path, hm.storage_bucket, hm.storage_path,
 bs.id, bs.title, bs.slug, bsp.position,
 prev_c.slug, next_c.slug, c.video_source_id,
 c.intel_source_id, c.intel_record_id,
 src.id, src.name, src.slug, src.logo_url, src.source_url, src.trust_level;

-- name: GetContentDetailBySlugAndType :one
-- Enriched single-content row for the public /post/{content_type}/{slug} social
-- page. Disambiguates by content_type so the same slug can exist for multiple
-- content types. Mirrors GetContentDetailBySlug with org/space/video details.
SELECT c.id, c.author_id, c.space_id, c.organization_id,
 c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count,
 (SELECT COUNT(*)
 FROM comments
 WHERE content_id = c.id
 AND deleted_at IS NULL
 AND status = 'published') AS comment_count,
 c.share_count,
 c.reshare_count,
 (SELECT COUNT(*)::int FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reaction_count,
 c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country, c.video_source_id,
 COALESCE(up.display_name, u.public_username, u.username, '') AS author_name,
 COALESCE(u.public_username, u.username, '') AS author_username,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 m.storage_bucket AS featured_storage_bucket,
 m.storage_path AS featured_storage_path,
 hm.storage_bucket AS header_storage_bucket,
 hm.storage_path AS header_storage_path,
 o.name AS organization_name,
 o.slug AS organization_slug,
 om.storage_bucket AS organization_logo_storage_bucket,
 om.storage_path AS organization_logo_storage_path,
 COALESCE(string_agg(DISTINCT t.slug, ',' ORDER BY t.slug), '')::text AS tags_csv,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 bs.id AS series_id,
 bs.title AS series_title,
 bs.slug AS series_slug,
 bsp.position AS series_part,
 (SELECT COUNT(*) FROM blog_series_posts WHERE series_id = bs.id) AS series_total,
 COALESCE(prev_c.slug, '') AS series_prev_slug,
 COALESCE(next_c.slug, '') AS series_next_slug
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN intel_sources src ON src.id = c.intel_source_id
LEFT JOIN media m ON m.id = c.featured_image_id AND m.deleted_at IS NULL
LEFT JOIN media hm ON hm.id = c.header_image_id AND hm.deleted_at IS NULL
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN blog_series_posts bsp ON bsp.content_id = c.id
LEFT JOIN blog_series bs ON bs.id = bsp.series_id AND bs.deleted_at IS NULL
LEFT JOIN LATERAL (
 SELECT c3.slug
 FROM blog_series_posts bsp4
 JOIN contents c3 ON c3.id = bsp4.content_id AND c3.deleted_at IS NULL
 WHERE bsp4.series_id = bs.id AND bsp4.position < bsp.position
 ORDER BY bsp4.position DESC
 LIMIT 1
) prev_c ON true
LEFT JOIN LATERAL (
 SELECT c2.slug
 FROM blog_series_posts bsp3
 JOIN contents c2 ON c2.id = bsp3.content_id AND c2.deleted_at IS NULL
 WHERE bsp3.series_id = bs.id AND bsp3.position > bsp.position
 ORDER BY bsp3.position ASC
 LIMIT 1
) next_c ON true
WHERE c.slug = @slug AND c.content_type = @content_type AND c.deleted_at IS NULL
GROUP BY c.id, up.display_name, up.avatar_url, u.username, u.public_username, s.name, s.slug,
 sm.storage_bucket, sm.storage_path,
 m.storage_bucket, m.storage_path, hm.storage_bucket, hm.storage_path,
 o.name, o.slug, om.storage_bucket, om.storage_path,
 bs.id, bs.title, bs.slug, bsp.position,
 prev_c.slug, next_c.slug, c.video_source_id,
 vs.source_url, vs.source_type, vs.duration_secs, vs.thumbnail_url;

-- name: ContentSlugExists :one
-- Returns true if a live contents row uses the given slug. Mirrors the
-- partial unique index (000745): soft-deleted rows no longer occupy slugs.
SELECT EXISTS(SELECT 1 FROM contents WHERE slug = @slug AND deleted_at IS NULL);

-- name: ListContents :many
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.field_values->>'purpose' AS purpose,
 EXISTS(SELECT 1 FROM content_featured cf WHERE cf.content_id = c.id
 AND (cf.featured_until IS NULL OR cf.featured_until > now())) AS is_featured,
 COALESCE(u.public_username, u.username, '') AS author_username, up.display_name AS author_display_name,
 s.name AS space_name
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
WHERE c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
 AND (@q::text IS NULL OR c.title ILIKE '%' || @q || '%' OR c.slug ILIKE '%' || @q || '%' OR c.body ILIKE '%' || @q || '%')
 AND (c.created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- name: ListContentsByIDs :many
-- Fetches content rows by a list of IDs with the same JOINed enrichment
-- as ListContents. Used by the Meilisearch search integration:
-- when ?q= is present, the handler queries Meilisearch for matching IDs,
-- then uses this query to fetch the full rows with author/space names.
-- Ref: docs/PERFORMANCE_AUDIT.md §4 (Search Offload)
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.field_values->>'purpose' AS purpose,
 COALESCE(u.public_username, u.username, '') AS author_username, up.display_name AS author_display_name,
 s.name AS space_name
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
WHERE c.id = ANY(@ids::uuid[])
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- name: ListContentsBySlugs :many
-- Fetches published content rows by a list of slugs with the same JOINed
-- enrichment as the other feed queries. Used by the journey-aware roadmap feed
-- to render the next steps from a readiness assessment in slug order, paginated.
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.field_values->>'purpose' AS purpose,
 COALESCE(u.public_username, u.username, '') AS author_username,
 up.display_name AS author_display_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 s.name AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 COALESCE(cs.vote_score, 0) AS vote_score,
 COALESCE(cs.bookmark_count, 0) AS bookmark_count,
 COALESCE(cs.velocity_score, 0) AS velocity_score,
 LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400)::text AS excerpt,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE((SELECT string_agg(DISTINCT t.name, ',' ORDER BY t.name)
 FROM content_tags ct
 JOIN tags t ON t.id = ct.tag_id
 WHERE ct.content_id = c.id), '') AS tags
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id) AND m.deleted_at IS NULL
WHERE c.slug = ANY(@slugs::text[])
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
ORDER BY array_position(@slugs::text[], c.slug)
LIMIT @page_limit OFFSET @page_offset;

-- name: ListContentsByType :many
-- Lists content filtered by content_type with optional guide_category.
-- Supports composite cursor (created_at, id) so pages with identical timestamps
-- are not skipped by timestamp-only cursor pagination.
-- @sort 'popular' orders by view_count; 'roadmap' ranks by the visitor's
-- journey-context signals (v_* params from the resolved VisitorContext —
-- stage/destination/visa/origin matches first, then canonical §87 stage
-- order) so the /guides Roadmap tab surfaces the most relevant guides per
-- stage. Any other value keeps created_at order. page_offset paginates
-- non-cursor sorts.
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.field_values->>'purpose' AS purpose,
 EXISTS(SELECT 1 FROM content_featured cf WHERE cf.content_id = c.id
 AND (cf.featured_until IS NULL OR cf.featured_until > now())) AS is_featured,
 COALESCE(u.public_username, u.username, '') AS author_username, up.display_name AS author_display_name,
 s.name AS space_name
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
WHERE c.content_type = @content_type
 AND (@guide_category::text = '' OR c.guide_category = @guide_category::text)
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
 AND (
 (@cursor_created_at::timestamptz IS NULL)
 OR (c.created_at < @cursor_created_at::timestamptz)
 OR (c.created_at = @cursor_created_at::timestamptz AND (@cursor_id::uuid IS NULL OR c.id < @cursor_id))
 )
ORDER BY
 CASE WHEN @sort::text = 'roadmap' THEN
 (CASE WHEN @v_stage::text <> '' AND c.journey_stage = @v_stage::text THEN 8 ELSE 0 END) +
 (CASE WHEN @v_dest::text <> '' AND c.target_country = @v_dest::text THEN 4 ELSE 0 END) +
 (CASE WHEN @v_visa::text <> '' AND c.visa_route = @v_visa::text THEN 2 ELSE 0 END) +
 (CASE WHEN @v_origin::text <> '' AND c.source_country = @v_origin::text THEN 1 ELSE 0 END)
 END DESC,
 CASE WHEN @sort::text = 'roadmap' THEN
 CASE c.journey_stage
 WHEN 'considering' THEN 0 WHEN 'planning' THEN 1 WHEN 'preparing' THEN 2
 WHEN 'applied' THEN 3 WHEN 'approved' THEN 4 WHEN 'relocated' THEN 5
 WHEN 'settled' THEN 6 ELSE 7
 END
 END ASC,
 CASE WHEN @sort::text = 'popular' THEN c.view_count END DESC NULLS LAST,
 c.created_at DESC,
 c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListFeaturedContentsByType :many
-- Featured rows from the generic content_featured junction, for the
-- Featured tabs on /guides, /stories, /forum (Section 14 — generic
-- featured list). Optional content_type filter; ordered by
-- featured_order then created_at. Same visibility gating as the list.
SELECT cf.id AS featured_id, cf.featured_order, cf.featured_until,
 c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.field_values->>'purpose' AS purpose,
 COALESCE(ft.vote_score, cs.vote_score, 0) AS vote_score,
 COALESCE(u.public_username, u.username, '') AS author_username, up.display_name AS author_display_name,
 s.name AS space_name
FROM content_featured cf
JOIN contents c ON c.id = cf.content_id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN forum_threads ft ON ft.content_id = c.id AND ft.deleted_at IS NULL
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
WHERE (cf.featured_until IS NULL OR cf.featured_until > now())
 AND (@content_type::text = '' OR c.content_type = @content_type::text)
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
ORDER BY cf.featured_order ASC, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListContentsBySpace :many
-- Lists content assigned to a given space via the authoritative
-- content_spaces table (BUG-010), with visibility gating. The space name
-- returned is the requested space, regardless of whether it is the primary.
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.field_values->>'purpose' AS purpose,
 EXISTS(SELECT 1 FROM content_featured cf WHERE cf.content_id = c.id
 AND (cf.featured_until IS NULL OR cf.featured_until > now())) AS is_featured,
 COALESCE(u.public_username, u.username, '') AS author_username, up.display_name AS author_display_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 s.name AS space_name,
 EXISTS(SELECT 1 FROM space_pinned_posts spp
  WHERE spp.space_id = @space_id AND spp.content_id = c.id) AS is_pinned
FROM contents c
JOIN content_spaces cs ON cs.content_id = c.id AND cs.space_id = @space_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = cs.space_id
WHERE c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
 AND (c.created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- name: SearchContentsBySpace :many
-- Searches content assigned to a given space via the authoritative
-- content_spaces table (BUG-010), with visibility gating.
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country
FROM contents c
JOIN content_spaces cs ON cs.content_id = c.id AND cs.space_id = @space_id
WHERE c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
 AND c.title ILIKE '%' || @query || '%'
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: SearchContentsByTypeAndSpace :many
-- Searches content of a given content_type assigned to a space via the
-- authoritative content_spaces table (BUG-010), with visibility gating.
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country
FROM contents c
JOIN content_spaces cs ON cs.content_id = c.id AND cs.space_id = @space_id
WHERE c.content_type = @content_type
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
 AND c.title ILIKE '%' || @query || '%'
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateContent :one
UPDATE contents
SET title = @title, slug = @slug, body = @body, body_html = @body_html,
 field_values = @field_values, content_hash = COALESCE(sqlc.narg(content_hash), content_hash),
 geo_space_id = sqlc.narg(geo_space_id), canonical_url = sqlc.narg(canonical_url),
 header_image_id = sqlc.narg(header_image_id),
 featured_image_id = sqlc.narg(featured_image_id),
 meta_title = sqlc.narg(meta_title),
 meta_description = sqlc.narg(meta_description),
 og_image_url = sqlc.narg(og_image_url),
 seo_indexable = COALESCE(sqlc.narg(seo_indexable), seo_indexable),
 status = @status, visibility = @visibility, read_time_minutes = @read_time_minutes,
 publish_at = sqlc.narg(publish_at), published_at = sqlc.narg(published_at),
 is_locked = @is_locked, is_pinned = @is_pinned,
 organization_id = sqlc.narg(organization_id), guide_category = COALESCE(sqlc.narg(guide_category), guide_category),
 ai_generated_at = COALESCE(sqlc.narg(ai_generated_at), ai_generated_at),
 ai_model = COALESCE(sqlc.narg(ai_model), ai_model),
 journey_stage = COALESCE(sqlc.narg(journey_stage), journey_stage),
 visa_route = COALESCE(sqlc.narg(visa_route), visa_route),
 source_country = COALESCE(sqlc.narg(source_country), source_country),
 target_country = COALESCE(sqlc.narg(target_country), target_country),
 video_source_id = COALESCE(sqlc.narg(video_source_id), video_source_id),
 intel_source_id = COALESCE(sqlc.narg(intel_source_id), intel_source_id),
 intel_record_id = COALESCE(sqlc.narg(intel_record_id), intel_record_id),
 updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
 field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
 canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
 status, visibility, is_locked, is_pinned, read_time_minutes,
 view_count, comment_count, share_count, reshare_count, publish_at, published_at,
 created_at, updated_at, guide_category,
 journey_stage, visa_route, source_country, target_country, video_source_id,
 intel_source_id, intel_record_id;

-- name: UpdateContentStatus :one
UPDATE contents
SET status = @status, updated_at = now(),
 published_at = CASE WHEN @status = 'published' AND published_at IS NULL THEN now() ELSE published_at END
WHERE id = @id AND deleted_at IS NULL
RETURNING id, author_id, space_id, content_type, title, slug, body, body_html,
 field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
 canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
 status, visibility, is_locked, is_pinned, read_time_minutes,
 view_count, comment_count, share_count, reshare_count, publish_at, published_at,
 created_at, updated_at, guide_category,
 journey_stage, visa_route, source_country, target_country;

-- name: UpdateContentIntelSourceID :exec
-- Links a content row to its canonical intel_sources row. Used when a
-- comparison data source is attached to a comparison content.
UPDATE contents
SET intel_source_id = @intel_source_id, updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: ListComparisonContentsBySourceID :many
-- Source item feed: comparison contents linked to a canonical source.
SELECT c.id, c.title, c.slug, c.body, c.content_type,
 c.status, c.visibility, c.published_at, c.created_at, c.updated_at,
 c.view_count, c.comment_count, c.share_count,
 (SELECT COUNT(*)::int FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS vote_score,
 COALESCE(u.public_username, u.username, '') AS author_username
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
WHERE c.intel_source_id = @intel_source_id
 AND c.content_type = 'comparison'
 AND c.deleted_at IS NULL
 AND c.status = 'published'
 AND c.visibility = 'public'
ORDER BY
 CASE
 WHEN @sort_mode::text = 'popular' THEN c.view_count
 WHEN @sort_mode::text = 'discussed' THEN c.comment_count
 WHEN @sort_mode::text = 'upvoted' THEN
  (SELECT COUNT(*)::int FROM reactions WHERE entity_id = c.id AND entity_type = 'content')
 END DESC NULLS LAST,
 c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountComparisonContentsBySourceID :one
SELECT COUNT(*)::int AS total
FROM contents
WHERE intel_source_id = @intel_source_id
 AND content_type = 'comparison'
 AND deleted_at IS NULL
 AND status = 'published'
 AND visibility = 'public';

-- name: ReassignSingleContentAuthor :one
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Authorship Assignment)
-- Updates a single content row's author_id and field_values in one
-- statement. The caller (agent handler) reads the current field_values,
-- merges the _ai_provenance.author_assigned_by / author_assignment_reason
-- metadata in Go, and passes the full updated JSONB here. Platform-scoped
-- for defense-in-depth (BUG-014). Distinct from gdpr.sql's
-- ReassignContentAuthor (:exec bulk reassign on user deletion).
UPDATE contents
SET author_id = @author_id, field_values = @field_values, updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING id, author_id, space_id, content_type, title, slug, body, body_html,
 field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
 canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
 status, visibility, is_locked, is_pinned, read_time_minutes,
 view_count, comment_count, share_count, reshare_count, publish_at, published_at,
 created_at, updated_at, guide_category,
 journey_stage, visa_route, source_country, target_country;

-- name: ScheduleContent :one
-- Sets status='scheduled' and publish_at without touching title/body/other
-- fields (catalog: POST /content/:id/schedule). The scheduled_publish Asynq
-- task (enqueued by the caller with ProcessAt=publish_at) flips status to
-- 'published' when the time arrives.
UPDATE contents
SET status = 'scheduled', publish_at = @publish_at, updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING id, author_id, space_id, content_type, title, slug, body, body_html,
 field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
 canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
 status, visibility, is_locked, is_pinned, read_time_minutes,
 view_count, comment_count, share_count, reshare_count, publish_at, published_at,
 created_at, updated_at, guide_category,
 journey_stage, visa_route, source_country, target_country;

-- name: IncrementContentView :exec
UPDATE contents SET view_count = view_count + 1 WHERE id = @id AND deleted_at IS NULL;

-- name: RecordContentView :execresult
-- Inserts a deduplicated view record into user_content_views (Section 14).
-- The PK deduplicates per user/session per day; ON CONFLICT DO NOTHING.
-- country_code is resolved server-side from the client IP via GeoIP.
-- Returns the command tag so callers can detect whether a row was actually
-- inserted (RowsAffected == 1) vs. deduplicated (RowsAffected == 0).
INSERT INTO user_content_views (content_id, user_id, session_id, country_code, viewed_at) VALUES (@content_id, @user_id, @session_id, @country_code, now())

ON CONFLICT DO NOTHING;

-- name: IncrementContentCommentCount :exec
-- Increments the denormalized comment_count on a content row. Called
-- synchronously when a comment is created so feed cards (which read
-- c.comment_count) show accurate counts without waiting for the async
-- scoring pipeline. Ref: Section 32 (feed reads contents.comment_count).
UPDATE contents SET comment_count = comment_count + 1
WHERE id = @id AND deleted_at IS NULL;

-- name: DecrementContentCommentCount :exec
-- Decrements comment_count when a comment is soft-deleted/removed.
UPDATE contents SET comment_count = GREATEST(comment_count - 1, 0)
WHERE id = @id AND deleted_at IS NULL;

-- name: DeleteContent :exec
UPDATE contents SET deleted_at = now(), updated_at = now() WHERE id = @id AND deleted_at IS NULL;

-- name: ContentExistsAny :one
SELECT EXISTS(SELECT 1 FROM contents WHERE id = @id);

-- name: GetContentSlugTypeAny :one
-- slug + content_type for any row including soft-deleted — the hard-delete
-- path needs them to purge the canonical URL at the edge (ADR-134);
-- GetContentByID filters deleted_at and would miss the row.
SELECT slug, content_type FROM contents WHERE id = @id;

-- name: HardDeleteContent :execrows
-- Admin force-delete — only rows already soft-deleted are removed
-- (gated by admin.hard_delete via ?force=true). Children cascade via
-- ON DELETE CASCADE; NO ACTION dependents surface as FK violations.
DELETE FROM contents WHERE id = @id AND deleted_at IS NOT NULL;

-- =====================
-- Content Revisions
-- =====================

-- name: CreateContentRevision :one
INSERT INTO content_revisions (content_id, field_values, title, body, body_html, edited_by) VALUES (@content_id, @field_values, @title, @body, @body_html, @edited_by)

RETURNING id, content_id, field_values, title, body, body_html, edited_by, created_at;

-- name: ListContentRevisions :many
SELECT cr.id, cr.content_id, cr.field_values, cr.title, cr.body, cr.body_html, cr.edited_by, cr.created_at
FROM content_revisions cr
JOIN contents c ON c.id = cr.content_id
WHERE cr.content_id = @content_id
ORDER BY cr.created_at DESC, cr.id DESC;

-- name: GetContentRevision :one
SELECT cr.id, cr.content_id, cr.field_values, cr.title, cr.body, cr.body_html, cr.edited_by, cr.created_at
FROM content_revisions cr
JOIN contents c ON c.id = cr.content_id
WHERE cr.id = @id
 AND cr.content_id = @content_id;

-- =====================
-- Slug History
-- =====================

-- name: CreateSlugHistory :one
INSERT INTO slug_history (entity_type, entity_id, old_slug, new_slug) VALUES (@entity_type, @entity_id, @old_slug, @new_slug)

RETURNING id, entity_type, entity_id, old_slug, new_slug, created_at;

-- name: GetSlugRedirect :one
SELECT id, entity_type, entity_id, old_slug, new_slug, created_at
FROM slug_history
WHERE entity_type = @entity_type AND old_slug = @old_slug
ORDER BY created_at DESC
LIMIT 1;

-- =====================
-- Space Pinned Posts
-- =====================

-- name: PinContentToSpace :one
INSERT INTO space_pinned_posts (space_id, content_id, pinned_by, position) VALUES (@space_id, @content_id, @pinned_by, @position)

RETURNING id, space_id, content_id, pinned_by, position, created_at;

-- name: UnpinContentFromSpace :exec
DELETE FROM space_pinned_posts WHERE space_id = @space_id AND content_id = @content_id;

-- name: ListPinnedContent :many
SELECT spp.id, spp.space_id, spp.content_id, spp.pinned_by, spp.position, spp.created_at
FROM space_pinned_posts spp
WHERE spp.space_id = @space_id
ORDER BY spp.position ASC, spp.created_at ASC;

-- name: ListPinnedContentBySpace :many
-- Returns the pinned content rows for a space (space_pinned_posts joined to
-- contents) in pin order, with the same visibility gating as
-- ListContentsBySpace so guests never see members/private pins. is_pinned is
-- true by definition for every returned row. The space name returned is the
-- requested space (BUG-010 junction semantics).
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at, c.guide_category, c.journey_stage, c.visa_route, c.source_country, c.target_country,
 c.field_values->>'purpose' AS purpose,
 COALESCE(u.public_username, u.username, '') AS author_username, up.display_name AS author_display_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 s.name AS space_name,
 true AS is_pinned
FROM space_pinned_posts spp
JOIN contents c ON c.id = spp.content_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = spp.space_id
WHERE spp.space_id = @space_id
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
ORDER BY spp.position ASC, spp.created_at ASC;

-- name: IsContentPinned :one
SELECT EXISTS(SELECT 1 FROM space_pinned_posts WHERE space_id = @space_id AND content_id = @content_id) AS exists;

-- name: IsContentInSpace :one
-- True when the content is assigned to the space via the authoritative
-- content_spaces junction (BUG-010).
SELECT EXISTS(SELECT 1 FROM content_spaces WHERE space_id = @space_id AND content_id = @content_id) AS exists;

-- =====================
-- JSON-LD Templates
-- =====================

-- name: GetJSONLDTemplate :one
SELECT id, content_type, template, is_active, created_at, updated_at
FROM content_type_jsonld_templates
WHERE content_type = @content_type AND is_active = true;

-- name: ListJSONLDTemplates :many
SELECT id, content_type, template, is_active, created_at, updated_at
FROM content_type_jsonld_templates
WHERE (@q::text = '' OR content_type ILIKE '%' || @q || '%' OR template::text ILIKE '%' || @q || '%')
ORDER BY created_at ASC, id ASC;

-- name: UpsertJSONLDTemplate :one
INSERT INTO content_type_jsonld_templates (content_type, template, is_active) VALUES (@content_type, @template, @is_active)

ON CONFLICT (content_type) DO UPDATE
SET template = EXCLUDED.template, is_active = EXCLUDED.is_active, updated_at = now()
RETURNING id, content_type, template, is_active, created_at, updated_at;

-- name: DeleteJSONLDTemplate :exec
DELETE FROM content_type_jsonld_templates WHERE id = @id;

-- =====================
-- Content Scores
-- =====================

-- name: UpsertContentScore :one
INSERT INTO content_scores (content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score) VALUES (@content_id, @vote_score, @comment_count, @view_count, @bookmark_count, @velocity_score)

ON CONFLICT (content_id) DO UPDATE
SET vote_score = EXCLUDED.vote_score, comment_count = EXCLUDED.comment_count,
 view_count = EXCLUDED.view_count, bookmark_count = EXCLUDED.bookmark_count,
 velocity_score = EXCLUDED.velocity_score, last_scored_at = now()
RETURNING content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score, reaction_count, relevance_embedding, last_scored_at;

-- name: EnsureContentScore :exec
-- Creates a zero-default content_scores row if one does not yet exist.
-- Called on content publish and by the engagement worker so that INNER JOIN
-- feed queries (explore/top/rising/space/profile) never exclude content for
-- lacking a score row, and so the worker can increment counts in place.
-- ON CONFLICT DO NOTHING preserves any existing scores.
INSERT INTO content_scores (content_id) VALUES (@content_id)

ON CONFLICT (content_id) DO NOTHING;

-- name: IncrementScoreVote :exec
-- Atomically increments vote_score by delta. Used by the engagement worker
-- to avoid the read-modify-write race that loses deltas when concurrent
-- events (vote, comment, bookmark, view) overwrite each other's columns.
UPDATE content_scores
SET vote_score = vote_score + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreComment :exec
-- Atomically adjusts comment_count by delta (clamped at 0).
UPDATE content_scores
SET comment_count = GREATEST(0, comment_count + @delta), last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreView :exec
-- Atomically increments view_count by delta.
UPDATE content_scores
SET view_count = view_count + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreBookmark :exec
-- Atomically increments bookmark_count by delta.
UPDATE content_scores
SET bookmark_count = bookmark_count + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreReaction :exec
-- Atomically adjusts reaction_count by delta (clamped at 0).
UPDATE content_scores
SET reaction_count = GREATEST(0, reaction_count + @delta), last_scored_at = now()
WHERE content_id = @content_id;

-- name: GetContentScore :one
SELECT content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score, reaction_count, relevance_embedding, last_scored_at
FROM content_scores
WHERE content_id = @content_id;

-- name: ListContentIDsScoredSince :many
-- Returns content IDs whose content_scores.last_scored_at is newer than the
-- given cursor, joined to contents for platform scoping. Used by the §13
-- batched engagement-score re-index cron to find content whose search index
-- document needs refreshing without re-indexing on every engagement event.
-- Limits to @batch_size rows ordered by last_scored_at ascending so the
-- cursor advances monotonically.
SELECT c.id
FROM content_scores cs
JOIN contents c ON c.id = cs.content_id
WHERE cs.last_scored_at > @cursor AND c.deleted_at IS NULL
ORDER BY cs.last_scored_at ASC
LIMIT @batch_size;

-- =====================
-- Content Tags
-- =====================

-- name: AddContentTag :exec
INSERT INTO content_tags (content_id, tag_id) VALUES (@content_id, @tag_id)

ON CONFLICT (content_id, tag_id) DO NOTHING;

-- name: RemoveContentTag :exec
DELETE FROM content_tags WHERE content_id = @content_id AND tag_id = @tag_id;

-- name: ListContentTags :many
SELECT t.id, t.name, t.slug, t.description, t.color, t.follower_count
FROM content_tags ct
JOIN tags t ON t.id = ct.tag_id
WHERE ct.content_id = @content_id
ORDER BY t.name;

-- name: GetContentSpaces :many
-- Returns all space assignments for a content item (Section 14: content_spaces).
-- Used by feed fan-out (Section 32) to distribute a post to every assigned space.
SELECT cs.space_id, cs.is_primary
FROM content_spaces cs
WHERE cs.content_id = @content_id
ORDER BY cs.is_primary DESC, cs.created_at ASC;

-- name: DeleteContentSpaces :exec
-- Removes all space assignments for a content item (Section 14).
-- Used by UpdateContentSpaces to replace the full assignment set.
DELETE FROM content_spaces WHERE content_id = @content_id;

-- name: AddContentSpace :exec
-- Inserts a single space assignment for a content item (Section 14).
-- is_primary must be true for exactly one row per content item.
INSERT INTO content_spaces (content_id, space_id, is_primary) VALUES (@content_id, @space_id, @is_primary)
;

-- name: SetContentSpaceID :exec
-- Updates the primary space columns (space_id and geo_space_id) on a content
-- item to keep contents and content_spaces synchronized. Used by the
-- auto-assign feature and UpdateContentSpaces. Ref: Section 14.
UPDATE contents SET space_id = @space_id, geo_space_id = @space_id, updated_at = now()
WHERE id = @content_id;

-- name: SetContentPrimarySpace :exec
-- Sets is_primary=false on all existing content_spaces for this content,
-- then ensures the requested space is present with is_primary=true. Used
-- when UpdateContent changes the primary space without losing additional
-- assignments. Ref: Section 14.
WITH reassigned AS (
 UPDATE content_spaces
 SET is_primary = false
 WHERE content_id = @content_id
 RETURNING space_id
)
INSERT INTO content_spaces (content_id, space_id, is_primary)
SELECT @content_id, @space_id, true
FROM reassigned
ON CONFLICT (content_id, space_id) DO UPDATE SET is_primary = true;

-- name: ListContentByTag :many
-- Composite cursor (created_at, id) avoids skipping items that share the
-- same created_at timestamp as the last item on a page. Ref: §4.1.
-- Enriched with author/space/excerpt/tags so tag-page feed cards render the
-- same fields as the main feed queries (feeds.sql).
SELECT c.id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
 c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
 c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
 c.created_at, c.updated_at,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 COALESCE(up.display_name, u.public_username, u.username, '') AS author_name,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 MAX(LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400))::text AS excerpt,
 COALESCE(string_agg(DISTINCT t2.name, ',' ORDER BY t2.name), '') AS tags
FROM contents c
JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN content_tags ct2 ON ct2.content_id = c.id
LEFT JOIN tags t2 ON t2.id = ct2.tag_id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id) AND m.deleted_at IS NULL
WHERE ct.tag_id = @tag_id
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (
 (@can_see_unpublished::boolean AND @can_see_private::boolean)
 OR (c.author_id = @requester_id::uuid AND c.author_id IS NOT NULL)
 OR (
 c.status = 'published'
 AND (
 c.visibility = 'public'
 OR (c.visibility = 'members' AND @can_see_members::boolean)
 OR (c.visibility = 'private' AND (c.author_id = @requester_id::uuid OR @can_see_private::boolean))
 )
 )
 )
 AND (
 @cursor_created_at::timestamptz IS NULL
 OR c.created_at < @cursor_created_at::timestamptz
 OR (c.created_at = @cursor_created_at::timestamptz AND c.id < @cursor_id::uuid)
 )
GROUP BY c.id, u.username, u.public_username, up.display_name, up.avatar_url,
 s.name, s.slug, m.storage_bucket, m.storage_path
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- =====================
-- Content Shares
-- =====================

-- name: CreateContentShare :one
INSERT INTO content_shares (content_id, user_id, platform) VALUES (@content_id, @user_id, @platform)

RETURNING id, content_id, user_id, platform, created_at;

-- name: CountContentShares :one
SELECT COUNT(*)::int AS count
FROM content_shares
WHERE content_id = @content_id;

-- =====================
-- Bookmarks (polymorphic)
-- =====================

-- name: CreateBookmark :one
INSERT INTO bookmarks (user_id, entity_type, entity_id, folder, folder_id) VALUES (@user_id, @entity_type, @entity_id, @folder, @folder_id)

ON CONFLICT (user_id, entity_type, entity_id) DO UPDATE SET folder = EXCLUDED.folder, folder_id = EXCLUDED.folder_id
RETURNING id, user_id, entity_type, entity_id, folder, folder_id, created_at;

-- name: SetBookmarkFolder :exec
UPDATE bookmarks SET folder_id = @folder_id
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: DeleteBookmark :exec
DELETE FROM bookmarks
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: DeleteBookmarkByID :execrows
-- Deletes a single bookmark by its row ID, scoped to platform + owner.
-- Used by DELETE /me/bookmarks/{id} — the polymorphic remove endpoint that
-- works regardless of entity_type (content, forum_thread, space, comment, etc.).
DELETE FROM bookmarks
WHERE id = @id AND user_id = @user_id;

-- name: IsBookmarked :one
SELECT EXISTS(
 SELECT 1 FROM bookmarks
 WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id
) AS is_bookmarked;

-- name: ListBookmarkedContentIDs :many
-- Batch bookmark check for card lists (GET /me/content-states): returns which
-- of the given content ids the caller has bookmarked.
SELECT entity_id
FROM bookmarks
WHERE user_id = @user_id AND entity_type = 'content' AND entity_id = ANY(@ids::uuid[]);

-- name: ListBookmarksByUser :many
SELECT b.id, b.user_id, b.entity_type, b.entity_id, b.folder, b.folder_id, b.created_at
FROM bookmarks b
WHERE b.user_id = @user_id
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListBookmarksByFolder :many
SELECT b.id, b.user_id, b.entity_type, b.entity_id, b.folder, b.folder_id, b.created_at
FROM bookmarks b
WHERE b.user_id = @user_id AND b.folder_id = @folder_id
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountBookmarksByUser :one
SELECT COUNT(*)::int AS count
FROM bookmarks
WHERE user_id = @user_id;

-- name: ListPublishedContentByAuthor :many
-- Public profile Posts tab (GET /users/{id}/content): a user's PUBLISHED PUBLIC
-- content, newest first, paginated. author_id is the user id (FK users.id).
-- LEFT JOINs organizations so dual-surface posts carry the "on behalf of" badge.
-- Pinned posts (is_pinned = true) are sorted to the top so the profile
-- surfaces the author's featured/top content first.
SELECT c.id, c.author_id, c.organization_id, c.space_id, c.content_type,
 c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
 c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
 c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.video_source_id,
 c.publish_at, c.published_at, c.created_at, c.updated_at,
 c.journey_stage, c.visa_route, c.source_country, c.target_country, c.guide_category,
 COALESCE(up.display_name, u.public_username, u.username, '') AS author_name,
 COALESCE(u.public_username, u.username, '') AS author_username,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 COALESCE((SELECT string_agg(DISTINCT t.name, ',' ORDER BY t.name)
 FROM content_tags ct
 JOIN tags t ON t.id = ct.tag_id
 WHERE ct.content_id = c.id), '') AS tags,
 (SELECT COUNT(*)::int FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reaction_count,
 COALESCE(cs.bookmark_count, 0) AS bookmark_count,
 COALESCE(cs.velocity_score, 0) AS velocity_score,
 LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400)::text AS excerpt,
 o.name AS org_name, o.slug AS org_slug, o.logo_media_id AS org_logo_media_id,
 om.storage_bucket AS org_logo_storage_bucket,
 om.storage_path AS org_logo_storage_path
FROM contents c
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
WHERE c.author_id = @author_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.is_pinned DESC, c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPublishedContentByOrganization :many
-- Org profile Posts tab (GET /organizations/{id}/content): the org's PUBLISHED
-- PUBLIC content, newest first, paginated. Keyed by organization_id (ADR-062).
-- author_id (the human) is projected for accountability; org badge fields via join.
SELECT c.id, c.author_id, c.organization_id, c.space_id, c.content_type,
 c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
 c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
 c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count, c.video_source_id,
 c.publish_at, c.published_at, c.created_at, c.updated_at,
 c.journey_stage, c.visa_route, c.source_country, c.target_country, c.guide_category,
 COALESCE(up.display_name, u.public_username, u.username, '') AS author_name,
 COALESCE(u.public_username, u.username, '') AS author_username,
 COALESCE(up.avatar_url, '') AS author_avatar_url,
 COALESCE(s.name, '') AS space_name,
 COALESCE(s.slug, '') AS space_slug,
 sm.storage_bucket AS space_avatar_storage_bucket,
 sm.storage_path AS space_avatar_storage_path,
 m.storage_bucket AS cover_storage_bucket,
 m.storage_path AS cover_storage_path,
 vs.source_url AS video_source_url,
 vs.source_type AS video_source_type,
 vs.duration_secs AS video_duration_secs,
 vs.thumbnail_url AS video_thumbnail_url,
 COALESCE((SELECT string_agg(DISTINCT t.name, ',' ORDER BY t.name)
 FROM content_tags ct
 JOIN tags t ON t.id = ct.tag_id
 WHERE ct.content_id = c.id), '') AS tags,
 (SELECT COUNT(*)::int FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reaction_count,
 COALESCE(cs.bookmark_count, 0) AS bookmark_count,
 COALESCE(cs.velocity_score, 0) AS velocity_score,
 LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400)::text AS excerpt,
 o.name AS org_name, o.slug AS org_slug, o.logo_media_id AS org_logo_media_id,
 om.storage_bucket AS org_logo_storage_bucket,
 om.storage_path AS org_logo_storage_path
FROM contents c
LEFT JOIN organizations o ON o.id = c.organization_id
LEFT JOIN media om ON om.id = o.logo_media_id AND om.deleted_at IS NULL
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN media sm ON sm.id = s.logo_media_id AND sm.deleted_at IS NULL
LEFT JOIN video_sources vs ON vs.id = c.video_source_id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN media m ON m.id = COALESCE(c.featured_image_id, c.header_image_id, vs.thumbnail_media_id) AND m.deleted_at IS NULL
WHERE c.organization_id = @organization_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: PublishScheduledContent :execrows
UPDATE contents
SET status = 'published',
 published_at = COALESCE(published_at, now()),
 updated_at = now()
WHERE id = @id
 AND status = 'scheduled'
 AND publish_at <= now();

-- name: ListScheduledContentReady :many
SELECT id
FROM contents
WHERE status = 'scheduled'
 AND publish_at IS NOT NULL
 AND publish_at <= now()
ORDER BY publish_at ASC
LIMIT 100;

-- name: GetContentTitleAndBody :one
SELECT COALESCE(title, '') AS title, COALESCE(body, '') AS body
FROM contents
WHERE id = @id AND deleted_at IS NULL;

-- name: ListPublishedSlugsForSitemap :many
SELECT c.slug, c.updated_at, c.visibility, c.seo_indexable, c.content_type,
 c.comment_count, c.share_count, c.reshare_count,
 array_length(string_to_array(trim(c.body), ' '), 1) AS word_count,
 COALESCE(ft.reply_count, 0) AS reply_count,
 COALESCE(ft.solved, false) AS solved,
 (ft.id IS NOT NULL AND ft.solved = false) AS is_question,
 COALESCE(r.reaction_count, 0) AS reaction_count
FROM contents c
LEFT JOIN forum_threads ft ON ft.content_id = c.id
LEFT JOIN (
 SELECT entity_id, count(*) AS reaction_count
 FROM reactions
 WHERE entity_type IN ('content', 'blog_post', 'forum_thread', 'discussion', 'guide', 'comparison')
 GROUP BY entity_id
) r ON r.entity_id = c.id
WHERE c.status = 'published'
 AND c.deleted_at IS NULL AND c.visibility = 'public' AND c.seo_indexable = true
ORDER BY c.updated_at DESC
LIMIT 10000;

-- name: ListPublishedContentForRSS :many
SELECT title, slug, created_at
FROM contents
WHERE status = 'published'
ORDER BY created_at DESC
LIMIT 50;

-- name: ListPublishedContentForLLMsTxt :many
-- Same visibility/indexability contract as ListPublishedSlugsForSitemap:
-- soft-deleted and seo_indexable=false content must never be advertised
-- to AI crawlers via /llms.txt. The computed columns feed
-- bots.IsContentIndexableByType so the file honors the same seo.sitemap.*
-- quality thresholds as the content sitemap.
SELECT c.title, c.slug, c.content_type, c.visibility, c.seo_indexable,
 c.comment_count, c.share_count, c.reshare_count,
 array_length(string_to_array(trim(c.body), ' '), 1) AS word_count,
 COALESCE(ft.reply_count, 0) AS reply_count,
 COALESCE(ft.solved, false) AS solved,
 (ft.id IS NOT NULL AND ft.solved = false) AS is_question,
 COALESCE(r.reaction_count, 0) AS reaction_count
FROM contents c
LEFT JOIN forum_threads ft ON ft.content_id = c.id
LEFT JOIN (
 SELECT entity_id, count(*) AS reaction_count
 FROM reactions
 WHERE entity_type IN ('content', 'blog_post', 'forum_thread', 'discussion', 'guide', 'comparison')
 GROUP BY entity_id
) r ON r.entity_id = c.id
WHERE c.status = 'published'
 AND c.deleted_at IS NULL AND c.visibility = 'public' AND c.seo_indexable = true
ORDER BY c.created_at DESC
LIMIT 1000;

-- name: ListPublishedContentForDigest :many
SELECT title, slug, created_at
FROM contents
WHERE status = 'published'
 AND created_at >= now() - interval '7 days'
ORDER BY created_at DESC
LIMIT 10;

-- name: GetPublishedContentByID :one
-- Public API (Section 47): returns a single content item only if it is
-- published, public, and not soft-deleted. Drafts, scheduled, private, and
-- member-only content are never exposed to external API consumers.
-- Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
SELECT id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
 field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
 canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
 status, visibility, is_locked, is_pinned, read_time_minutes,
 view_count, comment_count, share_count, reshare_count, publish_at, published_at,
 created_at, updated_at, guide_category
FROM contents
WHERE id = @id
 AND status = 'published'
 AND visibility = 'public'
 AND deleted_at IS NULL;

-- name: ArchiveContent :execrows
UPDATE contents SET status = 'archived', updated_at = now()
WHERE id = @id AND status != 'archived';

-- name: GetPublicAuthorAnalytics :one
-- Public profile Analytics tab (GET /users/{id}/analytics): aggregates
-- published+public content stats for the profile owner. Soft-deleted and
-- non-public/draft content is excluded. total_likes counts reactions on the
-- author's published+public content. Follower/following counts come from the
-- users table / follows subquery so the analytics response is self-contained.
-- Uses subqueries for content aggregates so the user row is always returned
-- even when the user has zero published posts (no GROUP BY needed).
SELECT
 COALESCE((
 SELECT SUM(c.view_count)::int FROM contents c
 WHERE c.author_id = @author_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND c.deleted_at IS NULL
 ), 0)::int AS total_views,
 COALESCE((
 SELECT SUM(c.share_count)::int FROM contents c
 WHERE c.author_id = @author_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND c.deleted_at IS NULL
 ), 0)::int AS total_shares,
 COALESCE((
 SELECT COUNT(c.id)::int FROM contents c
 WHERE c.author_id = @author_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND c.deleted_at IS NULL
 ), 0)::int AS total_posts,
 COALESCE((
 SELECT COUNT(*)::int FROM reactions r
 WHERE r.entity_type = 'content'
 AND r.entity_id IN (
 SELECT c2.id FROM contents c2
 WHERE c2.author_id = @author_id
 AND c2.status = 'published'
 AND c2.visibility = 'public'
 AND c2.deleted_at IS NULL
 )
 ), 0)::int AS total_likes,
 u.follower_count::int AS total_followers,
 (SELECT COUNT(*)::int FROM follows f
 WHERE f.follower_id = u.id) AS total_following
FROM users u
WHERE u.id = @author_id;

-- name: CountPostsByAuthor :one
-- Counts published, non-deleted content by the given author in a platform.
-- Used by the spam pipeline's new-user link restriction (§41 Layer 3) to
-- determine whether a user has passed the "first N posts" threshold.
SELECT COUNT(*)::int AS count
FROM contents c
WHERE c.author_id = @author_id
 AND c.status = 'published'
 AND c.deleted_at IS NULL;

-- name: FindRoadmapContentByDimension :many
-- Returns published content rows that match a scoring dimension and the
-- visitor's journey context. Destination-specific content is returned first,
-- then global fallback content, with stage/visa/origin/purpose matches used as
-- tie-breakers. Used by the assessment scoring worker to resolve a generic
-- curated step to the most specific published guide for the visitor's context.
--
-- The dimension is read from the content's field_values JSONB so any guide
-- category can be tagged with the readiness dimension it addresses.
SELECT c.slug, c.title, LEFT(COALESCE(NULLIF(c.body_html, ''), c.body), 400)::text AS description,
 c.target_country, c.journey_stage, c.visa_route, c.source_country,
 c.field_values->>'purpose' AS purpose
FROM contents c
WHERE c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND c.status = 'published'
 AND c.visibility IN ('public', 'members')
 AND c.field_values->>'dimension' = @dimension
 AND (
 @destination_country::text = ''
 OR c.target_country = @destination_country
 OR c.target_country IS NULL
 OR c.target_country = ''
 )
 AND (
 @journey_stage::text = ''
 OR c.journey_stage = @journey_stage
 OR c.journey_stage IS NULL
 OR c.journey_stage = ''
 )
 AND (
 @visa_route::text = ''
 OR c.visa_route = @visa_route
 OR c.visa_route IS NULL
 OR c.visa_route = ''
 )
 AND (
 @origin_country::text = ''
 OR c.source_country = @origin_country
 OR c.source_country IS NULL
 OR c.source_country = ''
 OR c.target_country = @destination_country
 )
 AND (
 @purpose::text = ''
 OR c.field_values->>'purpose' = @purpose
 OR c.field_values->>'purpose' IS NULL
 OR c.field_values->>'purpose' = ''
 )
ORDER BY
 CASE WHEN c.target_country = @destination_country THEN 0 ELSE 1 END,
 CASE WHEN c.journey_stage = @journey_stage THEN 0 ELSE 1 END,
 CASE WHEN c.visa_route = @visa_route THEN 0 ELSE 1 END,
 CASE WHEN c.source_country = @origin_country THEN 0 ELSE 1 END,
 CASE WHEN c.field_values->>'purpose' = @purpose THEN 0 ELSE 1 END,
 c.view_count DESC,
 c.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: IsContentVisibleToUser :one
-- Phase 3 (spec 2026-09-24-agent-data-api-phase3): per-subscriber content
-- visibility check. Used by the personal-webhook delivery filter and the
-- public bookmark write guard so API-key consumers never learn whether an
-- inaccessible content row exists. Mirrors the detail-page predicate:
-- published + not deleted/hidden; the parent space must be live and either
-- public or hold an approved membership for the viewer; 'members' content
-- is visible to any authenticated viewer, 'private' to the author only.
SELECT EXISTS (
    SELECT 1
    FROM contents c
    LEFT JOIN spaces s ON s.id = c.space_id
    WHERE c.id = @content_id
      AND c.status = 'published'
      AND c.deleted_at IS NULL
      AND c.auto_hidden = false
      AND (
          s.id IS NULL
          OR (s.deleted_at IS NULL AND (
              s.visibility = 'public'
              OR EXISTS (
                  SELECT 1 FROM space_members sm
                  WHERE sm.space_id = s.id
                    AND sm.user_id = @user_id
                    AND sm.status = 'approved'
              )
          ))
      )
      AND (
          c.visibility = 'public'
          OR c.visibility = 'members'
          OR (c.visibility = 'private' AND c.author_id = @user_id)
      )
) AS visible;
