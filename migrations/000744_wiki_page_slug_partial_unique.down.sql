-- Reverses 000744_wiki_page_slug_partial_unique: restores full (space_id,
-- slug) uniqueness across deleted and live rows alike. Fails if a live and
-- a soft-deleted page currently share a slug — resolve by force-deleting
-- the stale row first (DELETE /admin/wiki/{id}?force=true).

BEGIN;

DROP INDEX IF EXISTS wiki_pages_space_id_slug_key;

CREATE UNIQUE INDEX wiki_pages_space_id_slug_key
    ON wiki_pages (space_id, slug);

COMMIT;
