-- 000138_seed_new_entity_rbac_nodes.up.sql
-- Seeds RBAC permission nodes for the three new entities created in 000136
-- and grants them to the super_admin_role.
-- Ref: ADR-054

BEGIN;

-- 1. Insert new permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('menus.manage', 'Manage navigation menus and menu items'),
    ('email.templates.manage', 'Manage email templates (view, edit, test, reset)'),
    ('redirects.manage', 'Manage URL redirect rules')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant to super_admin_role (idempotent).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN ('menus.manage', 'email.templates.manage', 'redirects.manage')
ON CONFLICT DO NOTHING;

COMMIT;
