-- 000633_video_user_columns.down.sql
ALTER TABLE user_profiles DROP COLUMN IF EXISTS banner_media_id, DROP COLUMN IF EXISTS location;
ALTER TABLE users DROP COLUMN IF EXISTS is_synthetic, DROP COLUMN IF EXISTS synthetic_source;
