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
SET name = @name, description = sqlc.narg(description), icon = sqlc.narg(icon),
    item_label = @item_label
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
LEFT JOIN user_groups ug ON ug.id = cta.user_group_id
WHERE cta.type_id = @type_id;

-- name: DeleteComparisonTypeAccess :exec
DELETE FROM comparison_type_access
WHERE type_id = @type_id AND user_group_id = sqlc.narg(user_group_id);

-- =====================
-- Comparison Items CRUD
-- =====================

-- name: CreateComparisonItem :one
INSERT INTO comparison_items (
    content_id, tenant_id, title, description, image_id, geo_space_id, position
) VALUES (
    @content_id, @tenant_id, @title, sqlc.narg(description),
    sqlc.narg(image_id), sqlc.narg(geo_space_id), @position
)
RETURNING id, content_id, tenant_id, title, description, image_id, geo_space_id, position, created_at;

-- name: ListComparisonItemsByContent :many
SELECT id, content_id, tenant_id, title, description, image_id, geo_space_id, position, created_at
FROM comparison_items
WHERE content_id = @content_id AND tenant_id = @tenant_id
ORDER BY position ASC, created_at ASC;

-- name: GetComparisonItemByID :one
SELECT id, content_id, tenant_id, title, description, image_id, geo_space_id, position, created_at
FROM comparison_items
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateComparisonItem :one
UPDATE comparison_items
SET title = @title, description = sqlc.narg(description),
    image_id = sqlc.narg(image_id), geo_space_id = sqlc.narg(geo_space_id),
    position = @position
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, content_id, tenant_id, title, description, image_id, geo_space_id, position, created_at;

-- name: DeleteComparisonItem :exec
DELETE FROM comparison_items
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ReorderComparisonItems :exec
UPDATE comparison_items SET position = @position
WHERE id = @id AND content_id = @content_id AND tenant_id = @tenant_id;

-- =====================
-- Comparison Dimensions CRUD
-- =====================

-- name: CreateComparisonDimension :one
INSERT INTO comparison_dimensions (
    content_id, tenant_id, label, dimension_type, position
) VALUES (
    @content_id, @tenant_id, @label, @dimension_type, @position
)
RETURNING id, content_id, tenant_id, label, dimension_type, position, created_at;

-- name: ListComparisonDimensionsByContent :many
SELECT id, content_id, tenant_id, label, dimension_type, position, created_at
FROM comparison_dimensions
WHERE content_id = @content_id AND tenant_id = @tenant_id
ORDER BY position ASC, created_at ASC;

-- name: GetComparisonDimensionByID :one
SELECT id, content_id, tenant_id, label, dimension_type, position, created_at
FROM comparison_dimensions
WHERE id = @id AND tenant_id = @tenant_id;

-- name: UpdateComparisonDimension :one
UPDATE comparison_dimensions
SET label = @label, dimension_type = @dimension_type, position = @position
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, content_id, tenant_id, label, dimension_type, position, created_at;

-- name: DeleteComparisonDimension :exec
DELETE FROM comparison_dimensions
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ReorderComparisonDimensions :exec
UPDATE comparison_dimensions SET position = @position
WHERE id = @id AND content_id = @content_id AND tenant_id = @tenant_id;

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
    value_boolean = EXCLUDED.value_boolean,
    created_at = now()
RETURNING dimension_id, item_id, tenant_id, value, value_rating, value_boolean, created_at;

-- name: ListComparisonValuesByDimension :many
SELECT cv.dimension_id, cv.item_id, cv.tenant_id, cv.value, cv.value_rating,
       cv.value_boolean, cv.created_at,
       ci.title AS item_title
FROM comparison_values cv
JOIN comparison_items ci ON ci.id = cv.item_id
WHERE cv.dimension_id = @dimension_id AND cv.tenant_id = @tenant_id;

-- name: GetComparisonValue :one
SELECT dimension_id, item_id, tenant_id, value, value_rating, value_boolean, created_at
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
       cv.value_boolean, cv.created_at
FROM comparison_values cv
JOIN comparison_items ci ON ci.id = cv.item_id
JOIN comparison_dimensions cd ON cd.id = cv.dimension_id
WHERE ci.content_id = @content_id AND cv.tenant_id = @tenant_id;
