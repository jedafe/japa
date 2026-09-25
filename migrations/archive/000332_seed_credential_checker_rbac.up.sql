-- 000332_seed_credential_checker_rbac.up.sql
-- RBAC nodes for Professional Credential Checker (§94, ADR-102).
--   credential_checker.view   — all
--   credential_checker.manage — admin, super_admin
--   credential_checker.note   — member+

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('credential_checker.view',   'View credential recognition pathways (all users)'),
    ('credential_checker.manage', 'Manage credential pathway data (admin+)'),
    ('credential_checker.note',   'Submit community notes on credential pathways (member+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'credential_checker.note'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'credential_checker.manage'
ON CONFLICT DO NOTHING;

COMMIT;
