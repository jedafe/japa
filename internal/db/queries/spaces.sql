-- Space type, space, space rule, and permission preset queries.

-- =====================
-- Space Permission Presets
-- =====================

-- name: ListPermissionPresetsByTenant :many
SELECT id, tenant_id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at
FROM space_permission_presets
WHERE tenant_id = @tenant_id
ORDER BY created_at ASC, id ASC;

-- name: GetPermissionPresetByName :one
SELECT id, tenant_id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at
FROM space_permission_presets
WHERE name = @name AND tenant_id = @tenant_id;

-- name: UpdatePermissionPreset :one
UPDATE space_permission_presets
SET visibility = @visibility, posting_perm = @posting_perm, replying_perm = @replying_perm,
    commenting_perm = @commenting_perm, joining_perm = @joining_perm
WHERE id = @id AND tenant_id = @tenant_id AND is_system = true
RETURNING id, tenant_id, name, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, is_system, created_at, updated_at;

-- =====================
-- Space Types
-- =====================

-- name: ListSpaceTypesByTenant :many
SELECT id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at
FROM space_types
WHERE tenant_id = @tenant_id
ORDER BY created_at ASC, id ASC;

-- name: GetSpaceTypeByID :one
SELECT id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at
FROM space_types
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetSpaceTypeBySlug :one
SELECT id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at
FROM space_types
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: CreateSpaceType :one
INSERT INTO space_types (tenant_id, name, slug, description, icon, field_group_id, is_system)
VALUES (@tenant_id, @name, @slug, sqlc.narg(description), sqlc.narg(icon), sqlc.narg(field_group_id), @is_system)
RETURNING id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at;

-- name: UpdateSpaceType :one
UPDATE space_types
SET name = @name, description = sqlc.narg(description), icon = sqlc.narg(icon),
    field_group_id = sqlc.narg(field_group_id)
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false
RETURNING id, tenant_id, name, slug, description, icon, field_group_id, is_system, created_at, updated_at;

-- name: DeleteSpaceType :exec
DELETE FROM space_types
WHERE id = @id AND tenant_id = @tenant_id AND is_system = false;

-- =====================
-- Spaces
-- =====================

-- name: ListSpacesByTenant :many
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC, id DESC;

-- name: ListSpacesByLevel :many
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE tenant_id = @tenant_id AND level = @level
ORDER BY created_at DESC, id DESC;

-- name: ListSpacesByType :many
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE tenant_id = @tenant_id AND space_type_id = @space_type_id
ORDER BY created_at DESC, id DESC;

-- name: ListChildSpaces :many
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE parent_id = @parent_id AND tenant_id = @tenant_id
ORDER BY created_at ASC, id ASC;

-- name: GetSpaceByID :one
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetSpaceBySlug :one
SELECT id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
       logo_media_id, header_image_id, geo_code, level, member_count, post_count,
       follower_count, visibility, posting_perm, replying_perm, commenting_perm,
       joining_perm, status, created_at, updated_at
FROM spaces
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: CreateSpace :one
INSERT INTO spaces (
    tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
    geo_code, level, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status
) VALUES (
    @tenant_id, sqlc.narg(parent_id), @space_type_id, @name, @slug, sqlc.narg(description),
    sqlc.narg(wiki_content), sqlc.narg(geo_code), @level, @visibility, @posting_perm,
    @replying_perm, @commenting_perm, @joining_perm, @status
)
RETURNING id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
          logo_media_id, header_image_id, geo_code, level, member_count, post_count,
          follower_count, visibility, posting_perm, replying_perm, commenting_perm,
          joining_perm, status, created_at, updated_at;

-- name: UpdateSpace :one
UPDATE spaces
SET name = @name, description = sqlc.narg(description), wiki_content = sqlc.narg(wiki_content),
    geo_code = sqlc.narg(geo_code), visibility = @visibility, posting_perm = @posting_perm,
    replying_perm = @replying_perm, commenting_perm = @commenting_perm, joining_perm = @joining_perm,
    status = @status
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, parent_id, space_type_id, name, slug, description, wiki_content,
          logo_media_id, header_image_id, geo_code, level, member_count, post_count,
          follower_count, visibility, posting_perm, replying_perm, commenting_perm,
          joining_perm, status, created_at, updated_at;

-- name: DeleteSpace :exec
DELETE FROM spaces
WHERE id = @id AND tenant_id = @tenant_id;

-- name: IncrementSpaceCount :exec
UPDATE spaces
SET member_count = member_count + @delta
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Space Rules
-- =====================

-- name: ListSpaceRulesBySpace :many
SELECT id, space_id, title, body, position, created_at, updated_at
FROM space_rules
WHERE space_id = @space_id
ORDER BY position ASC, created_at ASC;

-- name: CreateSpaceRule :one
INSERT INTO space_rules (space_id, title, body, position)
VALUES (@space_id, @title, @body, @position)
RETURNING id, space_id, title, body, position, created_at, updated_at;

-- name: UpdateSpaceRule :one
UPDATE space_rules
SET title = @title, body = @body, position = @position
WHERE id = @id AND space_id = @space_id
RETURNING id, space_id, title, body, position, created_at, updated_at;

-- name: DeleteSpaceRule :exec
DELETE FROM space_rules
WHERE id = @id AND space_id = @space_id;

-- =====================
-- Space Members
-- =====================

-- name: CheckSpaceMembership :one
SELECT EXISTS(
    SELECT 1 FROM space_members
    WHERE space_id = @space_id AND user_id = @user_id AND tenant_id = @tenant_id
) AS is_member;

-- name: AddSpaceMember :exec
INSERT INTO space_members (space_id, user_id, tenant_id, role)
VALUES (@space_id, @user_id, @tenant_id, 'member')
ON CONFLICT (space_id, user_id) DO NOTHING;

-- name: RemoveSpaceMember :exec
DELETE FROM space_members
WHERE space_id = @space_id AND user_id = @user_id AND tenant_id = @tenant_id;

-- name: CountSpaceMembers :one
SELECT COUNT(*) AS count
FROM space_members
WHERE space_id = @space_id AND tenant_id = @tenant_id;

-- =====================
-- Space Moderators
-- =====================

-- name: AssignSpaceModerator :exec
INSERT INTO space_moderators (tenant_id, space_id, user_id, assigned_by, assigned_by_user_id)
VALUES (@tenant_id, @space_id, @user_id, @assigned_by, sqlc.narg(assigned_by_user_id))
ON CONFLICT (tenant_id, space_id, user_id) DO NOTHING;

-- name: RemoveSpaceModerator :exec
DELETE FROM space_moderators
WHERE space_id = @space_id AND user_id = @user_id;

-- name: ListSpaceModerators :many
SELECT sm.id, sm.tenant_id, sm.space_id, sm.user_id, sm.assigned_by,
       sm.assigned_by_user_id, sm.created_at,
       u.username AS user_username, up.display_name AS user_display_name
FROM space_moderators sm
JOIN users u ON u.id = sm.user_id
LEFT JOIN user_profiles up ON up.user_id = sm.user_id AND up.tenant_id = sm.tenant_id
WHERE sm.space_id = @space_id
ORDER BY sm.created_at ASC;

-- name: GetSpaceModerator :one
SELECT id, tenant_id, space_id, user_id, assigned_by, assigned_by_user_id, created_at
FROM space_moderators
WHERE space_id = @space_id AND user_id = @user_id;