-- 000566_wiki_public_settings.up.sql
-- Marks wiki settings that the main frontend consumes as public so they
-- appear in GET /site/settings/public (Rule 3 / §80).
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

UPDATE system_settings
SET is_public = true
WHERE key IN (
    'wiki.min_reputation_to_edit',
    'wiki.community_edits'
);

COMMIT;
