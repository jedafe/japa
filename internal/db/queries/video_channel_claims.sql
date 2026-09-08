-- Video channel claims: ownership claim / takedown requests.

-- name: CreateVideoChannelClaim :one
INSERT INTO video_channel_claims (
    tenant_id, claimant_user_id, video_author_user_id, video_source_id, claim_type,
    evidence_url, notes, status
) VALUES (
    @tenant_id, @claimant_user_id, @video_author_user_id, sqlc.narg(video_source_id), @claim_type,
    @evidence_url, @notes, COALESCE(@status, 'pending')
)
RETURNING
    id, tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status, reviewed_by, resolution_note, created_at, resolved_at,
    video_source_id;

-- name: GetVideoChannelClaimByID :one
SELECT
    id, tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status, reviewed_by, resolution_note, created_at, resolved_at,
    video_source_id
FROM video_channel_claims
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListVideoChannelClaimsByAuthor :many
SELECT
    id, tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status, reviewed_by, resolution_note, created_at, resolved_at,
    video_source_id
FROM video_channel_claims
WHERE tenant_id = @tenant_id AND video_author_user_id = @video_author_user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListVideoChannelClaimsByClaimant :many
-- Claims filed by the requesting user (GET /me/video-claims).
SELECT
    id, tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status, reviewed_by, resolution_note, created_at, resolved_at,
    video_source_id
FROM video_channel_claims
WHERE tenant_id = @tenant_id AND claimant_user_id = @claimant_user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountOpenVideoChannelClaimsByAuthor :one
-- Prevent duplicate pending/approved ownership claims for a channel.
SELECT COUNT(*)::int AS count
FROM video_channel_claims
WHERE tenant_id = @tenant_id
  AND video_author_user_id = @video_author_user_id
  AND claim_type = 'claim'
  AND status IN ('pending', 'approved');

-- name: ListVideoChannelClaimsByTenant :many
-- Admin queue with optional status filter and pagination.
SELECT
    vcc.id, vcc.tenant_id, vcc.claimant_user_id, vcc.video_author_user_id, vcc.claim_type,
    vcc.evidence_url, vcc.notes, vcc.status, vcc.reviewed_by, vcc.resolution_note, vcc.created_at, vcc.resolved_at,
    vcc.video_source_id,
    cu.username AS claimant_username,
    au.username AS author_username
FROM video_channel_claims vcc
JOIN users cu ON cu.id = vcc.claimant_user_id AND cu.tenant_id = vcc.tenant_id
JOIN users au ON au.id = vcc.video_author_user_id AND au.tenant_id = vcc.tenant_id
WHERE vcc.tenant_id = @tenant_id
  AND (@status::text = '' OR vcc.status = @status)
ORDER BY vcc.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountVideoChannelClaimsByTenant :one
-- Total for admin queue pagination (same filter as ListVideoChannelClaimsByTenant).
SELECT COUNT(*)::int AS total
FROM video_channel_claims vcc
WHERE vcc.tenant_id = @tenant_id
  AND (@status::text = '' OR vcc.status = @status);

-- name: UpdateVideoChannelClaimStatus :one
-- Records the review decision and stamps resolved_at.
UPDATE video_channel_claims
SET status = @status,
    reviewed_by = @reviewed_by,
    resolution_note = @resolution_note,
    resolved_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING
    id, tenant_id, claimant_user_id, video_author_user_id, claim_type,
    evidence_url, notes, status, reviewed_by, resolution_note, created_at, resolved_at,
    video_source_id;
