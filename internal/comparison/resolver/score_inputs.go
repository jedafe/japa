package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/costofliving"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/visaroutes"
)

// resolverOptions carries optional dependencies for resolvers.
// Keeping them in a single struct makes constructors variadic while
// remaining nil-safe.
type resolverOptions struct {
	colStore *costofliving.Store
	visaSvc  *visaroutes.Service
	queries  *gen.Queries
}

// Option configures an optional resolver dependency.
type Option func(*resolverOptions)

// WithCOLStore provides a cost-of-living store for populating geo scores.
func WithCOLStore(s *costofliving.Store) Option {
	return func(o *resolverOptions) { o.colStore = s }
}

// WithVisaService provides a visa-routes service for PR-path-ease lookups.
func WithVisaService(s *visaroutes.Service) Option {
	return func(o *resolverOptions) { o.visaSvc = s }
}

// WithQueries provides a sqlc Queries instance for direct custom-field reads.
func WithQueries(q *gen.Queries) Option {
	return func(o *resolverOptions) { o.queries = q }
}

func newResolverOptions(opts ...Option) *resolverOptions {
	o := &resolverOptions{}
	for _, opt := range opts {
		opt(o)
	}

	return o
}

// scoreDeps holds the optional data sources used to backfill real score_inputs.
type scoreDeps struct {
	queries  *gen.Queries
	colStore *costofliving.Store
	visaSvc  *visaroutes.Service
}

// spaceBase is a normalised view of a space row used when building resolved items.
type spaceBase struct {
	ID            pgtype.UUID
	ParentID      pgtype.UUID
	SpaceTypeID   pgtype.UUID
	Name          string
	Slug          string
	Description   *string
	GeoCode       *string
	Level         string
	LogoMediaID   pgtype.UUID
	HeaderImageID pgtype.UUID
	MemberCount   int32
}

func spaceBaseFromRow(row any) (spaceBase, bool) {
	var b spaceBase
	switch r := row.(type) {
	case gen.GetSpaceByIDRow:
		b = spaceBase{
			ID: r.ID, ParentID: r.ParentID, SpaceTypeID: r.SpaceTypeID,
			Name: r.Name, Slug: r.Slug, Description: r.Description,
			GeoCode: r.GeoCode, Level: r.Level,
			LogoMediaID: r.LogoMediaID, HeaderImageID: r.HeaderImageID,
			MemberCount: r.MemberCount,
		}
	case gen.GetSpaceBySlugRow:
		b = spaceBase{
			ID: r.ID, ParentID: r.ParentID, SpaceTypeID: r.SpaceTypeID,
			Name: r.Name, Slug: r.Slug, Description: r.Description,
			GeoCode: r.GeoCode, Level: r.Level,
			LogoMediaID: r.LogoMediaID, HeaderImageID: r.HeaderImageID,
			MemberCount: r.MemberCount,
		}
	case gen.ListSpacesByTypePaginatedRow:
		b = spaceBase{
			ID: r.ID, ParentID: r.ParentID, SpaceTypeID: r.SpaceTypeID,
			Name: r.Name, Slug: r.Slug, Description: r.Description,
			GeoCode: r.GeoCode, Level: r.Level,
			LogoMediaID: r.LogoMediaID, HeaderImageID: r.HeaderImageID,
			MemberCount: r.MemberCount,
		}
	case gen.ListSpacesPaginatedRow:
		b = spaceBase{
			ID: r.ID, ParentID: r.ParentID, SpaceTypeID: r.SpaceTypeID,
			Name: r.Name, Slug: r.Slug, Description: r.Description,
			GeoCode: r.GeoCode, Level: r.Level,
			LogoMediaID: r.LogoMediaID, HeaderImageID: r.HeaderImageID,
			MemberCount: r.MemberCount,
		}
	case gen.GetSpaceByGeoCodeRow:
		b = spaceBase{
			ID: r.ID, ParentID: r.ParentID, SpaceTypeID: r.SpaceTypeID,
			Name: r.Name, Slug: r.Slug, Description: r.Description,
			GeoCode: r.GeoCode, Level: r.Level,
			LogoMediaID: r.LogoMediaID, HeaderImageID: r.HeaderImageID,
			MemberCount: r.MemberCount,
		}
	case gen.ListSpacesByAncestorAndTypeRow:
		b = spaceBase{
			ID: r.ID, ParentID: r.ParentID, SpaceTypeID: r.SpaceTypeID,
			Name: r.Name, Slug: r.Slug, Description: r.Description,
			GeoCode: r.GeoCode, Level: r.Level,
			LogoMediaID: r.LogoMediaID, HeaderImageID: r.HeaderImageID,
			MemberCount: r.MemberCount,
		}
	default:
		return b, false
	}

	return b, true
}

// depsFieldValues fetches one space-family entity's community field values
// (entity_type normalized to 'space') — nil-safe for the nil-deps helpers.
func depsFieldValues(ctx context.Context, deps *scoreDeps, entityType string, id pgtype.UUID) map[string]any {
	if deps == nil || deps.queries == nil {
		return nil
	}

	return customfields.EntityFieldValues(ctx, deps.queries, entityType, id)
}

// batchedSpaceFieldValues fetches community field values for a page of
// space-family entities in one query — never per-row (§80).
func batchedSpaceFieldValues(ctx context.Context, deps *scoreDeps, entityType string, bases []spaceBase) map[pgtype.UUID]map[string]any {
	if deps == nil || deps.queries == nil {
		return nil
	}

	ids := make([]pgtype.UUID, 0, len(bases))
	for _, b := range bases {
		if b.ID.Valid {
			ids = append(ids, b.ID)
		}
	}

	return customfields.EntityFieldValuesForEntities(ctx, deps.queries, entityType, ids)
}

func resolveSpaceBase(ctx context.Context, deps *scoreDeps, entityType string, base spaceBase, cfv map[string]any) ResolvedItem {
	desc := ""
	if base.Description != nil {
		desc = *base.Description
	}

	imageID := ""
	if base.LogoMediaID.Valid {
		imageID = base.LogoMediaID.String()
	} else if base.HeaderImageID.Valid {
		imageID = base.HeaderImageID.String()
	}

	meta := map[string]any{
		"geo_space_id": base.ID.String(),
		"slug":         base.Slug,
		"level":        base.Level,
		"geo_code":     deref(base.GeoCode),
		"image_id":     imageID,
		"member_count": base.MemberCount,
		"score_inputs": spaceScoreInputs(ctx, deps, entityType, base, cfv),
	}

	// §30 Sub-spec 5: expose the entity's custom field values so matrix cells
	// can hydrate from them when no comparison_values row exists. The caller
	// passes cfv (batched in Search paths) — never fetched per row here.
	if len(cfv) > 0 {
		meta["field_values"] = cfv
	}

	if entityType != "space" {
		meta["space"] = true
	}

	if base.Level == "country" && base.GeoCode != nil && *base.GeoCode != "" {
		meta["country"] = *base.GeoCode
	}

	return ResolvedItem{
		Type:        entityType,
		ID:          base.ID.String(),
		Title:       base.Name,
		Description: Excerpt(desc, 160),
		Metadata:    meta,
	}
}

func spaceScoreInputs(ctx context.Context, deps *scoreDeps, entityType string, base spaceBase, cfv map[string]any) map[string]any {
	if entityType == "university" {
		return universityScoreInputs(ctx, deps, base, cfv)
	}

	lang := languagesForGeoCode(deref(base.GeoCode))
	inputs := map[string]any{
		"cost":             nil,
		"income":           nil,
		"speed":            nil,
		"pr_path_ease":     nil,
		"safety_index":     nil,
		"healthcare_index": nil,
		"language":         lang,
		"geo_space_id":     base.ID.String(),
	}

	if deps != nil && deps.colStore != nil {
		row, err := deps.colStore.GetCityByGeoSpaceID(ctx, base.ID)
		if err == nil && row.ID.Valid {
			cost, income, safety, healthcare := colScoreInputs(row.IntelCostData)
			inputs["cost"] = cost
			inputs["income"] = income
			inputs["safety_index"] = safety
			inputs["healthcare_index"] = healthcare
		}
	}

	if deps != nil && deps.visaSvc != nil {
		country := base
		if base.Level != "country" {
			country = findCountrySpace(ctx, deps, base)
		}

		if country.ID.Valid {
			inputs["pr_path_ease"] = avgPRPathEase(ctx, deps.visaSvc, country.ID)
		}
	}

	return inputs
}

func universityScoreInputs(ctx context.Context, deps *scoreDeps, base spaceBase, cfv map[string]any) map[string]any {
	lang := languagesForGeoCode(deref(base.GeoCode))
	inputs := map[string]any{
		"cost":                   nil,
		"income":                 nil,
		"acceptance_rate":        nil,
		"duration_years":         nil,
		"qs_ranking":             nil,
		"pr_path_ease":           nil,
		"post_study_work_months": nil,
		"language":               lang,
		"accreditation":          []string{},
		"fields_of_study":        []string{},
		"geo_space_id":           base.ID.String(),
	}

	// cfv arrives prefetched by the caller — the same map resolveSpaceBase
	// exposes as field_values — so score inputs and matrix hydration share
	// one read per entity (§80).
	if cfv != nil {
		for k, v := range universityScoreInputsFromCustomFields(cfv) {
			inputs[k] = v
		}
	}

	// University cost is intentionally left nil when the tuition_range custom
	// field is absent. Falling back to city cost-of-living would treat living
	// expenses as tuition, which is not the intended factor semantics.

	if deps != nil && deps.visaSvc != nil {
		country := base
		if base.Level != "country" {
			country = findCountrySpace(ctx, deps, base)
		}

		if country.ID.Valid {
			inputs["pr_path_ease"] = avgPRPathEase(ctx, deps.visaSvc, country.ID)
		}
	}

	return inputs
}

// resolveFieldValues returns an entity's custom field values keyed by field
// key, read from custom_field_values joined to custom_fields (§30 Sub-spec 5
// entity-field hydration). The entity type is normalized through
// customfields.ValueEntityType so space-backed types read entity_type='space'.
// It returns nil when the resolver has no queries handle, the entity ID is
// invalid, or the lookup fails — field_values metadata is best-effort
// enrichment and must never fail a resolve.
func resolveFieldValues(ctx context.Context, q *gen.Queries, entityType string, entityID pgtype.UUID) map[string]any {
	return customfields.EntityFieldValues(ctx, q, entityType, entityID)
}

func universityScoreInputsFromCustomFields(values map[string]any) map[string]any {
	out := map[string]any{
		"cost":                   nil,
		"acceptance_rate":        nil,
		"qs_ranking":             nil,
		"duration_years":         nil,
		"post_study_work_months": nil,
		"accreditation":          []string{},
		"fields_of_study":        []string{},
	}

	if v := fieldValueNumber(values, "acceptance_rate"); v != nil {
		out["acceptance_rate"] = v
	}

	if v := fieldValueNumber(values, "world_ranking", "qs_ranking"); v != nil {
		out["qs_ranking"] = v
	}

	if v := fieldValueNumber(values, "duration", "duration_years"); v != nil {
		out["duration_years"] = v
	}

	if v := fieldValueNumber(values, "post_study_work_months", "post_study_work", "work_permit_months"); v != nil {
		out["post_study_work_months"] = v
	}

	if fields := fieldValueStrings(values, "programs_offered", "fields_of_study"); fields != nil {
		out["fields_of_study"] = fields
	}

	if accr := fieldValueStrings(values, "accreditation_body", "accreditation"); accr != nil {
		out["accreditation"] = accr
	}

	if v, ok := values["tuition_range"]; ok && v != nil {
		out["cost"] = parseTuitionRange(v)
	}

	return out
}

func parseTuitionRange(v any) any {
	s, ok := v.(string)
	if !ok {
		s = fmt.Sprint(v)
	}

	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "free" {
		return 0.0
	}

	re := regexp.MustCompile(`[\d,]+`)

	matches := re.FindAllString(s, -1)
	if len(matches) == 0 {
		if strings.Contains(s, "<") {
			return 2500.0
		}

		if strings.Contains(s, ">") {
			return 40000.0
		}

		return nil
	}

	var nums []float64

	for _, m := range matches {
		m = strings.ReplaceAll(m, ",", "")
		if f, err := strconv.ParseFloat(m, 64); err == nil {
			// Values like "5k" are in thousands of USD.
			if f < 1000 && strings.Contains(s, "k") {
				f *= 1000
			}

			nums = append(nums, f)
		}
	}

	if len(nums) == 0 {
		return nil
	}

	if len(nums) == 1 {
		n := nums[0]
		if strings.Contains(s, "<") && !strings.Contains(s, "-") {
			return n / 2
		}

		if strings.Contains(s, ">") && !strings.Contains(s, "-") {
			return n * 1.5
		}

		return n
	}

	sum := 0.0
	for _, n := range nums {
		sum += n
	}

	return sum / float64(len(nums))
}

func colScoreInputs(data []byte) (cost, income, safety, healthcare any) {
	parsed := parseCOLJSON(data)
	if parsed == nil {
		return nil, nil, nil, nil
	}

	cost = colCostFromParsed(parsed)
	income = colNumeric(parsed, "avg_net_salary")
	safety = colNumeric(parsed, "safety_index")
	healthcare = colNumeric(parsed, "healthcare_index")

	return
}

func parseCOLJSON(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}

	return parsed
}

func colNumeric(parsed map[string]any, key string) any {
	if v, ok := parsed[key]; ok && v != nil {
		if n, ok := coerceNumber(v); ok {
			return reflectAnyToFloat(n)
		}
	}

	return nil
}

func colCostFromParsed(parsed map[string]any) any {
	if v, ok := parsed["cost"]; ok && v != nil {
		if n, ok := coerceNumber(v); ok {
			return reflectAnyToFloat(n)
		}
	}

	keys := []string{
		"rent_1br_city_centre",
		"rent_1br_outside",
		"groceries_monthly",
		"transport_monthly",
		"utilities_monthly",
		"internet_monthly",
		"gym_monthly",
	}
	total := 0.0
	hasAny := false

	for i, k := range keys {
		if i == 1 {
			// Count only one rent category; prefer city centre.
			if _, ok := parsed["rent_1br_city_centre"]; ok {
				continue
			}
		}

		if v, ok := parsed[k]; ok && v != nil {
			if n, ok := coerceNumber(v); ok {
				total += reflectAnyToFloat(n)
				hasAny = true
			}
		}
	}

	if hasAny {
		return total
	}

	return nil
}

func findCountrySpace(ctx context.Context, deps *scoreDeps, base spaceBase) spaceBase {
	if base.Level == "country" {
		return base
	}

	if deps == nil || deps.queries == nil {
		return spaceBase{}
	}

	if base.GeoCode != nil && *base.GeoCode != "" {
		row, err := deps.queries.GetSpaceByGeoCode(ctx,
			base.GeoCode,
		)
		if err == nil && row.Level == "country" {
			if b, ok := spaceBaseFromRow(row); ok {
				return b
			}
		}
	}

	current := base
	seen := make(map[pgtype.UUID]bool)

	for range 10 {
		if !current.ParentID.Valid {
			break
		}

		if seen[current.ParentID] {
			break
		}

		seen[current.ParentID] = true

		row, err := deps.queries.GetSpaceByID(ctx,
			current.ParentID,
		)
		if err != nil {
			break
		}

		parent, ok := spaceBaseFromRow(row)
		if !ok {
			break
		}

		if parent.Level == "country" {
			return parent
		}

		current = parent
	}

	return spaceBase{}
}

func avgPRPathEase(ctx context.Context, svc *visaroutes.Service, countryID pgtype.UUID) any {
	if svc == nil || !countryID.Valid {
		return nil
	}

	const pageSize int32 = 100

	var (
		page      int32 = 1
		collected int64
		total     int64
		scores    []float64
	)

	for {
		routes, t, err := svc.List(ctx, countryID, "", "", 0, page, pageSize)
		if err != nil {
			return nil
		}

		total = t

		for _, r := range routes {
			if r.PathToPR == "" {
				continue
			}

			score, _ := prPathEaseAndEligibility(r.PathToPR)
			if score == nil {
				continue
			}

			f := numericToFloat64(score)
			if f == nil {
				continue
			}

			scores = append(scores, *f)
		}

		collected += int64(len(routes))
		if collected >= total || len(routes) == 0 {
			break
		}

		page++
	}

	if len(scores) == 0 {
		return nil
	}

	sum := 0.0
	for _, s := range scores {
		sum += s
	}

	return sum / float64(len(scores))
}

func numericToFloat64(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		f := float64(n)
		return &f
	case int:
		f := float64(n)
		return &f
	case int8:
		f := float64(n)
		return &f
	case int16:
		f := float64(n)
		return &f
	case int32:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	case uint:
		f := float64(n)
		return &f
	case uint8:
		f := float64(n)
		return &f
	case uint16:
		f := float64(n)
		return &f
	case uint32:
		f := float64(n)
		return &f
	case uint64:
		f := float64(n)
		return &f
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return &f
		}
	}

	return nil
}
