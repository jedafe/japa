-- 000398_fix_cv_formatter_rbac_grants.up.sql
-- Fix RBAC grants for CV Formatter (§95, ADR-103).
--   cv_formatter.unlimited should be granted to premium+ (not just super_admin)
--   cv_formatter.manage should be granted to admin+ (not just super_admin)
-- Ref: conventions/permission-key-naming.md lines 244-246

BEGIN;

-- Grant cv_formatter.unlimited to premium role (spec: premium+).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'premium'
  AND p.key = 'cv_formatter.unlimited'
ON CONFLICT DO NOTHING;

-- Grant cv_formatter.manage to admin role (spec: admin+).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'admin'
  AND p.key = 'cv_formatter.manage'
ON CONFLICT DO NOTHING;

COMMIT;
