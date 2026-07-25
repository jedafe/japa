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

// GetInvitationByToken returns an invitation by token hash.
func (s *Store) GetInvitationByToken(ctx context.Context, tokenHash string) (gen.OrganizationInvitation, error) {
	return s.read.GetOrganizationInvitationByToken(ctx, tokenHash)
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
func (s *Store) IncrementMemberCount(ctx context.Context, orgID pgtype.UUID, delta int32) error {
	return s.write.IncrementOrganizationMemberCount(ctx, gen.IncrementOrganizationMemberCountParams{
		ID:          orgID,
		MemberCount: delta,
	})
}

// ActiveOrgExistsByID reports whether an ACTIVE, non-deleted org with this id
// exists in the tenant.
func (s *Store) ActiveOrgExistsByID(ctx context.Context, tenantID, id pgtype.UUID) (bool, error) {
	return s.read.ActiveOrgExistsByID(ctx, gen.ActiveOrgExistsByIDParams{TenantID: tenantID, ID: id})
}
