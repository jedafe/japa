DROP INDEX IF EXISTS idx_forum_threads_created_by;
ALTER TABLE forum_threads DROP COLUMN IF EXISTS created_by_user_id;
DROP INDEX IF EXISTS idx_contents_created_by;
ALTER TABLE contents DROP COLUMN IF EXISTS created_by_user_id;
