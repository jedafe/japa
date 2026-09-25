-- 000566_wiki_public_settings.down.sql
BEGIN;

UPDATE system_settings
SET is_public = false
WHERE key IN (
    'wiki.min_reputation_to_edit',
    'wiki.community_edits'
);

COMMIT;
