-- 000549_seed_campaign_settings.up.sql
-- Section 104 (AI Content Campaigns) — default system settings.
--
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'campaign.enabled',  'true'),
    ('00000000-0000-0000-0000-000000000001', 'campaign.max_count', '50')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
