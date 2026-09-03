-- 000608_seed_tier_defaults_and_cleanup.up.sql
-- Set users.profile_type default, seed per-tier system settings, and clean up
-- Professional Profile feature flag / dashboard menu gating.

BEGIN;

-- New users default to the regular tier (replaces old 'personal' default).
ALTER TABLE users ALTER COLUMN profile_type SET DEFAULT 'regular';

-- Seed per-tier operational limits for the default tenant.
-- These mirror the internal/control/settings_catalog.go defaults.
INSERT INTO system_settings (tenant_id, key, value)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'organizations.max_per_user.regular',      to_jsonb(1)),
    ('00000000-0000-0000-0000-000000000001', 'organizations.max_per_user.pro',          to_jsonb(5)),
    ('00000000-0000-0000-0000-000000000001', 'organizations.max_per_user.business',     to_jsonb(-1)),
    ('00000000-0000-0000-0000-000000000001', 'ai.assistant.daily_queries.regular',      to_jsonb(10)),
    ('00000000-0000-0000-0000-000000000001', 'ai.assistant.daily_queries.pro',          to_jsonb(100)),
    ('00000000-0000-0000-0000-000000000001', 'ai.assistant.daily_queries.business',     to_jsonb(-1)),
    ('00000000-0000-0000-0000-000000000001', 'content.posts_per_day.regular',           to_jsonb(5)),
    ('00000000-0000-0000-0000-000000000001', 'content.posts_per_day.pro',               to_jsonb(-1)),
    ('00000000-0000-0000-0000-000000000001', 'content.posts_per_day.business',          to_jsonb(-1)),
    ('00000000-0000-0000-0000-000000000001', 'credits.pack_discount_pct.regular',       to_jsonb(0)),
    ('00000000-0000-0000-0000-000000000001', 'credits.pack_discount_pct.pro',           to_jsonb(0)),
    ('00000000-0000-0000-0000-000000000001', 'credits.pack_discount_pct.business',      to_jsonb(10)),
    ('00000000-0000-0000-0000-000000000001', 'jce.comparisons.max_per_day.regular',     to_jsonb(5)),
    ('00000000-0000-0000-0000-000000000001', 'jce.comparisons.max_per_day.pro',         to_jsonb(25)),
    ('00000000-0000-0000-0000-000000000001', 'jce.comparisons.max_per_day.business',    to_jsonb(-1)),
    ('00000000-0000-0000-0000-000000000001', 'wallet.max_daily_earn_points.regular',    to_jsonb(100)),
    ('00000000-0000-0000-0000-000000000001', 'wallet.max_daily_earn_points.pro',        to_jsonb(500)),
    ('00000000-0000-0000-0000-000000000001', 'wallet.max_daily_earn_points.business',   to_jsonb(-1))
ON CONFLICT (tenant_id, key)
DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- Remove the Professional Profiles dashboard rail item.
DELETE FROM menu_items
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label = 'dashboard.nav.profiles';

-- Organizations are now a core feature, not gated by a Professional Profiles flag.
UPDATE menu_items
SET feature_flag = NULL
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label = 'dashboard.nav.organizations';

-- Remove the legacy Professional Profiles feature flag for the default tenant.
DELETE FROM feature_flags
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'feature.professional';

COMMIT;
