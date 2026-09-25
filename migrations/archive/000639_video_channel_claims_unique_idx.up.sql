-- 000639_video_channel_claims_unique_idx.up.sql
-- Include video_source_id in the partial unique index so a single user can
-- file distinct pending takedown requests for different videos by the same
-- author, while still only allowing one pending ownership claim per channel.

DROP INDEX IF EXISTS idx_video_channel_claims_pending;

CREATE UNIQUE INDEX idx_video_channel_claims_pending
    ON video_channel_claims (claimant_user_id, video_author_user_id, claim_type, COALESCE(video_source_id, '00000000-0000-0000-0000-000000000000'))
    WHERE status = 'pending';
