DROP INDEX IF EXISTS idx_forum_replies_created_by;
ALTER TABLE forum_replies DROP COLUMN IF EXISTS created_by_user_id;
DROP INDEX IF EXISTS idx_comments_created_by;
ALTER TABLE comments DROP COLUMN IF EXISTS created_by_user_id;
