-- 000484_recommendations_trending_window.up.sql
-- ADR-064a: Make the recommendation engine trending-snapshot window
-- configurable via system_settings (Rule 3 — no hardcoded windows).
--
-- Adds:
-- 1. geo_personalization.recommendations_trending_window — the trending
--    snapshot window used when sourcing content and space recommendations
--    (Section 53, ADR-064a §5). Default "7d" (7 days). Passed to
--    GetTrendingContent / GetTrendingSpaces.
--
-- Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.recommendations_trending_window', '"7d"')
ON CONFLICT (tenant_id, key) DO NOTHING;
