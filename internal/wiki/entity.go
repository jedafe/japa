package wiki

// Entity-bound wiki pages (JCE ↔ Wiki field-group mapping, spec
// 2026-10-20-jce-wiki-field-group-mapping-design.md): wiki types whose
// pages bind real entity rows (visa_routes, organizations, course
// offerings, scholarships, embassies) instead of spaces. The space_types
// row is only the type key; wiki_type_configs.entity_kind /
// entity_discriminator declare the binding.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// EntityRef identifies an entity row: slug for most kinds, id for kinds
// without a slug column (scholarship).
type EntityRef struct {
	Kind          string
	ID            pgtype.UUID
	Slug          string
	Name          string
	ParentSpaceID pgtype.UUID // hosting space (country/university), nullable
	Discriminator string      // actual discriminator value on the row (visa_category, org_type…)
}

// EntityChild is a destination-tree row: one entity child of a host space.
type EntityChild struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Discriminator string `json:"discriminator,omitempty"`
	// ProgramSlug links a course offering to its canonical catalog wiki
	// page (/wiki/e/program/{slug}) — empty for other kinds.
	ProgramSlug string `json:"program_slug,omitempty"`
}

// discriminatorValue returns the first value of an entity_discriminator
// {column: value} map; "" when empty or malformed.
func discriminatorValue(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, v := range m {
		return v
	}
	return ""
}

// GetEntityKindConfig resolves the wiki type config for a type key slug
// (e.g. 'student_visa'). Returns the row incl. entity_kind + discriminator.
func (s *Store) GetEntityKindConfig(ctx context.Context, typeSlug string) (gen.GetWikiTypeConfigForKindRow, error) {
	return s.read.GetWikiTypeConfigForKind(ctx, typeSlug)
}

// ResolveEntityRef resolves an entity ref (slug, or id for slug-less
// kinds) against the kind's source table.
func (s *Store) ResolveEntityRef(ctx context.Context, kind, ref string) (EntityRef, error) {
	out := EntityRef{Kind: kind, Slug: ref}
	var err error
	switch kind {
	case "visa_route":
		var r gen.GetVisaRouteForWikiRow
		r, err = s.read.GetVisaRouteForWiki(ctx, ref)
		if err == nil {
			out.ID, out.Name, out.ParentSpaceID, out.Discriminator = r.ID, r.Name, r.DestinationSpaceID, r.VisaCategory
		}
	case "organization":
		var r gen.GetOrganizationForWikiRow
		r, err = s.read.GetOrganizationForWiki(ctx, ref)
		if err == nil {
			out.ID, out.Name, out.ParentSpaceID, out.Discriminator = r.ID, r.Name, r.GeoSpaceID, r.OrgType
		}
	case "embassy":
		var r gen.GetEmbassyForWikiRow
		r, err = s.read.GetEmbassyForWiki(ctx, ref)
		if err == nil {
			out.ID, out.Name, out.ParentSpaceID = r.ID, r.Name, r.HostSpaceID
		}
	case "course":
		var r gen.GetCourseOfferingForWikiRow
		r, err = s.read.GetCourseOfferingForWiki(ctx, ref)
		if err == nil {
			out.ID, out.Name, out.ParentSpaceID = r.ID, r.Name, r.UniversityID
		}
	case "program":
		var r gen.GetProgramForWikiRow
		r, err = s.read.GetProgramForWiki(ctx, ref)
		if err == nil {
			out.ID, out.Name = r.ID, r.Name
		}
	case "scholarship":
		var r gen.GetScholarshipForWikiRow
		r, err = s.read.GetScholarshipForWiki(ctx, ref)
		if err == nil {
			out.ID, out.Name, out.ParentSpaceID = r.ID, r.Title, r.CountrySpaceID
		}
	default:
		return out, fmt.Errorf("wiki: unknown entity kind %q", kind)
	}
	return out, err
}

// ListEntityChildren lists entity rows of a kind hosted by a space —
// the destination tree's entity children. discriminator filters
// source-table slices (visa kinds, bank orgs); empty string = all rows.
func (s *Store) ListEntityChildren(ctx context.Context, spaceID pgtype.UUID, kind, discriminator string) ([]EntityChild, error) {
	switch kind {
	case "visa_route":
		rows, err := s.read.ListVisaRoutesForWiki(ctx, gen.ListVisaRoutesForWikiParams{SpaceID: spaceID, VisaCategory: discriminator})
		if err != nil {
			return nil, err
		}
		out := make([]EntityChild, 0, len(rows))
		for _, r := range rows {
			out = append(out, EntityChild{ID: uuidStr(r.ID), Slug: r.Slug, Name: r.Name, Discriminator: r.VisaCategory})
		}
		return out, nil
	case "organization":
		rows, err := s.read.ListOrganizationsForWiki(ctx, gen.ListOrganizationsForWikiParams{SpaceID: spaceID, OrgType: discriminator})
		if err != nil {
			return nil, err
		}
		out := make([]EntityChild, 0, len(rows))
		for _, r := range rows {
			out = append(out, EntityChild{ID: uuidStr(r.ID), Slug: r.Slug, Name: r.Name, Discriminator: r.OrgType})
		}
		return out, nil
	case "embassy":
		rows, err := s.read.ListEmbassiesForWiki(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		out := make([]EntityChild, 0, len(rows))
		for _, r := range rows {
			out = append(out, EntityChild{ID: uuidStr(r.ID), Slug: r.Slug, Name: r.Name})
		}
		return out, nil
	case "course":
		rows, err := s.read.ListCourseOfferingsForWiki(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		out := make([]EntityChild, 0, len(rows))
		for _, r := range rows {
			out = append(out, EntityChild{ID: uuidStr(r.ID), Slug: r.Slug, Name: r.Name, ProgramSlug: r.ProgramSlug})
		}
		return out, nil
	case "scholarship":
		rows, err := s.read.ListScholarshipsForWiki(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		out := make([]EntityChild, 0, len(rows))
		for _, r := range rows {
			out = append(out, EntityChild{ID: uuidStr(r.ID), Slug: r.Slug, Name: r.Title})
		}
		return out, nil
	}
	return nil, fmt.Errorf("wiki: unknown entity kind %q", kind)
}

func uuidStr(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b, _ := id.MarshalJSON()
	var s string
	_ = json.Unmarshal(b, &s)
	return s
}

// EntityPageDTO is the entity-bound counterpart of WikiPageDetailDTO.
type EntityPageDTO struct {
	ID             string               `json:"id"`
	Kind           string               `json:"kind"`
	TypeSlug       string               `json:"type_slug"`
	EntityID       string               `json:"entity_id"`
	EntitySlug     string               `json:"entity_slug"`
	EntityName     string               `json:"entity_name"`
	Slug           string               `json:"slug"`
	Title          string               `json:"title"`
	Category       string               `json:"category"`
	IsLocked       bool                 `json:"is_locked"`
	IsFeatured     bool                 `json:"is_featured"`
	Lazy           bool                 `json:"lazy"`
	Content        json.RawMessage      `json:"content,omitempty"`
	FieldGroupID   *string              `json:"field_group_id,omitempty"`
	FieldGroupSlug string               `json:"field_group_slug,omitempty"`
	FieldGroupName string               `json:"field_group_name,omitempty"`
	Fields         *EntityFieldGroupDTO `json:"fields,omitempty"`
	// Parity with WikiPageDetailDTO: revision metadata, reader feedback,
	// sibling pages on the same entity, and host-space breadcrumbs.
	RevisionSummary string             `json:"revision_summary,omitempty"`
	SubmittedBy     string             `json:"submitted_by,omitempty"`
	Helpful         *WikiHelpfulDTO    `json:"helpful,omitempty"`
	Siblings        []EntitySiblingDTO `json:"siblings,omitempty"`
	Ancestors       []WikiSpaceRefDTO  `json:"ancestors,omitempty"`
	AncestorsAlt    []WikiSpaceRefDTO  `json:"ancestors_alt,omitempty"`
	// Related is the mode:"related" peer listing (course → sibling-offering
	// universities, program → all offering universities); nil when the
	// category has no related config or the listing is empty.
	Related *EntityRelatedDTO `json:"related,omitempty"`
}

// EntitySiblingDTO is a sibling page on the same entity (index links).
type EntitySiblingDTO struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Category string `json:"category"`
}

// WikiRelatedSpaceDTO is one card in an entity's related-universities
// grid — the space identity plus the linking offering's wiki slug.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiRelatedSpaceDTO struct {
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	GeoCode      *string `json:"geo_code,omitempty"`
	OfferingSlug string  `json:"offering_slug,omitempty"`
}

// EntityRelatedDTO is the peer listing attached to a mode:"related"
// entity category page. Kind names the relation ("universities").
// //nolint:revive // exported: DTO naming follows package convention.
type EntityRelatedDTO struct {
	Kind  string                `json:"kind"`
	Items []WikiRelatedSpaceDTO `json:"items"`
	Total int                   `json:"total"`
}

// GetEntityWikiPage resolves GET /wiki/e/{type_slug}/{entity}/{page_slug}.
// Materialized rows win; a slug matching the type's categories or page
// templates returns a lazy (unmaterialized) page — entity pages render
// lazily like space pages. Anything else is a real 404.
func (s *Service) GetEntityWikiPage(ctx context.Context, userID pgtype.UUID, typeSlug, entityRef, pageSlug string) (EntityPageDTO, error) {
	cfg, err := s.store.GetEntityKindConfig(ctx, typeSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EntityPageDTO{}, ErrSpaceNotFound
		}
		return EntityPageDTO{}, fmt.Errorf("wiki: entity page: resolve type %q: %w", typeSlug, err)
	}
	if cfg.EntityKind == nil {
		return EntityPageDTO{}, ErrSpaceNotFound // type key exists but is space-bound
	}
	kind := *cfg.EntityKind

	ref, err := s.store.ResolveEntityRef(ctx, kind, entityRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EntityPageDTO{}, pgx.ErrNoRows
		}
		return EntityPageDTO{}, fmt.Errorf("wiki: entity page: resolve %s %q: %w", kind, entityRef, err)
	}

	// Discriminator check: 'student_visa' must not serve a 'work' route.
	if len(cfg.EntityDiscriminator) > 0 {
		var disc map[string]string
		if err := json.Unmarshal(cfg.EntityDiscriminator, &disc); err == nil {
			for _, want := range disc {
				if ref.Discriminator != "" && ref.Discriminator != want {
					return EntityPageDTO{}, pgx.ErrNoRows
				}
			}
		}
	}

	page, err := s.store.read.GetEntityWikiPage(ctx, gen.GetEntityWikiPageParams{
		EntityType: &kind,
		EntityID:   ref.ID,
		PageSlug:   pageSlug,
	})
	if err == nil {
		dto := EntityPageDTO{
			ID: uuidStr(page.ID), Kind: kind, TypeSlug: typeSlug,
			EntityID: uuidStr(ref.ID), EntitySlug: ref.Slug, EntityName: ref.Name,
			Slug: page.Slug, Title: page.Title, Category: page.Category,
			IsLocked: page.IsLocked, IsFeatured: page.IsFeatured,
		}
		if rev, rerr := s.store.GetApprovedRevision(ctx, page.ID); rerr == nil {
			dto.Content = json.RawMessage(rev.Content)
			if rev.Summary != nil {
				dto.RevisionSummary = *rev.Summary
			}
			if rev.SubmittedByName != nil {
				dto.SubmittedBy = *rev.SubmittedByName
			}
		}
		dto.Helpful = s.pageFeedbackDTO(ctx, page.ID, userID)
		s.attachEntityContext(ctx, userID, cfg.SpaceTypeID, kind, ref, page.ID, &dto)
		return dto, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return EntityPageDTO{}, fmt.Errorf("wiki: entity page: %w", err)
	}

	// Lazy: a row that exists but is inactive/deleted is a real 404 —
	// same guard as the space path.
	if exists, xerr := s.store.read.EntityWikiPageRowExists(ctx, gen.EntityWikiPageRowExistsParams{
		EntityType: &kind, EntityID: ref.ID, PageSlug: pageSlug,
	}); xerr == nil && exists {
		return EntityPageDTO{}, ErrPageDeleted
	}

	// Lazy via the type's categories / page templates.
	lazy, lerr := s.entityLazyPage(ctx, userID, cfg.SpaceTypeID, kind, typeSlug, ref, pageSlug)
	if lerr != nil {
		return EntityPageDTO{}, lerr
	}
	return lazy, nil
}

// attachEntityFields hydrates the page's bound field group with the
// entity's cfv — the entity-bound counterpart of the space page's
// fields block. The host space (ref.ParentSpaceID) supplies §58
// currency defaults.
func (s *Service) attachEntityContext(ctx context.Context, userID, typeID pgtype.UUID, kind string, ref EntityRef, pageID pgtype.UUID, dto *EntityPageDTO) {
	s.attachEntityFields(ctx, userID, typeID, kind, ref, dto)

	// Sibling pages on the same entity — the tab strip of categories.
	if rows, err := s.store.read.ListEntityWikiPages(ctx, gen.ListEntityWikiPagesParams{
		EntityType: &kind, EntityID: ref.ID,
	}); err == nil {
		for _, r := range rows {
			if r.ID == pageID {
				continue
			}
			dto.Siblings = append(dto.Siblings, EntitySiblingDTO{Slug: r.Slug, Title: r.Title, Category: r.Category})
		}
	}

	// Host-space ancestors — the entity sits under its host space in the
	// tree (a visa route's breadcrumbs are its country's).
	if ref.ParentSpaceID.Valid {
		if hostTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, ref.ParentSpaceID); terr == nil {
			dto.Ancestors, dto.AncestorsAlt = s.getAncestors(ctx, ref.ParentSpaceID, hostTypeID)
		}
	}

	// mode:"related" categories attach the entity's peer listing — the
	// category row's rollup decides (a course's 'universities' page lists
	// sibling-offering universities, a program's lists all offering unis).
	if cat, err := s.store.read.ResolveWikiTypeCategoryToken(ctx, gen.ResolveWikiTypeCategoryTokenParams{
		SpaceTypeID: typeID, Token: dto.Category,
	}); err == nil {
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && rcfg.Mode == rollupModeRelated {
			dto.Related = s.computeEntityRelated(ctx, kind, ref)
		}
	}
}

func (s *Service) attachEntityFields(ctx context.Context, userID, typeID pgtype.UUID, kind string, ref EntityRef, dto *EntityPageDTO) {
	var groupID pgtype.UUID
	if dto.FieldGroupID != nil {
		_ = groupID.Scan(*dto.FieldGroupID)
	}
	if !groupID.Valid {
		// Pages bound before the group's binding (or seeded without it)
		// fall back to the category's bound group — same precedence as
		// the space path (cat.FieldGroupID fills page.FieldGroupID NULL).
		cat, err := s.store.read.ResolveWikiTypeCategoryToken(ctx, gen.ResolveWikiTypeCategoryTokenParams{
			SpaceTypeID: typeID, Token: dto.Category,
		})
		if err != nil {
			return
		}
		groupID = cat.FieldGroupID
	}
	if !groupID.Valid {
		return
	}
	// Group identity always reaches the client — even when the group has
	// zero seeded fields — so the fields panel and SuggestEdit can scope
	// on the schema that will exist (mirrors the space page's DTO).
	if g := uuidStr(groupID); dto.FieldGroupID == nil {
		dto.FieldGroupID = &g
	}
	if grp, err := s.getFieldsForEntity(ctx, userID, kind, ref.ID, groupID, ref.ParentSpaceID); err == nil && grp != nil {
		dto.Fields = grp
		dto.FieldGroupSlug = grp.Slug
		dto.FieldGroupName = grp.Name
	} else if s.fields != nil {
		if g, gerr := s.fields.GetFieldGroup(ctx, groupID); gerr == nil {
			dto.FieldGroupSlug = g.Slug
			dto.FieldGroupName = g.Name
		}
	}
}

// entityLazyPage synthesizes an unmaterialized page when pageSlug matches a
// category slug or page-template slug on the entity-bound type. Prose is
// empty; the page materializes on the first submission.
func (s *Service) entityLazyPage(ctx context.Context, userID, typeID pgtype.UUID, kind, typeSlug string, ref EntityRef, pageSlug string) (EntityPageDTO, error) {
	base := EntityPageDTO{
		Kind: kind, TypeSlug: typeSlug,
		EntityID: uuidStr(ref.ID), EntitySlug: ref.Slug, EntityName: ref.Name,
		Slug: pageSlug, Lazy: true,
	}
	// Page-template match first (subpages take precedence). {name} in
	// the label resolves to the entity name — same substitution the
	// space path runs via substituteTemplateLabel.
	if t, err := s.store.read.GetWikiTypePageTemplateBySlug(ctx, gen.GetWikiTypePageTemplateBySlugParams{
		SpaceTypeID: typeID, Slug: pageSlug,
	}); err == nil {
		base.Title = substituteTemplateLabel(t.Label, ref.Name)
		base.Category = t.CategoryToken
		if t.FieldGroupID.Valid {
			g := uuidStr(t.FieldGroupID)
			base.FieldGroupID = &g
		}
		s.attachEntityFields(ctx, userID, typeID, kind, ref, &base)
		return base, nil
	}
	// Category match — the type's canonical category pages render lazily.
	if cat, err := s.store.read.ResolveWikiTypeCategoryToken(ctx, gen.ResolveWikiTypeCategoryTokenParams{
		SpaceTypeID: typeID, Token: pageSlug,
	}); err == nil {
		base.Title = substituteTemplateLabel(cat.Label, ref.Name)
		base.Category = cat.Category
		if cat.FieldGroupID.Valid {
			g := uuidStr(cat.FieldGroupID)
			base.FieldGroupID = &g
		}
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && rcfg.Mode == rollupModeRelated {
			base.Related = s.computeEntityRelated(ctx, kind, ref)
		}
		s.attachEntityFields(ctx, userID, typeID, kind, ref, &base)
		return base, nil
	}
	return EntityPageDTO{}, pgx.ErrNoRows
}

// HandleGetEntityPage handles GET /wiki/e/{type_slug}/{entity}/{page_slug}
// — the entity-bound counterpart of HandleGetPage. Anonymous browse like
// the space path.
func (h *Handlers) HandleGetEntityPage(w http.ResponseWriter, r *http.Request) {
	typeSlug := chi.URLParam(r, "type_slug")
	entityRef := chi.URLParam(r, "entity")
	pageSlug := chi.URLParam(r, "page_slug")
	if typeSlug == "" || entityRef == "" || pageSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Type, entity, and page slugs are required")
		return
	}

	var userID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		userID = u.UserID
	}

	page, err := h.service.GetEntityWikiPage(r.Context(), userID, typeSlug, entityRef, pageSlug)
	if err != nil {
		switch {
		case errors.Is(err, ErrSpaceNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Wiki type not found")
		case errors.Is(err, ErrPageDeleted):
			errpkg.WriteError(w, http.StatusGone, "page_removed", "This wiki page has been removed")
		case errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
		default:
			slog.Error("wiki: get entity page failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get wiki page")
		}
		return
	}

	h.resolveLiterals(r, &page)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// computeEntityRelated resolves an entity-bound type's peer listing:
// course → sibling published offerings' universities (self excluded),
// program → every published offering's universities. Fails open — nil
// on lookup error or empty so the page renders without the grid.
func (s *Service) computeEntityRelated(ctx context.Context, kind string, ref EntityRef) *EntityRelatedDTO {
	dto := &EntityRelatedDTO{Kind: "universities"}
	switch kind {
	case "course":
		rows, err := s.store.read.ListUniversitiesForCourseOffering(ctx, ref.Slug)
		if err != nil {
			slog.Warn("wiki: related: course siblings failed", "error", err)
			return nil
		}
		for _, r := range rows {
			dto.Items = append(dto.Items, WikiRelatedSpaceDTO{
				Slug: r.Slug, Name: r.Name, GeoCode: r.GeoCode, OfferingSlug: r.OfferingSlug,
			})
		}
	case "program":
		rows, err := s.store.read.ListUniversitiesForProgram(ctx, ref.Slug)
		if err != nil {
			slog.Warn("wiki: related: program universities failed", "error", err)
			return nil
		}
		for _, r := range rows {
			dto.Items = append(dto.Items, WikiRelatedSpaceDTO{
				Slug: r.Slug, Name: r.Name, GeoCode: r.GeoCode, OfferingSlug: r.OfferingSlug,
			})
		}
	default:
		return nil
	}
	if len(dto.Items) == 0 {
		return nil
	}
	dto.Total = len(dto.Items)
	return dto
}

// entityChildren builds the index's entity-children map: every
// entity-bound wiki type parented to the host space's type contributes
// one key (type_slug → children), filtered by the type's discriminator.
func (s *Service) entityChildren(ctx context.Context, spaceID, spaceTypeID pgtype.UUID) map[string][]EntityChild {
	types, err := s.store.read.ListEntityWikiTypesForParent(ctx, spaceTypeID)
	if err != nil || len(types) == 0 {
		return nil
	}
	out := make(map[string][]EntityChild, len(types))
	for _, t := range types {
		kind := ""
		if t.EntityKind != nil {
			kind = *t.EntityKind
		}
		children, cerr := s.store.ListEntityChildren(ctx, spaceID, kind, discriminatorValue(t.EntityDiscriminator))
		if cerr != nil || len(children) == 0 {
			continue
		}
		out[t.TypeSlug] = children
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// createEntityPageRow inserts just the entity-bound wiki_pages row — the
// freeform create path pairs it with a single content revision (space-path
// parity); lazy materialization goes through createEntityPageBound which
// also records the birth revision.
func (s *Store) createEntityPageRow(ctx context.Context, kind string, entityID, createdBy pgtype.UUID, slug, title, category string, tmplID, groupID pgtype.UUID) (pgtype.UUID, error) {
	row, err := s.write.CreateEntityWikiPage(ctx, gen.CreateEntityWikiPageParams{
		EntityType:     &kind,
		EntityID:       entityID,
		Slug:           slug,
		Title:          title,
		Category:       category,
		CreatedBy:      createdBy,
		PageTemplateID: tmplID,
		FieldGroupID:   groupID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return pgtype.UUID{}, ErrPageExists
		}
		return pgtype.UUID{}, fmt.Errorf("wiki: create entity page: %w", err)
	}
	return row.ID, nil
}

// createEntityPageBound wraps createEntityPageRow with the initial 'birth'
// revision — the materialization record the space path also writes.
func (s *Store) createEntityPageBound(ctx context.Context, kind string, entityID, createdBy pgtype.UUID, slug, title, category string, tmplID, groupID pgtype.UUID) (pgtype.UUID, error) {
	pageID, err := s.createEntityPageRow(ctx, kind, entityID, createdBy, slug, title, category, tmplID, groupID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	contentHash := revisionContentHash(nil, nil)
	if _, err := s.write.InsertInitialWikiRevision(ctx, gen.InsertInitialWikiRevisionParams{
		WikiPageID:  pageID,
		Content:     nil,
		SubmittedBy: createdBy,
		ContentHash: &contentHash,
	}); err != nil {
		slog.Warn("wiki: entity page initial revision failed", "page_id", pageID.String(), "error", err)
	}
	return pageID, nil
}

// ensureEntityPage resolves the entity page row, materializing a lazy
// template/category page on first write — shared by the prose and ops
// submit paths.
func (s *Service) ensureEntityPage(ctx context.Context, userID pgtype.UUID, cfg gen.GetWikiTypeConfigForKindRow, ref EntityRef, typeSlug, pageSlug string) (gen.GetEntityWikiPageRow, error) {
	kind := ref.Kind
	page, err := s.store.read.GetEntityWikiPage(ctx, gen.GetEntityWikiPageParams{
		EntityType: &kind, EntityID: ref.ID, PageSlug: pageSlug,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		return page, err
	}
	lazy, lerr := s.entityLazyPage(ctx, userID, cfg.SpaceTypeID, kind, typeSlug, ref, pageSlug)
	if lerr != nil {
		return page, lerr
	}
	var groupID, tmplID pgtype.UUID
	if lazy.FieldGroupID != nil {
		_ = groupID.Scan(*lazy.FieldGroupID)
	}
	page.ID, err = s.store.createEntityPageBound(ctx, kind, ref.ID, userID,
		pageSlug, lazy.Title, lazy.Category, tmplID, groupID)
	if errors.Is(err, ErrPageExists) {
		// Unique-index hit with the read returning NoRows → an inactive or
		// deleted row occupies the slug. Deleted rows release the slug via
		// the partial index (000904), so this is a deactivated page — the
		// same 410 the read path reports via EntityWikiPageRowExists.
		return page, ErrPageDeleted
	}
	return page, err
}

// SubmitEntityRevision mirrors SubmitRevision for entity-bound pages:
// resolve type+entity, materialize the page when the slug is a known
// template/category, attach the revision.
func (s *Service) SubmitEntityRevision(ctx context.Context, userID pgtype.UUID, typeSlug, entityRef, pageSlug string, content json.RawMessage, summary string) (string, pgtype.UUID, bool, error) {
	cfg, ref, err := s.resolveEntityTarget(ctx, typeSlug, entityRef)
	if err != nil {
		return "", pgtype.UUID{}, false, err
	}

	page, err := s.ensureEntityPage(ctx, userID, cfg, ref, typeSlug, pageSlug)
	if err != nil {
		return "", pgtype.UUID{}, false, fmt.Errorf("wiki: entity submit: %w", err)
	}

	superAdmin := s.isSuperAdmin(ctx, userID)
	if page.IsLocked && !superAdmin {
		return "", pgtype.UUID{}, false, ErrPageLocked
	}

	normalized, err := normalizeWikiContent(content)
	if err != nil {
		return "", pgtype.UUID{}, false, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	revID, err := s.store.SubmitRevision(ctx, page.ID, userID, "content", normalized, nil, summary, nil, pgtype.UUID{})
	if err != nil {
		return "", pgtype.UUID{}, false, fmt.Errorf("wiki: entity submit: %w", err)
	}
	if !superAdmin {
		return revID.String(), page.ID, false, nil
	}
	if _, err := s.store.ApproveRevisionByID(ctx, revID, userID, "auto-approved (super_admin)"); err != nil {
		return revID.String(), page.ID, false, fmt.Errorf("wiki: entity submit: auto-approve: %w", err)
	}
	return revID.String(), page.ID, true, nil
}

// SubmitEntityPageFieldOps is the entity-bound counterpart of
// SubmitPageFieldOps: ops validate BEFORE the page materializes — a
// rejected submission leaves no empty wiki_pages row. The ops target
// defaults to the page's entity (resolveOpsTarget falls back to the
// kind+id we pass); per-op targets still override within the same
// revision-target consistency rules.
func (s *Service) SubmitEntityPageFieldOps(ctx context.Context, userID pgtype.UUID, typeSlug, entityRef, pageSlug string, body []byte, ops []FieldOp, summary string, canManage bool) (string, bool, error) {
	cfg, ref, err := s.resolveEntityTarget(ctx, typeSlug, entityRef)
	if err != nil {
		return "", false, err
	}

	normBody, targetType, targetID, err := s.validateFieldOps(ctx, userID, ref.Kind, ref.ID, body, ops, canManage)
	if err != nil {
		return "", false, err
	}

	page, err := s.ensureEntityPage(ctx, userID, cfg, ref, typeSlug, pageSlug)
	if err != nil {
		return "", false, err
	}
	if page.IsLocked && !s.isSuperAdmin(ctx, userID) {
		return "", false, ErrPageLocked
	}

	return s.persistFieldOps(ctx, userID, page.ID, normBody, targetType, targetID, ops, summary, canManage)
}

// resolveEntityTarget is the shared front-half of entity page reads and
// writes: type config → entity ref → discriminator check.
func (s *Service) resolveEntityTarget(ctx context.Context, typeSlug, entityRef string) (gen.GetWikiTypeConfigForKindRow, EntityRef, error) {
	cfg, err := s.store.GetEntityKindConfig(ctx, typeSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return cfg, EntityRef{}, ErrSpaceNotFound
		}
		return cfg, EntityRef{}, fmt.Errorf("wiki: entity: resolve type %q: %w", typeSlug, err)
	}
	if cfg.EntityKind == nil {
		return cfg, EntityRef{}, ErrSpaceNotFound
	}
	kind := *cfg.EntityKind

	ref, err := s.store.ResolveEntityRef(ctx, kind, entityRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return cfg, EntityRef{}, pgx.ErrNoRows
		}
		return cfg, EntityRef{}, fmt.Errorf("wiki: entity: resolve %s %q: %w", kind, entityRef, err)
	}
	if len(cfg.EntityDiscriminator) > 0 {
		var disc map[string]string
		if err := json.Unmarshal(cfg.EntityDiscriminator, &disc); err != nil || len(disc) == 0 {
			// Malformed discriminator config must fail closed — a type
			// that can't verify its discriminator must not serve pages.
			return cfg, EntityRef{}, fmt.Errorf("wiki: entity: type %q has invalid entity_discriminator: %w", typeSlug, err)
		}
		for _, want := range disc {
			if ref.Discriminator != want {
				return cfg, EntityRef{}, pgx.ErrNoRows
			}
		}
	}
	return cfg, ref, nil
}

// HandleSubmitEntityRevision handles
// POST /wiki/e/{type_slug}/{entity}/{page_slug}/revisions (wiki.edit).
// Same gates as the space path: community_edits setting + reputation
// threshold on top of the wiki.edit permission node.
func (h *Handlers) HandleSubmitEntityRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	typeSlug := chi.URLParam(r, "type_slug")
	entityRef := chi.URLParam(r, "entity")
	pageSlug := chi.URLParam(r, "page_slug")

	// B5: community_edits gate — identical to HandleSubmitRevision.
	if !h.service.CommunityEditsAllowed(r.Context()) && h.perms != nil {
		isMod := h.perms.HasPermission(r.Context(), user.UserID, "wiki.review") ||
			h.perms.HasPermission(r.Context(), user.UserID, "wiki.manage")
		if !isMod {
			errpkg.WriteError(w, http.StatusForbidden, "community_edits_disabled",
				"Community edits are currently disabled for this wiki")
			return
		}
	}

	// B4: reputation gate — the route middleware checks wiki.edit; the
	// lifetime-points threshold applies on top, same as the space path.
	if !h.checkReputationGate(w, r, user.UserID, "edit this wiki page") {
		return
	}

	var input struct {
		Content json.RawMessage `json:"content"`
		Ops     []FieldOp       `json:"ops"`
		Summary string          `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if len(input.Content) == 0 && len(input.Ops) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content", "Content or field ops are required")
		return
	}

	var revID string
	var autoApproved bool
	var pageID pgtype.UUID

	if len(input.Ops) > 0 {
		if !h.service.StructuredEditsEnabled(r.Context()) {
			errpkg.WriteError(w, http.StatusNotFound, "feature_disabled", "This feature is not available")
			return
		}
		canManage := h.perms != nil && h.perms.HasPermission(r.Context(), user.UserID, PermFieldsManage)
		var oerr error
		revID, autoApproved, oerr = h.service.SubmitEntityPageFieldOps(r.Context(), user.UserID, typeSlug, entityRef, pageSlug, input.Content, input.Ops, input.Summary, canManage)
		if oerr != nil {
			switch {
			case errors.Is(oerr, ErrSpaceNotFound):
				errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Wiki type not found")
			case errors.Is(oerr, pgx.ErrNoRows):
				errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			case errors.Is(oerr, ErrPageLocked):
				errpkg.WriteError(w, http.StatusForbidden, "page_locked", "This wiki page is locked")
			default:
				writeFieldOpsError(w, oerr)
			}
			return
		}
	} else {
		var serr error
		revID, pageID, autoApproved, serr = h.service.SubmitEntityRevision(r.Context(), user.UserID, typeSlug, entityRef, pageSlug, input.Content, input.Summary)
		if serr != nil {
			switch {
			case errors.Is(serr, ErrPageLocked):
				errpkg.WriteError(w, http.StatusForbidden, "page_locked", "This page is locked")
			case errors.Is(serr, ErrSpaceNotFound):
				errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Wiki type not found")
			case errors.Is(serr, ErrPageDeleted):
				errpkg.WriteError(w, http.StatusGone, "page_removed", "This wiki page has been removed")
			case errors.Is(serr, pgx.ErrNoRows):
				errpkg.WriteError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			case errors.Is(serr, ErrInvalidInput):
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", serr.Error())
			case errors.Is(serr, ErrDuplicateContent):
				errpkg.WriteError(w, http.StatusConflict, "duplicate_content", "No changes from the current approved revision")
			default:
				slog.Error("wiki: entity submit failed", "error", serr)
				errpkg.WriteError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit revision")
			}
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"revision_id": revID, "page_id": pageID, "auto_approved": autoApproved,
	}})
}

// CreateEntityPageWithRevision is the entity-bound counterpart of
// CreatePageWithRevision: freeform page create on an entity — slug +
// title + category (+ optional content). The category resolves to the
// type's managed token when it exists (same canonicalization the space
// path does so sibling bucketing never fragments); a slug matching a
// type template picks up its field-group binding.
func (s *Service) CreateEntityPageWithRevision(ctx context.Context, userID pgtype.UUID, input CreateWikiPageInput, typeSlug, entityRef string) (string, string, string, bool, error) {
	slug, category, title, err := canonicalizePageInput(input)
	if err != nil {
		return "", "", "", false, err
	}
	if len(input.Content) == 0 {
		return "", "", "", false, fmt.Errorf("%w: content is required", ErrInvalidInput)
	}
	normalizedContent, err := normalizeWikiContent(input.Content)
	if err != nil {
		return "", "", "", false, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	cfg, ref, err := s.resolveEntityTarget(ctx, typeSlug, entityRef)
	if err != nil {
		return "", "", "", false, err
	}
	kind := ref.Kind

	// Canonical category token — freeform categories pass through
	// unchanged (same fallback as canonicalCategory on the space path).
	if cat, cerr := s.store.read.ResolveWikiTypeCategoryToken(ctx, gen.ResolveWikiTypeCategoryTokenParams{
		SpaceTypeID: cfg.SpaceTypeID, Token: category,
	}); cerr == nil {
		category = cat.Category
	}

	// Template binding for this (category, slug) pair when declared.
	var tmplID, tmplGroupID pgtype.UUID
	if t, terr := s.store.read.GetWikiTypePageTemplateBySlug(ctx, gen.GetWikiTypePageTemplateBySlugParams{
		SpaceTypeID: cfg.SpaceTypeID, Slug: slug,
	}); terr == nil && t.CategoryToken == category {
		tmplID = t.ID
		tmplGroupID = t.FieldGroupID
	}

	pageID, err := s.store.createEntityPageRow(ctx, kind, ref.ID, userID,
		slug, title, category, tmplID, tmplGroupID)
	if err != nil {
		if errors.Is(err, ErrPageExists) {
			return "", "", "", false, ErrPageExists
		}
		return "", "", "", false, fmt.Errorf("wiki: create entity page: %w", err)
	}

	// Attach the single content revision — the page row carries no prose;
	// the revision IS the initial write, matching the space create path.
	contentHash := revisionContentHash(normalizedContent, nil)
	var summaryPtr *string
	if input.Summary != "" {
		summaryPtr = &input.Summary
	}
	revID, err := s.store.write.InsertInitialWikiRevision(ctx, gen.InsertInitialWikiRevisionParams{
		WikiPageID:  pageID,
		Content:     normalizedContent,
		Summary:     summaryPtr,
		SubmittedBy: userID,
		ContentHash: &contentHash,
	})
	if err != nil {
		return "", "", "", false, fmt.Errorf("wiki: create entity page: revision: %w", err)
	}

	if !s.isSuperAdmin(ctx, userID) {
		return pageID.String(), revID.String(), slug, false, nil
	}
	if _, err := s.store.ApproveRevisionByID(ctx, revID, userID, "auto-approved (super_admin)"); err != nil {
		return pageID.String(), revID.String(), slug, false, fmt.Errorf("wiki: create entity page: auto-approve: %w", err)
	}
	return pageID.String(), revID.String(), slug, true, nil
}

// HandleCreateEntityPage handles POST /wiki/e/{type_slug}/{entity} —
// freeform page create on an entity (wiki.edit + same gates as space).
func (h *Handlers) HandleCreateEntityPage(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	typeSlug := chi.URLParam(r, "type_slug")
	entityRef := chi.URLParam(r, "entity")

	if !h.service.CommunityEditsAllowed(r.Context()) && h.perms != nil {
		isMod := h.perms.HasPermission(r.Context(), user.UserID, "wiki.review") ||
			h.perms.HasPermission(r.Context(), user.UserID, "wiki.manage")
		if !isMod {
			errpkg.WriteError(w, http.StatusForbidden, "community_edits_disabled",
				"Community edits are currently disabled for this wiki")
			return
		}
	}
	if !h.checkReputationGate(w, r, user.UserID, "create a wiki page") {
		return
	}

	var input CreateWikiPageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	pageID, revID, pageSlug, autoApproved, err := h.service.CreateEntityPageWithRevision(r.Context(), user.UserID, input, typeSlug, entityRef)
	if err != nil {
		switch {
		case errors.Is(err, ErrSpaceNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Wiki type not found")
		case errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "entity_not_found", "Entity not found")
		case errors.Is(err, ErrPageExists):
			errpkg.WriteError(w, http.StatusConflict, "page_exists", "A page with this slug already exists")
		case errors.Is(err, ErrInvalidInput):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
		default:
			slog.Error("wiki: entity create failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create wiki page")
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"page_id": pageID, "revision_id": revID, "slug": pageSlug, "auto_approved": autoApproved,
	}})
}
