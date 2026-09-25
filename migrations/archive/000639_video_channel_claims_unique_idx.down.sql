-- 000639_video_channel_claims_unique_idx.down.sql

DROP INDEX IF EXISTS idx_video_channel_claims_pending;

CREATE UNIQUE INDEX idx_video_channel_claims_pending
    ON video_channel_claims (claimant_user_id, video_author_user_id, claim_type)
    WHERE status = 'pending';
