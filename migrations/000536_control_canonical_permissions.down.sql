-- 000536_control_canonical_permissions.down.sql
-- Reverts 000536: removes canonical system.* control-panel permission nodes
-- and re-seeds the legacy control.* variants for super_admin.

BEGIN;

-- 1. Remove canonical grants first (FK safety).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('system.tenants.manage', 'system.settings.view', 'system.settings.edit')
);

-- 2. Remove canonical permission nodes.
DELETE FROM permissions
WHERE key IN ('system.tenants.manage', 'system.settings.view', 'system.settings.edit');

-- 3. Re-seed legacy nodes.
INSERT INTO permissions (key, description) VALUES
    ('control.tenants.manage', 'Manage tenants'),
    ('control.settings.manage', 'Manage system settings')
ON CONFLICT (key) DO NOTHING;

-- 4. Grant legacy nodes to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
JOIN permissions p ON p.key IN ('control.tenants.manage', 'control.settings.manage')
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
ON CONFLICT DO NOTHING;

COMMIT;
