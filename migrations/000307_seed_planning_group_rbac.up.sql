-- 000307_seed_planning_group_rbac.up.sql
-- RBAC nodes for Japa Planning Groups (§82, ADR-090).
--   planning_group.create      — member+
--   planning_group.manage_own  — group owner
--   planning_group.manage_any  — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('planning_group.create',     'Create a planning group (member+)'),
    ('planning_group.manage_own', 'Manage own planning group (owner)'),
    ('planning_group.manage_any', 'Manage any planning group (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'planning_group.create'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'planning_group.manage_any'
ON CONFLICT DO NOTHING;

COMMIT;
