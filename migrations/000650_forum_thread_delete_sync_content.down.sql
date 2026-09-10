-- 000650_forum_thread_delete_sync_content.down.sql
-- Revert the forum_threads delete trigger.

BEGIN;

DROP TRIGGER IF EXISTS soft_delete_forum_thread_content ON forum_threads;
DROP FUNCTION IF EXISTS trg_soft_delete_forum_thread_content();

COMMIT;
