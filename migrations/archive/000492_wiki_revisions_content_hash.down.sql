-- 000492_wiki_revisions_content_hash.down.sql
-- Reverses 000492: drops the content_hash column and its unique index.

BEGIN;

DROP INDEX IF EXISTS idx_wiki_revisions_page_hash;
ALTER TABLE wiki_revisions DROP COLUMN IF EXISTS content_hash;

COMMIT;
