-- 000637_video_channel_claims_source_id.down.sql

DROP INDEX IF EXISTS idx_video_channel_claims_source_id;

ALTER TABLE video_channel_claims DROP COLUMN IF EXISTS video_source_id;
