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
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
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

// ErrWikiCategoryExists is returned when a category's slug or category
// token collides with an existing template row on the same type.
var ErrWikiCategoryExists = errors.New("wiki category already exists")

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
// admin editor share the shape. field_group_* identify the bound group
// (always present — the group is the page's schema; a prose-only page has
// a group with no fields).
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
	// Rollup is the category's descendant-aggregation config (§80
	// Hierarchical UX) — passed through verbatim so the admin editor and
	// public readers share the shape.
	Rollup json.RawMessage `json:"rollup,omitempty"`
	// Icon is an optional lucide icon name rendered on the category's tab
	// and card; nil falls back to the frontend's slug→icon map.
	Icon *string `json:"icon,omitempty"`
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
	if len(c.Rollup) > 0 {
		d.Rollup = json.RawMessage(c.Rollup)
	}
	d.Icon = c.Icon

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

	typeID, typeSlug, err := s.store.GetSpaceTypeRef(ctx, spaceID)
	if err != nil {
		return nil, "", err
	}

	cats, err := s.store.ListCategoriesBySpaceSlug(ctx, spaceSlug)
	if err != nil {
		return nil, "", err
	}

	// §80 directories: a directory-mode category only earns a tab/card
	// when the space actually has matching descendants — a "Universities"
	// stub on a state with no linked universities is a dead end. Fail
	// open on lookup errors so a transient failure never hides a
	// legitimately populated category.
	for i := range cats {
		rcfg := parseRollupConfig(cats[i].Rollup)
		if rcfg == nil || rcfg.Mode != rollupModeDirectory {
			continue
		}
		typeIDs, terr := s.rollupTypeIDs(ctx, typeID, rcfg.Levels)
		if terr != nil {
			continue
		}
		if len(typeIDs) == 0 {
			cats[i].Empty = true
			continue
		}
		has, herr := s.store.HasDirectoryChildren(ctx, spaceID, typeIDs)
		if herr != nil {
			continue
		}
		cats[i].Empty = !has
	}

	return cats, typeSlug, nil
}

// ListAllCategories returns the union of every enabled type's category
// template — the public fallback when no ?space= is given.
func (s *Service) ListAllCategories(ctx context.Context) ([]WikiCategory, error) {
	return s.store.ListAllCategories(ctx)
}

// ListPageTemplatesForSpace returns the space type's page templates with
// the `{name}` label token substituted for the space's display name —
// the public discovery surface for lazy template pages
// (/wiki/{space}/{template_slug}). Returns nil for a space whose type
// declares none.
func (s *Service) ListPageTemplatesForSpace(ctx context.Context, spaceSlug string) ([]WikiPageTemplate, error) {
	spaceID, spaceName, err := s.store.GetSpaceBySlug(ctx, spaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSpaceNotFound
		}

		return nil, err
	}

	typeID, _, err := s.store.GetSpaceTypeRef(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	templates, err := s.store.ListPageTemplatesBySpaceType(ctx, typeID)
	if err != nil {
		return nil, err
	}

	for i := range templates {
		templates[i].Label = substituteTemplateLabel(templates[i].Label, spaceName)
	}

	return templates, nil
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

// validateIconName checks an optional lucide icon name — kebab-case, same
// charset as a page slug. nil stays nil (no icon); a present-but-empty
// string clears to nil.
func validateIconName(icon *string) (*string, error) {
	if icon == nil {
		return nil, nil
	}
	v := strings.TrimSpace(strings.ToLower(*icon))
	if v == "" {
		return nil, nil
	}
	if !wikiSlugPattern.MatchString(v) {
		return nil, fmt.Errorf("%w: invalid icon name", ErrInvalidInput)
	}

	return &v, nil
}

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
	// Rollup opts the category into descendant aggregation (§80); validated
	// by parseWikiRollup — omitted leaves the category prose/fields only.
	Rollup json.RawMessage `json:"rollup"`
	// Icon is an optional lucide icon name (kebab-case) for the category's
	// tab/card.
	Icon *string `json:"icon"`
}

// CreateWikiTypeCategory adds a category to a type's template. Every
// category binds a field group — the group IS the page's schema: an
// explicit field_group_id rebinds an existing group (validated against
// custom_field_groups), while an omitted one auto-provisions a dedicated
// group named wiki_{type}_{category} assigned to entity_class
// 'space_type:{type_slug}' (§80 — per-level schema isolation for JCE).
func (s *Service) CreateWikiTypeCategory(ctx context.Context, spaceTypeIDStr string, req CreateWikiCategoryRequest) (WikiCategoryDTO, error) {
	spaceTypeID, err := s.resolveSpaceTypeID(ctx, spaceTypeIDStr, spaceTypeIDStr)
	if err != nil {
		return WikiCategoryDTO{}, err
	}

	wt, err := s.store.GetWikiType(ctx, spaceTypeID)
	if err != nil {
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

	var fgID pgtype.UUID
	if strings.TrimSpace(req.FieldGroupID) == "" {
		fgID, err = s.provisionCategoryGroup(ctx, wt.SpaceTypeSlug, category, label)
	} else {
		fgID, err = s.validateFieldGroup(ctx, req.FieldGroupID)
	}
	if err != nil {
		return WikiCategoryDTO{}, err
	}

	rollup, err := validateRollupInput(req.Rollup)
	if err != nil {
		return WikiCategoryDTO{}, err
	}

	icon, err := validateIconName(req.Icon)
	if err != nil {
		return WikiCategoryDTO{}, err
	}

	c, err := s.store.CreateWikiTypeCategory(ctx, spaceTypeID, slug, category, label, fgID, req.Position, false, rollup, icon)
	if err != nil {
		return WikiCategoryDTO{}, err
	}

	// The create RETURNING does not join the group — fill slug/name so the
	// admin UI can show the (often auto-provisioned) binding immediately.
	if c.FieldGroupID.Valid && s.fields != nil {
		if g, err := s.fields.GetFieldGroup(ctx, c.FieldGroupID); err == nil {
			c.FieldGroupSlug = &g.Slug
			c.FieldGroupName = &g.Name
		}
	}

	return wikiCategoryToDTO(c), nil
}

// provisionCategoryGroup get-or-creates the category's dedicated field
// group (slug wiki_{type}_{category}) and ensures its entity-class
// assignment (space_type:{type}) exists. A group that survives a deleted
// category is reused rather than duplicated, keeping the binding stable
// across delete/recreate.
func (s *Service) provisionCategoryGroup(ctx context.Context, typeSlug, category, label string) (pgtype.UUID, error) {
	if s.fieldProvisioner == nil || s.fields == nil {
		return pgtype.UUID{}, fmt.Errorf("%w: field_group_id required (group provisioning unavailable)", ErrInvalidInput)
	}

	groupSlug := "wiki_" + typeSlug + "_" + category
	group, err := s.fieldProvisioner.GetFieldGroupBySlug(ctx, groupSlug)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, err
		}
		group, err = s.fieldProvisioner.CreateFieldGroup(ctx, label+" ("+typeSlug+")", groupSlug, nil, false)
		if err != nil {
			// A concurrent create (or a read replica that shadowed an
			// existing row) took the slug — reuse it rather than fail.
			group, err = s.fieldProvisioner.GetFieldGroupBySlug(ctx, groupSlug)
			if err != nil {
				return pgtype.UUID{}, err
			}
		}
	}

	entityClass := "space_type:" + typeSlug
	assignments, err := s.fields.ListAssignmentsByEntityClass(ctx, entityClass)
	if err != nil {
		return pgtype.UUID{}, err
	}
	assigned := false
	for _, a := range assignments {
		if a.GroupID == group.ID && !a.EntityID.Valid {
			assigned = true
			break
		}
	}
	if !assigned {
		if _, err := s.fieldProvisioner.CreateAssignment(ctx, gen.CreateAssignmentParams{
			GroupID:     group.ID,
			EntityClass: entityClass,
			Position:    0,
		}); err != nil {
			// A concurrent create (or a read replica that shadowed a
			// committed row) may have beaten this insert — re-read the class
			// assignments and continue only when the row now exists. A real
			// failure must surface: without the assignment the group is
			// invisible to entity-field resolution even though the page
			// renders it via field_group_id.
			retry, rerr := s.fields.ListAssignmentsByEntityClass(ctx, entityClass)
			if rerr != nil {
				return pgtype.UUID{}, fmt.Errorf("wiki: provision category group: %w", rerr)
			}
			for _, a := range retry {
				if a.GroupID == group.ID && !a.EntityID.Valid {
					return group.ID, nil
				}
			}
			return pgtype.UUID{}, fmt.Errorf("wiki: provision category group assignment: %w", err)
		}
	}

	return group.ID, nil
}

// validateFieldGroup resolves an explicit field_group_id — it must be a
// real custom_field_groups row. The binding is mandatory (NOT NULL);
// category create without a value goes through provisionCategoryGroup
// instead.
func (s *Service) validateFieldGroup(ctx context.Context, idStr string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if strings.TrimSpace(idStr) == "" {
		return pgtype.UUID{}, fmt.Errorf("%w: field_group_id required", ErrInvalidInput)
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
// rebinds to an existing group. The binding is mandatory — an empty string
// is rejected (a category cannot exist without a group).
// //nolint:revive // exported: request DTO naming follows package convention
type UpdateWikiCategoryRequest struct {
	Label        *string `json:"label"`
	FieldGroupID *string `json:"field_group_id"`
	Position     *int32  `json:"position"`
	// Rollup is three-state (§80): absent leaves the config untouched, an
	// explicit null clears it, an object replaces it (validated).
	Rollup json.RawMessage `json:"rollup"`
	// Icon is three-state like Rollup: absent leaves it, null clears it,
	// a string sets the lucide icon name.
	Icon json.RawMessage `json:"icon"`
}

// UpdateWikiTypeCategory patches a category's label, group binding, and
// position. Slug and category are immutable (page URLs).
func (s *Service) UpdateWikiTypeCategory(ctx context.Context, idStr string, req UpdateWikiCategoryRequest) error {
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil || !id.Valid {
		return ErrWikiCategoryNotFound
	}

	if req.Label != nil {
		*req.Label = strings.TrimSpace(*req.Label)
		if *req.Label == "" {
			return fmt.Errorf("%w: category label required", ErrInvalidInput)
		}
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

	var rollup []byte
	updateRollup := len(req.Rollup) > 0
	if updateRollup {
		v, err := validateRollupInput(req.Rollup)
		if err != nil {
			return err
		}
		rollup = v
	}

	var icon *string
	updateIcon := len(req.Icon) > 0
	if updateIcon {
		var v *string
		if strings.TrimSpace(string(req.Icon)) != "null" {
			var s string
			if err := json.Unmarshal(req.Icon, &s); err != nil {
				return fmt.Errorf("%w: invalid icon", ErrInvalidInput)
			}
			v = &s
		}
		v, err := validateIconName(v)
		if err != nil {
			return err
		}
		icon = v
	}

	return s.store.UpdateWikiTypeCategory(ctx, id, req.Label, updateFG, fgID, req.Position, updateRollup, rollup, updateIcon, icon)
}

// DeleteWikiTypeCategory removes a non-system category. Blocked while pages
// in the category (any space of the type) carry approved prose. The bound
// field group (and any field definitions / stored values) is left intact —
// orphaned groups are cleaned up in the Admin Panel.
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

// ---------------------------------------------------------------------------
// Rollup config + page templates (§80 Hierarchical UX & Itemized Pricing)
// ---------------------------------------------------------------------------

// validRollupMetrics is the aggregate vocabulary a category rollup may
// request.
var validRollupMetrics = map[string]bool{"min": true, "avg": true, "max": true}

// validateRollupInput checks a rollup JSONB payload: null/empty clears,
// otherwise {enabled bool, mode ∈ {"", "directory"}, metrics ⊆ {min,avg,max},
// levels []slug, section_limit ≥ 0}.
// Returns the canonical bytes to store (nil when clearing).
func validateRollupInput(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var cfg WikiRollupConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("%w: rollup must be an object", ErrInvalidInput)
	}

	// mode selects the renderer: empty = fold-up aggregation (default),
	// "directory" = child-space list (§80). Anything else is a typo the
	// frontend would silently ignore — reject it.
	if cfg.Mode != "" && cfg.Mode != rollupModeDirectory {
		return nil, fmt.Errorf("%w: unknown rollup mode %q", ErrInvalidInput, cfg.Mode)
	}
	for _, m := range cfg.Metrics {
		if !validRollupMetrics[m] {
			return nil, fmt.Errorf("%w: unknown rollup metric %q", ErrInvalidInput, m)
		}
	}
	for _, l := range cfg.Levels {
		if !wikiCategoryPattern.MatchString(l) && !wikiSlugPattern.MatchString(l) {
			return nil, fmt.Errorf("%w: invalid rollup level %q", ErrInvalidInput, l)
		}
	}
	if cfg.SectionLimit < 0 {
		return nil, fmt.Errorf("%w: rollup section_limit must be >= 0", ErrInvalidInput)
	}

	return raw, nil
}

// WikiPageTemplateDTO is the admin representation of a page template.
// parent_template_id nests the template under a same-category top-level
// template (§80 Wiki Topic Pages); section_label/preview_limit configure
// the composite section the child contributes to the parent's page.
// //nolint:revive // exported: DTO naming follows package convention
type WikiPageTemplateDTO struct {
	ID               string  `json:"id,omitempty"`
	CategoryID       string  `json:"category_id"`
	Slug             string  `json:"slug"`
	Label            string  `json:"label"`
	FieldGroupID     string  `json:"field_group_id,omitempty"`
	FieldGroupSlug   string  `json:"field_group_slug,omitempty"`
	FieldGroupName   string  `json:"field_group_name,omitempty"`
	ParentTemplateID *string `json:"parent_template_id,omitempty"`
	SectionLabel     *string `json:"section_label,omitempty"`
	PreviewLimit     int32   `json:"preview_limit"`
	Position         int32   `json:"position"`
	IsSystem         bool    `json:"is_system"`
}

func wikiPageTemplateToDTO(t WikiPageTemplate) WikiPageTemplateDTO {
	d := WikiPageTemplateDTO{
		ID:           t.ID.String(),
		CategoryID:   t.CategoryID.String(),
		Slug:         t.Slug,
		Label:        t.Label,
		SectionLabel: t.SectionLabel,
		PreviewLimit: t.PreviewLimit,
		Position:     t.Position,
		IsSystem:     t.IsSystem,
	}
	if t.FieldGroupID.Valid {
		d.FieldGroupID = t.FieldGroupID.String()
	}
	if t.FieldGroupSlug != nil {
		d.FieldGroupSlug = *t.FieldGroupSlug
	}
	if t.FieldGroupName != nil {
		d.FieldGroupName = *t.FieldGroupName
	}
	if t.ParentTemplateID.Valid {
		pid := t.ParentTemplateID.String()
		d.ParentTemplateID = &pid
	}

	return d
}

// ListPageTemplates returns a category's page templates (admin).
func (s *Service) ListPageTemplates(ctx context.Context, categoryIDStr string) ([]WikiPageTemplateDTO, error) {
	var categoryID pgtype.UUID
	if err := categoryID.Scan(categoryIDStr); err != nil || !categoryID.Valid {
		return nil, ErrWikiCategoryNotFound
	}
	if _, err := s.store.GetWikiTypeCategory(ctx, categoryID); err != nil {
		return nil, err
	}

	templates, err := s.store.ListPageTemplates(ctx, categoryID)
	if err != nil {
		return nil, err
	}

	out := make([]WikiPageTemplateDTO, 0, len(templates))
	for _, t := range templates {
		out = append(out, wikiPageTemplateToDTO(t))
	}

	return out, nil
}

// ErrInvalidParentTemplate is returned when a template's parent_template_id
// does not name a top-level template in the same category (§80 Wiki Topic
// Pages — nesting depth is capped at one child level, and parents may not
// cross categories).
var ErrInvalidParentTemplate = errors.New("invalid parent template")

// resolveTemplateParent validates an optional parent_template_id: the
// parent must exist, belong to the same category, and be top-level itself
// (v1 caps nesting at one level). A blank id clears the parent.
func (s *Service) resolveTemplateParent(ctx context.Context, categoryID, selfID pgtype.UUID, idStr string) (pgtype.UUID, error) {
	if strings.TrimSpace(idStr) == "" {
		return pgtype.UUID{}, nil
	}

	var parentID pgtype.UUID
	if err := parentID.Scan(idStr); err != nil || !parentID.Valid {
		return pgtype.UUID{}, fmt.Errorf("%w: parent_template_id is not a uuid", ErrInvalidParentTemplate)
	}
	if parentID == selfID {
		return pgtype.UUID{}, fmt.Errorf("%w: a template cannot parent itself", ErrInvalidParentTemplate)
	}

	parent, err := s.store.GetPageTemplate(ctx, parentID)
	if err != nil {
		if errors.Is(err, ErrWikiPageTemplateNotFound) {
			return pgtype.UUID{}, fmt.Errorf("%w: not found", ErrInvalidParentTemplate)
		}
		return pgtype.UUID{}, err
	}
	if parent.CategoryID != categoryID {
		return pgtype.UUID{}, fmt.Errorf("%w: parent must belong to the same category", ErrInvalidParentTemplate)
	}
	if parent.ParentTemplateID.Valid {
		return pgtype.UUID{}, fmt.Errorf("%w: parent must be a top-level template", ErrInvalidParentTemplate)
	}

	return parentID, nil
}

// CreateWikiPageTemplateRequest is the admin add-template payload.
// //nolint:revive // exported: request DTO naming follows package convention
type CreateWikiPageTemplateRequest struct {
	Slug         string `json:"slug"`
	Label        string `json:"label"`
	FieldGroupID string `json:"field_group_id"`
	// ParentTemplateID nests the template under a same-category top-level
	// template (§80 Wiki Topic Pages); empty = top-level.
	ParentTemplateID *string `json:"parent_template_id"`
	// SectionLabel overrides the label the synthesized composite section on
	// the parent page carries (nil → the template's own label is used).
	SectionLabel *string `json:"section_label"`
	// PreviewLimit caps the rows the parent's composite preview section
	// shows before linking to the full topic page (nil → default 5).
	PreviewLimit *int32 `json:"preview_limit"`
	Position     int32  `json:"position"`
}

// CreateWikiPageTemplate adds a page template under a category. An omitted
// field_group_id auto-provisions the shared group wiki_{type}_page_{slug}
// assigned to space_type:{slug} — one group across every space of the type
// keeps item field keys comparable for JCE (§80).
func (s *Service) CreateWikiPageTemplate(ctx context.Context, categoryIDStr string, req CreateWikiPageTemplateRequest) (WikiPageTemplateDTO, error) {
	var categoryID pgtype.UUID
	if err := categoryID.Scan(categoryIDStr); err != nil || !categoryID.Valid {
		return WikiPageTemplateDTO{}, ErrWikiCategoryNotFound
	}

	cat, err := s.store.GetWikiTypeCategory(ctx, categoryID)
	if err != nil {
		return WikiPageTemplateDTO{}, err
	}

	wt, err := s.store.GetWikiType(ctx, cat.SpaceTypeID)
	if err != nil {
		return WikiPageTemplateDTO{}, err
	}

	slug := strings.TrimSpace(strings.ToLower(req.Slug))
	if !wikiSlugPattern.MatchString(slug) {
		return WikiPageTemplateDTO{}, fmt.Errorf("%w: invalid template slug", ErrInvalidInput)
	}

	label := strings.TrimSpace(req.Label)
	if label == "" {
		return WikiPageTemplateDTO{}, fmt.Errorf("%w: template label required", ErrInvalidInput)
	}

	var fgID pgtype.UUID
	if strings.TrimSpace(req.FieldGroupID) == "" {
		fgID, err = s.provisionPageTemplateGroup(ctx, wt.SpaceTypeSlug, slug, label)
	} else {
		fgID, err = s.validateFieldGroup(ctx, req.FieldGroupID)
	}
	if err != nil {
		return WikiPageTemplateDTO{}, err
	}

	var parentID pgtype.UUID
	if req.ParentTemplateID != nil {
		parentID, err = s.resolveTemplateParent(ctx, categoryID, pgtype.UUID{}, *req.ParentTemplateID)
		if err != nil {
			return WikiPageTemplateDTO{}, err
		}
	}

	if req.PreviewLimit != nil && *req.PreviewLimit < 0 {
		return WikiPageTemplateDTO{}, fmt.Errorf("%w: preview_limit must be >= 0", ErrInvalidInput)
	}

	t, err := s.store.CreatePageTemplate(ctx, categoryID, slug, label, fgID, parentID, req.SectionLabel, req.PreviewLimit, req.Position, false)
	if err != nil {
		return WikiPageTemplateDTO{}, err
	}

	if t.FieldGroupID.Valid && s.fields != nil {
		if g, err := s.fields.GetFieldGroup(ctx, t.FieldGroupID); err == nil {
			t.FieldGroupSlug = &g.Slug
			t.FieldGroupName = &g.Name
		}
	}

	return wikiPageTemplateToDTO(t), nil
}

// provisionPageTemplateGroup get-or-creates the template's dedicated field
// group (slug wiki_{type}_page_{slug}) + its space_type:{slug} assignment —
// the same get-or-create pattern as provisionCategoryGroup.
func (s *Service) provisionPageTemplateGroup(ctx context.Context, typeSlug, pageSlug, label string) (pgtype.UUID, error) {
	if s.fieldProvisioner == nil || s.fields == nil {
		return pgtype.UUID{}, fmt.Errorf("%w: field_group_id required (group provisioning unavailable)", ErrInvalidInput)
	}

	groupSlug := "wiki_" + typeSlug + "_page_" + strings.ReplaceAll(pageSlug, "-", "_")
	group, err := s.fieldProvisioner.GetFieldGroupBySlug(ctx, groupSlug)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, err
		}
		group, err = s.fieldProvisioner.CreateFieldGroup(ctx, label+" ("+typeSlug+")", groupSlug, nil, false)
		if err != nil {
			group, err = s.fieldProvisioner.GetFieldGroupBySlug(ctx, groupSlug)
			if err != nil {
				return pgtype.UUID{}, err
			}
		}
	}

	entityClass := "space_type:" + typeSlug
	assignments, err := s.fields.ListAssignmentsByEntityClass(ctx, entityClass)
	if err != nil {
		return pgtype.UUID{}, err
	}
	assigned := false
	for _, a := range assignments {
		if a.GroupID == group.ID && !a.EntityID.Valid {
			assigned = true
			break
		}
	}
	if !assigned {
		if _, err := s.fieldProvisioner.CreateAssignment(ctx, gen.CreateAssignmentParams{
			GroupID:     group.ID,
			EntityClass: entityClass,
			Position:    0,
		}); err != nil {
			retry, rerr := s.fields.ListAssignmentsByEntityClass(ctx, entityClass)
			if rerr != nil {
				return pgtype.UUID{}, fmt.Errorf("wiki: provision template group: %w", rerr)
			}
			for _, a := range retry {
				if a.GroupID == group.ID && !a.EntityID.Valid {
					return group.ID, nil
				}
			}
			return pgtype.UUID{}, fmt.Errorf("wiki: provision template group assignment: %w", err)
		}
	}

	return group.ID, nil
}

// UpdateWikiPageTemplateRequest is the admin patch payload; nil fields are
// left unchanged. field_group_id rebinding follows the category contract:
// an explicit value must resolve to a real group. parent_template_id and
// section_label are three-state: absent leaves them, explicit null clears
// them (top-level / no section-label override), a value validates/sets.
// //nolint:revive // exported: request DTO naming follows package convention
type UpdateWikiPageTemplateRequest struct {
	Label        *string `json:"label"`
	FieldGroupID *string `json:"field_group_id"`
	Position     *int32  `json:"position"`

	// ParentTemplateID is a uuid string or null; absent leaves parentage.
	ParentTemplateID json.RawMessage `json:"parent_template_id"`
	// SectionLabel is a string or null; absent leaves the override.
	SectionLabel json.RawMessage `json:"section_label"`
	// PreviewLimit replaces the preview cap when non-nil (>= 0; 0 = all).
	PreviewLimit *int32 `json:"preview_limit"`
}

// UpdateWikiPageTemplate patches a template's label, group binding,
// position, parentage, and preview config. Slug is immutable (page URLs).
// Re-parenting a template that itself has children is rejected — a child
// promoted to parent of another would silently exceed the depth cap.
func (s *Service) UpdateWikiPageTemplate(ctx context.Context, idStr string, req UpdateWikiPageTemplateRequest) error {
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil || !id.Valid {
		return ErrWikiPageTemplateNotFound
	}

	if req.Label != nil {
		*req.Label = strings.TrimSpace(*req.Label)
		if *req.Label == "" {
			return fmt.Errorf("%w: template label required", ErrInvalidInput)
		}
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

	updateParent := len(req.ParentTemplateID) > 0
	var parentID pgtype.UUID
	if updateParent {
		var pid *string
		if strings.TrimSpace(string(req.ParentTemplateID)) != "null" {
			var s string
			if err := json.Unmarshal(req.ParentTemplateID, &s); err != nil {
				return fmt.Errorf("%w: parent_template_id must be a uuid string or null", ErrInvalidInput)
			}
			pid = &s
		}
		if pid != nil {
			// resolveTemplateParent needs the template's category — fetch
			// the row once to check same-category parentage.
			t, err := s.store.GetPageTemplate(ctx, id)
			if err != nil {
				return err
			}
			// Depth cap (§80 Wiki Topic Pages): a template that already has
			// children cannot be nested under a parent — that would produce
			// a second child level.
			siblings, err := s.store.ListPageTemplates(ctx, t.CategoryID)
			if err != nil {
				return err
			}
			for _, sib := range siblings {
				if sib.ParentTemplateID == id {
					return fmt.Errorf("%w: template has child topics and cannot be nested", ErrInvalidParentTemplate)
				}
			}
			parentID, err = s.resolveTemplateParent(ctx, t.CategoryID, id, *pid)
			if err != nil {
				return err
			}
		}
	}

	updateSectionLabel := len(req.SectionLabel) > 0
	var sectionLabel *string
	if updateSectionLabel {
		if strings.TrimSpace(string(req.SectionLabel)) != "null" {
			var s string
			if err := json.Unmarshal(req.SectionLabel, &s); err != nil {
				return fmt.Errorf("%w: section_label must be a string or null", ErrInvalidInput)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				sectionLabel = nil
			} else {
				sectionLabel = &s
			}
		}
	}

	if req.PreviewLimit != nil && *req.PreviewLimit < 0 {
		return fmt.Errorf("%w: preview_limit must be >= 0", ErrInvalidInput)
	}

	return s.store.UpdatePageTemplate(ctx, id, req.Label, updateFG, fgID, req.Position, updateParent, parentID, updateSectionLabel, sectionLabel, req.PreviewLimit)
}

// DeleteWikiPageTemplate removes a non-system template. Blocked while bound
// pages carry approved prose; bound pages otherwise keep their stored
// field_group_id (the binding survives deletion).
func (s *Service) DeleteWikiPageTemplate(ctx context.Context, idStr string) error {
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil || !id.Valid {
		return ErrWikiPageTemplateNotFound
	}

	t, err := s.store.GetPageTemplate(ctx, id)
	if err != nil {
		return err
	}
	if t.IsSystem {
		return ErrWikiPageTemplateProtected
	}

	n, err := s.store.CountTemplateContentPages(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrWikiPageTemplateInUse
	}

	return s.store.DeletePageTemplate(ctx, id)
}
