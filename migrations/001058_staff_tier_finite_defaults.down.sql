-- 001058_staff_tier_finite_defaults.down.sql
-- Restores the 001049-seeded unlimited (-1) defaults for staff/admin keys.

BEGIN;

UPDATE system_settings SET value = to_jsonb(-1), updated_at = now() WHERE key IN (
    'organizations.max_per_user.staff',
    'organizations.max_per_user.admin',
    'ai.assistant.daily_queries.staff',
    'ai.assistant.daily_queries.admin',
    'content.posts_per_day.staff',
    'content.posts_per_day.admin',
    'jce.comparisons.max_per_day.staff',
    'jce.comparisons.max_per_day.admin',
    'wallet.max_daily_earn_points.staff',
    'wallet.max_daily_earn_points.admin'
);

COMMIT;
