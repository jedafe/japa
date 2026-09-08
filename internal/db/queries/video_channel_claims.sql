-- Video channel claims: ownership claim / takedown requests.

-- name: CreateVideoChannelClaim :one
INSERT INTO video_channel_claims (
    tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status
) VALUES (
    @tenant_id, @claimant_user_id, @video_author_user_id, @claim_type,
    @evidence_url, @notes, COALESCE(@status, 'pending')
)
RETURNING
    id, tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status, reviewed_by, resolution_note, created_at, resolved_at;

-- name: GetVideoChannelClaimByID :one
SELECT
    id, tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status, reviewed_by, resolution_note, created_at, resolved_at
FROM video_channel_claims
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListVideoChannelClaimsByAuthor :many
SELECT
    id, tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status, reviewed_by, resolution_note, created_at, resolved_at
FROM video_channel_claims
WHERE tenant_id = @tenant_id AND video_author_user_id = @video_author_user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;
