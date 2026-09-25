-- 000338_seed_attestation_guide_rbac.up.sql
-- RBAC nodes for Attestation Guide (§97, ADR-105).
--   attestation_guide.view   — all
--   attestation_guide.manage — admin, super_admin
--   attestation_guide.update — member+

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('attestation_guide.view',   'View attestation pathway guides (all users)'),
    ('attestation_guide.manage', 'Manage attestation pathway data (admin+)'),
    ('attestation_guide.update', 'Submit community updates on attestation pathways (member+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'attestation_guide.update'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'attestation_guide.manage'
ON CONFLICT DO NOTHING;

COMMIT;
