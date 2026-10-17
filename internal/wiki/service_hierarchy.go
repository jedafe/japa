// Package wiki — service_hierarchy.go implements the §80 Hierarchical UX &
// Itemized Pricing amendment's read-side enrichment: breadcrumb ancestors
// walked over the actual space links and arbitrated by the declared type
// edges, category sibling ordering for Prev/Next, and read-time descendant
// rollups for rollup-enabled categories.
// // Ref: japaconnect-engineering/sections/80-destination-wiki.md
package wiki

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
)

// ancestorDepth caps the breadcrumb walk — mirrors the ?within= join depth
// so navigation and descendant filtering share the same horizon.
const ancestorDepth = 3

// WikiSpaceRefDTO is one node in an ancestor chain or rollup row — the
// minimal space identity a breadcrumb or table cell needs.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiSpaceRefDTO struct {
	// ID enables compare-preselect deep links on rollup rows (§80 Reader
	// UX); omitted when unknown.
	ID       string `json:"id,omitempty"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Level    string `json:"level"`
	TypeSlug string `json:"type_slug,omitempty"`
	// TypeLabel is the ancestor's wiki-type display label (§6 of the §80
	// amendment) — breadcrumb segments render it instead of a hardcoded
	// string.
	TypeLabel string `json:"type_label,omitempty"`
}

// WikiSiblingDTO is one sibling page in a category's Prev/Next ordering.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiSiblingDTO struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Position int32  `json:"position"`
	Current  bool   `json:"current,omitempty"`
}

// WikiRollupConfig is the parsed shape of wiki_type_categories.rollup.
// Levels omitted means "every declared descendant type" (the transitive
// closure of wiki_type_parents); metrics selects which aggregates render.
// SectionLimit caps the rows table (§7 cardinality guard); zero or absent
// falls back to defaultRollupRowsLimit.
// Mode "directory" switches the category page from metric aggregation to a
// child-space list (§80).
// //nolint:revive // exported: DTO naming follows package convention.
type WikiRollupConfig struct {
	Enabled      bool     `json:"enabled"`
	Mode         string   `json:"mode,omitempty"`
	Metrics      []string `json:"metrics"`
	Levels       []string `json:"levels"`
	SectionLimit int      `json:"section_limit,omitempty"`
	// Pages scopes a mode:"pages" rollup to the named page-template
	// slugs (config order = row order); absent/empty means every
	// template under the category.
	Pages []string `json:"pages,omitempty"`
	// IncludeSelf (pages mode only) prepends one row for the
	// category's own bound group — the page's summary fields —
	// linked at the category slug. Off means topic rows only.
	IncludeSelf bool `json:"include_self,omitempty"`
}

// rollupModeDirectory switches a category rollup from metric aggregation
// to a child-space directory: instead of fold-up sections the page lists
// every linked descendant wiki space (with geo_code + page_count), no
// field-value requirement.
const rollupModeDirectory = "directory"

// rollupModePages switches a category rollup from descendant-space
// aggregation to same-space sub-page aggregation: rows are the
// category's configured page templates, values the space's cfv in each
// page's bound group (§80 page rollup amendment).
const rollupModePages = "pages"

// rollupScopePages marks WikiRollupDTO.Scope for page-mode payloads —
// the frontend picks row labels/links off it.
const rollupScopePages = "pages"

// defaultRollupRowsLimit bounds the descendants table when the category
// config sets no section_limit — the full set remains reachable through
// GET /wiki/destinations?within={slug}&level={level}.
const defaultRollupRowsLimit = 50

// WikiRollupMetricDTO carries the requested aggregates for one field —
// nil members are metrics the config didn't request.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiRollupMetricDTO struct {
	Min *float64 `json:"min,omitempty"`
	Avg *float64 `json:"avg,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// WikiRollupFieldDTO is one aggregated field: for scalar numeric fields
// min/avg/max are over the stored values; for price_range fields min is the
// minimum of mins, avg the mean of avgs, max the maximum of maxes.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiRollupFieldDTO struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	FieldType string `json:"field_type"`
	Section   string `json:"section,omitempty"`
	// CurrencyCode is the ISO 4217 code declared on the field's options
	// ({currency_code}) for currency/price_range fields — lets the read
	// surface render the currency name/symbol next to aggregated amounts.
	CurrencyCode string `json:"currency_code,omitempty"`
	// §80 Wiki Topic Pages — the part a price_parts field aggregates/
	// displays (options.primary_part); lets row cells project the
	// comparable member of the stored {parts:{...}} object.
	PrimaryPart string `json:"primary_part,omitempty"`
	// PrimaryKind is the primary part's declared kind (money|number) —
	// row cells format money parts with the currency chain, number bare.
	PrimaryKind string              `json:"primary_kind,omitempty"`
	Metrics     WikiRollupMetricDTO `json:"metrics"`
	Samples     int                 `json:"samples"`
}

// WikiRollupPageRefDTO identifies a sub-page rollup row — the
// /wiki/{space}/{slug} link target plus its display title
// (section_label when set, else the {name}-substituted label).
// //nolint:revive // exported: DTO naming follows package convention.
type WikiRollupPageRefDTO struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// WikiRollupRowDTO is one contributing descendant space and its values —
// the "Cost of Living by City" table rows on a country page. In pages
// mode Page names the sub-page the row aggregates and Space is the host
// space for shape stability.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiRollupRowDTO struct {
	Space  WikiSpaceRefDTO       `json:"space"`
	Page   *WikiRollupPageRefDTO `json:"page,omitempty"`
	Values map[string]any        `json:"values"`
}

// WikiRollupDTO is the read-time descendant aggregate attached to a
// rollup-enabled category page. Never persisted — recomputed per request.
// Scope is "pages" for page-mode payloads (empty for the legacy
// descendant axis) so the frontend picks row labels/links without
// sniffing rows.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiRollupDTO struct {
	Scope  string               `json:"scope,omitempty"`
	Fields []WikiRollupFieldDTO `json:"fields"`
	Rows   []WikiRollupRowDTO   `json:"rows"`
	Spaces int                  `json:"spaces"`
}

// WikiDirectoryChildDTO is one card in a directory-mode listing — the
// space's identity plus geo_code and its active wiki page count. ImageURL,
// Blurb, and Kind carry the richer card surface added by §80 (attraction
// directory grids); they are empty when the child has no media/fields.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiDirectoryChildDTO struct {
	Slug      string  `json:"slug"`
	Name      string  `json:"name"`
	Level     string  `json:"level"`
	TypeSlug  string  `json:"type_slug,omitempty"`
	GeoCode   *string `json:"geo_code,omitempty"`
	ImageURL  string  `json:"image_url,omitempty"`
	Blurb     string  `json:"blurb,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	PageCount int64   `json:"page_count"`
	Depth     int32   `json:"depth"`
}

// WikiDirectoryDTO is the read-time child-space listing attached to a
// directory-mode category page. Total counts every matching descendant
// (pre-cap); Children is capped for the card grid — the frontend reaches
// the full set via GET /wiki/destinations?within={slug}&level={level}.
// Levels lists the distinct child type slugs across the full (pre-cap)
// listing, so type filters stay complete even when the cap truncates
// Children.
// //nolint:revive // exported: DTO naming follows package convention.
type WikiDirectoryDTO struct {
	Levels   []string                `json:"levels"`
	Children []WikiDirectoryChildDTO `json:"children"`
	Total    int                     `json:"total"`
}

// defaultDirectoryLimit caps the directory card grid — the full set stays
// reachable through the destinations endpoint.
const defaultDirectoryLimit = 200

// computeDirectory lists a space's linked wiki-type descendants for a
// directory-mode category page (§80): eligible types come from
// rollup.levels or the declared descendant-type closure; children come
// from space_parent_spaces ∪ parent_id gated to enabled wiki types. Nil
// for non-directory configs, unresolvable levels, or no matching
// descendants — a category that lists nothing shouldn't render the panel.
func (s *Service) computeDirectory(ctx context.Context, spaceID, spaceTypeID pgtype.UUID, cfg *WikiRollupConfig) *WikiDirectoryDTO {
	if cfg.Mode != rollupModeDirectory {
		return nil
	}

	typeIDs, err := s.rollupTypeIDs(ctx, spaceTypeID, cfg.Levels)
	if err != nil {
		slog.Warn("wiki: directory: type scope failed", "error", err)
		return nil
	}

	if len(typeIDs) == 0 {
		return nil
	}

	children, err := s.store.ListDirectoryChildren(ctx, spaceID, typeIDs)
	if err != nil {
		slog.Warn("wiki: directory: children failed",
			"space_id", spaceID.String(), "error", err)
		return nil
	}

	if len(children) == 0 {
		return nil
	}

	limit := defaultDirectoryLimit

	if s.settings != nil {
		if n := s.settings.GetIntOr(ctx, "wiki.directory_default_limit", defaultDirectoryLimit); n > 0 {
			limit = n
		}
	}

	// Levels reports the distinct child type slugs across the full listing
	// — a type whose children fall entirely past the cap still gets a
	// filter chip because scoped search (?within=&level=) reaches it.
	seen := make(map[string]bool, 4)
	levels := make([]string, 0, 4)

	for _, c := range children {
		if c.SpaceTypeSlug == "" || seen[c.SpaceTypeSlug] {
			continue
		}

		seen[c.SpaceTypeSlug] = true

		levels = append(levels, c.SpaceTypeSlug)
	}

	// §80 directory cards: sort+cap first so image/field batch resolution
	// only touches the children actually rendered — not the full listing.
	sortDirectoryChildren(children)

	n := len(children)
	if limit > 0 && n > limit {
		n = limit
	}

	capped := children[:n]

	imageURLs := s.directoryChildImages(ctx, capped)
	kinds := s.directoryChildKinds(ctx, capped)

	return &WikiDirectoryDTO{
		Levels:   levels,
		Children: directoryChildDTOs(capped, imageURLs, kinds),
		Total:    len(children),
	}
}

// sortDirectoryChildren orders directory children by depth, then name,
// then slug — deterministic so cap truncation drops the same tail every
// time, not an arbitrary subset.
func sortDirectoryChildren(children []WikiDirectoryChild) {
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].Depth != children[j].Depth {
			return children[i].Depth < children[j].Depth
		}

		if children[i].Name != children[j].Name {
			return children[i].Name < children[j].Name
		}

		return children[i].Slug < children[j].Slug
	})
}

// directoryChildDTOs maps (already sorted+capped) directory children to
// card DTOs, merging the batch-resolved image URLs and kind values.
func directoryChildDTOs(
	children []WikiDirectoryChild,
	imageURLs map[pgtype.UUID]string,
	kinds map[pgtype.UUID]string,
) []WikiDirectoryChildDTO {
	out := make([]WikiDirectoryChildDTO, 0, len(children))

	for _, c := range children {
		dto := WikiDirectoryChildDTO{
			Slug: c.Slug, Name: c.Name, Level: c.Level,
			TypeSlug: c.SpaceTypeSlug, GeoCode: c.GeoCode,
			Blurb:     c.Blurb,
			PageCount: c.PageCount, Depth: c.Depth,
		}
		if url := imageURLs[c.ID]; url != "" {
			dto.ImageURL = url
		}

		if kind := kinds[c.ID]; kind != "" {
			dto.Kind = kind
		}

		out = append(out, dto)
	}

	return out
}

// directoryChildImages batch-resolves each child's card image: header image
// preferred (card aspect), logo as fallback. One GetMediaByIDs query serves
// the whole capped page; URL signing goes through the injected media
// resolver. Best-effort — nil maps on any failure.
func (s *Service) directoryChildImages(ctx context.Context, children []WikiDirectoryChild) map[pgtype.UUID]string {
	if s.media == nil || len(children) == 0 {
		return nil
	}

	seen := make(map[pgtype.UUID]struct{}, len(children))
	ids := make([]pgtype.UUID, 0, len(children)*2)

	for _, c := range children {
		for _, mid := range []pgtype.UUID{c.HeaderImageID, c.LogoMediaID} {
			if mid.Valid {
				if _, dup := seen[mid]; !dup {
					seen[mid] = struct{}{}

					ids = append(ids, mid)
				}
			}
		}
	}

	if len(ids) == 0 {
		return nil
	}

	mediaRows, err := s.store.ListMediaByIDs(ctx, ids)
	if err != nil {
		slog.Warn("wiki: directory: media batch failed", "error", err)
		return nil
	}

	byID := make(map[pgtype.UUID]*gen.Medium, len(mediaRows))

	for i := range mediaRows {
		m := mediaRows[i]
		byID[m.ID] = &m
	}

	out := make(map[pgtype.UUID]string, len(children))

	for _, c := range children {
		var m *gen.Medium

		switch {
		case c.HeaderImageID.Valid:
			m = byID[c.HeaderImageID]
		case c.LogoMediaID.Valid:
			m = byID[c.LogoMediaID]
		}

		if m == nil {
			continue
		}

		out[c.ID] = s.media.GetMediaURL(m, media.ImageProxyOpts{Width: 640, Height: 360, Format: "webp"})
	}

	return out
}

// selectValueJSON extracts the scalar from a select-type field value's
// value_json blob — selects store {"value": "museum"}, not a value_text
// (§80 directory cards read `kind` this way).
func selectValueJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}

	// Stored shapes seen across writers: {"value":"x"}, {"values":["x",…]},
	// and the bare JSON string "x".
	switch t := probe.(type) {
	case string:
		return t
	case map[string]any:
		if s, ok := t["value"].(string); ok {
			return s
		}

		if arr, ok := t["values"].([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok {
				return s
			}
		}
	}

	return ""
}

// directoryChildKinds batch-reads the `kind` select field for the capped
// children so attraction cards can render a kind chip. Best-effort — nil
// map when the field-value reader is unwired or errors.
func (s *Service) directoryChildKinds(ctx context.Context, children []WikiDirectoryChild) map[pgtype.UUID]string {
	if s.fields == nil || len(children) == 0 {
		return nil
	}

	ids := make([]pgtype.UUID, 0, len(children))
	for _, c := range children {
		ids = append(ids, c.ID)
	}

	vals, err := s.fields.ListFieldValuesWithDefsForEntities(ctx, "space", ids)
	if err != nil {
		slog.Warn("wiki: directory: kind values failed", "error", err)
		return nil
	}

	out := make(map[pgtype.UUID]string, len(children))

	for _, v := range vals {
		if v.FieldKey != "kind" {
			continue
		}

		if kind := selectValueJSON(v.ValueJson); kind != "" {
			out[v.EntityID] = kind
		}
	}

	return out
}

// parseRollupConfig decodes a category's rollup JSONB; nil on absent or
// malformed config (rollups are strictly opt-in).
func parseRollupConfig(raw []byte) *WikiRollupConfig {
	if len(raw) == 0 {
		return nil
	}

	var cfg WikiRollupConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		slog.Warn("wiki: malformed category rollup config", "error", err)
		return nil
	}

	if !cfg.Enabled {
		return nil
	}

	return &cfg
}

// getAncestors walks the actual space hierarchy upward (≤ ancestorDepth
// hops), picking one parent per level. Multi-parent spaces are arbitrated
// by the declared type edges: among linked parents prefer the one whose
// type is a declared parent of the current node's type; when several
// qualify, prefer the deepest in the declared chain (a parent whose type is
// itself a declared child of another candidate's type); then is_primary,
// then name for determinism. Undeclared junction parents are excluded from
// the chain — the primary (spaces.parent_id) link alone may continue it
// when no declared edge applies — and every non-picked parent is returned
// as an alternate ("also under X" per §6 of the §80 amendment). Returns
// the chain root-first plus the alternates.
func (s *Service) getAncestors(ctx context.Context, spaceID, spaceTypeID pgtype.UUID) (chain, alternates []WikiSpaceRefDTO) {
	visited := map[pgtype.UUID]bool{spaceID: true}

	curID, curTypeID := spaceID, spaceTypeID
	for range ancestorDepth {
		parents, err := s.store.ListSpaceParentLinks(ctx, curID)
		if err != nil {
			slog.Warn("wiki: ancestors: parent links failed",
				"space_id", curID.String(), "error", err)
			break
		}

		if len(parents) == 0 {
			break
		}

		pick, rest := s.pickAncestor(ctx, parents, curTypeID, visited)
		for _, r := range rest {
			alternates = append(alternates, spaceRefFromLink(r))
		}

		if pick == nil {
			break
		}

		visited[pick.ID] = true

		chain = append([]WikiSpaceRefDTO{spaceRefFromLink(*pick)}, chain...)

		curID, curTypeID = pick.ID, pick.SpaceTypeID
	}

	// A parent unpicked at one hop can still enter the chain via a deeper
	// path (e.g. a university linked to both its state and its country —
	// the state wins arbitration, then the country is picked again as the
	// state's parent). That placement is already in the breadcrumb, so it
	// is not an "also under X" alternate; drop it, and dedupe the rest.
	if len(alternates) > 0 {
		seen := make(map[string]bool, len(chain)+len(alternates))
		for _, a := range chain {
			seen[a.ID] = true
		}

		out := make([]WikiSpaceRefDTO, 0, len(alternates))

		for _, a := range alternates {
			if !seen[a.ID] {
				seen[a.ID] = true

				out = append(out, a)
			}
		}

		alternates = out
	}

	return chain, alternates
}

func spaceRefFromLink(p WikiParentLink) WikiSpaceRefDTO {
	return WikiSpaceRefDTO{
		ID:        p.ID.String(),
		Slug:      p.Slug,
		Name:      p.Name,
		Level:     p.Level,
		TypeSlug:  p.SpaceTypeSlug,
		TypeLabel: p.SpaceTypeLabel,
	}
}

// pickAncestor selects one parent among a node's actual links using the
// declared type edges of the node's type. It also returns the links it
// rejected: declared-but-not-picked parents and undeclared junction
// parents — the ancestors_alt set.
func (s *Service) pickAncestor(ctx context.Context, parents []WikiParentLink, curTypeID pgtype.UUID, visited map[pgtype.UUID]bool) (*WikiParentLink, []WikiParentLink) {
	candidates := make([]WikiParentLink, 0, len(parents))

	for _, p := range parents {
		if !visited[p.ID] {
			candidates = append(candidates, p)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Declared parent types of the current node's type.
	declared := map[pgtype.UUID]bool{}

	if curTypeID.Valid {
		if ids, err := s.store.ListTypeParentIDs(ctx, curTypeID); err == nil {
			for _, id := range ids {
				declared[id] = true
			}
		}
	}

	pool := candidates
	if in := filterLinks(candidates, func(p WikiParentLink) bool {
		return declared[p.SpaceTypeID]
	}); len(in) > 0 {
		pool = in
	} else {
		// No declared edge applies — only the primary (spaces.parent_id)
		// link continues the chain. Undeclared junction parents are
		// alternates, never ancestors (§80 amendment §6).
		pool = filterLinks(candidates, func(p WikiParentLink) bool {
			return p.IsPrimary
		})
		if len(pool) == 0 {
			return nil, candidates
		}
	}

	// Deepest in the declared chain: a candidate whose type is itself a
	// declared child of another candidate's type (city below state below
	// country). Drops the "higher" candidate each comparison.
	if len(pool) > 1 {
		childOf := map[pgtype.UUID]bool{}

		for _, p := range pool {
			kids, err := s.store.ListTypeChildIDs(ctx, p.SpaceTypeID)
			if err != nil {
				continue
			}

			for _, k := range kids {
				childOf[k] = true
			}
		}

		if deep := filterLinks(pool, func(p WikiParentLink) bool {
			return childOf[p.SpaceTypeID]
		}); len(deep) > 0 {
			pool = deep
		}
	}

	if in := filterLinks(pool, func(p WikiParentLink) bool {
		return p.IsPrimary
	}); len(in) > 0 {
		pool = in
	}

	sort.Slice(pool, func(i, j int) bool { return pool[i].Name < pool[j].Name })

	pick := pool[0]
	rest := make([]WikiParentLink, 0, len(candidates)-1)

	for _, p := range candidates {
		if p.ID != pick.ID {
			rest = append(rest, p)
		}
	}

	return &pick, rest
}

func filterLinks(in []WikiParentLink, keep func(WikiParentLink) bool) []WikiParentLink {
	out := make([]WikiParentLink, 0, len(in))

	for _, p := range in {
		if keep(p) {
			out = append(out, p)
		}
	}

	return out
}

// getSiblings returns a page's same-parentage sibling set with the current
// page flagged — Prev/Next derives from neighbors in this list (§80 Wiki
// Topic Pages §6).
//
//   - Nested topic page (parentTemplateID valid): its parent's child
//     templates, ordered by position. Lazy templates are included — they
//     resolve at the URL — so Prev/Next spans the full declared topic set.
//   - Top-level page (managed category): the union of the category stub
//     (position 0), top-level templates, and materialized ad-hoc pages —
//     ordered position, created_at, slug.
//   - Unmanaged category (cat nil): materialized pages only — the freeform
//     behavior from before typed categories.
//
// templates is the category's full template set (nested included); nil is
// fine — the union degrades to stub + materialized pages.
func (s *Service) getSiblings(ctx context.Context, spaceID pgtype.UUID, spaceName string, cat *WikiCategory, categoryToken, currentSlug string, parentTemplateID pgtype.UUID, templates []WikiPageTemplate) []WikiSiblingDTO {
	if cat == nil {
		return s.materializedSiblings(ctx, spaceID, categoryToken, currentSlug)
	}

	if parentTemplateID.Valid {
		out := make([]WikiSiblingDTO, 0, 4)

		for _, t := range templates {
			if t.ParentTemplateID != parentTemplateID {
				continue
			}

			out = append(out, WikiSiblingDTO{
				Slug:     t.Slug,
				Title:    substituteTemplateLabel(t.Label, spaceName),
				Position: t.Position,
				Current:  t.Slug == currentSlug,
			})
		}
		// ListPageTemplates already orders position,slug — re-sort anyway
		// so the Prev/Next set is correct for any caller's input order.
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Position != out[j].Position {
				return out[i].Position < out[j].Position
			}

			return out[i].Slug < out[j].Slug
		})

		return out
	}

	// Top-level scope. The stub entry carries the category's page slug —
	// the category page materializes under it — with position 0 so it
	// leads the union ("Cost of Living" is the natural Prev of "Prices").
	type siblingRef struct {
		dto       WikiSiblingDTO
		createdAt pgtype.Timestamptz
	}

	refs := []siblingRef{{
		dto: WikiSiblingDTO{
			Slug:    cat.Slug,
			Title:   substituteTemplateLabel(cat.Label, spaceName),
			Current: cat.Slug == currentSlug,
		},
	}}

	templateSlugs := make(map[string]bool, len(templates))
	for _, t := range templates {
		templateSlugs[t.Slug] = true

		if t.ParentTemplateID.Valid {
			continue // nested topics sibling only within their parent scope
		}

		refs = append(refs, siblingRef{
			dto: WikiSiblingDTO{
				Slug:     t.Slug,
				Title:    substituteTemplateLabel(t.Label, spaceName),
				Position: t.Position,
				Current:  t.Slug == currentSlug,
			},
			createdAt: t.CreatedAt,
		})
	}

	// Materialized ad-hoc pages join the union — except slugs already
	// represented by the stub or a template (a materialized template page
	// IS the template's instance, not a separate sibling).
	if rows, err := s.store.ListPageSiblings(ctx, spaceID, categoryToken); err == nil {
		for _, r := range rows {
			if r.Slug == cat.Slug || templateSlugs[r.Slug] {
				continue
			}

			refs = append(refs, siblingRef{
				dto: WikiSiblingDTO{
					Slug:     r.Slug,
					Title:    r.Title,
					Position: r.Position,
					Current:  r.Slug == currentSlug,
				},
				createdAt: r.CreatedAt,
			})
		}
	} else {
		slog.Warn("wiki: siblings: list failed",
			"space_id", spaceID.String(), "category", categoryToken, "error", err)
	}

	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].dto.Position != refs[j].dto.Position {
			return refs[i].dto.Position < refs[j].dto.Position
		}

		if refs[i].createdAt.Valid != refs[j].createdAt.Valid {
			return refs[j].createdAt.Valid // stub's null time sorts first
		}

		if refs[i].createdAt != refs[j].createdAt {
			return refs[i].createdAt.Time.Before(refs[j].createdAt.Time)
		}

		return refs[i].dto.Slug < refs[j].dto.Slug
	})

	out := make([]WikiSiblingDTO, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.dto)
	}

	return out
}

// materializedSiblings is the pre-typed-category sibling set — the
// materialized pages of one space+category in display order.
func (s *Service) materializedSiblings(ctx context.Context, spaceID pgtype.UUID, category, currentSlug string) []WikiSiblingDTO {
	rows, err := s.store.ListPageSiblings(ctx, spaceID, category)
	if err != nil {
		slog.Warn("wiki: siblings: list failed",
			"space_id", spaceID.String(), "category", category, "error", err)
		return nil
	}

	out := make([]WikiSiblingDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiSiblingDTO{
			Slug:     r.Slug,
			Title:    r.Title,
			Position: r.Position,
			Current:  r.Slug == currentSlug,
		})
	}

	return out
}

// pageParents builds the page-level ancestor chain for the breadcrumb and
// "Back to {parent}" (§80 Wiki Topic Pages §5): the category stub first,
// then template ancestors root → immediate parent. tmpl is the page's
// bound template (nil for the category stub page itself — it IS the first
// segment — and for ad-hoc pages, which get the stub alone). Returns nil
// when cat is nil (unmanaged category — no page ancestry to express).
func (s *Service) pageParents(ctx context.Context, cat *WikiCategory, tmpl *WikiPageTemplate, spaceName string) []WikiPageParentDTO {
	if cat == nil {
		return nil
	}

	out := []WikiPageParentDTO{{
		Slug:  cat.Slug,
		Title: substituteTemplateLabel(cat.Label, spaceName),
	}}

	// Walk parent_template_id — v1 caps nesting at one level, but the loop
	// is cycle-safe (visited set) so a deeper chain survives a data fix.
	// The walk collects child→root, then reverses to root→immediate
	// parent so the emitted chain matches its documented order.
	if tmpl != nil {
		var ancestors []WikiPageParentDTO

		visited := map[pgtype.UUID]bool{tmpl.ID: true}
		for pid := tmpl.ParentTemplateID; pid.Valid; {
			if visited[pid] {
				break
			}

			visited[pid] = true

			p, err := s.store.GetPageTemplate(ctx, pid)
			if err != nil {
				break
			}

			ancestors = append(ancestors, WikiPageParentDTO{
				Slug:  p.Slug,
				Title: substituteTemplateLabel(p.Label, spaceName),
			})
			pid = p.ParentTemplateID
		}

		for i := len(ancestors) - 1; i >= 0; i-- {
			out = append(out, ancestors[i])
		}
	}

	return out
}

// childTemplates returns the templates nested under parentID (the full
// category set ordered position,slug — callers pass the list already
// fetched for page_templates). Nil parent → nil.
func childTemplates(templates []WikiPageTemplate, parentID pgtype.UUID) []WikiPageTemplate {
	if !parentID.Valid {
		return nil
	}

	out := make([]WikiPageTemplate, 0, 4)

	for _, t := range templates {
		if t.ParentTemplateID == parentID {
			out = append(out, t)
		}
	}

	return out
}

// topLevelTemplates returns the templates with no parent — the
// category stub page's composite-section set (§80 page rollup
// amendment). Distinct from childTemplates, whose invalid-parent guard
// returns nil: a nil parent means "no children" for a template page,
// but means "the whole top level" for the category page.
func topLevelTemplates(templates []WikiPageTemplate) []WikiPageTemplate {
	out := make([]WikiPageTemplate, 0, len(templates))

	for _, t := range templates {
		if !t.ParentTemplateID.Valid {
			out = append(out, t)
		}
	}

	return out
}

// compositeSections synthesizes one preview section per child template of
// the page's bound template (§80 Wiki Topic Pages §4): rows are the child
// group's own visible fields with this space's stored cfv values, capped
// at the child's preview_limit (0 = all). The section's key/page_slug is
// the child template's slug; the label is section_label fallback the
// {name}-substituted label.
//
// Fail closed: a child group the reader cannot see contributes an empty
// section (fields without can_view grants are dropped entirely — the
// preview must not leak member-restricted rows), never an error. The
// section is emitted regardless so the drill-down link still renders.
func (s *Service) compositeSections(ctx context.Context, userID, spaceID pgtype.UUID, spaceName string, children []WikiPageTemplate) []EntityFieldSectionDTO {
	out := make([]EntityFieldSectionDTO, 0, len(children))

	for _, child := range children {
		label := child.Label
		if child.SectionLabel != nil && strings.TrimSpace(*child.SectionLabel) != "" {
			label = *child.SectionLabel
		}

		fields := []EntityFieldDTO{}

		if grp, err := s.GetPageFields(ctx, userID, spaceID, child.FieldGroupID); err == nil && grp != nil {
			for _, f := range grp.Fields {
				if !f.Visible {
					continue
				}

				fields = append(fields, f)
			}

			if child.PreviewLimit > 0 && int32(len(fields)) > child.PreviewLimit { // #nosec G115 -- slice length bounded by request-scoped data
				fields = fields[:child.PreviewLimit]
			}
		} else if err != nil {
			slog.Warn("wiki: composite section fields load failed",
				"space_id", spaceID.String(), "template", child.Slug, "error", err)
		}

		out = append(out, EntityFieldSectionDTO{
			Key:      child.Slug,
			Label:    substituteTemplateLabel(label, spaceName),
			PageSlug: child.Slug,
			Fields:   fields,
		})
	}

	return out
}

// attachCompositeSections appends the child-template preview sections to
// the page's fields block, materializing a minimal group DTO when the
// page's own group carried no fields (composite-only parents).
func (s *Service) attachCompositeSections(ctx context.Context, userID, spaceID pgtype.UUID, spaceName string, groupID pgtype.UUID, dto *WikiPageDetailDTO, children []WikiPageTemplate) {
	if len(children) == 0 {
		return
	}

	sections := s.compositeSections(ctx, userID, spaceID, spaceName, children)
	if len(sections) == 0 {
		return
	}

	if dto.Fields == nil {
		grp := &EntityFieldGroupDTO{}
		if groupID.Valid {
			grp.ID = groupID.String()
			grp.Slug = dto.FieldGroupSlug

			if s.fields != nil {
				if g, err := s.fields.GetFieldGroup(ctx, groupID); err == nil {
					grp.Name = g.Name
					grp.Slug = g.Slug
				}
			}
		}

		dto.Fields = grp
	}

	dto.Fields.Sections = append(dto.Fields.Sections, sections...)
}

// SectionQuickFacts is the reserved section token (§80 Wiki Topic Pages):
// fields in it render in the Quick Facts rail card instead of a body table
// and are excluded from rollup aggregation.
const SectionQuickFacts = "quick_facts"

// rollupFieldTypes are the field types a category rollup aggregates —
// scalar numerics contribute their value directly; price_range contributes
// its composite bounds; price_parts contributes the part named by
// options.primary_part when declared (§80 Wiki Topic Pages) — a parts
// field without a primary_part still lands in rows[].values but feeds no
// aggregate.
var rollupFieldTypes = map[string]bool{
	customfields.FieldTypeNumber:     true,
	customfields.FieldTypeCurrency:   true,
	customfields.FieldTypePercentage: true,
	customfields.FieldTypePriceRange: true,
	customfields.FieldTypePriceParts: true,
}

// computeRollup aggregates descendant spaces' values for a rollup-enabled
// category (§80): eligible descendant types come from rollup.levels or the
// transitive closure of the declared type edges; rows from
// space_parent_spaces ∪ parent_id; source fields are the union of the
// same-category-token groups across those types. userID scopes the source
// fields to the caller's can_view grants — aggregates must not leak values
// for fields the reader cannot see.
func (s *Service) computeRollup(ctx context.Context, userID, spaceID, spaceTypeID pgtype.UUID, cfg *WikiRollupConfig, categoryToken string) *WikiRollupDTO {
	typeIDs, err := s.rollupTypeIDs(ctx, spaceTypeID, cfg.Levels)
	if err != nil {
		slog.Warn("wiki: rollup: type scope failed", "error", err)
		return nil
	}

	if len(typeIDs) == 0 {
		return nil
	}

	descendants, err := s.store.ListDescendantSpaces(ctx, spaceID, typeIDs)
	if err != nil {
		slog.Warn("wiki: rollup: descendants failed",
			"space_id", spaceID.String(), "error", err)
		return nil
	}

	if len(descendants) == 0 {
		return nil
	}

	// Source fields: the union of fields on each descendant type's
	// same-category group PLUS every page-template group nested under that
	// category — the itemized field groups (e.g. wiki_city_prices under
	// cost_of_living) are template groups, and a template's group is the
	// shared schema its sibling pages render. Field ids differ per group;
	// the union is keyed by field key — first type wins on a key
	// collision.

	// Field-level RBAC (§06/§80): the caller's can_view grants scope which
	// fields contribute to the aggregates and the per-descendant rows.
	// visibleFieldIDs is fail-closed — a nil set for a non-manage caller
	// (lookup error) must exclude everything, not bypass.
	canManage := s.perms != nil && s.perms.HasPermission(ctx, userID, PermFieldsManage)
	visibleSet := s.visibleFieldIDs(ctx, userID, canManage)
	fieldVisible := func(id pgtype.UUID) bool {
		if canManage {
			return true
		}

		return visibleSet != nil && visibleSet[id.String()]
	}

	defsByID := map[pgtype.UUID]rollupFieldDef{}
	defsByKey := map[string]rollupFieldDef{}
	fieldOrder := []string{}

	typeSeen := map[pgtype.UUID]bool{}
	groupSeen := map[pgtype.UUID]bool{}

	for _, d := range descendants {
		if typeSeen[d.SpaceTypeID] || s.fields == nil {
			continue
		}

		typeSeen[d.SpaceTypeID] = true

		cat, cerr := s.store.ResolveCategoryToken(ctx, d.SpaceTypeID, categoryToken)
		if cerr != nil {
			continue
		}

		groupIDs := []pgtype.UUID{}
		if cat.FieldGroupID.Valid {
			groupIDs = append(groupIDs, cat.FieldGroupID)
		}

		if templates, terr := s.store.ListPageTemplates(ctx, cat.ID); terr == nil {
			for _, t := range templates {
				if t.FieldGroupID.Valid {
					groupIDs = append(groupIDs, t.FieldGroupID)
				}
			}
		}

		for _, groupID := range groupIDs {
			if groupSeen[groupID] {
				continue
			}

			groupSeen[groupID] = true

			fields, ferr := s.fields.ListFieldsByGroup(ctx, groupID)
			if ferr != nil {
				continue
			}

			for _, f := range fields {
				if !rollupFieldTypes[f.FieldType] {
					continue
				}
				// §80 Wiki Topic Pages — the reserved `quick_facts` section
				// is rail metadata, not a comparable metric.
				if f.Section != nil && *f.Section == SectionQuickFacts {
					continue
				}

				if !fieldVisible(f.ID) {
					continue
				}

				def := rollupFieldDef{key: f.Key, label: f.Label, typ: f.FieldType, pos: f.Position}
				if f.Section != nil {
					def.section = *f.Section
				}

				def.currency = customfields.CurrencyCodeFromOptions(f.Options)
				if def.typ == customfields.FieldTypePriceParts {
					def.primaryPart = customfields.PrimaryPartFromOptions(f.Options)
					if pp := customfields.PricePartsOptionsFromJSON(f.Options); pp != nil {
						def.primaryKind = pp.PricePartKind(def.primaryPart)
						// The field-level currency_code is the schema's
						// declared code — wins over an empty context code.
						if pp.CurrencyCode != "" {
							def.currency = pp.CurrencyCode
						}
					}
				}
				// Per-field aggregation keeps its own declared type so a
				// mixed-type key (price_range on one type, number on
				// another) never feeds incompatible values into one slice;
				// the emitted row metadata is the first-seen def — the
				// descendants query orders by id/depth so that choice is
				// deterministic.
				defsByID[f.ID] = def

				if _, seen := defsByKey[f.Key]; !seen {
					defsByKey[f.Key] = def

					fieldOrder = append(fieldOrder, f.Key)
				}
			}
		}
	}

	if len(defsByID) == 0 {
		return nil
	}

	ids := make([]pgtype.UUID, 0, len(descendants))
	bySpace := map[pgtype.UUID]WikiDescendant{}

	for _, d := range descendants {
		ids = append(ids, d.ID)
		bySpace[d.ID] = d
	}

	vals, err := s.fields.ListFieldValuesWithDefsForEntities(ctx, "space", ids)
	if err != nil {
		slog.Warn("wiki: rollup: values failed",
			"space_id", spaceID.String(), "error", err)
		return nil
	}

	metrics := map[string]bool{}
	for _, m := range cfg.Metrics {
		metrics[m] = true
	}

	if len(metrics) == 0 {
		metrics = map[string]bool{"min": true, "avg": true, "max": true}
	}

	fieldsOut, rowsBySpace := aggregateRollupValues(vals, defsByID, defsByKey, fieldOrder, metrics)

	// §80 §7 cardinality guard: the config's section_limit caps the
	// descendants table; the full set stays reachable via
	// GET /wiki/destinations?within={slug}&level={level}.
	rowsLimit := cfg.SectionLimit
	if rowsLimit <= 0 {
		rowsLimit = defaultRollupRowsLimit
		if s.settings != nil {
			rowsLimit = s.settings.GetIntOr(ctx, "wiki.rollup_default_rows_limit", defaultRollupRowsLimit)
		}
	}

	if rowsLimit <= 0 {
		rowsLimit = defaultRollupRowsLimit
	}

	// ListDescendantSpaces returns rows in DISTINCT ON order (UUID) — sort
	// by depth then name so the rendered "Cost of Living by City" table is
	// deterministic and section_limit truncation drops the tail, not an
	// arbitrary subset.
	sort.SliceStable(descendants, func(i, j int) bool {
		if descendants[i].Depth != descendants[j].Depth {
			return descendants[i].Depth < descendants[j].Depth
		}

		return descendants[i].Name < descendants[j].Name
	})

	rowsOut := make([]WikiRollupRowDTO, 0, len(descendants))
	for _, d := range descendants {
		if len(rowsOut) >= rowsLimit {
			break
		}

		vals, ok := rowsBySpace[d.ID]
		if !ok || len(vals) == 0 {
			continue
		}

		rowsOut = append(rowsOut, WikiRollupRowDTO{
			Space: WikiSpaceRefDTO{
				ID:       d.ID.String(),
				Slug:     d.Slug,
				Name:     d.Name,
				Level:    d.Level,
				TypeSlug: d.SpaceTypeSlug,
			},
			Values: vals,
		})
	}

	return &WikiRollupDTO{
		Fields: fieldsOut,
		Rows:   rowsOut,
		Spaces: len(rowsBySpace),
	}
}

// computePageRollup aggregates a category's own sub-pages (mode
// "pages"): rows are the configured page templates — or every template
// under the category when cfg.Pages is empty — and values are the host
// space's cfv in each page's bound group. Field defs, metric defaults,
// the can_view gate, and the aggregation fold are identical to
// computeRollup; only the row axis changes.
func (s *Service) computePageRollup(ctx context.Context, userID, spaceID pgtype.UUID, spaceSlug, spaceName string, cfg *WikiRollupConfig, cat WikiCategory, templates []WikiPageTemplate) *WikiRollupDTO {
	if s.fields == nil {
		return nil
	}

	sources := make([]WikiPageTemplate, 0, len(templates))

	if len(cfg.Pages) > 0 {
		bySlug := make(map[string]WikiPageTemplate, len(templates))
		for _, t := range templates {
			bySlug[t.Slug] = t
		}

		for _, slug := range cfg.Pages {
			if t, ok := bySlug[slug]; ok {
				sources = append(sources, t)
			} else {
				slog.Warn("wiki: page rollup: unknown template slug", "slug", slug)
			}
		}
	} else {
		sources = templates
	}

	if len(sources) == 0 {
		return nil
	}

	// §7 cardinality guard: section_limit caps the rows table when
	// configured. Unconfigured needs no default — the row count is
	// already bounded by the template count.
	if cfg.SectionLimit > 0 && len(sources) > cfg.SectionLimit {
		sources = sources[:cfg.SectionLimit]
	}

	// include_self prepends the category's own group as the first row
	// — its slug links back to this very page. Prepended (not appended)
	// so it wins a group-sharing template in the groupSeen dedup.
	if cfg.IncludeSelf && cat.FieldGroupID.Valid {
		sources = append([]WikiPageTemplate{{
			Slug:         cat.Slug,
			Label:        cat.Label,
			FieldGroupID: cat.FieldGroupID,
		}}, sources...)
	}

	canManage := s.perms != nil && s.perms.HasPermission(ctx, userID, PermFieldsManage)
	visibleSet := s.visibleFieldIDs(ctx, userID, canManage)
	fieldVisible := func(id pgtype.UUID) bool {
		if canManage {
			return true
		}

		return visibleSet != nil && visibleSet[id.String()]
	}

	defsByID := map[pgtype.UUID]rollupFieldDef{}
	defsByKey := map[string]rollupFieldDef{}
	fieldOrder := []string{}
	fieldSource := map[pgtype.UUID]int{} // field id → index into sources
	groupSeen := map[pgtype.UUID]bool{}

	for i, t := range sources {
		if !t.FieldGroupID.Valid || groupSeen[t.FieldGroupID] {
			continue // two templates sharing a group → fields attach to the first
		}

		groupSeen[t.FieldGroupID] = true

		fields, err := s.fields.ListFieldsByGroup(ctx, t.FieldGroupID)
		if err != nil {
			slog.Warn("wiki: page rollup: fields failed", "group_id", t.FieldGroupID.String(), "error", err)
			continue
		}

		for _, f := range fields {
			if !rollupFieldTypes[f.FieldType] {
				continue
			}

			if f.Section != nil && *f.Section == SectionQuickFacts {
				continue
			}

			if !fieldVisible(f.ID) {
				continue
			}

			def := rollupFieldDef{key: f.Key, label: f.Label, typ: f.FieldType, pos: f.Position}
			if f.Section != nil {
				def.section = *f.Section
			}

			def.options = f.Options
			def.currency = customfields.CurrencyCodeFromOptions(f.Options)

			if def.typ == customfields.FieldTypePriceParts {
				def.primaryPart = customfields.PrimaryPartFromOptions(f.Options)
				if pp := customfields.PricePartsOptionsFromJSON(f.Options); pp != nil {
					def.primaryKind = pp.PricePartKind(def.primaryPart)
					if pp.CurrencyCode != "" {
						def.currency = pp.CurrencyCode
					}
				}
			}

			defsByID[f.ID] = def
			fieldSource[f.ID] = i

			if _, seen := defsByKey[f.Key]; !seen {
				defsByKey[f.Key] = def

				fieldOrder = append(fieldOrder, f.Key)
			}
		}
	}

	if len(defsByID) == 0 {
		return nil
	}

	vals, err := s.fields.ListFieldValuesWithDefsForEntities(ctx, "space", []pgtype.UUID{spaceID})
	if err != nil {
		slog.Warn("wiki: page rollup: values failed", "space_id", spaceID.String(), "error", err)
		return nil
	}

	metrics := map[string]bool{}
	for _, m := range cfg.Metrics {
		metrics[m] = true
	}

	if len(metrics) == 0 {
		metrics = map[string]bool{"min": true, "avg": true, "max": true}
	}

	// Computed fields never read their own stored cfv — a stale row must
	// not shadow the fold either in cells or in the column aggregates
	// (§80 computed contract). Stored rows of computed fields drop out
	// of both passes; each computed field's folded value re-enters as a
	// synthetic row so aggregateRollupValues folds it as its one sample.
	byCoord := map[string]gen.ListCustomFieldValuesWithDefsForEntitiesRow{}
	for _, v := range vals {
		byCoord[v.FieldGroupID.String()+"/"+v.FieldKey] = v
	}

	groupIDs := map[string]pgtype.UUID{} // slug → id memo
	resolveGroup := func(slug string) pgtype.UUID {
		if gid, ok := groupIDs[slug]; ok {
			return gid
		}

		g, err := s.fields.GetFieldGroupBySlug(ctx, slug)
		if err != nil {
			groupIDs[slug] = pgtype.UUID{}
			return pgtype.UUID{}
		}

		groupIDs[slug] = g.ID

		return g.ID
	}
	computedCell := func(def rollupFieldDef) (any, bool) {
		spec := customfields.ComputedSpecFromOptions(def.options)
		if spec == nil {
			return nil, false
		}

		scalars := make([]float64, 0, len(spec.Sources))

		for _, src := range spec.Sources {
			gid := resolveGroup(src.Group)
			if !gid.Valid {
				continue
			}

			sv, ok := byCoord[gid.String()+"/"+src.Key]
			if !ok || !fieldVisible(sv.FieldID) {
				continue
			}

			if f, ok := customfields.ComputedScalarFromValue(sv.FieldType, sv.FieldOptions, sv.ValueJson, sv.ValueNumber); ok {
				scalars = append(scalars, f)
			}
		}

		if len(scalars) == 0 {
			return nil, false
		}

		return customfields.ComputedDisplayValue(def.typ, def.options, customfields.FoldComputedMetric(spec.Metric, scalars)), true
	}

	foldVals := make([]gen.ListCustomFieldValuesWithDefsForEntitiesRow, 0, len(vals))
	computedCells := map[pgtype.UUID]any{} // field id → folded display value

	for _, v := range vals {
		def, ok := defsByID[v.FieldID]
		if !ok {
			foldVals = append(foldVals, v)
			continue
		}

		if !customfields.ComputedEligibleField(def.typ, def.options) {
			foldVals = append(foldVals, v)
			continue
		}

		if cell, ok := computedCell(def); ok {
			computedCells[v.FieldID] = cell

			if syn, ok := rollupSyntheticRow(v, def, cell); ok {
				foldVals = append(foldVals, syn)
			}
		}
	}
	// Computed fields with no stored row never enter vals — fold them
	// into the cells and column samples the same way.
	for id, def := range defsByID {
		if !customfields.ComputedEligibleField(def.typ, def.options) {
			continue
		}

		if _, done := computedCells[id]; done {
			continue
		}

		if cell, ok := computedCell(def); ok {
			computedCells[id] = cell

			if syn, ok := rollupSyntheticRow(
				gen.ListCustomFieldValuesWithDefsForEntitiesRow{FieldID: id, EntityID: spaceID, FieldType: def.typ},
				def, cell); ok {
				foldVals = append(foldVals, syn)
			}
		}
	}

	fieldsOut, _ := aggregateRollupValues(foldVals, defsByID, defsByKey, fieldOrder, metrics)

	// Row values bucket by the field's source template — the entity axis
	// is the single host space, so the fold's rowsByEntity map cannot
	// split pages; fieldSource carries the per-field ownership.
	pageValues := make([]map[string]any, len(sources))

	for _, v := range vals {
		def, ok := defsByID[v.FieldID]
		if !ok {
			continue
		}

		i, ok := fieldSource[v.FieldID]
		if !ok {
			continue
		}

		if _, computed := computedCells[v.FieldID]; computed {
			continue // written from the folded value below
		}

		if customfields.ComputedEligibleField(def.typ, def.options) {
			continue // spec'd but unfolded (no usable sources) — no stored fallback
		}

		if pageValues[i] == nil {
			pageValues[i] = map[string]any{}
		}

		pageValues[i][def.key] = customfields.FieldValueFromDefRow(
			v.ValueText, v.ValueNumber, v.ValueBool, v.ValueDate, v.ValueJson, v.FieldType)
	}

	for id, cell := range computedCells {
		def := defsByID[id]

		i, ok := fieldSource[id]
		if !ok {
			continue
		}

		if pageValues[i] == nil {
			pageValues[i] = map[string]any{}
		}

		pageValues[i][def.key] = cell
	}

	// Every resolved template emits a row — a page with no values still
	// renders its drill-down link with '—' cells. Spaces counts the pages
	// that contributed at least one value.
	contrib := 0
	rows := make([]WikiRollupRowDTO, 0, len(sources))

	for i, t := range sources {
		title := substituteTemplateLabel(t.Label, spaceName)
		if t.SectionLabel != nil && strings.TrimSpace(*t.SectionLabel) != "" {
			title = *t.SectionLabel
		}

		pv := pageValues[i]
		if pv == nil {
			pv = map[string]any{}
		} else {
			contrib++
		}

		rows = append(rows, WikiRollupRowDTO{
			Space:  WikiSpaceRefDTO{Slug: spaceSlug, Name: spaceName},
			Page:   &WikiRollupPageRefDTO{Slug: t.Slug, Title: title},
			Values: pv,
		})
	}

	return &WikiRollupDTO{Scope: rollupScopePages, Fields: fieldsOut, Rows: rows, Spaces: contrib}
}

// rollupSyntheticRow rebuilds a folded computed value as a cfv row in
// the field type's storage shape so aggregateRollupValues folds it as
// the field's one sample — scalar types land in value_number,
// price_range/price_parts marshal their display JSON back.
func rollupSyntheticRow(v gen.ListCustomFieldValuesWithDefsForEntitiesRow, def rollupFieldDef, cell any) (gen.ListCustomFieldValuesWithDefsForEntitiesRow, bool) {
	out := v // FieldID/EntityID/FieldType carry over
	out.ValueText, out.ValueBool, out.ValueJson = nil, nil, nil
	out.ValueDate = pgtype.Timestamptz{}
	out.ValueNumber = pgtype.Numeric{}

	switch def.typ {
	case customfields.FieldTypePriceRange, customfields.FieldTypePriceParts:
		b, err := json.Marshal(cell)
		if err != nil {
			return out, false
		}

		out.ValueJson = b
	default:
		f, ok := cell.(float64)
		if !ok {
			return out, false
		}

		var n pgtype.Numeric
		if err := n.Scan(strconv.FormatFloat(f, 'f', -1, 64)); err != nil {
			return out, false
		}

		out.ValueNumber = n
	}

	return out, true
}

// rollupFieldDef is the schema member a rollup aggregates — one custom
// field on a descendant type's same-category group. primaryPart is the
// price_parts comparable member (§80 Wiki Topic Pages) — empty when the
// field declares no primary_part (non-comparable; rows-only).
// primaryKind is that part's declared kind (money|number) so row-cell
// renderers format it correctly without the full options schema.
// options is the field's raw options JSON — pages mode reads
// options.computed from it.
type rollupFieldDef struct {
	key         string
	label       string
	typ         string
	section     string
	options     []byte
	currency    string
	primaryPart string
	primaryKind string
	pos         int32
}

// aggregateRollupValues folds raw cfv rows into per-key metric DTOs and
// the per-space values map. Pure — no I/O — so the dangerous parts of the
// rollup (empty aggregate slices, partial/malformed price_range payloads,
// non-finite numerics) are unit-testable without a database.
//
// Safety contract: minOf/maxOf/avgOf index v[0] — every call site must be
// behind a len guard, and only finite values may enter the accumulators
// (a stored NaN would both poison avgOf and fail json.Marshal).
func aggregateRollupValues(
	vals []gen.ListCustomFieldValuesWithDefsForEntitiesRow,
	defsByID map[pgtype.UUID]rollupFieldDef,
	defsByKey map[string]rollupFieldDef,
	fieldOrder []string,
	metrics map[string]bool,
) ([]WikiRollupFieldDTO, map[pgtype.UUID]map[string]any) {
	type accum struct {
		mins, avgs, maxs []float64
		scalars          []float64
		// priceN / scalarN count value rows that produced at least one
		// usable number for that interpretation — the key's n counts
		// only contributions the emitted field type can use ("spaces
		// lacking a value for a key are excluded from that key's n").
		priceN, scalarN int
	}

	accByKey := map[string]*accum{}
	rowsBySpace := map[pgtype.UUID]map[string]any{}

	for _, v := range vals {
		def, ok := defsByID[v.FieldID]
		if !ok {
			continue
		}

		if rowsBySpace[v.EntityID] == nil {
			rowsBySpace[v.EntityID] = map[string]any{}
		}

		rowsBySpace[v.EntityID][def.key] = customfields.FieldValueFromDefRow(
			v.ValueText, v.ValueNumber, v.ValueBool, v.ValueDate, v.ValueJson, v.FieldType)

		a := accByKey[def.key]
		if a == nil {
			a = &accum{}
			accByKey[def.key] = a
		}

		if def.typ == customfields.FieldTypePriceRange {
			// Admin-written values can bypass normalizePriceRange, so
			// tolerate any partial object here: each bound contributes
			// to its own metric independently, and a value with no
			// recognized bound contributes nothing.
			var pr struct {
				Avg *float64 `json:"avg"`
				Min *float64 `json:"min"`
				Max *float64 `json:"max"`
			}

			if json.Unmarshal(v.ValueJson, &pr) != nil {
				continue
			}

			contributed := false

			if pr.Min != nil && finite(*pr.Min) {
				a.mins = append(a.mins, *pr.Min)
				contributed = true
			}

			if pr.Avg != nil && finite(*pr.Avg) {
				a.avgs = append(a.avgs, *pr.Avg)
				contributed = true
			}

			if pr.Max != nil && finite(*pr.Max) {
				a.maxs = append(a.maxs, *pr.Max)
				contributed = true
			}
			// §80 §7: an {avg}-only record informs all three metrics —
			// the record's avg stands in for the bounds it didn't
			// declare, per record (a {min:10} plus {avg:2} set yields
			// min=2, not 10).
			if pr.Min == nil && pr.Avg != nil && finite(*pr.Avg) {
				a.mins = append(a.mins, *pr.Avg)
			}

			if pr.Max == nil && pr.Avg != nil && finite(*pr.Avg) {
				a.maxs = append(a.maxs, *pr.Avg)
			}

			if contributed {
				a.priceN++
			}
		} else if def.typ == customfields.FieldTypePriceParts {
			// §80 Wiki Topic Pages: project the declared primary part — a
			// parts field without one is rows-only (non-comparable).
			if def.primaryPart == "" {
				continue
			}

			var pp struct {
				Parts map[string]json.RawMessage `json:"parts"`
			}

			if json.Unmarshal(v.ValueJson, &pp) != nil {
				continue
			}

			raw, ok := pp.Parts[def.primaryPart]
			if !ok {
				continue
			}

			var f float64
			if json.Unmarshal(raw, &f) != nil || !finite(f) {
				continue
			}

			a.scalars = append(a.scalars, f)
			a.scalarN++
		} else if f, ok := cfvNumeric(v.ValueNumber); ok && finite(f) {
			// A stored NaN/Inf (pg numeric accepts 'NaN') would poison
			// avgOf and fail json.Marshal downstream.
			a.scalars = append(a.scalars, f)
			a.scalarN++
		}
	}

	fieldsOut := make([]WikiRollupFieldDTO, 0, len(fieldOrder))

	for _, key := range fieldOrder {
		def := defsByKey[key]

		a := accByKey[key]
		if a == nil {
			continue
		}

		samples := a.scalarN
		if def.typ == customfields.FieldTypePriceRange {
			samples = a.priceN
		}

		if samples == 0 {
			continue
		}

		// Each metric aggregates only the bounds that were actually
		// contributed — a {min,max} price_range yields min/max and no
		// avg; empty slices are guarded, never indexed.
		var m WikiRollupMetricDTO

		if def.typ == customfields.FieldTypePriceRange {
			if metrics["min"] && len(a.mins) > 0 {
				m.Min = f64ptr(minOf(a.mins))
			}

			if metrics["avg"] && len(a.avgs) > 0 {
				m.Avg = f64ptr(avgOf(a.avgs))
			}

			if metrics["max"] && len(a.maxs) > 0 {
				m.Max = f64ptr(maxOf(a.maxs))
			}
		} else {
			if metrics["min"] && len(a.scalars) > 0 {
				m.Min = f64ptr(minOf(a.scalars))
			}

			if metrics["avg"] && len(a.scalars) > 0 {
				m.Avg = f64ptr(avgOf(a.scalars))
			}

			if metrics["max"] && len(a.scalars) > 0 {
				m.Max = f64ptr(maxOf(a.scalars))
			}
		}

		fieldsOut = append(fieldsOut, WikiRollupFieldDTO{
			Key: key, Label: def.label, FieldType: def.typ, Section: def.section,
			CurrencyCode: def.currency, PrimaryPart: def.primaryPart, PrimaryKind: def.primaryKind,
			Metrics: m, Samples: samples,
		})
	}

	return fieldsOut, rowsBySpace
}

// rollupTypeIDs resolves the eligible descendant space types: explicit
// level slugs via space_types, or the transitive closure (depth ≤3) of the
// declared child edges when levels is omitted.
func (s *Service) rollupTypeIDs(ctx context.Context, spaceTypeID pgtype.UUID, levels []string) ([]pgtype.UUID, error) {
	if len(levels) > 0 {
		ids := make([]pgtype.UUID, 0, len(levels))

		for _, lvl := range levels {
			id, err := s.store.SpaceTypeIDBySlug(ctx, lvl)
			if err != nil {
				continue // unknown level slugs are ignored, not fatal
			}

			ids = append(ids, id)
		}

		return ids, nil
	}

	seen := map[pgtype.UUID]bool{}

	frontier := []pgtype.UUID{spaceTypeID}
	for depth := 0; depth < 3 && len(frontier) > 0; depth++ {
		next := []pgtype.UUID{}

		for _, tid := range frontier {
			kids, err := s.store.ListTypeChildIDs(ctx, tid)
			if err != nil {
				return nil, err
			}

			for _, k := range kids {
				if !seen[k] && k != spaceTypeID {
					seen[k] = true

					next = append(next, k)
				}
			}
		}

		frontier = next
	}

	out := make([]pgtype.UUID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}

	return out, nil
}

func f64ptr(f float64) *float64 { return &f }

func minOf(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		if x < m {
			m = x
		}
	}

	return m
}

func maxOf(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}

	return m
}

func avgOf(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x
	}

	return sum / float64(len(v))
}

// finite rejects NaN/Inf — pg numeric accepts 'NaN', and a non-finite
// aggregate fails json.Marshal on the response.
func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// cfvNumeric extracts a float64 from a numeric cfv column.
func cfvNumeric(n pgtype.Numeric) (float64, bool) {
	if !n.Valid {
		return 0, false
	}

	if f, err := n.Float64Value(); err == nil && f.Valid {
		return f.Float64, true
	}

	return 0, false
}
