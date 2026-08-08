-- RBAC store queries: user groups, roles, permissions, and their mappings.

-- =====================
-- Group Operations
-- =====================

-- name: ListGroupMembersByGroupID :many
SELECT user_id FROM user_group_memberships WHERE group_id = @group_id;

-- name: CreateUserGroup :one
INSERT INTO user_groups (tenant_id, name, slug)
VALUES (@tenant_id, @name, @slug)
RETURNING id;

-- name: DeleteUserGroup :exec
DELETE FROM user_groups WHERE tenant_id = @tenant_id AND slug = @slug AND is_system = false;

-- name: DeleteUserGroupByID :exec
DELETE FROM user_groups WHERE id = @id AND is_system = false;

-- name: GetUserGroupByID :one
SELECT id, tenant_id, name, slug, is_system
FROM user_groups
WHERE id = @id;

-- name: AddUserToGroup :exec
INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
VALUES (@tenant_id, @user_id, @group_id)
ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING;

-- name: RemoveUserFromGroup :exec
DELETE FROM user_group_memberships
WHERE tenant_id = @tenant_id AND user_id = @user_id AND group_id = @group_id;

-- name: ListUserGroupsByTenant :many
SELECT id, tenant_id, name, slug, is_system
FROM user_groups
WHERE tenant_id = @tenant_id
ORDER BY name;

-- name: CountGroupMembersByTenant :many
SELECT ug.id AS group_id, COUNT(ugm.user_id)::int AS member_count
FROM user_groups ug
LEFT JOIN user_group_memberships ugm ON ugm.group_id = ug.id
WHERE ug.tenant_id = @tenant_id
GROUP BY ug.id;

-- name: ListGroupPermissionsByTenant :many
SELECT DISTINCT ug.id AS group_id, p.key
FROM user_groups ug
JOIN group_roles gr ON gr.group_id = ug.id
JOIN role_permissions rp ON rp.role_id = gr.role_id
JOIN permissions p ON rp.permission_id = p.id
WHERE ug.tenant_id = @tenant_id
ORDER BY ug.id, p.key;

-- name: ListGroupMembersWithUser :many
SELECT u.id, u.username, COALESCE(up.display_name, '')
FROM user_group_memberships ugm
JOIN users u ON ugm.user_id = u.id
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = ugm.tenant_id
WHERE ugm.group_id = @group_id AND ugm.tenant_id = @tenant_id
ORDER BY u.username;

-- name: UpdateUserGroup :exec
UPDATE user_groups SET name = @name, slug = @slug WHERE id = @id;

-- =====================
-- Role Operations
-- =====================

-- name: ListAllPermissions :many
SELECT key, description
FROM permissions
ORDER BY key;

-- name: ListAllRoles :many
SELECT id, name, slug
FROM user_roles
ORDER BY name;

-- name: ListRolesPaginated :many
SELECT id, name, slug
FROM user_roles
WHERE (@search::text = '' OR name ILIKE '%' || @search || '%')
ORDER BY name
LIMIT @page_limit OFFSET @page_offset;

-- name: CountRoles :one
SELECT COUNT(*)::int AS count
FROM user_roles
WHERE (@search::text = '' OR name ILIKE '%' || @search || '%');

-- name: CreateUserRole :one
INSERT INTO user_roles (tenant_id, name, slug)
VALUES (@tenant_id, @name, @slug)
RETURNING id;

-- name: ListRolePermissionsByKey :many
SELECT p.key, p.description
FROM role_permissions rp
JOIN permissions p ON rp.permission_id = p.id
WHERE rp.role_id = @role_id
ORDER BY p.key;

-- name: ListGroupRolesByGroupID :many
SELECT ur.id, ur.name, ur.slug
FROM group_roles gr
JOIN user_roles ur ON gr.role_id = ur.id
WHERE gr.group_id = @group_id
ORDER BY ur.name;

-- name: RemoveRoleFromGroup :exec
DELETE FROM group_roles WHERE group_id = @group_id AND role_id = @role_id;

-- name: DeleteUserRole :exec
DELETE FROM role_permissions WHERE role_id = @role_id;
DELETE FROM group_roles WHERE role_id = @role_id;
DELETE FROM user_roles WHERE id = @role_id;

-- name: UpdateUserRole :exec
UPDATE user_roles SET name = @name WHERE id = @id;

-- =====================
-- Permission Operations
-- =====================

-- name: RegisterPermission :exec
INSERT INTO permissions (key, description)
VALUES (@key, @description)
ON CONFLICT (key) DO NOTHING;

-- name: AssignPermissionToRole :exec
INSERT INTO role_permissions (role_id, permission_id)
SELECT @role_id, id FROM permissions WHERE key = @perm_key
ON CONFLICT DO NOTHING;

-- name: ListGroupsByRoleID :many
SELECT group_id FROM group_roles WHERE role_id = @role_id;

-- name: RemovePermissionFromRole :exec
DELETE FROM role_permissions
WHERE role_id = @role_id AND permission_id = (SELECT id FROM permissions WHERE key = @perm_key);

-- name: AssignRoleToGroup :exec
INSERT INTO group_roles (group_id, role_id)
VALUES (@group_id, @role_id)
ON CONFLICT DO NOTHING;

-- =====================
-- Role Permission Management
-- =====================

-- name: DeleteRolePermissionsByRoleID :exec
DELETE FROM role_permissions WHERE role_id = @role_id;

-- name: InsertRolePermissionByKey :exec
INSERT INTO role_permissions (role_id, permission_id)
SELECT @role_id, id FROM permissions WHERE key = @perm_key
ON CONFLICT DO NOTHING;

-- =====================
-- RBAC Resolution (for rbac.go)
-- =====================

-- name: ResolveUserPermissions :many
SELECT DISTINCT p.key
FROM user_group_memberships ugm
JOIN user_groups ug ON ugm.group_id = ug.id
JOIN group_roles gr ON gr.group_id = ug.id
JOIN user_roles ur ON gr.role_id = ur.id
JOIN role_permissions rp ON rp.role_id = ur.id
JOIN permissions p ON rp.permission_id = p.id
WHERE ugm.tenant_id = @tenant_id AND ugm.user_id = @user_id;

-- name: ResolveUserGroups :many
SELECT ug.id, ug.tenant_id, ug.name, ug.slug, ug.is_system
FROM user_group_memberships ugm
JOIN user_groups ug ON ugm.group_id = ug.id
WHERE ugm.tenant_id = @tenant_id AND ugm.user_id = @user_id;

-- name: IsUserInGroup :one
SELECT EXISTS(
    SELECT 1
    FROM user_group_memberships ugm
    JOIN user_groups ug ON ugm.group_id = ug.id
    WHERE ugm.tenant_id = @tenant_id AND ugm.user_id = @user_id AND ug.slug = @group_slug
) AS in_group;
