-- 001057_staff_tier_hardening.up.sql
-- admin-group carries the 'admin' staff tier (001049) but was seeded
-- is_system=false — deletable via the admin UI, which would silently drop
-- the tier mapping for every member. Mark it system-protected like the
-- other system groups (members/guest/super_admin/staff).

BEGIN;

UPDATE user_groups SET is_system = true, updated_at = now()
WHERE slug = 'admin-group';

COMMIT;
