// Package costofliving — service.go holds the business logic for the Cost of
// Living Calculator. It reads col_calculator.* system_settings via the
// hot-reloadable config.Settings and computes the comparison matrix with
// ratios vs the baseline city and a weighted Overall Cost Index.
//
// Ref: japaconnect-engineering/sections/72-cost-of-living.md
//
//	japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md
package costofliving

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Default system_settings values (Section 72). These are used when a setting
// is unset or unparseable — see config.Settings.GetBoolOr/GetIntOr.
const (
	defaultEnabled   = true
	defaultStaleDays = 180
	defaultMaxCities = 3
)

// ErrCalculatorDisabled is returned by service methods when the
// col_calculator.enabled system_settings toggle is false. Handlers translate
// it into a 404 `col_calculator_disabled` response per ADR-042 (feature-flag
// middleware returns 404, not 403/500, when a feature is disabled).
var ErrCalculatorDisabled = fmt.Errorf("costofliving: calculator disabled")

// CityData is a single city row in the comparison matrix. Values maps
// dimension key (dimension_id string) → raw value as stored (text).
type CityData struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	GeoSpaceName string            `json:"geo_space_name"`
	IsBaseline   bool              `json:"is_baseline"`
	LastUpdated  string            `json:"last_updated"`
	Values       map[string]string `json:"values"`
}

// CategoryData is a single cost category (dimension) across all cities.
// Values maps city ID → raw numeric value; Ratios maps city ID → ratio vs
// the baseline city's value for this category.
type CategoryData struct {
	Key    string             `json:"key"`
	Label  string             `json:"label"`
	Values map[string]float64 `json:"values"`
	Ratios map[string]float64 `json:"ratios"`
}

// ComparisonMatrix is the full comparison result returned by CompareCities.
// OverallIndex maps city ID → weighted Overall Cost Index (1.0 = baseline).
type ComparisonMatrix struct {
	Cities         []CityData         `json:"cities"`
	Categories     []CategoryData     `json:"categories"`
	BaselineCityID string             `json:"baseline_city_id"`
	OverallIndex   map[string]float64 `json:"overall_index"`
}

// CityListItem is a single city in the paginated list response.
type CityListItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	GeoSpaceName string `json:"geo_space_name"`
	GeoSpaceSlug string `json:"geo_space_slug"`
	IsBaseline   bool   `json:"is_baseline"`
	LastUpdated  string `json:"last_updated"`
	Position     int32  `json:"position"`
}

// CitiesListResponse is the paginated list response for ListCities.
type CitiesListResponse struct {
	Cities []CityListItem `json:"cities"`
	Total  int64          `json:"total"`
	Page   int            `json:"page"`
	Limit  int            `json:"limit"`
}

// AdminStatsItem is a single city row in the admin stats report.
type AdminStatsItem struct {
	CityID           string  `json:"city_id"`
	CityName         string  `json:"city_name"`
	IsBaseline       bool    `json:"is_baseline"`
	LastUpdated      string  `json:"last_updated"`
	IsStale          bool    `json:"is_stale"`
	TotalDimensions  int64   `json:"total_dimensions"`
	FilledDimensions int64   `json:"filled_dimensions"`
	CompletenessPct  float64 `json:"completeness_pct"`
}

// AdminStatsResponse is the paginated admin stats report.
type AdminStatsResponse struct {
	Cities []AdminStatsItem `json:"cities"`
	Total  int64            `json:"total"`
	Page   int              `json:"page"`
	Limit  int              `json:"limit"`
}

// Service holds the business logic for the cost-of-living calculator.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a cost-of-living service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// IsStale reports whether the given last_updated_at timestamp is older than
// the col_calculator.stale_days threshold. Section 72: default 180 days.
func (s *Service) IsStale(ctx context.Context, tenantID pgtype.UUID, lastUpdatedAt time.Time) bool {
	staleDays := defaultStaleDays
	if s.settings != nil {
		staleDays = s.settings.GetIntOr(ctx, tenantID, "col_calculator.stale_days", defaultStaleDays)
	}
	cutoff := time.Now().AddDate(0, 0, -staleDays)
	return lastUpdatedAt.Before(cutoff)
}

// IsStaleAt reports whether the given timestamp is older than staleDays.
// This is the pure variant used by tests that do not need config resolution.
func IsStaleAt(lastUpdatedAt time.Time, now time.Time, staleDays int) bool {
	cutoff := now.AddDate(0, 0, -staleDays)
	return lastUpdatedAt.Before(cutoff)
}

// maxCities returns the configured col_calculator.max_cities limit.
func (s *Service) maxCities(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return defaultMaxCities
	}
	return s.settings.GetIntOr(ctx, tenantID, "col_calculator.max_cities", defaultMaxCities)
}

// ListCities returns a paginated list of cities with cost_of_living data.
// Public route (col_calculator.use — all users including anonymous).
func (s *Service) ListCities(ctx context.Context, tenantID pgtype.UUID, search string, page, limit int) (CitiesListResponse, error) {
	if s.settings != nil {
		if enabled := s.settings.GetBoolOr(ctx, tenantID, "col_calculator.enabled", defaultEnabled); !enabled {
			return CitiesListResponse{}, ErrCalculatorDisabled
		}
	}

	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	typeDef, err := s.store.GetCostOfLivingType(ctx, tenantID)
	if err != nil {
		return CitiesListResponse{}, fmt.Errorf("costofliving: list cities: %w", err)
	}

	rows, err := s.store.ListCities(ctx, tenantID, typeDef.ID, search, offset, int32(limit))
	if err != nil {
		return CitiesListResponse{}, err
	}

	total, err := s.store.CountCities(ctx, tenantID, typeDef.ID, search)
	if err != nil {
		return CitiesListResponse{}, err
	}

	cities := make([]CityListItem, 0, len(rows))
	for _, r := range rows {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}
		geoName := ""
		if r.GeoSpaceName != nil {
			geoName = *r.GeoSpaceName
		}
		geoSlug := ""
		if r.GeoSpaceSlug != nil {
			geoSlug = *r.GeoSpaceSlug
		}
		lastUpdated := ""
		if r.LastUpdatedAt.Valid {
			lastUpdated = r.LastUpdatedAt.Time.UTC().Format(time.RFC3339)
		}
		cities = append(cities, CityListItem{
			ID:           r.ID.String(),
			Title:        r.Title,
			Description:  desc,
			GeoSpaceName: geoName,
			GeoSpaceSlug: geoSlug,
			IsBaseline:   r.IsBaseline,
			LastUpdated:  lastUpdated,
			Position:     r.Position,
		})
	}

	return CitiesListResponse{
		Cities: cities,
		Total:  total,
		Page:   page,
		Limit:  limit,
	}, nil
}

// CompareCities returns a comparison matrix with raw values + ratios vs the
// baseline city and a weighted Overall Cost Index. Steps per ADR-072 §2:
//  1. Get the cost_of_living type definition (for config weights).
//  2. Get the baseline city.
//  3. Get values for all requested cities + the baseline.
//  4. Pivot values into a matrix.
//  5. Compute ratios vs baseline for each category.
//  6. Compute weighted Overall Cost Index using config weights.
//
// The baseline city is always included in the result even if not in cityIDs.
func (s *Service) CompareCities(ctx context.Context, tenantID pgtype.UUID, cityIDs []pgtype.UUID) (ComparisonMatrix, error) {
	if s.settings != nil {
		if enabled := s.settings.GetBoolOr(ctx, tenantID, "col_calculator.enabled", defaultEnabled); !enabled {
			return ComparisonMatrix{}, ErrCalculatorDisabled
		}
	}

	maxCities := s.maxCities(ctx, tenantID)

	// Deduplicate city IDs before the max_cities check so that duplicate
	// IDs in the query string don't trigger a false rejection or produce
	// duplicate entries in the result.
	seen := make(map[pgtype.UUID]bool, len(cityIDs))
	uniqueIDs := make([]pgtype.UUID, 0, len(cityIDs))
	for _, id := range cityIDs {
		if id.Valid && !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	if len(uniqueIDs) > maxCities {
		return ComparisonMatrix{}, fmt.Errorf("costofliving: too many cities: %d > max %d", len(uniqueIDs), maxCities)
	}
	cityIDs = uniqueIDs

	// Step 1: get the cost_of_living type definition (config holds weights).
	typeDef, err := s.store.GetCostOfLivingType(ctx, tenantID)
	if err != nil {
		return ComparisonMatrix{}, fmt.Errorf("costofliving: compare: get type: %w", err)
	}
	weights := parseWeights(typeDef.Config)

	// Step 2: get the baseline city.
	baseline, err := s.store.GetBaselineCity(ctx, tenantID, typeDef.ID)
	if err != nil {
		return ComparisonMatrix{}, fmt.Errorf("costofliving: compare: get baseline: %w", err)
	}
	baselineID := baseline.ID

	// Step 3: collect the set of city IDs to fetch values for (requested +
	// baseline, deduplicated). The baseline is always included.
	wantSet := make(map[pgtype.UUID]bool, len(cityIDs)+1)
	wantSet[baselineID] = true
	for _, id := range cityIDs {
		if id.Valid {
			wantSet[id] = true
		}
	}
	allIDs := make([]pgtype.UUID, 0, len(wantSet))
	for id := range wantSet {
		allIDs = append(allIDs, id)
	}

	values, err := s.store.GetValuesForCities(ctx, tenantID, allIDs)
	if err != nil {
		return ComparisonMatrix{}, fmt.Errorf("costofliving: compare: get values: %w", err)
	}

	// Step 4: pivot values into a matrix.
	// dimValues[dimensionID][cityID] = float64 value (parsed from text).
	// dimMeta[dimensionID] = {label, key, slug}.
	type dimMeta struct {
		Label string
		Key   string
		Slug  string
	}
	dimValues := make(map[string]map[string]float64)
	dimOrder := make([]string, 0)
	dimMetaMap := make(map[string]dimMeta)
	cityRawValues := make(map[string]map[string]string)

	for _, v := range values {
		dimKey := v.DimensionID.String()
		cityKey := v.ItemID.String()

		if _, ok := dimMetaMap[dimKey]; !ok {
			slug := ""
			if v.DimensionSlug != nil {
				slug = *v.DimensionSlug
			}
			dimMetaMap[dimKey] = dimMeta{Label: v.DimensionLabel, Key: dimKey, Slug: slug}
			dimOrder = append(dimOrder, dimKey)
		}
		if dimValues[dimKey] == nil {
			dimValues[dimKey] = make(map[string]float64)
		}
		if cityRawValues[cityKey] == nil {
			cityRawValues[cityKey] = make(map[string]string)
		}

		rawStr := ""
		if v.Value != nil {
			rawStr = *v.Value
		}
		cityRawValues[cityKey][dimKey] = rawStr

		if f, ok := parseFloatValue(v.Value, v.ValueRating); ok {
			dimValues[dimKey][cityKey] = f
		}
	}

	// Build the Cities slice (baseline first, then requested in order).
	cityOrder := make([]pgtype.UUID, 0, len(allIDs))
	cityOrder = append(cityOrder, baselineID)
	for _, id := range cityIDs {
		if id.Valid && id != baselineID {
			cityOrder = append(cityOrder, id)
		}
	}

	// Batch-fetch all non-baseline cities in one query (avoids N+1).
	nonBaselineIDs := make([]pgtype.UUID, 0, len(cityOrder))
	for _, id := range cityOrder {
		if id != baselineID {
			nonBaselineIDs = append(nonBaselineIDs, id)
		}
	}
	cityRows := make(map[pgtype.UUID]gen.GetCOLCitiesByIDsRow, len(nonBaselineIDs))
	if len(nonBaselineIDs) > 0 {
		rows, err := s.store.GetCitiesByIDs(ctx, tenantID, nonBaselineIDs)
		if err != nil {
			slog.Warn("costofliving: compare: batch get cities", "error", err)
		} else {
			for _, r := range rows {
				cityRows[r.ID] = r
			}
		}
	}

	cities := make([]CityData, 0, len(cityOrder))
	for _, id := range cityOrder {
		var title, geoName string
		var isBaseline bool
		var lastUpdated time.Time
		if id == baselineID {
			title = baseline.Title
			if baseline.GeoSpaceName != nil {
				geoName = *baseline.GeoSpaceName
			}
			isBaseline = true
			if baseline.LastUpdatedAt.Valid {
				lastUpdated = baseline.LastUpdatedAt.Time
			}
		} else {
			c, ok := cityRows[id]
			if !ok {
				slog.Warn("costofliving: compare: city not found", "id", id.String())
				continue
			}
			title = c.Title
			if c.GeoSpaceName != nil {
				geoName = *c.GeoSpaceName
			}
			isBaseline = c.IsBaseline
			if c.LastUpdatedAt.Valid {
				lastUpdated = c.LastUpdatedAt.Time
			}
		}
		lastUpdatedStr := ""
		if !lastUpdated.IsZero() {
			lastUpdatedStr = lastUpdated.UTC().Format(time.RFC3339)
		}
		// Ensure Values is never nil so JSON serializes as {} not null.
		rawVals := cityRawValues[id.String()]
		if rawVals == nil {
			rawVals = map[string]string{}
		}
		cities = append(cities, CityData{
			ID:           id.String(),
			Title:        title,
			GeoSpaceName: geoName,
			IsBaseline:   isBaseline,
			LastUpdated:  lastUpdatedStr,
			Values:       rawVals,
		})
	}

	// Step 5: compute ratios vs baseline for each category.
	baselineKey := baselineID.String()
	categories := make([]CategoryData, 0, len(dimOrder))
	overall := make(map[string]float64)

	for _, dimKey := range dimOrder {
		meta := dimMetaMap[dimKey]
		cat := CategoryData{
			Key:    meta.Key,
			Label:  meta.Label,
			Values: make(map[string]float64),
			Ratios: make(map[string]float64),
		}
		baseVal, hasBase := dimValues[dimKey][baselineKey]
		for _, id := range cityOrder {
			cityKey := id.String()
			val, ok := dimValues[dimKey][cityKey]
			if ok {
				cat.Values[cityKey] = val
			}
			if hasBase && baseVal > 0 && ok {
				cat.Ratios[cityKey] = val / baseVal
			} else if hasBase && baseVal == 0 && ok {
				// Baseline is zero — ratio undefined; report 0 to avoid NaN.
				cat.Ratios[cityKey] = 0
			}
		}
		categories = append(categories, cat)

		// Step 6: accumulate weighted Overall Cost Index.
		// Weight is looked up by dimension slug in the config weights
		// (config JSONB weights are keyed by slug, e.g. "rent_1br_city_centre").
		weight, hasWeight := weights[meta.Slug]
		if !hasWeight || weight <= 0 {
			continue
		}
		for cityKey, ratio := range cat.Ratios {
			overall[cityKey] += ratio * weight
		}
	}

	return ComparisonMatrix{
		Cities:         cities,
		Categories:     categories,
		BaselineCityID: baselineKey,
		OverallIndex:   overall,
	}, nil
}

// AdminStats returns the data completeness + staleness report for admin.
// Gated by col_calculator.manage.
func (s *Service) AdminStats(ctx context.Context, tenantID pgtype.UUID, page, limit int) (AdminStatsResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	typeDef, err := s.store.GetCostOfLivingType(ctx, tenantID)
	if err != nil {
		return AdminStatsResponse{}, fmt.Errorf("costofliving: admin stats: %w", err)
	}

	rows, err := s.store.AdminStats(ctx, tenantID, typeDef.ID, offset, int32(limit))
	if err != nil {
		return AdminStatsResponse{}, err
	}

	items := make([]AdminStatsItem, 0, len(rows))
	for _, r := range rows {
		lastUpdated := ""
		var lastUpdatedT time.Time
		if r.LastUpdatedAt.Valid {
			lastUpdatedT = r.LastUpdatedAt.Time
			lastUpdated = lastUpdatedT.UTC().Format(time.RFC3339)
		}
		stale := false
		if !lastUpdatedT.IsZero() {
			stale = s.IsStale(ctx, tenantID, lastUpdatedT)
		}
		pct := 0.0
		if r.TotalDimensions > 0 {
			pct = float64(r.FilledDimensions) / float64(r.TotalDimensions) * 100
		}
		items = append(items, AdminStatsItem{
			CityID:           r.CityID.String(),
			CityName:         r.CityName,
			IsBaseline:       r.IsBaseline,
			LastUpdated:      lastUpdated,
			IsStale:          stale,
			TotalDimensions:  r.TotalDimensions,
			FilledDimensions: r.FilledDimensions,
			CompletenessPct:  pct,
		})
	}

	// Total count of cities for pagination metadata.
	total, err := s.store.CountCities(ctx, tenantID, typeDef.ID, "")
	if err != nil {
		return AdminStatsResponse{}, err
	}

	return AdminStatsResponse{
		Cities: items,
		Total:  total,
		Page:   page,
		Limit:  limit,
	}, nil
}

// parseWeights extracts the per-category weights from the comparison type
// definition's config JSONB. The config shape is:
//
//	{"weights": {"<dimension_slug>": 0.25, ...}}
//
// Weight keys are dimension slugs (e.g. "rent_1br_city_centre"), matching
// the slug column on comparison_dimensions. Returns an empty map if config
// is nil or malformed.
func parseWeights(config []byte) map[string]float64 {
	if len(config) == 0 {
		return map[string]float64{}
	}
	var wrapper struct {
		Weights map[string]float64 `json:"weights"`
	}
	if err := json.Unmarshal(config, &wrapper); err != nil {
		slog.Warn("costofliving: parse weights: invalid config json", "error", err)
		return map[string]float64{}
	}
	if wrapper.Weights == nil {
		return map[string]float64{}
	}
	return wrapper.Weights
}

// parseFloatValue parses a numeric cost value from the comparison_values row.
// Cost data is stored as text (value field) per Section 72; value_rating is
// the numeric variant. Returns the float64 and true if a number was found.
func parseFloatValue(textVal *string, numericVal pgtype.Numeric) (float64, bool) {
	if textVal != nil && *textVal != "" {
		s := strings.TrimSpace(*textVal)
		// Strip common currency prefixes/suffixes before parsing.
		s = strings.TrimPrefix(s, "$")
		s = strings.TrimPrefix(s, "USD")
		s = strings.TrimSpace(s)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, true
		}
	}
	// Fall back to the numeric column if the text parse failed.
	if numericVal.Valid {
		if f, err := numericVal.Float64Value(); err == nil {
			return f.Float64, true
		}
	}
	return 0, false
}
