// Package ai — tools_knowledge_test.go covers the Phase 4 knowledge tool
// handlers (rag.search + external.search) (Section 103, ADR-111).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4)
package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// rag.search (task 4.09)
// ---------------------------------------------------------------------------

func TestRAGSearchHandler_Success(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "00000000-0000-0000-0000-000000000001", EntityType: "content", ChunkText: "TUM scholarships...", Similarity: 0.92},
				{EntityID: "00000000-0000-0000-0000-000000000002", EntityType: "content", ChunkText: "Stuttgart funding...", Similarity: 0.85},
			},
		},
	}

	handler := RAGSearchHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"scholarships"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	results, _ := m["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if m["geo_scoped"] != false {
		t.Error("expected geo_scoped = false")
	}
}

func TestRAGSearchHandler_GeoFiltered(t *testing.T) {
	t.Parallel()

	var capturedGeo *pgtype.UUID
	deps := &ToolDeps{
		RAG: &stubRAGService{
			chunks: []RAGChunk{{EntityID: "x", EntityType: "content", ChunkText: "geo result"}},
		},
	}

	// Wrap to capture the geoSpaceID.
	deps.RAG = &ragCapture{geo: capturedGeo, chunks: []RAGChunk{{EntityID: "x", EntityType: "content", ChunkText: "geo result"}}}

	handler := RAGSearchHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"x","geo_space_id":"00000000-0000-0000-0000-000000000005"}`), pgtype.UUID{}, pgtype.UUID{})
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

type ragCapture struct {
	geo    *pgtype.UUID
	chunks []RAGChunk
}

func (r *ragCapture) Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string, geoSpaceID *pgtype.UUID) ([]RAGChunk, error) {
	r.geo = geoSpaceID
	return r.chunks, nil
}

func TestRAGSearchHandler_MissingQuery(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{RAG: &stubRAGService{}}
	handler := RAGSearchHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestRAGSearchHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := RAGSearchHandler(nil)
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"x"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestRAGSearchHandler_Limit(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "1", ChunkText: "a"},
				{EntityID: "2", ChunkText: "b"},
				{EntityID: "3", ChunkText: "c"},
				{EntityID: "4", ChunkText: "d"},
			},
		},
	}

	handler := RAGSearchHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"x","limit":2}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := parseToolResult(t, tr)
	results, _ := m["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("expected 2 results after limit, got %d", len(results))
	}
}

func TestRAGSearchHandler_PrivacyFiltersExempt(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "00000000-0000-0000-0000-000000000001", EntityType: "content", ChunkText: "exempt chunk"},
				{EntityID: "00000000-0000-0000-0000-000000000002", EntityType: "content", ChunkText: "public chunk"},
				{EntityID: "00000000-0000-0000-0000-000000000003", EntityType: "user", ChunkText: "user chunk (not filtered)"},
			},
		},
		Privacy: &selectivePrivacyChecker{
			exemptIDs: map[string]bool{
				"00000000-0000-0000-0000-000000000001": true,
			},
		},
	}

	handler := RAGSearchHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"x"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	results, _ := m["results"].([]any)
	// Should be 2: the exempt content chunk is filtered, the user chunk is
	// not checked (entity_type != "content"), and the public content chunk
	// passes.
	if len(results) != 2 {
		t.Fatalf("expected 2 results after privacy filter, got %d", len(results))
	}
}

func TestRAGSearchHandler_NoPrivacyChecker(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "00000000-0000-0000-0000-000000000001", EntityType: "content", ChunkText: "a"},
				{EntityID: "00000000-0000-0000-0000-000000000002", EntityType: "content", ChunkText: "b"},
			},
		},
		// Privacy is nil — no filtering
	}

	handler := RAGSearchHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"x"}`), pgtype.UUID{}, pgtype.UUID{})
	m := parseToolResult(t, tr)
	results, _ := m["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("expected 2 results (no privacy filter), got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// external.search (task 4.10)
// ---------------------------------------------------------------------------

func TestExternalSearchHandler_NoAPIKey(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Settings: &stubSettingsReader{stringVal: ""}, // empty API key
	}
	handler := ExternalSearchHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"test"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["code"] != "external_search_unavailable" {
		t.Errorf("expected code = external_search_unavailable, got %v", m["code"])
	}
}

func TestExternalSearchHandler_NoSettings(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{}
	handler := ExternalSearchHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"test"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestExternalSearchHandler_MissingQuery(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Settings: &stubSettingsReader{stringVal: "test-key"},
	}
	handler := ExternalSearchHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestExternalSearchHandler_UnknownProvider(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Settings: &stubSettingsReader{stringVal: "test-key"},
	}
	// Override the provider by using a settings reader that returns
	// "unknown" for the provider key. The stub returns the same string
	// for all keys, so we need a custom settings reader.
	deps.Settings = &providerSettingsReader{provider: "unknown", apiKey: "test-key"}

	handler := ExternalSearchHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"test"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["code"] != "unknown_provider" {
		t.Errorf("expected code = unknown_provider, got %v", m["code"])
	}
}

func TestExternalSearchHandler_CacheHit(t *testing.T) {
	t.Parallel()

	cache := newStubCacheClient()
	cachedJSON := `{"query":"test","provider":"tavily","results":[{"title":"Cached","url":"https://example.com","content":"cached"}],"total":1,"cached":false}`
	// Use the real computed cache key so the handler finds the cached entry.
	cacheKey := externalSearchCacheKey("test", 5)
	cache.store[cacheKey] = cachedJSON

	deps := &ToolDeps{
		Settings: &providerSettingsReader{provider: "tavily", apiKey: "test-key"},
		Cache:    cache,
	}

	handler := ExternalSearchHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"test"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if tr.Content != cachedJSON {
		t.Error("expected cached content to be returned directly")
	}
}

func TestExternalSearchHandler_TavilyLive(t *testing.T) {
	t.Parallel()

	// searchTavily and searchBrave hardcode their provider API URLs, so
	// they cannot be pointed at a httptest server without refactoring. The
	// handler-level tests above cover the dispatch + caching + error paths.
	// The provider functions are exercised in integration tests against the
	// real APIs. This test is a no-op smoke test retained for documentation.
	skip := true
	if skip {
		t.Skip("searchTavily/searchBrave hit real provider APIs; covered by integration tests")
	}
}

// ---------------------------------------------------------------------------
// externalSearchCacheKey
// ---------------------------------------------------------------------------

func TestExternalSearchCacheKey(t *testing.T) {
	t.Parallel()

	key1 := externalSearchCacheKey("scholarships", 5)
	key2 := externalSearchCacheKey("scholarships", 5)
	key3 := externalSearchCacheKey("scholarships", 10)

	if key1 != key2 {
		t.Error("same query+limit should produce same key")
	}
	if key1 == key3 {
		t.Error("different limit should produce different key")
	}
	if !strings.HasPrefix(key1, "jc:agent:external:") {
		t.Errorf("key should start with jc:agent:external:, got %s", key1)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// providerSettingsReader returns specific values for provider and API key.
type providerSettingsReader struct {
	provider string
	apiKey   string
}

func (s *providerSettingsReader) GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string {
	switch key {
	case "agent.external_search_provider":
		if s.provider != "" {
			return s.provider
		}
		return fallback
	case "agent.external_search_api_key":
		return s.apiKey
	default:
		return fallback
	}
}

func (s *providerSettingsReader) GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int {
	return fallback
}

func (s *providerSettingsReader) GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64 {
	return fallback
}

func (s *providerSettingsReader) GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool {
	return fallback
}
