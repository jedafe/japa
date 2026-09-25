-- 000400_fix_digest_rbac_admin_grant.up.sql
-- Fix BUG-004: the original 000311 seed only granted digest.manage to
-- super_admin. The §84 spec requires admin, super_admin. This migration
-- adds the missing admin role grant for existing databases.
-- Ref: japaconnect-engineering/sections/84-community-digest-widget.md

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin')
  AND p.key = 'digest.manage'
ON CONFLICT DO NOTHING;

COMMIT;
