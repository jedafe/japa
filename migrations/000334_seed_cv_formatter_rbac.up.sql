-- 000334_seed_cv_formatter_rbac.up.sql
-- RBAC nodes for CV Formatter (§95, ADR-103).
--   cv_formatter.use       — member+
--   cv_formatter.unlimited — premium+
--   cv_formatter.manage    — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('cv_formatter.use',       'Use the CV formatter tool (member+)'),
    ('cv_formatter.unlimited', 'Unlimited CV formatter usage (premium+)'),
    ('cv_formatter.manage',    'Manage CV standards (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'cv_formatter.use'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key IN ('cv_formatter.unlimited','cv_formatter.manage')
ON CONFLICT DO NOTHING;

COMMIT;
