// Package ai — tools_content_test.go covers the Phase 4 content tool
// handlers (Section 103, ADR-111).
//
// The handlers are tested with stub services that implement the
// ContentService, CustomFieldsService, and PrivacyExemptionChecker
// interfaces. No live database is required.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubContentService implements ContentService for testing.
type stubContentService struct {
	searchResults      []gen.ListContentsByTenantRow
	searchErr          error
	spaceSearchResults []gen.SearchContentsBySpaceRow
	spaceSearchErr     error
	content            gen.GetContentByIDRow
	getContentErr      error
}

func (s *stubContentService) ListContents(ctx context.Context, tenantID pgtype.UUID, q string, cursor pgtype.Timestamptz, limit int32, filter visibility.VisibilityFilter) ([]gen.ListContentsByTenantRow, error) {
	return s.searchResults, s.searchErr
}

func (s *stubContentService) SearchContentsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID, query string, limit, offset int32, filter visibility.VisibilityFilter) ([]gen.SearchContentsBySpaceRow, error) {
	return s.spaceSearchResults, s.spaceSearchErr
}

func (s *stubContentService) GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error) {
	return s.content, s.getContentErr
}

// stubCustomFieldsService implements CustomFieldsService for testing.
type stubCustomFieldsService struct {
	fields    []gen.ListCustomFieldsByGroupRow
	fieldsErr error
	cfvs      []gen.CustomFieldValue
	cfvsErr   error
	upserted  []gen.UpsertCustomFieldValueParams
	upsertErr error
}

func (s *stubCustomFieldsService) ListFieldsByGroup(ctx context.Context, groupID, tenantID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error) {
	return s.fields, s.fieldsErr
}

func (s *stubCustomFieldsService) ListFieldValuesByEntity(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) ([]gen.CustomFieldValue, error) {
	return s.cfvs, s.cfvsErr
}

func (s *stubCustomFieldsService) UpsertFieldValue(ctx context.Context, params gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error) {
	s.upserted = append(s.upserted, params)
	if s.upsertErr != nil {
		return gen.CustomFieldValue{}, s.upsertErr
	}
	return gen.CustomFieldValue{
		FieldID:    params.FieldID,
		EntityType: params.EntityType,
		EntityID:   params.EntityID,
	}, nil
}

// stubPrivacyChecker implements PrivacyExemptionChecker for testing.
type stubPrivacyChecker struct {
	exempt bool
	err    error
}

func (s *stubPrivacyChecker) IsExempt(ctx context.Context, entityType PrivacyEntityType, tenantID, entityID pgtype.UUID) (bool, error) {
	return s.exempt, s.err
}

// selectivePrivacyChecker marks specific entity IDs as exempt.
type selectivePrivacyChecker struct {
	exemptIDs map[string]bool
}

func (s *selectivePrivacyChecker) IsExempt(ctx context.Context, entityType PrivacyEntityType, tenantID, entityID pgtype.UUID) (bool, error) {
	if !entityID.Valid {
		return false, nil
	}
	return s.exemptIDs[entityID.String()], nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("scanning uuid %q: %v", s, err)
	}
	return u
}

func parseToolResult(t *testing.T, tr *ToolResult) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(tr.Content), &m); err != nil {
		t.Fatalf("parsing tool result content: %v", err)
	}
	return m
}

// ---------------------------------------------------------------------------
// content.search (task 4.01)
// ---------------------------------------------------------------------------

func TestContentSearchHandler_TenantWide(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Content: &stubContentService{
			searchResults: []gen.ListContentsByTenantRow{
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Title: "TUM Guide", Slug: "tum-guide", ContentType: "article", Status: "published"},
			},
		},
	}

	handler := ContentSearchHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"TUM"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["geo_scoped"] != false {
		t.Error("expected geo_scoped = false")
	}
	results, _ := m["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestContentSearchHandler_GeoScoped(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Content: &stubContentService{
			spaceSearchResults: []gen.SearchContentsBySpaceRow{
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000002"), Title: "Berlin Guide", Slug: "berlin-guide", ContentType: "article", Status: "published"},
			},
		},
	}

	handler := ContentSearchHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"Berlin","geo_space_id":"00000000-0000-0000-0000-000000000003"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["geo_scoped"] != true {
		t.Error("expected geo_scoped = true")
	}
}

func TestContentSearchHandler_PrivacyFiltersExempt(t *testing.T) {
	t.Parallel()

	// The stub returns 2 results; the privacy checker marks the first as
	// exempt. The handler should filter it out.
	deps := &ToolDeps{
		Content: &stubContentService{
			searchResults: []gen.ListContentsByTenantRow{
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Title: "Exempt", Slug: "exempt", ContentType: "article", Status: "published"},
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000002"), Title: "Public", Slug: "public", ContentType: "article", Status: "published"},
			},
		},
		Privacy: &stubPrivacyChecker{
			exempt: true, // marks ALL content as exempt in this stub
		},
	}

	// Use a privacy checker that only marks the first content as exempt.
	deps.Privacy = &selectivePrivacyChecker{
		exemptIDs: map[string]bool{
			"00000000-0000-0000-0000-000000000001": true,
		},
	}

	handler := ContentSearchHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"test"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	results, _ := m["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected 1 result after privacy filter, got %d", len(results))
	}
}

func TestContentSearchHandler_NoPrivacyChecker(t *testing.T) {
	t.Parallel()

	// When Privacy is nil, no filtering is applied — all results pass through.
	deps := &ToolDeps{
		Content: &stubContentService{
			searchResults: []gen.ListContentsByTenantRow{
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Title: "A", Slug: "a", ContentType: "article", Status: "published"},
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000002"), Title: "B", Slug: "b", ContentType: "article", Status: "published"},
			},
		},
		// Privacy is nil — no filtering
	}

	handler := ContentSearchHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"test"}`), pgtype.UUID{}, pgtype.UUID{})
	m := parseToolResult(t, tr)
	results, _ := m["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("expected 2 results (no privacy filter), got %d", len(results))
	}
}

func TestContentSearchHandler_MissingQuery(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Content: &stubContentService{}}
	handler := ContentSearchHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestContentSearchHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := ContentSearchHandler(nil)
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"x"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

// ---------------------------------------------------------------------------
// content.read (task 4.02)
// ---------------------------------------------------------------------------

func TestContentReadHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	deps := &ToolDeps{
		Content: &stubContentService{
			content: gen.GetContentByIDRow{
				ID:          contentID,
				Title:       "Test",
				Slug:        "test",
				ContentType: "article",
				Body:        "body text",
				Status:      "published",
			},
		},
		CustomFields: &stubCustomFieldsService{
			cfvs: []gen.CustomFieldValue{
				{FieldID: mustUUID(t, "00000000-0000-0000-0000-000000000010"), EntityType: "content", EntityID: contentID, ValueText: strPtr("value1")},
				{FieldID: mustUUID(t, "00000000-0000-0000-0000-000000000011"), EntityType: "content", EntityID: contentID, ValueText: strPtr("exempt_val"), AiExempt: true},
			},
		},
	}

	handler := ContentReadHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	fvs, _ := m["field_values"].([]any)
	if len(fvs) != 1 {
		t.Fatalf("expected 1 non-exempt field value, got %d", len(fvs))
	}
}

func TestContentReadHandler_NotFound(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Content: &stubContentService{
			getContentErr: errors.New("not found"),
		},
	}

	handler := ContentReadHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestContentReadHandler_InvalidUUID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Content: &stubContentService{}}
	handler := ContentReadHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"content_id":"not-a-uuid"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestContentReadPrivacyTarget(t *testing.T) {
	t.Parallel()

	et, id, err := ContentReadPrivacyTarget(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if et != PrivacyEntityContent {
		t.Errorf("entity type = %q, want %q", et, PrivacyEntityContent)
	}
	if !id.Valid {
		t.Error("expected valid UUID")
	}
}

// ---------------------------------------------------------------------------
// content.create (task 4.03)
// ---------------------------------------------------------------------------

func TestContentCreateHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	userID := mustUUID(t, "00000000-0000-0000-0000-000000000099")
	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, tenantID pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			// Verify _ai_provenance was injected into field_values.
			var fv map[string]any
			if err := json.Unmarshal(input.FieldValues, &fv); err != nil {
				t.Fatalf("field values not valid JSON: %v", err)
			}
			prov, ok := fv["_ai_provenance"]
			if !ok {
				t.Fatal("expected _ai_provenance in field_values")
			}
			provMap, ok := prov.(map[string]any)
			if !ok {
				t.Fatal("_ai_provenance is not an object")
			}
			if provMap["ai_generated"] != true {
				t.Error("expected ai_generated = true")
			}
			if provMap["prompted_by_user_id"] != userID.String() {
				t.Errorf("expected prompted_by_user_id = %s, got %v", userID.String(), provMap["prompted_by_user_id"])
			}
			if input.Status != "draft" {
				t.Errorf("expected status = draft, got %s", input.Status)
			}
			return ContentCreateRow{ID: contentID, Title: input.Title, Slug: "test-slug", Status: "draft", ContentType: input.ContentType, AuthorID: input.AuthorID, CreatedAt: pgtype.Timestamptz{}}, nil
		},
	}

	handler := ContentCreateHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"title":"Test Article","content_type":"article","body":"body"}`), userID, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["content_id"] != contentID.String() {
		t.Errorf("content_id = %v, want %s", m["content_id"], contentID.String())
	}
}

func TestContentCreateHandler_MissingTitle(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, tenantID pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			t.Fatal("creator should not be called for invalid input")
			return ContentCreateRow{}, nil
		},
	}
	handler := ContentCreateHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"content_type":"article"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestContentCreateHandler_NoCreatorWired(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{Content: &stubContentService{}}
	handler := ContentCreateHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"title":"T","content_type":"article"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

// ---------------------------------------------------------------------------
// content.update (task 4.04)
// ---------------------------------------------------------------------------

func TestContentUpdateHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	userID := mustUUID(t, "00000000-0000-0000-0000-000000000099")
	deps := &ToolDeps{
		Content: &stubContentService{content: gen.GetContentByIDRow{ID: contentID}},
		ContentUpdater: func(ctx context.Context, id, tid, editedBy pgtype.UUID, input ContentUpdateInput) (ContentUpdateRow, error) {
			if input.Title == nil || *input.Title != "New Title" {
				t.Error("expected title = New Title")
			}
			return ContentUpdateRow{ID: id, Title: *input.Title, Slug: "test", Status: "draft"}, nil
		},
	}

	handler := ContentUpdateHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","title":"New Title"}`), userID, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
}

func TestContentUpdateHandler_PrivacyBlocked(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	deps := &ToolDeps{
		Content: &stubContentService{content: gen.GetContentByIDRow{ID: contentID}},
		Privacy: &stubPrivacyChecker{exempt: true},
		ContentUpdater: func(ctx context.Context, id, tid, editedBy pgtype.UUID, input ContentUpdateInput) (ContentUpdateRow, error) {
			t.Fatal("updater should not be called for exempt content")
			return ContentUpdateRow{}, nil
		},
	}

	handler := ContentUpdateHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","title":"New"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusPrivacyBlocked {
		t.Fatalf("status = %q, want privacy_blocked", tr.Status)
	}
}

func TestContentUpdatePrivacyTarget(t *testing.T) {
	t.Parallel()

	et, id, err := ContentUpdatePrivacyTarget(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if et != PrivacyEntityContent {
		t.Errorf("entity type = %q, want %q", et, PrivacyEntityContent)
	}
	if !id.Valid {
		t.Error("expected valid UUID")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// buildProvenance
// ---------------------------------------------------------------------------

func TestBuildProvenance(t *testing.T) {
	t.Parallel()

	userID := mustUUID(t, "00000000-0000-0000-0000-000000000099")

	t.Run("nil provenance args uses defaults", func(t *testing.T) {
		t.Parallel()
		p := buildProvenance(nil, userID)
		if p["ai_generated"] != true {
			t.Error("expected ai_generated = true")
		}
		if p["prompted_by_user_id"] != userID.String() {
			t.Errorf("expected prompted_by_user_id = %s", userID.String())
		}
	})

	t.Run("with persona and prompt", func(t *testing.T) {
		t.Parallel()
		p := buildProvenance(&aiProvenanceArgs{
			AIPersonaID:    "00000000-0000-0000-0000-000000000050",
			OriginalPrompt: "Compare TUM and Stuttgart",
		}, userID)
		if p["ai_persona_id"] != "00000000-0000-0000-0000-000000000050" {
			t.Errorf("unexpected persona id: %v", p["ai_persona_id"])
		}
		if p["original_prompt"] != "Compare TUM and Stuttgart" {
			t.Errorf("unexpected prompt: %v", p["original_prompt"])
		}
	})
}

// ---------------------------------------------------------------------------
// filterNonExemptFieldValues
// ---------------------------------------------------------------------------

func TestFilterNonExemptFieldValues(t *testing.T) {
	t.Parallel()

	cfvs := []gen.CustomFieldValue{
		{FieldID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), ValueText: strPtr("public"), AiExempt: false},
		{FieldID: mustUUID(t, "00000000-0000-0000-0000-000000000002"), ValueText: strPtr("private"), AiExempt: true},
		{FieldID: mustUUID(t, "00000000-0000-0000-0000-000000000003"), ValueText: strPtr("also_public"), AiExempt: false},
	}

	out := filterNonExemptFieldValues(cfvs)
	if len(out) != 2 {
		t.Fatalf("expected 2 non-exempt values, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Shared helpers used by all tool test files
// ---------------------------------------------------------------------------

// stubSettingsReader implements SettingsReader for testing.
type stubSettingsReader struct {
	stringVal string
	intVal    int
}

func (s *stubSettingsReader) GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string {
	if s.stringVal != "" {
		return s.stringVal
	}
	return fallback
}

func (s *stubSettingsReader) GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int {
	if s.intVal != 0 {
		return s.intVal
	}
	return fallback
}

func (s *stubSettingsReader) GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64 {
	return fallback
}

func (s *stubSettingsReader) GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool {
	return fallback
}

// stubCacheClient implements CacheClient for testing.
type stubCacheClient struct {
	store map[string]string
	hit   bool
}

func newStubCacheClient() *stubCacheClient {
	return &stubCacheClient{store: make(map[string]string)}
}

func (c *stubCacheClient) Get(ctx context.Context, key string) (string, error) {
	if v, ok := c.store[key]; ok {
		c.hit = true
		return v, nil
	}
	c.hit = false
	return "", errors.New("not found")
}

func (c *stubCacheClient) Set(ctx context.Context, key, value string, ttlSeconds int) error {
	c.store[key] = value
	return nil
}

// stubRAGService implements RAGService for testing.
type stubRAGService struct {
	chunks []RAGChunk
	err    error
}

func (s *stubRAGService) Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string, geoSpaceID *pgtype.UUID) ([]RAGChunk, error) {
	return s.chunks, s.err
}

// ensure errors is used (stubs reference it)
var _ = errors.New
