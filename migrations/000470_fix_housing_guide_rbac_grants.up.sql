-- 000470_fix_housing_guide_rbac_grants.up.sql
-- Fix BUG-003 + BUG-009: the original seed (000336) only granted
-- housing_guide.manage to super_admin, but §96 RBAC table says
-- "admin, super_admin". It also inserted housing_guide.view without
-- any role_permissions row, despite the spec granting it to "all".
--
-- Ref: japaconnect-engineering/sections/96-housing-guide.md
--      japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

-- Grant housing_guide.manage to the admin role (was super_admin only).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin')
  AND p.key = 'housing_guide.manage'
ON CONFLICT DO NOTHING;

-- Grant housing_guide.view to all member-level roles per §96 ("all").
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin', 'super_admin')
  AND p.key = 'housing_guide.view'
ON CONFLICT DO NOTHING;

COMMIT;
