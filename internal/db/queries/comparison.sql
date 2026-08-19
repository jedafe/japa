-- Generic Comparison System — type definitions, items, dimensions, values.

-- =====================
-- Comparison Type Definitions CRUD
-- =====================

-- name: CreateComparisonType :one
INSERT INTO comparison_type_definitions (
    tenant_id, name, slug, description, icon, field_group_id, item_label, is_system
) VALUES (
    @tenant_id, @name, @slug, sqlc.narg(description), sqlc.narg(icon),
    @field_group_id, @item_label, @is_system
)
RETURNING id, tenant_id, name, slug, description, icon, field_group_id, item_label, is_system, created_at;

-- name: GetComparisonTypeByID :one
SELECT id, tenant_id, name, slug, description, icon, field_group_id, item_label, is_system, created_at
FROM comparison_type_definitions
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetComparisonTypeBySlug :one
SELECT id, tenant_id, name, slug, description, icon, field_group_id, item_label, is_system, created_at
FROM comparison_type_definitions
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: ListComparisonTypesByTenant :many
SELECT id, tenant_id, name, slug, description, icon, field_group_id, item_label, is_system, created_at
FROM comparison_type_definitions
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateComparisonType :one
UPDATE comparison_type_definitions
SET name = @name, slug = sqlc.narg(slug), description = sqlc.narg(description), icon = sqlc.narg(icon),
    field_group_id = sqlc.narg(field_group_id), item_label = @item_label
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, name, slug, description, icon, field_group_id, item_label, is_system, created_at;

-- name: DeleteComparisonType :exec
DELETE FROM comparison_type_definitions
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Comparison Type Access
-- =====================

-- name: SetComparisonTypeAccess :exec
INSERT INTO comparison_type_access (type_id, user_group_id, can_create, can_fill, ai_can_fill)
VALUES (@type_id, sqlc.narg(user_group_id), @can_create, @can_fill, @ai_can_fill)
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
WHERE cta.type_id = @type_id AND ctd.tenant_id = @tenant_id;

-- name: DeleteComparisonTypeAccess :exec
DELETE FROM comparison_type_access
WHERE type_id = @type_id
  AND user_group_id = sqlc.narg(user_group_id)
  AND type_id IN (
      SELECT id FROM comparison_type_definitions WHERE id = @type_id AND tenant_id = @tenant_id
  );

-- =====================
-- Comparison Items CRUD
-- =====================

-- name: CreateComparisonItem :one
INSERT INTO comparison_items (
    content_id, tenant_id, type_id, title, description, image_id, geo_space_id, position
) VALUES (
    @content_id, @tenant_id, sqlc.narg(type_id), @title, sqlc.narg(description),
    sqlc.narg(image_id), sqlc.narg(geo_space_id), @position
)
RETURNING id, content_id, tenant_id, type_id, title, description, image_id, geo_space_id, position, created_at, deleted_at;

-- name: ListComparisonItemsByContent :many
SELECT id, content_id, tenant_id, type_id, title, description, image_id, geo_space_id, position, created_at, deleted_at
FROM comparison_items
WHERE content_id = @content_id AND tenant_id = @tenant_id AND deleted_at IS NULL
ORDER BY position ASC, created_at ASC;

-- name: GetComparisonItemByID :one
SELECT id, content_id, tenant_id, type_id, title, description, image_id, geo_space_id, position, created_at, deleted_at
FROM comparison_items
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: UpdateComparisonItem :one
UPDATE comparison_items
SET title = @title, description = sqlc.narg(description),
    image_id = sqlc.narg(image_id), geo_space_id = sqlc.narg(geo_space_id),
    position = @position
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, content_id, tenant_id, type_id, title, description, image_id, geo_space_id, position, created_at, deleted_at;

-- name: DeleteComparisonItem :exec
UPDATE comparison_items SET deleted_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ReorderComparisonItems :exec
UPDATE comparison_items SET position = @position
WHERE id = @id AND content_id = @content_id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Comparison Dimensions CRUD
-- =====================

-- name: CreateComparisonDimension :one
INSERT INTO comparison_dimensions (
    content_id, tenant_id, type_id, label, dimension_type, position
) VALUES (
    @content_id, @tenant_id, sqlc.narg(type_id), @label, @dimension_type, @position
)
RETURNING id, content_id, tenant_id, type_id, label, dimension_type, position, created_at, deleted_at;

-- name: ListComparisonDimensionsByContent :many
SELECT id, content_id, tenant_id, type_id, label, dimension_type, position, created_at, deleted_at
FROM comparison_dimensions
WHERE content_id = @content_id AND tenant_id = @tenant_id AND deleted_at IS NULL
ORDER BY position ASC, created_at ASC;

-- name: GetComparisonDimensionByID :one
SELECT id, content_id, tenant_id, type_id, label, dimension_type, position, created_at, deleted_at
FROM comparison_dimensions
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: UpdateComparisonDimension :one
UPDATE comparison_dimensions
SET label = @label, dimension_type = @dimension_type, position = @position
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, content_id, tenant_id, type_id, label, dimension_type, position, created_at, deleted_at;

-- name: DeleteComparisonDimension :exec
UPDATE comparison_dimensions SET deleted_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ReorderComparisonDimensions :exec
UPDATE comparison_dimensions SET position = @position
WHERE id = @id AND content_id = @content_id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Comparison Values CRUD
-- =====================

-- name: UpsertComparisonValue :one
INSERT INTO comparison_values (
    dimension_id, item_id, tenant_id, value, value_rating, value_boolean
) VALUES (
    @dimension_id, @item_id, @tenant_id, sqlc.narg(value),
    sqlc.narg(value_rating), sqlc.narg(value_boolean)
)
ON CONFLICT (dimension_id, item_id) DO UPDATE
SET value = EXCLUDED.value,
    value_rating = EXCLUDED.value_rating,
    value_boolean = EXCLUDED.value_boolean
RETURNING dimension_id, item_id, tenant_id, value, value_rating, value_boolean, created_at, metadata;

-- name: UpsertComparisonValueWithMetadata :one
-- Upserts a comparison value together with its provenance metadata JSONB
-- (Section 103 — Comparison Engine Provenance). Used by the AI Comparison
-- Engine to record the source (custom_field|rag|external|llm_knowledge),
-- source_ref, confidence, and retrieved_at timestamp for each value it
-- fills. The metadata column was added by migration 000373.
INSERT INTO comparison_values (
    dimension_id, item_id, tenant_id, value, value_rating, value_boolean, metadata
) VALUES (
    @dimension_id, @item_id, @tenant_id, sqlc.narg(value),
    sqlc.narg(value_rating), sqlc.narg(value_boolean), sqlc.narg(metadata)
)
ON CONFLICT (dimension_id, item_id) DO UPDATE
SET value = EXCLUDED.value,
    value_rating = EXCLUDED.value_rating,
    value_boolean = EXCLUDED.value_boolean,
    metadata = COALESCE(EXCLUDED.metadata, comparison_values.metadata)
RETURNING dimension_id, item_id, tenant_id, value, value_rating, value_boolean, created_at, metadata;

-- name: ListComparisonValuesByDimension :many
SELECT cv.dimension_id, cv.item_id, cv.tenant_id, cv.value, cv.value_rating,
       cv.value_boolean, cv.created_at, cv.metadata,
       ci.title AS item_title
FROM comparison_values cv
JOIN comparison_items ci ON ci.id = cv.item_id
WHERE cv.dimension_id = @dimension_id AND cv.tenant_id = @tenant_id
  AND ci.deleted_at IS NULL;

-- name: GetComparisonValue :one
SELECT dimension_id, item_id, tenant_id, value, value_rating, value_boolean, created_at, metadata
FROM comparison_values
WHERE dimension_id = @dimension_id AND item_id = @item_id AND tenant_id = @tenant_id;

-- name: DeleteComparisonValuesByContent :exec
DELETE FROM comparison_values cv
WHERE cv.dimension_id IN (
    SELECT cd.id FROM comparison_dimensions cd
    WHERE cd.content_id = @content_id AND cd.tenant_id = @tenant_id
) AND cv.tenant_id = @tenant_id;

-- name: DeleteComparisonValuesByDimension :exec
DELETE FROM comparison_values
WHERE dimension_id = @dimension_id AND tenant_id = @tenant_id;

-- name: GetComparisonValuesByContent :many
SELECT cv.dimension_id, cv.item_id, cv.tenant_id, cv.value, cv.value_rating,
       cv.value_boolean, cv.created_at, cv.metadata
FROM comparison_values cv
JOIN comparison_items ci ON ci.id = cv.item_id
JOIN comparison_dimensions cd ON cd.id = cv.dimension_id
WHERE ci.content_id = @content_id AND cv.tenant_id = @tenant_id
  AND ci.deleted_at IS NULL AND cd.deleted_at IS NULL;

-- =====================
-- Comparison list with type + space filters (Section 30, BUG-007)
-- =====================

-- name: ListComparisonsFiltered :many
-- Offset-paginated per conventions/api-response-format.md (?page_size=N&page_offset=N).
-- Public list: only published, publicly-visible comparisons are returned so the
-- /comparisons endpoint can be served to unauthenticated visitors (Section 30).
SELECT DISTINCT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
       c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at
FROM contents c
LEFT JOIN comparison_items ci ON ci.content_id = c.id AND ci.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.content_type = 'comparison'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
  AND (sqlc.narg(type_id)::uuid IS NULL OR ci.type_id = sqlc.narg(type_id))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Comparison admin list (Section 30)
-- =====================

-- name: ListComparisonsFilteredAdmin :many
-- Admin list: returns all non-deleted comparison content rows for the tenant,
-- optionally filtered by type_id and space_id.
SELECT DISTINCT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
       c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at
FROM contents c
LEFT JOIN comparison_items ci ON ci.content_id = c.id AND ci.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.content_type = 'comparison'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
  AND (sqlc.narg(type_id)::uuid IS NULL OR ci.type_id = sqlc.narg(type_id))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountComparisonItemsByType :one
SELECT count(*)::int FROM comparison_items WHERE type_id = @type_id AND tenant_id = @tenant_id AND deleted_at IS NULL;

