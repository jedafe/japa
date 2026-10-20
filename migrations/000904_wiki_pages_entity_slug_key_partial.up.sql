-- 000904_wiki_pages_entity_slug_key_partial.up.sql
-- Fix-forward review correction (post-000899): the entity-bound page
-- unique index was created without the deleted_at partial filter that
-- wiki_pages_space_id_slug_key carries. A soft-deleted entity page row
-- therefore still occupies its (entity_type, entity_id, slug) slot and
-- blocks re-creation — diverging from the space path where deleted pages
-- release their slug (migration 000744's convention). Recreate the index
-- with the same partial predicate so deleted entity pages release slugs.

BEGIN;

DROP INDEX IF EXISTS wiki_pages_entity_slug_key;

CREATE UNIQUE INDEX wiki_pages_entity_slug_key
    ON wiki_pages (entity_type, entity_id, slug)
    WHERE entity_id IS NOT NULL AND deleted_at IS NULL;

COMMIT;
