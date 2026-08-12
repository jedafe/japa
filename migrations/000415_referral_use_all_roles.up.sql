-- 000415_referral_use_all_roles.up.sql
-- BUG-009 fix: referral.use is documented in §60 and permission-key-naming.md
-- as "all authenticated users", but migration 000259 only granted it to the
-- 'member' role. Users in other non-guest roles (professional, agent,
-- moderator, admin, super_admin) could not access their own referral code.
--
-- This migration grants referral.use to every non-guest role in the default
-- tenant. Idempotent (ON CONFLICT DO NOTHING). Safe to re-run.
BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug != 'guest'
  AND p.key = 'referral.use'
ON CONFLICT DO NOTHING;

COMMIT;
