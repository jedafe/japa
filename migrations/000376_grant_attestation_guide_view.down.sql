-- 000376_grant_attestation_guide_view.down.sql
-- Reverse: revoke attestation_guide.view from all roles.

BEGIN;

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.key = 'attestation_guide.view';

COMMIT;
