-- 000474_seed_language_tests_and_fix_rbac.down.sql
-- Reverse of 000474: remove the seeded language tests and the RBAC grants
-- added by the up migration. The permission nodes themselves (seeded by
-- 000340) are not removed here — that is the responsibility of 000340's
-- down migration.

BEGIN;

-- Remove the RBAC grants added by 000474.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'language_test_hub.view')
  AND role_id IN (
      SELECT id FROM user_roles
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug IN ('member', 'professional', 'agent', 'moderator', 'admin', 'super_admin')
  );

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'language_test_hub.manage')
  AND role_id IN (
      SELECT id FROM user_roles
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug = 'admin'
  );

-- Remove the seeded language tests. Only delete the canonical 8 slugs so
-- admin-created rows (if any) are preserved.
DELETE FROM language_tests
WHERE slug IN ('ielts', 'oet', 'toefl', 'pte', 'celpip', 'goethe', 'delf', 'other');

COMMIT;
