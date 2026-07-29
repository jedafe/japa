-- 000184_seed_control_panel_access_rbac_node.down.sql
-- Reverses the up migration: removes the control_panel.access permission node
-- and its grant to super_admin_role.

BEGIN;

-- 1. Remove the role_permissions grant.
DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id FROM permissions WHERE key = 'control_panel.access'
);

-- 2. Remove the permission node itself.
DELETE FROM permissions WHERE key = 'control_panel.access';

COMMIT;
