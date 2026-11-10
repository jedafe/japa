-- name: ListCustomFieldEntityTypes :many
SELECT id, value, label, is_system, sort_order, created_at, updated_at
FROM custom_field_entity_types

ORDER BY sort_order ASC, label ASC;

-- name: ListCustomFieldEntityTypesByPrefix :many
SELECT id, value, label, is_system, sort_order, created_at, updated_at
FROM custom_field_entity_types
WHERE value LIKE @prefix
ORDER BY sort_order ASC, label ASC;

-- name: GetCustomFieldEntityTypeByValue :one
SELECT id, value, label, is_system, sort_order, created_at, updated_at
FROM custom_field_entity_types
WHERE value = @value;

-- name: CreateCustomFieldEntityType :one
INSERT INTO custom_field_entity_types (value, label, is_system, sort_order) VALUES (@value, @label, false, @sort_order)

RETURNING id, value, label, is_system, sort_order, created_at, updated_at;

-- name: UpdateCustomFieldEntityType :one
UPDATE custom_field_entity_types
SET label = @label,
 sort_order = @sort_order,
 updated_at = now()
WHERE id = @id
RETURNING id, value, label, is_system, sort_order, created_at, updated_at;

-- name: DeleteCustomFieldEntityType :execrows
-- System entity types cannot be deleted (is_system = true).
DELETE FROM custom_field_entity_types
WHERE id = @id AND is_system = false;

-- name: ListGuideCategoriesWithGroups :many
-- Guide categories with their field groups resolved in a single query —
-- group slug follows the 'guide_{category}_fields' convention (§14).
SELECT t.value, t.label, t.is_system, t.sort_order,
       g.id AS group_id, g.slug AS group_slug
FROM custom_field_entity_types t
LEFT JOIN custom_field_groups g
  ON g.slug = 'guide_' || substr(t.value, length('guide_category:') + 1) || '_fields'
WHERE t.value LIKE 'guide_category:%'
ORDER BY t.sort_order ASC, t.label ASC;
