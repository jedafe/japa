-- 000456_grant_attestation_guide_update_member_plus.down.sql
-- Revert BUG-010 fix: remove attestation_guide.update grants from
-- professional, agent, moderator, admin (revert to member+super_admin only).

BEGIN;

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('professional', 'agent', 'moderator', 'admin')
  AND p.key = 'attestation_guide.update';

COMMIT;
