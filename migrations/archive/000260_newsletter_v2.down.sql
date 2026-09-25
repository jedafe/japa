-- 000260_newsletter_v2.down.sql
-- Reverses 000260_newsletter_v2.up.sql.

BEGIN;

-- Revoke newsletter.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('newsletter.subscribe', 'newsletter.manage', 'newsletter.import');

-- Remove newsletter.* permission nodes.
DELETE FROM permissions WHERE key IN ('newsletter.subscribe', 'newsletter.manage', 'newsletter.import');

-- Remove newsletter.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('newsletter.enabled', 'newsletter.digest_enabled', 'newsletter.digest_day',
              'newsletter.digest_hour_utc', 'newsletter.batch_size');

-- Drop the unsubscribe tokens table.
DROP TABLE IF EXISTS newsletter_unsubscribe_tokens;

-- Drop V2 columns from newsletter_campaigns.
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS created_by;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS click_count;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS open_count;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS recipient_count;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS scheduled_at;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS segment_id;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS segment;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS body;
ALTER TABLE newsletter_campaigns DROP COLUMN IF EXISTS preview_text;

-- Drop V2 columns from newsletter_subscribers.
ALTER TABLE newsletter_subscribers DROP COLUMN IF EXISTS source;
ALTER TABLE newsletter_subscribers DROP COLUMN IF EXISTS unsubscribed_at;
ALTER TABLE newsletter_subscribers DROP COLUMN IF EXISTS subscribed_at;
ALTER TABLE newsletter_subscribers DROP COLUMN IF EXISTS campaign_enabled;
ALTER TABLE newsletter_subscribers DROP COLUMN IF EXISTS digest_enabled;

-- Drop V2 indexes.
DROP INDEX IF EXISTS idx_newsletter_sub_user;
DROP INDEX IF EXISTS idx_newsletter_sub_status;

COMMIT;
