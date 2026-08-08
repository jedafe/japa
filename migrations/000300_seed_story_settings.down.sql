-- 000300_seed_story_settings.down.sql
-- Reverses 000300_seed_story_settings.up.sql — removes the three story.*
-- system_settings keys.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('story.enabled', 'story.subdomain', 'story.custom_domain');

COMMIT;
