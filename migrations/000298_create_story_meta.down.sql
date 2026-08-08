-- 000298_create_story_meta.down.sql
-- Reverses 000298_create_story_meta.up.sql — drops the story_meta table.
-- The contents rows with content_type='story' are left intact (soft-delete
-- model per Rule 11); only the extension table is removed.

BEGIN;

DROP TABLE IF EXISTS story_meta;

COMMIT;
