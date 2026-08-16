-- 000505_bookmarks_entity_type_check_expand.down.sql
-- Reverses 000505: restores the original restrictive CHECK constraint
-- from 000103. WARNING: any rows inserted with the expanded entity types
-- between 000505 and this rollback will violate the restored constraint
-- and must be deleted first.

BEGIN;

ALTER TABLE bookmarks DROP CONSTRAINT IF EXISTS bookmarks_entity_type_check;

ALTER TABLE bookmarks ADD CONSTRAINT bookmarks_entity_type_check
    CHECK (entity_type IN ('content', 'thread', 'reply'));

COMMIT;
