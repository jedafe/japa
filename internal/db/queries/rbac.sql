-- RBAC store queries: user groups, roles, permissions, and their mappings.

-- =====================
-- Group Operations
-- =====================

-- name: ListGroupMembersByGroupID :many
SELECT user_id FROM user_group_memberships WHERE group_id = @group_id;

-- name: CreateUserGroup :one
INSERT INTO user_groups (name, slug, description) VALUES (@name, @slug, @description)

RETURNING id;

-- name: DeleteUserGroup :exec
DELETE FROM user_groups WHERE slug = @slug AND is_system = false;

-- name: DeleteUserGroupByID :exec
DELETE FROM user_groups WHERE id = @id AND is_system = false;

-- name: GetUserGroupByID :one
SELECT id, name, slug, is_system, description
FROM user_groups
WHERE id = @id;

-- name: AddUserToGroup :exec
INSERT INTO user_group_memberships (user_id, group_id) VALUES (@user_id, @group_id)

ON CONFLICT (user_id, group_id) DO NOTHING;

-- name: RemoveUserFromGroup :exec
DELETE FROM user_group_memberships
WHERE user_id = @user_id AND group_id = @group_id;

-- name: RetireMentorWithoutBecomeMentorPerm :exec
-- 000824/§102: when a user loses the mentorship.become_mentor node (group
-- removal), their mentor shadow listing must leave the directory — the
-- shadow would otherwise stay published behind a mentor who can no longer
-- respond to requests (PATCH/respond are permission-gated). Soft-deletes
-- the shadow and clears is_mentor + mentor_listing_content_id in one pass.
-- No-op while any remaining group still grants the node.
WITH has_perm AS (
    SELECT 1
    FROM user_group_memberships ugm
    JOIN group_roles gr ON gr.group_id = ugm.group_id
    JOIN role_permissions rp ON rp.role_id = gr.role_id
    JOIN permissions p ON rp.permission_id = p.id
    WHERE ugm.user_id = @user_id
      AND (p.key = 'mentorship.become_mentor' OR p.key = '*')
), retired AS (
    UPDATE contents c
    SET deleted_at = now(), updated_at = now()
    WHERE c.author_id = @user_id
      AND c.content_type = 'service_listing'
      AND c.deleted_at IS NULL
      AND NOT EXISTS (SELECT 1 FROM has_perm)
      AND EXISTS (
          SELECT 1 FROM service_listing_meta sm
          WHERE sm.content_id = c.id AND sm.listing_kind = 'mentorship')
    RETURNING c.id
)
UPDATE user_profiles up
SET is_mentor = false,
    mentor_listing_content_id = NULL,
    updated_at = now()
WHERE up.user_id = @user_id
  AND up.is_mentor = true
  AND NOT EXISTS (SELECT 1 FROM has_perm);

-- name: RetireGroupMentorsWithoutBecomeMentorPerm :exec
-- Bulk variant of RetireMentorWithoutBecomeMentorPerm for role→group
-- detach and role→permission revoke demotions (§102/000824): retires the
-- mentor profile + shadow for every affected member who no longer holds
-- mentorship.become_mentor (or '*') through ANY remaining group.
WITH demoted AS (
    SELECT ugm.user_id
    FROM user_group_memberships ugm
    WHERE ugm.group_id = @group_id
      AND NOT EXISTS (
          SELECT 1 FROM user_group_memberships ugm2
          JOIN group_roles gr ON gr.group_id = ugm2.group_id
          JOIN role_permissions rp ON rp.role_id = gr.role_id
          JOIN permissions p ON rp.permission_id = p.id
          WHERE ugm2.user_id = ugm.user_id
            AND (p.key = 'mentorship.become_mentor' OR p.key = '*'))
), retired AS (
    UPDATE contents c
    SET deleted_at = now(), updated_at = now()
    WHERE c.author_id IN (SELECT user_id FROM demoted)
      AND c.content_type = 'service_listing'
      AND c.deleted_at IS NULL
      AND EXISTS (
          SELECT 1 FROM service_listing_meta sm
          WHERE sm.content_id = c.id AND sm.listing_kind = 'mentorship')
    RETURNING c.id
)
UPDATE user_profiles up
SET is_mentor = false,
    mentor_listing_content_id = NULL,
    updated_at = now()
WHERE up.user_id IN (SELECT user_id FROM demoted)
  AND up.is_mentor = true;

-- name: ListUserGroups :many
SELECT id, name, slug, is_system, description
FROM user_groups

ORDER BY name;

-- name: ListUserGroupsPaged :many
-- Paginated variant for the admin groups table — supports search over
-- name/description and whitelisted sort on name/member_count. member_count
-- is computed inline so ordering by it works in the same query.
SELECT sub.id, sub.name, sub.slug, sub.is_system, sub.description, sub.member_count
FROM (
 SELECT g.id, g.name, g.slug, g.is_system, g.description,
  (SELECT COUNT(*)::int FROM user_group_memberships ugm WHERE ugm.group_id = g.id) AS member_count
 FROM user_groups g
 WHERE (@search::text = '' OR g.name ILIKE '%' || @search || '%' OR COALESCE(g.description, '') ILIKE '%' || @search || '%')
) sub
ORDER BY
 CASE WHEN @sort_key::text = 'name' AND @sort_dir::text = 'desc' THEN sub.name END DESC,
 CASE WHEN @sort_key::text = 'member_count' AND @sort_dir::text = 'asc' THEN sub.member_count END ASC,
 CASE WHEN @sort_key::text = 'member_count' AND @sort_dir::text = 'desc' THEN sub.member_count END DESC,
 sub.name
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUserGroups :one
SELECT COUNT(*)::int AS count
FROM user_groups g
WHERE (@search::text = '' OR g.name ILIKE '%' || @search || '%' OR COALESCE(g.description, '') ILIKE '%' || @search || '%');

-- name: CountGroupMembers :many
SELECT ug.id AS group_id, COUNT(ugm.user_id)::int AS member_count
FROM user_groups ug
LEFT JOIN user_group_memberships ugm ON ugm.group_id = ug.id

GROUP BY ug.id;

-- name: ListGroupPermissions :many
SELECT DISTINCT ug.id AS group_id, p.key
FROM user_groups ug
JOIN group_roles gr ON gr.group_id = ug.id
JOIN role_permissions rp ON rp.role_id = gr.role_id
JOIN permissions p ON rp.permission_id = p.id

ORDER BY ug.id, p.key;

-- name: ListGroupMembersWithUser :many
SELECT u.id, u.username, COALESCE(up.display_name, '')
FROM user_group_memberships ugm
JOIN users u ON ugm.user_id = u.id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE ugm.group_id = @group_id
ORDER BY u.username;

-- name: UpdateUserGroup :exec
UPDATE user_groups SET name = @name, slug = @slug, description = @description WHERE id = @id;

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
-- user_count = distinct users holding the role through their group
-- memberships (role → group_roles → user_group_memberships), not the
-- number of groups assigned the role.
SELECT sub.id, sub.name, sub.slug, sub.user_count
FROM (
 SELECT ur.id, ur.name, ur.slug,
  (SELECT COUNT(DISTINCT ugm.user_id)::int
   FROM group_roles gr
   JOIN user_group_memberships ugm ON ugm.group_id = gr.group_id
   WHERE gr.role_id = ur.id) AS user_count
 FROM user_roles ur
 WHERE (@search::text = '' OR ur.name ILIKE '%' || @search || '%')
) sub
ORDER BY
 CASE WHEN @sort_key::text = 'name' AND @sort_dir::text = 'desc' THEN sub.name END DESC,
 CASE WHEN @sort_key::text = 'user_count' AND @sort_dir::text = 'asc' THEN sub.user_count END ASC,
 CASE WHEN @sort_key::text = 'user_count' AND @sort_dir::text = 'desc' THEN sub.user_count END DESC,
 sub.name
LIMIT @page_limit OFFSET @page_offset;

-- name: CountRoles :one
SELECT COUNT(*)::int AS count
FROM user_roles
WHERE (@search::text = '' OR name ILIKE '%' || @search || '%');

-- name: ListRolePermissions :many
SELECT rp.role_id, p.key, p.description
FROM role_permissions rp
JOIN user_roles ur ON rp.role_id = ur.id
JOIN permissions p ON rp.permission_id = p.id

ORDER BY rp.role_id, p.key;

-- name: CountGroupsByRole :many
SELECT gr.role_id, COUNT(gr.group_id)::int AS group_count
FROM group_roles gr
JOIN user_roles ur ON gr.role_id = ur.id

GROUP BY gr.role_id;

-- name: CreateUserRole :one
INSERT INTO user_roles (name, slug) VALUES (@name, @slug)

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
-- role_permissions and group_roles have ON DELETE CASCADE from user_roles,
-- so deleting the user_roles row cleans up all dependent rows automatically.
DELETE FROM user_roles WHERE id = @role_id;

-- name: UpdateUserRole :exec
UPDATE user_roles SET name = @name WHERE id = @id;

-- =====================
-- Permission Operations
-- =====================

-- name: RegisterPermission :exec
INSERT INTO permissions (key, description) VALUES (@key, @description)

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
INSERT INTO group_roles (group_id, role_id) VALUES (@group_id, @role_id)

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
WHERE ugm.user_id = @user_id;

-- name: ResolveUserGroups :many
SELECT ug.id, ug.name, ug.slug, ug.is_system
FROM user_group_memberships ugm
JOIN user_groups ug ON ugm.group_id = ug.id
WHERE ugm.user_id = @user_id;

-- name: IsUserInGroup :one
SELECT EXISTS(
 SELECT 1
 FROM user_group_memberships ugm
 JOIN user_groups ug ON ugm.group_id = ug.id
 WHERE ugm.user_id = @user_id AND ug.slug = @group_slug
) AS in_group;

-- name: ListUserIDsByPermissionKey :many
-- Returns the distinct user IDs that hold the given permission key via the
-- RBAC chain: user_group_memberships → user_groups → group_roles →
-- role_permissions → permissions. Used by report:notify_admin to fan out
-- moderation notifications to every user with reports.review.
SELECT DISTINCT ugm.user_id
FROM user_group_memberships ugm
JOIN user_groups ug ON ugm.group_id = ug.id
JOIN group_roles gr ON gr.group_id = ug.id
JOIN role_permissions rp ON rp.role_id = gr.role_id
JOIN permissions p ON rp.permission_id = p.id
WHERE p.key = @permission_key;

-- name: GetUserGroupBySlug :one
-- Resolves a group by slug — used by the owner/super_admin lifecycle paths to
-- find the super_admin group (§03 owner amendment).
SELECT id, name, slug, is_system, description
FROM user_groups
WHERE slug = @slug;

-- name: IsUserInGroupID :one
-- Membership check by group id — validates a chosen public_group_id belongs to
-- the user's own groups (§03 alias amendment).
SELECT EXISTS(
 SELECT 1 FROM user_group_memberships
 WHERE user_id = @user_id AND group_id = @group_id
) AS in_group;

-- name: ClearPublicGroupRole :exec
-- §03 alias amendment: when a user loses membership of the group they chose
-- as their public role, clear the dangling pointer so public surfaces no
-- longer show a role they no longer hold.
UPDATE users SET public_group_id = NULL, updated_at = now()
WHERE id = @user_id AND public_group_id = @group_id;
