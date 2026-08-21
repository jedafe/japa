-- 000536_control_canonical_permissions.up.sql
-- Adds the canonical system.* permission nodes for the Control Panel
-- as specified in Section 21 (system.tenants.manage, system.settings.view,
-- system.settings.edit) and maps any existing grants from the legacy
-- control.* variants to the canonical nodes.

BEGIN;

-- 1. Insert canonical permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('system.tenants.manage', 'Manage tenants'),
    ('system.settings.view',  'View system settings'),
    ('system.settings.edit',  'Edit system settings')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant to the super_admin role on the default tenant.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
JOIN permissions p ON p.key IN ('system.tenants.manage', 'system.settings.view', 'system.settings.edit')
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
ON CONFLICT DO NOTHING;

-- 3. Migrate legacy control.* grants to the canonical system.* nodes.
-- control.tenants.manage -> system.tenants.manage
INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, new_perm.id
FROM role_permissions rp
JOIN permissions old_perm ON old_perm.id = rp.permission_id AND old_perm.key = 'control.tenants.manage'
JOIN permissions new_perm ON new_perm.key = 'system.tenants.manage'
ON CONFLICT DO NOTHING;

-- control.settings.manage -> system.settings.view and system.settings.edit
INSERT INTO role_permissions (role_id, permission_id)
SELECT rp.role_id, new_perm.id
FROM role_permissions rp
JOIN permissions old_perm ON old_perm.id = rp.permission_id AND old_perm.key = 'control.settings.manage'
JOIN permissions new_perm ON new_perm.key IN ('system.settings.view', 'system.settings.edit')
ON CONFLICT DO NOTHING;

-- 4. Remove the legacy control.* nodes (and their grants) now that canonical
-- nodes are in place and grants have been migrated.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('control.tenants.manage', 'control.settings.manage')
);

DELETE FROM permissions
WHERE key IN ('control.tenants.manage', 'control.settings.manage');

COMMIT;
