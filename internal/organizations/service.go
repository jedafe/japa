package organizations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CreateOrganizationInput is the input for creating an organization.
type CreateOrganizationInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Tagline     string `json:"tagline,omitempty"`
	Description string `json:"description,omitempty"`
	WebsiteURL  string `json:"website_url,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	OrgType     string `json:"org_type,omitempty"`
}

// UpdateOrganizationInput is the input for updating an organization.
type UpdateOrganizationInput struct {
	Name        string `json:"name,omitempty"`
	Tagline     string `json:"tagline,omitempty"`
	Description string `json:"description,omitempty"`
	WebsiteURL  string `json:"website_url,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	OrgType     string `json:"org_type,omitempty"`
}

// InviteMemberInput is the input for inviting a member.
type InviteMemberInput struct {
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

// ClaimOwnershipInput is the input for claiming ownership.
type ClaimOwnershipInput struct {
	EvidenceURL string `json:"evidence_url,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// ReviewClaimInput is the input for reviewing a claim.
type ReviewClaimInput struct {
	Status         string `json:"status"`
	ResolutionNote string `json:"resolution_note,omitempty"`
}

// CreateOrganization creates a new organization and adds the creator as owner.
func (s *Service) CreateOrganization(ctx context.Context, tenantID, userID pgtype.UUID, input CreateOrganizationInput) (gen.Organization, error) {
	if input.Name == "" {
		return gen.Organization{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return gen.Organization{}, errors.New("slug is required")
	}

	orgType := input.OrgType
	if orgType == "" {
		orgType = OrgTypeBusiness
	}

	org, err := s.store.CreateOrganization(ctx, gen.CreateOrganizationParams{
		TenantID:    tenantID,
		OwnerUserID: userID,
		Name:        input.Name,
		Slug:        input.Slug,
		Tagline:     strPtr(input.Tagline),
		Description: strPtr(input.Description),
		WebsiteUrl:  strPtr(input.WebsiteURL),
		CountryCode: strPtr(input.CountryCode),
		OrgType:     orgType,
	})
	if err != nil {
		return gen.Organization{}, fmt.Errorf("creating organization: %w", err)
	}

	// Add creator as owner member.
	if err := s.store.AddMember(ctx, gen.AddOrganizationMemberParams{
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           MemberRoleOwner,
	}); err != nil {
		return gen.Organization{}, fmt.Errorf("adding owner member: %w", err)
	}

	// Increment member count.
	if err := s.store.IncrementMemberCount(ctx, org.ID, 1); err != nil {
		return gen.Organization{}, fmt.Errorf("incrementing member count: %w", err)
	}

	return org, nil
}

// GetOrganization returns an organization by ID.
func (s *Service) GetOrganization(ctx context.Context, id, tenantID pgtype.UUID) (gen.Organization, error) {
	return s.store.GetOrganizationByID(ctx, id, tenantID)
}

// GetOrganizationBySlug returns an organization by slug.
func (s *Service) GetOrganizationBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.Organization, error) {
	return s.store.GetOrganizationBySlug(ctx, slug, tenantID)
}

// ListOrganizations returns organizations filtered by type, country, search, and verified status.
func (s *Service) ListOrganizations(ctx context.Context, tenantID pgtype.UUID, orgType, countryCode, search, verified string, limit, offset int32) ([]gen.ListOrganizationsRow, int64, error) {
	items, err := s.store.ListOrganizations(ctx, tenantID, orgType, countryCode, search, verified, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountOrganizations(ctx, tenantID, orgType, countryCode, search, verified)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// UpdateOrganization updates an organization. Only owner or admin can update.
func (s *Service) UpdateOrganization(ctx context.Context, id, tenantID, userID pgtype.UUID, input UpdateOrganizationInput) error {
	role, err := s.store.GetMemberRole(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return errors.New("insufficient permissions")
	}

	return s.store.UpdateOrganization(ctx, gen.UpdateOrganizationParams{
		ID:          id,
		TenantID:    tenantID,
		Name:        input.Name,
		Tagline:     strPtr(input.Tagline),
		Description: strPtr(input.Description),
		WebsiteUrl:  strPtr(input.WebsiteURL),
		CountryCode: strPtr(input.CountryCode),
		OrgType:     input.OrgType,
	})
}

// DeleteOrganization soft-deletes an organization. Only owner can delete.
func (s *Service) DeleteOrganization(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	role, err := s.store.GetMemberRole(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner {
		return errors.New("only the owner can delete an organization")
	}

	return s.store.SoftDeleteOrganization(ctx, id, tenantID)
}

// InviteMember invites a user to the organization. Only owner or admin can invite.
func (s *Service) InviteMember(ctx context.Context, orgID, _, invitedBy pgtype.UUID, input InviteMemberInput) (gen.OrganizationInvitation, error) {
	if input.Email == "" {
		return gen.OrganizationInvitation{}, errors.New("email is required")
	}

	role, err := s.store.GetMemberRole(ctx, orgID, invitedBy)
	if err != nil {
		return gen.OrganizationInvitation{}, fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return gen.OrganizationInvitation{}, errors.New("insufficient permissions")
	}

	invRole := input.Role
	if invRole == "" {
		invRole = InvitationRoleMember
	}

	token := uuid.New().String()
	tokenHash := hashToken(token)

	return s.store.CreateInvitation(ctx, gen.CreateOrganizationInvitationParams{
		OrganizationID: orgID,
		Email:          input.Email,
		Role:           invRole,
		InvitedBy:      invitedBy,
		TokenHash:      tokenHash,
	})
}

// ListInvitations returns pending invitations for an organization.
func (s *Service) ListInvitations(ctx context.Context, orgID, userID pgtype.UUID) ([]gen.OrganizationInvitation, error) {
	role, err := s.store.GetMemberRole(ctx, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return nil, errors.New("insufficient permissions")
	}

	return s.store.ListInvitations(ctx, orgID)
}

// AcceptInvitation accepts an invitation by token and adds the user as a member.
func (s *Service) AcceptInvitation(ctx context.Context, tokenHash string, userID pgtype.UUID) error {
	inv, err := s.store.GetInvitationByToken(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("finding invitation: %w", err)
	}

	if err := s.store.AcceptInvitation(ctx, inv.ID, inv.OrganizationID); err != nil {
		return fmt.Errorf("accepting invitation: %w", err)
	}

	if err := s.store.AddMember(ctx, gen.AddOrganizationMemberParams{
		OrganizationID: inv.OrganizationID,
		UserID:         userID,
		Role:           inv.Role,
	}); err != nil {
		return fmt.Errorf("adding member: %w", err)
	}

	if err := s.store.IncrementMemberCount(ctx, inv.OrganizationID, 1); err != nil {
		return fmt.Errorf("incrementing member count: %w", err)
	}

	return nil
}

// RemoveMember removes a member from the organization. Only owner or admin can remove.
func (s *Service) RemoveMember(ctx context.Context, orgID, removedBy, targetUserID pgtype.UUID) error {
	role, err := s.store.GetMemberRole(ctx, orgID, removedBy)
	if err != nil {
		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return errors.New("insufficient permissions")
	}

	// Cannot remove the owner.
	targetRole, err := s.store.GetMemberRole(ctx, orgID, targetUserID)
	if err != nil {
		return fmt.Errorf("checking target membership: %w", err)
	}

	if targetRole == MemberRoleOwner {
		return errors.New("cannot remove the owner")
	}

	if err := s.store.RemoveMember(ctx, orgID, targetUserID); err != nil {
		return fmt.Errorf("removing member: %w", err)
	}

	if err := s.store.IncrementMemberCount(ctx, orgID, -1); err != nil {
		return fmt.Errorf("decrementing member count: %w", err)
	}

	return nil
}

// ListMembers returns all members of an organization.
func (s *Service) ListMembers(ctx context.Context, orgID pgtype.UUID) ([]gen.ListOrganizationMembersRow, error) {
	return s.store.ListMembers(ctx, orgID)
}

// UpdateMemberRole changes a member's role. Only owner can change roles.
func (s *Service) UpdateMemberRole(ctx context.Context, orgID, updatedBy, targetUserID pgtype.UUID, newRole string) error {
	role, err := s.store.GetMemberRole(ctx, orgID, updatedBy)
	if err != nil {
		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner {
		return errors.New("only the owner can change roles")
	}

	if err := s.store.UpdateMemberRole(ctx, gen.UpdateOrganizationMemberRoleParams{
		OrganizationID: orgID,
		UserID:         targetUserID,
		Role:           newRole,
	}); err != nil {
		return err
	}
	// Note: If org membership feeds into RBAC resolution, call InvalidateUserCache here.
	slog.Info("organization: member role updated", "org_id", orgID.String(), "user_id", targetUserID.String(), "role", newRole)
	return nil
}

// ClaimOwnership creates an ownership claim for an unclaimed organization.
func (s *Service) ClaimOwnership(ctx context.Context, tenantID, userID, orgID pgtype.UUID, input ClaimOwnershipInput) (gen.OwnershipClaim, error) {
	return s.store.CreateOwnershipClaim(ctx, gen.CreateOwnershipClaimParams{
		TenantID:       tenantID,
		ClaimantUserID: userID,
		OrganizationID: orgID,
		EvidenceUrl:    strPtr(input.EvidenceURL),
		Notes:          strPtr(input.Notes),
	})
}

// ListOwnershipClaims returns claims filtered by status (admin).
func (s *Service) ListOwnershipClaims(ctx context.Context, tenantID pgtype.UUID, status string, limit, offset int32) ([]gen.ListOwnershipClaimsRow, error) {
	return s.store.ListOwnershipClaims(ctx, tenantID, status, limit, offset)
}

// GetPendingOwnershipClaims returns pending claims with details (admin).
func (s *Service) GetPendingOwnershipClaims(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetPendingOwnershipClaimsRow, error) {
	return s.store.GetPendingOwnershipClaims(ctx, tenantID)
}

// ReviewOwnershipClaim approves or rejects a claim (admin).
func (s *Service) ReviewOwnershipClaim(ctx context.Context, claimID, tenantID, reviewedBy pgtype.UUID, input ReviewClaimInput) error {
	return s.store.ReviewOwnershipClaim(ctx, gen.ReviewOwnershipClaimParams{
		ID:             claimID,
		TenantID:       tenantID,
		Status:         input.Status,
		ReviewedBy:     reviewedBy,
		ResolutionNote: strPtr(input.ResolutionNote),
	})
}

// ActiveOrgExists reports whether an ACTIVE, non-deleted org with this id
// exists in the tenant. Powers the public profile gate.
func (s *Service) ActiveOrgExists(ctx context.Context, tenantID, id pgtype.UUID) (bool, error) {
	return s.store.ActiveOrgExistsByID(ctx, tenantID, id)
}

// GetPublicOrganizationBySlug returns an active org for the public profile header.
func (s *Service) GetPublicOrganizationBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.Organization, error) {
	return s.store.GetPublicOrganizationBySlug(ctx, slug, tenantID)
}

// ListPublicMembers returns the org's ACTIVE members for the public Members tab.
func (s *Service) ListPublicMembers(ctx context.Context, orgID, tenantID pgtype.UUID) ([]gen.ListPublicOrganizationMembersRow, error) {
	return s.store.ListPublicMembers(ctx, orgID, tenantID)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// TokenHashForTest exports hashToken for testing.
func TokenHashForTest(token string) string {
	return hashToken(token)
}

// NowForTest returns the current time (for testing expiration checks).
func NowForTest() time.Time {
	return time.Now()
}
