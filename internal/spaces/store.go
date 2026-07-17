package spaces

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Permission Presets
// ---------------------------------------------------------------------------

// ListPermissionPresets returns every permission preset owned by the given
// tenant, oldest first.
func (s *Service) ListPermissionPresets(ctx context.Context, tenantID pgtype.UUID) ([]gen.SpacePermissionPreset, error) {
	presets, err := s.read.ListPermissionPresetsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing permission presets: %w", err)
	}

	return presets, nil
}

// GetPermissionPreset returns the permission preset matching the given name,
// scoped to the tenant.
func (s *Service) GetPermissionPreset(ctx context.Context, name string, tenantID pgtype.UUID) (gen.SpacePermissionPreset, error) {
	preset, err := s.read.GetPermissionPresetByName(ctx, gen.GetPermissionPresetByNameParams{
		Name:     name,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.SpacePermissionPreset{}, fmt.Errorf("getting permission preset: %w", err)
	}

	return preset, nil
}

// UpdatePermissionPreset updates the five permission axes of a system preset,
// scoped to the tenant.
func (s *Service) UpdatePermissionPreset(ctx context.Context, params gen.UpdatePermissionPresetParams) (gen.SpacePermissionPreset, error) {
	preset, err := s.write.UpdatePermissionPreset(ctx, params)
	if err != nil {
		return gen.SpacePermissionPreset{}, fmt.Errorf("updating permission preset: %w", err)
	}

	return preset, nil
}

// ---------------------------------------------------------------------------
// Space Types
// ---------------------------------------------------------------------------

// ListSpaceTypes returns every space type owned by the given tenant,
// oldest first.
func (s *Service) ListSpaceTypes(ctx context.Context, tenantID pgtype.UUID) ([]gen.SpaceType, error) {
	types, err := s.read.ListSpaceTypesByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing space types: %w", err)
	}

	return types, nil
}

// GetSpaceType returns the space type with the given ID, scoped to the
// tenant.
func (s *Service) GetSpaceType(ctx context.Context, id, tenantID pgtype.UUID) (gen.SpaceType, error) {
	st, err := s.read.GetSpaceTypeByID(ctx, gen.GetSpaceTypeByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.SpaceType{}, fmt.Errorf("getting space type: %w", err)
	}

	return st, nil
}

// GetSpaceTypeBySlug returns the space type matching the given slug, scoped
// to the tenant.
func (s *Service) GetSpaceTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.SpaceType, error) {
	st, err := s.read.GetSpaceTypeBySlug(ctx, gen.GetSpaceTypeBySlugParams{
		Slug:     slug,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.SpaceType{}, fmt.Errorf("getting space type by slug: %w", err)
	}

	return st, nil
}

// CreateSpaceType creates a new space type. Tenant-created types are never
// system types; set IsSystem to false from API handlers.
func (s *Service) CreateSpaceType(ctx context.Context, params gen.CreateSpaceTypeParams) (gen.SpaceType, error) {
	st, err := s.write.CreateSpaceType(ctx, params)
	if err != nil {
		return gen.SpaceType{}, fmt.Errorf("creating space type: %w", err)
	}

	return st, nil
}

// UpdateSpaceType updates the mutable fields of a non-system space type,
// scoped to the tenant. System types are protected at the database level.
func (s *Service) UpdateSpaceType(ctx context.Context, params gen.UpdateSpaceTypeParams) (gen.SpaceType, error) {
	st, err := s.write.UpdateSpaceType(ctx, params)
	if err != nil {
		return gen.SpaceType{}, fmt.Errorf("updating space type: %w", err)
	}

	return st, nil
}

// DeleteSpaceType deletes a non-system space type, scoped to the tenant.
// System types are protected at the database level.
func (s *Service) DeleteSpaceType(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteSpaceType(ctx, gen.DeleteSpaceTypeParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting space type: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Spaces
// ---------------------------------------------------------------------------

// ListSpaces returns every space owned by the given tenant, newest first.
func (s *Service) ListSpaces(ctx context.Context, tenantID pgtype.UUID) ([]gen.Space, error) {
	spaces, err := s.read.ListSpacesByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing spaces: %w", err)
	}

	return spaces, nil
}

// ListSpacesByMember returns every space the given user belongs to, most
// recently joined first (catalog: GET /me/spaces).
func (s *Service) ListSpacesByMember(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.Space, error) {
	spaces, err := s.read.ListSpacesByMemberUser(ctx, gen.ListSpacesByMemberUserParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by member: %w", err)
	}

	return spaces, nil
}

// ListSpacesByModerator returns every space the given user moderates
// (catalog: GET /me/spaces/moderated).
func (s *Service) ListSpacesByModerator(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.Space, error) {
	spaces, err := s.read.ListSpacesByModeratorUser(ctx, gen.ListSpacesByModeratorUserParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by moderator: %w", err)
	}

	return spaces, nil
}

// ListStatesByCountryCode returns the state/province spaces parented to the
// country with the given ISO geo_code (catalog: GET /geo/countries/:code/states).
func (s *Service) ListStatesByCountryCode(ctx context.Context, tenantID pgtype.UUID, countryCode string) ([]gen.Space, error) {
	states, err := s.read.ListStatesByCountryGeoCode(ctx, gen.ListStatesByCountryGeoCodeParams{
		TenantID:    tenantID,
		CountryCode: &countryCode,
	})
	if err != nil {
		return nil, fmt.Errorf("listing states by country code: %w", err)
	}

	return states, nil
}

// ListCitiesByState returns the city spaces parented to the given state
// (catalog: GET /geo/states/:id/cities).
func (s *Service) ListCitiesByState(ctx context.Context, tenantID, stateID pgtype.UUID) ([]gen.Space, error) {
	cities, err := s.read.ListCitiesByStateID(ctx, gen.ListCitiesByStateIDParams{
		StateID:  stateID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing cities by state: %w", err)
	}

	return cities, nil
}

// ListSpacesByLevel returns every space at the given hierarchy level for the
// tenant, newest first.
func (s *Service) ListSpacesByLevel(ctx context.Context, tenantID pgtype.UUID, level string) ([]gen.Space, error) {
	spaces, err := s.read.ListSpacesByLevel(ctx, gen.ListSpacesByLevelParams{
		TenantID: tenantID,
		Level:    level,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by level: %w", err)
	}

	return spaces, nil
}

// ListSpacesByType returns every space of the given type for the tenant,
// newest first.
func (s *Service) ListSpacesByType(ctx context.Context, tenantID, spaceTypeID pgtype.UUID) ([]gen.Space, error) {
	spaces, err := s.read.ListSpacesByType(ctx, gen.ListSpacesByTypeParams{
		TenantID:    tenantID,
		SpaceTypeID: spaceTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by type: %w", err)
	}

	return spaces, nil
}

// ListChildSpaces returns every space whose parent is the given space,
// scoped to the tenant, oldest first.
func (s *Service) ListChildSpaces(ctx context.Context, tenantID, parentID pgtype.UUID) ([]gen.Space, error) {
	spaces, err := s.read.ListChildSpaces(ctx, gen.ListChildSpacesParams{
		ParentID: parentID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing child spaces: %w", err)
	}

	return spaces, nil
}

// GetSpace returns the space with the given ID, scoped to the tenant.
func (s *Service) GetSpace(ctx context.Context, id, tenantID pgtype.UUID) (gen.Space, error) {
	space, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.Space{}, fmt.Errorf("getting space: %w", err)
	}

	return space, nil
}

// GetSpaceBySlug returns the space matching the given slug, scoped to the
// tenant.
func (s *Service) GetSpaceBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.Space, error) {
	space, err := s.read.GetSpaceBySlug(ctx, gen.GetSpaceBySlugParams{
		Slug:     slug,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.Space{}, fmt.Errorf("getting space by slug: %w", err)
	}

	return space, nil
}

// CreateSpace creates a new space. The caller is responsible for validating
// the level and permission axes before calling this method.
func (s *Service) CreateSpace(ctx context.Context, params gen.CreateSpaceParams) (gen.Space, error) {
	space, err := s.write.CreateSpace(ctx, params)
	if err != nil {
		return gen.Space{}, fmt.Errorf("creating space: %w", err)
	}

	return space, nil
}

// UpdateSpace updates the mutable fields of a space, scoped to the tenant.
func (s *Service) UpdateSpace(ctx context.Context, params gen.UpdateSpaceParams) (gen.Space, error) {
	space, err := s.write.UpdateSpace(ctx, params)
	if err != nil {
		return gen.Space{}, fmt.Errorf("updating space: %w", err)
	}

	return space, nil
}

// UpdateSpaceWikiContent updates only the wiki_content field of a space.
func (s *Service) UpdateSpaceWikiContent(ctx context.Context, spaceID, tenantID pgtype.UUID, wikiContent []byte) (gen.Space, error) {
	// Load current space to preserve all other fields.
	space, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{ID: spaceID, TenantID: tenantID})
	if err != nil {
		return gen.Space{}, fmt.Errorf("getting space for wiki update: %w", err)
	}

	space.WikiContent = wikiContent
	updated, err := s.write.UpdateSpace(ctx, gen.UpdateSpaceParams{
		Name:           space.Name,
		Description:    space.Description,
		WikiContent:    wikiContent,
		GeoCode:        space.GeoCode,
		Visibility:     space.Visibility,
		PostingPerm:    space.PostingPerm,
		ReplyingPerm:   space.ReplyingPerm,
		CommentingPerm: space.CommentingPerm,
		JoiningPerm:    space.JoiningPerm,
		Status:         space.Status,
		ID:             spaceID,
		TenantID:       tenantID,
	})
	if err != nil {
		return gen.Space{}, fmt.Errorf("updating wiki content: %w", err)
	}

	return updated, nil
}

// DeleteSpace deletes a space, scoped to the tenant.
func (s *Service) DeleteSpace(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteSpace(ctx, gen.DeleteSpaceParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting space: %w", err)
	}

	return nil
}

// IncrementSpaceCount adjusts the member count of a space by delta (positive
// or negative), scoped to the tenant.
func (s *Service) IncrementSpaceCount(ctx context.Context, id, tenantID pgtype.UUID, delta int32) error {
	if err := s.write.IncrementSpaceCount(ctx, gen.IncrementSpaceCountParams{
		Delta:    delta,
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("incrementing space count: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Space Rules
// ---------------------------------------------------------------------------

// ListSpaceRules returns every rule belonging to the given space, ordered by
// position then creation time.
func (s *Service) ListSpaceRules(ctx context.Context, spaceID pgtype.UUID) ([]gen.SpaceRule, error) {
	rules, err := s.read.ListSpaceRulesBySpace(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("listing space rules: %w", err)
	}

	return rules, nil
}

// CreateSpaceRule adds a new rule to a space.
func (s *Service) CreateSpaceRule(ctx context.Context, params gen.CreateSpaceRuleParams) (gen.SpaceRule, error) {
	rule, err := s.write.CreateSpaceRule(ctx, params)
	if err != nil {
		return gen.SpaceRule{}, fmt.Errorf("creating space rule: %w", err)
	}

	return rule, nil
}

// UpdateSpaceRule updates the title, body, and position of a rule, identified
// by both rule ID and space ID.
func (s *Service) UpdateSpaceRule(ctx context.Context, params gen.UpdateSpaceRuleParams) (gen.SpaceRule, error) {
	rule, err := s.write.UpdateSpaceRule(ctx, params)
	if err != nil {
		return gen.SpaceRule{}, fmt.Errorf("updating space rule: %w", err)
	}

	return rule, nil
}

// DeleteSpaceRule removes a rule from a space, identified by both rule ID
// and space ID.
func (s *Service) DeleteSpaceRule(ctx context.Context, id, spaceID pgtype.UUID) error {
	if err := s.write.DeleteSpaceRule(ctx, gen.DeleteSpaceRuleParams{
		ID:      id,
		SpaceID: spaceID,
	}); err != nil {
		return fmt.Errorf("deleting space rule: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Space Members
// ---------------------------------------------------------------------------

// IsMember returns true if the user is a member of the space.
func (s *Service) IsMember(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error) {
	isMember, err := s.read.CheckSpaceMembership(ctx, gen.CheckSpaceMembershipParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return false, fmt.Errorf("checking space membership: %w", err)
	}

	return isMember, nil
}

// Join adds the user as a member of the space.
func (s *Service) Join(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) error {
	if err := s.write.AddSpaceMember(ctx, gen.AddSpaceMemberParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("adding space member: %w", err)
	}

	return nil
}

// Leave removes the user from the space membership.
func (s *Service) Leave(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) error {
	if err := s.write.RemoveSpaceMember(ctx, gen.RemoveSpaceMemberParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("removing space member: %w", err)
	}

	return nil
}

// CountMembers returns the number of members in the space.
func (s *Service) CountMembers(ctx context.Context, spaceID, tenantID pgtype.UUID) (int64, error) {
	count, err := s.read.CountSpaceMembers(ctx, gen.CountSpaceMembersParams{
		SpaceID:  spaceID,
		TenantID: tenantID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting space members: %w", err)
	}

	return count, nil
}

// SpaceMemberRow represents a space member with user profile information.
type SpaceMemberRow struct {
	UserID      pgtype.UUID        `json:"user_id"`
	Username    string             `json:"username"`
	DisplayName *string            `json:"display_name"`
	AvatarURL   *string            `json:"avatar_url"`
	JoinedAt    pgtype.Timestamptz `json:"joined_at"`
}

// ListMembers returns paginated members of a space with user profile data.
func (s *Service) ListMembers(ctx context.Context, spaceID, tenantID pgtype.UUID, limit, offset int32) ([]SpaceMemberRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.read.ListSpaceMembersWithProfile(ctx, gen.ListSpaceMembersWithProfileParams{
		SpaceID:    spaceID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing space members: %w", err)
	}

	members := make([]SpaceMemberRow, 0, len(rows))

	for _, r := range rows {
		members = append(members, SpaceMemberRow{
			UserID:      r.UserID,
			Username:    r.Username,
			DisplayName: r.DisplayName,
			AvatarURL:   r.AvatarUrl,
			JoinedAt:    r.JoinedAt,
		})
	}

	return members, nil
}

// ---------------------------------------------------------------------------
// Space Moderators
// ---------------------------------------------------------------------------

// AssignModerator adds a user as a moderator of the space. Duplicate
// assignments are silently ignored via ON CONFLICT DO NOTHING.
func (s *Service) AssignModerator(ctx context.Context, params gen.AssignSpaceModeratorParams) error {
	if err := s.write.AssignSpaceModerator(ctx, params); err != nil {
		return fmt.Errorf("assigning space moderator: %w", err)
	}

	return nil
}

// RemoveModerator removes a user from the space moderator list.
func (s *Service) RemoveModerator(ctx context.Context, spaceID, userID pgtype.UUID) error {
	if err := s.write.RemoveSpaceModerator(ctx, gen.RemoveSpaceModeratorParams{
		SpaceID: spaceID,
		UserID:  userID,
	}); err != nil {
		return fmt.Errorf("removing space moderator: %w", err)
	}

	return nil
}

// GetUserIDByUsername resolves a username to a user ID within the tenant.
func (s *Service) GetUserIDByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := s.db.Reader.QueryRow(ctx,
		`SELECT id FROM users WHERE username = $1 AND tenant_id = $2`, username, tenantID,
	).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return id, fmt.Errorf("user not found")
		}
		return id, fmt.Errorf("resolving username: %w", err)
	}
	return id, nil
}

// GetSpaceByID returns a space by its ID within the tenant.
func (s *Service) GetSpaceByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.Space, error) {
	return s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{TenantID: tenantID, ID: id})
}

// ListModerators returns every moderator assigned to the given space,
// including their username and display name.
func (s *Service) ListModerators(ctx context.Context, spaceID pgtype.UUID) ([]gen.ListSpaceModeratorsRow, error) {
	mods, err := s.read.ListSpaceModerators(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("listing space moderators: %w", err)
	}

	return mods, nil
}

// IsModerator returns true if the user is a moderator of the space.
func (s *Service) IsModerator(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error) {
	_, err := s.read.GetSpaceModerator(ctx, gen.GetSpaceModeratorParams{
		SpaceID: spaceID,
		UserID:  userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("checking space moderator: %w", err)
	}

	return true, nil
}

// ---------------------------------------------------------------------------
// Space Parent Spaces (Many-to-Many Hierarchy)
// ---------------------------------------------------------------------------

// AddParentSpace links a space to a parent space via the junction table.
// The self-reference check is enforced at the database level (CHECK constraint).
func (s *Service) AddParentSpace(ctx context.Context, spaceID, parentSpaceID pgtype.UUID) error {
	if err := s.write.AddParentSpace(ctx, gen.AddParentSpaceParams{
		SpaceID:       spaceID,
		ParentSpaceID: parentSpaceID,
	}); err != nil {
		return fmt.Errorf("adding parent space: %w", err)
	}

	return nil
}

// RemoveParentSpace unlinks a space from a parent space.
func (s *Service) RemoveParentSpace(ctx context.Context, spaceID, parentSpaceID pgtype.UUID) error {
	if err := s.write.RemoveParentSpace(ctx, gen.RemoveParentSpaceParams{
		SpaceID:       spaceID,
		ParentSpaceID: parentSpaceID,
	}); err != nil {
		return fmt.Errorf("removing parent space: %w", err)
	}

	return nil
}

// GetParentSpaces returns every parent space for the given space, resolved
// through the junction table and scoped to the tenant.
func (s *Service) GetParentSpaces(ctx context.Context, spaceID, tenantID pgtype.UUID) ([]gen.Space, error) {
	spaces, err := s.read.GetParentSpacesBySpaceID(ctx, gen.GetParentSpacesBySpaceIDParams{
		SpaceID:  spaceID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("querying parent spaces: %w", err)
	}

	return spaces, nil
}

// GetChildSpaces returns every child space for the given space, resolved
// through the junction table and scoped to the tenant.
func (s *Service) GetChildSpaces(ctx context.Context, spaceID, tenantID pgtype.UUID) ([]gen.Space, error) {
	spaces, err := s.read.GetChildSpacesByParentID(ctx, gen.GetChildSpacesByParentIDParams{
		ParentSpaceID: spaceID,
		TenantID:      tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("querying child spaces: %w", err)
	}

	return spaces, nil
}
