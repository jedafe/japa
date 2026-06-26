-- Custom field group, field, assignment, permission, and value queries.

-- =====================
-- Custom Field Groups
-- =====================

-- name: ListCustomFieldGroupsByTenant :many
SELECT id, tenant_id, name, slug, description, is_system, created_at
FROM custom_field_groups
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC, id DESC;

-- name: GetCustomFieldGroupByID :one
SELECT id, tenant_id, name, slug, description, is_system, created_at
FROM custom_field_groups
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetCustomFieldGroupBySlug :one
SELECT id, tenant_id, name, slug, description, is_system, created_at
FROM custom_field_groups
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: CreateCustomFieldGroup :one
INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
VALUES (@tenant_id, @name, @slug, @description, @is_system)
RETURNING id, tenant_id, name, slug, description, is_system, created_at;

-- name: UpdateCustomFieldGroup :one
UPDATE custom_field_groups
SET name = @name, description = @description
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false
RETURNING id, tenant_id, name, slug, description, is_system, created_at;

-- name: DeleteCustomFieldGroup :exec
DELETE FROM custom_field_groups
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false;

-- =====================
-- Custom Fields
-- =====================

-- name: ListCustomFieldsByGroup :many
SELECT id, group_id, tenant_id, label, key, field_type, options, embed_platforms,
       placeholder, default_value, is_required, is_system, show_in_frontend,
       filterable, sortable, searchable, position, created_at
FROM custom_fields
WHERE group_id = @group_id AND tenant_id = @tenant_id
ORDER BY position ASC, created_at ASC;

-- name: GetCustomFieldByID :one
SELECT id, group_id, tenant_id, label, key, field_type, options, embed_platforms,
       placeholder, default_value, is_required, is_system, show_in_frontend,
       filterable, sortable, searchable, position, created_at
FROM custom_fields
WHERE id = @id AND tenant_id = @tenant_id;

-- name: CreateCustomField :one
INSERT INTO custom_fields (
    group_id, tenant_id, label, key, field_type, options, embed_platforms,
    placeholder, default_value, is_required, is_system, show_in_frontend,
    filterable, sortable, searchable, position
) VALUES (
    @group_id, @tenant_id, @label, @key, @field_type, @options, @embed_platforms,
    @placeholder, @default_value, @is_required, @is_system, @show_in_frontend,
    @filterable, @sortable, @searchable, @position
)
RETURNING id, group_id, tenant_id, label, key, field_type, options, embed_platforms,
          placeholder, default_value, is_required, is_system, show_in_frontend,
          filterable, sortable, searchable, position, created_at;

-- name: UpdateCustomField :one
UPDATE custom_fields
SET label = @label, field_type = @field_type, options = @options,
    embed_platforms = @embed_platforms, placeholder = @placeholder,
    default_value = @default_value, is_required = @is_required,
    show_in_frontend = @show_in_frontend, filterable = @filterable,
    sortable = @sortable, searchable = @searchable, position = @position
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false
RETURNING id, group_id, tenant_id, label, key, field_type, options, embed_platforms,
          placeholder, default_value, is_required, is_system, show_in_frontend,
          filterable, sortable, searchable, position, created_at;

-- name: DeleteCustomField :exec
DELETE FROM custom_fields
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false;

-- =====================
-- Group Assignments
-- =====================

-- name: ListAssignmentsByEntityClass :many
SELECT id, group_id, entity_class, entity_id, position, created_at
FROM custom_field_group_assignments
WHERE entity_class = @entity_class
ORDER BY position ASC;

-- name: CreateAssignment :one
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES (@group_id, @entity_class, sqlc.narg(entity_id), @position)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING
RETURNING id, group_id, entity_class, entity_id, position, created_at;

-- name: DeleteAssignment :exec
DELETE FROM custom_field_group_assignments
WHERE group_id = @group_id AND entity_class = @entity_class
  AND entity_id IS NOT DISTINCT FROM sqlc.narg(entity_id);

-- =====================
-- Field Permissions
-- =====================

-- name: ListPermissionsByField :many
SELECT field_id, user_group_id, can_view, can_edit
FROM custom_field_permissions
WHERE field_id = @field_id;

-- name: UpsertFieldPermission :exec
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
VALUES (@field_id, @user_group_id, @can_view, @can_edit)
ON CONFLICT (field_id, user_group_id) DO UPDATE
SET can_view = EXCLUDED.can_view, can_edit = EXCLUDED.can_edit;

-- name: DeleteFieldPermission :exec
DELETE FROM custom_field_permissions
WHERE field_id = @field_id AND user_group_id = @user_group_id;

-- =====================
-- Field Values
-- =====================

-- name: GetCustomFieldValue :one
SELECT id, field_id, entity_type, entity_id, value_text, value_number,
       value_bool, value_date, value_json, created_at, updated_at
FROM custom_field_values
WHERE field_id = @field_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: ListCustomFieldValuesByEntity :many
SELECT id, field_id, entity_type, entity_id, value_text, value_number,
       value_bool, value_date, value_json, created_at, updated_at
FROM custom_field_values
WHERE entity_type = @entity_type AND entity_id = @entity_id;

-- name: UpsertCustomFieldValue :one
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_text, value_number, value_bool, value_date, value_json)
VALUES (@field_id, @entity_type, @entity_id, sqlc.narg(value_text), sqlc.narg(value_number), sqlc.narg(value_bool), sqlc.narg(value_date), sqlc.narg(value_json))
ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_text = EXCLUDED.value_text,
    value_number = EXCLUDED.value_number,
    value_bool = EXCLUDED.value_bool,
    value_date = EXCLUDED.value_date,
    value_json = EXCLUDED.value_json,
    updated_at = now()
RETURNING id, field_id, entity_type, entity_id, value_text, value_number,
          value_bool, value_date, value_json, created_at, updated_at;

-- name: DeleteCustomFieldValue :exec
DELETE FROM custom_field_values
WHERE field_id = @field_id AND entity_type = @entity_type AND entity_id = @entity_id;
