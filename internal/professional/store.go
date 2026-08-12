package professional

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for professional profile operations.
// The Service layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Profile Type Definitions
// ---------------------------------------------------------------------------

// Reserved for future features.
func (s *Store) createProfessionalProfileType(ctx context.Context, p gen.CreateProfessionalProfileTypeParams) (gen.ProfessionalProfileType, error) {
	row, err := s.write.CreateProfessionalProfileType(ctx, p)
	if err != nil {
		return gen.ProfessionalProfileType{}, fmt.Errorf("creating professional profile type: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) getProfessionalProfileTypeByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ProfessionalProfileType, error) {
	row, err := s.read.GetProfessionalProfileTypeByID(ctx, gen.GetProfessionalProfileTypeByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ProfessionalProfileType{}, fmt.Errorf("getting professional profile type by ID: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) getProfessionalProfileTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.ProfessionalProfileType, error) {
	row, err := s.read.GetProfessionalProfileTypeBySlug(ctx, gen.GetProfessionalProfileTypeBySlugParams{Slug: slug, TenantID: tenantID})
	if err != nil {
		return gen.ProfessionalProfileType{}, fmt.Errorf("getting professional profile type by slug: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) listProfessionalProfileTypesByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalProfileType, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListProfessionalProfileTypesByTenant(ctx, gen.ListProfessionalProfileTypesByTenantParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing professional profile types: %w", err)
	}

	return rows, nil
}

// Reserved for future features.
func (s *Store) updateProfessionalProfileType(ctx context.Context, p gen.UpdateProfessionalProfileTypeParams) (gen.ProfessionalProfileType, error) {
	row, err := s.write.UpdateProfessionalProfileType(ctx, p)
	if err != nil {
		return gen.ProfessionalProfileType{}, fmt.Errorf("updating professional profile type: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) deleteProfessionalProfileType(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteProfessionalProfileType(ctx, gen.DeleteProfessionalProfileTypeParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting professional profile type: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Professional Profiles
// ---------------------------------------------------------------------------

// getUserProfileType returns the user's profile_type (personal, business, organisation).
func (s *Store) getUserProfileType(ctx context.Context, userID, tenantID pgtype.UUID) (string, error) {
	row, err := s.read.GetUserByID(ctx, gen.GetUserByIDParams{UserID: userID, TenantID: tenantID})
	if err != nil {
		return "", fmt.Errorf("getting user profile type: %w", err)
	}
	return row.ProfileType, nil
}

// getUserEmail returns the user's email address, tenant-scoped.
func (s *Store) getUserEmail(ctx context.Context, userID, tenantID pgtype.UUID) (string, error) {
	row, err := s.read.GetUserByID(ctx, gen.GetUserByIDParams{UserID: userID, TenantID: tenantID})
	if err != nil {
		return "", fmt.Errorf("getting user email: %w", err)
	}
	return row.Email, nil
}

// Reserved for future features.
func (s *Store) createProfessionalProfile(ctx context.Context, p gen.CreateProfessionalProfileParams) (gen.ProfessionalProfile, error) {
	row, err := s.write.CreateProfessionalProfile(ctx, p)
	if err != nil {
		return gen.ProfessionalProfile{}, fmt.Errorf("creating professional profile: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) getProfessionalProfileByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ProfessionalProfile, error) {
	row, err := s.read.GetProfessionalProfileByID(ctx, gen.GetProfessionalProfileByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ProfessionalProfile{}, fmt.Errorf("getting professional profile by ID: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) getProfessionalProfileBySlug(ctx context.Context, slug string) (gen.ProfessionalProfile, error) {
	row, err := s.read.GetProfessionalProfileBySlug(ctx, slug)
	if err != nil {
		return gen.ProfessionalProfile{}, fmt.Errorf("getting professional profile by slug: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) listProfessionalProfilesByOwner(ctx context.Context, ownerUserID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalProfile, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListProfessionalProfilesByOwner(ctx, gen.ListProfessionalProfilesByOwnerParams{
		OwnerUserID: ownerUserID,
		TenantID:    tenantID,
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing professional profiles by owner: %w", err)
	}

	return rows, nil
}

// Reserved for future features.
func (s *Store) listProfessionalProfilesByType(ctx context.Context, typeID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalProfile, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListProfessionalProfilesByType(ctx, gen.ListProfessionalProfilesByTypeParams{
		TypeID:     typeID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing professional profiles by type: %w", err)
	}

	return rows, nil
}

// listAllProfessionalProfiles returns all profiles in a tenant (admin scope).
func (s *Store) listAllProfessionalProfiles(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalProfile, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListAllProfessionalProfiles(ctx, gen.ListAllProfessionalProfilesParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing all professional profiles: %w", err)
	}

	return rows, nil
}

// Reserved for future features.
func (s *Store) updateProfessionalProfile(ctx context.Context, p gen.UpdateProfessionalProfileParams) (gen.ProfessionalProfile, error) {
	row, err := s.write.UpdateProfessionalProfile(ctx, p)
	if err != nil {
		return gen.ProfessionalProfile{}, fmt.Errorf("updating professional profile: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) deleteProfessionalProfile(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteProfessionalProfile(ctx, gen.DeleteProfessionalProfileParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting professional profile: %w", err)
	}

	return nil
}

// setProfessionalProfileVerified sets the verified flag on a profile.
func (s *Store) setProfessionalProfileVerified(ctx context.Context, id, tenantID pgtype.UUID, verified bool) error {
	if err := s.write.SetProfessionalProfileVerified(ctx, gen.SetProfessionalProfileVerifiedParams{
		ID: id, TenantID: tenantID, Verified: verified,
	}); err != nil {
		return fmt.Errorf("setting professional profile verified: %w", err)
	}

	return nil
}

// Reserved for future features.
func (s *Store) incrementFollowerCount(ctx context.Context, id, tenantID pgtype.UUID, delta int32) error {
	if err := s.write.IncrementFollowerCount(ctx, gen.IncrementFollowerCountParams{ID: id, TenantID: tenantID, Delta: delta}); err != nil {
		return fmt.Errorf("incrementing follower count: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Professional Profile Members
// ---------------------------------------------------------------------------

// Reserved for future features.
func (s *Store) addProfessionalProfileMember(ctx context.Context, p gen.AddProfessionalProfileMemberParams) error {
	if err := s.write.AddProfessionalProfileMember(ctx, p); err != nil {
		return fmt.Errorf("adding professional profile member: %w", err)
	}

	return nil
}

// Reserved for future features.
func (s *Store) removeProfessionalProfileMember(ctx context.Context, profileID, userID pgtype.UUID) error {
	if err := s.write.RemoveProfessionalProfileMember(ctx, gen.RemoveProfessionalProfileMemberParams{ProfileID: profileID, UserID: userID}); err != nil {
		return fmt.Errorf("removing professional profile member: %w", err)
	}

	return nil
}

// Reserved for future features.
func (s *Store) listProfessionalProfileMembers(ctx context.Context, profileID, tenantID pgtype.UUID) ([]gen.ListProfessionalProfileMembersRow, error) {
	rows, err := s.read.ListProfessionalProfileMembers(ctx, gen.ListProfessionalProfileMembersParams{
		ProfileID: profileID,
		TenantID:  tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing professional profile members: %w", err)
	}

	return rows, nil
}

// Reserved for future features.
func (s *Store) updateProfessionalProfileMemberRole(ctx context.Context, profileID, userID pgtype.UUID, role string) error {
	if err := s.write.UpdateProfessionalProfileMemberRole(ctx, gen.UpdateProfessionalProfileMemberRoleParams{
		ProfileID: profileID,
		UserID:    userID,
		Role:      role,
	}); err != nil {
		return fmt.Errorf("updating professional profile member role: %w", err)
	}

	return nil
}

// Reserved for future features.
func (s *Store) getProfessionalProfileMember(ctx context.Context, profileID, userID pgtype.UUID) (gen.ProfessionalProfileMember, error) {
	row, err := s.read.GetProfessionalProfileMember(ctx, gen.GetProfessionalProfileMemberParams{ProfileID: profileID, UserID: userID})
	if err != nil {
		return gen.ProfessionalProfileMember{}, fmt.Errorf("getting professional profile member: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) isProfessionalProfileMember(ctx context.Context, profileID, userID pgtype.UUID) (bool, error) {
	exists, err := s.read.IsProfessionalProfileMember(ctx, gen.IsProfessionalProfileMemberParams{ProfileID: profileID, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("checking professional profile membership: %w", err)
	}

	return exists, nil
}

// Reserved for future features.
func (s *Store) isProfessionalProfileOwner(ctx context.Context, profileID, userID, tenantID pgtype.UUID) (bool, error) {
	exists, err := s.read.IsProfessionalProfileOwner(ctx, gen.IsProfessionalProfileOwnerParams{
		ProfileID: profileID,
		UserID:    userID,
		TenantID:  tenantID,
	})
	if err != nil {
		return false, fmt.Errorf("checking professional profile ownership: %w", err)
	}

	return exists, nil
}

// ---------------------------------------------------------------------------
// Professional Profile Invitations
// ---------------------------------------------------------------------------

// Reserved for future features.
func (s *Store) createProfessionalProfileInvitation(ctx context.Context, p gen.CreateProfessionalProfileInvitationParams) (gen.ProfessionalProfileInvitation, error) {
	row, err := s.write.CreateProfessionalProfileInvitation(ctx, p)
	if err != nil {
		return gen.ProfessionalProfileInvitation{}, fmt.Errorf("creating professional profile invitation: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) getProfessionalProfileInvitationByToken(ctx context.Context, token string) (gen.ProfessionalProfileInvitation, error) {
	row, err := s.read.GetProfessionalProfileInvitationByToken(ctx, token)
	if err != nil {
		return gen.ProfessionalProfileInvitation{}, fmt.Errorf("getting professional profile invitation by token: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) acceptProfessionalProfileInvitation(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.AcceptProfessionalProfileInvitation(ctx, id); err != nil {
		return fmt.Errorf("accepting professional profile invitation: %w", err)
	}

	return nil
}

// Reserved for future features.
func (s *Store) listProfessionalProfileInvitations(ctx context.Context, profileID pgtype.UUID) ([]gen.ProfessionalProfileInvitation, error) {
	rows, err := s.read.ListProfessionalProfileInvitations(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("listing professional profile invitations: %w", err)
	}

	return rows, nil
}

// Reserved for future features.
func (s *Store) deleteExpiredInvitations(ctx context.Context) error {
	if err := s.write.DeleteExpiredInvitations(ctx); err != nil {
		return fmt.Errorf("deleting expired invitations: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Professional Profile Relocations
// ---------------------------------------------------------------------------

// Reserved for future features.
func (s *Store) createProfessionalRelocation(ctx context.Context, p gen.CreateProfessionalRelocationParams) (gen.ProfessionalRelocation, error) {
	row, err := s.write.CreateProfessionalRelocation(ctx, p)
	if err != nil {
		return gen.ProfessionalRelocation{}, fmt.Errorf("creating professional relocation: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) listProfessionalRelocationsByProfile(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalRelocation, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListProfessionalRelocationsByProfile(ctx, gen.ListProfessionalRelocationsByProfileParams{
		ProfileID:  profileID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing professional relocations: %w", err)
	}

	return rows, nil
}

// Reserved for future features.
func (s *Store) updateProfessionalRelocation(ctx context.Context, p gen.UpdateProfessionalRelocationParams) (gen.ProfessionalRelocation, error) {
	row, err := s.write.UpdateProfessionalRelocation(ctx, p)
	if err != nil {
		return gen.ProfessionalRelocation{}, fmt.Errorf("updating professional relocation: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) deleteProfessionalRelocation(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteProfessionalRelocation(ctx, gen.DeleteProfessionalRelocationParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting professional relocation: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Professional Profile Documents
// ---------------------------------------------------------------------------

// Reserved for future features.
func (s *Store) createProfessionalDocument(ctx context.Context, p gen.CreateProfessionalDocumentParams) (gen.ProfessionalDocument, error) {
	row, err := s.write.CreateProfessionalDocument(ctx, p)
	if err != nil {
		return gen.ProfessionalDocument{}, fmt.Errorf("creating professional document: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) listProfessionalDocumentsByProfile(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ProfessionalDocument, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListProfessionalDocumentsByProfile(ctx, gen.ListProfessionalDocumentsByProfileParams{
		ProfileID:  profileID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing professional documents: %w", err)
	}

	return rows, nil
}

// Reserved for future features.
func (s *Store) updateProfessionalDocument(ctx context.Context, p gen.UpdateProfessionalDocumentParams) (gen.ProfessionalDocument, error) {
	row, err := s.write.UpdateProfessionalDocument(ctx, p)
	if err != nil {
		return gen.ProfessionalDocument{}, fmt.Errorf("updating professional document: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) deleteProfessionalDocument(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteProfessionalDocument(ctx, gen.DeleteProfessionalDocumentParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting professional document: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Professional Profile Verifiers
// ---------------------------------------------------------------------------

// Reserved for future features.
func (s *Store) createProfessionalVerifier(ctx context.Context, p gen.CreateProfessionalVerifierParams) (gen.ProfessionalVerifier, error) {
	row, err := s.write.CreateProfessionalVerifier(ctx, p)
	if err != nil {
		return gen.ProfessionalVerifier{}, fmt.Errorf("creating professional verifier: %w", err)
	}

	return row, nil
}

// Reserved for future features.
func (s *Store) listProfessionalVerifiersByProfile(ctx context.Context, profileID, tenantID pgtype.UUID, limit, offset int32) ([]gen.ListProfessionalVerifiersByProfileRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListProfessionalVerifiersByProfile(ctx, gen.ListProfessionalVerifiersByProfileParams{
		ProfileID:  profileID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing professional verifiers: %w", err)
	}

	return rows, nil
}

// Reserved for future features.
func (s *Store) deleteProfessionalVerifier(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteProfessionalVerifier(ctx, gen.DeleteProfessionalVerifierParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting professional verifier: %w", err)
	}

	return nil
}

// getProfessionalVerifierByID returns a verifier by ID, scoped to tenant.
func (s *Store) getProfessionalVerifierByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ProfessionalVerifier, error) {
	row, err := s.read.GetProfessionalVerifierByID(ctx, gen.GetProfessionalVerifierByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ProfessionalVerifier{}, fmt.Errorf("getting professional verifier by ID: %w", err)
	}

	return row, nil
}

// ---------------------------------------------------------------------------
// Parent profile lookups for child resources (ownership checks)
// ---------------------------------------------------------------------------

// getRelocationProfileID returns the profile_id that owns the given relocation.
//
// Reserved for future features.
func (s *Store) getRelocationProfileID(ctx context.Context, relocationID, tenantID pgtype.UUID) (pgtype.UUID, error) {
	row, err := s.read.GetProfessionalRelocationByID(ctx, gen.GetProfessionalRelocationByIDParams{ID: relocationID, TenantID: tenantID})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting relocation profile ID: %w", err)
	}

	return row.ProfileID, nil
}

// getDocumentProfileID returns the profile_id that owns the given document.
//
// Reserved for future features.
func (s *Store) getDocumentProfileID(ctx context.Context, documentID, tenantID pgtype.UUID) (pgtype.UUID, error) {
	row, err := s.read.GetProfessionalDocumentByID(ctx, gen.GetProfessionalDocumentByIDParams{ID: documentID, TenantID: tenantID})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting document profile ID: %w", err)
	}

	return row.ProfileID, nil
}

func (s *Store) listProfileContent(ctx context.Context, tenantID, profileID pgtype.UUID, limit, offset int32) ([]gen.ListContentsByAuthorProfileRow, error) {
	rows, err := s.read.ListContentsByAuthorProfile(ctx, gen.ListContentsByAuthorProfileParams{
		AuthorProfileID: profileID,
		TenantID:        tenantID,
		PageLimit:       limit,
		PageOffset:      offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing profile content: %w", err)
	}

	return rows, nil
}
