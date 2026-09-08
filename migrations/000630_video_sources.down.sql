-- 000630_video_sources.down.sql
DROP INDEX IF EXISTS idx_video_sources_author;
DROP INDEX IF EXISTS idx_video_sources_external_id;
DROP TABLE IF EXISTS video_sources;
