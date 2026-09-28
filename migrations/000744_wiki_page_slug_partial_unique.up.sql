-- 000744_wiki_page_slug_partial_unique.up.sql
-- Scopes wiki_pages slug uniqueness to live pages: 000742 introduced soft
-- delete (deleted_at), but the (space_id, slug) unique index still covered
-- deleted rows, so a soft-deleted page kept occupying its slug and any
-- recreate attempt failed with "A wiki page with this slug already exists."
-- A partial unique index frees the slug once the page is deleted while
-- keeping the slug reserved for as long as the row is live.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

DROP INDEX IF EXISTS wiki_pages_space_id_slug_key;

CREATE UNIQUE INDEX wiki_pages_space_id_slug_key
    ON wiki_pages (space_id, slug)
    WHERE deleted_at IS NULL;

COMMIT;
