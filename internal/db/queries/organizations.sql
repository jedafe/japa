-- name: CreateOrganization :one
INSERT INTO organizations (owner_user_id, name, slug, tagline, description, website_url, country_code, geo_space_id, org_type) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)

RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetOrganizationByIDOnly :one
-- Used by invitation acceptance where the id is not yet known; the org's
-- the id is read from the row and used for subsequent platform-scoped calls.
SELECT * FROM organizations
WHERE id = $1 AND deleted_at IS NULL;

-- name: CountOrganizationsByOwner :one
-- Counts active (non-deleted) organizations owned by a user in a platform.
-- Used by the organizations.max_per_user quantity gate (BUG-015).
SELECT COUNT(*) FROM organizations
WHERE owner_user_id = $1 AND deleted_at IS NULL;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations
WHERE slug = $1 AND deleted_at IS NULL;

-- name: ListOrganizations :many
-- Soft-deleted orgs are hidden by default; $7 'only'|'all' surfaces them so
-- admins can force-delete (same contract as AdminListWikiPages).
SELECT o.*, COALESCE(u.public_username, u.username, '') AS owner_username
FROM organizations o
LEFT JOIN users u ON u.id = o.owner_user_id
WHERE ($1::text = '' OR o.org_type = $1)
 AND ($2::text = '' OR o.country_code = $2)
 AND ($3::text = '' OR o.name ILIKE '%' || $3 || '%' OR o.slug ILIKE '%' || $3 || '%')
 AND ($4::text = '' OR o.verified = ($4 = 'true'))
 AND CASE $7::text
       WHEN 'only' THEN o.deleted_at IS NOT NULL
       WHEN 'all'  THEN true
       ELSE o.deleted_at IS NULL
     END
ORDER BY
 -- Whitelisted sort keys ($8) + direction ($9) — unknown keys fall back to
 -- the stable created_at DESC ordering. Same contract as ListUsers.
 CASE WHEN $8::text = 'name' AND $9::text = 'asc' THEN o.name END ASC,
 CASE WHEN $8::text = 'name' AND $9::text = 'desc' THEN o.name END DESC,
 CASE WHEN $8::text = 'slug' AND $9::text = 'asc' THEN o.slug END ASC,
 CASE WHEN $8::text = 'slug' AND $9::text = 'desc' THEN o.slug END DESC,
 CASE WHEN $8::text = 'org_type' AND $9::text = 'asc' THEN o.org_type END ASC,
 CASE WHEN $8::text = 'org_type' AND $9::text = 'desc' THEN o.org_type END DESC,
 CASE WHEN $8::text = 'owner' AND $9::text = 'asc' THEN COALESCE(u.public_username, u.username, '') END ASC,
 CASE WHEN $8::text = 'owner' AND $9::text = 'desc' THEN COALESCE(u.public_username, u.username, '') END DESC,
 CASE WHEN $8::text = 'member_count' AND $9::text = 'asc' THEN o.member_count END ASC,
 CASE WHEN $8::text = 'member_count' AND $9::text = 'desc' THEN o.member_count END DESC,
 CASE WHEN $8::text = 'follower_count' AND $9::text = 'asc' THEN o.follower_count END ASC,
 CASE WHEN $8::text = 'follower_count' AND $9::text = 'desc' THEN o.follower_count END DESC,
 CASE WHEN $8::text = 'verified' AND $9::text = 'asc' THEN o.verified END ASC,
 CASE WHEN $8::text = 'verified' AND $9::text = 'desc' THEN o.verified END DESC,
 CASE WHEN $8::text = 'created_at' AND $9::text = 'asc' THEN o.created_at END ASC,
 o.created_at DESC, o.id
LIMIT $5 OFFSET $6;

-- name: CountOrganizations :one
SELECT COUNT(*) FROM organizations
WHERE ($1::text = '' OR org_type = $1)
 AND ($2::text = '' OR country_code = $2)
 AND ($3::text = '' OR name ILIKE '%' || $3 || '%' OR slug ILIKE '%' || $3 || '%')
 AND ($4::text = '' OR verified = ($4 = 'true'))
 AND CASE $5::text
       WHEN 'only' THEN deleted_at IS NOT NULL
       WHEN 'all'  THEN true
       ELSE deleted_at IS NULL
     END;

-- name: UpdateOrganization :exec
-- Partial update: a NULL param keeps the stored value; a non-NULL param
-- writes it — including an explicit empty string, which the outer NULLIF
-- turns back into NULL so callers can clear optional fields (BUG-004/005
-- plus PATCH clear semantics for tagline/description/website_url/
-- country_code).
UPDATE organizations
SET name = COALESCE(NULLIF(@name::text, ''), name),
 tagline = NULLIF(COALESCE(@tagline, tagline), ''),
 description = NULLIF(COALESCE(@description, description), ''),
 website_url = NULLIF(COALESCE(@website_url, website_url), ''),
 country_code = NULLIF(COALESCE(@country_code, country_code), ''),
 geo_space_id = COALESCE(@geo_space_id, geo_space_id),
 org_type = COALESCE(NULLIF(@org_type::text, ''), org_type),
 updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: RestoreOrganization :exec
-- Reverse a soft delete — only acts on already-deleted rows so a restore on a
-- live org is a silent no-op (same contract as HardDeleteOrganization).
UPDATE organizations
SET deleted_at = NULL, updated_at = now()
WHERE id = $1 AND deleted_at IS NOT NULL;

-- name: SoftDeleteOrganization :exec
UPDATE organizations
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: HardDeleteOrganization :execrows
-- Permanent delete — only for already-soft-deleted organisations (admin
-- ?force=true). Members/invitations/boosts cascade; ownership claims must
-- be cleared first (NO ACTION FK), so they are guarded here.
DELETE FROM organizations o
WHERE o.id = $1 AND o.deleted_at IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM ownership_claims c WHERE c.organization_id = o.id);

-- name: OrganizationExists :one
-- Existence probe for the hard-delete path: distinguishes 404 not_found
-- from 409 not_deleted/in_use when the guarded DELETE affects no row.
SELECT EXISTS(SELECT 1 FROM organizations WHERE id = $1) AS exists;

-- name: OrganizationHasOwnershipClaims :one
-- Dependency probe for the hard-delete path (claims must be cleared first).
SELECT EXISTS(SELECT 1 FROM ownership_claims WHERE organization_id = $1) AS exists;

-- name: AddOrganizationMember :exec
INSERT INTO organization_members (organization_id, user_id, role, invited_by) VALUES ($1, $2, $3, $4)

ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: RemoveOrganizationMember :exec
DELETE FROM organization_members
WHERE organization_id = $1 AND user_id = $2;

-- name: ListOrganizationMembers :many
-- Platform-scoped via the users join (defense-in-depth, BUG-004).
SELECT om.*, COALESCE(u.public_username, u.username, '') AS username, up.display_name, up.avatar_url
FROM organization_members om
JOIN users u ON u.id = om.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE om.organization_id = @organization_id
 AND (
   sqlc.narg(viewer_id)::uuid IS NULL
   OR NOT EXISTS (
     SELECT 1 FROM user_blocks ub
     WHERE (ub.blocker_id = sqlc.narg(viewer_id)::uuid AND ub.blocked_id = om.user_id)
        OR (ub.blocker_id = om.user_id AND ub.blocked_id = sqlc.narg(viewer_id)::uuid)
   )
 )
ORDER BY om.joined_at DESC;

-- name: UpdateOrganizationSourceID :exec
-- Link an organization to its canonical intel_sources row (v2 source directory).
UPDATE organizations
SET source_id = $1, updated_at = now()
WHERE id = $2;

-- name: UpdateOrganizationMemberRole :exec
UPDATE organization_members
SET role = $3
WHERE organization_id = $1 AND user_id = $2;

-- name: CreateOrganizationInvitation :one
INSERT INTO organization_invitations (organization_id, email, role, invited_by, token_hash) VALUES ($1, $2, $3, $4, $5)

RETURNING *;

-- name: GetOrganizationInvitationByToken :one
SELECT * FROM organization_invitations
WHERE token_hash = $1 AND expires_at > now() AND accepted_at IS NULL;

-- name: ListOrganizationInvitations :many
-- Platform-scoped via the organizations join (defense-in-depth, BUG-005).
SELECT oi.* FROM organization_invitations oi
JOIN organizations o ON o.id = oi.organization_id
WHERE oi.organization_id = $1 AND oi.accepted_at IS NULL
ORDER BY oi.created_at DESC;

-- name: GetPendingInvitationByEmail :one
-- Checks if a pending (un-accepted) invitation already exists for the given
-- email+org (BUG-017). Returns pgx.ErrNoRows if none exists.
SELECT id FROM organization_invitations
WHERE organization_id = $1 AND email = $2 AND accepted_at IS NULL
 AND (
   sqlc.narg(viewer_id)::uuid IS NULL
   OR NOT EXISTS (
     SELECT 1 FROM user_blocks ub
     WHERE (ub.blocker_id = sqlc.narg(viewer_id)::uuid AND ub.blocked_id = om.user_id)
        OR (ub.blocker_id = om.user_id AND ub.blocked_id = sqlc.narg(viewer_id)::uuid)
   )
 )
LIMIT 1;

-- name: AcceptOrganizationInvitation :exec
UPDATE organization_invitations
SET accepted_at = now()
WHERE id = $1 AND organization_id = $2 AND expires_at > now() AND accepted_at IS NULL;

-- name: RevokeOrganizationInvitation :exec
-- Revoke (hard-delete) a pending invitation. Only un-accepted invitations
-- can be revoked; an already-accepted invitation is a member relationship
-- and must be removed via DELETE /organizations/{id}/members/{userId}.
-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
DELETE FROM organization_invitations
WHERE id = $1 AND organization_id = $2 AND accepted_at IS NULL;

-- name: CreateOwnershipClaim :one
INSERT INTO ownership_claims (claimant_user_id, organization_id, evidence_url, notes, form_submission_id) VALUES ($1, $2, $3, $4, sqlc.narg(form_submission_id))

RETURNING *;

-- name: ListOwnershipClaims :many
SELECT oc.*, u.username AS claimant_username, o.name AS org_name
FROM ownership_claims oc
JOIN users u ON u.id = oc.claimant_user_id
JOIN organizations o ON o.id = oc.organization_id
WHERE ($1::text = '' OR oc.status = $1)
ORDER BY oc.created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetPendingOwnershipClaim :one
SELECT id FROM ownership_claims
WHERE organization_id = $1 AND status = 'pending'
ORDER BY created_at DESC LIMIT 1;

-- name: ListOwnershipClaimsByClaimant :many
-- Claims filed by one user (GET /me/claim-ownership). Claimants have no
-- other way to discover their claim IDs — without this, GET/DELETE
-- /claim-ownership/{id} are unreachable after the POST response is lost.
-- Joined with the org name/slug so the "My claims" surface can link back
-- to the org profile.
SELECT oc.id, oc.organization_id, oc.claimant_user_id, oc.evidence_url,
 oc.notes, oc.status, oc.reviewed_by, oc.resolution_note, oc.created_at,
 oc.reviewed_at, oc.form_submission_id,
 o.name AS org_name, o.slug AS org_slug
FROM ownership_claims oc
JOIN organizations o ON o.id = oc.organization_id
WHERE oc.claimant_user_id = $1
ORDER BY oc.created_at DESC;

-- name: GetPendingClaimByClaimant :one
-- Checks if a user already has a pending claim for an org (BUG-010).
-- Used to prevent duplicate pending claims per user per org.
SELECT id FROM ownership_claims
WHERE organization_id = $1 AND claimant_user_id = $2 AND status = 'pending'
LIMIT 1;

-- name: GetOwnershipClaimByID :one
-- Returns a single ownership claim by ID (for GET /claim-ownership/{id}).
-- Joined with the claimant's username and org name for display.
SELECT oc.*, u.username AS claimant_username, o.name AS org_name
FROM ownership_claims oc
JOIN users u ON u.id = oc.claimant_user_id
JOIN organizations o ON o.id = oc.organization_id
WHERE oc.id = $1;

-- name: WithdrawOwnershipClaim :exec
-- Withdraw (delete) a pending claim by the claimant. Only pending claims
-- can be withdrawn; reviewed claims are historical records.
DELETE FROM ownership_claims
WHERE id = $1 AND claimant_user_id = $2 AND status = 'pending';

-- name: GetPendingOwnershipClaims :many
SELECT oc.id, oc.organization_id, oc.claimant_user_id, oc.evidence_url, oc.notes, oc.created_at, oc.status,
 u.username AS claimant_username, u.email AS claimant_email,
 o.name AS org_name, o.slug AS org_slug
FROM ownership_claims oc
JOIN users u ON u.id = oc.claimant_user_id
JOIN organizations o ON o.id = oc.organization_id
WHERE oc.status = 'pending'
ORDER BY oc.created_at DESC;

-- name: ListOwnershipClaimsForAdmin :many
-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
-- Admin claims list with optional status filter. An empty status_filter or
-- 'all' returns every claim (history view); any other value filters by that
-- exact status (pending|approved|rejected|withdrawn). Used by
-- GET /admin/organizations/pending-claims?status=.
-- Includes form_submission_id, resolution_note, reviewed_by, and reviewed_at
-- so the admin can inspect the claimant's submitted form data and review history.
SELECT oc.id, oc.organization_id, oc.claimant_user_id, oc.evidence_url, oc.notes, oc.created_at, oc.status,
 oc.form_submission_id, oc.resolution_note, oc.reviewed_by, oc.reviewed_at,
 u.username AS claimant_username, u.email AS claimant_email,
 o.name AS org_name, o.slug AS org_slug,
 reviewer.username AS reviewed_by_username
FROM ownership_claims oc
JOIN users u ON u.id = oc.claimant_user_id
JOIN organizations o ON o.id = oc.organization_id
LEFT JOIN users reviewer ON reviewer.id = oc.reviewed_by
WHERE ($1::text = '' OR $1::text = 'all' OR oc.status = $1)
ORDER BY oc.created_at DESC;

-- name: ListOwnershipClaimsForAdminPaged :many
-- Paginated + searchable variant used by GET /admin/organizations/pending-claims
-- when page/limit/q params are supplied. $5 is sort direction (asc|desc) and $6
-- the sort key, whitelisted to the frontend's sortable columns.
SELECT oc.id, oc.organization_id, oc.claimant_user_id, oc.evidence_url, oc.notes, oc.created_at, oc.status,
 oc.form_submission_id, oc.resolution_note, oc.reviewed_by, oc.reviewed_at,
 u.username AS claimant_username, u.email AS claimant_email,
 o.name AS org_name, o.slug AS org_slug,
 reviewer.username AS reviewed_by_username
FROM ownership_claims oc
JOIN users u ON u.id = oc.claimant_user_id
JOIN organizations o ON o.id = oc.organization_id
LEFT JOIN users reviewer ON reviewer.id = oc.reviewed_by
WHERE ($1::text = '' OR $1::text = 'all' OR oc.status = $1)
  AND ($2::text = '' OR o.name ILIKE '%' || $2 || '%' OR u.username ILIKE '%' || $2 || '%' OR u.email ILIKE '%' || $2 || '%')
ORDER BY
  CASE WHEN $5 = 'asc' THEN CASE $6
    WHEN 'org_name' THEN o.name WHEN 'claimant_username' THEN u.username
    WHEN 'claimant_email' THEN u.email WHEN 'status' THEN oc.status END
  END ASC NULLS LAST,
  CASE WHEN $5 = 'desc' THEN CASE $6
    WHEN 'org_name' THEN o.name WHEN 'claimant_username' THEN u.username
    WHEN 'claimant_email' THEN u.email WHEN 'status' THEN oc.status END
  END DESC NULLS LAST,
  CASE WHEN $6 = 'created_at' AND $5 = 'asc' THEN oc.created_at END ASC NULLS LAST,
  oc.created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountOwnershipClaimsForAdmin :one
SELECT COUNT(*)::bigint
FROM ownership_claims oc
JOIN users u ON u.id = oc.claimant_user_id
JOIN organizations o ON o.id = oc.organization_id
WHERE ($1::text = '' OR $1::text = 'all' OR oc.status = $1)
  AND ($2::text = '' OR o.name ILIKE '%' || $2 || '%' OR u.username ILIKE '%' || $2 || '%' OR u.email ILIKE '%' || $2 || '%');

-- name: ApproveOwnershipClaim :exec
UPDATE ownership_claims
SET status = 'approved', reviewed_by = $2, resolution_note = $3, reviewed_at = now()
WHERE id = $1;

-- name: TransferOrganizationOwnership :exec
-- Transfers an organization's owner_user_id to a new owner (the claimant
-- whose ownership claim was approved). Platform-scoped for defense-in-depth.
-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
UPDATE organizations
SET owner_user_id = $2, updated_at = now()
WHERE id = $1;

-- name: ReviewOwnershipClaim :exec
UPDATE ownership_claims
SET status = $2, reviewed_by = $3, resolution_note = $4, reviewed_at = now()
WHERE id = $1;

-- name: IsOrganizationMember :one
SELECT EXISTS(
 SELECT 1 FROM organization_members
 WHERE organization_id = $1 AND user_id = $2
) AS is_member;

-- name: GetOrganizationMemberRole :one
SELECT role FROM organization_members
WHERE organization_id = $1 AND user_id = $2;

-- name: IncrementOrganizationMemberCount :exec
-- Defense-in-depth: platform-scoped to prevent cross-platform count corruption.
UPDATE organizations
SET member_count = member_count + $2, updated_at = now()
WHERE id = $1;

-- name: ActiveOrgExistsByID :one
-- Existence check for the public org profile gate: is there an ACTIVE,
-- non-deleted organization with this id in this platform?
SELECT EXISTS (
 SELECT 1 FROM organizations
 WHERE id = @id
 AND status = 'active' AND deleted_at IS NULL
) AS exists;

-- name: GetPublicOrganizationBySlug :one
-- Public org profile header: only ACTIVE, non-deleted orgs (unverified-but-
-- active is a valid public state per Section 07). Non-active/deleted → no row → 404.
SELECT * FROM organizations
WHERE slug = @slug
 AND status = 'active' AND deleted_at IS NULL;

-- name: GetOrgOwnerPublicProfile :one
-- Returns the org owner's public identity (username, display_name, avatar_url)
-- for the public org profile sidebar "Org Leader" widget. Unlike
-- ListPublicOrganizationMembers, this does NOT filter by user.status — the
-- owner is shown even if their account is restricted (they are the org's
-- leader regardless of their own account standing). Platform-scoped for
-- defense-in-depth.
SELECT u.id, COALESCE(u.public_username, u.username, '') AS username, up.display_name, up.avatar_url
FROM organizations o
JOIN users u ON u.id = o.owner_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE o.id = @organization_id AND o.deleted_at IS NULL;

-- name: ListPublicOrganizationMembers :many
-- Public Members tab: ACTIVE members only (banned/suspended/deleted users
-- excluded), platform-scoped via the users join (defense-in-depth). Excludes the
-- internal invited_by column by projecting only public fields.
-- §19/§50: roster rows honour each member's profile_visibility — guests see
-- 'public' members only; authenticated viewers additionally see 'members'
-- and 'followers' profiles they already follow. Org owners/admins always
-- see the full roster — a hidden member would be un-moderatable otherwise.
SELECT om.organization_id, om.user_id, om.role, om.joined_at,
 COALESCE(u.public_username, u.username, '') AS username, up.display_name, up.avatar_url
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.status = 'active'
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN user_privacy_settings ups ON ups.user_id = om.user_id
WHERE om.organization_id = @organization_id
 AND (
   (
     sqlc.narg(viewer_id)::uuid IS NOT NULL
     AND (EXISTS (SELECT 1 FROM organization_members om2
                  WHERE om2.organization_id = om.organization_id
                    AND om2.user_id = sqlc.narg(viewer_id)::uuid
                    AND om2.role IN ('owner', 'admin'))
          OR EXISTS (SELECT 1 FROM organizations o2
                     WHERE o2.id = om.organization_id
                       AND o2.owner_user_id = sqlc.narg(viewer_id)::uuid))
   )
   OR (
     COALESCE(ups.profile_visibility, 'public') = 'public'
     OR (sqlc.narg(viewer_id)::uuid IS NOT NULL AND COALESCE(ups.profile_visibility, 'public') = 'members')
     OR (sqlc.narg(viewer_id)::uuid IS NOT NULL AND COALESCE(ups.profile_visibility, 'public') = 'followers'
         AND (om.user_id = sqlc.narg(viewer_id)::uuid OR EXISTS (
              SELECT 1 FROM follows vf
              WHERE vf.follower_id = sqlc.narg(viewer_id)::uuid AND vf.entity_type = 'user' AND vf.entity_id = om.user_id)))
   )
 )
 AND (
   sqlc.narg(viewer_id)::uuid IS NULL
   OR NOT EXISTS (
     SELECT 1 FROM user_blocks ub
     WHERE (ub.blocker_id = sqlc.narg(viewer_id)::uuid AND ub.blocked_id = om.user_id)
        OR (ub.blocker_id = om.user_id AND ub.blocked_id = sqlc.narg(viewer_id)::uuid)
   )
 )
ORDER BY om.joined_at DESC;

-- name: ListPublicOrganizations :many
-- Public Discover "Organizations": ACTIVE orgs only. filter semantics:
--   'claimed'    → orgs with an owner (default — a real operator is behind
--                  the page; informational directory entities are never in
--                  this table, migration 001045)
--   'verified'   → badge-holders (implies claimed; verify requires an owner)
--   'unverified' → orgs without a badge
--   ''           → all active orgs
-- Search filters by name or slug. org_type filters by type (empty = all).
-- Ordered by created_at DESC (newest first). claimed is projected so the
-- frontend can distinguish claimed vs unclaimed listings.
SELECT id, name, slug, tagline, org_type, logo_media_id, header_media_id,
 country_code, verified, follower_count, member_count, created_at,
 CAST(owner_user_id IS NOT NULL AS bool) AS claimed
FROM organizations
WHERE status = 'active' AND deleted_at IS NULL
 AND (
    @filter::text = ''
    OR (@filter = 'claimed' AND owner_user_id IS NOT NULL)
    OR (@filter = 'verified' AND verified)
    OR (@filter = 'unverified' AND NOT verified)
 )
 AND (@org_type::text = '' OR org_type = @org_type)
 AND (@search::text = '' OR name ILIKE '%' || @search || '%' OR slug ILIKE '%' || @search || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListOrganizationsByMember :many
-- Returns orgs the given user is a member of (for GET /me/organizations).
-- Includes the member's role so the dashboard can badge ownership/admin.
SELECT o.*, om.role AS member_role
FROM organizations o
JOIN organization_members om ON om.organization_id = o.id AND om.user_id = @user_id
WHERE o.deleted_at IS NULL
ORDER BY o.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPublicOrganizations :one
SELECT COUNT(*) FROM organizations
WHERE status = 'active' AND deleted_at IS NULL
 AND (
    @filter::text = ''
    OR (@filter = 'claimed' AND owner_user_id IS NOT NULL)
    OR (@filter = 'verified' AND verified)
    OR (@filter = 'unverified' AND NOT verified)
 )
 AND (@org_type::text = '' OR org_type = @org_type)
 AND (@search::text = '' OR name ILIKE '%' || @search || '%' OR slug ILIKE '%' || @search || '%');

-- name: GetOrganizationAnalytics :one
-- Aggregated analytics for a single organization (Section 07, 40).
-- Returns total views (sum of content view_count), total posts, members,
-- followers, and posts created in the last @period_days days (default 30).
SELECT
 COALESCE(SUM(c.view_count), 0)::bigint AS total_views,
 COUNT(c.id)::bigint AS total_posts,
 (SELECT COUNT(*)::bigint FROM organization_members om WHERE om.organization_id = @org_id) AS total_members,
 o.follower_count::bigint AS total_followers,
 COUNT(c.id) FILTER (WHERE c.created_at >= NOW() - make_interval(days => @period_days))::bigint AS posts_last_30d
FROM organizations o
LEFT JOIN contents c ON c.organization_id = o.id AND c.status != 'draft' AND c.deleted_at IS NULL
WHERE o.id = @org_id AND o.deleted_at IS NULL
GROUP BY o.id, o.follower_count;

-- name: ExpireOrganizationBoosts :exec
UPDATE organization_boosts
SET status = 'ended'
WHERE status = 'active'
 AND end_date < now();
