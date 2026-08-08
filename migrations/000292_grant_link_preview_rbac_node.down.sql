-- 000292_grant_link_preview_rbac_node.down.sql
-- Reverses 000292: removes the link_preview.fetch grant from the member role.
-- The permission node itself remains — it is owned by migrations 000099/000109.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'link_preview.fetch'
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'member'
);

COMMIT;
