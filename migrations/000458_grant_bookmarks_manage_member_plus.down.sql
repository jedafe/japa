-- Reverts 000458_grant_bookmarks_manage_member_plus.
-- Removes the bookmarks.manage grant added for member+ roles. Note: this
-- returns the system to the pre-fix state where no role holds the node and
-- the /me/bookmark-folders mutating routes (which enforce it) would deny
-- all callers — re-apply 000458 before deploying the route wiring.
BEGIN;

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin')
  AND p.key = 'bookmarks.manage';

COMMIT;
