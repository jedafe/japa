// Wiki types & category templates (§80 Wiki Types & Field-Group Pages
// amendment, migration 000739). A wiki type is a space_type with a
// wiki_type_configs row; wiki_type_parents declares allowed nesting; each
// type's wiki_type_categories template may bind a custom field group — the
// shared page schema every entity of the type renders at
// /wiki/{entity}/{slug}.
// // Ref: japaconnect-engineering/sections/80-destination-wiki.md
package wiki

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// WikiType is a space_type's wiki configuration joined to the space_type
// identity (slug/name) and its declared parent types.
type WikiType struct {
	SpaceTypeID   pgtype.UUID
	SpaceTypeSlug string
	SpaceTypeName string
	Enabled       bool
	Label         string // resolved: config label or space_type name
	Icon          *string
	SeedStubs     bool
	Position      int32
	ParentTypeIDs []string
	CreatedAt     pgtype.Timestamptz
}

// WikiCategory is one row of a wiki type's category template.
// FieldGroupID is always bound (NOT NULL) — the group is the page's
// schema; a prose-only page has a group with no fields.
type WikiCategory struct {
	ID             pgtype.UUID
	SpaceTypeID    pgtype.UUID
	SpaceTypeSlug  string
	Slug           string // page slug used in /wiki/{entity}/{slug}
	Category       string // wiki_pages.category token
	Label          string
	FieldGroupID   pgtype.UUID
	FieldGroupSlug *string
	FieldGroupName *string
	Position       int32
	IsSystem       bool
	// Rollup is the category's descendant-aggregation config (§80
	// Hierarchical UX): {"enabled":true,"metrics":[...],"levels":[...]} —
	// nil when the category does not roll up.
	Rollup []byte
	// Icon is an optional lucide icon name for the category's tab/card —
	// nil falls back to the frontend's slug→icon map.
	Icon      *string
	CreatedAt pgtype.Timestamptz
	// Empty is space-scoped (ListCategoriesForSpace only): true when a
	// directory-mode category has zero matching descendants for the
	// requested space — the read surfaces hide those tabs/cards so a
	// "Universities" listing never renders for a space without any (§80).
	Empty bool
}

// ErrWikiTypeNotFound is returned when a wiki_type_configs row does not
// exist for the given space type.
var ErrWikiTypeNotFound = errors.New("wiki type not found")

// ErrWikiCategoryNotFound is returned when a wiki_type_categories row does
// not exist (or does not belong to the space's type).
var ErrWikiCategoryNotFound = errors.New("wiki category not found")

// spaceTypeIDOrErr resolves a space_type slug to its UUID for admin writes
// that address types by slug.
func (s *Store) SpaceTypeIDBySlug(ctx context.Context, slug string) (pgtype.UUID, error) {
	id, err := s.read.GetSpaceTypeIDBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgx.ErrNoRows
		}

		return pgtype.UUID{}, fmt.Errorf("wiki: space type by slug: %w", err)
	}

	return id, nil
}

// SpaceSeedStubs reports whether the space's wiki type opts into stub-page
// seeding. Returns false when the space or its type has no wiki config.
func (s *Store) SpaceSeedStubs(ctx context.Context, spaceID pgtype.UUID) (bool, error) {
	seed, err := s.read.GetSpaceSeedStubs(ctx, spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("wiki: space seed stubs: %w", err)
	}

	return seed, nil
}

// ListSeedCategories returns the category template rows (slug, category
// token, label) the stub seeder materializes for a space of this type.
func (s *Store) ListSeedCategories(ctx context.Context, spaceTypeID pgtype.UUID) ([]gen.ListSeedCategoriesBySpaceTypeRow, error) {
	rows, err := s.read.ListSeedCategoriesBySpaceType(ctx, spaceTypeID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list seed categories: %w", err)
	}

	return rows, nil
}

// ListCategoriesBySpaceSlug resolves a space's type and returns its ordered
// category template — the public GET /wiki/categories source (replaces the
// hardcoded geo/university category slices).
func (s *Store) ListCategoriesBySpaceSlug(ctx context.Context, spaceSlug string) ([]WikiCategory, error) {
	rows, err := s.read.ListWikiCategoriesBySpaceSlug(ctx, spaceSlug)
	if err != nil {
		return nil, fmt.Errorf("wiki: list categories: %w", err)
	}

	out := make([]WikiCategory, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiCategory{
			ID:             r.ID,
			SpaceTypeSlug:  r.SpaceTypeSlug,
			Slug:           r.Slug,
			Category:       r.Category,
			Label:          r.Label,
			FieldGroupID:   r.FieldGroupID,
			FieldGroupSlug: r.FieldGroupSlug,
			Position:       r.Position,
			IsSystem:       r.IsSystem,
			Rollup:         r.Rollup,
			Icon:           r.Icon,
		})
	}

	return out, nil
}

// ListAllCategories returns the union of every enabled type's category
// template — the public fallback when GET /wiki/categories has no ?space=.
func (s *Store) ListAllCategories(ctx context.Context) ([]WikiCategory, error) {
	rows, err := s.read.ListAllWikiCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("wiki: list all categories: %w", err)
	}

	out := make([]WikiCategory, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiCategory{
			SpaceTypeSlug:  r.SpaceTypeSlug,
			Slug:           r.Slug,
			Category:       r.Category,
			Label:          r.Label,
			FieldGroupID:   r.FieldGroupID,
			FieldGroupSlug: r.FieldGroupSlug,
			Position:       r.Position,
			IsSystem:       r.IsSystem,
			Rollup:         r.Rollup,
			Icon:           r.Icon,
		})
	}

	return out, nil
}

// GetCategoryForSpace resolves a page slug (or category token) for a
// concrete space to its category template row including the bound field
// group's identity. Returns ErrWikiCategoryNotFound when no template
// matches.
func (s *Store) GetCategoryForSpace(ctx context.Context, spaceID pgtype.UUID, slugOrCategory string) (WikiCategory, error) {
	r, err := s.read.GetWikiCategoryForSpace(ctx, gen.GetWikiCategoryForSpaceParams{
		SpaceID: spaceID,
		Slug:    slugOrCategory,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiCategory{}, ErrWikiCategoryNotFound
		}

		return WikiCategory{}, fmt.Errorf("wiki: category for space: %w", err)
	}

	return WikiCategory{
		ID:             r.ID,
		SpaceTypeID:    r.SpaceTypeID,
		Slug:           r.Slug,
		Category:       r.Category,
		Label:          r.Label,
		FieldGroupID:   r.FieldGroupID,
		FieldGroupSlug: r.FieldGroupSlug,
		FieldGroupName: r.FieldGroupName,
		Position:       r.Position,
		IsSystem:       r.IsSystem,
		Rollup:         r.Rollup,
		Icon:           r.Icon,
		CreatedAt:      r.CreatedAt,
	}, nil
}

// ListEnabledWikiTypes returns enabled wiki types for public navigation and
// destination filters.
func (s *Store) ListEnabledWikiTypes(ctx context.Context) ([]WikiType, error) {
	rows, err := s.read.ListEnabledWikiTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("wiki: list enabled types: %w", err)
	}

	out := make([]WikiType, 0, len(rows))
	for _, r := range rows {
		// §80 Hierarchical UX: the public type list carries the declared
		// parent edges — they drive the landing drill-down, breadcrumb
		// arbitration, and rollup descendant scoping.
		parents := r.ParentTypeIds

		out = append(out, WikiType{
			SpaceTypeID:   r.SpaceTypeID,
			SpaceTypeSlug: r.SpaceTypeSlug,
			SpaceTypeName: r.SpaceTypeName,
			Enabled:       true,
			Label:         r.Label,
			Icon:          r.Icon,
			SeedStubs:     r.SeedStubs,
			Position:      r.Position,
			ParentTypeIDs: parents,
		})
	}

	return out, nil
}

// ListWikiTypes returns every wiki type (admin view) with parent edges.
func (s *Store) ListWikiTypes(ctx context.Context) ([]WikiType, error) {
	rows, err := s.read.ListWikiTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("wiki: list types: %w", err)
	}

	out := make([]WikiType, 0, len(rows))
	for _, r := range rows {
		label := r.SpaceTypeName
		if r.Label != nil && *r.Label != "" {
			label = *r.Label
		}

		parents := r.ParentTypeIds

		out = append(out, WikiType{
			SpaceTypeID:   r.SpaceTypeID,
			SpaceTypeSlug: r.SpaceTypeSlug,
			SpaceTypeName: r.SpaceTypeName,
			Enabled:       r.Enabled,
			Label:         label,
			Icon:          r.Icon,
			SeedStubs:     r.SeedStubs,
			Position:      r.Position,
			ParentTypeIDs: parents,
			CreatedAt:     r.CreatedAt,
		})
	}

	return out, nil
}

// GetWikiType returns one wiki type config by space_type id.
func (s *Store) GetWikiType(ctx context.Context, spaceTypeID pgtype.UUID) (WikiType, error) {
	return s.getWikiType(ctx, s.read, spaceTypeID)
}

// getWikiType reads a wiki type through the given query handle — callers
// needing read-your-writes (UpsertWikiType's post-commit re-read) pass the
// write pool so a read replica's lag can't shadow a fresh write.
func (s *Store) getWikiType(ctx context.Context, q *gen.Queries, spaceTypeID pgtype.UUID) (WikiType, error) {
	r, err := q.GetWikiTypeConfig(ctx, spaceTypeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiType{}, ErrWikiTypeNotFound
		}

		return WikiType{}, fmt.Errorf("wiki: get type config: %w", err)
	}

	parentIDs, err := q.ListWikiTypeParentIDs(ctx, spaceTypeID)
	if err != nil {
		return WikiType{}, fmt.Errorf("wiki: list type parents: %w", err)
	}

	parents := make([]string, 0, len(parentIDs))
	for _, id := range parentIDs {
		parents = append(parents, id.String())
	}

	label := r.SpaceTypeName
	if r.Label != nil && *r.Label != "" {
		label = *r.Label
	}

	return WikiType{
		SpaceTypeID:   r.SpaceTypeID,
		SpaceTypeSlug: r.SpaceTypeSlug,
		SpaceTypeName: r.SpaceTypeName,
		Enabled:       r.Enabled,
		Label:         label,
		Icon:          r.Icon,
		SeedStubs:     r.SeedStubs,
		Position:      r.Position,
		ParentTypeIDs: parents,
		CreatedAt:     r.CreatedAt,
	}, nil
}

// UpsertWikiType creates or re-enables a wiki type for a space type.
// parentIDs replaces the full parent-edge set inside one transaction.
func (s *Store) UpsertWikiType(ctx context.Context, spaceTypeID pgtype.UUID, enabled bool, label, icon *string, seedStubs bool, position int32, parentIDs []pgtype.UUID) (WikiType, error) {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return WikiType{}, fmt.Errorf("wiki: upsert type: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := s.write.WithTx(tx)

	if _, err := q.UpsertWikiTypeConfig(ctx, gen.UpsertWikiTypeConfigParams{
		SpaceTypeID: spaceTypeID,
		Enabled:     enabled,
		Label:       label,
		Icon:        icon,
		SeedStubs:   seedStubs,
		Position:    position,
	}); err != nil {
		return WikiType{}, fmt.Errorf("wiki: upsert type config: %w", err)
	}

	if err := replaceWikiTypeParents(ctx, q, spaceTypeID, parentIDs); err != nil {
		return WikiType{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WikiType{}, fmt.Errorf("wiki: upsert type: commit: %w", err)
	}

	// Re-read through the joined query so the response carries the space
	// type's slug/name alongside the config — via the write pool so a read
	// replica's lag can't shadow the just-committed row.
	out, err := s.getWikiType(ctx, s.write, spaceTypeID)
	if err != nil {
		return WikiType{}, fmt.Errorf("wiki: upsert type: re-read: %w", err)
	}

	return out, nil
}

// UpdateWikiType patches the optional config fields; parent edges are
// replaced only when parentIDs is non-nil (nil = keep existing edges).
func (s *Store) UpdateWikiType(ctx context.Context, spaceTypeID pgtype.UUID, enabled *bool, label, icon *string, seedStubs *bool, position *int32, parentIDs []pgtype.UUID) error {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("wiki: update type: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := s.write.WithTx(tx)

	affected, err := q.UpdateWikiTypeConfig(ctx, gen.UpdateWikiTypeConfigParams{
		Enabled:     enabled,
		Label:       label,
		Icon:        icon,
		SeedStubs:   seedStubs,
		Position:    position,
		SpaceTypeID: spaceTypeID,
	})
	if err != nil {
		return fmt.Errorf("wiki: update type config: %w", err)
	}

	if affected == 0 {
		return ErrWikiTypeNotFound
	}

	if parentIDs != nil {
		if err := replaceWikiTypeParents(ctx, q, spaceTypeID, parentIDs); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("wiki: update type: commit: %w", err)
	}

	return nil
}

func replaceWikiTypeParents(ctx context.Context, q *gen.Queries, spaceTypeID pgtype.UUID, parentIDs []pgtype.UUID) error {
	if err := q.DeleteWikiTypeParents(ctx, spaceTypeID); err != nil {
		return fmt.Errorf("wiki: clear type parents: %w", err)
	}

	for _, pid := range parentIDs {
		if err := q.InsertWikiTypeParent(ctx, gen.InsertWikiTypeParentParams{
			SpaceTypeID:       spaceTypeID,
			ParentSpaceTypeID: pid,
		}); err != nil {
			return fmt.Errorf("wiki: insert type parent: %w", err)
		}
	}

	return nil
}

// DeleteWikiType removes a wiki type config (categories and parent edges
// cascade). The caller guards against deleting types that still have active
// wiki pages — see CountActivePagesBySpaceType.
func (s *Store) DeleteWikiType(ctx context.Context, spaceTypeID pgtype.UUID) error {
	affected, err := s.write.DeleteWikiTypeConfig(ctx, spaceTypeID)
	if err != nil {
		return fmt.Errorf("wiki: delete type: %w", err)
	}

	if affected == 0 {
		return ErrWikiTypeNotFound
	}

	return nil
}

// CountActivePagesBySpaceType is the delete-type guard: active wiki pages on
// spaces of this type. Reads through the write pool — a replica lagging a
// just-created page would let the delete proceed and orphan it.
func (s *Store) CountActivePagesBySpaceType(ctx context.Context, spaceTypeID pgtype.UUID) (int64, error) {
	n, err := s.write.CountActivePagesBySpaceType(ctx, spaceTypeID)
	if err != nil {
		return 0, fmt.Errorf("wiki: count pages by type: %w", err)
	}

	return n, nil
}

// ListWikiTypeCategories returns a type's category template with the bound
// group's slug/name for admin display.
func (s *Store) ListWikiTypeCategories(ctx context.Context, spaceTypeID pgtype.UUID) ([]WikiCategory, error) {
	rows, err := s.read.ListWikiTypeCategories(ctx, spaceTypeID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list type categories: %w", err)
	}

	out := make([]WikiCategory, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiCategory{
			ID:             r.ID,
			SpaceTypeID:    r.SpaceTypeID,
			Slug:           r.Slug,
			Category:       r.Category,
			Label:          r.Label,
			FieldGroupID:   r.FieldGroupID,
			FieldGroupSlug: r.FieldGroupSlug,
			FieldGroupName: r.FieldGroupName,
			Position:       r.Position,
			IsSystem:       r.IsSystem,
			Rollup:         r.Rollup,
			Icon:           r.Icon,
			CreatedAt:      r.CreatedAt,
		})
	}

	return out, nil
}

// GetWikiTypeCategory returns one category row by id (admin edit/delete
// guards need the slug + space_type_id first).
func (s *Store) GetWikiTypeCategory(ctx context.Context, id pgtype.UUID) (WikiCategory, error) {
	r, err := s.read.GetWikiTypeCategory(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiCategory{}, ErrWikiCategoryNotFound
		}

		return WikiCategory{}, fmt.Errorf("wiki: get category: %w", err)
	}

	return WikiCategory{
		ID:           r.ID,
		SpaceTypeID:  r.SpaceTypeID,
		Slug:         r.Slug,
		Category:     r.Category,
		Label:        r.Label,
		FieldGroupID: r.FieldGroupID,
		Position:     r.Position,
		IsSystem:     r.IsSystem,
		Rollup:       r.Rollup,
		Icon:         r.Icon,
		CreatedAt:    r.CreatedAt,
	}, nil
}

// CreateWikiTypeCategory adds a category to a type's template.
func (s *Store) CreateWikiTypeCategory(ctx context.Context, spaceTypeID pgtype.UUID, slug, category, label string, fieldGroupID pgtype.UUID, position int32, isSystem bool, rollup []byte, icon *string) (WikiCategory, error) {
	r, err := s.write.CreateWikiTypeCategory(ctx, gen.CreateWikiTypeCategoryParams{
		SpaceTypeID:  spaceTypeID,
		Slug:         slug,
		Category:     category,
		Label:        label,
		FieldGroupID: fieldGroupID,
		Position:     position,
		IsSystem:     isSystem,
		Rollup:       rollup,
		Icon:         icon,
	})
	if err != nil {
		// Unique collision on (space_type_id, slug|category) → 409, not 500.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return WikiCategory{}, fmt.Errorf("%w: %s", ErrWikiCategoryExists, pgErr.ConstraintName)
		}

		return WikiCategory{}, fmt.Errorf("wiki: create category: %w", err)
	}

	return WikiCategory{
		ID:           r.ID,
		SpaceTypeID:  r.SpaceTypeID,
		Slug:         r.Slug,
		Category:     r.Category,
		Label:        r.Label,
		FieldGroupID: r.FieldGroupID,
		Position:     r.Position,
		IsSystem:     r.IsSystem,
		Rollup:       r.Rollup,
		Icon:         r.Icon,
		CreatedAt:    r.CreatedAt,
	}, nil
}

// UpdateWikiTypeCategory patches label, bound group, position, and icon —
// slug and category are immutable (page URLs are /wiki/{entity}/{slug}).
// updateFieldGroup distinguishes "don't touch" from "rebind": when true,
// fieldGroupID replaces the binding (the column is NOT NULL — callers must
// pass a validated group id, never a zero UUID).
func (s *Store) UpdateWikiTypeCategory(ctx context.Context, id pgtype.UUID, label *string, updateFieldGroup bool, fieldGroupID pgtype.UUID, position *int32, updateRollup bool, rollup []byte, updateIcon bool, icon *string) error {
	affected, err := s.write.UpdateWikiTypeCategory(ctx, gen.UpdateWikiTypeCategoryParams{
		Label:            label,
		UpdateFieldGroup: updateFieldGroup,
		FieldGroupID:     fieldGroupID,
		Position:         position,
		UpdateRollup:     updateRollup,
		Rollup:           rollup,
		UpdateIcon:       updateIcon,
		Icon:             icon,
		ID:               id,
	})
	if err != nil {
		return fmt.Errorf("wiki: update category: %w", err)
	}

	if affected == 0 {
		return ErrWikiCategoryNotFound
	}

	return nil
}

// DeleteWikiTypeCategory removes a non-system category. The query's
// is_system=false predicate makes system categories undeletable; a zero
// affected count means the row was missing or protected — the caller checks
// GetWikiTypeCategory first to distinguish.
func (s *Store) DeleteWikiTypeCategory(ctx context.Context, id pgtype.UUID) error {
	affected, err := s.write.DeleteWikiTypeCategory(ctx, id)
	if err != nil {
		return fmt.Errorf("wiki: delete category: %w", err)
	}

	if affected == 0 {
		return ErrWikiCategoryNotFound
	}

	return nil
}

// CountCategoryProsePages is the delete-category guard: pages in this
// category (any space of the type) that carry an approved content/mixed
// revision — i.e. real prose that deletion would orphan. Reads through the
// write pool so a lagging replica can't mask fresh prose.
func (s *Store) CountCategoryProsePages(ctx context.Context, category string, spaceTypeID pgtype.UUID) (int64, error) {
	n, err := s.write.CountCategoryProsePages(ctx, gen.CountCategoryProsePagesParams{
		Category:    category,
		SpaceTypeID: spaceTypeID,
	})
	if err != nil {
		return 0, fmt.Errorf("wiki: count category prose: %w", err)
	}

	return n, nil
}

// ---------------------------------------------------------------------------
// Page templates (§80 Hierarchical UX & Itemized Pricing amendment)
// ---------------------------------------------------------------------------

// WikiPageTemplate is one row of a category's page template — a named
// sibling page (e.g. "prices" under cost_of_living) binding a shared
// type-level field group so item keys stay comparable across spaces.
// §80 Wiki Topic Pages: ParentTemplateID nests the template under a
// same-category top-level template (max depth 1 — the parent must itself
// have no parent). SectionLabel labels the synthesized composite section
// the child contributes to the parent's page; PreviewLimit caps the rows
// shown in that preview (0 = all).
type WikiPageTemplate struct {
	ID               pgtype.UUID
	CategoryID       pgtype.UUID
	Slug             string
	Label            string
	FieldGroupID     pgtype.UUID
	FieldGroupSlug   *string
	FieldGroupName   *string
	ParentTemplateID pgtype.UUID // invalid → top-level
	SectionLabel     *string     // nil → Label is used for the composite section
	PreviewLimit     int32
	Position         int32
	IsSystem         bool
	CreatedAt        pgtype.Timestamptz
}

// ErrWikiPageTemplateNotFound is returned when a wiki_type_page_templates
// row does not exist.
var ErrWikiPageTemplateNotFound = errors.New("wiki page template not found")

// ErrWikiPageTemplateExists is returned when a template's slug collides
// with an existing template row on the same category.
var ErrWikiPageTemplateExists = errors.New("wiki page template already exists")

// ErrWikiPageTemplateProtected is returned when mutating a system template.
var ErrWikiPageTemplateProtected = errors.New("wiki page template is system-protected")

// ErrWikiPageTemplateInUse is returned when deleting a template whose bound
// pages carry approved prose.
var ErrWikiPageTemplateInUse = errors.New("wiki page template has pages with content")

func pageTemplateFromRow(id, categoryID pgtype.UUID, slug, label string, fieldGroupID pgtype.UUID, fgSlug, fgName *string, parentID pgtype.UUID, sectionLabel *string, previewLimit, position int32, isSystem bool, createdAt pgtype.Timestamptz) WikiPageTemplate {
	return WikiPageTemplate{
		ID: id, CategoryID: categoryID, Slug: slug, Label: label,
		FieldGroupID: fieldGroupID, FieldGroupSlug: fgSlug, FieldGroupName: fgName,
		ParentTemplateID: parentID, SectionLabel: sectionLabel, PreviewLimit: previewLimit,
		Position: position, IsSystem: isSystem, CreatedAt: createdAt,
	}
}

// ListPageTemplates returns a category's ordered page templates with bound
// group identities (admin editor).
func (s *Store) ListPageTemplates(ctx context.Context, categoryID pgtype.UUID) ([]WikiPageTemplate, error) {
	rows, err := s.read.ListWikiPageTemplates(ctx, categoryID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list page templates: %w", err)
	}

	out := make([]WikiPageTemplate, 0, len(rows))
	for _, r := range rows {
		out = append(out, pageTemplateFromRow(r.ID, r.CategoryID, r.Slug, r.Label, r.FieldGroupID, r.FieldGroupSlug, r.FieldGroupName, r.ParentTemplateID, r.SectionLabel, r.PreviewLimit, r.Position, r.IsSystem, r.CreatedAt))
	}

	return out, nil
}

// ListPageTemplatesBySpaceType returns every page template across a type's
// categories — one fetch for the admin editor's per-category template lists.
func (s *Store) ListPageTemplatesBySpaceType(ctx context.Context, spaceTypeID pgtype.UUID) ([]WikiPageTemplate, error) {
	rows, err := s.read.ListWikiPageTemplatesBySpaceType(ctx, spaceTypeID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list page templates by type: %w", err)
	}

	out := make([]WikiPageTemplate, 0, len(rows))
	for _, r := range rows {
		out = append(out, pageTemplateFromRow(r.ID, r.CategoryID, r.Slug, r.Label, r.FieldGroupID, r.FieldGroupSlug, r.FieldGroupName, r.ParentTemplateID, r.SectionLabel, r.PreviewLimit, r.Position, r.IsSystem, r.CreatedAt))
	}

	return out, nil
}

// GetPageTemplate returns one template row by id.
func (s *Store) GetPageTemplate(ctx context.Context, id pgtype.UUID) (WikiPageTemplate, error) {
	r, err := s.read.GetWikiPageTemplate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiPageTemplate{}, ErrWikiPageTemplateNotFound
		}

		return WikiPageTemplate{}, fmt.Errorf("wiki: get page template: %w", err)
	}

	return pageTemplateFromRow(r.ID, r.CategoryID, r.Slug, r.Label, r.FieldGroupID, nil, nil, r.ParentTemplateID, r.SectionLabel, r.PreviewLimit, r.Position, r.IsSystem, r.CreatedAt), nil
}

// GetPageTemplateForSpace resolves a (category, slug) pair to the space
// type's page template — the binding wiki_pages rows store at create.
// Category scoping keeps resolution deterministic when two categories on
// one type declare the same template slug. Returns
// ErrWikiPageTemplateNotFound for freeform (untemplated) slugs.
func (s *Store) GetPageTemplateForSpace(ctx context.Context, spaceID pgtype.UUID, category, slug string) (WikiPageTemplate, error) {
	r, err := s.read.GetWikiPageTemplateForSpace(ctx, gen.GetWikiPageTemplateForSpaceParams{
		SpaceID:  spaceID,
		Slug:     slug,
		Category: category,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiPageTemplate{}, ErrWikiPageTemplateNotFound
		}

		return WikiPageTemplate{}, fmt.Errorf("wiki: page template for space: %w", err)
	}

	return pageTemplateFromRow(r.ID, r.CategoryID, r.Slug, r.Label, r.FieldGroupID, nil, nil, r.ParentTemplateID, r.SectionLabel, r.PreviewLimit, r.Position, r.IsSystem, r.CreatedAt), nil
}

// WikiPageTemplateRef is a page template plus its owning category's token
// — the lazy-page read needs the category to set the synthesized page's
// `category` (siblings and the field-group fallback key on it).
type WikiPageTemplateRef struct {
	WikiPageTemplate
	// CategoryToken is the owning category's wiki_pages.category value.
	CategoryToken string
}

// GetPageTemplateBySlug resolves a slug-only template across all of the
// space type's categories (§80 lazy pages — the /wiki/{space}/{slug} URL
// carries no category). Category/template position ordering keeps the pick
// deterministic when two categories reuse a slug.
func (s *Store) GetPageTemplateBySlug(ctx context.Context, spaceID pgtype.UUID, slug string) (WikiPageTemplateRef, error) {
	r, err := s.read.GetWikiPageTemplateBySlug(ctx, gen.GetWikiPageTemplateBySlugParams{
		SpaceID: spaceID,
		Slug:    slug,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiPageTemplateRef{}, ErrWikiPageTemplateNotFound
		}

		return WikiPageTemplateRef{}, fmt.Errorf("wiki: page template by slug: %w", err)
	}

	return WikiPageTemplateRef{
		WikiPageTemplate: pageTemplateFromRow(r.ID, r.CategoryID, r.Slug, r.Label, r.FieldGroupID, r.FieldGroupSlug, nil, r.ParentTemplateID, r.SectionLabel, r.PreviewLimit, r.Position, r.IsSystem, r.CreatedAt),
		CategoryToken:    r.CategoryToken,
	}, nil
}

// CreatePageTemplate adds a page template to a category. parentID nests the
// template under a same-category top-level template (§80 Wiki Topic Pages);
// the service validates parentage before calling.
func (s *Store) CreatePageTemplate(ctx context.Context, categoryID pgtype.UUID, slug, label string, fieldGroupID, parentID pgtype.UUID, sectionLabel *string, previewLimit *int32, position int32, isSystem bool) (WikiPageTemplate, error) {
	r, err := s.write.CreateWikiPageTemplate(ctx, gen.CreateWikiPageTemplateParams{
		CategoryID:       categoryID,
		Slug:             slug,
		Label:            label,
		FieldGroupID:     fieldGroupID,
		ParentTemplateID: parentID,
		SectionLabel:     sectionLabel,
		PreviewLimit:     previewLimit,
		Position:         position,
		IsSystem:         isSystem,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return WikiPageTemplate{}, fmt.Errorf("%w: %s", ErrWikiPageTemplateExists, pgErr.ConstraintName)
		}

		return WikiPageTemplate{}, fmt.Errorf("wiki: create page template: %w", err)
	}

	return pageTemplateFromRow(r.ID, r.CategoryID, r.Slug, r.Label, r.FieldGroupID, nil, nil, r.ParentTemplateID, r.SectionLabel, r.PreviewLimit, r.Position, r.IsSystem, r.CreatedAt), nil
}

// UpdatePageTemplate patches label, bound group, position, parentage, and
// preview config — slug is immutable (page URLs). updateFieldGroup
// distinguishes "don't touch" from "rebind" (field_group_id is NOT NULL —
// pass a validated id, never zero). updateParent does the same for
// parentage (invalid parentID clears → top-level); updateSectionLabel the
// same for the composite-section label (nil clears → the child's label
// renders).
func (s *Store) UpdatePageTemplate(ctx context.Context, id pgtype.UUID, label *string, updateFieldGroup bool, fieldGroupID pgtype.UUID, position *int32, updateParent bool, parentID pgtype.UUID, updateSectionLabel bool, sectionLabel *string, previewLimit *int32) error {
	affected, err := s.write.UpdateWikiPageTemplate(ctx, gen.UpdateWikiPageTemplateParams{
		Label:              label,
		UpdateFieldGroup:   updateFieldGroup,
		FieldGroupID:       fieldGroupID,
		Position:           position,
		UpdateParent:       updateParent,
		ParentTemplateID:   parentID,
		UpdateSectionLabel: updateSectionLabel,
		SectionLabel:       sectionLabel,
		PreviewLimit:       previewLimit,
		ID:                 id,
	})
	if err != nil {
		return fmt.Errorf("wiki: update page template: %w", err)
	}

	if affected == 0 {
		return ErrWikiPageTemplateNotFound
	}

	return nil
}

// DeletePageTemplate removes a non-system template. Bound pages keep their
// field_group_id (the page's schema survives — §80: deleting a template
// must not invalidate existing page bindings).
func (s *Store) DeletePageTemplate(ctx context.Context, id pgtype.UUID) error {
	affected, err := s.write.DeleteWikiPageTemplate(ctx, id)
	if err != nil {
		return fmt.Errorf("wiki: delete page template: %w", err)
	}

	if affected == 0 {
		return ErrWikiPageTemplateNotFound
	}

	return nil
}

// CountTemplateContentPages is the delete-template guard: pages bound to
// the template carrying approved prose/field revisions or stored values in
// the template's group. Reads through the write pool so a lagging replica
// can't mask fresh content.
func (s *Store) CountTemplateContentPages(ctx context.Context, id pgtype.UUID) (int64, error) {
	n, err := s.write.CountTemplateContentPages(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("wiki: count template content: %w", err)
	}

	return n, nil
}

// ResolveCategoryToken canonicalizes a category token/slug to the type's
// managed category row — the rollup source-group lookup by
// (space_type_id, category) (§80 Hierarchical UX).
func (s *Store) ResolveCategoryToken(ctx context.Context, spaceTypeID pgtype.UUID, token string) (WikiCategory, error) {
	r, err := s.read.ResolveWikiTypeCategoryToken(ctx, gen.ResolveWikiTypeCategoryTokenParams{
		SpaceTypeID: spaceTypeID,
		Token:       token,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiCategory{}, ErrWikiCategoryNotFound
		}

		return WikiCategory{}, fmt.Errorf("wiki: resolve category token: %w", err)
	}

	return WikiCategory{
		ID:           r.ID,
		SpaceTypeID:  r.SpaceTypeID,
		Slug:         r.Slug,
		Category:     r.Category,
		Label:        r.Label,
		FieldGroupID: r.FieldGroupID,
		Position:     r.Position,
		IsSystem:     r.IsSystem,
		Rollup:       r.Rollup,
		Icon:         r.Icon,
		CreatedAt:    r.CreatedAt,
	}, nil
}
