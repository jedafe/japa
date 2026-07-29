-- 000184_seed_control_panel_access_rbac_node.up.sql
-- Seeds the control_panel.access RBAC node that gates entry to the Control
-- Panel SPA (apps/control). The node is granted to the super_admin_role so
-- the wildcard '*' is not the only path to access — explicit grants keep the
-- node visible in role-permission listings and survive a wildcard removal.
-- Ref: japaconnect-engineering/sections/03-rbac.md

BEGIN;

-- 1. Insert the control_panel.access permission node.
INSERT INTO permissions (key, description) VALUES
    ('control_panel.access', 'Access the Control Panel SPA (apps/control, port 3002)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant to super_admin_role (idempotent). Super-admin also bypasses RBAC
-- via the wildcard '*' node (Section 03), but the explicit grant keeps the
-- node visible in role-permission listings.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key = 'control_panel.access'
ON CONFLICT DO NOTHING;

COMMIT;
