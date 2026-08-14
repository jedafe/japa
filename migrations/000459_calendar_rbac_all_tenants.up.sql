-- 000459_calendar_rbac_all_tenants.up.sql
-- BUG-010: The original calendar RBAC seed (000315) and its fix (000379)
-- only granted calendar.submit/review/manage to roles in the default tenant
-- (00000000-0000-0000-0000-000000000001). This migration replicates the
-- grants across ALL tenants so that members in non-default tenants can
-- submit calendar entries, moderators/admins can review, and admins can
-- manage.
--
-- Ref: japaconnect-engineering/sections/86-community-calendar.md
-- Ref: japaconnect-engineering/sections/03-rbac.md

BEGIN;

-- calendar.submit — member + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'calendar.submit'
  AND ur.slug IN ('member', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- calendar.review — moderator + admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'calendar.review'
  AND ur.slug IN ('moderator', 'admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- calendar.manage — admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'calendar.manage'
  AND ur.slug IN ('admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
