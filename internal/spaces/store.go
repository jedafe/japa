package spaces

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
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

// ListSpaceTypesFiltered returns space types for the given tenant, optionally
// filtered by a search query (matched against name and slug), a category
// filter, and a selectable-only flag. Pass empty strings and false booleans
// to skip filters.
func (s *Service) ListSpaceTypesFiltered(ctx context.Context, tenantID pgtype.UUID, search, categoryFilter string, selectableOnly bool) ([]gen.SpaceType, error) {
	types, err := s.read.ListSpaceTypesByTenantFiltered(ctx, gen.ListSpaceTypesByTenantFilteredParams{
		TenantID:       tenantID,
		Search:         search,
		CategoryFilter: categoryFilter,
		SelectableOnly: selectableOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("listing space types (filtered): %w", err)
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
// The viewerUserID filters by visibility: unauthenticated callers (zero UUID)
// only see public spaces; authenticated callers also see members/private
// spaces they belong to (Section 05 line 272, §9 multi-tenancy, BUG-007).
func (s *Service) ListSpaces(ctx context.Context, tenantID, viewerUserID pgtype.UUID) ([]gen.ListSpacesByTenantRow, error) {
	spaces, err := s.read.ListSpacesByTenant(ctx, gen.ListSpacesByTenantParams{
		TenantID:     tenantID,
		ViewerUserID: viewerUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces: %w", err)
	}

	return spaces, nil
}

// ListSpacesPaginated returns a page of spaces with SQL-level search, geo,
// level, sort, and visibility filtering (SPACES-009/011). Replaces the
// in-memory filter+paginate in the ListSpaces handler for the default
// (no type/parent filter) branch.
func (s *Service) ListSpacesPaginated(ctx context.Context, tenantID, viewerUserID pgtype.UUID, search, geoCode, level, sort string, limit, offset int32) ([]gen.ListSpacesPaginatedRow, error) {
	spaces, err := s.read.ListSpacesPaginated(ctx, gen.ListSpacesPaginatedParams{
		TenantID:     tenantID,
		ViewerUserID: viewerUserID,
		Search:       search,
		GeoCode:      geoCode,
		Level:        level,
		Sort:         sort,
		LimitVal:     limit,
		OffsetVal:    offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces paginated: %w", err)
	}
	return spaces, nil
}

// CountSpaces returns the total count of spaces matching the paginated
// discovery filters (SPACES-009).
func (s *Service) CountSpaces(ctx context.Context, tenantID, viewerUserID pgtype.UUID, search, geoCode, level string) (int64, error) {
	total, err := s.read.CountSpaces(ctx, gen.CountSpacesParams{
		TenantID:     tenantID,
		ViewerUserID: viewerUserID,
		Search:       search,
		GeoCode:      geoCode,
		Level:        level,
	})
	if err != nil {
		return 0, fmt.Errorf("counting spaces: %w", err)
	}
	return total, nil
}

// AdminListSpaces returns paginated spaces for the admin view with owner and
// space_type name resolved via JOINs. Search, status, and visibility filters
// are applied at the SQL level — pass "" for no filter.
func (s *Service) AdminListSpaces(ctx context.Context, tenantID pgtype.UUID, search, status, visibility string, limit, offset int32) ([]gen.AdminListSpacesRow, error) {
	rows, err := s.read.AdminListSpaces(ctx, gen.AdminListSpacesParams{
		TenantID:   tenantID,
		Search:     search,
		Status:     status,
		Visibility: visibility,
		LimitVal:   limit,
		OffsetVal:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("admin listing spaces: %w", err)
	}
	return rows, nil
}

// AdminListSpacesCount returns the total number of spaces for the tenant,
// respecting the same search/status/visibility filters as AdminListSpaces.
func (s *Service) AdminListSpacesCount(ctx context.Context, tenantID pgtype.UUID, search, status, visibility string) (int64, error) {
	count, err := s.read.AdminListSpacesCount(ctx, gen.AdminListSpacesCountParams{
		TenantID:   tenantID,
		Search:     search,
		Status:     status,
		Visibility: visibility,
	})
	if err != nil {
		return 0, fmt.Errorf("admin counting spaces: %w", err)
	}
	return count, nil
}

// ListSpacesByMember returns every space the given user belongs to, most
// recently joined first (catalog: GET /me/spaces). The search parameter
// filters by name/slug (case-insensitive) at the SQL level — pass "" for
// no filter. limit/offset are applied at the SQL level to keep the payload
// small for the authenticated shell (most callers only need the first page).
func (s *Service) ListSpacesByMember(ctx context.Context, tenantID, userID pgtype.UUID, search string, limit, offset int32) ([]gen.ListSpacesByMemberUserRow, error) {
	spaces, err := s.read.ListSpacesByMemberUser(ctx, gen.ListSpacesByMemberUserParams{
		UserID:    userID,
		TenantID:  tenantID,
		Search:    search,
		LimitVal:  limit,
		OffsetVal: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by member: %w", err)
	}

	return spaces, nil
}

// ListAllSpacesForAdmin returns every active space in the tenant with no
// visibility or membership filter. Used by GET /me/spaces when the caller is
// a super_admin so they can post to any space without joining it first
// (Section 03 — admin override).
func (s *Service) ListAllSpacesForAdmin(ctx context.Context, tenantID pgtype.UUID, search string, limit, offset int32) ([]gen.ListAllSpacesForAdminRow, error) {
	spaces, err := s.read.ListAllSpacesForAdmin(ctx, gen.ListAllSpacesForAdminParams{
		TenantID:  tenantID,
		Search:    search,
		LimitVal:  limit,
		OffsetVal: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing all spaces for admin: %w", err)
	}
	return spaces, nil
}

// IsSuperAdmin reports whether the user belongs to the super_admin user group
// for the given tenant. Used to gate admin-only space listing.
func (s *Service) IsSuperAdmin(ctx context.Context, tenantID, userID pgtype.UUID) (bool, error) {
	return s.read.IsUserInGroup(ctx, gen.IsUserInGroupParams{
		TenantID:  tenantID,
		UserID:    userID,
		GroupSlug: "super_admin",
	})
}

// ListSpacesByModerator returns every space the given user moderates
// (catalog: GET /me/spaces/moderated).
func (s *Service) ListSpacesByModerator(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.ListSpacesByModeratorUserRow, error) {
	spaces, err := s.read.ListSpacesByModeratorUser(ctx, gen.ListSpacesByModeratorUserParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by moderator: %w", err)
	}

	return spaces, nil
}

// ListSpacesByOwner returns every space the given user owns
// (space_members.role = 'owner') (catalog: GET /me/spaces/owned).
func (s *Service) ListSpacesByOwner(ctx context.Context, tenantID, userID pgtype.UUID) ([]gen.ListSpacesByOwnerUserRow, error) {
	spaces, err := s.read.ListSpacesByOwnerUser(ctx, gen.ListSpacesByOwnerUserParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by owner: %w", err)
	}

	return spaces, nil
}

// CountSpacesByOwner returns the number of spaces the user owns, for the
// max_spaces_creatable quota check (Section 03, §26).
func (s *Service) CountSpacesByOwner(ctx context.Context, tenantID, userID pgtype.UUID) (int, error) {
	count, err := s.read.CountSpacesByOwnerUser(ctx, gen.CountSpacesByOwnerUserParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting spaces by owner: %w", err)
	}
	return int(count), nil
}

// GetUserMaxSpacesCreatable returns the most permissive max_spaces_creatable
// from the user's group memberships. -1 = unlimited, 0 = none, N = capped.
func (s *Service) GetUserMaxSpacesCreatable(ctx context.Context, tenantID, userID pgtype.UUID) (int, error) {
	max, err := s.read.GetUserMaxSpacesCreatable(ctx, gen.GetUserMaxSpacesCreatableParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return 0, fmt.Errorf("getting max_spaces_creatable: %w", err)
	}
	return int(max), nil
}

// ListPublicByMemberUser returns the PUBLIC, active spaces the given user is
// a member of, paginated. Powers the public profile Spaces tab — excludes
// members-only/private and non-active spaces.
func (s *Service) ListPublicByMemberUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.ListPublicSpacesByMemberUserRow, error) {
	spaces, err := s.read.ListPublicSpacesByMemberUser(ctx, gen.ListPublicSpacesByMemberUserParams{
		UserID:     userID,
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing public spaces by member: %w", err)
	}

	return spaces, nil
}

// ListStatesByCountryCode returns the state/province spaces parented to the
// country with the given ISO geo_code (catalog: GET /geo/countries/:code/states).
func (s *Service) ListStatesByCountryCode(ctx context.Context, tenantID pgtype.UUID, countryCode string) ([]gen.ListStatesByCountryGeoCodeRow, error) {
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
func (s *Service) ListCitiesByState(ctx context.Context, tenantID, stateID pgtype.UUID) ([]gen.ListCitiesByStateIDRow, error) {
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
// tenant, newest first. The viewerUserID filters by visibility: unauthenticated
// callers (zero UUID) only see public spaces; authenticated callers also see
// members/private spaces they belong to (SPACES-001, Section 05 line 272).
func (s *Service) ListSpacesByLevel(ctx context.Context, tenantID, viewerUserID pgtype.UUID, level string) ([]gen.ListSpacesByLevelRow, error) {
	spaces, err := s.read.ListSpacesByLevel(ctx, gen.ListSpacesByLevelParams{
		TenantID:     tenantID,
		Level:        level,
		ViewerUserID: viewerUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by level: %w", err)
	}

	return spaces, nil
}

// GetSpaceByGeoCode returns the country-level Space whose geo_code matches the
// given ISO-3166-1 alpha-2 country code, scoped to the tenant. Used by the
// /geo/me suggested_spaces algorithm (Section 37) and the onboarding primary
// space auto-join (Section 05, ADR-005).
func (s *Service) GetSpaceByGeoCode(ctx context.Context, tenantID pgtype.UUID, countryCode string) (gen.GetSpaceByGeoCodeRow, error) {
	space, err := s.read.GetSpaceByGeoCode(ctx, gen.GetSpaceByGeoCodeParams{
		GeoCode:  &countryCode,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetSpaceByGeoCodeRow{}, fmt.Errorf("getting space by geo code: %w", err)
	}

	return space, nil
}

// FindCitySpaceByName returns the city-level Space whose name matches the
// given city name (case-insensitive), scoped to the tenant. Used by the
// /geo/me suggested_spaces algorithm (Section 37) and the reverse-geocoding
// flow (ADR-064a).
func (s *Service) FindCitySpaceByName(ctx context.Context, tenantID pgtype.UUID, cityName string) (gen.FindCitySpaceByNameRow, error) {
	space, err := s.read.FindCitySpaceByName(ctx, gen.FindCitySpaceByNameParams{
		CityName: cityName,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.FindCitySpaceByNameRow{}, fmt.Errorf("finding city space by name: %w", err)
	}

	return space, nil
}

// ListSpacesByType returns every space of the given type for the tenant,
// newest first. The viewerUserID filters by visibility (SPACES-001).
func (s *Service) ListSpacesByType(ctx context.Context, tenantID, viewerUserID, spaceTypeID pgtype.UUID) ([]gen.ListSpacesByTypeRow, error) {
	spaces, err := s.read.ListSpacesByType(ctx, gen.ListSpacesByTypeParams{
		TenantID:     tenantID,
		SpaceTypeID:  spaceTypeID,
		ViewerUserID: viewerUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by type: %w", err)
	}

	return spaces, nil
}

// ListSpacesByTypePaginated returns a page of spaces of the given type,
// filtered by optional search and geo_code, with server-side pagination and
// optional sort (” = newest, 'name', 'members', 'followers', 'posts').
func (s *Service) ListSpacesByTypePaginated(ctx context.Context, tenantID, spaceTypeID pgtype.UUID, search, geoCode, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, error) {
	spaces, err := s.read.ListSpacesByTypePaginated(ctx, gen.ListSpacesByTypePaginatedParams{
		TenantID:    tenantID,
		SpaceTypeID: spaceTypeID,
		Search:      search,
		GeoCode:     geoCode,
		Sort:        sort,
		LimitVal:    limit,
		OffsetVal:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing spaces by type (paginated): %w", err)
	}
	return spaces, nil
}

// CountSpacesByType returns the total count of spaces of the given type,
// matching the same optional search and geo_code filters.
func (s *Service) CountSpacesByType(ctx context.Context, tenantID, spaceTypeID pgtype.UUID, search, geoCode string) (int64, error) {
	total, err := s.read.CountSpacesByType(ctx, gen.CountSpacesByTypeParams{
		TenantID:    tenantID,
		SpaceTypeID: spaceTypeID,
		Search:      search,
		GeoCode:     geoCode,
	})
	if err != nil {
		return 0, fmt.Errorf("counting spaces by type: %w", err)
	}
	return total, nil
}

// ListChildSpaces returns every space whose parent is the given space,
// scoped to the tenant, oldest first. The viewerUserID filters by
// visibility (SPACES-001).
func (s *Service) ListChildSpaces(ctx context.Context, tenantID, viewerUserID, parentID pgtype.UUID) ([]gen.ListChildSpacesRow, error) {
	spaces, err := s.read.ListChildSpaces(ctx, gen.ListChildSpacesParams{
		ParentID:     parentID,
		TenantID:     tenantID,
		ViewerUserID: viewerUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing child spaces: %w", err)
	}

	return spaces, nil
}

// GetSpace returns the space with the given ID, scoped to the tenant.
func (s *Service) GetSpace(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetSpaceByIDRow, error) {
	space, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetSpaceByIDRow{}, fmt.Errorf("getting space: %w", err)
	}

	return space, nil
}

// GetSpaceBySlug returns the space matching the given slug, scoped to the
// tenant.
func (s *Service) GetSpaceBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetSpaceBySlugRow, error) {
	space, err := s.read.GetSpaceBySlug(ctx, gen.GetSpaceBySlugParams{
		Slug:     slug,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetSpaceBySlugRow{}, fmt.Errorf("getting space by slug: %w", err)
	}

	return space, nil
}

// CreateSpace creates a new space. The caller is responsible for validating
// the level and permission axes before calling this method.
func (s *Service) CreateSpace(ctx context.Context, params gen.CreateSpaceParams) (gen.CreateSpaceRow, error) {
	space, err := s.write.CreateSpace(ctx, params)
	if err != nil {
		return gen.CreateSpaceRow{}, fmt.Errorf("creating space: %w", err)
	}

	return space, nil
}

// CreateSpaceWithOwner creates a space, adds the creator as owner, increments
// the member count, and links any primary parent spaces — all in a single
// transaction (BUG-001). If any step fails the entire operation rolls back,
// preventing orphaned spaces without owners.
//
// Ref: sections/05-geo-first-space-system.md lines 198-224 (moderator
// assignment requires creator ownership).
func (s *Service) CreateSpaceWithOwner(
	ctx context.Context,
	params gen.CreateSpaceParams,
	creatorUserID pgtype.UUID,
	parentSpaceIDs []pgtype.UUID,
) (gen.CreateSpaceRow, error) {
	return db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (gen.CreateSpaceRow, error) {
		space, err := q.CreateSpace(ctx, params)
		if err != nil {
			return gen.CreateSpaceRow{}, fmt.Errorf("creating space: %w", err)
		}

		if err := q.AddSpaceOwner(ctx, gen.AddSpaceOwnerParams{
			SpaceID:  space.ID,
			UserID:   creatorUserID,
			TenantID: params.TenantID,
		}); err != nil {
			return gen.CreateSpaceRow{}, fmt.Errorf("adding space owner: %w", err)
		}

		if err := q.IncrementSpaceCount(ctx, gen.IncrementSpaceCountParams{
			Delta:    1,
			ID:       space.ID,
			TenantID: params.TenantID,
		}); err != nil {
			return gen.CreateSpaceRow{}, fmt.Errorf("incrementing member count: %w", err)
		}

		for _, psid := range parentSpaceIDs {
			if err := q.AddParentSpace(ctx, gen.AddParentSpaceParams{
				SpaceID:       space.ID,
				ParentSpaceID: psid,
			}); err != nil {
				return gen.CreateSpaceRow{}, fmt.Errorf("linking parent space: %w", err)
			}
		}

		return space, nil
	})
}

var slugReplaceRe = regexp.MustCompile(`[^\w\s-]+`)
var slugSpaceRe = regexp.MustCompile(`[\s_]+`)
var slugHyphenRe = regexp.MustCompile(`-{2,}`)

// slugify converts a human-readable name to a URL-friendly slug.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugReplaceRe.ReplaceAllString(s, "")
	s = slugSpaceRe.ReplaceAllString(s, "-")
	s = slugHyphenRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// CreateCity creates a city-level geo space under the given state. It is
// intentionally separate from CreateSpaceWithOwner because adding a missing
// city to the geo catalog should not count against the user's
// max_spaces_creatable quota (the user does not own the city).
func (s *Service) CreateCity(ctx context.Context, tenantID, stateID pgtype.UUID, name string) (gen.CreateSpaceRow, error) {
	state, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{ID: stateID, TenantID: tenantID})
	if err != nil {
		return gen.CreateSpaceRow{}, fmt.Errorf("getting state: %w", err)
	}
	if state.Level != LevelState {
		return gen.CreateSpaceRow{}, errors.New("parent is not a state")
	}

	cityType, err := s.read.GetSpaceTypeBySlug(ctx, gen.GetSpaceTypeBySlugParams{
		Slug:     "city",
		TenantID: tenantID,
	})
	if err != nil {
		return gen.CreateSpaceRow{}, fmt.Errorf("getting city space type: %w", err)
	}

	axes := DefaultPermAxes()
	params := gen.CreateSpaceParams{
		TenantID:       tenantID,
		ParentID:       stateID,
		SpaceTypeID:    cityType.ID,
		Name:           strings.TrimSpace(name),
		Slug:           slugify(name),
		Level:          LevelCity,
		Visibility:     axes.Visibility,
		PostingPerm:    axes.Posting,
		ReplyingPerm:   axes.Replying,
		CommentingPerm: axes.Commenting,
		JoiningPerm:    axes.Joining,
		Status:         StatusActive,
	}

	return db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (gen.CreateSpaceRow, error) {
		space, err := q.CreateSpace(ctx, params)
		if err != nil {
			return gen.CreateSpaceRow{}, fmt.Errorf("creating city: %w", err)
		}

		if err := q.AddParentSpace(ctx, gen.AddParentSpaceParams{
			SpaceID:       space.ID,
			ParentSpaceID: stateID,
		}); err != nil {
			return gen.CreateSpaceRow{}, fmt.Errorf("linking city to state: %w", err)
		}

		return space, nil
	})
}

// LinkSpaceToPrimarySpaces keyword-matches the space name against public
// primary (country/state/city) spaces and returns the most specific matched
// space ID (city > state > country) plus all matched IDs. The matches are
// intended to populate the space_parent_spaces junction.
func (s *Service) LinkSpaceToPrimarySpaces(ctx context.Context, spaceID, tenantID pgtype.UUID, name string) (pgtype.UUID, []pgtype.UUID, error) {
	keywords := extractSpaceKeywords(name)
	if len(keywords) == 0 {
		return pgtype.UUID{}, nil, nil
	}

	matches, err := s.FindSpacesByKeywords(ctx, tenantID, keywords)
	if err != nil {
		return pgtype.UUID{}, nil, fmt.Errorf("finding primary spaces by keywords: %w", err)
	}
	if len(matches) == 0 {
		return pgtype.UUID{}, nil, nil
	}

	parentIDs := make([]pgtype.UUID, 0, len(matches))
	for _, m := range matches {
		parentIDs = append(parentIDs, m.ID)
	}

	return matches[0].ID, parentIDs, nil
}

// SetSpaceParent updates a space's parent_id and links it to the given primary
// parent spaces in a single transaction.
func (s *Service) SetSpaceParent(ctx context.Context, spaceID, tenantID, parentID pgtype.UUID, parentIDs []pgtype.UUID) error {
	return db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if err := q.UpdateSpaceParentID(ctx, gen.UpdateSpaceParentIDParams{
			ID:       spaceID,
			TenantID: tenantID,
			ParentID: parentID,
		}); err != nil {
			return fmt.Errorf("updating parent_id: %w", err)
		}
		for _, pid := range parentIDs {
			if err := q.AddParentSpace(ctx, gen.AddParentSpaceParams{
				SpaceID:       spaceID,
				ParentSpaceID: pid,
			}); err != nil {
				return fmt.Errorf("adding parent space: %w", err)
			}
		}
		return nil
	})
}

// UpdateSpace updates the mutable fields of a space, scoped to the tenant.
func (s *Service) UpdateSpace(ctx context.Context, params gen.UpdateSpaceParams) (gen.UpdateSpaceRow, error) {
	space, err := s.write.UpdateSpace(ctx, params)
	if err != nil {
		return gen.UpdateSpaceRow{}, fmt.Errorf("updating space: %w", err)
	}

	return space, nil
}

// UpdateSpaceSlug updates only the slug of a space, scoped to the tenant.
// The caller is responsible for mapping a unique-violation (pgcode 23505)
// to a 409. Ref: sections/39-space-types-university.md (admin slug rename).
func (s *Service) UpdateSpaceSlug(ctx context.Context, id, tenantID pgtype.UUID, slug string) (gen.UpdateSpaceSlugRow, error) {
	space, err := s.write.UpdateSpaceSlug(ctx, gen.UpdateSpaceSlugParams{
		ID:       id,
		TenantID: tenantID,
		Slug:     slug,
	})
	if err != nil {
		return gen.UpdateSpaceSlugRow{}, fmt.Errorf("updating space slug: %w", err)
	}
	return space, nil
}

// UpdateSpaceKeywords updates only the keywords array on a space.
// Ref: sections/05-geo-first-space-system.md
func (s *Service) UpdateSpaceKeywords(ctx context.Context, id, tenantID pgtype.UUID, keywords []string) error {
	if err := s.write.UpdateSpaceKeywords(ctx, gen.UpdateSpaceKeywordsParams{
		Keywords: keywords,
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("updating space keywords: %w", err)
	}
	return nil
}

// GetSpaceKeywords returns the keywords array for a space.
func (s *Service) GetSpaceKeywords(ctx context.Context, id, tenantID pgtype.UUID) ([]string, error) {
	return s.read.GetSpaceKeywords(ctx, gen.GetSpaceKeywordsParams{
		ID:       id,
		TenantID: tenantID,
	})
}

// UpdateSpaceWikiContent updates only the wiki_content field of a space.
func (s *Service) UpdateSpaceWikiContent(ctx context.Context, spaceID, tenantID pgtype.UUID, wikiContent []byte) (gen.UpdateSpaceRow, error) {
	// Load current space to preserve all other fields.
	space, err := s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{ID: spaceID, TenantID: tenantID})
	if err != nil {
		return gen.UpdateSpaceRow{}, fmt.Errorf("getting space for wiki update: %w", err)
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
		return gen.UpdateSpaceRow{}, fmt.Errorf("updating wiki content: %w", err)
	}

	return updated, nil
}

// UpdateSpaceLogoMediaID updates only the logo_media_id field of a space.
// Used by the university:fetch_logo task (Section 39) to attach a downloaded
// university logo to its space.
func (s *Service) UpdateSpaceLogoMediaID(ctx context.Context, params gen.UpdateSpaceLogoMediaIDParams) (gen.UpdateSpaceLogoMediaIDRow, error) {
	space, err := s.write.UpdateSpaceLogoMediaID(ctx, params)
	if err != nil {
		return gen.UpdateSpaceLogoMediaIDRow{}, fmt.Errorf("updating space logo_media_id: %w", err)
	}
	return space, nil
}

// UpdateSpaceHeaderImageID updates only the header_image_id field of a space.
// Used by the space settings UI to attach a banner image to a space.
func (s *Service) UpdateSpaceHeaderImageID(ctx context.Context, params gen.UpdateSpaceHeaderImageIDParams) (gen.UpdateSpaceHeaderImageIDRow, error) {
	space, err := s.write.UpdateSpaceHeaderImageID(ctx, params)
	if err != nil {
		return gen.UpdateSpaceHeaderImageIDRow{}, fmt.Errorf("updating space header_image_id: %w", err)
	}
	return space, nil
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

// IsOwner returns true if the user is the owner (role='owner') of the space.
func (s *Service) IsOwner(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error) {
	isOwner, err := s.read.CheckSpaceOwnership(ctx, gen.CheckSpaceOwnershipParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return false, fmt.Errorf("checking space ownership: %w", err)
	}

	return isOwner, nil
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

// JoinPending submits a pending join request for spaces with
// joining_perm='request' (Section 05 lines 358-360, BUG-004). The row is
// inserted with status='pending' and must be approved by an admin before the
// user is considered a member.
func (s *Service) JoinPending(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) error {
	if err := s.write.AddSpaceMemberPending(ctx, gen.AddSpaceMemberPendingParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("adding pending space member: %w", err)
	}

	return nil
}

// AddOwner inserts the creator as the space owner (role='owner') so they can
// exercise the space_owner moderator-assignment path (Section 05 lines
// 198-224, BUG-001).
func (s *Service) AddOwner(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) error {
	if err := s.write.AddSpaceOwner(ctx, gen.AddSpaceOwnerParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("adding space owner: %w", err)
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
// SPACES-015: tenant-scoped for defense-in-depth.
func (s *Service) RemoveModerator(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) error {
	if err := s.write.RemoveSpaceModerator(ctx, gen.RemoveSpaceModeratorParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("removing space moderator: %w", err)
	}

	return nil
}

// GetUserIDByUsername resolves a username to a user ID within the tenant.
// Uses the sqlc-generated GetUserByUsername query (Rule 2 — no raw SQL).
func (s *Service) GetUserIDByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (pgtype.UUID, error) {
	row, err := s.read.GetUserByUsername(ctx, gen.GetUserByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, fmt.Errorf("user not found")
		}
		return pgtype.UUID{}, fmt.Errorf("resolving username: %w", err)
	}
	// GetUserByUsername selects `COALESCE(up.id, gen_random_uuid()) AS id`
	// (the user_profiles row id — or a random UUID when no profile exists)
	// alongside `u.id AS user_id`. The caller needs the USERS table id, which
	// is row.UserID; returning row.ID here would insert a profile id (or a
	// non-existent random UUID) into space_moderators.user_id and trip the
	// FK-to-users(id) constraint.
	return row.UserID, nil
}

// GetSpaceByID returns a space by its ID within the tenant.
func (s *Service) GetSpaceByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.GetSpaceByIDRow, error) {
	return s.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{TenantID: tenantID, ID: id})
}

// ListModerators returns every moderator assigned to the given space,
// including their username and display name.
// SPACES-015: tenant-scoped for defense-in-depth.
func (s *Service) ListModerators(ctx context.Context, spaceID, tenantID pgtype.UUID) ([]gen.ListSpaceModeratorsRow, error) {
	mods, err := s.read.ListSpaceModerators(ctx, gen.ListSpaceModeratorsParams{
		SpaceID:  spaceID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing space moderators: %w", err)
	}

	return mods, nil
}

// IsModerator returns true if the user is a moderator of the space.
// SPACES-015: tenant-scoped for defense-in-depth.
func (s *Service) IsModerator(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error) {
	_, err := s.read.GetSpaceModerator(ctx, gen.GetSpaceModeratorParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("checking space moderator: %w", err)
	}

	return true, nil
}

// CanManageSpace returns true if the user may manage the space — i.e. is the
// space owner (role='owner' in space_members) or an assigned moderator
// (space_moderators). Used to gate the space analytics dashboard
// (GET /spaces/{id}/analytics, [auth, space-admin] per JEOS §215) so guests
// and non-admin members receive 403 analytics_forbidden.
func (s *Service) CanManageSpace(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error) {
	isOwner, err := s.read.CheckSpaceOwnership(ctx, gen.CheckSpaceOwnershipParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return false, fmt.Errorf("checking space ownership: %w", err)
	}
	if isOwner {
		return true, nil
	}
	isMod, err := s.IsModerator(ctx, spaceID, userID, tenantID)
	if err != nil {
		return false, err
	}
	return isMod, nil
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
func (s *Service) GetParentSpaces(ctx context.Context, spaceID, tenantID pgtype.UUID) ([]gen.GetParentSpacesBySpaceIDRow, error) {
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
func (s *Service) GetChildSpaces(ctx context.Context, spaceID, tenantID pgtype.UUID) ([]gen.GetChildSpacesByParentIDRow, error) {
	spaces, err := s.read.GetChildSpacesByParentID(ctx, gen.GetChildSpacesByParentIDParams{
		ParentSpaceID: spaceID,
		TenantID:      tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("querying child spaces: %w", err)
	}

	return spaces, nil
}

// ReorderPinned atomically updates the position of each pinned content in the
// given order. All updates run in a single transaction so a mid-loop failure
// rolls back the entire reorder (BUG-007 fix, §6.2 transaction boundary).
func (s *Service) ReorderPinned(ctx context.Context, spaceID pgtype.UUID, contentIDs []pgtype.UUID) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning reorder transaction: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after commit

	q := gen.New(tx)
	for i, contentID := range contentIDs {
		if err := q.UpdateSpacePinPosition(ctx, gen.UpdateSpacePinPositionParams{
			SpaceID:   spaceID,
			ContentID: contentID,
			Position:  int32(i),
		}); err != nil {
			return fmt.Errorf("updating pin position %d: %w", i, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing reorder: %w", err)
	}
	return nil
}

// SpaceAnalyticsSummary holds the three aggregate counts returned by
// GetSpaceAnalytics in a single round-trip (BUG-020).
type SpaceAnalyticsSummary struct {
	MemberCount  int
	ContentCount int
	ViewCount    int
}

// GetSpaceAnalyticsSummary returns member/content/view counts for a space
// in a single SQL query instead of three sequential ones (BUG-020).
func (s *Service) GetSpaceAnalyticsSummary(ctx context.Context, spaceID, tenantID pgtype.UUID) (SpaceAnalyticsSummary, error) {
	row, err := s.read.GetSpaceAnalyticsSummary(ctx, gen.GetSpaceAnalyticsSummaryParams{
		SpaceID:  spaceID,
		TenantID: tenantID,
	})
	if err != nil {
		return SpaceAnalyticsSummary{}, fmt.Errorf("getting space analytics summary: %w", err)
	}
	return SpaceAnalyticsSummary{
		MemberCount:  int(row.MemberCount),
		ContentCount: int(row.ContentCount),
		ViewCount:    int(row.ViewCount),
	}, nil
}
