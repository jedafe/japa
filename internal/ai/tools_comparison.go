// Package ai — tools_comparison.go implements the comparison tool handlers
// for the AI Agent Executor (Section 103 — Comparison Engine, extends §30).
//
// Tools implemented here (Phase 7, tasks 7.06):
//
//   - comparison.types.list   — list comparison type definitions
//   - comparison.create       — create a comparison content + items + dimensions
//   - comparison.fill_values  — fill comparison_values with provenance metadata
//   - comparison.add_dimension — add a dimension to an existing comparison
//   - comparison.read         — read a comparison with its full value matrix
//
// Privacy: comparison data is content-backed. The executor's privacy gate
// (PrivacyTargetFunc) is registered for comparison.read and
// comparison.fill_values so a call targeting an ai_exempt content row is
// blocked. comparison.types.list, comparison.create, and
// comparison.add_dimension do not target a single existing content row, so
// no privacy target is registered (the create/add tools write new rows).
//
// Provenance: comparison.fill_values attaches a ComparisonProvenance record
// to each written value's metadata JSONB. The source is provided by the LLM
// (custom_field|rag|external|llm_knowledge); the engine records it together
// with the source_ref, confidence, and retrieved_at timestamp.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine, Tool Catalog)
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7, task 7.06)
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// comparison.types.list (task 7.06a)
// ---------------------------------------------------------------------------

// comparisonTypesListArgs is the LLM-produced argument shape for
// comparison.types.list.
type comparisonTypesListArgs struct {
	Limit  int32 `json:"limit,omitempty"`
	Offset int32 `json:"offset,omitempty"`
}

// ComparisonTypesListHandler returns a ToolHandler that lists comparison
// type definitions for the tenant. The LLM uses this to discover existing
// types (Tier 1 + Tier 2 classification) before deciding whether to create
// a new type (Tier 3).
func ComparisonTypesListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comparison == nil {
			return errorToolResult("comparison_unavailable", "Comparison service is not configured"), nil
		}

		var a comparisonTypesListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Limit <= 0 || a.Limit > 100 {
			a.Limit = 50
		}

		types, err := deps.Comparison.ListComparisonTypes(ctx, tenantID, a.Limit, a.Offset)
		if err != nil {
			return errorToolResult("list_failed", fmt.Sprintf("listing comparison types: %v", err)), nil
		}

		out := make([]map[string]any, 0, len(types))
		for _, t := range types {
			entry := map[string]any{
				"id":         uuidString(t.ID),
				"name":       t.Name,
				"slug":       t.Slug,
				"item_label": t.ItemLabel,
				"is_system":  t.IsSystem,
			}
			if t.Description != nil {
				entry["description"] = *t.Description
			}
			if t.FieldGroupID.Valid {
				entry["field_group_id"] = t.FieldGroupID.String()
			}
			out = append(out, entry)
		}

		return successToolResult(map[string]any{
			"types":  out,
			"total":  len(out),
			"limit":  a.Limit,
			"offset": a.Offset,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// comparison.create (task 7.06b)
// ---------------------------------------------------------------------------

// comparisonCreateArgs is the LLM-produced argument shape for
// comparison.create.
type comparisonCreateArgs struct {
	Title      string                    `json:"title"`
	Body       string                    `json:"body,omitempty"`
	EntityType string                    `json:"entity_type,omitempty"` // slug of an existing type
	Visibility string                    `json:"visibility,omitempty"`
	SpaceID    string                    `json:"space_id,omitempty"`
	Depth      string                    `json:"depth,omitempty"` // 'quick' or 'detailed'
	Items      []comparisonCreateItemArg `json:"items"`
	Dimensions []string                  `json:"dimensions"`
	CreateType *comparisonCreateTypeArg  `json:"create_type,omitempty"`
}

type comparisonCreateItemArg struct {
	Name    string         `json:"name"`
	Slug    string         `json:"slug,omitempty"`
	ItemRef map[string]any `json:"item_ref,omitempty"`
}

type comparisonCreateTypeArg struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	ItemLabel   string `json:"item_label,omitempty"`
}

// ComparisonCreateHandler returns a ToolHandler that creates a new
// comparison content row with items + dimensions. When create_type is
// provided, a new comparison_type_definition is created first (Tier 3).
// When entity_type is provided, the comparison is linked to that existing
// type (Tier 2). The status is always forced to "draft" — the agent never
// publishes directly (§103 behavior rule).
func ComparisonCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comparison == nil {
			return errorToolResult("comparison_unavailable", "Comparison service is not configured"), nil
		}

		var a comparisonCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Title == "" {
			return errorToolResult("missing_title", "title is required"), nil
		}
		if len(a.Items) == 0 {
			return errorToolResult("missing_items", "at least one item is required"), nil
		}

		engine := NewComparisonEngine(deps)

		var spaceID pgtype.UUID
		if a.SpaceID != "" {
			var err error
			spaceID, err = scanUUID(a.SpaceID)
			if err != nil {
				return errorToolResult("invalid_space_id", "space_id is not a valid UUID"), nil
			}
		}

		// Resolve the type: either create a new one or look up an existing
		// one by slug.
		var typeID pgtype.UUID
		createdType := false

		if a.CreateType != nil {
			if a.CreateType.Name == "" || a.CreateType.Slug == "" {
				return errorToolResult("invalid_type", "create_type requires name and slug"), nil
			}
			td, err := deps.Comparison.CreateComparisonType(ctx, tenantID, ComparisonTypeCreateInput{
				Name:        a.CreateType.Name,
				Slug:        a.CreateType.Slug,
				Description: a.CreateType.Description,
				ItemLabel:   a.CreateType.ItemLabel,
			})
			if err != nil {
				return errorToolResult("type_create_failed", fmt.Sprintf("creating comparison type: %v", err)), nil
			}
			typeID = td.ID
			createdType = true
		} else if a.EntityType != "" {
			td, err := deps.Comparison.GetComparisonTypeBySlug(ctx, a.EntityType, tenantID)
			if err != nil {
				return errorToolResult("type_not_found", fmt.Sprintf("comparison type %q not found: %v", a.EntityType, err)), nil
			}
			typeID = td.ID
		}

		items := make([]ComparisonItemInput, 0, len(a.Items))
		for _, it := range a.Items {
			if it.Name == "" {
				continue
			}
			items = append(items, ComparisonItemInput{Name: it.Name, Slug: it.Slug, ItemRef: it.ItemRef})
		}

		res, err := engine.CreateComparison(ctx, tenantID, userID, CreateComparisonInput{
			CreateType: false, // type already created/resolved above
			TypeID:     typeID,
			Title:      a.Title,
			Body:       a.Body,
			Visibility: a.Visibility,
			SpaceID:    spaceID,
			Depth:      a.Depth,
			Items:      items,
			Dimensions: a.Dimensions,
		})
		if err != nil {
			return errorToolResult("create_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"content_id":    uuidString(res.ContentID),
			"type_id":       uuidString(res.TypeID),
			"title":         res.Title,
			"slug":          res.Slug,
			"created_type":  createdType,
			"depth":         res.Depth,
			"dimension_ids": uuidsToStrings(res.DimensionIDs),
			"item_ids":      uuidsToStrings(res.ItemIDs),
			"status":        "draft",
		}), nil
	}
}

// ---------------------------------------------------------------------------
// comparison.fill_values (task 7.06c)
// ---------------------------------------------------------------------------

// comparisonFillValuesArgs is the LLM-produced argument shape for
// comparison.fill_values.
type comparisonFillValuesArgs struct {
	ContentID string                   `json:"content_id"`
	Values    []comparisonFillValueArg `json:"values"`
}

type comparisonFillValueArg struct {
	DimensionID string   `json:"dimension_id"`
	ItemID      string   `json:"item_id"`
	Value       string   `json:"value,omitempty"`
	ValueRating *float64 `json:"value_rating,omitempty"`
	ValueBool   *bool    `json:"value_boolean,omitempty"`
	Source      string   `json:"source"`
	SourceRef   string   `json:"source_ref,omitempty"`
	Confidence  string   `json:"confidence,omitempty"`
}

// validProvenanceSources is the set of accepted provenance source values
// (Section 103 — Provenance).
var validProvenanceSources = map[string]bool{
	ProvenanceSourceCustomField: true,
	ProvenanceSourceRAG:         true,
	ProvenanceSourceExternal:    true,
	ProvenanceSourceLLM:         true,
}

// validConfidenceLevels is the set of accepted confidence values.
var validConfidenceLevels = map[string]bool{
	ConfidenceHigh:   true,
	ConfidenceMedium: true,
	ConfidenceLow:    true,
}

// ComparisonFillValuesHandler returns a ToolHandler that fills
// comparison_values with provenance metadata. Each value records its source
// (custom_field|rag|external|llm_knowledge), source_ref, confidence, and
// retrieved_at timestamp in the comparison_values.metadata JSONB column.
//
// Privacy: the executor's privacy gate blocks the call when the backing
// content row is ai_exempt (registered via ComparisonFillValuesPrivacyTarget).
func ComparisonFillValuesHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comparison == nil {
			return errorToolResult("comparison_unavailable", "Comparison service is not configured"), nil
		}

		var a comparisonFillValuesArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ContentID == "" {
			return errorToolResult("missing_content_id", "content_id is required"), nil
		}
		if len(a.Values) == 0 {
			return errorToolResult("missing_values", "at least one value is required"), nil
		}

		inputs := make([]FillValueInput, 0, len(a.Values))
		for _, v := range a.Values {
			dimID, err := scanUUID(v.DimensionID)
			if err != nil {
				return errorToolResult("invalid_dimension_id", "dimension_id must be a valid UUID"), nil
			}
			itemID, err := scanUUID(v.ItemID)
			if err != nil {
				return errorToolResult("invalid_item_id", "item_id must be a valid UUID"), nil
			}
			source := v.Source
			if source == "" {
				source = ProvenanceSourceLLM
			} else if !validProvenanceSources[source] {
				return errorToolResult("invalid_source",
					fmt.Sprintf("source must be one of: custom_field, rag, external, llm_knowledge (got %q)", source)), nil
			}
			confidence := v.Confidence
			if confidence == "" {
				confidence = ConfidenceMedium
			} else if !validConfidenceLevels[confidence] {
				return errorToolResult("invalid_confidence",
					fmt.Sprintf("confidence must be one of: high, medium, low (got %q)", confidence)), nil
			}
			inputs = append(inputs, FillValueInput{
				DimensionID: dimID,
				ItemID:      itemID,
				Value:       v.Value,
				ValueRating: v.ValueRating,
				ValueBool:   v.ValueBool,
				Provenance:  newProvenance(source, v.SourceRef, confidence),
			})
		}

	engine := NewComparisonEngine(deps)
	contentID, err := scanUUID(a.ContentID)
	if err != nil {
		return errorToolResult("invalid_content_id", "content_id must be a valid UUID"), nil
	}

	result, err := engine.FillValues(ctx, tenantID, inputs)
	if err != nil {
		return errorToolResult("fill_failed", err.Error()), nil
	}

	deps.Comparison.DeleteJCEComparisonCaches(ctx, contentID, tenantID)

	return successToolResult(map[string]any{
			"filled_count": len(result.Filled),
			"error_count":  len(result.Errors),
			"filled":       result.Filled,
			"errors":       result.Errors,
		}), nil
	}
}

// ComparisonFillValuesPrivacyTarget extracts the content_id from
// comparison.fill_values arguments so the executor's privacy gate can block
// the call when the backing content row is ai_exempt.
func ComparisonFillValuesPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a comparisonFillValuesArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}
	if a.ContentID == "" {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}
	id, err := scanUUID(a.ContentID)
	if err != nil || !id.Valid {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}
	return PrivacyEntityContent, id, nil
}

// ---------------------------------------------------------------------------
// comparison.add_dimension (task 7.06d)
// ---------------------------------------------------------------------------

// comparisonAddDimensionArgs is the LLM-produced argument shape for
// comparison.add_dimension.
type comparisonAddDimensionArgs struct {
	ContentID  string                      `json:"content_id"`
	Dimensions []comparisonAddDimensionArg `json:"dimensions"`
}

type comparisonAddDimensionArg struct {
	Label         string `json:"label"`
	DimensionType string `json:"dimension_type,omitempty"`
}

// validDimensionTypes is the set of accepted comparison dimension types
// (Section 30 — Generic Comparison System).
var validDimensionTypes = map[string]bool{
	"text":     true,
	"rating":   true,
	"boolean":  true,
	"number":   true,
	"currency": true,
}

// ComparisonAddDimensionHandler returns a ToolHandler that adds one or more
// dimensions to an existing comparison content row (Tier 2). The caller
// must be authorized — the executor's RBAC gate checks the tool's
// required_permission before dispatch.
func ComparisonAddDimensionHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comparison == nil {
			return errorToolResult("comparison_unavailable", "Comparison service is not configured"), nil
		}

		var a comparisonAddDimensionArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ContentID == "" {
			return errorToolResult("missing_content_id", "content_id is required"), nil
		}
		if len(a.Dimensions) == 0 {
			return errorToolResult("missing_dimensions", "at least one dimension is required"), nil
		}

		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id must be a valid UUID"), nil
		}

		inputs := make([]AddDimensionInput, 0, len(a.Dimensions))
		for _, d := range a.Dimensions {
			if d.Label == "" {
				continue
			}
			dimType := d.DimensionType
			if dimType == "" {
				dimType = "text"
			} else if !validDimensionTypes[dimType] {
				return errorToolResult("invalid_dimension_type",
					fmt.Sprintf("dimension_type must be one of: text, rating, boolean, number, currency (got %q)", dimType)), nil
			}
			inputs = append(inputs, AddDimensionInput{
				ContentID:     contentID,
				Label:         d.Label,
				DimensionType: dimType,
			})
		}
		if len(inputs) == 0 {
			return errorToolResult("missing_dimensions", "at least one dimension with a label is required"), nil
		}

		engine := NewComparisonEngine(deps)
		created, err := engine.AddDimensions(ctx, tenantID, inputs)
		if err != nil {
			return errorToolResult("add_dimension_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"content_id":    uuidString(contentID),
			"created_count": len(created),
			"dimensions":    created,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// comparison.read (task 7.06e)
// ---------------------------------------------------------------------------

// comparisonReadArgs is the LLM-produced argument shape for
// comparison.read.
type comparisonReadArgs struct {
	ContentID string `json:"content_id"`
}

// ComparisonReadHandler returns a ToolHandler that reads a comparison with
// its full items×dimensions value matrix. Privacy: the executor's privacy
// gate blocks the call when the backing content row is ai_exempt.
func ComparisonReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Comparison == nil {
			return errorToolResult("comparison_unavailable", "Comparison service is not configured"), nil
		}

		var a comparisonReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.ContentID == "" {
			return errorToolResult("missing_content_id", "content_id is required"), nil
		}

		contentID, err := scanUUID(a.ContentID)
		if err != nil {
			return errorToolResult("invalid_content_id", "content_id must be a valid UUID"), nil
		}

		engine := NewComparisonEngine(deps)
		res, err := engine.ReadComparison(ctx, contentID, tenantID)
		if err != nil {
			return errorToolResult("read_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"comparison": res.Comparison,
			"matrix":     res.Matrix,
		}), nil
	}
}

// ComparisonReadPrivacyTarget extracts the content_id from
// comparison.read arguments so the executor's privacy gate can block the
// call when the backing content row is ai_exempt.
func ComparisonReadPrivacyTarget(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
	var a comparisonReadArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}
	if a.ContentID == "" {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}
	id, err := scanUUID(a.ContentID)
	if err != nil || !id.Valid {
		return PrivacyEntityType(""), pgtype.UUID{}, nil
	}
	return PrivacyEntityContent, id, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// uuidsToStrings converts a slice of pgtype.UUID to a slice of strings,
// skipping invalid UUIDs.
func uuidsToStrings(ids []pgtype.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id.Valid {
			out = append(out, id.String())
		}
	}
	return out
}
