-- 000752_wiki_revisions_soft_delete.down.sql
-- Reverts the wiki_revisions soft-delete column and restores the
-- pre-752 duplicate-content index shape.

BEGIN;

DROP INDEX IF EXISTS idx_wiki_revisions_page_hash;
CREATE UNIQUE INDEX idx_wiki_revisions_page_hash
    ON wiki_revisions (wiki_page_id, content_hash)
    WHERE content_hash IS NOT NULL;

ALTER TABLE wiki_revisions DROP COLUMN IF EXISTS deleted_at;

COMMIT;
