package organizations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
)

// validOrgTypes is the allow-list enforced by the organizations.org_type
// CHECK constraint (migration 000105). Validating here surfaces a 400
// invalid_org_type envelope instead of a raw 500 from the DB constraint.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
var validOrgTypes = map[string]bool{
	OrgTypeBusiness: true, OrgTypeNGO: true, OrgTypeGovernment: true,
	OrgTypeEmbassy: true, OrgTypeUniversity: true, OrgTypeMedia: true,
	OrgTypeCommunity: true,
}

// validateOrgType returns errpkg.ErrInvalid wrapped with a descriptive
// message when orgType is non-empty and not in the allow-list.
func validateOrgType(orgType string) error {
	if orgType == "" {
		return nil
	}

	if !validOrgTypes[orgType] {
		return fmt.Errorf("%w: invalid org_type %q", errpkg.ErrInvalid, orgType)
	}

	return nil
}

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
	EvidenceURL      string       `json:"evidence_url,omitempty"`
	Notes            string       `json:"notes,omitempty"`
	FormSubmissionID *pgtype.UUID `json:"form_submission_id,omitempty"`
}

// ReviewClaimInput is the input for reviewing a claim.
type ReviewClaimInput struct {
	Status         string `json:"status"`
	ResolutionNote string `json:"resolution_note,omitempty"`
}

func (s *Service) sourceAutoCreateEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return false
	}
	enabled, err := s.settings.GetSettingBool(ctx, "sources.auto_create_for_organizations")
	if err != nil {
		return false
	}
	return enabled
}

// CreateOrganization creates a new organization and adds the creator as owner.
// The org insert, owner member insert, and member-count increment are wrapped
// in a single transaction so a partial failure cannot leave an orphaned org
// (BUG-007).
func (s *Service) CreateOrganization(ctx context.Context, userID pgtype.UUID, input CreateOrganizationInput) (gen.Organization, error) {
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

	if err := validateOrgType(orgType); err != nil {
		return gen.Organization{}, err
	}

	// Quantity gate: per-tier organizations.max_per_user.{tier} (0 = disabled,
	// -1 = unlimited, N = cap). Falls back to the legacy
	// organizations.max_per_user key for backward compatibility. If neither is
	// configured, the default is unlimited so unseeded tenants are not blocked.
	// Ref: §unified-tiers, japaconnect-engineering/conventions/system-settings-catalog.md
	if s.settings != nil {
		tier := "regular"

		if s.tierResolver != nil {
			resolved, err := s.tierResolver.ResolveUserTier(ctx, userID)
			if err == nil && resolved != "" {
				tier = resolved
			}
		}

		maxPerUser := -1
		perTierKey := "organizations.max_per_user." + tier

		if v, err := s.settings.GetSettingInt(ctx, perTierKey); err == nil {
			maxPerUser = v
		} else if v, err := s.settings.GetSettingInt(ctx, "organizations.max_per_user"); err == nil {
			maxPerUser = v
		}

		if maxPerUser == 0 {
			return gen.Organization{}, fmt.Errorf("%w: organization creation is disabled", errpkg.ErrForbidden)
		}

		if maxPerUser > 0 {
			count, err := s.store.CountOrganizationsByOwner(ctx, userID)
			if err != nil {
				return gen.Organization{}, fmt.Errorf("checking org count: %w", err)
			}

			if count >= int64(maxPerUser) {
				return gen.Organization{}, fmt.Errorf("%w: maximum organizations reached (%d)", errpkg.ErrForbidden, maxPerUser)
			}
		}
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return gen.Organization{}, fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	org, err := qtx.CreateOrganization(ctx, gen.CreateOrganizationParams{

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
		// BUG-014: Map unique constraint violation (slug) to 409 already_exists.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.Organization{}, fmt.Errorf("%w: slug already taken", errpkg.ErrDuplicate)
		}

		return gen.Organization{}, fmt.Errorf("creating organization: %w", err)
	}

	// Add creator as owner member.
	if err := qtx.AddOrganizationMember(ctx, gen.AddOrganizationMemberParams{
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           MemberRoleOwner,
	}); err != nil {
		return gen.Organization{}, fmt.Errorf("adding owner member: %w", err)
	}

	// Increment member count.
	if err := qtx.IncrementOrganizationMemberCount(ctx, gen.IncrementOrganizationMemberCountParams{
		ID:          org.ID,
		MemberCount: 1,
	}); err != nil {
		return gen.Organization{}, fmt.Errorf("incrementing member count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return gen.Organization{}, fmt.Errorf("committing transaction: %w", err)
	}

	s.InvalidateOrgCache(ctx)

	// v2 source directory: auto-create an intel_sources row for the organization
	// when the tenant setting is enabled (default false).
	if s.sources != nil && s.sourceAutoCreateEnabled(ctx) {
		category := "community_organisation"
		if org.OrgType == "university" {
			category = "university"
		} else if org.OrgType == "business" || org.OrgType == "media" {
			category = org.OrgType
		}
		source, sErr := s.sources.CreateSourceFromEntity(ctx, org.ID, userID, "organization", org.Name, org.Slug, maybeString(org.WebsiteUrl), category)
		if sErr != nil {
			slog.Warn("organizations: failed to create source for org", "org_id", org.ID, "error", sErr)
		} else {
			if err := s.store.UpdateSourceID(ctx, org.ID, source.ID); err != nil {
				slog.Warn("organizations: failed to link org source_id", "org_id", org.ID, "source_id", source.ID, "error", err)
			}
		}
	}

	// ADR-064a: The org's country_code is sourced from the spaces country
	// table, so it is a geo-aware signal. Feed it into the user's
	// destination_country custom field so the geo-aware personalization
	// system picks it up. Non-fatal — org creation succeeds regardless.
	if input.CountryCode != "" {
		if err := s.setDestinationCountry(ctx, userID, input.CountryCode); err != nil {
			slog.Warn("organizations: failed to set destination_country from org creation", "error", err)
		}
	}

	return org, nil
}

// GetOrganization returns an organization by ID.
func (s *Service) GetOrganization(ctx context.Context, id pgtype.UUID) (gen.Organization, error) {
	return s.store.GetOrganizationByID(ctx, id)
}

// IsMember reports whether the given user is a member of the organization.
func (s *Service) IsMember(ctx context.Context, orgID, userID pgtype.UUID) (bool, error) {
	return s.store.IsMember(ctx, orgID, userID)
}

// MyOrganization is one row of GET /me/organizations — the org + the caller's
// role in it. The role lets the dashboard badge owner/admin/member.
type MyOrganization struct {
	gen.Organization
	MemberRole string `json:"member_role"`
}

// ListMyOrganizations returns orgs the user is a member of, with the member's
// role projected through. Used by GET /me/organizations.
func (s *Service) ListMyOrganizations(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]MyOrganization, error) {
	rows, err := s.store.ListOrganizationsByMember(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing organizations by member: %w", err)
	}

	result := make([]MyOrganization, len(rows))

	for i, r := range rows {
		org := gen.Organization{
			ID: r.ID, OwnerUserID: r.OwnerUserID,
			Name: r.Name, Slug: r.Slug, Tagline: r.Tagline, Description: r.Description,
			LogoMediaID: r.LogoMediaID, HeaderMediaID: r.HeaderMediaID,
			WebsiteUrl: r.WebsiteUrl, CountryCode: r.CountryCode, GeoSpaceID: r.GeoSpaceID,
			OrgType: r.OrgType, Verified: r.Verified, VerificationType: r.VerificationType,
			FollowerCount: r.FollowerCount, MemberCount: r.MemberCount, Status: r.Status,
			FieldValues: r.FieldValues, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			DeletedAt: r.DeletedAt,
		}
		result[i] = MyOrganization{Organization: org, MemberRole: r.MemberRole}
	}

	return result, nil
}

// GetOrganizationBySlug returns an organization by slug.
func (s *Service) GetOrganizationBySlug(ctx context.Context, slug string) (gen.Organization, error) {
	return s.store.GetOrganizationBySlug(ctx, slug)
}

// ListOrganizations returns organizations filtered by type, country, search, and verified status.
func (s *Service) ListOrganizations(ctx context.Context, orgType, countryCode, search, verified string, limit, offset int32) ([]gen.ListOrganizationsRow, int64, error) {
	items, err := s.store.ListOrganizations(ctx, orgType, countryCode, search, verified, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountOrganizations(ctx, orgType, countryCode, search, verified)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// UpdateOrganization updates an organization. Only owner or admin can update.
func (s *Service) UpdateOrganization(ctx context.Context, id, userID pgtype.UUID, input UpdateOrganizationInput) error {
	if err := validateOrgType(input.OrgType); err != nil {
		return err
	}

	role, err := s.store.GetMemberRole(ctx, id, userID)
	if err != nil {
		// BUG-021: A non-member (ErrNoRows) should get 403, not 404.
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
		}

		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
	}

	if err := s.store.UpdateOrganization(ctx, gen.UpdateOrganizationParams{
		ID: id,

		Name:        input.Name,
		Tagline:     strPtr(input.Tagline),
		Description: strPtr(input.Description),
		WebsiteUrl:  strPtr(input.WebsiteURL),
		CountryCode: strPtr(input.CountryCode),
		OrgType:     input.OrgType,
	}); err != nil {
		return err
	}

	s.InvalidateOrgCache(ctx)

	// ADR-064a: Feed the updated country_code into the user's
	// destination_country custom field (geo-aware personalization).
	// Non-fatal — the org update succeeds regardless.
	if input.CountryCode != "" {
		if err := s.setDestinationCountry(ctx, userID, input.CountryCode); err != nil {
			slog.Warn("organizations: failed to set destination_country from org update", "error", err)
		}
	}

	return nil
}

// DeleteOrganization soft-deletes an organization. Only owner can delete.
func (s *Service) DeleteOrganization(ctx context.Context, id, userID pgtype.UUID) error {
	role, err := s.store.GetMemberRole(ctx, id, userID)
	if err != nil {
		// BUG-021: A non-member (ErrNoRows) should get 403, not 500.
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
		}

		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner {
		return fmt.Errorf("%w: only the owner can delete an organization", errpkg.ErrForbidden)
	}

	if err := s.store.SoftDeleteOrganization(ctx, id); err != nil {
		return err
	}

	s.InvalidateOrgCache(ctx)

	return nil
}

// InviteMember invites a user to the organization. Only owner or admin can invite.
func (s *Service) InviteMember(ctx context.Context, orgID, invitedBy pgtype.UUID, input InviteMemberInput) (gen.OrganizationInvitation, error) {
	if input.Email == "" {
		return gen.OrganizationInvitation{}, errors.New("email is required")
	}

	role, err := s.store.GetMemberRole(ctx, orgID, invitedBy)
	if err != nil {
		// BUG-021: A non-member (ErrNoRows) should get 403, not 500.
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.OrganizationInvitation{}, fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
		}

		return gen.OrganizationInvitation{}, fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return gen.OrganizationInvitation{}, fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
	}

	// BUG-017: Check for existing pending invitation for this email+org.
	if _, err := s.store.GetPendingInvitationByEmail(ctx, orgID, input.Email); err == nil {
		return gen.OrganizationInvitation{}, fmt.Errorf("%w: invitation already pending for this email", errpkg.ErrDuplicate)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return gen.OrganizationInvitation{}, fmt.Errorf("checking existing invitations: %w", err)
	}

	invRole := input.Role
	if invRole == "" {
		invRole = InvitationRoleMember
	}

	token := uuid.New().String()
	tokenHash := hashToken(token)

	inv, err := s.store.CreateInvitation(ctx, gen.CreateOrganizationInvitationParams{
		OrganizationID: orgID,
		Email:          input.Email,
		Role:           invRole,
		InvitedBy:      invitedBy,
		TokenHash:      tokenHash,
	})
	if err != nil {
		return gen.OrganizationInvitation{}, err
	}

	// BUG-018: Enqueue a fire-and-forget email:send task to notify the
	// invitee. Best-effort — the invitation is already persisted.
	org, orgErr := s.store.GetOrganizationByIDOnly(ctx, orgID)
	if orgErr == nil {
		s.enqueueInvitationEmail(ctx, org.Name, input.Email, invRole, token)
	} else {
		slog.Warn("organizations: fetch org for invitation email",
			"org_id", orgID.String(), "error", orgErr)
	}

	return inv, nil
}

// ListInvitations returns pending invitations for an organization.
// Tenant-scoped for defense-in-depth (BUG-005).
func (s *Service) ListInvitations(ctx context.Context, orgID, userID pgtype.UUID) ([]gen.OrganizationInvitation, error) {
	role, err := s.store.GetMemberRole(ctx, orgID, userID)
	if err != nil {
		// BUG-021: A non-member (ErrNoRows) should get 403, not 500.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
		}

		return nil, fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return nil, fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
	}

	return s.store.ListInvitations(ctx, orgID)
}

// RevokeInvitation revokes a pending invitation. Only owner or admin can revoke.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Service) RevokeInvitation(ctx context.Context, orgID, userID, invitationID pgtype.UUID) error {
	role, err := s.store.GetMemberRole(ctx, orgID, userID)
	if err != nil {
		// BUG-021: A non-member (ErrNoRows) should get 403, not 500.
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
		}

		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
	}

	return s.store.RevokeInvitation(ctx, invitationID, orgID)
}

// AcceptInvitation accepts an invitation by token and adds the user as a member.
// The invitation accept, member insert, and member-count increment are wrapped
// in a single transaction so a partial failure cannot leave an accepted-but-
// unjoined invitation (BUG-008). The accepting user's email must match the
// invited email (BUG-003).
func (s *Service) AcceptInvitation(ctx context.Context, tokenHash string, userID pgtype.UUID) (string, error) {
	inv, err := s.store.GetInvitationByToken(ctx, tokenHash)
	if err != nil {
		return "", fmt.Errorf("finding invitation: %w", err)
	}

	// Fetch the org to get its for the tenant-scoped member-count update.
	org, err := s.store.GetOrganizationByIDOnly(ctx, inv.OrganizationID)
	if err != nil {
		return "", fmt.Errorf("finding organization: %w", err)
	}

	// Verify the accepting user's email matches the invited email (BUG-003).
	userEmail, err := s.store.GetUserEmail(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("fetching user email: %w", err)
	}

	if userEmail == "" || inv.Email == "" {
		return "", fmt.Errorf("%w: invitation email mismatch", errpkg.ErrForbidden)
	}

	if !strings.EqualFold(userEmail, inv.Email) {
		return "", fmt.Errorf("%w: invitation email mismatch", errpkg.ErrForbidden)
	}

	// Accepting an invitation while already a member must not overwrite the
	// existing role (AddOrganizationMember upserts role on conflict) nor
	// inflate member_count — only the invitation is consumed.
	alreadyMember, err := s.store.IsMember(ctx, inv.OrganizationID, userID)
	if err != nil {
		return "", fmt.Errorf("checking membership: %w", err)
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	if err := qtx.AcceptOrganizationInvitation(ctx, gen.AcceptOrganizationInvitationParams{
		ID:             inv.ID,
		OrganizationID: inv.OrganizationID,
	}); err != nil {
		return "", fmt.Errorf("accepting invitation: %w", err)
	}

	if !alreadyMember {
		if err := qtx.AddOrganizationMember(ctx, gen.AddOrganizationMemberParams{
			OrganizationID: inv.OrganizationID,
			UserID:         userID,
			Role:           inv.Role,
		}); err != nil {
			return "", fmt.Errorf("adding member: %w", err)
		}

		if err := qtx.IncrementOrganizationMemberCount(ctx, gen.IncrementOrganizationMemberCountParams{
			ID:          inv.OrganizationID,
			MemberCount: 1,
		}); err != nil {
			return "", fmt.Errorf("incrementing member count: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("committing transaction: %w", err)
	}

	return org.Slug, nil
}

// AddMember adds a user to an organization. Only owner or admin can add
// members. The member insert and member-count increment are wrapped in a
// single transaction so a partial failure cannot leave a stale member_count
// (BUG-013, mirroring RemoveMember).
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Service) AddMember(ctx context.Context, orgID, addedBy, targetUserID pgtype.UUID, role string) error {
	callerRole, err := s.store.GetMemberRole(ctx, orgID, addedBy)
	if err != nil {
		// BUG-021: A non-member (ErrNoRows) should get 403, not 500.
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
		}

		return fmt.Errorf("checking membership: %w", err)
	}

	if callerRole != MemberRoleOwner && callerRole != MemberRoleAdmin {
		return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
	}

	// Cannot add a member with the owner role — only the creator is owner.
	if role == MemberRoleOwner {
		return fmt.Errorf("%w: cannot add a member with the owner role", errpkg.ErrForbidden)
	}

	// Reject adding someone who is already a member. AddOrganizationMember
	// upserts the role on conflict, which would silently overwrite the
	// existing role, and the unconditional count increment would inflate
	// member_count. Role changes must go through UpdateMemberRole.
	isMember, err := s.store.IsMember(ctx, orgID, targetUserID)
	if err != nil {
		return fmt.Errorf("checking membership: %w", err)
	}
	if isMember {
		return fmt.Errorf("%w: user is already a member", errpkg.ErrAlreadyExists)
	}

	// Fetch the org for the tenant-scoped member-count update (read before tx).
	_, err = s.store.GetOrganizationByIDOnly(ctx, orgID)
	if err != nil {
		return fmt.Errorf("finding organization: %w", err)
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	if err := qtx.AddOrganizationMember(ctx, gen.AddOrganizationMemberParams{
		OrganizationID: orgID,
		UserID:         targetUserID,
		Role:           role,
		InvitedBy:      addedBy,
	}); err != nil {
		return fmt.Errorf("adding member: %w", err)
	}

	if err := qtx.IncrementOrganizationMemberCount(ctx, gen.IncrementOrganizationMemberCountParams{
		ID:          orgID,
		MemberCount: 1,
	}); err != nil {
		return fmt.Errorf("incrementing member count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// RemoveMember removes a member from the organization. Only owner or admin
// can remove. The member removal and count decrement are wrapped in a single
// transaction so a partial failure cannot leave a stale member_count
// (BUG-013).
func (s *Service) RemoveMember(ctx context.Context, orgID, removedBy, targetUserID pgtype.UUID) error {
	role, err := s.store.GetMemberRole(ctx, orgID, removedBy)
	if err != nil {
		// BUG-021: A non-member (ErrNoRows) should get 403, not 500.
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
		}

		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner && role != MemberRoleAdmin {
		return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
	}

	// Cannot remove the owner.
	targetRole, err := s.store.GetMemberRole(ctx, orgID, targetUserID)
	if err != nil {
		// Target is not a member → 404, not a 500.
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: member not found in this organization", errpkg.ErrNotFound)
		}

		return fmt.Errorf("checking target membership: %w", err)
	}

	if targetRole == MemberRoleOwner {
		return fmt.Errorf("%w: cannot remove the owner", errpkg.ErrForbidden)
	}

	// Fetch the org for tenant-scoped member-count update.
	_, err = s.store.GetOrganizationByIDOnly(ctx, orgID)
	if err != nil {
		return fmt.Errorf("finding organization: %w", err)
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	if err := qtx.RemoveOrganizationMember(ctx, gen.RemoveOrganizationMemberParams{
		OrganizationID: orgID,
		UserID:         targetUserID,
	}); err != nil {
		return fmt.Errorf("removing member: %w", err)
	}

	if err := qtx.IncrementOrganizationMemberCount(ctx, gen.IncrementOrganizationMemberCountParams{
		ID:          orgID,
		MemberCount: -1,
	}); err != nil {
		return fmt.Errorf("decrementing member count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// ListMembers returns all members of an organization. Tenant-scoped for
// defense-in-depth (BUG-004).
func (s *Service) ListMembers(ctx context.Context, orgID pgtype.UUID) ([]gen.ListOrganizationMembersRow, error) {
	return s.store.ListMembers(ctx, orgID)
}

// UpdateMemberRole changes a member's role. Only owner can change roles.
func (s *Service) UpdateMemberRole(ctx context.Context, orgID, updatedBy, targetUserID pgtype.UUID, newRole string) error {
	role, err := s.store.GetMemberRole(ctx, orgID, updatedBy)
	if err != nil {
		// BUG-021: A non-member (ErrNoRows) should get 403, not 500.
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: insufficient permissions", errpkg.ErrForbidden)
		}

		return fmt.Errorf("checking membership: %w", err)
	}

	if role != MemberRoleOwner {
		return fmt.Errorf("%w: only the owner can change roles", errpkg.ErrForbidden)
	}

	if err := s.store.UpdateMemberRole(ctx, gen.UpdateOrganizationMemberRoleParams{
		OrganizationID: orgID,
		UserID:         targetUserID,
		Role:           newRole,
	}); err != nil {
		return err
	}
	// Org membership does not feed into RBAC resolution (no org-role-based
	// permission nodes exist in the seed), so no perm-cache invalidation is
	// needed here. If org roles ever become RBAC-relevant, call
	// rbac.InvalidatePermCache(ctx, valkey, targetUserID) here.
	slog.Info("organization: member role updated", "org_id", orgID.String(), "user_id", targetUserID.String(), "role", newRole)

	return nil
}

// ClaimOwnership creates an ownership claim for an unclaimed organization.
// ClaimOwnership creates a new ownership claim. Before creating the claim,
// the service verifies: (1) the organization exists and is active (BUG-012),
// (2) the user doesn't already have a pending claim for this org (BUG-010),
// and (3) when a form submission is linked, its status is 'new' (BUG-009).
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Service) ClaimOwnership(ctx context.Context, userID, orgID pgtype.UUID, input ClaimOwnershipInput) (gen.OwnershipClaim, error) {
	// BUG-012: Verify the org exists and is active.
	active, err := s.store.ActiveOrgExistsByID(ctx, orgID)
	if err != nil {
		return gen.OwnershipClaim{}, fmt.Errorf("checking organization: %w", err)
	}

	if !active {
		return gen.OwnershipClaim{}, fmt.Errorf("%w: organization not found or inactive", errpkg.ErrNotFound)
	}

	// BUG-010: Check for existing pending claim by this user for this org.
	if _, err := s.store.GetPendingClaimByClaimant(ctx, orgID, userID); err == nil {
		return gen.OwnershipClaim{}, fmt.Errorf("%w: you already have a pending claim for this organization", errpkg.ErrDuplicate)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return gen.OwnershipClaim{}, fmt.Errorf("checking existing claims: %w", err)
	}

	var formSubmissionID pgtype.UUID
	if input.FormSubmissionID != nil {
		formSubmissionID = *input.FormSubmissionID

		// BUG-009: Verify the linked form submission has status='new'.
		submission, err := s.store.GetFormSubmissionByID(ctx, formSubmissionID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.OwnershipClaim{}, fmt.Errorf("%w: form submission not found", errpkg.ErrNotFound)
			}

			return gen.OwnershipClaim{}, fmt.Errorf("fetching form submission: %w", err)
		}

		if submission.Status != "new" {
			return gen.OwnershipClaim{}, fmt.Errorf("%w: form submission must have status 'new'", errpkg.ErrInvalid)
		}
	}

	return s.store.CreateOwnershipClaim(ctx, gen.CreateOwnershipClaimParams{

		ClaimantUserID:   userID,
		OrganizationID:   orgID,
		EvidenceUrl:      strPtr(input.EvidenceURL),
		Notes:            strPtr(input.Notes),
		FormSubmissionID: formSubmissionID,
	})
}

// ListOwnershipClaims returns claims filtered by status (admin).
func (s *Service) ListOwnershipClaims(ctx context.Context, status string, limit, offset int32) ([]gen.ListOwnershipClaimsRow, error) {
	return s.store.ListOwnershipClaims(ctx, status, limit, offset)
}

// ListMyClaims returns all claims filed by the requesting user
// (GET /me/claim-ownership). Claimant-scoped — this is the only way a
// claimant can discover their claim IDs to use GET/DELETE
// /claim-ownership/{id}.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Service) ListMyClaims(ctx context.Context, claimantUserID pgtype.UUID) ([]gen.ListOwnershipClaimsByClaimantRow, error) {
	return s.store.ListOwnershipClaimsByClaimant(ctx, claimantUserID)
}

// GetPendingOwnershipClaims returns pending claims with details (admin).
func (s *Service) GetPendingOwnershipClaims(ctx context.Context) ([]gen.GetPendingOwnershipClaimsRow, error) {
	return s.store.GetPendingOwnershipClaims(ctx)
}

// GetOwnershipClaimByID returns a single claim by ID. The caller must verify
// the requesting user is the claimant or an admin (handler enforces this).
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Service) GetOwnershipClaimByID(ctx context.Context, claimID pgtype.UUID) (gen.GetOwnershipClaimByIDRow, error) {
	return s.store.GetOwnershipClaimByID(ctx, claimID)
}

// WithdrawOwnershipClaim withdraws a pending claim by the claimant.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Service) WithdrawOwnershipClaim(ctx context.Context, claimID, claimantUserID pgtype.UUID) error {
	return s.store.WithdrawOwnershipClaim(ctx, claimID, claimantUserID)
}

// ReviewOwnershipClaim approves or rejects a claim (admin). When approving,
// the organization's owner_user_id is transferred to the claimant and the
// claimant is added as an 'owner' member (BUG-006). The claim review, ownership
// transfer, and owner-member insert are wrapped in a single transaction so a
// partial failure cannot leave an approved claim with no ownership change or
// an owner_user_id pointing at a non-member (BUG-006/BUG-007).
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Service) ReviewOwnershipClaim(ctx context.Context, claimID, reviewedBy pgtype.UUID, input ReviewClaimInput) error {
	// Fetch the claim before reviewing. This is required (not best-effort): on
	// approval the claimant_user_id and organization_id drive the ownership
	// transfer, so a failed fetch must abort the review — otherwise the claim
	// could be marked approved while the transfer is silently skipped.
	claim, err := s.store.GetOwnershipClaimByID(ctx, claimID)
	if err != nil {
		return fmt.Errorf("fetching claim for review: %w", err)
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.store.write.WithTx(tx)

	if err := qtx.ReviewOwnershipClaim(ctx, gen.ReviewOwnershipClaimParams{
		ID: claimID,

		Status:         input.Status,
		ReviewedBy:     reviewedBy,
		ResolutionNote: strPtr(input.ResolutionNote),
	}); err != nil {
		return fmt.Errorf("reviewing claim: %w", err)
	}

	// When approving, transfer org ownership to the claimant and ensure the
	// claimant is an owner member (BUG-006). Both writes are inside the same
	// transaction as the review so the three states stay consistent.
	approved := input.Status == ClaimStatusApproved && claim.ClaimantUserID.Valid
	if approved {
		if err := qtx.TransferOrganizationOwnership(ctx, gen.TransferOrganizationOwnershipParams{
			ID:          claim.OrganizationID,
			OwnerUserID: claim.ClaimantUserID,
		}); err != nil {
			return fmt.Errorf("transferring ownership: %w", err)
		}

		if err := qtx.AddOrganizationMember(ctx, gen.AddOrganizationMemberParams{
			OrganizationID: claim.OrganizationID,
			UserID:         claim.ClaimantUserID,
			Role:           MemberRoleOwner,
		}); err != nil {
			return fmt.Errorf("adding owner member: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	if approved {
		s.InvalidateOrgCache(ctx)
	}

	var eventType string

	switch input.Status {
	case ClaimStatusApproved:
		eventType = "claim.approved"
	case ClaimStatusRejected:
		eventType = "claim.rejected"
	}

	if eventType != "" {
		s.emitWebhook(ctx, eventType, map[string]any{
			"claim_id":    claimID,
			"entity_type": "organization",
			"entity_id":   claim.OrganizationID,
			"status":      input.Status,
		})
	}

	return nil
}

// VerifyOrganization marks an organization as verified and emits the
// organization.verified webhook event (Section 12). adminID is the
// administrator who performed the verification (for the webhook payload).
func (s *Service) VerifyOrganization(ctx context.Context, id, adminID pgtype.UUID) error {
	// Fetch the org name for the webhook payload (best-effort).
	org, err := s.store.GetOrganizationByID(ctx, id)
	if err != nil {
		slog.Warn("organizations: fetch org for verify webhook payload", "org_id", id.String(), "error", err)
	}

	if err := s.store.VerifyOrganization(ctx, id); err != nil {
		return fmt.Errorf("verifying organization: %w", err)
	}

	s.InvalidateOrgCache(ctx)

	s.emitWebhook(ctx, "organization.verified", map[string]any{
		"organization_id": id,
		"name":            org.Name,
		"verified_by":     adminID,
	})

	return nil
}

// UnverifyOrganization revokes an organization's verification badge and
// emits the organization.unverified webhook event (Section 12). Clears
// verification_type as well (BUG-016). adminID is the administrator who
// performed the unverification (for the webhook payload).
func (s *Service) UnverifyOrganization(ctx context.Context, id, adminID pgtype.UUID) error {
	if err := s.store.UnverifyOrganization(ctx, id); err != nil {
		return fmt.Errorf("unverifying organization: %w", err)
	}

	s.InvalidateOrgCache(ctx)

	s.emitWebhook(ctx, "organization.unverified", map[string]any{
		"organization_id": id,
		"unverified_by":   adminID,
	})

	return nil
}

// ActiveOrgExists reports whether an ACTIVE, non-deleted org with this id
// exists in the tenant. Powers the public profile gate.
func (s *Service) ActiveOrgExists(ctx context.Context, id pgtype.UUID) (bool, error) {
	return s.store.ActiveOrgExistsByID(ctx, id)
}

// GetPublicOrganizationBySlug returns an active org for the public profile header.
// Uses read-through Valkey cache (jc:org:profile:{}:{slug}) when
// a Valkey client is configured; falls back to direct DB on cache miss or
// when caching is disabled.
func (s *Service) GetPublicOrganizationBySlug(ctx context.Context, slug string) (gen.Organization, error) {
	return s.getOrSetOrgProfile(ctx, slug)
}

// ListPublicMembers returns the org's ACTIVE members for the public Members tab.
func (s *Service) ListPublicMembers(ctx context.Context, orgID pgtype.UUID) ([]gen.ListPublicOrganizationMembersRow, error) {
	return s.store.ListPublicMembers(ctx, orgID)
}

// GetOrgOwnerPublicProfile returns the org owner's public identity for the
// "Org Leader" sidebar widget.
func (s *Service) GetOrgOwnerPublicProfile(ctx context.Context, orgID pgtype.UUID) (gen.GetOrgOwnerPublicProfileRow, error) {
	return s.store.GetOrgOwnerPublicProfile(ctx, orgID)
}

// ListPublicOrganizations returns active orgs for the public Discover list.
// search filters by name/slug, orgType filters by org_type, verified filters
// by verified status (empty = all, "true" = verified only, "false" = unverified
// only). By default the handler passes verified="true" to preserve the original
// behavior of only showing verified orgs.
func (s *Service) ListPublicOrganizations(ctx context.Context, search, orgType, verified string, limit, offset int32) ([]gen.ListPublicOrganizationsRow, int64, error) {
	return s.getOrSetOrgList(ctx, search, orgType, verified, limit, offset)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func maybeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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

// GetOrganizationAnalytics returns aggregated analytics for a single org.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md.
func (s *Service) GetOrganizationAnalytics(ctx context.Context, orgID pgtype.UUID, periodDays int32) (gen.GetOrganizationAnalyticsRow, error) {
	return s.store.GetOrganizationAnalytics(ctx, orgID, periodDays)
}

// ─── Admin methods (bypass ownership checks) ─────────────────────────────

// AdminListOrganizations lists all organisations for admin view (no ownership filter).
func (s *Service) AdminListOrganizations(ctx context.Context, orgType, countryCode, search, verified string, limit, offset int32) ([]gen.ListOrganizationsRow, int64, error) {
	items, err := s.store.ListOrganizations(ctx, orgType, countryCode, search, verified, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountOrganizations(ctx, orgType, countryCode, search, verified)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// AdminGetOrganization retrieves a single organisation by ID (no ownership check).
func (s *Service) AdminGetOrganization(ctx context.Context, id pgtype.UUID) (gen.Organization, error) {
	return s.store.GetOrganizationByID(ctx, id)
}

// AdminUpdateOrganization updates an organisation without checking membership role.
func (s *Service) AdminUpdateOrganization(ctx context.Context, id pgtype.UUID, input UpdateOrganizationInput) error {
	if err := validateOrgType(input.OrgType); err != nil {
		return err
	}

	if err := s.store.UpdateOrganization(ctx, gen.UpdateOrganizationParams{
		ID: id,

		Name:        input.Name,
		Tagline:     strPtr(input.Tagline),
		Description: strPtr(input.Description),
		WebsiteUrl:  strPtr(input.WebsiteURL),
		CountryCode: strPtr(input.CountryCode),
		OrgType:     input.OrgType,
	}); err != nil {
		return err
	}

	s.InvalidateOrgCache(ctx)

	return nil
}

// AdminDeleteOrganization soft-deletes an organisation without checking ownership.
func (s *Service) AdminDeleteOrganization(ctx context.Context, id pgtype.UUID) error {
	if err := s.store.SoftDeleteOrganization(ctx, id); err != nil {
		return err
	}

	s.InvalidateOrgCache(ctx)

	return nil
}

// AdminListMembers lists members of an organisation (no membership check).
// Tenant-scoped for defense-in-depth (BUG-004/005).
func (s *Service) AdminListMembers(ctx context.Context, orgID pgtype.UUID) ([]gen.ListOrganizationMembersRow, error) {
	return s.store.ListMembers(ctx, orgID)
}

// AdminListInvitations lists invitations for an organisation (no membership check).
// Tenant-scoped for defense-in-depth (BUG-005).
func (s *Service) AdminListInvitations(ctx context.Context, orgID pgtype.UUID) ([]gen.OrganizationInvitation, error) {
	return s.store.ListInvitations(ctx, orgID)
}

// setDestinationCountry saves the given ISO 3166-1 alpha-2 code as the
// `destination_country` custom field on the user's profile (ADR-064a). Since
// the org's country_code is sourced from the spaces country table, it is a
// geo-aware signal — creating or editing an org in a country means the user
// is operationally tied to that country, so we feed it into the geo-aware
// personalization system. Mirrors users.Store.setGeoCustomField.
func (s *Service) setDestinationCountry(ctx context.Context, userID pgtype.UUID, geoCode string) error {
	code := strings.ToUpper(strings.TrimSpace(geoCode))
	if code == "" {
		return nil
	}

	field, err := s.store.read.GetCustomFieldByKey(ctx,
		"destination_country",
	)
	if err != nil {
		return fmt.Errorf("looking up destination_country custom field: %w", err)
	}

	profileID, err := s.store.read.GetUserProfileID(ctx,
		userID,
	)
	if err != nil {
		return fmt.Errorf("getting user profile id: %w", err)
	}

	if _, err := s.store.write.UpsertCustomFieldValue(ctx, gen.UpsertCustomFieldValueParams{
		FieldID:    field.ID,
		EntityType: "user_profile",
		EntityID:   profileID,
		ValueText:  &code,
	}); err != nil {
		return fmt.Errorf("upserting destination_country custom field value: %w", err)
	}

	// ADR-064a: Invalidate the VisitorContext cache so the next request
	// picks up the new destination_country resolution.
	if s.vk != nil {
		_ = s.vk.Del(ctx, valkey.MustKey("visitor:ctx:user:%s", userID.String())).Err()
		_ = s.vk.Del(ctx, valkey.MustKey("recommendations:user:%s", userID.String())).Err()
	}

	return nil
}
