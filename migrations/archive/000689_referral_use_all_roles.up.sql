-- 000689_referral_use_all_roles.up.sql
-- Renumbered from 000415 — it collided with 000415_recommendations_config_settings
-- (parallel branches merged with the same version prefix). The body below is the
-- de-tenanted form: user_roles.tenant_id was dropped by 000668, so the original
-- tenant filter is gone. Runs at the tail position on fresh databases; databases
-- that applied the 000415 file skip this via the 000000 registry marker.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug != 'guest'
  AND p.key = 'referral.use'
ON CONFLICT DO NOTHING;

COMMIT;
