-- 000578_jce_subspec4_settings.up.sql
-- JCE Sub-spec 4: default freshness, advisor, and refresh scheduling settings.
-- Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_stale_hours',        to_jsonb(168)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_decay_per_hour',     to_jsonb(2)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_official_weight',    to_jsonb(100)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_third_party_weight', to_jsonb(75)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_ai_estimate_weight', to_jsonb(50)),
    ('00000000-0000-0000-0000-000000000001', 'jce.freshness_cache_ttl_minutes',  to_jsonb(10)),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_model',                '""'),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_max_tokens',           to_jsonb(1024)),
    ('00000000-0000-0000-0000-000000000001', 'jce.advisor_cache_ttl_minutes',    to_jsonb(15))
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
