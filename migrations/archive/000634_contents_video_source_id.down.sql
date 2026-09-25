-- 000634_contents_video_source_id.down.sql
DROP INDEX IF EXISTS idx_contents_video_source;
ALTER TABLE contents DROP COLUMN IF EXISTS video_source_id;
