package organizations

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides database access for organizations.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	pool  *pgxpool.Pool
}

// NewStore creates a Store with the given read/write queries and write pool.
func NewStore(read, write *gen.Queries, pool *pgxpool.Pool) *Store {
	return &Store{read: read, write: write, pool: pool}
}

func clampLimit(limit int32) int32 {
	if limit <= 0 || limit > MaxPageSize {
		return DefaultPageSize
	}
	return limit
}

// CreateOrganization inserts a new organization.
func (s *Store) CreateOrganization(ctx context.Context, params gen.CreateOrganizationParams) (gen.Organization, error) {
	return s.write.CreateOrganization(ctx, params)
}

// GetOrganizationByID returns an organization by ID.
func (s *Store) GetOrganizationByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.Organization, error) {
	return s.read.GetOrganizationByID(ctx, gen.GetOrganizationByIDParams{ID: id, TenantID: tenantID})
}

// GetOrganizationByIDOnly returns an organization by ID without tenant scoping.
// Used by invitation acceptance where tenant_id is not yet known.
func (s *Store) GetOrganizationByIDOnly(ctx context.Context, id pgtype.UUID) (gen.Organization, error) {
	return s.read.GetOrganizationByIDOnly(ctx, id)
}

// CountOrganizationsByOwner counts active orgs owned by a user in a tenant.
func (s *Store) CountOrganizationsByOwner(ctx context.Context, ownerID, tenantID pgtype.UUID) (int64, error) {
	return s.read.CountOrganizationsByOwner(ctx, gen.CountOrganizationsByOwnerParams{
		OwnerUserID: ownerID,
		TenantID:    tenantID,
	})
}

// GetOrganizationBySlug returns an organization by slug.
func (s *Store) GetOrganizationBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.Organization, error) {
	return s.read.GetOrganizationBySlug(ctx, gen.GetOrganizationBySlugParams{Slug: slug, TenantID: tenantID})
}

// ListOrganizations returns paginated organizations.
func (s *Store) ListOrganizations(ctx context.Context, tenantID pgtype.UUID, orgType, countryCode, search, verified string, limit, offset int32) ([]gen.ListOrganizationsRow, error) {
	return s.read.ListOrganizations(ctx, gen.ListOrganizationsParams{
		TenantID: tenantID,
		Column2:  orgType,
		Column3:  countryCode,
		Column4:  search,
		Column5:  verified,
		Limit:    clampLimit(limit),
		Offset:   offset,
	})
}

// CountOrganizations returns the total count of matching organizations.
func (s *Store) CountOrganizations(ctx context.Context, tenantID pgtype.UUID, orgType, countryCode, search, verified string) (int64, error) {
	return s.read.CountOrganizations(ctx, gen.CountOrganizationsParams{
		TenantID: tenantID,
		Column2:  orgType,
		Column3:  countryCode,
		Column4:  search,
		Column5:  verified,
	})
}

// UpdateOrganization updates an organization.
func (s *Store) UpdateOrganization(ctx context.Context, params gen.UpdateOrganizationParams) error {
	return s.write.UpdateOrganization(ctx, params)
}

// SoftDeleteOrganization soft-deletes an organization.
func (s *Store) SoftDeleteOrganization(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.write.SoftDeleteOrganization(ctx, gen.SoftDeleteOrganizationParams{ID: id, TenantID: tenantID})
}

// AddMember adds a member to an organization.
func (s *Store) AddMember(ctx context.Context, params gen.AddOrganizationMemberParams) error {
	return s.write.AddOrganizationMember(ctx, params)
}

// RemoveMember removes a member from an organization.
func (s *Store) RemoveMember(ctx context.Context, organizationID, userID pgtype.UUID) error {
	return s.write.RemoveOrganizationMember(ctx, gen.RemoveOrganizationMemberParams{
		OrganizationID: organizationID,
		UserID:         userID,
	})
}

// ListMembers returns members of an organization.
func (s *Store) ListMembers(ctx context.Context, organizationID pgtype.UUID) ([]gen.ListOrganizationMembersRow, error) {
	return s.read.ListOrganizationMembers(ctx, organizationID)
}

// UpdateMemberRole updates a member role.
func (s *Store) UpdateMemberRole(ctx context.Context, params gen.UpdateOrganizationMemberRoleParams) error {
	return s.write.UpdateOrganizationMemberRole(ctx, params)
}

// IsMember checks if a user is a member.
func (s *Store) IsMember(ctx context.Context, organizationID, userID pgtype.UUID) (bool, error) {
	return s.read.IsOrganizationMember(ctx, gen.IsOrganizationMemberParams{
		OrganizationID: organizationID,
		UserID:         userID,
	})
}

// GetMemberRole returns the role of a member.
func (s *Store) GetMemberRole(ctx context.Context, organizationID, userID pgtype.UUID) (string, error) {
	return s.read.GetOrganizationMemberRole(ctx, gen.GetOrganizationMemberRoleParams{
		OrganizationID: organizationID,
		UserID:         userID,
	})
}

// CreateInvitation creates an invitation.
func (s *Store) CreateInvitation(ctx context.Context, params gen.CreateOrganizationInvitationParams) (gen.OrganizationInvitation, error) {
	return s.write.CreateOrganizationInvitation(ctx, params)
}

// ListInvitations returns pending invitations.
func (s *Store) ListInvitations(ctx context.Context, organizationID pgtype.UUID) ([]gen.OrganizationInvitation, error) {
	return s.read.ListOrganizationInvitations(ctx, organizationID)
}

// AcceptInvitation accepts an invitation.
func (s *Store) AcceptInvitation(ctx context.Context, id, organizationID pgtype.UUID) error {
	return s.write.AcceptOrganizationInvitation(ctx, gen.AcceptOrganizationInvitationParams{
		ID:             id,
		OrganizationID: organizationID,
	})
}

// RevokeInvitation revokes (hard-deletes) a pending invitation.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
func (s *Store) RevokeInvitation(ctx context.Context, id, organizationID pgtype.UUID) error {
	return s.write.RevokeOrganizationInvitation(ctx, gen.RevokeOrganizationInvitationParams{
		ID:             id,
		OrganizationID: organizationID,
	})
}

// GetInvitationByToken returns an invitation by token hash.
func (s *Store) GetInvitationByToken(ctx context.Context, tokenHash string) (gen.OrganizationInvitation, error) {
	return s.read.GetOrganizationInvitationByToken(ctx, tokenHash)
}

// ListOrganizationsByMember returns orgs the user is a member of, with the
// member's role projected through. Used by GET /me/organizations.
func (s *Store) ListOrganizationsByMember(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.ListOrganizationsByMemberRow, error) {
	return s.read.ListOrganizationsByMember(ctx, gen.ListOrganizationsByMemberParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

// CreateOwnershipClaim creates an ownership claim.
func (s *Store) CreateOwnershipClaim(ctx context.Context, params gen.CreateOwnershipClaimParams) (gen.OwnershipClaim, error) {
	return s.write.CreateOwnershipClaim(ctx, params)
}

// ListOwnershipClaims returns paginated ownership claims.
func (s *Store) ListOwnershipClaims(ctx context.Context, tenantID pgtype.UUID, status string, limit, offset int32) ([]gen.ListOwnershipClaimsRow, error) {
	return s.read.ListOwnershipClaims(ctx, gen.ListOwnershipClaimsParams{
		TenantID: tenantID,
		Column2:  status,
		Limit:    clampLimit(limit),
		Offset:   offset,
	})
}

// ReviewOwnershipClaim reviews an ownership claim.
func (s *Store) ReviewOwnershipClaim(ctx context.Context, params gen.ReviewOwnershipClaimParams) error {
	return s.write.ReviewOwnershipClaim(ctx, params)
}

// GetPendingOwnershipClaim returns the most recent pending claim for an org.
func (s *Store) GetPendingOwnershipClaim(ctx context.Context, orgID, tenantID pgtype.UUID) (pgtype.UUID, error) {
	return s.read.GetPendingOwnershipClaim(ctx, gen.GetPendingOwnershipClaimParams{
		OrganizationID: orgID,
		TenantID:       tenantID,
	})
}

// GetPendingOwnershipClaims returns all pending ownership claims with details.
func (s *Store) GetPendingOwnershipClaims(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetPendingOwnershipClaimsRow, error) {
	return s.read.GetPendingOwnershipClaims(ctx, tenantID)
}

// GetOwnershipClaimByID returns a single ownership claim by ID.
func (s *Store) GetOwnershipClaimByID(ctx context.Context, claimID, tenantID pgtype.UUID) (gen.GetOwnershipClaimByIDRow, error) {
	return s.read.GetOwnershipClaimByID(ctx, gen.GetOwnershipClaimByIDParams{
		ID:       claimID,
		TenantID: tenantID,
	})
}

// WithdrawOwnershipClaim withdraws (deletes) a pending claim by the claimant.
func (s *Store) WithdrawOwnershipClaim(ctx context.Context, claimID, tenantID, claimantUserID pgtype.UUID) error {
	return s.write.WithdrawOwnershipClaim(ctx, gen.WithdrawOwnershipClaimParams{
		ID:              claimID,
		TenantID:        tenantID,
		ClaimantUserID:  claimantUserID,
	})
}

// ApproveOwnershipClaim approves a pending ownership claim.
func (s *Store) ApproveOwnershipClaim(ctx context.Context, claimID, tenantID, reviewedBy pgtype.UUID, note string) error {
	return s.write.ApproveOwnershipClaim(ctx, gen.ApproveOwnershipClaimParams{
		ID:             claimID,
		TenantID:       tenantID,
		ReviewedBy:     reviewedBy,
		ResolutionNote: &note,
	})
}

// IncrementMemberCount atomically increments/decrements member count.
// Tenant-scoped for defense-in-depth.
func (s *Store) IncrementMemberCount(ctx context.Context, orgID, tenantID pgtype.UUID, delta int32) error {
	return s.write.IncrementOrganizationMemberCount(ctx, gen.IncrementOrganizationMemberCountParams{
		ID:          orgID,
		MemberCount: delta,
		TenantID:    tenantID,
	})
}

// ActiveOrgExistsByID reports whether an ACTIVE, non-deleted org with this id
// exists in the tenant.
func (s *Store) ActiveOrgExistsByID(ctx context.Context, tenantID, id pgtype.UUID) (bool, error) {
	return s.read.ActiveOrgExistsByID(ctx, gen.ActiveOrgExistsByIDParams{TenantID: tenantID, ID: id})
}

func (s *Store) GetPublicOrganizationBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.Organization, error) {
	return s.read.GetPublicOrganizationBySlug(ctx, gen.GetPublicOrganizationBySlugParams{Slug: slug, TenantID: tenantID})
}

func (s *Store) ListPublicMembers(ctx context.Context, orgID, tenantID pgtype.UUID) ([]gen.ListPublicOrganizationMembersRow, error) {
	return s.read.ListPublicOrganizationMembers(ctx, gen.ListPublicOrganizationMembersParams{OrganizationID: orgID, TenantID: tenantID})
}

// GetOrgOwnerPublicProfile returns the org owner's public identity for the
// "Org Leader" sidebar widget. Unlike ListPublicMembers, this does NOT filter
// by user.status — the owner is shown regardless of their account standing.
func (s *Store) GetOrgOwnerPublicProfile(ctx context.Context, orgID, tenantID pgtype.UUID) (gen.GetOrgOwnerPublicProfileRow, error) {
	return s.read.GetOrgOwnerPublicProfile(ctx, gen.GetOrgOwnerPublicProfileParams{OrganizationID: orgID, TenantID: tenantID})
}

func (s *Store) ListPublicOrganizations(ctx context.Context, tenantID pgtype.UUID, search, orgType, verified string, limit, offset int32) ([]gen.ListPublicOrganizationsRow, error) {
	return s.read.ListPublicOrganizations(ctx, gen.ListPublicOrganizationsParams{
		TenantID:   tenantID,
		Search:     search,
		OrgType:    orgType,
		Verified:   verified,
		PageLimit:  clampLimit(limit),
		PageOffset: offset,
	})
}

func (s *Store) CountPublicOrganizations(ctx context.Context, tenantID pgtype.UUID, search, orgType, verified string) (int64, error) {
	return s.read.CountPublicOrganizations(ctx, gen.CountPublicOrganizationsParams{
		TenantID: tenantID,
		Search:   search,
		OrgType:  orgType,
		Verified: verified,
	})
}

// GetOrganizationAnalytics returns aggregated analytics for a single org.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
func (s *Store) GetOrganizationAnalytics(ctx context.Context, orgID, tenantID pgtype.UUID, periodDays int32) (gen.GetOrganizationAnalyticsRow, error) {
	return s.read.GetOrganizationAnalytics(ctx, gen.GetOrganizationAnalyticsParams{
		OrgID:      orgID,
		TenantID:   tenantID,
		PeriodDays: periodDays,
	})
}
