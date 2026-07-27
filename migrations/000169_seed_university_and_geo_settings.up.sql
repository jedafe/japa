-- 000169_seed_university_and_geo_settings.up.sql
-- Seeds the 7 university.* system_settings (ADR-063) and the
-- geo_personalization.enabled killswitch (ADR-064).
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md

INSERT INTO system_settings (tenant_id, key, value) VALUES
    -- Japa University (ADR-063)
    ('00000000-0000-0000-0000-000000000001', 'university.subdomain',                  '"university"'),
    ('00000000-0000-0000-0000-000000000001', 'university.custom_domain',              '""'),
    ('00000000-0000-0000-0000-000000000001', 'university.creator_percentage_default', '75'),
    ('00000000-0000-0000-0000-000000000001', 'university.platform_percentage_default', '25'),
    ('00000000-0000-0000-0000-000000000001', 'university.payout_schedule',            '"monthly"'),
    ('00000000-0000-0000-0000-000000000001', 'university.min_payout_kobo',            '500000'),
    ('00000000-0000-0000-0000-000000000001', 'university.pro_stream_rate_kobo',       '5000'),
    -- Geo personalization killswitch (ADR-064)
    ('00000000-0000-0000-0000-000000000001', 'geo_personalization.enabled',           'true')
ON CONFLICT (tenant_id, key) DO NOTHING;
