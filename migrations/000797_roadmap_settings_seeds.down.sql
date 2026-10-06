-- 000797_roadmap_settings_seeds.down.sql
-- Remove the seeded §113 roadmap settings. Code defaults still apply
-- (submissions on, no custom domain, 60s board TTL, 90-day shipped window).

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'roadmap.submissions_enabled',
    'roadmap.custom_domain',
    'roadmap.board_cache_ttl_seconds',
    'roadmap.shipped_window_days'
);

COMMIT;
