package spaces

import (
	"context"
	"fmt"

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
