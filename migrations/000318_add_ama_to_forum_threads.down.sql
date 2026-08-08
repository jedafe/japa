-- 000318_add_ama_to_forum_threads.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_forum_threads_ama;
ALTER TABLE forum_threads DROP COLUMN IF EXISTS ama_config;
COMMIT;
