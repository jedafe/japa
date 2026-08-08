-- 000303_seed_wiki_settings.down.sql
-- Reverses 000303_seed_wiki_settings.up.sql.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('wiki.enabled', 'wiki.community_edits', 'wiki.min_reputation_to_edit');

COMMIT;
