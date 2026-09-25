-- 000309_seed_visa_timeline_rbac.up.sql
-- RBAC nodes for Visa Timeline Tracker (§83, ADR-091).
--   visa_timeline.create      — member+
--   visa_timeline.manage_own  — member+
--   visa_timeline.manage_any  — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('visa_timeline.create',     'Create a visa timeline (member+)'),
    ('visa_timeline.manage_own', 'Manage own visa timeline (member+)'),
    ('visa_timeline.manage_any', 'Manage any visa timeline (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key IN ('visa_timeline.create','visa_timeline.manage_own')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'visa_timeline.manage_any'
ON CONFLICT DO NOTHING;

COMMIT;
