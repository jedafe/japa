-- 000321_seed_achievement_rbac.up.sql
-- RBAC nodes for Community Achievement Feed (§89, ADR-097).
--   achievement.create     — member+
--   achievement.edit_own   — member+
--   achievement.delete_own — member+
--   achievement.moderate   — moderator, admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('achievement.create',     'Post an achievement (member+)'),
    ('achievement.edit_own',   'Edit own achievement (member+)'),
    ('achievement.delete_own', 'Delete own achievement (member+)'),
    ('achievement.moderate',   'Moderate achievements (moderator+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key IN ('achievement.create','achievement.edit_own','achievement.delete_own')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'achievement.moderate'
ON CONFLICT DO NOTHING;

COMMIT;
