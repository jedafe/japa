// Wiki types & category template administration (§80 Wiki Types &
// Field-Group Pages amendment, migration 000739). DTOs and service methods
// backing the /wiki/types and /admin/wiki/types[/categories] routes —
// a wiki type is a space_type with an enabled wiki_type_configs row, and
// each type's category template may bind a custom field group so every
// entity of the type shares the same page schema.
// // Ref: japaconnect-engineering/sections/80-destination-wiki.md
package wiki

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrWikiTypeInUse is returned when deleting a wiki type while spaces of
// the type still have active wiki pages.
var ErrWikiTypeInUse = errors.New("wiki type has active pages")

// ErrWikiCategoryProtected is returned when deleting/updating a system
// category row (seeded categories keep the baseline page schema stable).
var ErrWikiCategoryProtected = errors.New("wiki category is system-protected")

// ErrWikiCategoryInUse is returned when deleting a category whose pages
// carry approved prose.
var ErrWikiCategoryInUse = errors.New("wiki category has pages with content")

// ErrFieldGroupNotFound is returned when a category binds a
// custom_field_groups id that does not exist.
var ErrFieldGroupNotFound = errors.New("field group not found")

// ErrInvalidInput wraps request-body validation failures; handlers map it
// to 400 invalid_input.
var ErrInvalidInput = errors.New("invalid input")

// WikiTypeDTO is the public/admin representation of a wiki type.
// //nolint:revive // exported: DTO naming follows package convention
type WikiTypeDTO struct {
	SpaceTypeID   string   `json:"space_type_id"`
	SpaceTypeSlug string   `json:"space_type_slug"`
	SpaceTypeName string   `json:"space_type_name"`
	Enabled       bool     `json:"enabled"`
	Label         string   `json:"label"`
	Icon          *string  `json:"icon,omitempty"`
	SeedStubs     bool     `json:"seed_stubs"`
	Position      int32    `json:"position"`
	ParentTypeIDs []string `json:"parent_type_ids"`
}

// WikiCategoryDTO is a category template row — the public category list and
// admin editor share the shape. field_group_* identify the bound group;
// empty means the category is prose-only.
// //nolint:revive // exported: DTO naming follows package convention
type WikiCategoryDTO struct {
	ID             string `json:"id,omitempty"`
	Slug           string `json:"slug"`
	Category       string `json:"category"`
	Label          string `json:"label"`
	FieldGroupID   string `json:"field_group_id,omitempty"`
	FieldGroupSlug string `json:"field_group_slug,omitempty"`
	FieldGroupName string `json:"field_group_name,omitempty"`
	Position       int32  `json:"position"`
	IsSystem       bool   `json:"is_system"`
}

func wikiTypeToDTO(t WikiType) WikiTypeDTO {
	return WikiTypeDTO{
		SpaceTypeID:   t.SpaceTypeID.String(),
		SpaceTypeSlug: t.SpaceTypeSlug,
		SpaceTypeName: t.SpaceTypeName,
		Enabled:       t.Enabled,
		Label:         t.Label,
		Icon:          t.Icon,
		SeedStubs:     t.SeedStubs,
		Position:      t.Position,
		ParentTypeIDs: t.ParentTypeIDs,
	}
}

func wikiCategoryToDTO(c WikiCategory) WikiCategoryDTO {
	d := WikiCategoryDTO{
		Slug:     c.Slug,
		Category: c.Category,
		Label:    c.Label,
		Position: c.Position,
		IsSystem: c.IsSystem,
	}
	if c.ID.Valid {
		d.ID = c.ID.String()
	}
	if c.FieldGroupID.Valid {
		d.FieldGroupID = c.FieldGroupID.String()
	}
	if c.FieldGroupSlug != nil {
		d.FieldGroupSlug = *c.FieldGroupSlug
	}
	if c.FieldGroupName != nil {
		d.FieldGroupName = *c.FieldGroupName
	}

	return d
}

// ListEnabledWikiTypes returns the enabled wiki types for public
// navigation/destination filters (§80 — new types appear automatically).
func (s *Service) ListEnabledWikiTypes(ctx context.Context) ([]WikiTypeDTO, error) {
	types, err := s.store.ListEnabledWikiTypes(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]WikiTypeDTO, 0, len(types))
	for _, t := range types {
		out = append(out, wikiTypeToDTO(t))
	}

	return out, nil
}

// AdminListWikiTypes returns every wiki type with parent edges (admin).
func (s *Service) AdminListWikiTypes(ctx context.Context) ([]WikiTypeDTO, error) {
	types, err := s.store.ListWikiTypes(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]WikiTypeDTO, 0, len(types))
	for _, t := range types {
		out = append(out, wikiTypeToDTO(t))
	}

	return out, nil
}

// UpsertWikiTypeRequest is the admin create/enable payload.
// //nolint:revive // exported: request DTO naming follows package convention
type UpsertWikiTypeRequest struct {
	SpaceTypeID   string   `json:"space_type_id"`
	SpaceTypeSlug string   `json:"space_type_slug"` // alternative to space_type_id
	Enabled       *bool    `json:"enabled"`
	Label         *string  `json:"label"`
	Icon          *string  `json:"icon"`
	SeedStubs     bool     `json:"seed_stubs"`
	Position      int32    `json:"position"`
	ParentTypeIDs []string `json:"parent_type_ids"`
}

// resolveSpaceTypeID accepts either the UUID or the space_type slug.
func (s *Service) resolveSpaceTypeID(ctx context.Context, idStr, slug string) (pgtype.UUID, error) {
	if idStr != "" {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil || !id.Valid {
			return pgtype.UUID{}, ErrInvalidInput
		}
		return id, nil
	}

	if slug != "" {
		id, err := s.store.SpaceTypeIDBySlug(ctx, slug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return pgtype.UUID{}, ErrInvalidInput
			}
			return pgtype.UUID{}, err
		}
		return id, nil
	}

	return pgtype.UUID{}, ErrInvalidInput
}

// resolveParentIDs parses + validates the parent edge set: each parent must
// itself be a wiki type (wiki_type_configs row), and self-parenting is
// rejected (the DB CHECK also guards it).
func (s *Service) resolveParentIDs(ctx context.Context, spaceTypeID pgtype.UUID, raw []string) ([]pgtype.UUID, error) {
	ids := make([]pgtype.UUID, 0, len(raw))
	for _, p := range raw {
		var id pgtype.UUID
		if err := id.Scan(p); err != nil || !id.Valid {
			return nil, ErrInvalidInput
		}
		if id == spaceTypeID {
			return nil, ErrInvalidInput
		}
		if _, err := s.store.GetWikiType(ctx, id); err != nil {
			if errors.Is(err, ErrWikiTypeNotFound) {
				return nil, fmt.Errorf("%w: parent %s is not a wiki type", ErrInvalidInput, p)
			}
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// UpsertWikiType creates or re-enables a wiki type for a space type.
func (s *Service) UpsertWikiType(ctx context.Context, req UpsertWikiTypeRequest) (WikiTypeDTO, error) {
	spaceTypeID, err := s.resolveSpaceTypeID(ctx, req.SpaceTypeID, req.SpaceTypeSlug)
	if err != nil {
		return WikiTypeDTO{}, err
	}

	parentIDs, err := s.resolveParentIDs(ctx, spaceTypeID, req.ParentTypeIDs)
	if err != nil {
		return WikiTypeDTO{}, err
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	t, err := s.store.UpsertWikiType(ctx, spaceTypeID, enabled, req.Label, req.Icon, req.SeedStubs, req.Position, parentIDs)
	if err != nil {
		return WikiTypeDTO{}, err
	}

	return wikiTypeToDTO(t), nil
}

// UpdateWikiTypeRequest is the admin patch payload; nil fields are left
// unchanged. parent_type_ids replaces the edge set when non-nil.
// //nolint:revive // exported: request DTO naming follows package convention
type UpdateWikiTypeRequest struct {
	Enabled       *bool    `json:"enabled"`
	Label         *string  `json:"label"`
	Icon          *string  `json:"icon"`
	SeedStubs     *bool    `json:"seed_stubs"`
	Position      *int32   `json:"position"`
	ParentTypeIDs []string `json:"parent_type_ids"`
}

// UpdateWikiType patches a wiki type config and optionally replaces its
// parent edges.
func (s *Service) UpdateWikiType(ctx context.Context, spaceTypeIDStr string, req UpdateWikiTypeRequest) error {
	var spaceTypeID pgtype.UUID
	if err := spaceTypeID.Scan(spaceTypeIDStr); err != nil || !spaceTypeID.Valid {
		return ErrWikiTypeNotFound
	}

	var parentIDs []pgtype.UUID
	if req.ParentTypeIDs != nil {
		ids, err := s.resolveParentIDs(ctx, spaceTypeID, req.ParentTypeIDs)
		if err != nil {
			return err
		}
		parentIDs = ids
	}

	return s.store.UpdateWikiType(ctx, spaceTypeID, req.Enabled, req.Label, req.Icon, req.SeedStubs, req.Position, parentIDs)
}

// DeleteWikiType removes a wiki type (categories and parent edges cascade).
// Blocked while spaces of the type still have active wiki pages.
func (s *Service) DeleteWikiType(ctx context.Context, spaceTypeIDStr string) error {
	var spaceTypeID pgtype.UUID
	if err := spaceTypeID.Scan(spaceTypeIDStr); err != nil || !spaceTypeID.Valid {
		return ErrWikiTypeNotFound
	}

	n, err := s.store.CountActivePagesBySpaceType(ctx, spaceTypeID)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrWikiTypeInUse
	}

	return s.store.DeleteWikiType(ctx, spaceTypeID)
}

// ListCategoriesForSpace returns the space's type category template and
// the space_type slug (response scope) — the public
// GET /wiki/categories?space= source. Returns ErrSpaceNotFound when the
// slug resolves to no space.
func (s *Service) ListCategoriesForSpace(ctx context.Context, spaceSlug string) ([]WikiCategory, string, error) {
	spaceID, _, err := s.store.GetSpaceBySlug(ctx, spaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrSpaceNotFound
		}

		return nil, "", err
	}

	_, typeSlug, err := s.store.GetSpaceTypeRef(ctx, spaceID)
	if err != nil {
		return nil, "", err
	}

	cats, err := s.store.ListCategoriesBySpaceSlug(ctx, spaceSlug)
	if err != nil {
		return nil, "", err
	}

	return cats, typeSlug, nil
}

// ListAllCategories returns the union of every enabled type's category
// template — the public fallback when no ?space= is given.
func (s *Service) ListAllCategories(ctx context.Context) ([]WikiCategory, error) {
	return s.store.ListAllCategories(ctx)
}

// ListWikiTypeCategories returns a type's full category template (admin).
func (s *Service) ListWikiTypeCategories(ctx context.Context, spaceTypeIDStr string) ([]WikiCategoryDTO, error) {
	spaceTypeID, err := s.resolveSpaceTypeID(ctx, spaceTypeIDStr, spaceTypeIDStr)
	if err != nil {
		return nil, err
	}
	if !spaceTypeID.Valid {
		return nil, ErrWikiTypeNotFound
	}

	// Type must have a config — a plain space_type id is not a wiki type.
	if _, err := s.store.GetWikiType(ctx, spaceTypeID); err != nil {
		return nil, err
	}

	cats, err := s.store.ListWikiTypeCategories(ctx, spaceTypeID)
	if err != nil {
		return nil, err
	}

	out := make([]WikiCategoryDTO, 0, len(cats))
	for _, c := range cats {
		out = append(out, wikiCategoryToDTO(c))
	}

	return out, nil
}

// wikiSlugPattern constrains page-slug input: lowercase, hyphenated —
// matching the existing stub-page convention (visa-routes, cost-of-living).
var wikiSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// wikiCategoryPattern constrains the wiki_pages.category token: lowercase,
// underscored (cost_of_living). The admin may omit it; it is then derived
// from the slug (hyphens → underscores).
var wikiCategoryPattern = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// CreateWikiCategoryRequest is the admin add-category payload.
// //nolint:revive // exported: request DTO naming follows package convention
type CreateWikiCategoryRequest struct {
	Slug         string `json:"slug"`
	Category     string `json:"category"`
	Label        string `json:"label"`
	FieldGroupID string `json:"field_group_id"`
	Position     int32  `json:"position"`
}

// CreateWikiTypeCategory adds a category to a type's template. The
// field-group binding is validated against custom_field_groups so the
// schema the page renders always resolves.
func (s *Service) CreateWikiTypeCategory(ctx context.Context, spaceTypeIDStr string, req CreateWikiCategoryRequest) (WikiCategoryDTO, error) {
	spaceTypeID, err := s.resolveSpaceTypeID(ctx, spaceTypeIDStr, spaceTypeIDStr)
	if err != nil {
		return WikiCategoryDTO{}, err
	}

	if _, err := s.store.GetWikiType(ctx, spaceTypeID); err != nil {
		return WikiCategoryDTO{}, err
	}

	slug := strings.TrimSpace(strings.ToLower(req.Slug))
	if !wikiSlugPattern.MatchString(slug) {
		return WikiCategoryDTO{}, fmt.Errorf("%w: invalid category slug", ErrInvalidInput)
	}

	category := strings.TrimSpace(strings.ToLower(req.Category))
	if category == "" {
		category = strings.ReplaceAll(slug, "-", "_")
	}
	if !wikiCategoryPattern.MatchString(category) {
		return WikiCategoryDTO{}, fmt.Errorf("%w: invalid category token", ErrInvalidInput)
	}

	label := strings.TrimSpace(req.Label)
	if label == "" {
		return WikiCategoryDTO{}, fmt.Errorf("%w: category label required", ErrInvalidInput)
	}

	fgID, err := s.validateFieldGroup(ctx, req.FieldGroupID)
	if err != nil {
		return WikiCategoryDTO{}, err
	}

	c, err := s.store.CreateWikiTypeCategory(ctx, spaceTypeID, slug, category, label, fgID, req.Position, false)
	if err != nil {
		return WikiCategoryDTO{}, err
	}

	return wikiCategoryToDTO(c), nil
}

// validateFieldGroup resolves an optional field_group_id — empty means
// prose-only (NULL). A non-empty value must be a real group.
func (s *Service) validateFieldGroup(ctx context.Context, idStr string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if strings.TrimSpace(idStr) == "" {
		return id, nil
	}
	if err := id.Scan(idStr); err != nil || !id.Valid {
		return pgtype.UUID{}, ErrFieldGroupNotFound
	}
	if s.fields != nil {
		if _, err := s.fields.GetFieldGroup(ctx, id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return pgtype.UUID{}, ErrFieldGroupNotFound
			}
			return pgtype.UUID{}, err
		}
	}

	return id, nil
}

// UpdateWikiCategoryRequest is the admin patch payload; nil fields are left
// unchanged. field_group_id participates via a pointer: an explicit value
// (including "") replaces the binding — "" clears it to NULL.
// //nolint:revive // exported: request DTO naming follows package convention
type UpdateWikiCategoryRequest struct {
	Label        *string `json:"label"`
	FieldGroupID *string `json:"field_group_id"`
	Position     *int32  `json:"position"`
}

// UpdateWikiTypeCategory patches a category's label, group binding, and
// position. Slug and category are immutable (page URLs).
func (s *Service) UpdateWikiTypeCategory(ctx context.Context, idStr string, req UpdateWikiCategoryRequest) error {
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil || !id.Valid {
		return ErrWikiCategoryNotFound
	}

	updateFG := req.FieldGroupID != nil
	var fgID pgtype.UUID
	if updateFG {
		v, err := s.validateFieldGroup(ctx, *req.FieldGroupID)
		if err != nil {
			return err
		}
		fgID = v
	}

	return s.store.UpdateWikiTypeCategory(ctx, id, req.Label, updateFG, fgID, req.Position)
}

// DeleteWikiTypeCategory removes a non-system category. Blocked while pages
// in the category (any space of the type) carry approved prose.
func (s *Service) DeleteWikiTypeCategory(ctx context.Context, idStr string) error {
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil || !id.Valid {
		return ErrWikiCategoryNotFound
	}

	cat, err := s.store.GetWikiTypeCategory(ctx, id)
	if err != nil {
		return err
	}
	if cat.IsSystem {
		return ErrWikiCategoryProtected
	}

	n, err := s.store.CountCategoryProsePages(ctx, cat.Category, cat.SpaceTypeID)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrWikiCategoryInUse
	}

	return s.store.DeleteWikiTypeCategory(ctx, id)
}
