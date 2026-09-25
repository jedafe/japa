-- 000376_grant_attestation_guide_view.up.sql
-- Fix BUG-018: attestation_guide.view is documented as "all" in §97 but
-- the original seed (000338) only inserted the permission node without
-- granting it to any role. This migration grants it to all standard roles
-- in the default tenant so anonymous/public access works correctly.
--
-- Ref: japaconnect-engineering/sections/97-attestation-guide.md

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'moderator', 'admin', 'super_admin')
  AND p.key = 'attestation_guide.view'
ON CONFLICT DO NOTHING;

COMMIT;
