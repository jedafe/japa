-- 000315_seed_calendar_rbac.up.sql
-- RBAC nodes for Community Calendar (§86, ADR-094).
--   calendar.submit  — member+
--   calendar.review  — moderator, admin, super_admin
--   calendar.manage  — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('calendar.submit', 'Submit a calendar entry (member+)'),
    ('calendar.review', 'Review and approve calendar entries (moderator+)'),
    ('calendar.manage', 'Manage calendar entries (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'calendar.submit'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'calendar.review'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'calendar.manage'
ON CONFLICT DO NOTHING;

COMMIT;
