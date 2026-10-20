-- 000911_wiki_rollup_pages_dedupe.down.sql
-- Reversal is a no-op: the dedupe only removed redundant entries from a
-- set-semantic array; the original duplicate ordering is unrecoverable
-- and undesirable anyway.
BEGIN;
SELECT 1;
COMMIT;
