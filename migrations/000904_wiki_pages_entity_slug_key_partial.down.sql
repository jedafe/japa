-- 000904_wiki_pages_entity_slug_key_partial.down.sql
-- Reverses 000904: restores the unfiltered entity unique index as created
-- by 000899 (without the deleted_at predicate).

BEGIN;

DROP INDEX IF EXISTS wiki_pages_entity_slug_key;

CREATE UNIQUE INDEX wiki_pages_entity_slug_key
    ON wiki_pages (entity_type, entity_id, slug)
    WHERE entity_id IS NOT NULL;

COMMIT;
