-- 000326_seed_visa_routes_rbac.up.sql
-- RBAC nodes for Visa Route Comparison (§90, ADR-098).
--   visa_routes.view   — all (anonymous browse)
--   visa_routes.manage — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('visa_routes.view',   'View visa route comparisons (all users)'),
    ('visa_routes.manage', 'Manage visa route data (admin+)')
ON CONFLICT (key) DO NOTHING;

-- super_admin gets manage
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'visa_routes.manage'
ON CONFLICT DO NOTHING;

COMMIT;
