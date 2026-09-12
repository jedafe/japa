-- Custom Field Entity Types (Section 06).
-- DB-backed registry of entity types for custom field group assignment.

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
