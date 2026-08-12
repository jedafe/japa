-- 000415_referral_use_all_roles.down.sql
-- Reverts BUG-009 fix: removes referral.use grants from all non-member,
-- non-guest roles, restoring the pre-fix state (only member holds it).
BEGIN;

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug NOT IN ('guest', 'member')
  AND p.key = 'referral.use';

COMMIT;
