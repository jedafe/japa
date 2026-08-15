-- 000481_planning_group_rbac_all_tenants.up.sql
-- PG-001: The original planning group RBAC seed (000307) only granted
-- permissions in the DEFAULT tenant (00000000-0000-0000-0000-000000000001)
-- and never granted planning_group.manage_own to any role. Since the
-- PATCH/DELETE/invite routes enforce rbac.RequirePermission(...,
-- "planning_group.manage_own"), every user — including super_admin and
-- the group owner — received 403. This migration:
--   1. Grants planning_group.manage_own to member + super_admin (all
--      tenants) — ownership is enforced in-handler per §82.
--   2. Re-grants planning_group.create to member + super_admin across
--      ALL tenants (000307 only covered tenant 1).
--   3. Re-grants planning_group.manage_any to admin + super_admin
--      across ALL tenants (000307 only covered tenant 1).
--
-- Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
-- Ref: japaconnect-engineering/sections/03-rbac.md
-- Pattern: 000479_mutual_aid_rbac_all_tenants.up.sql

BEGIN;

-- planning_group.manage_own — member + super_admin in all tenants.
-- The ownership check (group.owner_id == userID) is enforced in the
-- service layer; the permission node gates access to the route.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'planning_group.manage_own'
  AND ur.slug IN ('member', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- planning_group.create — member + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'planning_group.create'
  AND ur.slug IN ('member', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- planning_group.manage_any — admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'planning_group.manage_any'
  AND ur.slug IN ('admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
