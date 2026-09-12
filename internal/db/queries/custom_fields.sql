-- name: ListCustomFieldGroupsByTenant :many
SELECT id, tenant_id, name, slug, description, is_system, created_at
FROM custom_field_groups

ORDER BY created_at DESC, id DESC;

-- name: ListCustomFieldGroupsByTenantAndEntityClass :many
SELECT DISTINCT cfg.id, cfg.name, cfg.slug, cfg.description,
 cfg.is_system, cfg.created_at
FROM custom_field_groups cfg
JOIN custom_field_group_assignments cfga ON cfg.id = cfga.group_id
WHERE cfga.entity_class = @entity_class
ORDER BY cfg.created_at DESC, cfg.id DESC;

-- name: GetCustomFieldGroupByID :one
SELECT id, tenant_id, name, slug, description, is_system, created_at
FROM custom_field_groups
WHERE id = @id;

-- name: GetCustomFieldGroupBySlug :one
SELECT id, tenant_id, name, slug, description, is_system, created_at
FROM custom_field_groups
WHERE slug = @slug;

-- name: CreateCustomFieldGroup :one
INSERT INTO custom_field_groups (name, slug, description, is_system) VALUES (@name, @slug, @description, @is_system)

RETURNING id, tenant_id, name, slug, description, is_system, created_at;

-- name: UpdateCustomFieldGroup :one
UPDATE custom_field_groups
SET name = @name, description = @description
WHERE id = @id AND is_system = false
RETURNING id, tenant_id, name, slug, description, is_system, created_at;

-- name: DeleteCustomFieldGroup :exec
DELETE FROM custom_field_groups
WHERE id = @id AND is_system = false;

-- name: CountCustomFieldValuesByGroup :one
-- BUG-008: counts custom_field_values referencing any field in the given
-- group, so DeleteFieldGroup can reject deletion when the group is in use.
-- Tenant-scoped via the JOIN to custom_fields.
SELECT count(*)::bigint
FROM custom_field_values cfv
JOIN custom_fields cf ON cfv.field_id = cf.id
WHERE cf.group_id = @group_id;

-- =====================
-- Custom Fields
-- =====================

-- name: ListCustomFieldsByGroup :many
SELECT id, group_id, label, key, field_type, options, embed_platforms,
 placeholder, default_value, is_required, is_system, show_in_frontend,
 filterable, sortable, searchable, position, created_at
FROM custom_fields
WHERE group_id = @group_id
ORDER BY position ASC, created_at ASC;

-- name: ListAllCustomFieldsByTenant :many
-- All fields for all groups belonging to a tenant, used to enrich
-- ListFieldGroups with fields[] without N+1 per-group field lookups.
SELECT id, group_id, label, key, field_type, options, embed_platforms,
 placeholder, default_value, is_required, is_system, show_in_frontend,
 filterable, sortable, searchable, position, created_at
FROM custom_fields

ORDER BY group_id, position ASC, created_at ASC;

-- name: GetCustomFieldByID :one
SELECT id, group_id, label, key, field_type, options, embed_platforms,
 placeholder, default_value, is_required, is_system, show_in_frontend,
 filterable, sortable, searchable, position, created_at
FROM custom_fields
WHERE id = @id;

-- name: GetCustomFieldByKey :one
-- ADR-064a: Looks up a system custom field by its key within a tenant.
-- Used by the users service to resolve field IDs for origin_country and
-- destination_country when saving geo profile fields.
SELECT id, group_id, label, key, field_type, options, embed_platforms,
 placeholder, default_value, is_required, is_system, show_in_frontend,
 filterable, sortable, searchable, position, created_at
FROM custom_fields
WHERE key = @key;

-- name: CreateCustomField :one
INSERT INTO custom_fields (group_id, label, key, field_type, options, embed_platforms, placeholder, default_value, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position) VALUES (@group_id, @label, @key, @field_type, @options, @embed_platforms, @placeholder, @default_value, @is_required, @is_system, @show_in_frontend, @filterable, @sortable, @searchable, @position)
RETURNING id, group_id, label, key, field_type, options, embed_platforms,
 placeholder, default_value, is_required, is_system, show_in_frontend,
 filterable, sortable, searchable, position, created_at;

-- name: UpdateCustomField :one
UPDATE custom_fields
SET label = @label, field_type = @field_type, options = @options,
 embed_platforms = @embed_platforms, placeholder = @placeholder,
 default_value = @default_value, is_required = @is_required,
 show_in_frontend = @show_in_frontend, filterable = @filterable,
 sortable = @sortable, searchable = @searchable, position = @position
WHERE id = @id AND is_system = false
RETURNING id, group_id, label, key, field_type, options, embed_platforms,
 placeholder, default_value, is_required, is_system, show_in_frontend,
 filterable, sortable, searchable, position, created_at;

-- name: DeleteCustomField :exec
DELETE FROM custom_fields
WHERE id = @id AND is_system = false;

-- name: UpdateCustomFieldPosition :exec
UPDATE custom_fields
SET position = @position
WHERE id = @id AND group_id = @group_id AND is_system = false;

-- =====================
-- Group Assignments
-- =====================

-- name: ListAssignmentsByEntityClass :many
SELECT id, group_id, entity_class, entity_id, position, created_at
FROM custom_field_group_assignments
WHERE entity_class = @entity_class
ORDER BY position ASC;

-- name: ListAssignmentsByTenantGroups :many
-- All assignments for groups belonging to a tenant, used to enrich
-- ListFieldGroups with entity_type without N+1 per-group assignment lookups.
SELECT cfga.id, cfga.group_id, cfga.entity_class, cfga.entity_id, cfga.position, cfga.created_at
FROM custom_field_group_assignments cfga
JOIN custom_field_groups cfg ON cfga.group_id = cfg.id

ORDER BY cfga.position ASC;

-- name: GetAssignmentByEntity :one
SELECT id, group_id, entity_class, entity_id, position, created_at
FROM custom_field_group_assignments
WHERE entity_class = @entity_class AND entity_id = @entity_id;

-- name: CreateAssignment :one
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position) VALUES (@group_id, @entity_class, sqlc.narg(entity_id), @position)

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
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit) VALUES (@field_id, @user_group_id, @can_view, @can_edit)

ON CONFLICT (field_id, user_group_id) DO UPDATE
SET can_view = EXCLUDED.can_view, can_edit = EXCLUDED.can_edit;

-- name: DeleteFieldPermission :exec
DELETE FROM custom_field_permissions
WHERE field_id = @field_id AND user_group_id = @user_group_id;

-- =====================
-- Field Values
-- =====================

-- name: GetCustomFieldValue :one
-- BUG-014: JOIN custom_fields to enforce tenant scoping at the SQL layer
-- (defense-in-depth). The custom_field_values table has no tenant_id column;
-- tenancy is enforced transitively via field_id → custom_fields.tenant_id.
SELECT cfv.id, cfv.field_id, cfv.entity_type, cfv.entity_id, cfv.value_text,
 cfv.value_number, cfv.value_bool, cfv.value_date, cfv.value_json,
 cfv.created_at, cfv.updated_at, cfv.ai_exempt
FROM custom_field_values cfv
JOIN custom_fields cf ON cfv.field_id = cf.id
WHERE cfv.field_id = @field_id AND cfv.entity_type = @entity_type AND cfv.entity_id = @entity_id;

-- name: ListCustomFieldValuesByEntity :many
-- BUG-014: JOIN custom_fields to enforce tenant scoping at the SQL layer.
SELECT cfv.id, cfv.field_id, cfv.entity_type, cfv.entity_id, cfv.value_text,
 cfv.value_number, cfv.value_bool, cfv.value_date, cfv.value_json,
 cfv.created_at, cfv.updated_at, cfv.ai_exempt
FROM custom_field_values cfv
JOIN custom_fields cf ON cfv.field_id = cf.id
WHERE cfv.entity_type = @entity_type AND cfv.entity_id = @entity_id;

-- name: ListCustomFieldValuesByEntityForUser :many
-- BUG-017: push the visible-field-ID filter into SQL instead of fetching
-- all values and filtering in Go. The visible_field_ids array is the set
-- of field IDs the user is allowed to view per field-level RBAC.
-- Also BUG-014: tenant-scoped via JOIN to custom_fields.
SELECT cfv.id, cfv.field_id, cfv.entity_type, cfv.entity_id, cfv.value_text,
 cfv.value_number, cfv.value_bool, cfv.value_date, cfv.value_json,
 cfv.created_at, cfv.updated_at, cfv.ai_exempt
FROM custom_field_values cfv
JOIN custom_fields cf ON cfv.field_id = cf.id
WHERE cfv.entity_type = @entity_type AND cfv.entity_id = @entity_id
 AND cfv.field_id = ANY(@visible_field_ids::uuid[]);

-- name: ListCustomFieldValuesWithDefsByEntity :many
-- Display-ready field values: joins custom_field_values with their
-- custom_fields definitions (label, key, field_type, options, position) so
-- a single row carries both the stored value and the metadata needed to
-- render it. Filtered to show_in_frontend = true so admin-only fields never
-- leak to public surfaces (e.g. the /universities/{slug} About facts grid,
-- Section 39). Tenant-scoped via the JOIN (BUG-014). Ordered by the field's
-- group position for stable display ordering.
SELECT cfv.id, cfv.field_id, cfv.entity_type, cfv.entity_id, cfv.value_text,
 cfv.value_number, cfv.value_bool, cfv.value_date, cfv.value_json,
 cfv.created_at, cfv.updated_at,
 cf.label AS field_label, cf.key AS field_key, cf.field_type,
 cf.options AS field_options, cf.position AS field_position
FROM custom_field_values cfv
JOIN custom_fields cf ON cfv.field_id = cf.id
WHERE cfv.entity_type = @entity_type AND cfv.entity_id = @entity_id
 AND cf.show_in_frontend = true
ORDER BY cf.position ASC, cf.created_at ASC;

-- name: UpsertCustomFieldValue :one
-- Tenant scoping for upsert is enforced at the handler layer via a
-- GetField(fieldID, tenantID) pre-check (the handler rejects 404 before
-- reaching this query). The custom_field_values table has no tenant_id
-- column, so the INSERT itself cannot be tenant-scoped at the SQL level.
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_text, value_number, value_bool, value_date, value_json) VALUES (@field_id, @entity_type, @entity_id, sqlc.narg(value_text), sqlc.narg(value_number), sqlc.narg(value_bool), sqlc.narg(value_date), sqlc.narg(value_json))

ON CONFLICT (field_id, entity_type, entity_id) DO UPDATE
SET value_text = EXCLUDED.value_text,
 value_number = EXCLUDED.value_number,
 value_bool = EXCLUDED.value_bool,
 value_date = EXCLUDED.value_date,
 value_json = EXCLUDED.value_json,
 updated_at = now()
RETURNING id, field_id, entity_type, entity_id, value_text, value_number,
 value_bool, value_date, value_json, created_at, updated_at, ai_exempt;

-- name: DeleteCustomFieldValue :exec
-- BUG-014: tenant scoping via a subquery check on custom_fields.tenant_id.
DELETE FROM custom_field_values
WHERE field_id = @field_id AND entity_type = @entity_type AND entity_id = @entity_id
 AND EXISTS (SELECT 1 FROM custom_fields cf WHERE cf.id = custom_field_values.field_id);

-- name: CheckFieldPermissionForUser :one
-- Field-level RBAC (Section 06): check whether a user belongs to any
-- user_group that has can_view / can_edit on the given field. Returns
-- a row with two booleans. When no permission rows exist for the field,
-- both default to false (fail-closed) — admins must explicitly grant
-- visibility/editability per user group. Super-admins bypass this check
-- at the handler layer via rbac.IsSuperAdmin.
SELECT
 COALESCE(bool_or(cfp.can_view), false) AS can_view,
 COALESCE(bool_or(cfp.can_edit), false) AS can_edit
FROM custom_field_permissions cfp
JOIN user_group_memberships ugm ON ugm.group_id = cfp.user_group_id
WHERE cfp.field_id = @field_id
 AND ugm.user_id = @user_id;

-- name: ListVisibleFieldIDsForUser :many
-- Returns the field_ids the given user is allowed to view. Used by
-- ListFieldValuesByEntity to filter out values for fields the user
-- cannot see (Section 06 field-level RBAC).
SELECT DISTINCT cfp.field_id
FROM custom_field_permissions cfp
JOIN user_group_memberships ugm ON ugm.group_id = cfp.user_group_id
WHERE ugm.user_id = @user_id
 AND cfp.can_view = true;
