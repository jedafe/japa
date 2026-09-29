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

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// ancestorDepth caps the breadcrumb walk — mirrors the ?within= join depth
// so navigation and descendant filtering share the same horizon.
const ancestorDepth = 3

// WikiSpaceRefDTO is one node in an ancestor chain or rollup row — the
// minimal space identity a breadcrumb or table cell needs.
// //nolint:revive // exported: DTO naming follows package convention
type WikiSpaceRefDTO struct {
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
// //nolint:revive // exported: DTO naming follows package convention
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
// //nolint:revive // exported: DTO naming follows package convention
type WikiRollupConfig struct {
	Enabled      bool     `json:"enabled"`
	Mode         string   `json:"mode,omitempty"`
	Metrics      []string `json:"metrics"`
	Levels       []string `json:"levels"`
	SectionLimit int      `json:"section_limit,omitempty"`
}

// rollupModeDirectory switches a category rollup from metric aggregation
// to a child-space directory: instead of fold-up sections the page lists
// every linked descendant wiki space (with geo_code + page_count), no
// field-value requirement.
const rollupModeDirectory = "directory"

// defaultRollupRowsLimit bounds the descendants table when the category
// config sets no section_limit — the full set remains reachable through
// GET /wiki/destinations?within={slug}&level={level}.
const defaultRollupRowsLimit = 50

// WikiRollupMetricDTO carries the requested aggregates for one field —
// nil members are metrics the config didn't request.
// //nolint:revive // exported: DTO naming follows package convention
type WikiRollupMetricDTO struct {
	Min *float64 `json:"min,omitempty"`
	Avg *float64 `json:"avg,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// WikiRollupFieldDTO is one aggregated field: for scalar numeric fields
// min/avg/max are over the stored values; for price_range fields min is the
// minimum of mins, avg the mean of avgs, max the maximum of maxes.
// //nolint:revive // exported: DTO naming follows package convention
type WikiRollupFieldDTO struct {
	Key       string              `json:"key"`
	Label     string              `json:"label"`
	FieldType string              `json:"field_type"`
	Section   string              `json:"section,omitempty"`
	Metrics   WikiRollupMetricDTO `json:"metrics"`
	Samples   int                 `json:"samples"`
}

// WikiRollupRowDTO is one contributing descendant space and its values —
// the "Cost of Living by City" table rows on a country page.
// //nolint:revive // exported: DTO naming follows package convention
type WikiRollupRowDTO struct {
	Space  WikiSpaceRefDTO `json:"space"`
	Values map[string]any  `json:"values"`
}

// WikiRollupDTO is the read-time descendant aggregate attached to a
// rollup-enabled category page. Never persisted — recomputed per request.
// //nolint:revive // exported: DTO naming follows package convention
type WikiRollupDTO struct {
	Fields []WikiRollupFieldDTO `json:"fields"`
	Rows   []WikiRollupRowDTO   `json:"rows"`
	Spaces int                  `json:"spaces"`
}

// WikiDirectoryChildDTO is one card in a directory-mode listing — the
// space's identity plus geo_code and its active wiki page count.
// //nolint:revive // exported: DTO naming follows package convention
type WikiDirectoryChildDTO struct {
	Slug      string  `json:"slug"`
	Name      string  `json:"name"`
	Level     string  `json:"level"`
	TypeSlug  string  `json:"type_slug,omitempty"`
	GeoCode   *string `json:"geo_code,omitempty"`
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
// //nolint:revive // exported: DTO naming follows package convention
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

	return &WikiDirectoryDTO{
		Levels:   levels,
		Children: directoryChildRows(children, limit),
		Total:    len(children),
	}
}

// directoryChildRows maps directory children to DTOs, ordered by depth
// then name (slug tie-break) and capped at limit — deterministic so the
// truncated tail is always the same spaces, not an arbitrary subset.
// limit <= 0 means uncapped.
func directoryChildRows(children []WikiDirectoryChild, limit int) []WikiDirectoryChildDTO {
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].Depth != children[j].Depth {
			return children[i].Depth < children[j].Depth
		}
		if children[i].Name != children[j].Name {
			return children[i].Name < children[j].Name
		}
		return children[i].Slug < children[j].Slug
	})

	n := len(children)
	if limit > 0 && n > limit {
		n = limit
	}

	out := make([]WikiDirectoryChildDTO, 0, n)
	for _, c := range children[:n] {
		out = append(out, WikiDirectoryChildDTO{
			Slug: c.Slug, Name: c.Name, Level: c.Level,
			TypeSlug: c.SpaceTypeSlug, GeoCode: c.GeoCode,
			PageCount: c.PageCount, Depth: c.Depth,
		})
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
func (s *Service) getAncestors(ctx context.Context, spaceID, spaceTypeID pgtype.UUID) (chain []WikiSpaceRefDTO, alternates []WikiSpaceRefDTO) {
	visited := map[pgtype.UUID]bool{spaceID: true}

	curID, curTypeID := spaceID, spaceTypeID
	for depth := 0; depth < ancestorDepth; depth++ {
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

	return chain, alternates
}

func spaceRefFromLink(p WikiParentLink) WikiSpaceRefDTO {
	return WikiSpaceRefDTO{
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

// getSiblings returns a page's ordered sibling set with the current page
// flagged — Prev/Next derives from neighbors in this list.
func (s *Service) getSiblings(ctx context.Context, spaceID pgtype.UUID, category, currentSlug string) []WikiSiblingDTO {
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

// rollupFieldTypes are the field types a category rollup aggregates —
// scalar numerics contribute their value directly; price_range contributes
// its composite bounds.
var rollupFieldTypes = map[string]bool{
	customfields.FieldTypeNumber:     true,
	customfields.FieldTypeCurrency:   true,
	customfields.FieldTypePercentage: true,
	customfields.FieldTypePriceRange: true,
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
				if !fieldVisible(f.ID) {
					continue
				}

				def := rollupFieldDef{key: f.Key, label: f.Label, typ: f.FieldType, pos: f.Position}
				if f.Section != nil {
					def.section = *f.Section
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

// rollupFieldDef is the schema member a rollup aggregates — one custom
// field on a descendant type's same-category group.
type rollupFieldDef struct {
	key     string
	label   string
	typ     string
	section string
	pos     int32
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
