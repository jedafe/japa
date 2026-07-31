-- 000209_user_profiles_media_ids.down.sql
-- Reverse of 000209: drop avatar_media_id and header_media_id.

DROP INDEX IF EXISTS idx_user_profiles_header_media;
DROP INDEX IF EXISTS idx_user_profiles_avatar_media;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS header_media_id;
ALTER TABLE user_profiles DROP COLUMN IF EXISTS avatar_media_id;
