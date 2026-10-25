-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md (Verification Requests)
-- User-initiated verification badge requests: submit (with Form Builder
-- evidence), requester tracking/withdraw, admin review queue. Badge grant is
-- a separate admin step (POST /admin/{organizations|users}/{id}/verify).

-- name: CreateVerificationRequest :one
INSERT INTO verification_requests
  (requester_user_id, entity_type, organization_id, requested_type, form_submission_id)
VALUES ($1, $2, sqlc.narg(organization_id), sqlc.narg(requested_type), $3)
RETURNING *;

-- name: GetVerificationRequestByID :one
-- Requester-scoped fetch needs only vr.*; joins provide display context.
SELECT vr.*, u.username AS requester_username, o.name AS org_name, o.slug AS org_slug
FROM verification_requests vr
JOIN users u ON u.id = vr.requester_user_id
LEFT JOIN organizations o ON o.id = vr.organization_id
WHERE vr.id = $1;

-- name: GetPendingVerificationRequestForOrg :one
SELECT id FROM verification_requests
WHERE organization_id = $1 AND status = 'pending' AND entity_type = 'organization'
LIMIT 1;

-- name: GetPendingVerificationRequestForUser :one
SELECT id FROM verification_requests
WHERE requester_user_id = $1 AND status = 'pending' AND entity_type = 'user'
LIMIT 1;

-- name: ListVerificationRequestsByRequester :many
-- Requests filed by one user (GET /me/verification-requests). Requesters have
-- no other way to discover their request IDs — same contract as
-- ListOwnershipClaimsByClaimant.
SELECT vr.id, vr.entity_type, vr.organization_id, vr.requested_type, vr.status,
       vr.resolution_note, vr.created_at, vr.reviewed_at, vr.form_submission_id,
       o.name AS org_name, o.slug AS org_slug
FROM verification_requests vr
LEFT JOIN organizations o ON o.id = vr.organization_id
WHERE vr.requester_user_id = $1
ORDER BY vr.created_at DESC;

-- name: ListVerificationRequests :many
-- Admin review queue — ?status= and ?entity_type= filters, paginated.
SELECT vr.*, u.username AS requester_username, u.email AS requester_email,
       o.name AS org_name, o.slug AS org_slug, rv.username AS reviewed_by_username
FROM verification_requests vr
JOIN users u ON u.id = vr.requester_user_id
LEFT JOIN organizations o ON o.id = vr.organization_id
LEFT JOIN users rv ON rv.id = vr.reviewed_by
WHERE ($1::text = '' OR vr.status = $1)
  AND ($2::text = '' OR vr.entity_type = $2)
ORDER BY vr.created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountVerificationRequests :one
SELECT count(*) FROM verification_requests vr
WHERE ($1::text = '' OR vr.status = $1)
  AND ($2::text = '' OR vr.entity_type = $2);

-- name: ReviewVerificationRequest :execrows
-- Only pending requests are reviewable; withdrawn/approved/rejected are
-- terminal states for this transition.
UPDATE verification_requests
SET status = $2, reviewed_by = $3, resolution_note = sqlc.narg(resolution_note), reviewed_at = now()
WHERE id = $1 AND status = 'pending';

-- name: WithdrawVerificationRequest :execrows
UPDATE verification_requests
SET status = 'withdrawn', reviewed_at = now()
WHERE id = $1 AND requester_user_id = $2 AND status = 'pending';

-- name: ReopenVerificationRequest :exec
-- §22 appeal overturn: a rejected request goes back to pending.
UPDATE verification_requests
SET status = 'pending', reviewed_by = NULL, reviewed_at = NULL
WHERE id = $1 AND status = 'rejected';
