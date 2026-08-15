-- 000485_scam_alert_rbac_all_tenants.up.sql
-- BUG-001: The original scam alert RBAC seed (000305) only granted
-- scam_alert.review and scam_alert.pin to super_admin in the DEFAULT tenant
-- (00000000-0000-0000-0000-000000000001). §81 + permission-key-naming.md
-- specify:
--   scam_alert.review → moderator, admin, super_admin
--   scam_alert.pin    → admin, super_admin
-- Moderators and admins were locked out of the review/pin routes (403).
-- This migration:
--   1. Re-grants scam_alert.create to all roles across ALL tenants
--      (000305 only covered tenant 1).
--   2. Grants scam_alert.review to moderator + admin + super_admin
--      across ALL tenants.
--   3. Grants scam_alert.pin to admin + super_admin across ALL tenants.
--
-- Ref: japaconnect-engineering/sections/81-scam-alert-board.md
-- Ref: japaconnect-engineering/sections/03-rbac.md
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md
-- Pattern: 000481_planning_group_rbac_all_tenants.up.sql

BEGIN;

-- scam_alert.create — all roles in all tenants (§81: all users, rate-limited).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'scam_alert.create'
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- scam_alert.review — moderator + admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'scam_alert.review'
  AND ur.slug IN ('moderator', 'admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- scam_alert.pin — admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'scam_alert.pin'
  AND ur.slug IN ('admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
