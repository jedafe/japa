-- 000752_wiki_revisions_soft_delete.up.sql
-- §80 admin revision deletion: wiki_revisions gains deleted_at so the admin
-- review queue can soft-delete spam/vandal revisions and hard-delete them
-- later (?force=true on already-deleted rows), matching wiki_pages' model.
--
-- The duplicate-content unique index is recreated partial on
-- deleted_at IS NULL so a deleted revision no longer blocks resubmitting
-- the same content (idx_contents_hash convention).
--
-- Ref: .jeos/sections/80-destination-wiki.md

BEGIN;

ALTER TABLE wiki_revisions ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

-- Duplicate-content guard scopes to live revisions only.
DROP INDEX IF EXISTS idx_wiki_revisions_page_hash;
CREATE UNIQUE INDEX idx_wiki_revisions_page_hash
    ON wiki_revisions (wiki_page_id, content_hash)
    WHERE content_hash IS NOT NULL AND deleted_at IS NULL;

COMMIT;
