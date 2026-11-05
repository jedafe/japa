-- 001058_staff_tier_finite_defaults.up.sql
-- Rescope 001049 defaults: only super_admin is unlimited (-1). staff/admin
-- get finite-but-generous quotas admins can retune in the settings panel.

BEGIN;

UPDATE system_settings SET value = to_jsonb(10),   updated_at = now() WHERE key = 'organizations.max_per_user.staff';
UPDATE system_settings SET value = to_jsonb(25),   updated_at = now() WHERE key = 'organizations.max_per_user.admin';
UPDATE system_settings SET value = to_jsonb(250),  updated_at = now() WHERE key = 'ai.assistant.daily_queries.staff';
UPDATE system_settings SET value = to_jsonb(500),  updated_at = now() WHERE key = 'ai.assistant.daily_queries.admin';
UPDATE system_settings SET value = to_jsonb(50),   updated_at = now() WHERE key = 'content.posts_per_day.staff';
UPDATE system_settings SET value = to_jsonb(100),  updated_at = now() WHERE key = 'content.posts_per_day.admin';
UPDATE system_settings SET value = to_jsonb(50),   updated_at = now() WHERE key = 'jce.comparisons.max_per_day.staff';
UPDATE system_settings SET value = to_jsonb(100),  updated_at = now() WHERE key = 'jce.comparisons.max_per_day.admin';
UPDATE system_settings SET value = to_jsonb(1000), updated_at = now() WHERE key = 'wallet.max_daily_earn_points.staff';
UPDATE system_settings SET value = to_jsonb(2500), updated_at = now() WHERE key = 'wallet.max_daily_earn_points.admin';

COMMIT;
