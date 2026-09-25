-- 000415_recommendations_config_settings.up.sql
-- ADR-064a: Make the recommendation engine cache TTL and ?limit cap
-- configurable via system_settings (Rule 3 — no hardcoded limits/timeouts).
--
-- Adds:
-- 1. geo_personalization.recommendations_cache_ttl_seconds — TTL for the
--    jc:recommendations:* cache keys (default 900 = 15 minutes).
-- 2. geo_personalization.recommendations_max_limit — max value accepted for
--    the ?limit query param on GET /recommendations (default 20).
--
-- Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.recommendations_cache_ttl_seconds', '900')
ON CONFLICT (tenant_id, key) DO NOTHING;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.recommendations_max_limit', '20')
ON CONFLICT (tenant_id, key) DO NOTHING;
