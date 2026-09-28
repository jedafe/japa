-- name: CreateComparisonType :one
INSERT INTO comparison_type_definitions (name, slug, description, icon, field_group_id, item_label, is_system, config) VALUES (@name, @slug, sqlc.narg(description), sqlc.narg(icon), @field_group_id, @item_label, @is_system, sqlc.narg(config))
RETURNING id, name, slug, description, icon, field_group_id, item_label, is_system, config, created_at;

-- name: GetComparisonTypeByID :one
SELECT id, name, slug, description, icon, field_group_id, item_label, is_system, config, created_at
FROM comparison_type_definitions
WHERE id = @id;

-- name: GetComparisonTypeBySlug :one
SELECT id, name, slug, description, icon, field_group_id, item_label, is_system, config, created_at
FROM comparison_type_definitions
WHERE slug = @slug;

-- name: ListComparisonTypes :many
SELECT id, name, slug, description, icon, field_group_id, item_label, is_system, config, created_at
FROM comparison_type_definitions

ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateComparisonType :one
UPDATE comparison_type_definitions
SET name = @name, slug = sqlc.narg(slug), description = sqlc.narg(description), icon = sqlc.narg(icon),
 field_group_id = sqlc.narg(field_group_id), item_label = @item_label, config = sqlc.narg(config)
WHERE id = @id
RETURNING id, name, slug, description, icon, field_group_id, item_label, is_system, config, created_at;

-- name: DeleteComparisonType :exec
DELETE FROM comparison_type_definitions
WHERE id = @id;

-- =====================
-- Comparison Type Access
-- =====================

-- name: SetComparisonTypeAccess :exec
INSERT INTO comparison_type_access (type_id, user_group_id, can_create, can_fill, ai_can_fill) VALUES (@type_id, sqlc.narg(user_group_id), @can_create, @can_fill, @ai_can_fill)

ON CONFLICT (type_id, group_key) DO UPDATE
SET can_create = EXCLUDED.can_create,
 can_fill = EXCLUDED.can_fill,
 ai_can_fill = EXCLUDED.ai_can_fill;

-- name: ListComparisonTypeAccess :many
SELECT cta.type_id, cta.user_group_id, cta.can_create, cta.can_fill, cta.ai_can_fill,
 ug.name AS group_name
FROM comparison_type_access cta
JOIN comparison_type_definitions ctd ON ctd.id = cta.type_id
LEFT JOIN user_groups ug ON ug.id = cta.user_group_id
WHERE cta.type_id = @type_id;

-- name: DeleteComparisonTypeAccess :exec
DELETE FROM comparison_type_access
WHERE type_id = @type_id
 AND user_group_id = sqlc.narg(user_group_id)
 AND type_id IN (
 SELECT id FROM comparison_type_definitions WHERE id = @type_id
 );

-- =====================
-- Comparison Items CRUD
-- =====================

-- name: CreateComparisonItem :one
INSERT INTO comparison_items (content_id, type_id, title, description, image_id, geo_space_id, position, item_ref) VALUES (@content_id, sqlc.narg(type_id), @title, sqlc.narg(description), sqlc.narg(image_id), sqlc.narg(geo_space_id), @position, sqlc.narg(item_ref))
RETURNING id, content_id, type_id, title, description, image_id, geo_space_id, position, item_ref, created_at, deleted_at;

-- name: ListComparisonItemsByContent :many
SELECT id, content_id, type_id, title, description, image_id, geo_space_id, position, item_ref, created_at, deleted_at
FROM comparison_items
WHERE content_id = @content_id AND deleted_at IS NULL
ORDER BY position ASC, created_at ASC;

-- name: GetComparisonItemByID :one
SELECT id, content_id, type_id, title, description, image_id, geo_space_id, position, item_ref, created_at, deleted_at
FROM comparison_items
WHERE id = @id AND deleted_at IS NULL;

-- name: UpdateComparisonItem :one
UPDATE comparison_items
SET title = @title, description = sqlc.narg(description),
 image_id = sqlc.narg(image_id), geo_space_id = sqlc.narg(geo_space_id),
 position = @position, item_ref = sqlc.narg(item_ref)
WHERE id = @id AND deleted_at IS NULL
RETURNING id, content_id, type_id, title, description, image_id, geo_space_id, position, item_ref, created_at, deleted_at;

-- name: DeleteComparisonItem :exec
UPDATE comparison_items SET deleted_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: ReorderComparisonItems :exec
UPDATE comparison_items SET position = @position
WHERE id = @id AND content_id = @content_id AND deleted_at IS NULL;

-- =====================
-- Comparison Dimensions CRUD
-- =====================

-- name: CreateComparisonDimension :one
INSERT INTO comparison_dimensions (content_id, type_id, label, dimension_type, position, source_field_id) VALUES (@content_id, sqlc.narg(type_id), @label, @dimension_type, @position, sqlc.narg(source_field_id))
RETURNING id, content_id, type_id, label, dimension_type, position, source_field_id, created_at, deleted_at;

-- name: ListComparisonDimensionsByContent :many
-- LEFT JOINs custom_fields so the matrix assembler gets the source field key
-- without a second lookup (§30 Sub-spec 5 entity-field hydration).
SELECT d.id, d.content_id, d.type_id, d.label, d.dimension_type, d.position,
 d.source_field_id, cf.key AS source_field_key, d.created_at, d.deleted_at
FROM comparison_dimensions d
LEFT JOIN custom_fields cf ON cf.id = d.source_field_id
WHERE d.content_id = @content_id AND d.deleted_at IS NULL
ORDER BY d.position ASC, d.created_at ASC;

-- name: GetComparisonDimensionByID :one
SELECT id, content_id, type_id, label, dimension_type, position, source_field_id, created_at, deleted_at
FROM comparison_dimensions
WHERE id = @id AND deleted_at IS NULL;

-- name: UpdateComparisonDimension :one
UPDATE comparison_dimensions
SET label = @label, dimension_type = @dimension_type, position = @position,
 source_field_id = sqlc.narg(source_field_id)
WHERE id = @id AND deleted_at IS NULL
RETURNING id, content_id, type_id, label, dimension_type, position, source_field_id, created_at, deleted_at;

-- name: DeleteComparisonDimension :exec
UPDATE comparison_dimensions SET deleted_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: ReorderComparisonDimensions :exec
UPDATE comparison_dimensions SET position = @position
WHERE id = @id AND content_id = @content_id AND deleted_at IS NULL;

-- =====================
-- Comparison Values CRUD
-- =====================

-- name: UpsertComparisonValue :one
INSERT INTO comparison_values (dimension_id, item_id, value, value_rating, value_boolean, data_source_id, updated_at) VALUES (@dimension_id, @item_id, sqlc.narg(value), sqlc.narg(value_rating), sqlc.narg(value_boolean), sqlc.narg(data_source_id), now())
ON CONFLICT (dimension_id, item_id) DO UPDATE
SET value = EXCLUDED.value,
 value_rating = EXCLUDED.value_rating,
 value_boolean = EXCLUDED.value_boolean,
 data_source_id = EXCLUDED.data_source_id,
 updated_at = now()
RETURNING dimension_id, item_id, value, value_rating, value_boolean, created_at, metadata, updated_at, data_source_id;

-- name: UpsertComparisonValueWithMetadata :one
-- Upserts a comparison value together with its provenance metadata JSONB
-- (Section 103 — Comparison Engine Provenance). Used by the AI Comparison
-- Engine to record the source (custom_field|rag|external|llm_knowledge),
-- source_ref, confidence, and retrieved_at timestamp for each value it
-- fills. The metadata column was added by migration 000373.
INSERT INTO comparison_values (dimension_id, item_id, value, value_rating, value_boolean, metadata, data_source_id, updated_at) VALUES (@dimension_id, @item_id, sqlc.narg(value), sqlc.narg(value_rating), sqlc.narg(value_boolean), sqlc.narg(metadata), sqlc.narg(data_source_id), now())
ON CONFLICT (dimension_id, item_id) DO UPDATE
SET value = EXCLUDED.value,
 value_rating = EXCLUDED.value_rating,
 value_boolean = EXCLUDED.value_boolean,
 metadata = COALESCE(EXCLUDED.metadata, comparison_values.metadata),
 data_source_id = EXCLUDED.data_source_id,
 updated_at = now()
RETURNING dimension_id, item_id, value, value_rating, value_boolean, created_at, metadata, updated_at, data_source_id;

-- name: ListComparisonValuesByDimension :many
SELECT cv.dimension_id, cv.item_id, cv.value, cv.value_rating,
 cv.value_boolean, cv.created_at, cv.metadata, cv.updated_at, cv.data_source_id,
 ci.title AS item_title
FROM comparison_values cv
JOIN comparison_items ci ON ci.id = cv.item_id
WHERE cv.dimension_id = @dimension_id
 AND ci.deleted_at IS NULL;

-- name: GetComparisonValue :one
SELECT dimension_id, item_id, value, value_rating, value_boolean, created_at, metadata, updated_at, data_source_id
FROM comparison_values
WHERE dimension_id = @dimension_id AND item_id = @item_id;

-- name: DeleteComparisonValuesByContent :exec
DELETE FROM comparison_values cv
WHERE cv.dimension_id IN (
 SELECT cd.id FROM comparison_dimensions cd
 WHERE cd.content_id = @content_id
);

-- name: DeleteComparisonValuesByDimension :exec
DELETE FROM comparison_values
WHERE dimension_id = @dimension_id;

-- name: GetComparisonValuesByContent :many
SELECT cv.dimension_id, cv.item_id, cv.value, cv.value_rating,
 cv.value_boolean, cv.created_at, cv.metadata, cv.updated_at, cv.data_source_id
FROM comparison_values cv
JOIN comparison_items ci ON ci.id = cv.item_id
JOIN comparison_dimensions cd ON cd.id = cv.dimension_id
WHERE ci.content_id = @content_id
 AND ci.deleted_at IS NULL AND cd.deleted_at IS NULL;

-- =====================
-- Comparison Content Metadata (JCE)
-- =====================

-- name: InsertComparisonContentMetadata :exec
INSERT INTO comparison_content_metadata (content_id, depth, promoted_from_id, promoted_at, smart_mode, score_version, config) VALUES (@content_id, @depth, sqlc.narg(promoted_from_id), sqlc.narg(promoted_at), sqlc.narg(smart_mode), @score_version, sqlc.narg(config)) ;

-- name: GetComparisonContentMetadata :one
SELECT content_id, depth, promoted_from_id, promoted_at, smart_mode, score_version, config, created_at, updated_at
FROM comparison_content_metadata
WHERE content_id = @content_id;

-- name: UpdateComparisonContentMetadata :exec
UPDATE comparison_content_metadata
SET depth = sqlc.narg(depth), promoted_from_id = sqlc.narg(promoted_from_id),
 promoted_at = sqlc.narg(promoted_at), smart_mode = sqlc.narg(smart_mode),
 score_version = sqlc.narg(score_version), config = sqlc.narg(config), updated_at = now()
WHERE content_id = @content_id;

-- name: DeleteComparisonContentMetadata :exec
DELETE FROM comparison_content_metadata
WHERE content_id = @content_id;

-- =====================
-- Comparison list with type + space filters (Section 30, BUG-007)
-- =====================

-- name: ListComparisonsFiltered :many
-- Offset-paginated per conventions/api-response-format.md (?page_size=N&page_offset=N).
-- Public list: only published, publicly-visible comparisons are returned so the
-- /comparisons endpoint can be served to unauthenticated visitors (Section 30).
SELECT DISTINCT c.id, c.author_id, c.space_id, c.content_type,
 c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
 c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
 c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count,
 c.publish_at, c.published_at, c.created_at, c.updated_at
FROM contents c
LEFT JOIN comparison_items ci ON ci.content_id = c.id
WHERE c.content_type = 'comparison'
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
 AND (sqlc.narg(type_id)::uuid IS NULL OR ci.type_id = sqlc.narg(type_id))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListComparisonsFilteredAdmin :many
-- Admin list: returns all non-deleted comparison content rows for the platform,
-- optionally filtered by type_id and space_id.
SELECT DISTINCT c.id, c.author_id, c.space_id, c.content_type,
 c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
 c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
 c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count,
 c.publish_at, c.published_at, c.created_at, c.updated_at
FROM contents c
LEFT JOIN comparison_items ci ON ci.content_id = c.id
WHERE c.content_type = 'comparison'
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
 AND (sqlc.narg(type_id)::uuid IS NULL OR ci.type_id = sqlc.narg(type_id))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Comparison list with filters (JCE /api/v1/jce/comparisons)
-- =====================

-- name: ListComparisonsWithFilters :many
-- Supports public directory, mine=true, depth filters, status, visibility, q, sort.
SELECT c.id, c.author_id, c.space_id, c.content_type,
 c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
 c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
 c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count,
 c.publish_at, c.published_at, c.created_at, c.updated_at, c.deleted_at,
 m.depth, m.promoted_from_id, m.promoted_at, m.smart_mode, m.score_version, m.config AS metadata_config
FROM contents c
JOIN comparison_content_metadata m ON m.content_id = c.id
WHERE c.content_type = 'comparison'
 AND (
 CASE sqlc.narg(deleted)::text
 WHEN 'only' THEN c.deleted_at IS NOT NULL
 WHEN 'all' THEN TRUE
 ELSE (c.deleted_at IS NULL AND c.auto_hidden = false)
 END
 )
 AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
 AND (
 sqlc.narg(type_id)::uuid IS NULL
 OR EXISTS (
 SELECT 1 FROM comparison_items ci
 WHERE ci.content_id = c.id
 AND ci.type_id = sqlc.narg(type_id)
 AND ci.deleted_at IS NULL
 )
 )
 AND (sqlc.narg(depth)::text IS NULL OR m.depth = sqlc.narg(depth))
 AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status))
 AND (sqlc.narg(visibility)::text IS NULL OR c.visibility = sqlc.narg(visibility))
 AND (sqlc.narg(mine)::boolean IS NOT TRUE OR c.author_id = @user_id)
 AND (
 sqlc.narg(q)::text IS NULL OR sqlc.narg(q) = ''
 OR c.title ILIKE '%' || sqlc.narg(q) || '%'
 OR c.slug ILIKE '%' || sqlc.narg(q) || '%'
 )
ORDER BY
 CASE WHEN sqlc.narg(sort)::text = 'title' THEN c.title END ASC,
 CASE WHEN sqlc.narg(sort)::text = 'popularity' THEN c.view_count END DESC,
 c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountComparisonsWithFilters :one
-- Count of comparisons matching the same filters as ListComparisonsWithFilters.
SELECT count(*)::int
FROM contents c
JOIN comparison_content_metadata m ON m.content_id = c.id
WHERE c.content_type = 'comparison'
 AND (
 CASE sqlc.narg(deleted)::text
 WHEN 'only' THEN c.deleted_at IS NOT NULL
 WHEN 'all' THEN TRUE
 ELSE (c.deleted_at IS NULL AND c.auto_hidden = false)
 END
 )
 AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
 AND (
 sqlc.narg(type_id)::uuid IS NULL
 OR EXISTS (
 SELECT 1 FROM comparison_items ci
 WHERE ci.content_id = c.id
 AND ci.type_id = sqlc.narg(type_id)
 AND ci.deleted_at IS NULL
 )
 )
 AND (sqlc.narg(depth)::text IS NULL OR m.depth = sqlc.narg(depth))
 AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status))
 AND (sqlc.narg(visibility)::text IS NULL OR c.visibility = sqlc.narg(visibility))
 AND (sqlc.narg(mine)::boolean IS NOT TRUE OR c.author_id = @user_id)
 AND (
 sqlc.narg(q)::text IS NULL OR sqlc.narg(q) = ''
 OR c.title ILIKE '%' || sqlc.narg(q) || '%'
 OR c.slug ILIKE '%' || sqlc.narg(q) || '%'
 );

-- =====================
-- Comparison detail / matrix helpers
-- =====================

-- name: GetComparisonBySlugOrID :one
SELECT c.id, c.author_id, c.space_id, c.content_type,
 c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
 c.geo_space_id, c.source_country, c.target_country,
 c.header_image_id, c.featured_image_id, c.canonical_url,
 c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
 c.ai_exempt,
 c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
 c.view_count, c.comment_count, c.share_count, c.reshare_count,
 c.publish_at, c.published_at, c.created_at, c.updated_at,
 u.username AS author_name,
 u.username AS author_username,
 up.display_name AS author_display_name,
 up.avatar_url AS author_avatar_url,
 s.name AS space_name,
 s.slug AS space_slug,
 m.depth, m.promoted_from_id, m.promoted_at, m.smart_mode, m.score_version, m.config AS metadata_config,
 fm.storage_bucket AS featured_storage_bucket, fm.storage_path AS featured_storage_path,
 hm.storage_bucket AS header_storage_bucket, hm.storage_path AS header_storage_path
FROM contents c
JOIN comparison_content_metadata m ON m.content_id = c.id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN media fm ON fm.id = c.featured_image_id AND fm.deleted_at IS NULL
LEFT JOIN media hm ON hm.id = c.header_image_id AND hm.deleted_at IS NULL
WHERE c.content_type = 'comparison'
 AND c.deleted_at IS NULL
 AND (c.id = sqlc.narg(id) OR c.slug = sqlc.narg(slug));

-- name: CountComparisonItemsByType :one
SELECT count(*)::int FROM comparison_items WHERE type_id = @type_id AND deleted_at IS NULL;

-- =====================
-- Nested comparison item search (JCE nested item resolver)
-- =====================

-- name: SearchComparisonContents :many
-- Returns published detailed comparisons that may be used as items in another
-- comparison. Optionally filters by a search query (title/slug) and an array
-- of allowed nested comparison type slugs.
SELECT
 c.id, c.title, c.slug, c.body, c.status, c.visibility,
 c.geo_space_id, c.source_country, c.target_country,
 (SELECT ci.type_id FROM comparison_items ci
 WHERE ci.content_id = c.id AND ci.deleted_at IS NULL
 ORDER BY ci.position LIMIT 1) AS type_id,
 COALESCE((SELECT ctd.slug
 FROM comparison_items ci
 JOIN comparison_type_definitions ctd ON ctd.id = ci.type_id
 WHERE ci.content_id = c.id AND ci.deleted_at IS NULL
 ORDER BY ci.position LIMIT 1), '')::text AS type_slug,
 COALESCE((SELECT ctd.name
 FROM comparison_items ci
 JOIN comparison_type_definitions ctd ON ctd.id = ci.type_id
 WHERE ci.content_id = c.id AND ci.deleted_at IS NULL
 ORDER BY ci.position LIMIT 1), '')::text AS type_name,
 m.promoted_from_id
FROM contents c
JOIN comparison_content_metadata m ON m.content_id = c.id
WHERE c.content_type = 'comparison'
 AND c.deleted_at IS NULL
 AND c.auto_hidden = false
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND m.depth = 'detailed'
 AND (@q = '' OR c.title ILIKE '%' || @q || '%' OR c.slug ILIKE '%' || @q || '%')
 AND (
 sqlc.narg(type_slugs)::text[] IS NULL
 OR array_length(sqlc.narg(type_slugs)::text[], 1) IS NULL
 OR EXISTS (
 SELECT 1
 FROM comparison_items ci
 JOIN comparison_type_definitions ctd ON ctd.id = ci.type_id
 WHERE ci.content_id = c.id
 AND ci.deleted_at IS NULL
 AND ctd.slug = ANY(sqlc.narg(type_slugs)::text[])
 )
 )
ORDER BY c.title ASC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- item_ref propagation lookup (§30 Sub-spec 5)
-- =====================

-- name: ListComparisonContentIDsByItemRef :many
-- Finds comparison contents containing an item whose item_ref matches
-- {type, id} exactly (§30 Sub-spec 5 propagation).
SELECT DISTINCT content_id
FROM comparison_items
WHERE deleted_at IS NULL
  AND item_ref @> jsonb_build_object('type', @entity_type::text, 'id', @entity_id::text);
