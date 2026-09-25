-- 000440_fix_achievement_rbac_grants.up.sql
-- Fixes for Community Achievement Feed RBAC grants (§89, ADR-097) audit findings:
--   BUG-002: achievement.moderate should be granted to moderator + admin, not
--            just super_admin.
--   BUG-003: achievement.create, achievement.edit_own, achievement.delete_own
--            should be granted to moderator and admin roles, not just member
--            and super_admin. RBAC has no role inheritance — each role needs
--            an explicit grant.
--
-- Ref: japaconnect-engineering/sections/89-achievement-feed.md
--      japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

-- BUG-003: Grant member-level nodes to moderator and admin roles.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('moderator', 'admin')
  AND p.key IN ('achievement.create', 'achievement.edit_own', 'achievement.delete_own')
ON CONFLICT DO NOTHING;

-- BUG-002: Grant achievement.moderate to moderator and admin roles.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('moderator', 'admin')
  AND p.key = 'achievement.moderate'
ON CONFLICT DO NOTHING;

COMMIT;
