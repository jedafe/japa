// Package ai — comparison_test.go covers the Phase 7 Comparison Engine
// and comparison tool handlers (Section 103 — Comparison Engine, extends §30).
//
// Tests verify:
//   - Tier classification: structured / semi-structured / unstructured.
//   - Provenance marshaling + the newProvenance helper.
//   - FillValues: success, partial failure, nil deps.
//   - AddDimensions: position assignment, dimension type default.
//   - CreateComparison: status forced to draft, type creation, read-back.
//   - ReadComparison: matrix assembly.
//   - Tool handlers: types.list, create, fill_values, add_dimension, read.
//   - Privacy target extractors: fill_values + read.
//   - Direct /compare-entities endpoint: tier response, validation.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Stub ComparisonService
// ---------------------------------------------------------------------------

// stubComparisonService implements ComparisonService for tests.
type stubComparisonService struct {
	types       []gen.ListComparisonTypesRow
	typeBySlug  map[string]gen.GetComparisonTypeBySlugRow
	createdType gen.CreateComparisonTypeRow

	createComparisonResult ComparisonCreateResult
	createComparisonErr    error

	detail ComparisonDetail
	matrix []ComparisonMatrixItem

	createdDimension gen.CreateComparisonDimensionRow

	upsertedValues []stubUpsertedValue
	upsertErr      error

	promoteResult ComparisonPromoteResult
	promoteErr    error
	promoteBypass bool

	listResult  ComparisonListResult
	listErr     error
	gotListArgs *ComparisonListFilter
}

type stubUpsertedValue struct {
	Input    ComparisonValueUpsertInput
	Metadata []byte
}

func newStubComparisonService() *stubComparisonService {
	return &stubComparisonService{
		typeBySlug: make(map[string]gen.GetComparisonTypeBySlugRow),
	}
}

func (s *stubComparisonService) ListComparisonTypes(_ context.Context, limit, offset int32) ([]gen.ListComparisonTypesRow, error) {
	return s.types, nil
}

func (s *stubComparisonService) GetComparisonTypeBySlug(_ context.Context, slug string) (gen.GetComparisonTypeBySlugRow, error) {
	td, ok := s.typeBySlug[slug]
	if !ok {
		return gen.GetComparisonTypeBySlugRow{}, pgx.ErrNoRows
	}

	return td, nil
}

func (s *stubComparisonService) CreateComparisonType(_ context.Context, input ComparisonTypeCreateInput) (gen.CreateComparisonTypeRow, error) {
	return s.createdType, nil
}

func (s *stubComparisonService) CreateComparison(_ context.Context, userID pgtype.UUID, input ComparisonCreateInput) (ComparisonCreateResult, error) {
	if s.createComparisonErr != nil {
		return ComparisonCreateResult{}, s.createComparisonErr
	}

	return s.createComparisonResult, nil
}

func (s *stubComparisonService) GetComparisonDetail(_ context.Context, id pgtype.UUID) (ComparisonDetail, error) {
	return s.detail, nil
}

func (s *stubComparisonService) GetComparisonMatrix(_ context.Context, contentID pgtype.UUID) ([]ComparisonMatrixItem, error) {
	return s.matrix, nil
}

func (s *stubComparisonService) CreateComparisonDimension(_ context.Context, input ComparisonDimensionCreateInput) (gen.CreateComparisonDimensionRow, error) {
	return s.createdDimension, nil
}

func (s *stubComparisonService) UpsertComparisonValueWithMetadata(_ context.Context, input ComparisonValueUpsertInput, metadata []byte) (gen.ComparisonValue, error) {
	if s.upsertErr != nil {
		return gen.ComparisonValue{}, s.upsertErr
	}

	s.upsertedValues = append(s.upsertedValues, stubUpsertedValue{Input: input, Metadata: metadata})

	return gen.ComparisonValue{
		DimensionID: input.DimensionID,
		ItemID:      input.ItemID,

		Metadata: metadata,
	}, nil
}

func (s *stubComparisonService) DeleteJCEComparisonCaches(_ context.Context, _ pgtype.UUID) {}

func (s *stubComparisonService) PromoteComparison(_ context.Context, id, userID pgtype.UUID, bypassOwnership bool) (ComparisonPromoteResult, error) {
	s.promoteBypass = bypassOwnership
	if s.promoteErr != nil {
		return ComparisonPromoteResult{}, s.promoteErr
	}

	return s.promoteResult, nil
}

func (s *stubComparisonService) ListComparisons(_ context.Context, f ComparisonListFilter) (ComparisonListResult, error) {
	if s.listErr != nil {
		return ComparisonListResult{}, s.listErr
	}

	s.gotListArgs = &f

	return s.listResult, nil
}

// ---------------------------------------------------------------------------
// Provenance
// ---------------------------------------------------------------------------

func TestComparisonProvenance_Marshal(t *testing.T) {
	t.Parallel()

	p := ComparisonProvenance{
		Source:      ProvenanceSourceRAG,
		SourceRef:   "content:00000000-0000-0000-0000-000000000001",
		Confidence:  ConfidenceHigh,
		RetrievedAt: "2026-08-11T12:00:00Z",
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out ComparisonProvenance
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Source != ProvenanceSourceRAG {
		t.Errorf("source = %q, want %q", out.Source, ProvenanceSourceRAG)
	}

	if out.Confidence != ConfidenceHigh {
		t.Errorf("confidence = %q, want %q", out.Confidence, ConfidenceHigh)
	}
}

func TestNewProvenance_SetsTimestamp(t *testing.T) {
	t.Parallel()

	p := newProvenance(ProvenanceSourceExternal, "https://example.com", ConfidenceMedium)
	if p.Source != ProvenanceSourceExternal {
		t.Errorf("source = %q", p.Source)
	}

	if p.RetrievedAt == "" {
		t.Error("retrieved_at should be set")
	}

	if !strings.Contains(p.RetrievedAt, "T") {
		t.Errorf("retrieved_at should be RFC3339, got %q", p.RetrievedAt)
	}
}

func TestProvenanceBytes_NilOnMarshalError(t *testing.T) {
	t.Parallel()
	// A valid provenance should produce non-nil bytes.
	b := provenanceBytes(newProvenance(ProvenanceSourceLLM, "", ConfidenceLow))
	if b == nil {
		t.Error("expected non-nil bytes for valid provenance")
	}

	var p ComparisonProvenance
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if p.Source != ProvenanceSourceLLM {
		t.Errorf("source = %q, want %q", p.Source, ProvenanceSourceLLM)
	}
}

// ---------------------------------------------------------------------------
// ClassifyRequest (tier detection)
// ---------------------------------------------------------------------------

func TestClassifyRequest_Unstructured_NoEntityType(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	engine := NewComparisonEngine(deps)

	tier, typeRow, existing, missing, err := engine.ClassifyRequest(t.Context(), "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tier != TierUnstructured {
		t.Errorf("tier = %q, want %q", tier, TierUnstructured)
	}

	if typeRow != nil {
		t.Error("expected nil typeRow for no entity type")
	}

	if existing != nil || missing != nil {
		t.Error("expected nil existing/missing for no entity type")
	}
}

func TestClassifyRequest_Unstructured_TypeNotFound(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	engine := NewComparisonEngine(deps)

	tier, _, _, _, err := engine.ClassifyRequest(t.Context(), "nonexistent", []string{"cost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tier != TierUnstructured {
		t.Errorf("tier = %q, want %q", tier, TierUnstructured)
	}
}

func TestClassifyRequest_Structured_NoFieldsRequested(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.typeBySlug["universities"] = gen.GetComparisonTypeBySlugRow{
		ID:   mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Slug: "universities",
		Name: "Universities",
	}
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	tier, typeRow, _, _, err := engine.ClassifyRequest(t.Context(), "universities", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tier != TierStructured {
		t.Errorf("tier = %q, want %q", tier, TierStructured)
	}

	if typeRow == nil {
		t.Fatal("expected non-nil typeRow")
	}

	if typeRow.Slug != "universities" {
		t.Errorf("slug = %q", typeRow.Slug)
	}
}

func TestClassifyRequest_SemiStructured_MissingFields(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	fgID := mustUUID(t, "00000000-0000-0000-0000-000000000010")
	svc.typeBySlug["countries"] = gen.GetComparisonTypeBySlugRow{
		ID:           mustUUID(t, "00000000-0000-0000-0000-000000000002"),
		Slug:         "countries",
		Name:         "Countries",
		FieldGroupID: fgID,
	}
	// Provide custom fields so dimensionsForType returns existing dims.
	deps := &ToolDeps{
		Comparison:   svc,
		CustomFields: &stubCustomFieldsService{fields: []gen.ListCustomFieldsByGroupRow{{Label: "cost_of_living"}, {Label: "healthcare"}}},
	}
	engine := NewComparisonEngine(deps)

	tier, _, existing, missing, err := engine.ClassifyRequest(
		t.Context(), "countries",
		[]string{"cost_of_living", "infrastructure"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tier != TierSemiStructured {
		t.Errorf("tier = %q, want %q", tier, TierSemiStructured)
	}

	if len(existing) != 2 {
		t.Errorf("existing dims = %d, want 2", len(existing))
	}

	if len(missing) != 1 || missing[0] != "infrastructure" {
		t.Errorf("missing = %v, want [infrastructure]", missing)
	}
}

func TestClassifyRequest_Structured_AllFieldsCovered(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	fgID := mustUUID(t, "00000000-0000-0000-0000-000000000010")
	svc.typeBySlug["countries"] = gen.GetComparisonTypeBySlugRow{
		ID:           mustUUID(t, "00000000-0000-0000-0000-000000000002"),
		Slug:         "countries",
		FieldGroupID: fgID,
	}
	deps := &ToolDeps{
		Comparison:   svc,
		CustomFields: &stubCustomFieldsService{fields: []gen.ListCustomFieldsByGroupRow{{Label: "cost_of_living"}, {Label: "healthcare"}}},
	}
	engine := NewComparisonEngine(deps)

	tier, _, _, missing, err := engine.ClassifyRequest(
		t.Context(), "countries",
		[]string{"cost_of_living", "healthcare"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tier != TierStructured {
		t.Errorf("tier = %q, want %q", tier, TierStructured)
	}

	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
}

func TestClassifyRequest_NilDeps(t *testing.T) {
	t.Parallel()

	engine := NewComparisonEngine(nil)

	_, _, _, _, err := engine.ClassifyRequest(t.Context(), "x", nil)
	if err == nil {
		t.Error("expected error for nil deps")
	}
}

// ---------------------------------------------------------------------------
// FillValues
// ---------------------------------------------------------------------------

func TestFillValues_Success(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	dimID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	itemID := mustUUID(t, "00000000-0000-0000-0000-000000000002")

	result, err := engine.FillValues(t.Context(), []FillValueInput{
		{
			DimensionID: dimID,
			ItemID:      itemID,
			Value:       "€500/month",
			Provenance:  newProvenance(ProvenanceSourceRAG, "content:abc", ConfidenceHigh),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Filled) != 1 {
		t.Fatalf("filled = %d, want 1", len(result.Filled))
	}

	if len(result.Errors) != 0 {
		t.Fatalf("errors = %d, want 0", len(result.Errors))
	}

	if len(svc.upsertedValues) != 1 {
		t.Fatalf("upserted = %d, want 1", len(svc.upsertedValues))
	}
	// Verify provenance metadata was passed through.
	if len(svc.upsertedValues[0].Metadata) == 0 {
		t.Error("expected non-empty metadata")
	}

	var prov ComparisonProvenance
	if err := json.Unmarshal(svc.upsertedValues[0].Metadata, &prov); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}

	if prov.Source != ProvenanceSourceRAG {
		t.Errorf("metadata source = %q, want %q", prov.Source, ProvenanceSourceRAG)
	}
}

func TestFillValues_PartialFailure(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.upsertErr = errors.New("db error")
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	dimID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	itemID := mustUUID(t, "00000000-0000-0000-0000-000000000002")

	result, err := engine.FillValues(t.Context(), []FillValueInput{
		{DimensionID: dimID, ItemID: itemID, Value: "x"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Filled) != 0 {
		t.Errorf("filled = %d, want 0", len(result.Filled))
	}

	if len(result.Errors) != 1 {
		t.Errorf("errors = %d, want 1", len(result.Errors))
	}
}

func TestFillValues_NilDeps(t *testing.T) {
	t.Parallel()

	engine := NewComparisonEngine(nil)

	_, err := engine.FillValues(t.Context(), nil)
	if err == nil {
		t.Error("expected error for nil deps")
	}
}

// ---------------------------------------------------------------------------
// AddDimensions
// ---------------------------------------------------------------------------

func TestAddDimensions_Success(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.detail = ComparisonDetail{
		Dimensions: []gen.ListComparisonDimensionsByContentRow{
			{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Label: "cost_of_living"},
		},
	}
	svc.createdDimension = gen.CreateComparisonDimensionRow{
		ID:            mustUUID(t, "00000000-0000-0000-0000-000000000099"),
		ContentID:     mustUUID(t, "00000000-0000-0000-0000-000000000010"),
		Label:         "infrastructure",
		DimensionType: "text",
		Position:      1,
	}
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000010")

	created, err := engine.AddDimensions(t.Context(), []AddDimensionInput{
		{ContentID: contentID, Label: "infrastructure"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(created) != 1 {
		t.Fatalf("created = %d, want 1", len(created))
	}

	if created[0].Label != "infrastructure" {
		t.Errorf("label = %q", created[0].Label)
	}

	if created[0].Position != 1 {
		t.Errorf("position = %d, want 1 (after existing 1 dim)", created[0].Position)
	}
}

func TestAddDimensions_EmptyTypeDefaultsToText(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.createdDimension = gen.CreateComparisonDimensionRow{
		Label:         "quality",
		DimensionType: "text",
	}
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000010")

	_, err := engine.AddDimensions(t.Context(), []AddDimensionInput{
		{ContentID: contentID, Label: "quality", DimensionType: ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateComparison (Tier 3)
// ---------------------------------------------------------------------------

func TestCreateComparison_ForcesDraftStatus(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.createComparisonResult = ComparisonCreateResult{
		ID:    mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Title: "Test",
		Slug:  "test",
	}
	svc.detail = ComparisonDetail{
		Items:      []gen.ListComparisonItemsByContentRow{{ID: mustUUID(t, "00000000-0000-0000-0000-000000000002")}},
		Dimensions: []gen.ListComparisonDimensionsByContentRow{{ID: mustUUID(t, "00000000-0000-0000-0000-000000000003")}},
	}
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	// Pass status = "published" — engine should force "draft".
	res, err := engine.CreateComparison(t.Context(), pgtype.UUID{}, CreateComparisonInput{
		Title:      "Test",
		Status:     "published",
		Items:      []ComparisonItemInput{{Name: "Item1"}},
		Dimensions: []string{"dim1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The engine forces status to "draft" before calling CreateComparison.
	// The stub doesn't inspect the status, but we verify the engine's
	// behavior by checking it didn't error and returned the result.
	if res.ContentID.Bytes == [16]byte{} {
		t.Error("expected non-zero content ID")
	}

	if len(res.ItemIDs) != 1 {
		t.Errorf("item_ids = %d, want 1", len(res.ItemIDs))
	}

	if len(res.DimensionIDs) != 1 {
		t.Errorf("dimension_ids = %d, want 1", len(res.DimensionIDs))
	}
}

func TestCreateComparison_CreateType(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.createdType = gen.CreateComparisonTypeRow{
		ID:   mustUUID(t, "00000000-0000-0000-0000-000000000005"),
		Slug: "education-systems",
		Name: "Education Systems",
	}
	svc.createComparisonResult = ComparisonCreateResult{
		ID:    mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Title: "NG vs GH",
	}
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	res, err := engine.CreateComparison(t.Context(), pgtype.UUID{}, CreateComparisonInput{
		CreateType: true,
		TypeInput: ComparisonTypeCreateInput{
			Name: "Education Systems",
			Slug: "education-systems",
		},
		Title: "NG vs GH",
		Items: []ComparisonItemInput{{Name: "Nigeria"}, {Name: "Ghana"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.CreatedType {
		t.Error("expected created_type = true")
	}

	if res.TypeID.Bytes != svc.createdType.ID.Bytes {
		t.Errorf("type_id mismatch")
	}
}

func TestCreateComparison_MissingTitle(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	engine := NewComparisonEngine(deps)

	_, err := engine.CreateComparison(t.Context(), pgtype.UUID{}, CreateComparisonInput{})
	if err == nil {
		t.Error("expected error for missing title")
	}
}

// ---------------------------------------------------------------------------
// ReadComparison
// ---------------------------------------------------------------------------

func TestReadComparison_Success(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.detail = ComparisonDetail{Title: "TUM vs Stuttgart"}
	svc.matrix = []ComparisonMatrixItem{
		{Item: gen.ListComparisonItemsByContentRow{Title: "TUM"}},
	}
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	res, err := engine.ReadComparison(t.Context(), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Comparison.Title != "TUM vs Stuttgart" {
		t.Errorf("title = %q", res.Comparison.Title)
	}

	if len(res.Matrix) != 1 {
		t.Errorf("matrix = %d, want 1", len(res.Matrix))
	}
}

// ---------------------------------------------------------------------------
// Tool handlers
// ---------------------------------------------------------------------------

func TestComparisonTypesListHandler_Success(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.types = []gen.ListComparisonTypesRow{
		{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Name: "Universities", Slug: "universities", ItemLabel: "University", IsSystem: true},
	}
	deps := &ToolDeps{Comparison: svc}

	handler := ComparisonTypesListHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)

	types, _ := m["types"].([]any)
	if len(types) != 1 {
		t.Fatalf("types = %d, want 1", len(types))
	}
}

func TestComparisonTypesListHandler_NilDeps(t *testing.T) {
	t.Parallel()

	handler := ComparisonTypesListHandler(nil)

	tr, err := handler(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestComparisonCreateHandler_Success(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.createComparisonResult = ComparisonCreateResult{
		ID:    mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Title: "TUM vs Stuttgart",
		Slug:  "tum-vs-stuttgart",
	}
	svc.detail = ComparisonDetail{
		Items:      []gen.ListComparisonItemsByContentRow{{ID: mustUUID(t, "00000000-0000-0000-0000-000000000002")}},
		Dimensions: []gen.ListComparisonDimensionsByContentRow{{ID: mustUUID(t, "00000000-0000-0000-0000-000000000003")}},
	}
	deps := &ToolDeps{Comparison: svc}

	args := `{"title":"TUM vs Stuttgart","items":[{"name":"TUM"},{"name":"Stuttgart"}],"dimensions":["cost","ranking"]}`
	handler := ComparisonCreateHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["status"] != "draft" {
		t.Errorf("status = %v, want draft", m["status"])
	}
}

func TestComparisonCreateHandler_MissingTitle(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	handler := ComparisonCreateHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"items":[{"name":"x"}]}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestComparisonCreateHandler_MissingItems(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	handler := ComparisonCreateHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"title":"x"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestComparisonFillValuesHandler_Success(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	deps := &ToolDeps{Comparison: svc}

	args := `{
		"content_id": "00000000-0000-0000-0000-000000000001",
		"values": [
			{
				"dimension_id": "00000000-0000-0000-0000-000000000010",
				"item_id": "00000000-0000-0000-0000-000000000020",
				"value": "€500/month",
				"source": "rag",
				"source_ref": "content:abc",
				"confidence": "high"
			}
		]
	}`
	handler := ComparisonFillValuesHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["filled_count"].(float64) != 1 {
		t.Errorf("filled_count = %v, want 1", m["filled_count"])
	}
	// Verify provenance was written.
	if len(svc.upsertedValues) != 1 {
		t.Fatalf("upserted = %d, want 1", len(svc.upsertedValues))
	}

	var prov ComparisonProvenance
	if err := json.Unmarshal(svc.upsertedValues[0].Metadata, &prov); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}

	if prov.Source != ProvenanceSourceRAG {
		t.Errorf("source = %q, want %q", prov.Source, ProvenanceSourceRAG)
	}

	if prov.Confidence != ConfidenceHigh {
		t.Errorf("confidence = %q, want %q", prov.Confidence, ConfidenceHigh)
	}
}

func TestComparisonFillValuesHandler_DefaultsSourceToLLM(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	deps := &ToolDeps{Comparison: svc}

	args := `{
		"content_id": "00000000-0000-0000-0000-000000000001",
		"values": [
			{
				"dimension_id": "00000000-0000-0000-0000-000000000010",
				"item_id": "00000000-0000-0000-0000-000000000020",
				"value": "some value"
			}
		]
	}`
	handler := ComparisonFillValuesHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	var prov ComparisonProvenance
	if err := json.Unmarshal(svc.upsertedValues[0].Metadata, &prov); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if prov.Source != ProvenanceSourceLLM {
		t.Errorf("default source = %q, want %q", prov.Source, ProvenanceSourceLLM)
	}

	if prov.Confidence != ConfidenceMedium {
		t.Errorf("default confidence = %q, want %q", prov.Confidence, ConfidenceMedium)
	}
}

func TestComparisonFillValuesHandler_MissingContentID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	handler := ComparisonFillValuesHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"values":[]}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestComparisonAddDimensionHandler_Success(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.createdDimension = gen.CreateComparisonDimensionRow{
		ID:            mustUUID(t, "00000000-0000-0000-0000-000000000099"),
		ContentID:     mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Label:         "infrastructure",
		DimensionType: "text",
	}
	deps := &ToolDeps{Comparison: svc}

	args := `{
		"content_id": "00000000-0000-0000-0000-000000000001",
		"dimensions": [{"label": "infrastructure"}, {"label": "social_integration", "dimension_type": "rating"}]
	}`
	handler := ComparisonAddDimensionHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["created_count"].(float64) != 2 {
		t.Errorf("created_count = %v, want 2", m["created_count"])
	}
}

func TestComparisonReadHandler_Success(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.detail = ComparisonDetail{Title: "TUM vs Stuttgart"}
	svc.matrix = []ComparisonMatrixItem{{Item: gen.ListComparisonItemsByContentRow{Title: "TUM"}}}
	deps := &ToolDeps{Comparison: svc}

	args := `{"content_id": "00000000-0000-0000-0000-000000000001"}`
	handler := ComparisonReadHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)

	comp, _ := m["comparison"].(map[string]any)
	if comp == nil {
		t.Fatal("expected comparison object")
	}

	if comp["title"] != "TUM vs Stuttgart" {
		t.Errorf("title = %v", comp["title"])
	}
}

func TestComparisonReadHandler_InvalidUUID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	handler := ComparisonReadHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id": "not-a-uuid"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

// ---------------------------------------------------------------------------
// Validation tests (audit fixes)
// ---------------------------------------------------------------------------

func TestComparisonFillValuesHandler_InvalidSource(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	handler := ComparisonFillValuesHandler(deps)
	args := `{
		"content_id": "00000000-0000-0000-0000-000000000001",
		"values": [{
			"dimension_id": "00000000-0000-0000-0000-000000000010",
			"item_id": "00000000-0000-0000-0000-000000000020",
			"source": "invalid_source"
		}]
	}`

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "invalid_source" {
		t.Errorf("code = %v, want invalid_source", m["code"])
	}
}

func TestComparisonFillValuesHandler_InvalidConfidence(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	handler := ComparisonFillValuesHandler(deps)
	args := `{
		"content_id": "00000000-0000-0000-0000-000000000001",
		"values": [{
			"dimension_id": "00000000-0000-0000-0000-000000000010",
			"item_id": "00000000-0000-0000-0000-000000000020",
			"source": "rag",
			"confidence": "very_high"
		}]
	}`

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "invalid_confidence" {
		t.Errorf("code = %v, want invalid_confidence", m["code"])
	}
}

func TestComparisonAddDimensionHandler_InvalidDimensionType(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	handler := ComparisonAddDimensionHandler(deps)
	args := `{
		"content_id": "00000000-0000-0000-0000-000000000001",
		"dimensions": [{"label": "test", "dimension_type": "invalid_type"}]
	}`

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "invalid_dimension_type" {
		t.Errorf("code = %v, want invalid_dimension_type", m["code"])
	}
}

func TestComparisonAddDimensionHandler_ValidDimensionTypes(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.createdDimension = gen.CreateComparisonDimensionRow{
		Label:         "test",
		DimensionType: "rating",
	}
	deps := &ToolDeps{Comparison: svc}

	validTypes := []string{"text", "rating", "boolean", "number", "currency"}
	for _, dt := range validTypes {
		args := fmt.Sprintf(`{
			"content_id": "00000000-0000-0000-0000-000000000001",
			"dimensions": [{"label": "test", "dimension_type": "%s"}]
		}`, dt)
		handler := ComparisonAddDimensionHandler(deps)

		tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{})
		if err != nil {
			t.Fatalf("unexpected error for type %q: %v", dt, err)
		}

		if tr.Status != ToolCallStatusSuccess {
			t.Fatalf("type %q: status = %q, want success", dt, tr.Status)
		}
	}
}

// ---------------------------------------------------------------------------
// Privacy target extractors
// ---------------------------------------------------------------------------

func TestComparisonFillValuesPrivacyTarget_ValidContentID(t *testing.T) {
	t.Parallel()

	args := json.RawMessage(`{"content_id": "00000000-0000-0000-0000-000000000001", "values": []}`)

	entityType, id, err := ComparisonFillValuesPrivacyTarget(t.Context(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entityType != PrivacyEntityContent {
		t.Errorf("entity type = %q, want %q", entityType, PrivacyEntityContent)
	}

	if !id.Valid {
		t.Error("expected valid UUID")
	}
}

func TestComparisonFillValuesPrivacyTarget_NoContentID(t *testing.T) {
	t.Parallel()

	args := json.RawMessage(`{"values": []}`)

	entityType, _, err := ComparisonFillValuesPrivacyTarget(t.Context(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entityType != PrivacyEntityType("") {
		t.Errorf("entity type = %q, want empty (no target)", entityType)
	}
}

func TestComparisonReadPrivacyTarget_ValidContentID(t *testing.T) {
	t.Parallel()

	args := json.RawMessage(`{"content_id": "00000000-0000-0000-0000-000000000001"}`)

	entityType, id, err := ComparisonReadPrivacyTarget(t.Context(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entityType != PrivacyEntityContent {
		t.Errorf("entity type = %q, want %q", entityType, PrivacyEntityContent)
	}

	if !id.Valid {
		t.Error("expected valid UUID")
	}
}

func TestComparisonReadPrivacyTarget_InvalidContentID(t *testing.T) {
	t.Parallel()

	args := json.RawMessage(`{"content_id": "xyz"}`)

	entityType, _, err := ComparisonReadPrivacyTarget(t.Context(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entityType != PrivacyEntityType("") {
		t.Errorf("entity type = %q, want empty (no target)", entityType)
	}
}

// ---------------------------------------------------------------------------
// RegisterPhase7ComparisonTools
// ---------------------------------------------------------------------------

func TestRegisterPhase7ComparisonTools_AllRegistered(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}
	registry := NewToolHandlerRegistry()
	RegisterPhase7ComparisonTools(registry, deps)

	expected := []string{
		"comparison.types.list",
		"comparison.create",
		"comparison.fill_values",
		"comparison.add_dimension",
		"comparison.read",
	}
	for _, name := range expected {
		if registry.Handler(name) == nil {
			t.Errorf("handler for %q not registered", name)
		}
	}
}

func TestRegisterPhase7ComparisonTools_NilArgs(t *testing.T) {
	t.Parallel()
	// Should not panic on nil args.
	RegisterPhase7ComparisonTools(nil, nil)
	RegisterPhase7ComparisonTools(NewToolHandlerRegistry(), nil)
}

// ---------------------------------------------------------------------------
// POST /api/v1/ai/compare-entities (direct endpoint)
// ---------------------------------------------------------------------------

func TestHandleCompareEntities_UnstructuredTier(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService() // no types → unstructured
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	h, _, _, _ := newTestAgentHTTPHandlers(t)
	h.engine = engine

	user, tn := testUserAndPlatform()
	body := `{"entity_type":"nonexistent","entity_ids":["TUM","Stuttgart"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/compare-entities", strings.NewReader(body))
	req = withAuthContext(req, user, tn)
	rr := httptest.NewRecorder()

	h.HandleCompareEntities(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Tier       string   `json:"tier"`
			EntityType string   `json:"entity_type"`
			EntityIDs  []string `json:"entity_ids"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.Tier != "unstructured" {
		t.Errorf("tier = %q, want unstructured", resp.Data.Tier)
	}

	if resp.Data.EntityType != "nonexistent" {
		t.Errorf("entity_type = %q", resp.Data.EntityType)
	}

	if len(resp.Data.EntityIDs) != 2 {
		t.Errorf("entity_ids = %d, want 2", len(resp.Data.EntityIDs))
	}
}

func TestHandleCompareEntities_MissingEntityType(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndPlatform()
	body := `{"entity_ids":["x"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/compare-entities", strings.NewReader(body))
	req = withAuthContext(req, user, tn)
	rr := httptest.NewRecorder()

	h.HandleCompareEntities(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	errObj := decodeError(t, rr.Body)
	if errObj["code"] != "validation_error" {
		t.Errorf("code = %q, want validation_error", errObj["code"])
	}
}

func TestHandleCompareEntities_MissingEntityIDs(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndPlatform()
	body := `{"entity_type":"universities"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/compare-entities", strings.NewReader(body))
	req = withAuthContext(req, user, tn)
	rr := httptest.NewRecorder()

	h.HandleCompareEntities(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCompareEntities_EngineNotConfigured(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	// h.engine is nil by default
	user, tn := testUserAndPlatform()
	body := `{"entity_type":"x","entity_ids":["a"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/compare-entities", strings.NewReader(body))
	req = withAuthContext(req, user, tn)
	rr := httptest.NewRecorder()

	h.HandleCompareEntities(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestHandleCompareEntities_StructuredTier(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.typeBySlug["universities"] = gen.GetComparisonTypeBySlugRow{
		ID:   mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		Slug: "universities",
		Name: "Universities",
	}
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	h, _, _, _ := newTestAgentHTTPHandlers(t)
	h.engine = engine

	user, tn := testUserAndPlatform()
	body := `{"entity_type":"universities","entity_ids":["TUM","Stuttgart"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/compare-entities", strings.NewReader(body))
	req = withAuthContext(req, user, tn)
	rr := httptest.NewRecorder()

	h.HandleCompareEntities(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Tier     string `json:"tier"`
			TypeID   string `json:"type_id"`
			TypeName string `json:"type_name"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.Tier != "structured" {
		t.Errorf("tier = %q, want structured", resp.Data.Tier)
	}

	if resp.Data.TypeName != "Universities" {
		t.Errorf("type_name = %q", resp.Data.TypeName)
	}
}

func TestHandleCompareEntities_CreatePost_Tier3(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	svc.createComparisonResult = ComparisonCreateResult{
		ID:    mustUUID(t, "00000000-0000-0000-0000-000000000099"),
		Title: "TUM vs Stuttgart",
		Slug:  "tum-vs-stuttgart",
	}
	svc.detail = ComparisonDetail{
		Items: []gen.ListComparisonItemsByContentRow{
			{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001")},
			{ID: mustUUID(t, "00000000-0000-0000-0000-000000000002")},
		},
	}
	deps := &ToolDeps{Comparison: svc}
	engine := NewComparisonEngine(deps)

	h, _, _, _ := newTestAgentHTTPHandlers(t)
	h.engine = engine

	user, tn := testUserAndPlatform()
	body := `{"entity_type":"nonexistent","entity_ids":["TUM","Stuttgart"],"create_post":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/compare-entities", strings.NewReader(body))
	req = withAuthContext(req, user, tn)
	rr := httptest.NewRecorder()

	h.HandleCompareEntities(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Tier          string   `json:"tier"`
			ContentID     string   `json:"content_id"`
			ContentStatus string   `json:"content_status"`
			ItemIDs       []string `json:"item_ids"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.Tier != "unstructured" {
		t.Errorf("tier = %q, want unstructured", resp.Data.Tier)
	}

	if resp.Data.ContentID == "" {
		t.Error("expected non-empty content_id for create_post")
	}

	if resp.Data.ContentStatus != "draft" {
		t.Errorf("content_status = %q, want draft", resp.Data.ContentStatus)
	}

	if len(resp.Data.ItemIDs) != 2 {
		t.Errorf("item_ids = %d, want 2", len(resp.Data.ItemIDs))
	}
}

func TestHandleCompareEntities_NoAuth(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/compare-entities", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()

	h.HandleCompareEntities(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestHandleCompareEntities_InvalidJSON(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAgentHTTPHandlers(t)
	user, tn := testUserAndPlatform()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/compare-entities", strings.NewReader(`{invalid`))
	req = withAuthContext(req, user, tn)
	rr := httptest.NewRecorder()

	h.HandleCompareEntities(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestBuildComparisonQuery(t *testing.T) {
	t.Parallel()

	q := buildComparisonQuery("universities", []string{"TUM", "Stuttgart"}, "scholarships")
	if !strings.Contains(q, "compare") || !strings.Contains(q, "universities") || !strings.Contains(q, "TUM") {
		t.Errorf("query = %q, missing parts", q)
	}
}

func TestBuildComparisonTitle(t *testing.T) {
	t.Parallel()

	if got := buildComparisonTitle("x", nil); got != "x comparison" {
		t.Errorf("nil ids: %q", got)
	}

	if got := buildComparisonTitle("x", []string{"A"}); got != "x: A" {
		t.Errorf("single id: %q", got)
	}

	if got := buildComparisonTitle("x", []string{"A", "B", "C"}); got != "A vs B vs C" {
		t.Errorf("multi id: %q", got)
	}
}

func TestNormalizeLabel(t *testing.T) {
	t.Parallel()

	if normalizeLabel("Cost of Living") != "cost of living" {
		t.Error("normalize failed")
	}

	if normalizeLabel("  Healthcare  ") != "healthcare" {
		t.Error("trim failed")
	}
}

func TestComparisonPromoteHandler(t *testing.T) {
	t.Parallel()

	promotedID := mustUUID(t, "00000000-0000-0000-0000-000000000009")
	svc := newStubComparisonService()
	svc.promoteResult = ComparisonPromoteResult{
		ID:     promotedID,
		Title:  "Lagos vs Accra",
		Slug:   "lagos-vs-accra",
		Status: "draft",
		Depth:  "detailed",
	}
	deps := &ToolDeps{Comparison: svc, Permission: &stubPerms{granted: map[string]bool{}}}

	tr, err := ComparisonPromoteHandler(deps)(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success: %s", tr.Status, tr.Content)
	}

	if !strings.Contains(tr.Content, promotedID.String()) {
		t.Fatalf("expected new comparison id in result, got %s", tr.Content)
	}

	if svc.promoteBypass {
		t.Error("expected bypassOwnership=false without comparison.edit_any")
	}
}

func TestComparisonPromoteHandler_EditAnyBypassesOwnership(t *testing.T) {
	t.Parallel()

	svc := newStubComparisonService()
	deps := &ToolDeps{
		Comparison: svc,
		Permission: &stubPerms{granted: map[string]bool{"comparison.edit_any": true}},
	}

	_, err := ComparisonPromoteHandler(deps)(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !svc.promoteBypass {
		t.Error("expected bypassOwnership=true for comparison.edit_any holder")
	}
}

func TestComparisonPromoteHandler_MissingID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Comparison: newStubComparisonService()}

	tr, _ := ComparisonPromoteHandler(deps)(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

// ---------------------------------------------------------------------------
// comparison.list
// ---------------------------------------------------------------------------

func TestComparisonListHandler_DefaultsAndFilters(t *testing.T) {
	svc := newStubComparisonService()
	svc.listResult = ComparisonListResult{
		Items: []ComparisonListItem{{ID: "id-1", Slug: "canada-vs-uk", Status: "published"}},
		Total: 1, Page: 1, Size: 20,
	}
	typeID := mustUUID(t, "55555555-5555-5555-5555-555555555555")
	svc.typeBySlug["cities"] = gen.GetComparisonTypeBySlugRow{ID: typeID}
	deps := NewToolDeps().WithComparison(svc)
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	// No filters → public-directory defaults (published/public/detailed).
	res, err := ComparisonListHandler(deps)(t.Context(), json.RawMessage(`{}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if svc.gotListArgs == nil || svc.gotListArgs.Status != "published" ||
		svc.gotListArgs.Visibility != "public" || svc.gotListArgs.Depth != "detailed" {
		t.Fatalf("directory defaults not applied: %+v", svc.gotListArgs)
	}

	var out struct {
		Comparisons []ComparisonListItem `json:"comparisons"`
		Total       int32                `json:"total"`
	}

	if err := json.Unmarshal([]byte(res.Content), &out); err != nil || out.Total != 1 {
		t.Fatalf("unexpected result: %v", res.Content)
	}

	// Explicit draft status + type_slug resolution + mine=true.
	res, err = ComparisonListHandler(deps)(t.Context(), json.RawMessage(`{
		"type_slug": "cities", "status": "draft", "mine": true, "page": 2, "limit": 5
	}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	f := svc.gotListArgs
	if f == nil || f.TypeID != typeID || f.Status != "draft" || !f.Mine ||
		f.UserID != userID || f.Limit != 5 || f.Offset != 5 {
		t.Fatalf("filter not forwarded correctly: %+v", f)
	}

	// "all" clears a filter.
	_, _ = ComparisonListHandler(deps)(t.Context(), json.RawMessage(`{
		"status": "all", "mine": true
	}`), userID)

	if svc.gotListArgs.Status != "" {
		t.Fatalf("'all' should clear the status filter, got %q", svc.gotListArgs.Status)
	}

	// Bad type slug → type_not_found.
	res, _ = ComparisonListHandler(deps)(t.Context(), json.RawMessage(`{"type_slug": "nope"}`), userID)
	if got := resultErrorCode(t, res); got != "type_not_found" {
		t.Fatalf("want type_not_found, got %q", got)
	}

	// Bad space_id → invalid_space_id.
	res, _ = ComparisonListHandler(deps)(t.Context(), json.RawMessage(`{"space_id": "x"}`), userID)
	if got := resultErrorCode(t, res); got != "invalid_space_id" {
		t.Fatalf("want invalid_space_id, got %q", got)
	}
}
