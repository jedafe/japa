package professional

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Profile Type Definitions
// ---------------------------------------------------------------------------

// CreateProfessionalProfileTypeInput holds parameters for creating a profile type.
type CreateProfessionalProfileTypeInput struct {
	Name         string
	Slug         string
	Icon         string
	FieldGroupID pgtype.UUID
	IsSystem     bool
}

// CreateProfessionalProfileType creates a new admin-defined profile type.
func (s *Service) CreateProfessionalProfileType(ctx context.Context, tenantID pgtype.UUID, input CreateProfessionalProfileTypeInput) (gen.ProfessionalProfileType, error) {
	if input.Name == "" {
		return gen.ProfessionalProfileType{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return gen.ProfessionalProfileType{}, errors.New("slug is required")
	}

	var icon *string
	if input.Icon != "" {
		icon = &input.Icon
	}

	return s.store.createProfessionalProfileType(ctx, gen.CreateProfessionalProfileTypeParams{
		TenantID:     tenantID,
		Name:         input.Name,
		Slug:         input.Slug,
		Icon:         icon,
		FieldGroupID: input.FieldGroupID,
		IsSystem:     input.IsSystem,
	})
}

// GetProfessionalProfileType retrieves a profile type by ID.
func (s *Service) GetProfessionalProfileType(ctx context.Context, id, tenantID pgtype.UUID) (gen.ProfessionalProfileType, error) {
	return s.store.getProfessionalProfileTypeByID(ctx, id, tenantID)
}

// GetProfessionalProfileTypeBySlug retrieves a profile type by slug.
func (s *Service) GetProfessionalProfileTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.ProfessionalProfileType, error) {
	return s.store.getProfessionalProfileTypeBySlug(ctx, slug, tenantID)
}

// ListProfessionalProfileTypes returns all profile types for a tenant.
func (s *Service) ListProfessionalProfileTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalProfileType, error) {
	return s.store.listProfessionalProfileTypesByTenant(ctx, tenantID, limit, offset)
}

// UpdateProfessionalProfileTypeInput holds parameters for updating a profile type.
type UpdateProfessionalProfileTypeInput struct {
	Name         string
	Icon         string
	FieldGroupID pgtype.UUID
}

// UpdateProfessionalProfileType updates an existing profile type.
func (s *Service) UpdateProfessionalProfileType(ctx context.Context, id, tenantID pgtype.UUID, input UpdateProfessionalProfileTypeInput) (gen.ProfessionalProfileType, error) {
	if input.Name == "" {
		return gen.ProfessionalProfileType{}, errors.New("name is required")
	}

	var icon *string
	if input.Icon != "" {
		icon = &input.Icon
	}

	return s.store.updateProfessionalProfileType(ctx, gen.UpdateProfessionalProfileTypeParams{
		ID:           id,
		TenantID:     tenantID,
		Name:         input.Name,
		Icon:         icon,
		FieldGroupID: input.FieldGroupID,
	})
}

// DeleteProfessionalProfileType removes a profile type and its cascaded children.
func (s *Service) DeleteProfessionalProfileType(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteProfessionalProfileType(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Professional Profiles
// ---------------------------------------------------------------------------

// CreateProfessionalProfileInput holds parameters for creating a profile.
type CreateProfessionalProfileInput struct {
	TypeID           pgtype.UUID
	Name             string
	Slug             string
	Tagline          string
	Description      string
	LogoMediaID      pgtype.UUID
	HeaderMediaID    pgtype.UUID
	WebsiteURL       string
	VerificationType string
	FieldValues      string // JSON string
}

// CreateProfessionalProfile creates a new professional profile.
func (s *Service) CreateProfessionalProfile(ctx context.Context, tenantID, ownerUserID pgtype.UUID, input CreateProfessionalProfileInput) (gen.ProfessionalProfile, error) {
	if input.Name == "" {
		return gen.ProfessionalProfile{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return gen.ProfessionalProfile{}, errors.New("slug is required")
	}

	var tagline *string
	if input.Tagline != "" {
		tagline = &input.Tagline
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var websiteURL *string
	if input.WebsiteURL != "" {
		websiteURL = &input.WebsiteURL
	}

	var verificationType *string

	if input.VerificationType != "" {
		if !IsValidVerificationType(input.VerificationType) {
			return gen.ProfessionalProfile{}, fmt.Errorf("invalid verification type: %s", input.VerificationType)
		}

		verificationType = &input.VerificationType
	}

	fieldValues := []byte(input.FieldValues)
	if len(fieldValues) == 0 {
		fieldValues = []byte("{}")
	}

	profile, err := s.store.createProfessionalProfile(ctx, gen.CreateProfessionalProfileParams{
		TenantID:         tenantID,
		OwnerUserID:      ownerUserID,
		TypeID:           input.TypeID,
		Name:             input.Name,
		Slug:             input.Slug,
		Tagline:          tagline,
		Description:      description,
		LogoMediaID:      input.LogoMediaID,
		HeaderMediaID:    input.HeaderMediaID,
		WebsiteUrl:       websiteURL,
		VerificationType: verificationType,
		FieldValues:      fieldValues,
	})
	if err != nil {
		return gen.ProfessionalProfile{}, err
	}

	// Auto-add the owner as a member with 'owner' role.
	if addErr := s.store.addProfessionalProfileMember(ctx, gen.AddProfessionalProfileMemberParams{
		ProfileID: profile.ID,
		UserID:    ownerUserID,
		Role:      MemberRoleOwner,
	}); addErr != nil {
		return gen.ProfessionalProfile{}, fmt.Errorf("adding owner as member: %w", addErr)
	}

	return profile, nil
}

// GetProfessionalProfile retrieves a profile by ID.
func (s *Service) GetProfessionalProfile(ctx context.Context, id, tenantID pgtype.UUID) (gen.ProfessionalProfile, error) {
	return s.store.getProfessionalProfileByID(ctx, id, tenantID)
}

// GetProfessionalProfileBySlug retrieves a profile by slug (global uniqueness).
func (s *Service) GetProfessionalProfileBySlug(ctx context.Context, slug string) (gen.ProfessionalProfile, error) {
	return s.store.getProfessionalProfileBySlug(ctx, slug)
}

// ListProfessionalProfilesByOwner returns all profiles owned by a user.
func (s *Service) ListProfessionalProfilesByOwner(ctx context.Context, ownerUserID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalProfile, error) {
	return s.store.listProfessionalProfilesByOwner(ctx, ownerUserID, tenantID, limit, offset)
}

// ListProfessionalProfilesByType returns all profiles of a given type.
func (s *Service) ListProfessionalProfilesByType(ctx context.Context, typeID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalProfile, error) {
	return s.store.listProfessionalProfilesByType(ctx, typeID, tenantID, limit, offset)
}

// UpdateProfessionalProfileInput holds parameters for updating a profile.
type UpdateProfessionalProfileInput struct {
	Name             string
	Tagline          string
	Description      string
	LogoMediaID      pgtype.UUID
	HeaderMediaID    pgtype.UUID
	WebsiteURL       string
	VerificationType string
	Status           string
	FieldValues      string
}

// UpdateProfessionalProfile updates an existing profile.
func (s *Service) UpdateProfessionalProfile(ctx context.Context, id, tenantID pgtype.UUID, input UpdateProfessionalProfileInput) (gen.ProfessionalProfile, error) {
	if input.Name == "" {
		return gen.ProfessionalProfile{}, errors.New("name is required")
	}

	if input.Status != "" && !IsValidStatus(input.Status) {
		return gen.ProfessionalProfile{}, fmt.Errorf("invalid status: %s", input.Status)
	}

	var tagline *string
	if input.Tagline != "" {
		tagline = &input.Tagline
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var websiteURL *string
	if input.WebsiteURL != "" {
		websiteURL = &input.WebsiteURL
	}

	var verificationType *string

	if input.VerificationType != "" {
		if !IsValidVerificationType(input.VerificationType) {
			return gen.ProfessionalProfile{}, fmt.Errorf("invalid verification type: %s", input.VerificationType)
		}

		verificationType = &input.VerificationType
	}

	status := input.Status
	if status == "" {
		status = StatusActive
	}

	fieldValues := []byte(input.FieldValues)
	if len(fieldValues) == 0 {
		fieldValues = []byte("{}")
	}

	return s.store.updateProfessionalProfile(ctx, gen.UpdateProfessionalProfileParams{
		ID:               id,
		TenantID:         tenantID,
		Name:             input.Name,
		Tagline:          tagline,
		Description:      description,
		LogoMediaID:      input.LogoMediaID,
		HeaderMediaID:    input.HeaderMediaID,
		WebsiteUrl:       websiteURL,
		VerificationType: verificationType,
		Status:           status,
		FieldValues:      fieldValues,
	})
}

// DeleteProfessionalProfile removes a profile and its cascaded members/invitations.
func (s *Service) DeleteProfessionalProfile(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteProfessionalProfile(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Member Management
// ---------------------------------------------------------------------------

// AddMemberInput holds parameters for adding a member.
type AddMemberInput struct {
	UserID pgtype.UUID
	Role   string
}

// AddMember adds a user as a member of the profile.
func (s *Service) AddMember(ctx context.Context, profileID, invitedBy pgtype.UUID, input AddMemberInput) error {
	if input.Role == "" {
		input.Role = MemberRoleMember
	}

	if !IsValidMemberRole(input.Role) {
		return fmt.Errorf("invalid member role: %s", input.Role)
	}

	return s.store.addProfessionalProfileMember(ctx, gen.AddProfessionalProfileMemberParams{
		ProfileID: profileID,
		UserID:    input.UserID,
		Role:      input.Role,
		InvitedBy: invitedBy,
	})
}

// RemoveMember removes a user from the profile.
func (s *Service) RemoveMember(ctx context.Context, profileID, userID pgtype.UUID) error {
	return s.store.removeProfessionalProfileMember(ctx, profileID, userID)
}

// ListMembers returns all members of the profile.
func (s *Service) ListMembers(ctx context.Context, profileID pgtype.UUID) ([]gen.ListProfessionalProfileMembersRow, error) {
	return s.store.listProfessionalProfileMembers(ctx, profileID)
}

// UpdateMemberRoleInput holds parameters for updating a member role.
type UpdateMemberRoleInput struct {
	Role string
}

// UpdateMemberRole updates a member's role.
func (s *Service) UpdateMemberRole(ctx context.Context, profileID, userID pgtype.UUID, input UpdateMemberRoleInput) error {
	if !IsValidMemberRole(input.Role) {
		return fmt.Errorf("invalid member role: %s", input.Role)
	}

	return s.store.updateProfessionalProfileMemberRole(ctx, profileID, userID, input.Role)
}

// IsMember returns true if the user is a member of the profile.
func (s *Service) IsMember(ctx context.Context, profileID, userID pgtype.UUID) (bool, error) {
	return s.store.isProfessionalProfileMember(ctx, profileID, userID)
}

// IsOwner returns true if the user owns the profile.
func (s *Service) IsOwner(ctx context.Context, profileID, userID, tenantID pgtype.UUID) (bool, error) {
	return s.store.isProfessionalProfileOwner(ctx, profileID, userID, tenantID)
}

// ---------------------------------------------------------------------------
// Invitation Management
// ---------------------------------------------------------------------------

// generateInvitationToken creates a cryptographically random hex token.
func generateInvitationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating invitation token: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// CreateInvitationInput holds parameters for creating an invitation.
type CreateInvitationInput struct {
	Email string
	Role  string
}

// CreateInvitation creates a new invitation for the profile.
func (s *Service) CreateInvitation(ctx context.Context, profileID, invitedBy pgtype.UUID, input CreateInvitationInput) (gen.ProfessionalProfileInvitation, error) {
	if input.Email == "" {
		return gen.ProfessionalProfileInvitation{}, errors.New("email is required")
	}

	if input.Role == "" {
		input.Role = MemberRoleMember
	}

	if !IsValidMemberRole(input.Role) {
		return gen.ProfessionalProfileInvitation{}, fmt.Errorf("invalid member role: %s", input.Role)
	}

	token, err := generateInvitationToken()
	if err != nil {
		return gen.ProfessionalProfileInvitation{}, err
	}

	var expiresAt pgtype.Timestamptz
	if err := expiresAt.Scan("now() + interval '7 days'"); err != nil {
		return gen.ProfessionalProfileInvitation{}, fmt.Errorf("computing expiry: %w", err)
	}

	return s.store.createProfessionalProfileInvitation(ctx, gen.CreateProfessionalProfileInvitationParams{
		ProfileID: profileID,
		Email:     input.Email,
		Role:      input.Role,
		InvitedBy: invitedBy,
		Token:     token,
		ExpiresAt: expiresAt,
	})
}

// AcceptInvitation accepts an invitation by token and adds the user as a member.
func (s *Service) AcceptInvitation(ctx context.Context, token string, userID pgtype.UUID) error {
	inv, err := s.store.getProfessionalProfileInvitationByToken(ctx, token)
	if err != nil {
		return errors.New("invalid or expired invitation")
	}

	// Check if the invitation has expired by comparing with current time.
	var now pgtype.Timestamptz
	if err := now.Scan("now()"); err != nil {
		return fmt.Errorf("checking expiry: %w", err)
	}

	if inv.ExpiresAt.Time.Before(now.Time) {
		return errors.New("invitation has expired")
	}

	// Accept the invitation.
	if err := s.store.acceptProfessionalProfileInvitation(ctx, inv.ID); err != nil {
		return fmt.Errorf("accepting invitation: %w", err)
	}

	// Add the user as a member.
	if err := s.store.addProfessionalProfileMember(ctx, gen.AddProfessionalProfileMemberParams{
		ProfileID: inv.ProfileID,
		UserID:    userID,
		Role:      inv.Role,
		InvitedBy: inv.InvitedBy,
	}); err != nil {
		return fmt.Errorf("adding member from invitation: %w", err)
	}

	return nil
}

// ListInvitations returns all pending invitations for the profile.
func (s *Service) ListInvitations(ctx context.Context, profileID pgtype.UUID) ([]gen.ProfessionalProfileInvitation, error) {
	return s.store.listProfessionalProfileInvitations(ctx, profileID)
}

// ---------------------------------------------------------------------------
// Authorship Context
// ---------------------------------------------------------------------------

// AuthorContext represents the authorship context for content creation.
type AuthorContext struct {
	Type string      `json:"type"` // 'user' | 'professional_profile'
	ID   pgtype.UUID `json:"id"`
}

// GetAuthorContext returns the author context for a user on a profile.
// The user must be a member of the profile to post as it.
func (s *Service) GetAuthorContext(ctx context.Context, profileID, userID, _ pgtype.UUID) (AuthorContext, error) {
	// Check if the user is a member of the profile.
	isMember, err := s.store.isProfessionalProfileMember(ctx, profileID, userID)
	if err != nil {
		return AuthorContext{}, fmt.Errorf("checking membership: %w", err)
	}

	if !isMember {
		return AuthorContext{}, errors.New("user is not a member of this profile")
	}

	return AuthorContext{
		Type: AuthorTypeProfile,
		ID:   profileID,
	}, nil
}

// GetUserAuthorContext returns the default user author context.
func GetUserAuthorContext(userID pgtype.UUID) AuthorContext {
	return AuthorContext{
		Type: AuthorTypeUser,
		ID:   userID,
	}
}

// CanPostAsProfile returns true if the user can post as the given profile.
func (s *Service) CanPostAsProfile(ctx context.Context, profileID, userID, tenantID pgtype.UUID) (bool, error) {
	// Owner can always post.
	isOwner, err := s.store.isProfessionalProfileOwner(ctx, profileID, userID, tenantID)
	if err != nil {
		return false, err
	}

	if isOwner {
		return true, nil
	}

	// Member with editor or admin role can post.
	member, err := s.store.getProfessionalProfileMember(ctx, profileID, userID)
	if err != nil {
		return false, nil //nolint:nilerr // intentional: not a member = not authorized
	}

	return member.Role == MemberRoleAdmin || member.Role == MemberRoleEditor, nil
}

// IncrementFollowerCount adjusts the follower count of a profile.
func (s *Service) IncrementFollowerCount(ctx context.Context, id, tenantID pgtype.UUID, delta int32) error {
	return s.store.incrementFollowerCount(ctx, id, tenantID, delta)
}

// CleanupExpiredInvitations removes all expired, unaccepted invitations.
func (s *Service) CleanupExpiredInvitations(ctx context.Context) error {
	return s.store.deleteExpiredInvitations(ctx)
}

// ---------------------------------------------------------------------------
// Professional Profile Relocations
// ---------------------------------------------------------------------------

// CreateRelocationInput holds parameters for creating a relocation.
type CreateRelocationInput struct {
	Country string
	City    string
	Status  string
	Year    int32
}

// CreateRelocation records a relocation entry for a professional profile.
func (s *Service) CreateRelocation(ctx context.Context, tenantID, profileID pgtype.UUID, input CreateRelocationInput) (gen.ProfessionalRelocation, error) {
	if input.Country == "" {
		return gen.ProfessionalRelocation{}, errors.New("country is required")
	}

	status := input.Status
	if status == "" {
		status = RelocationStatusPlanned
	}

	if !IsValidRelocationStatus(status) {
		return gen.ProfessionalRelocation{}, fmt.Errorf("invalid relocation status: %s", status)
	}

	var city *string
	if input.City != "" {
		city = &input.City
	}

	statusPtr := &status

	var year *int32
	if input.Year != 0 {
		year = &input.Year
	}

	return s.store.createProfessionalRelocation(ctx, gen.CreateProfessionalRelocationParams{
		TenantID:  tenantID,
		ProfileID: profileID,
		Country:   input.Country,
		City:      city,
		Status:    statusPtr,
		Year:      year,
	})
}

// ListRelocations returns all relocations for a profile.
func (s *Service) ListRelocations(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalRelocation, error) {
	return s.store.listProfessionalRelocationsByProfile(ctx, profileID, tenantID, limit, offset)
}

// UpdateRelocationInput holds parameters for updating a relocation.
// All fields are optional; omitted fields retain their existing values.
type UpdateRelocationInput struct {
	Country string
	City    string
	Status  string
	Year    int32
}

// UpdateRelocation partially updates a relocation entry (PATCH semantics).
func (s *Service) UpdateRelocation(ctx context.Context, id, tenantID pgtype.UUID, input UpdateRelocationInput) (gen.ProfessionalRelocation, error) {
	if input.Status != "" && !IsValidRelocationStatus(input.Status) {
		return gen.ProfessionalRelocation{}, fmt.Errorf("invalid relocation status: %s", input.Status)
	}

	var country *string
	if input.Country != "" {
		country = &input.Country
	}

	var city *string
	if input.City != "" {
		city = &input.City
	}

	var status *string
	if input.Status != "" {
		status = &input.Status
	}

	var year *int32
	if input.Year != 0 {
		year = &input.Year
	}

	return s.store.updateProfessionalRelocation(ctx, gen.UpdateProfessionalRelocationParams{
		ID:       id,
		TenantID: tenantID,
		Country:  country,
		City:     city,
		Status:   status,
		Year:     year,
	})
}

// DeleteRelocation removes a relocation entry.
func (s *Service) DeleteRelocation(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteProfessionalRelocation(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Professional Profile Documents
// ---------------------------------------------------------------------------

// CreateDocumentInput holds parameters for creating a document.
type CreateDocumentInput struct {
	Title        string
	DocumentType string
	Description  string
	FileURL      string
}

// CreateDocument records a credential document for a professional profile.
func (s *Service) CreateDocument(ctx context.Context, tenantID, profileID pgtype.UUID, input CreateDocumentInput) (gen.ProfessionalDocument, error) {
	if input.Title == "" {
		return gen.ProfessionalDocument{}, errors.New("title is required")
	}

	if input.DocumentType == "" {
		return gen.ProfessionalDocument{}, errors.New("document_type is required")
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var fileURL *string
	if input.FileURL != "" {
		fileURL = &input.FileURL
	}

	return s.store.createProfessionalDocument(ctx, gen.CreateProfessionalDocumentParams{
		TenantID:     tenantID,
		ProfileID:    profileID,
		Title:        input.Title,
		DocumentType: input.DocumentType,
		Description:  description,
		FileUrl:      fileURL,
	})
}

// ListDocuments returns all documents for a profile.
func (s *Service) ListDocuments(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalDocument, error) {
	return s.store.listProfessionalDocumentsByProfile(ctx, profileID, tenantID, limit, offset)
}

// UpdateDocumentInput holds parameters for updating a document.
// All fields are optional; omitted fields retain their existing values.
type UpdateDocumentInput struct {
	Title        string
	DocumentType string
	Description  string
	FileURL      string
}

// UpdateDocument partially updates a document entry (PATCH semantics).
func (s *Service) UpdateDocument(ctx context.Context, id, tenantID pgtype.UUID, input UpdateDocumentInput) (gen.ProfessionalDocument, error) {
	var title *string
	if input.Title != "" {
		title = &input.Title
	}

	var documentType *string
	if input.DocumentType != "" {
		documentType = &input.DocumentType
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	var fileURL *string
	if input.FileURL != "" {
		fileURL = &input.FileURL
	}

	return s.store.updateProfessionalDocument(ctx, gen.UpdateProfessionalDocumentParams{
		ID:           id,
		TenantID:     tenantID,
		Title:        title,
		DocumentType: documentType,
		Description:  description,
		FileUrl:      fileURL,
	})
}

// DeleteDocument removes a document entry.
func (s *Service) DeleteDocument(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteProfessionalDocument(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Professional Profile Verifiers
// ---------------------------------------------------------------------------

// CreateVerifierInput holds parameters for assigning a verifier.
type CreateVerifierInput struct {
	UserID pgtype.UUID
	Role   string
}

// CreateVerifier assigns a user as a verifier for a profile. If the user is
// already a verifier for this profile, their role/status is updated instead.
func (s *Service) CreateVerifier(ctx context.Context, tenantID, profileID pgtype.UUID, input CreateVerifierInput) (gen.ProfessionalVerifier, error) {
	var role *string
	if input.Role != "" {
		role = &input.Role
	}

	status := VerifierStatusPending
	statusPtr := &status

	return s.store.createProfessionalVerifier(ctx, gen.CreateProfessionalVerifierParams{
		TenantID:  tenantID,
		ProfileID: profileID,
		UserID:    input.UserID,
		Role:      role,
		Status:    statusPtr,
	})
}

// ListVerifiers returns all verifiers for a profile, with user names resolved
// at render time via JOIN users.
func (s *Service) ListVerifiers(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ListProfessionalVerifiersByProfileRow, error) {
	return s.store.listProfessionalVerifiersByProfile(ctx, profileID, tenantID, limit, offset)
}

// DeleteVerifier removes a verifier entry.
func (s *Service) DeleteVerifier(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteProfessionalVerifier(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Ownership helpers for child resource authorization
// ---------------------------------------------------------------------------

// GetRelocationProfileID returns the profile_id that owns the given relocation.
func (s *Service) GetRelocationProfileID(ctx context.Context, relocationID, tenantID pgtype.UUID) (pgtype.UUID, error) {
	return s.store.getRelocationProfileID(ctx, relocationID, tenantID)
}

// GetDocumentProfileID returns the profile_id that owns the given document.
func (s *Service) GetDocumentProfileID(ctx context.Context, documentID, tenantID pgtype.UUID) (pgtype.UUID, error) {
	return s.store.getDocumentProfileID(ctx, documentID, tenantID)
}

// GetVerifierProfileID returns the profile_id that owns the given verifier.
// Uses raw SQL because verifiers have no dedicated GetByID sqlc query.
func (s *Service) GetVerifierProfileID(ctx context.Context, verifierID, tenantID pgtype.UUID) (pgtype.UUID, error) {
	query := `SELECT profile_id FROM professional_verifiers WHERE id = $1 AND tenant_id = $2`

	var profileID pgtype.UUID

	if err := s.db.Reader.QueryRow(ctx, query, verifierID, tenantID).Scan(&profileID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting verifier profile ID: %w", err)
	}

	return profileID, nil
}

// IsAdminMember returns true if the user has an admin role in the profile.
func (s *Service) IsAdminMember(ctx context.Context, profileID, userID pgtype.UUID) (bool, error) {
	member, err := s.store.getProfessionalProfileMember(ctx, profileID, userID)
	if err != nil {
		return false, nil //nolint:nilerr // intentional: not a member = not admin
	}

	return member.Role == MemberRoleAdmin, nil
}

// ListProfileContent returns paginated content authored by a professional profile.
func (s *Service) ListProfileContent(ctx context.Context, tenantID, profileID pgtype.UUID, limit, offset int32) ([]gen.Content, error) {
	return s.store.listProfileContent(ctx, tenantID, profileID, limit, offset)
}
