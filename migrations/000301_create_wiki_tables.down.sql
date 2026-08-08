-- 000301_create_wiki_tables.down.sql
-- Reverses 000301_create_wiki_tables.up.sql — drops wiki_revisions then
-- wiki_pages (FK order matters).

BEGIN;

DROP TABLE IF EXISTS wiki_revisions;
DROP TABLE IF EXISTS wiki_pages;

COMMIT;
