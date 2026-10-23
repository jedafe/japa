-- 000924_media_missing_at.down.sql
DROP INDEX IF EXISTS media_missing_at_idx;
ALTER TABLE media DROP COLUMN missing_at;
