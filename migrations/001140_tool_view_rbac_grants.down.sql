-- 001140_tool_view_rbac_grants.down.sql
-- Removes the tool .view grants added by the up migration. The permission
-- nodes themselves predate this migration and are left in place.

BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE slug IN ('member', 'pro_member', 'business_member', 'guest', 'super_admin')
)
AND permission_id IN (
    SELECT id FROM permissions WHERE key = 'credential_checker.view'
);

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE slug IN ('pro_member', 'business_member', 'guest')
)
AND permission_id IN (
    SELECT id FROM permissions WHERE key = 'eca_guide.view'
);

COMMIT;
