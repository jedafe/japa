-- 000608_seed_tier_defaults_and_cleanup.down.sql
-- Best-effort revert of tier defaults, per-tier settings, and Professional Profile
-- feature flag cleanup.

BEGIN;

-- Revert the default profile_type so it matches the pre-unification schema.
ALTER TABLE users ALTER COLUMN profile_type SET DEFAULT 'personal';

-- Remove seeded per-tier settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'organizations.max_per_user.regular',
    'organizations.max_per_user.pro',
    'organizations.max_per_user.business',
    'ai.assistant.daily_queries.regular',
    'ai.assistant.daily_queries.pro',
    'ai.assistant.daily_queries.business',
    'content.posts_per_day.regular',
    'content.posts_per_day.pro',
    'content.posts_per_day.business',
    'credits.pack_discount_pct.regular',
    'credits.pack_discount_pct.pro',
    'credits.pack_discount_pct.business',
    'jce.comparisons.max_per_day.regular',
    'jce.comparisons.max_per_day.pro',
    'jce.comparisons.max_per_day.business',
    'wallet.max_daily_earn_points.regular',
    'wallet.max_daily_earn_points.pro',
    'wallet.max_daily_earn_points.business'
  );

-- Restore the Professional Profiles dashboard rail item.
INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status, feature_flag)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'dashboard.nav.profiles', '/dashboard/profiles', 'briefcase', 12, 'authenticated', '_self', 'active', 'feature.professional'
FROM menus m
WHERE m.slug = 'dashboard_rail' AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

-- Restore the Professional Profiles feature flag gating on Organizations.
UPDATE menu_items
SET feature_flag = 'feature.professional'
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label = 'dashboard.nav.organizations';

-- Restore the legacy Professional Profiles feature flag for the default tenant.
INSERT INTO feature_flags (key, tenant_id, enabled)
VALUES ('feature.professional', '00000000-0000-0000-0000-000000000001', true)
ON CONFLICT (key, tenant_id) DO UPDATE SET enabled = true, updated_at = now();

COMMIT;
