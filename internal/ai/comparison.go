// Package ai — comparison.go implements the AI Comparison Engine
// (Section 103 — Comparison Engine, extends §30).
//
// The Comparison Engine orchestrates the three-tier comparison flow
// described in §103:
//
//	Tier 1 — Structured:      a comparison type definition exists with
//	                          dimensions covering the request. The engine
//	                          reads existing comparison_values, fills gaps
//	                          via RAG + external search, and writes the
//	                          missing values with provenance metadata.
//
//	Tier 2 — Semi-structured: a comparison type exists but the requested
//	                          dimension requires additional dimensions. The
//	                          engine proposes new dimensions, creates them
//	                          (if the user is authorized), then fills values.
//
//	Tier 3 — Unstructured:    no comparison type or dimensions exist. The
//	                          engine synthesizes knowledge from RAG +
//	                          external sources, identifies comparison
//	                          dimensions, and either proposes a new
//	                          comparison type (if authorized) or produces a
//	                          prose-only comparison.
//
// Provenance: every value the engine fills records its source
// (custom_field|rag|external|llm_knowledge), source_ref, confidence, and
// retrieved_at timestamp in the comparison_values.metadata JSONB column
// (added by migration 000373).
//
// The engine is a pure orchestrator — it does NOT call the LLM itself. The
// agent loop (Phase 5) or the direct /api/v1/ai/compare-entities endpoint
// (Phase 7, task 7.07) drives the LLM to synthesize values; the engine
// handles the data movement: reading existing values, identifying gaps,
// writing filled values with provenance, and creating missing dimensions /
// types when authorized.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7, tasks 7.01-7.07)
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Provenance
// ---------------------------------------------------------------------------

// ComparisonProvenance is the JSONB shape stored in comparison_values.metadata
// for every AI-filled value (Section 103 — Provenance).
type ComparisonProvenance struct {
	Source      string `json:"source"`       // custom_field|rag|external|llm_knowledge
	SourceRef   string `json:"source_ref"`   // custom_field_values:uuid|content:uuid|https://...
	Confidence  string `json:"confidence"`   // high|medium|low
	RetrievedAt string `json:"retrieved_at"` // RFC3339 timestamp
}

// Provenance source constants.
const (
	ProvenanceSourceCustomField = "custom_field"
	ProvenanceSourceRAG         = "rag"
	ProvenanceSourceExternal    = "external"
	ProvenanceSourceLLM         = "llm_knowledge"
)

// Confidence level constants.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// MarshalJSON returns the provenance as JSONB bytes for storage in the
// comparison_values.metadata column.
func (p ComparisonProvenance) MarshalJSON() ([]byte, error) {
	type alias ComparisonProvenance
	return json.Marshal((alias)(p))
}

// provenanceBytes marshals a ComparisonProvenance to JSONB bytes, returning
// nil on error (a nil metadata writes the column default '{}').
func provenanceBytes(p ComparisonProvenance) []byte {
	b, err := json.Marshal(p)
	if err != nil {
		slog.Warn("ai: marshaling comparison provenance", "error", err)
		return nil
	}
	return b
}

// newProvenance constructs a ComparisonProvenance with the current UTC time.
func newProvenance(source, sourceRef, confidence string) ComparisonProvenance {
	return ComparisonProvenance{
		Source:      source,
		SourceRef:   sourceRef,
		Confidence:  confidence,
		RetrievedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// ---------------------------------------------------------------------------
// Tier classification
// ---------------------------------------------------------------------------

// ComparisonTier is the structural tier the engine classifies a request into.
type ComparisonTier string

const (
	// TierStructured: a comparison type definition exists with dimensions
	// covering the requested comparison.
	TierStructured ComparisonTier = "structured"
	// TierSemiStructured: a comparison type exists but the requested
	// dimension requires additional dimensions.
	TierSemiStructured ComparisonTier = "semi_structured"
	// TierUnstructured: no comparison type or dimensions exist for the
	// requested comparison.
	TierUnstructured ComparisonTier = "unstructured"
)

// String returns the tier as a lowercase string.
func (t ComparisonTier) String() string { return string(t) }

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

// ComparisonEngine orchestrates the §103 three-tier comparison flow. It is
// constructed from a *ToolDeps (same dependency container as the Phase 4
// tool handlers) so it can be wired from the same deps and tested with the
// same stubs.
//
// The engine does NOT call the LLM. It performs the data movement steps
// (read existing values, identify gaps, write filled values with provenance,
// create missing dimensions/types). The caller (agent loop or the direct
// compare-entities endpoint) supplies the synthesized values via the
// FillValueInput.Value field.
type ComparisonEngine struct {
	deps *ToolDeps
}

// NewComparisonEngine creates a ComparisonEngine from a ToolDeps container.
// The deps.Comparison field is required for all engine operations; deps.RAG
// and deps.Privacy are used by the tier-classification and gap-filling
// helpers.
func NewComparisonEngine(deps *ToolDeps) *ComparisonEngine {
	if deps == nil {
		deps = NewToolDeps()
	}
	return &ComparisonEngine{deps: deps}
}

// ClassifyRequest determines the comparison tier for a request by checking
// whether a matching comparison type definition exists and whether its
// dimensions cover the requested comparison fields.
//
//   - entityType: the slug of the comparison type to look up (e.g.
//     "universities", "countries"). When empty, the engine returns
//     TierUnstructured.
//   - requestedFields: the comparison dimensions the user wants (e.g.
//     ["cost_of_living", "healthcare"]). When empty, the engine returns
//     TierStructured if the type exists (the type's own dimensions are
//     sufficient).
//
// Returns the tier, the matched type (when found), and the set of dimension
// labels the type already provides. The "missing" set is the requested
// fields not covered by the type's existing dimensions — non-empty missing
// set with a matching type yields TierSemiStructured.
func (e *ComparisonEngine) ClassifyRequest(
	ctx context.Context,
	tenantID pgtype.UUID,
	entityType string,
	requestedFields []string,
) (tier ComparisonTier, typeRow *gen.GetComparisonTypeBySlugRow, existingDims []string, missing []string, err error) {
	if e.deps == nil || e.deps.Comparison == nil {
		return TierUnstructured, nil, nil, nil, fmt.Errorf("comparison service not configured")
	}

	if entityType == "" {
		return TierUnstructured, nil, nil, nil, nil
	}

	td, lookupErr := e.deps.Comparison.GetComparisonTypeBySlug(ctx, entityType, tenantID)
	if lookupErr != nil {
		// No matching type → Tier 3 (unstructured). A not-found error is
		// the expected signal for Tier 3, not a hard failure.
		slog.Debug("ai: comparison type not found, classifying as unstructured",
			"entity_type", entityType, "error", lookupErr)
		return TierUnstructured, nil, nil, nil, nil
	}

	// Type exists. Fetch its dimensions by reading the comparison detail
	// of any existing comparison content linked to this type. When no
	// comparison content exists yet, the type's field-group fields are the
	// dimensions (Section 30: "each field = one comparison_dimension row").
	existingDims = e.dimensionsForType(ctx, tenantID, td)

	if len(requestedFields) == 0 {
		// No specific fields requested — the type's own dimensions cover it.
		return TierStructured, &td, existingDims, nil, nil
	}

	// Compute the missing set: requested fields not in existingDims.
	existingSet := make(map[string]bool, len(existingDims))
	for _, d := range existingDims {
		existingSet[normalizeLabel(d)] = true
	}
	for _, f := range requestedFields {
		if !existingSet[normalizeLabel(f)] {
			missing = append(missing, f)
		}
	}

	if len(missing) > 0 {
		return TierSemiStructured, &td, existingDims, missing, nil
	}
	return TierStructured, &td, existingDims, nil, nil
}

// dimensionsForType returns the dimension labels for a comparison type. It
// first tries to read dimensions from an existing comparison content row
// linked to the type; when none exist, it falls back to the type's
// field-group fields via the custom fields service. When neither is
// available, it returns an empty slice.
func (e *ComparisonEngine) dimensionsForType(ctx context.Context, tenantID pgtype.UUID, td gen.GetComparisonTypeBySlugRow) []string {
	// The comparison type's field-group fields are the canonical dimension
	// set (Section 30). We read them via the custom fields service when
	// available.
	if e.deps.CustomFields != nil && td.FieldGroupID.Valid {
		fields, err := e.deps.CustomFields.ListFieldsByGroup(ctx, td.FieldGroupID, tenantID)
		if err != nil {
			slog.Warn("ai: listing field-group fields for comparison type dimensions",
				"type_slug", td.Slug, "error", err)
			return nil
		}
		dims := make([]string, 0, len(fields))
		for _, f := range fields {
			dims = append(dims, f.Label)
		}
		return dims
	}
	return nil
}

// ---------------------------------------------------------------------------
// Gap filling (Tier 1 + Tier 2 shared step)
// ---------------------------------------------------------------------------

// FillValueInput is a single value the caller (LLM or direct endpoint) wants
// the engine to write. The caller supplies the Value; the engine attaches
// provenance metadata and writes via ComparisonService.UpsertComparisonValueWithMetadata.
type FillValueInput struct {
	DimensionID pgtype.UUID
	ItemID      pgtype.UUID
	Value       string
	ValueRating *float64
	ValueBool   *bool
	Provenance  ComparisonProvenance
}

// FillValues writes a batch of comparison values with provenance metadata.
// Each value is upserted independently (the ComparisonService interface
// exposes only per-call UpsertComparisonValueWithMetadata); a failure on one
// value does not abort the others. The result reports per-value status.
//
// This is the shared "fill gaps" step for both Tier 1 (fill missing values
// in an existing matrix) and Tier 2 (fill values for newly created
// dimensions).
func (e *ComparisonEngine) FillValues(
	ctx context.Context,
	tenantID pgtype.UUID,
	inputs []FillValueInput,
) (*FillValuesResult, error) {
	if e.deps == nil || e.deps.Comparison == nil {
		return nil, fmt.Errorf("comparison service not configured")
	}

	result := &FillValuesResult{
		Filled: make([]FilledValue, 0, len(inputs)),
		Errors: make([]FillValueError, 0),
	}

	for _, in := range inputs {
		meta := provenanceBytes(in.Provenance)
		_, err := e.deps.Comparison.UpsertComparisonValueWithMetadata(ctx, tenantID, ComparisonValueUpsertInput{
			DimensionID:  in.DimensionID,
			ItemID:       in.ItemID,
			Value:        in.Value,
			ValueRating:  in.ValueRating,
			ValueBoolean: in.ValueBool,
		}, meta)
		if err != nil {
			result.Errors = append(result.Errors, FillValueError{
				DimensionID: in.DimensionID,
				ItemID:      in.ItemID,
				Error:       err.Error(),
			})
			continue
		}
		result.Filled = append(result.Filled, FilledValue{
			DimensionID: in.DimensionID,
			ItemID:      in.ItemID,
			Source:      in.Provenance.Source,
			Confidence:  in.Provenance.Confidence,
		})
	}

	return result, nil
}

// FillValuesResult reports the outcome of a batch fill.
type FillValuesResult struct {
	Filled []FilledValue    `json:"filled"`
	Errors []FillValueError `json:"errors,omitempty"`
}

// FilledValue is a successfully written comparison value.
type FilledValue struct {
	DimensionID pgtype.UUID `json:"dimension_id"`
	ItemID      pgtype.UUID `json:"item_id"`
	Source      string      `json:"source"`
	Confidence  string      `json:"confidence"`
}

// FillValueError is a failed write.
type FillValueError struct {
	DimensionID pgtype.UUID `json:"dimension_id"`
	ItemID      pgtype.UUID `json:"item_id"`
	Error       string      `json:"error"`
}

// ---------------------------------------------------------------------------
// Tier 2 — add missing dimensions
// ---------------------------------------------------------------------------

// AddDimensionInput is a single dimension the engine creates on an existing
// comparison content row (Tier 2). The caller supplies the Label and
// DimensionType; the engine assigns the next position.
type AddDimensionInput struct {
	ContentID     pgtype.UUID
	Label         string
	DimensionType string
}

// AddDimensions creates missing dimensions on an existing comparison content
// row. Used by the Tier 2 flow after ClassifyRequest reports a non-empty
// missing set. The caller must be authorized (the executor's RBAC gate
// checks the tool's required_permission before dispatch).
//
// The DimensionType defaults to "text" when empty.
func (e *ComparisonEngine) AddDimensions(
	ctx context.Context,
	tenantID pgtype.UUID,
	inputs []AddDimensionInput,
) ([]CreatedDimension, error) {
	if e.deps == nil || e.deps.Comparison == nil {
		return nil, fmt.Errorf("comparison service not configured")
	}

	// Determine the next position by reading the existing dimensions for
	// each content row. We batch by content ID to avoid re-reading.
	positionByContent := make(map[pgtype.UUID]int32)
	for _, in := range inputs {
		if _, ok := positionByContent[in.ContentID]; !ok {
			detail, err := e.deps.Comparison.GetComparisonDetail(ctx, in.ContentID, tenantID)
			if err != nil {
				slog.Warn("ai: reading comparison detail for dimension position",
					"content_id", in.ContentID, "error", err)
				positionByContent[in.ContentID] = 0
			} else {
				positionByContent[in.ContentID] = int32(len(detail.Dimensions))
			}
		}
	}

	created := make([]CreatedDimension, 0, len(inputs))
	for _, in := range inputs {
		dimType := in.DimensionType
		if dimType == "" {
			dimType = "text"
		}
		pos := positionByContent[in.ContentID]
		row, err := e.deps.Comparison.CreateComparisonDimension(ctx, tenantID, ComparisonDimensionCreateInput{
			ContentID:     in.ContentID,
			Label:         in.Label,
			DimensionType: dimType,
			Position:      pos,
		})
		if err != nil {
			return created, fmt.Errorf("creating dimension %q: %w", in.Label, err)
		}
		created = append(created, CreatedDimension{
			ID:            row.ID,
			ContentID:     row.ContentID,
			Label:         row.Label,
			DimensionType: row.DimensionType,
			Position:      row.Position,
		})
		positionByContent[in.ContentID] = pos + 1
	}

	return created, nil
}

// CreatedDimension is a successfully created comparison dimension.
type CreatedDimension struct {
	ID            pgtype.UUID `json:"id"`
	ContentID     pgtype.UUID `json:"content_id"`
	Label         string      `json:"label"`
	DimensionType string      `json:"dimension_type"`
	Position      int32       `json:"position"`
}

// ---------------------------------------------------------------------------
// Tier 3 — create a new comparison (type + content + items + dimensions)
// ---------------------------------------------------------------------------

// CreateComparisonInput is the input for the Tier 3 flow when the engine
// creates a new comparison content row (and optionally a new comparison type).
type CreateComparisonInput struct {
	// The comparison type to create. When CreateType is true, the engine
	// creates a new comparison_type_definition first, then links the
	// comparison content to it. When false, TypeID must be provided.
	CreateType bool
	TypeInput  ComparisonTypeCreateInput
	TypeID     pgtype.UUID // set when CreateType is false

	// The comparison content row.
	Title      string
	Body       string
	Visibility string
	Status     string // always forced to "draft" by the engine
	SpaceID    pgtype.UUID

	// Items and dimensions.
	Items      []ComparisonItemInput
	Dimensions []string
}

// CreateComparisonResult is the outcome of the Tier 3 create flow.
type CreateComparisonResult struct {
	TypeID       pgtype.UUID   `json:"type_id,omitempty"`
	ContentID    pgtype.UUID   `json:"content_id"`
	Title        string        `json:"title"`
	Slug         string        `json:"slug"`
	DimensionIDs []pgtype.UUID `json:"dimension_ids"`
	ItemIDs      []pgtype.UUID `json:"item_ids"`
	CreatedType  bool          `json:"created_type"`
}

// CreateComparison creates a new comparison content row, optionally creating
// a new comparison type definition first (Tier 3). The status is always
// forced to "draft" — the agent never publishes directly (§103 behavior
// rule, same as the content.create tool).
//
// The caller must be authorized for the underlying comparison.create /
// comparison_type.create permissions (enforced by the executor's RBAC gate
// at the tool level; the direct /compare-entities endpoint checks
// agent.use).
func (e *ComparisonEngine) CreateComparison(
	ctx context.Context,
	tenantID, userID pgtype.UUID,
	in CreateComparisonInput,
) (*CreateComparisonResult, error) {
	if e.deps == nil || e.deps.Comparison == nil {
		return nil, fmt.Errorf("comparison service not configured")
	}
	if in.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	// §103: the agent never publishes directly.
	in.Status = "draft"

	var typeID pgtype.UUID
	createdType := false

	if in.CreateType {
		if in.TypeInput.Name == "" || in.TypeInput.Slug == "" {
			return nil, fmt.Errorf("create_type requires name and slug")
		}
		td, err := e.deps.Comparison.CreateComparisonType(ctx, tenantID, in.TypeInput)
		if err != nil {
			return nil, fmt.Errorf("creating comparison type: %w", err)
		}
		typeID = td.ID
		createdType = true
	} else {
		typeID = in.TypeID
	}

	// Build the comparison content row with items + dimensions.
	res, err := e.deps.Comparison.CreateComparison(ctx, tenantID, userID, ComparisonCreateInput{
		Title:      in.Title,
		Body:       in.Body,
		Visibility: in.Visibility,
		Status:     in.Status,
		SpaceID:    in.SpaceID,
		TypeID:     typeID,
		Items:      in.Items,
		Dimensions: in.Dimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("creating comparison content: %w", err)
	}

	// Read back the created items + dimensions to surface their IDs.
	detail, detailErr := e.deps.Comparison.GetComparisonDetail(ctx, res.ID, tenantID)
	if detailErr != nil {
		// The comparison was created; the detail read is best-effort.
		slog.Warn("ai: reading back created comparison detail", "content_id", res.ID, "error", detailErr)
	}
	dimIDs := make([]pgtype.UUID, 0)
	itemIDs := make([]pgtype.UUID, 0)
	for _, d := range detail.Dimensions {
		dimIDs = append(dimIDs, d.ID)
	}
	for _, i := range detail.Items {
		itemIDs = append(itemIDs, i.ID)
	}

	return &CreateComparisonResult{
		TypeID:       typeID,
		ContentID:    res.ID,
		Title:        res.Title,
		Slug:         res.Slug,
		DimensionIDs: dimIDs,
		ItemIDs:      itemIDs,
		CreatedType:  createdType,
	}, nil
}

// ---------------------------------------------------------------------------
// Read (shared by all tiers — assemble the matrix for the LLM / caller)
// ---------------------------------------------------------------------------

// ReadComparisonResult is the assembled comparison view the engine returns
// from ReadComparison. It includes the parent content metadata, items,
// dimensions, and the full items×dimensions value matrix.
type ReadComparisonResult struct {
	Comparison ComparisonDetail       `json:"comparison"`
	Matrix     []ComparisonMatrixItem `json:"matrix"`
}

// ReadComparison assembles the full comparison view: parent content metadata
// + items + dimensions + the items×dimensions value matrix. Used by the
// comparison.read tool and as the final step of all three tier flows.
func (e *ComparisonEngine) ReadComparison(
	ctx context.Context,
	contentID, tenantID pgtype.UUID,
) (*ReadComparisonResult, error) {
	if e.deps == nil || e.deps.Comparison == nil {
		return nil, fmt.Errorf("comparison service not configured")
	}

	detail, err := e.deps.Comparison.GetComparisonDetail(ctx, contentID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reading comparison detail: %w", err)
	}

	matrix, err := e.deps.Comparison.GetComparisonMatrix(ctx, contentID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reading comparison matrix: %w", err)
	}

	return &ReadComparisonResult{
		Comparison: detail,
		Matrix:     matrix,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// normalizeLabel lowercases and trims a dimension label for set comparison.
// Dimension labels are free-text; the engine treats "Cost of Living" and
// "cost of living" as the same dimension.
func normalizeLabel(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
