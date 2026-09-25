-- 000498_ama_rbac_all_tenants.up.sql
-- BUG-001 (ama-audit): The original AMA RBAC seed (000319) only granted
-- forum.ama.create and forum.ama.moderate to super_admin in the DEFAULT tenant
-- (00000000-0000-0000-0000-000000000001). §88 + permission-key-naming.md
-- specify:
--   forum.ama.create   → professional, agent, admin, super_admin
--   forum.ama.moderate → moderator, admin, super_admin
-- Professional/agent users could not create AMAs (403); moderators could not
-- moderate AMAs (403). This migration grants the correct roles across ALL
-- tenants.
--
-- Ref: japaconnect-engineering/sections/88-ask-an-expat.md
-- Ref: japaconnect-engineering/sections/03-rbac.md
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md
-- Pattern: 000485_scam_alert_rbac_all_tenants.up.sql

BEGIN;

-- forum.ama.create — professional + agent + admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'forum.ama.create'
  AND ur.slug IN ('professional', 'agent', 'admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- forum.ama.moderate — moderator + admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'forum.ama.moderate'
  AND ur.slug IN ('moderator', 'admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
