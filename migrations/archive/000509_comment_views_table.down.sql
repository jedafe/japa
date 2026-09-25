-- 000509_comment_views_table.down.sql
-- Reverses 000509: drops comment_views table, trigger, function,
-- and the comments.view_count column.

BEGIN;

DROP TRIGGER IF EXISTS trg_comment_views_after_insert ON comment_views;
DROP FUNCTION IF EXISTS trg_comment_views_increment();
DROP TABLE IF EXISTS comment_views;
ALTER TABLE comments DROP COLUMN IF EXISTS view_count;

COMMIT;
