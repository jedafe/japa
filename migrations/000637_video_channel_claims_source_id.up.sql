-- 000637_video_channel_claims_source_id.up.sql
-- Add optional video_source_id to video_channel_claims so takedown requests
-- can reference a specific source and remove it on approval.

ALTER TABLE video_channel_claims
    ADD COLUMN video_source_id uuid REFERENCES video_sources(id) ON DELETE SET NULL;

CREATE INDEX idx_video_channel_claims_source_id
    ON video_channel_claims (tenant_id, video_source_id)
    WHERE video_source_id IS NOT NULL;
