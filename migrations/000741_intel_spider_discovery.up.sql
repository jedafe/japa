-- 000741_intel_spider_discovery.up.sql
-- Seed the intel-channels Phase 4 settings: bounded same-host spider crawl
-- fan-out cap + outbound-link source discovery gates (intel-channels
-- curation spec §8). The worker falls back to the same defaults when the
-- rows are absent; the seeds exist to keep the keys admin-editable from
-- the control settings UI.
--
-- Post-de-tenant convention: rows are keyed globally on `key`.

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('intel.spider.max_children_per_crawl', to_jsonb(25)),
    ('intel.discovery.enabled', to_jsonb(true)),
    ('intel.discovery.max_trust_level', to_jsonb(2)),
    ('intel.discovery.domain_blocklist', to_jsonb('[]'::jsonb)),
    ('intel.discovery.max_per_crawl', to_jsonb(10))
ON CONFLICT (key) DO NOTHING;

COMMIT;
