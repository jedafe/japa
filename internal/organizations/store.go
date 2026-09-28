package organizations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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
func (s *Store) GetOrganizationByID(ctx context.Context, id pgtype.UUID) (gen.Organization, error) {
	return s.read.GetOrganizationByID(ctx, id)
}

// GetOrganizationByIDOnly returns an organization by ID without platform scoping.
// Used by invitation acceptance where is not yet known.
func (s *Store) GetOrganizationByIDOnly(ctx context.Context, id pgtype.UUID) (gen.Organization, error) {
	return s.read.GetOrganizationByIDOnly(ctx, id)
}

// CountOrganizationsByOwner counts active orgs owned by a user in a platform.
func (s *Store) CountOrganizationsByOwner(ctx context.Context, ownerID pgtype.UUID) (int64, error) {
	return s.read.CountOrganizationsByOwner(ctx,
		ownerID,
	)
}

// GetOrganizationBySlug returns an organization by slug.
func (s *Store) GetOrganizationBySlug(ctx context.Context, slug string) (gen.Organization, error) {
	return s.read.GetOrganizationBySlug(ctx, slug)
}

// ListOrganizations returns paginated organizations. deleted is '' (live
// only), 'only' (soft-deleted), or 'all'.
func (s *Store) ListOrganizations(ctx context.Context, orgType, countryCode, search, verified, deleted string, limit, offset int32) ([]gen.ListOrganizationsRow, error) {
	return s.read.ListOrganizations(ctx, gen.ListOrganizationsParams{Column1: orgType, Column2: countryCode, Column3: search, Column4: verified, Limit: clampLimit(limit),
		Offset:  offset,
		Column7: deleted,
	})
}

// CountOrganizations returns the total count of matching organizations.
func (s *Store) CountOrganizations(ctx context.Context, orgType, countryCode, search, verified, deleted string) (int64, error) {
	return s.read.CountOrganizations(ctx, gen.CountOrganizationsParams{Column1: orgType, Column2: countryCode, Column3: search, Column4: verified, Column5: deleted})
}

// UpdateOrganization updates an organization.
func (s *Store) UpdateOrganization(ctx context.Context, params gen.UpdateOrganizationParams) error {
	return s.write.UpdateOrganization(ctx, params)
}

// SoftDeleteOrganization soft-deletes an organization.
func (s *Store) SoftDeleteOrganization(ctx context.Context, id pgtype.UUID) error {
	return s.write.SoftDeleteOrganization(ctx, id)
}

// HardDeleteOrganization permanently removes a soft-deleted organization.
// Members, invitations, and boosts cascade; ownership claims must be cleared
// first (ErrInUse). Returns pgx.ErrNoRows when missing and
// errpkg.ErrNotDeleted while live.
func (s *Store) HardDeleteOrganization(ctx context.Context, id pgtype.UUID) error {
	affected, err := s.write.HardDeleteOrganization(ctx, id)
	if err != nil {
		return fmt.Errorf("hard deleting organization: %w", err)
	}

	if affected == 0 {
		exists, err := s.read.OrganizationExists(ctx, id)
		if err != nil {
			return fmt.Errorf("check organization exists: %w", err)
		}
		if !exists {
			return pgx.ErrNoRows
		}
		hasClaims, err := s.read.OrganizationHasOwnershipClaims(ctx, id)
		if err != nil {
			return fmt.Errorf("check organization claims: %w", err)
		}
		if hasClaims {
			return errpkg.ErrInUse
		}
		return errpkg.ErrNotDeleted
	}

	return nil
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

// ListMembers returns members of an organization. Platform-scoped for
// defense-in-depth (BUG-004).
func (s *Store) ListMembers(ctx context.Context, organizationID pgtype.UUID) ([]gen.ListOrganizationMembersRow, error) {
	return s.read.ListOrganizationMembers(ctx,
		organizationID,
	)
}

// UpdateMemberRole updates a member role.
func (s *Store) UpdateMemberRole(ctx context.Context, params gen.UpdateOrganizationMemberRoleParams) error {
	return s.write.UpdateOrganizationMemberRole(ctx, params)
}

// UpdateSourceID links an organization to its canonical intel_sources row.
func (s *Store) UpdateSourceID(ctx context.Context, orgID, sourceID pgtype.UUID) error {
	return s.write.UpdateOrganizationSourceID(ctx, gen.UpdateOrganizationSourceIDParams{

		ID:       orgID,
		SourceID: sourceID,
	})
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

// ListInvitations returns pending invitations. Platform-scoped for
// defense-in-depth (BUG-005).
func (s *Store) ListInvitations(ctx context.Context, organizationID pgtype.UUID) ([]gen.OrganizationInvitation, error) {
	return s.read.ListOrganizationInvitations(ctx,
		organizationID,
	)
}

// AcceptInvitation accepts an invitation.
func (s *Store) AcceptInvitation(ctx context.Context, id, organizationID pgtype.UUID) error {
	return s.write.AcceptOrganizationInvitation(ctx, gen.AcceptOrganizationInvitationParams{
		ID:             id,
		OrganizationID: organizationID,
	})
}

// RevokeInvitation revokes (hard-deletes) a pending invitation.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
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
func (s *Store) ListOrganizationsByMember(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.ListOrganizationsByMemberRow, error) {
	return s.read.ListOrganizationsByMember(ctx, gen.ListOrganizationsByMemberParams{

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
func (s *Store) ListOwnershipClaims(ctx context.Context, status string, limit, offset int32) ([]gen.ListOwnershipClaimsRow, error) {
	return s.read.ListOwnershipClaims(ctx, gen.ListOwnershipClaimsParams{Column1: status, Limit: clampLimit(limit),
		Offset: offset,
	})
}

// ReviewOwnershipClaim reviews an ownership claim.
func (s *Store) ReviewOwnershipClaim(ctx context.Context, params gen.ReviewOwnershipClaimParams) error {
	return s.write.ReviewOwnershipClaim(ctx, params)
}

// GetPendingOwnershipClaim returns the most recent pending claim for an org.
func (s *Store) GetPendingOwnershipClaim(ctx context.Context, orgID pgtype.UUID) (pgtype.UUID, error) {
	return s.read.GetPendingOwnershipClaim(ctx,
		orgID,
	)
}

// GetPendingOwnershipClaims returns all pending ownership claims with details.
func (s *Store) GetPendingOwnershipClaims(ctx context.Context) ([]gen.GetPendingOwnershipClaimsRow, error) {
	return s.read.GetPendingOwnershipClaims(ctx)
}

// GetOwnershipClaimByID returns a single ownership claim by ID.
func (s *Store) GetOwnershipClaimByID(ctx context.Context, claimID pgtype.UUID) (gen.GetOwnershipClaimByIDRow, error) {
	return s.read.GetOwnershipClaimByID(ctx,
		claimID,
	)
}

// ListOwnershipClaimsByClaimant returns all claims filed by a user
// (GET /me/claim-ownership), newest first, joined with org name/slug.
func (s *Store) ListOwnershipClaimsByClaimant(ctx context.Context, claimantUserID pgtype.UUID) ([]gen.ListOwnershipClaimsByClaimantRow, error) {
	return s.read.ListOwnershipClaimsByClaimant(ctx,

		claimantUserID,
	)
}

// WithdrawOwnershipClaim withdraws (deletes) a pending claim by the claimant.
func (s *Store) WithdrawOwnershipClaim(ctx context.Context, claimID, claimantUserID pgtype.UUID) error {
	return s.write.WithdrawOwnershipClaim(ctx, gen.WithdrawOwnershipClaimParams{
		ID: claimID,

		ClaimantUserID: claimantUserID,
	})
}

// ApproveOwnershipClaim approves a pending ownership claim.
func (s *Store) ApproveOwnershipClaim(ctx context.Context, claimID, reviewedBy pgtype.UUID, note string) error {
	return s.write.ApproveOwnershipClaim(ctx, gen.ApproveOwnershipClaimParams{
		ID: claimID,

		ReviewedBy:     reviewedBy,
		ResolutionNote: &note,
	})
}

// IncrementMemberCount atomically increments/decrements member count.
// Platform-scoped for defense-in-depth.
func (s *Store) IncrementMemberCount(ctx context.Context, orgID pgtype.UUID, delta int32) error {
	return s.write.IncrementOrganizationMemberCount(ctx, gen.IncrementOrganizationMemberCountParams{
		ID:          orgID,
		MemberCount: delta,
	})
}

// ActiveOrgExistsByID reports whether an ACTIVE, non-deleted org with this id
// exists in the platform.
func (s *Store) ActiveOrgExistsByID(ctx context.Context, id pgtype.UUID) (bool, error) {
	return s.read.ActiveOrgExistsByID(ctx, id)
}

// GetPublicOrganizationBySlug is an exported method.
func (s *Store) GetPublicOrganizationBySlug(ctx context.Context, slug string) (gen.Organization, error) {
	return s.read.GetPublicOrganizationBySlug(ctx, slug)
}

// ListPublicMembers is an exported method.
func (s *Store) ListPublicMembers(ctx context.Context, orgID pgtype.UUID) ([]gen.ListPublicOrganizationMembersRow, error) {
	return s.read.ListPublicOrganizationMembers(ctx, orgID)
}

// GetOrgOwnerPublicProfile returns the org owner's public identity for the
// "Org Leader" sidebar widget. Unlike ListPublicMembers, this does NOT filter
// by user.status — the owner is shown regardless of their account standing.
func (s *Store) GetOrgOwnerPublicProfile(ctx context.Context, orgID pgtype.UUID) (gen.GetOrgOwnerPublicProfileRow, error) {
	return s.read.GetOrgOwnerPublicProfile(ctx, orgID)
}

// ListPublicOrganizations is an exported method.
func (s *Store) ListPublicOrganizations(ctx context.Context, search, orgType, verified string, limit, offset int32) ([]gen.ListPublicOrganizationsRow, error) {
	return s.read.ListPublicOrganizations(ctx, gen.ListPublicOrganizationsParams{

		Search:     search,
		OrgType:    orgType,
		Verified:   verified,
		PageLimit:  clampLimit(limit),
		PageOffset: offset,
	})
}

// CountPublicOrganizations is an exported method.
func (s *Store) CountPublicOrganizations(ctx context.Context, search, orgType, verified string) (int64, error) {
	return s.read.CountPublicOrganizations(ctx, gen.CountPublicOrganizationsParams{

		Search:   search,
		OrgType:  orgType,
		Verified: verified,
	})
}

// GetOrganizationAnalytics returns aggregated analytics for a single org.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Store) GetOrganizationAnalytics(ctx context.Context, orgID pgtype.UUID, periodDays int32) (gen.GetOrganizationAnalyticsRow, error) {
	return s.read.GetOrganizationAnalytics(ctx, gen.GetOrganizationAnalyticsParams{
		OrgID: orgID,

		PeriodDays: periodDays,
	})
}

// VerifyOrganization marks an organization as verified (admin action).
// Platform-scoped to prevent cross-platform verification (BUG-002).
func (s *Store) VerifyOrganization(ctx context.Context, id pgtype.UUID) error {
	return s.write.VerifyOrganization(ctx, id)
}

// UnverifyOrganization revokes an organization's verification (admin action).
// Platform-scoped and clears verification_type (BUG-002, BUG-016).
func (s *Store) UnverifyOrganization(ctx context.Context, id pgtype.UUID) error {
	return s.write.UnverifyOrganization(ctx, id)
}

// TransferOrganizationOwnership updates an org's owner_user_id to the new
// owner (the claimant whose ownership claim was approved). Platform-scoped.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Store) TransferOrganizationOwnership(ctx context.Context, orgID, newOwnerID pgtype.UUID) error {
	return s.write.TransferOrganizationOwnership(ctx, gen.TransferOrganizationOwnershipParams{
		ID:          orgID,
		OwnerUserID: newOwnerID,
	})
}

// GetUserEmail returns the email address for the given user in the given platform.
// Used by AcceptInvitation to verify the accepting user's email matches the
// invited email (BUG-003).
func (s *Store) GetUserEmail(ctx context.Context, userID pgtype.UUID) (string, error) {
	return s.read.GetUserEmail(ctx,
		userID,
	)
}

// GetPendingClaimByClaimant checks if a user already has a pending claim for
// an org. Returns pgx.ErrNoRows if no pending claim exists (BUG-010).
func (s *Store) GetPendingClaimByClaimant(ctx context.Context, orgID, claimantUserID pgtype.UUID) (pgtype.UUID, error) {
	return s.read.GetPendingClaimByClaimant(ctx, gen.GetPendingClaimByClaimantParams{
		OrganizationID: orgID,

		ClaimantUserID: claimantUserID,
	})
}

// GetFormSubmissionByID returns a form submission by ID (platform-scoped).
// Used by ClaimOwnership to verify the linked form submission has status='new'
// before creating a claim (BUG-009).
func (s *Store) GetFormSubmissionByID(ctx context.Context, submissionID pgtype.UUID) (gen.GetFormSubmissionByIDRow, error) {
	return s.read.GetFormSubmissionByID(ctx,
		submissionID,
	)
}

// GetPendingInvitationByEmail checks if a pending invitation already exists
// for the given email+org. Returns pgx.ErrNoRows if none exists (BUG-017).
func (s *Store) GetPendingInvitationByEmail(ctx context.Context, orgID pgtype.UUID, email string) (pgtype.UUID, error) {
	return s.read.GetPendingInvitationByEmail(ctx, gen.GetPendingInvitationByEmailParams{
		OrganizationID: orgID,
		Email:          email,
	})
}
