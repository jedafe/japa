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
-- Admin review queue — ?status= and ?entity_type= filters, ?q= requester/org
-- search, whitelisted CASE sort (?sort=&sort_dir=), paginated. The entity's
-- current badge flag is joined so the admin page can hide "Grant badge" on
-- rows whose entity is already verified.
SELECT vr.*, u.username AS requester_username, u.email AS requester_email,
       u.verified AS requester_verified,
       o.name AS org_name, o.slug AS org_slug, o.verified AS org_verified,
       rv.username AS reviewed_by_username
FROM verification_requests vr
JOIN users u ON u.id = vr.requester_user_id
LEFT JOIN organizations o ON o.id = vr.organization_id
LEFT JOIN users rv ON rv.id = vr.reviewed_by
WHERE ($1::text = '' OR vr.status = $1)
  AND ($2::text = '' OR vr.entity_type = $2)
  AND ($3::text = '' OR u.username ILIKE '%' || $3 || '%'
                    OR u.email ILIKE '%' || $3 || '%'
                    OR o.name ILIKE '%' || $3 || '%')
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'requester' AND sqlc.arg(sort_dir)::text = 'desc' THEN u.username END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'requester' AND sqlc.arg(sort_dir)::text <> 'desc' THEN u.username END ASC,
  CASE WHEN sqlc.arg(sort)::text = 'entity_type' AND sqlc.arg(sort_dir)::text = 'desc' THEN vr.entity_type END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'entity_type' AND sqlc.arg(sort_dir)::text <> 'desc' THEN vr.entity_type END ASC,
  CASE WHEN sqlc.arg(sort)::text = 'status' AND sqlc.arg(sort_dir)::text = 'desc' THEN vr.status END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'status' AND sqlc.arg(sort_dir)::text <> 'desc' THEN vr.status END ASC,
  CASE WHEN sqlc.arg(sort)::text = 'created_at' AND sqlc.arg(sort_dir)::text = 'asc' THEN vr.created_at END ASC,
  CASE WHEN sqlc.arg(sort)::text NOT IN ('requester', 'entity_type', 'status', 'created_at')
        OR (sqlc.arg(sort)::text = 'created_at' AND sqlc.arg(sort_dir)::text <> 'asc') THEN vr.created_at END DESC
LIMIT $4 OFFSET $5;

-- name: CountVerificationRequests :one
SELECT count(*) FROM verification_requests vr
JOIN users u ON u.id = vr.requester_user_id
LEFT JOIN organizations o ON o.id = vr.organization_id
WHERE ($1::text = '' OR vr.status = $1)
  AND ($2::text = '' OR vr.entity_type = $2)
  AND ($3::text = '' OR u.username ILIKE '%' || $3 || '%'
                    OR u.email ILIKE '%' || $3 || '%'
                    OR o.name ILIKE '%' || $3 || '%');

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

-- name: GetVerificationRequestSubmission :one
-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md (Verification Requests)
-- Admin evidence view — resolves the form submission linked to a request.
SELECT fs.id, fs.form_id, fs.user_id, fs.field_values, fs.status, fs.created_at
FROM form_submissions fs
JOIN verification_requests vr ON vr.form_submission_id = fs.id
WHERE vr.id = @id;
