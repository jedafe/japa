-- 000311_seed_digest_rbac.up.sql
-- RBAC nodes for Community Digest Widget (§84, ADR-092).
--   digest.manage — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('digest.manage', 'Manage community digest editions (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'digest.manage'
ON CONFLICT DO NOTHING;

COMMIT;
