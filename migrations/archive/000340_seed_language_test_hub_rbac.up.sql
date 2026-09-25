-- 000340_seed_language_test_hub_rbac.up.sql
-- RBAC nodes for Language Test Hub (§99, ADR-107).
--   language_test_hub.view   — all
--   language_test_hub.manage — admin, super_admin
--   language_test_hub.story  — member+

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('language_test_hub.view',   'View language test resources and requirements (all users)'),
    ('language_test_hub.manage', 'Manage language test requirements (admin+)'),
    ('language_test_hub.story',  'Submit language test score stories (member+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'language_test_hub.story'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'language_test_hub.manage'
ON CONFLICT DO NOTHING;

COMMIT;
