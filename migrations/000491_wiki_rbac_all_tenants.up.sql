-- 000491_wiki_rbac_all_tenants.up.sql
-- BUG-001: The original wiki RBAC seed (000302) only granted wiki.review,
-- wiki.manage, and wiki.lock to super_admin in the DEFAULT tenant
-- (00000000-0000-0000-0000-000000000001). §80 + permission-key-naming.md
-- specify:
--   wiki.edit    → member (reputation-gated), all higher roles
--   wiki.review  → moderator, admin, super_admin
--   wiki.manage  → admin, super_admin
--   wiki.lock    → admin, super_admin
-- Moderators and admins were locked out of the review routes (403), and
-- wiki.edit was only granted to member + super_admin (missing professional,
-- agent, moderator, admin). This migration:
--   1. Re-grants wiki.edit to all member+ roles across ALL tenants.
--   2. Grants wiki.review to moderator + admin + super_admin across ALL tenants.
--   3. Grants wiki.manage to admin + super_admin across ALL tenants.
--   4. Grants wiki.lock to admin + super_admin across ALL tenants.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
-- Ref: japaconnect-engineering/sections/03-rbac.md
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md
-- Pattern: 000485_scam_alert_rbac_all_tenants.up.sql

BEGIN;

-- wiki.edit — member + professional + agent + moderator + admin + super_admin
-- in all tenants (reputation threshold is enforced in the service layer).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'wiki.edit'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- wiki.review — moderator + admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'wiki.review'
  AND ur.slug IN ('moderator', 'admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- wiki.manage — admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'wiki.manage'
  AND ur.slug IN ('admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- wiki.lock — admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'wiki.lock'
  AND ur.slug IN ('admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
