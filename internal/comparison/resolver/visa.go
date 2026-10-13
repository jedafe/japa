package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/visaroutes"
)

// visaRouteResolver resolves visa routes.
type visaRouteResolver struct {
	svc     *visaroutes.Service
	queries *gen.Queries // optional: enables custom_field_values merge
}

// NewVisaRouteResolver creates a resolver for visa routes. Pass WithQueries
// to merge custom_field_values (the canonical store) over requirements_json
// (§80 Tool Surface Hydration).
func NewVisaRouteResolver(svc *visaroutes.Service, opts ...Option) Resolver {
	o := newResolverOptions(opts...)

	return &visaRouteResolver{svc: svc, queries: o.queries}
}

func (r *visaRouteResolver) Resolve(ctx context.Context, ref map[string]any) (ResolvedItem, bool, error) {
	if idStr, ok := ref["id"].(string); ok && idStr != "" {
		id, err := ParseUUID(idStr)
		if err != nil {
			return ResolvedItem{}, false, err
		}

		route, err := r.svc.GetByID(ctx, id)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}

			return ResolvedItem{}, false, fmt.Errorf("fetching visa route by id: %w", err)
		}

		// GetByID already merges cfv into route.Requirements (§80) — pass nil
		// so the item builder uses that merged bag without a second cfv read.
		return routeToResolvedItem(route, nil), true, nil
	}

	if slug, ok := ref["slug"].(string); ok && slug != "" {
		route, err := r.svc.GetBySlug(ctx, slug)
		if err != nil {
			if IsNotFound(err) {
				return ResolvedItem{}, false, nil
			}

			return ResolvedItem{}, false, fmt.Errorf("fetching visa route by slug: %w", err)
		}

		return routeToResolvedItem(route, nil), true, nil
	}

	return ResolvedItem{}, false, errors.New("visa_route item_ref missing id or slug")
}

func (r *visaRouteResolver) Search(ctx context.Context, query string, filters map[string]any, limit, offset int32) ([]ResolvedItem, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}

	if offset < 0 {
		offset = 0
	}

	// Destination filter: accept a space UUID (country_id/geo_space_id/
	// destination_space_id) directly; otherwise leave the destination
	// unfiltered rather than silently matching nothing.
	var destSpaceID pgtype.UUID
	for _, key := range []string{"country_id", "geo_space_id", "destination_space_id", "dest"} {
		if v, ok := filters[key].(string); ok && v != "" {
			if id, err := ParseUUID(v); err == nil && id.Valid {
				destSpaceID = id
				break
			}
		}
	}

	var category string
	if v, ok := filters["visa_category"].(string); ok {
		category = v
	}

	page := int32(1)
	if limit > 0 {
		page = offset/limit + 1
	}

	routes, _, err := r.svc.AdminList(ctx, destSpaceID, "published", category, query, "", "", boolPtr(true), page, limit)
	if err != nil {
		return nil, fmt.Errorf("searching visa routes: %w", err)
	}

	// Textual destination filter (country name/code): AdminList only
	// filters by space ID, so narrow the page when a name/code was given.
	if dest, ok := filters["destination"].(string); ok && dest != "" {
		routes = filterVisaRoutesByDestination(routes, dest)
	}

	// AdminList intentionally returns the stored bag (§80 admin surfaces),
	// so the resolver does its own batched cfv read for the page.
	var cfvByRoute map[pgtype.UUID]map[string]any

	if r.queries != nil && len(routes) > 0 {
		ids := make([]pgtype.UUID, 0, len(routes))
		for _, route := range routes {
			if id, perr := ParseUUID(route.ID); perr == nil {
				ids = append(ids, id)
			}
		}

		cfvByRoute = customfields.EntityFieldValuesForEntities(ctx, r.queries, "visa_route", ids)
	}

	out := make([]ResolvedItem, 0, len(routes))
	for _, route := range routes {
		var cfv map[string]any

		if id, perr := ParseUUID(route.ID); perr == nil {
			cfv = cfvByRoute[id]
		}

		out = append(out, routeToResolvedItem(route, cfv))
	}

	return out, nil
}

func routeToResolvedItem(route visaroutes.RouteDTO, cfv map[string]any) ResolvedItem {
	desc := route.EligibilitySummary
	if route.DestinationSpaceName != "" {
		if desc != "" {
			desc += " · "
		}

		desc += route.DestinationSpaceName
	}

	// Merged field bag (requirements_json || cfv) — computed once, shared by
	// score_inputs and field_values so both see community values (§80).
	req := visaRequirementsMap(route.Requirements)

	if len(cfv) > 0 {
		if req == nil {
			req = make(map[string]any, len(cfv))
		}

		for k, v := range cfv {
			req[k] = v
		}
	}

	meta := map[string]any{
		"slug":            route.Slug,
		"visa_category":   route.VisaCategory,
		"destination":     route.DestinationSpaceName,
		"destination_geo": route.DestinationSpaceCode,
		"score_inputs":    visaRouteScoreInputs(route, req),
	}

	if route.DestinationSpaceID != "" {
		if id, err := ParseUUID(route.DestinationSpaceID); err == nil && id.Valid {
			meta["geo_space_id"] = id.String()
		}
	}

	if route.DestinationSpaceCode != "" {
		meta["country"] = route.DestinationSpaceCode
	}

	if route.VisaCategory != "" {
		meta["tags"] = []string{route.VisaCategory}
	}

	// §30 Sub-spec 5 + §80 Tool Surface Hydration: visa_routes has no
	// field_values column; its requirements_json object is the entity's field
	// bag, so the merged bag is exposed for matrix hydration.
	if len(req) > 0 {
		meta["field_values"] = req
	}

	return ResolvedItem{
		Type:        "visa_route",
		ID:          route.ID,
		Title:       route.Name,
		Description: Excerpt(desc, 160),
		Metadata:    meta,
	}
}

// visaRouteScoreInputs builds the item's score inputs from the route's typed
// columns plus the merged requirements bag (requirements_json || cfv — cfv
// wins per key, so approved community values feed scoring the same as
// ingested values; §80).
func visaRouteScoreInputs(route visaroutes.RouteDTO, req map[string]any) map[string]any {
	var cost any

	if route.TypicalFeeUSD != nil {
		if n, ok := coerceNumber(route.TypicalFeeUSD); ok {
			cost = n
		}
	}

	// Community-maintained fee fields fill the gap when the typed column is
	// absent (the typed column stays authoritative when set — §80).
	if cost == nil {
		cost = visaNumber(req, "typical_fee_usd", "fee_usd", "fee")
	}

	income := visaNumber(req, "income", "minimum_income", "min_income", "annual_income")
	speed := visaNumber(req, "speed", "processing_months", "processing_time", "duration_months")
	minimumFunds := visaNumber(req, "minimum_funds", "funds", "min_funds", "proof_of_funds")
	ageMin := visaNumber(req, "age_min", "minimum_age")
	ageMax := visaNumber(req, "age_max", "maximum_age")

	occupations := visaStrings(req, "occupation_eligible", "occupations", "eligible_occupations")
	if occupations == nil {
		occupations = []string{}
	}

	workAllowed := visaBool(req, "work_allowed", "right_to_work", "work_permit")
	familyIncluded := visaBool(req, "family_included", "dependents", "family")
	prEligible := visaBool(req, "pr_eligible", "path_to_pr")

	prPathEase, prFromPath := prPathEaseAndEligibility(route.PathToPR)
	if prFromPath != nil {
		if prEligible == nil {
			prEligible = prFromPath
		} else if prFromPathBool, ok := prFromPath.(bool); ok {
			// Path-to-PR string is a stronger signal than a missing key.
			prEligible = prFromPathBool
		}
	}

	// Fall back to the visa category for work permission when no explicit key
	// is present.
	if workAllowed == nil {
		workAllowed = workAllowedFromCategory(route.VisaCategory)
	}

	var destinationGeo any

	if route.DestinationSpaceID != "" {
		if id, err := ParseUUID(route.DestinationSpaceID); err == nil && id.Valid {
			destinationGeo = id.String()
		}
	}

	language := languagesForGeoCode(route.DestinationSpaceCode)

	return map[string]any{
		"cost":                     cost,
		"income":                   income,
		"speed":                    speed,
		"pr_path_ease":             prPathEase,
		"minimum_funds":            minimumFunds,
		"work_allowed":             workAllowed,
		"pr_eligible":              prEligible,
		"family_included":          familyIncluded,
		"language":                 language,
		"age_min":                  ageMin,
		"age_max":                  ageMax,
		"occupation_eligible":      occupations,
		"destination_geo_space_id": destinationGeo,
		"geo_space_id":             destinationGeo,
	}
}

func boolPtr(b bool) *bool { return &b }

// visaRequirementsMap normalises a visa-route requirements value (already parsed
// JSON, a JSON string, or raw bytes) into a map for key-based extraction. A
// non-map payload is preserved under "_raw" rather than dropped.
func visaRequirementsMap(req any) map[string]any {
	if req == nil {
		return nil
	}

	if m, ok := req.(map[string]any); ok {
		return m
	}

	if s, ok := req.(string); ok && s != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err == nil {
			return m
		}
	}

	if b, ok := req.([]byte); ok && len(b) > 0 {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err == nil {
			return m
		}
	}

	return map[string]any{"_raw": req}
}

// visaNumber extracts a numeric score input from the requirements map.
func visaNumber(req map[string]any, keys ...string) any {
	if req == nil {
		return nil
	}

	for _, k := range keys {
		if v, ok := req[k]; ok && v != nil {
			if n, ok := coerceNumber(v); ok {
				return n
			}
		}
	}

	return nil
}

// visaBool extracts a boolean score input from the requirements map.
func visaBool(req map[string]any, keys ...string) any {
	if req == nil {
		return nil
	}

	for _, k := range keys {
		if v, ok := req[k]; ok && v != nil {
			if b, ok := coerceBool(v); ok {
				return b
			}
		}
	}

	return nil
}

// visaStrings extracts a string slice score input from the requirements map.
func visaStrings(req map[string]any, keys ...string) []string {
	if req == nil {
		return nil
	}

	for _, k := range keys {
		if v, ok := req[k]; ok && v != nil {
			if s, ok := stringSlice(v); ok {
				return s
			}
		}
	}

	return nil
}

// prPathEaseAndEligibility maps the free-text "path to PR" field into a numeric
// ease score (0-100) and a boolean eligibility flag.
func prPathEaseAndEligibility(s string) (any, any) {
	if s == "" {
		return nil, nil
	}

	low := strings.ToLower(strings.TrimSpace(s))

	// Numeric path ease is used as-is when present.
	if f, err := strconv.ParseFloat(low, 64); err == nil {
		eligible := f > 0
		return f, eligible
	}

	positive := []string{"yes", "available", "true", "clear", "direct", "straightforward", "easy"}
	negative := []string{"no", "none", "false", "not", "not available", "no path", "none available"}

	for _, p := range positive {
		if strings.Contains(low, p) {
			return float64(100), true
		}
	}

	for _, n := range negative {
		if strings.Contains(low, n) {
			return float64(0), false
		}
	}

	return nil, nil
}

// workAllowedFromCategory returns a sensible boolean for work permission when
// the explicit key is missing, based on the visa category slug.
func workAllowedFromCategory(category string) any {
	low := strings.ToLower(strings.TrimSpace(category))
	switch low {
	case "work", "skilled_worker", "employment", "work_permit", "temporary_work", "intra_company_transfer":
		return true
	}

	return nil
}

// filterVisaRoutesByDestination narrows routes by destination name or
// geo code (case-insensitive substring). Used when the caller passed a
// textual destination instead of a space UUID.
func filterVisaRoutesByDestination(routes []visaroutes.RouteDTO, dest string) []visaroutes.RouteDTO {
	needle := strings.ToLower(strings.TrimSpace(dest))
	if needle == "" {
		return routes
	}
	kept := routes[:0]
	for _, route := range routes {
		if strings.Contains(strings.ToLower(route.DestinationSpaceName), needle) ||
			strings.Contains(strings.ToLower(route.DestinationSpaceCode), needle) {
			kept = append(kept, route)
		}
	}
	return kept
}
