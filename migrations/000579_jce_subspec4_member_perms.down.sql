-- 000579_jce_subspec4_member_perms.down.sql
-- Remove the member grants added in 000579.

BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'member'
)
AND permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('jce.refresh.trigger', 'jce.ai_advisor.use')
);

COMMIT;
