-- 000379_calendar_rbac_grant_fixes.up.sql
-- Fixes for Community Calendar RBAC grants (§86, ADR-094) audit findings:
--   BUG-021: calendar.review should be granted to moderator + admin, not
--            just super_admin.
--   BUG-022: calendar.manage should be granted to admin, not just
--            super_admin.
--
-- Ref: japaconnect-engineering/sections/86-community-calendar.md

BEGIN;

-- BUG-021: Grant calendar.review to moderator and admin roles.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('moderator', 'admin')
  AND p.key = 'calendar.review'
ON CONFLICT DO NOTHING;

-- BUG-022: Grant calendar.manage to admin role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin')
  AND p.key = 'calendar.manage'
ON CONFLICT DO NOTHING;

COMMIT;
