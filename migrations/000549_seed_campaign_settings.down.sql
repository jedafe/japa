-- 000549_seed_campaign_settings.down.sql

BEGIN;

DELETE FROM system_settings
WHERE key IN ('campaign.enabled', 'campaign.max_count');

COMMIT;
