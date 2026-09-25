-- 000689_referral_use_all_roles.down.sql
-- Renumbered from 000415 (duplicate version prefix). See the .up file.
-- 000415_referral_use_all_roles.down.sql
-- Reverts BUG-009 fix: removes referral.use grants from all non-member,
-- non-guest roles, restoring the pre-fix state (only member holds it).
-- De-tenanted form: user_roles.tenant_id was dropped by 000668, so the
-- original tenant filter is gone.
BEGIN;

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.slug NOT IN ('guest', 'member')
  AND p.key = 'referral.use';

COMMIT;
