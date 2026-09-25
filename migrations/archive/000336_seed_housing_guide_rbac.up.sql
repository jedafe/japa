-- 000336_seed_housing_guide_rbac.up.sql
-- RBAC nodes for Housing Guide (§96, ADR-104).
--   housing_guide.view   — all
--   housing_guide.manage — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('housing_guide.view',   'View housing guides (all users)'),
    ('housing_guide.manage', 'Manage housing guide content (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'housing_guide.manage'
ON CONFLICT DO NOTHING;

COMMIT;
