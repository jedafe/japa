-- Custom Field Entity Types (Section 06).
-- DB-backed registry of entity types for custom field group assignment.

-- name: ListCustomFieldEntityTypes :many
SELECT id, tenant_id, value, label, is_system, sort_order, created_at, updated_at
FROM custom_field_entity_types
WHERE tenant_id = @tenant_id
ORDER BY sort_order ASC, label ASC;

-- name: ListCustomFieldEntityTypesByPrefix :many
SELECT id, tenant_id, value, label, is_system, sort_order, created_at, updated_at
FROM custom_field_entity_types
WHERE tenant_id = @tenant_id AND value LIKE @prefix
ORDER BY sort_order ASC, label ASC;

-- name: GetCustomFieldEntityTypeByValue :one
SELECT id, tenant_id, value, label, is_system, sort_order, created_at, updated_at
FROM custom_field_entity_types
WHERE tenant_id = @tenant_id AND value = @value;

-- name: CreateCustomFieldEntityType :one
INSERT INTO custom_field_entity_types (tenant_id, value, label, is_system, sort_order)
VALUES (@tenant_id, @value, @label, false, @sort_order)
RETURNING id, tenant_id, value, label, is_system, sort_order, created_at, updated_at;

-- name: UpdateCustomFieldEntityType :one
UPDATE custom_field_entity_types
SET label = @label,
    sort_order = @sort_order,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, value, label, is_system, sort_order, created_at, updated_at;

-- name: DeleteCustomFieldEntityType :execrows
-- System entity types cannot be deleted (is_system = true).
DELETE FROM custom_field_entity_types
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false;
