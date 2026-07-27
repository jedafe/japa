-- name: CreateOrganization :one
INSERT INTO organizations (tenant_id, owner_user_id, name, slug, tagline, description, website_url, country_code, geo_space_id, org_type)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations
WHERE slug = $1 AND tenant_id = $2 AND deleted_at IS NULL;

-- name: ListOrganizations :many
SELECT o.*, u.username AS owner_username
FROM organizations o
LEFT JOIN users u ON u.id = o.owner_user_id
WHERE o.tenant_id = $1 AND o.deleted_at IS NULL
  AND ($2::text = '' OR o.org_type = $2)
  AND ($3::text = '' OR o.country_code = $3)
  AND ($4::text = '' OR o.name ILIKE '%' || $4 || '%' OR o.slug ILIKE '%' || $4 || '%')
  AND ($5::text = '' OR o.verified = ($5 = 'true'))
ORDER BY o.created_at DESC
LIMIT $6 OFFSET $7;

-- name: CountOrganizations :one
SELECT COUNT(*) FROM organizations
WHERE tenant_id = $1 AND deleted_at IS NULL
  AND ($2::text = '' OR org_type = $2)
  AND ($3::text = '' OR country_code = $3)
  AND ($4::text = '' OR name ILIKE '%' || $4 || '%' OR slug ILIKE '%' || $4 || '%')
  AND ($5::text = '' OR verified = ($5 = 'true'));

-- name: UpdateOrganization :exec
UPDATE organizations
SET name = $3, tagline = $4, description = $5, website_url = $6,
    country_code = $7, geo_space_id = $8, org_type = $9, updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteOrganization :exec
UPDATE organizations
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL;

-- name: AddOrganizationMember :exec
INSERT INTO organization_members (organization_id, user_id, role, invited_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: RemoveOrganizationMember :exec
DELETE FROM organization_members
WHERE organization_id = $1 AND user_id = $2;

-- name: ListOrganizationMembers :many
SELECT om.*, u.username, up.display_name, up.avatar_url
FROM organization_members om
JOIN users u ON u.id = om.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
WHERE om.organization_id = $1
ORDER BY om.joined_at DESC;

-- name: UpdateOrganizationMemberRole :exec
UPDATE organization_members
SET role = $3
WHERE organization_id = $1 AND user_id = $2;

-- name: CreateOrganizationInvitation :one
INSERT INTO organization_invitations (organization_id, email, role, invited_by, token_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetOrganizationInvitationByToken :one
SELECT * FROM organization_invitations
WHERE token_hash = $1 AND expires_at > now() AND accepted_at IS NULL;

-- name: ListOrganizationInvitations :many
SELECT * FROM organization_invitations
WHERE organization_id = $1 AND accepted_at IS NULL
ORDER BY created_at DESC;

-- name: AcceptOrganizationInvitation :exec
UPDATE organization_invitations
SET accepted_at = now()
WHERE id = $1 AND organization_id = $2 AND expires_at > now() AND accepted_at IS NULL;

-- name: CreateOwnershipClaim :one
INSERT INTO ownership_claims (tenant_id, claimant_user_id, organization_id, evidence_url, notes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListOwnershipClaims :many
SELECT oc.*, u.username AS claimant_username, o.name AS org_name
FROM ownership_claims oc
JOIN users u ON u.id = oc.claimant_user_id
JOIN organizations o ON o.id = oc.organization_id
WHERE oc.tenant_id = $1
  AND ($2::text = '' OR oc.status = $2)
ORDER BY oc.created_at DESC
LIMIT $3 OFFSET $4;

-- name: GetPendingOwnershipClaim :one
SELECT id FROM ownership_claims
WHERE organization_id = $1 AND tenant_id = $2 AND status = 'pending'
ORDER BY created_at DESC LIMIT 1;

-- name: GetPendingOwnershipClaims :many
SELECT oc.id, oc.organization_id, oc.claimant_user_id, oc.evidence_url, oc.notes, oc.created_at,
       u.username AS claimant_username, o.name AS org_name
FROM ownership_claims oc
JOIN users u ON u.id = oc.claimant_user_id
JOIN organizations o ON o.id = oc.organization_id
WHERE oc.tenant_id = $1 AND oc.status = 'pending'
ORDER BY oc.created_at DESC;

-- name: ApproveOwnershipClaim :exec
UPDATE ownership_claims
SET status = 'approved', reviewed_by = $3, resolution_note = $4, reviewed_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: ReviewOwnershipClaim :exec
UPDATE ownership_claims
SET status = $3, reviewed_by = $4, resolution_note = $5, reviewed_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: IsOrganizationMember :one
SELECT EXISTS(
    SELECT 1 FROM organization_members
    WHERE organization_id = $1 AND user_id = $2
) AS is_member;

-- name: GetOrganizationMemberRole :one
SELECT role FROM organization_members
WHERE organization_id = $1 AND user_id = $2;

-- name: IncrementOrganizationMemberCount :exec
UPDATE organizations
SET member_count = member_count + $2, updated_at = now()
WHERE id = $1;

-- name: ActiveOrgExistsByID :one
-- Existence check for the public org profile gate: is there an ACTIVE,
-- non-deleted organization with this id in this tenant?
SELECT EXISTS (
    SELECT 1 FROM organizations
    WHERE tenant_id = @tenant_id AND id = @id
      AND status = 'active' AND deleted_at IS NULL
) AS exists;

-- name: GetPublicOrganizationBySlug :one
-- Public org profile header: only ACTIVE, non-deleted orgs (unverified-but-
-- active is a valid public state per Section 07). Non-active/deleted → no row → 404.
SELECT * FROM organizations
WHERE slug = @slug AND tenant_id = @tenant_id
  AND status = 'active' AND deleted_at IS NULL;

-- name: ListPublicOrganizationMembers :many
-- Public Members tab: ACTIVE members only (banned/suspended/deleted users
-- excluded), tenant-scoped via the users join (defense-in-depth). Excludes the
-- internal invited_by column by projecting only public fields.
SELECT om.organization_id, om.user_id, om.role, om.joined_at,
       u.username, up.display_name, up.avatar_url
FROM organization_members om
JOIN users u ON u.id = om.user_id AND u.tenant_id = @tenant_id AND u.status = 'active'
LEFT JOIN user_profiles up ON up.user_id = u.id AND up.tenant_id = u.tenant_id
WHERE om.organization_id = @organization_id
ORDER BY om.joined_at DESC;

-- name: ListPublicOrganizations :many
-- Public Discover "Organizations": VERIFIED + ACTIVE orgs only, newest first.
SELECT id, name, slug, tagline, org_type, logo_media_id, header_media_id,
       country_code, verified, follower_count, member_count, created_at
FROM organizations
WHERE tenant_id = @tenant_id AND status = 'active' AND deleted_at IS NULL AND verified = true
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListOrganizationsByMember :many
-- Returns orgs the given user is a member of (for GET /me/organizations).
-- Includes the member's role so the dashboard can badge ownership/admin.
SELECT o.*, om.role AS member_role
FROM organizations o
JOIN organization_members om ON om.organization_id = o.id AND om.user_id = @user_id
WHERE o.tenant_id = @tenant_id AND o.deleted_at IS NULL
ORDER BY o.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPublicOrganizations :one
SELECT COUNT(*) FROM organizations
WHERE tenant_id = @tenant_id AND status = 'active' AND deleted_at IS NULL AND verified = true;

-- name: GetOrganizationAnalytics :one
-- Aggregated analytics for a single organization (Section 07, 40).
-- Returns total views (sum of content view_count), total posts, members,
-- followers, and posts created in the last 30 days.
SELECT
  COALESCE(SUM(c.view_count), 0)::bigint AS total_views,
  COUNT(c.id)::bigint AS total_posts,
  (SELECT COUNT(*)::bigint FROM organization_members om WHERE om.organization_id = @org_id) AS total_members,
  o.follower_count::bigint AS total_followers,
  COUNT(c.id) FILTER (WHERE c.created_at >= NOW() - INTERVAL '30 days')::bigint AS posts_last_30d
FROM organizations o
LEFT JOIN contents c ON c.organization_id = o.id AND c.status != 'draft' AND c.deleted_at IS NULL
WHERE o.id = @org_id AND o.tenant_id = @tenant_id AND o.deleted_at IS NULL
GROUP BY o.id, o.follower_count;

-- name: ExpireOrganizationBoosts :exec
UPDATE organization_boosts
SET status = 'ended'
WHERE status = 'active'
  AND end_date < now();
