DROP INDEX IF EXISTS idx_media_user_group;
ALTER TABLE media DROP COLUMN IF EXISTS user_group_id;
