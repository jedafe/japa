-- 000631_video_channel_claims.up.sql
-- Ref: .jeos/sections/106-video-content-and-import.md
-- Channel claim and takedown requests, with admin review queue.

CREATE TABLE video_channel_claims (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    claimant_user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_author_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    claim_type            text NOT NULL CHECK (claim_type IN ('claim','takedown')),
    evidence_url          text,
    notes                 text,
    status                text NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','approved','rejected','withdrawn')),
    reviewed_by           uuid REFERENCES users(id) ON DELETE SET NULL,
    resolution_note       text,
    created_at            timestamptz NOT NULL DEFAULT now(),
    resolved_at           timestamptz
);

CREATE UNIQUE INDEX idx_video_channel_claims_pending
    ON video_channel_claims (claimant_user_id, video_author_user_id, claim_type)
    WHERE status = 'pending';
