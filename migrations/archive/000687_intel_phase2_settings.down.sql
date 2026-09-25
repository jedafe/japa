-- 000687_intel_phase2_settings.down.sql — remove the Phase-2 seed row.

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'intel.sitemap.max_children_per_crawl'
);

COMMIT;
