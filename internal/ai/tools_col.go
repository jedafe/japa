// Package ai — tools_col.go implements col.city.get, the §72 cost-of-living
// read surface for the agent. It returns a city's canonical record the same
// way the calculator renders it: the comparison_items row (intel_cost_data),
// the dimension-value matrix (comparison_values joined to
// comparison_dimensions, with the bound source custom_field key), and the
// §80 community overlay (custom_field_values on the geo space — cfv wins
// per key over intel_cost_data, matching resolver/city.go).
//
// Read-only platform data — no privacy target is registered (the data is
// the same public dataset the COL calculator exposes). No catalog
// permission gate beyond agent.use either: required_permission is NULL in
// the ai_tools row (migration 000843).
//
// Ref: japaconnect-engineering/sections/72-cost-of-living.md
// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Tool Surface Hydration)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// col.city.get
// ---------------------------------------------------------------------------

type colCityGetArgs struct {
	CityID     string `json:"city_id,omitempty"`
	GeoSpaceID string `json:"geo_space_id,omitempty"`
}

// colCity is the unified city shape — GetCOLCityByIDRow and
// GetCOLByGeoSpaceIDRow carry identical fields under different generated
// types, so the handler normalizes before responding.
type colCity struct {
	ID            pgtype.UUID
	ContentID     pgtype.UUID
	Title         string
	GeoSpaceID    pgtype.UUID
	GeoSpaceName  *string
	GeoSpaceSlug  *string
	IsBaseline    bool
	LastUpdatedAt pgtype.Timestamptz
	IntelCostData []byte
}

// COLCityGetHandler returns a ToolHandler that reads a single city's
// cost-of-living record by comparison-item ID or geo-space ID.
func COLCityGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.COL == nil {
			return errorToolResult("col_unavailable", "cost-of-living reader is not configured"), nil
		}

		var a colCityGetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		var city colCity

		switch {
		case a.CityID != "":
			id, err := scanUUID(a.CityID)
			if err != nil {
				return errorToolResult("invalid_city_id", "city_id is not a valid UUID"), nil
			}

			row, err := deps.COL.GetCityByID(ctx, id)
			if err != nil {
				return errorToolResult("city_not_found", "cost-of-living city not found"), nil
			}

			city = colCity{
				ID: row.ID, ContentID: row.ContentID, Title: row.Title,
				GeoSpaceID: row.GeoSpaceID, GeoSpaceName: row.GeoSpaceName, GeoSpaceSlug: row.GeoSpaceSlug,
				IsBaseline: row.IsBaseline, LastUpdatedAt: row.LastUpdatedAt, IntelCostData: row.IntelCostData,
			}
		case a.GeoSpaceID != "":
			id, err := scanUUID(a.GeoSpaceID)
			if err != nil {
				return errorToolResult("invalid_geo_space_id", "geo_space_id is not a valid UUID"), nil
			}

			row, err := deps.COL.GetCityByGeoSpaceID(ctx, id)
			if err != nil {
				return errorToolResult("city_not_found", "no cost-of-living city for that geo space"), nil
			}

			city = colCity{
				ID: row.ID, ContentID: row.ContentID, Title: row.Title,
				GeoSpaceID: row.GeoSpaceID, GeoSpaceName: row.GeoSpaceName, GeoSpaceSlug: row.GeoSpaceSlug,
				IsBaseline: row.IsBaseline, LastUpdatedAt: row.LastUpdatedAt, IntelCostData: row.IntelCostData,
			}
		default:
			return errorToolResult("missing_city_ref", "city_id or geo_space_id is required"), nil
		}

		// Dimension-value matrix (comparison_values → dimensions + the bound
		// source custom_field key each wiki community value is keyed under).
		valueRows, err := deps.COL.GetValuesForCities(ctx, []pgtype.UUID{city.ID})
		if err != nil {
			return errorToolResult("query_failed", fmt.Sprintf("city values failed: %v", err)), nil
		}

		// §80 community overlay — cfv wins per key over intel_cost_data.
		cfv := map[string]any{}
		if city.GeoSpaceID.Valid {
			cfv = deps.COL.ListSpaceFieldValues(ctx, []pgtype.UUID{city.GeoSpaceID})[city.GeoSpaceID]
		}

		costData := map[string]any{}
		if len(city.IntelCostData) > 0 {
			_ = json.Unmarshal(city.IntelCostData, &costData)
		}

		for k, v := range cfv {
			costData[k] = v
		}

		dimensions := make([]map[string]any, 0, len(valueRows))

		for _, v := range valueRows {
			var rating any

			if v.ValueRating.Valid {
				f, _ := v.ValueRating.Float64Value()
				rating = f.Float64
			}

			dimensions = append(dimensions, map[string]any{
				"dimension_id":     uuidString(v.DimensionID),
				"dimension_label":  v.DimensionLabel,
				"dimension_slug":   v.DimensionSlug,
				"dimension_type":   v.DimensionType,
				"source_field_key": v.SourceFieldKey,
				"value":            v.Value,
				"value_rating":     rating,
				"value_boolean":    v.ValueBoolean,
			})
		}

		return successToolResult(map[string]any{
			"city_id":          uuidString(city.ID),
			"content_id":       uuidString(city.ContentID),
			"title":            city.Title,
			"geo_space_id":     uuidString(city.GeoSpaceID),
			"geo_space_name":   city.GeoSpaceName,
			"geo_space_slug":   city.GeoSpaceSlug,
			"is_baseline":      city.IsBaseline,
			"last_updated_at":  tsOrNil(city.LastUpdatedAt),
			"cost_data":        costData,
			"community_fields": len(cfv) > 0,
			"dimensions":       dimensions,
		}), nil
	}
}
