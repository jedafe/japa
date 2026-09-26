-- 000741_intel_spider_discovery.down.sql — remove the Phase-4 seed rows.

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'intel.spider.max_children_per_crawl',
    'intel.discovery.enabled',
    'intel.discovery.max_trust_level',
    'intel.discovery.domain_blocklist',
    'intel.discovery.max_per_crawl'
);

COMMIT;
