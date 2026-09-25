-- 000479_mutual_aid_rbac_all_tenants.up.sql
-- BUG-001/002/003: The original mutual aid RBAC seed (000313) only granted
-- create_offer/create_skill_swap to member+super_admin in the DEFAULT tenant
-- (00000000-0000-0000-0000-000000000001), granted moderate to super_admin
-- only, and never granted create_need to any role. This migration:
--   1. Grants mutual_aid.create_need to member + super_admin (all tenants)
--      — the reputation gate is enforced in-handler per §85.
--   2. Grants mutual_aid.create_offer / create_skill_swap to member +
--      super_admin across ALL tenants (000313 only covered tenant 1).
--   3. Grants mutual_aid.moderate to moderator + admin + super_admin
--      (all tenants) — 000313 only granted super_admin in tenant 1.
--
-- Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
-- Ref: japaconnect-engineering/sections/03-rbac.md
-- Pattern: 000459_calendar_rbac_all_tenants.up.sql

BEGIN;

-- mutual_aid.create_need — member + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'mutual_aid.create_need'
  AND ur.slug IN ('member', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- mutual_aid.create_offer — member + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'mutual_aid.create_offer'
  AND ur.slug IN ('member', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- mutual_aid.create_skill_swap — member + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'mutual_aid.create_skill_swap'
  AND ur.slug IN ('member', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- mutual_aid.moderate — moderator + admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'mutual_aid.moderate'
  AND ur.slug IN ('moderator', 'admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
