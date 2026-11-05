-- 001057_staff_tier_hardening.down.sql
-- Restores admin-group's pre-001057 non-system flag.

BEGIN;

UPDATE user_groups SET is_system = false, updated_at = now()
WHERE slug = 'admin-group';

COMMIT;
