// Package ai — presearch_test.go covers the Phase 5 pre-search step
// (Section 103, ADR-111).
//
// Tests verify:
//   - Classification into full / partial / none based on RAG similarity
//     and the agent.presearch_full_threshold setting.
//   - ai_exempt content is excluded from both content search and RAG.
//   - Graceful degradation when deps are nil.
//   - Geo-scoped vs tenant-wide search dispatch.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Pre-Search Behavior)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 5, tasks 5.01-5.03)
package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Classification tests
// ---------------------------------------------------------------------------

func TestPreSearch_FullClassification(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "00000000-0000-0000-0000-000000000001", EntityType: "content", ChunkText: "TUM scholarships", Similarity: 0.92},
			},
		},
		Settings: &stubSettingsReader{},
	}

	ps := NewPreSearcher(deps)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "scholarships", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Level != PreSearchFull {
		t.Errorf("level = %q, want %q (similarity 0.92 >= threshold 0.85)", result.Level, PreSearchFull)
	}

	if result.BestScore != 0.92 {
		t.Errorf("best_score = %.2f, want 0.92", result.BestScore)
	}
}

func TestPreSearch_PartialClassification(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "00000000-0000-0000-0000-000000000001", EntityType: "content", ChunkText: "some info", Similarity: 0.50},
			},
		},
		Settings: &stubSettingsReader{},
	}

	ps := NewPreSearcher(deps)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Level != PreSearchPartial {
		t.Errorf("level = %q, want %q (similarity 0.50 < threshold 0.85)", result.Level, PreSearchPartial)
	}
}

func TestPreSearch_NoneClassification(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		RAG:      &stubRAGService{chunks: nil},
		Settings: &stubSettingsReader{},
	}

	ps := NewPreSearcher(deps)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Level != PreSearchNone {
		t.Errorf("level = %q, want %q", result.Level, PreSearchNone)
	}

	if result.HasResults() {
		t.Error("expected no results")
	}
}

func TestPreSearch_CustomThreshold(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "00000000-0000-0000-0000-000000000001", EntityType: "content", ChunkText: "info", Similarity: 0.70},
			},
		},
		Settings: &thresholdSettingsReader{threshold: 0.60},
	}

	ps := NewPreSearcher(deps)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 0.70 >= 0.60 threshold → FULL
	if result.Level != PreSearchFull {
		t.Errorf("level = %q, want %q (0.70 >= custom threshold 0.60)", result.Level, PreSearchFull)
	}
}

// ---------------------------------------------------------------------------
// Privacy filtering tests
// ---------------------------------------------------------------------------

func TestPreSearch_PrivacyFiltersExemptContent(t *testing.T) {
	t.Parallel()

	contentID1 := "00000000-0000-0000-0000-000000000001"
	contentID2 := "00000000-0000-0000-0000-000000000002"

	deps := &ToolDeps{
		Content: &stubContentService{
			searchResults: []gen.ListContentsByTenantRow{
				{ID: mustUUID(t, contentID1), Title: "Exempt content", Slug: "exempt", ContentType: "guide"},
				{ID: mustUUID(t, contentID2), Title: "Public content", Slug: "public", ContentType: "guide"},
			},
		},
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: contentID1, EntityType: "content", ChunkText: "exempt chunk", Similarity: 0.90},
				{EntityID: contentID2, EntityType: "content", ChunkText: "public chunk", Similarity: 0.80},
			},
		},
		Privacy: &selectivePrivacyChecker{
			exemptIDs: map[string]bool{contentID1: true},
		},
		Settings: &stubSettingsReader{},
	}

	ps := NewPreSearcher(deps)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The exempt content should be filtered from both content and RAG.
	for _, c := range result.Content {
		if c.ID == contentID1 {
			t.Error("exempt content should not appear in pre-search content results")
		}
	}

	for _, ch := range result.RAGChunks {
		if ch.EntityID == contentID1 {
			t.Error("exempt RAG chunk should not appear in pre-search results")
		}
	}

	// Best score should be from the non-exempt chunk (0.80, not 0.90).
	if result.BestScore != 0.80 {
		t.Errorf("best_score = %.2f, want 0.80 (exempt 0.90 chunk filtered)", result.BestScore)
	}

	// With 0.80 < 0.85 threshold → PARTIAL (not FULL, because the 0.90 was exempt).
	if result.Level != PreSearchPartial {
		t.Errorf("level = %q, want %q (best non-exempt 0.80 < 0.85)", result.Level, PreSearchPartial)
	}
}

func TestPreSearch_NoPrivacyChecker(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Content: &stubContentService{
			searchResults: []gen.ListContentsByTenantRow{
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Title: "Content", Slug: "c", ContentType: "guide"},
			},
		},
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "00000000-0000-0000-0000-000000000001", EntityType: "content", ChunkText: "chunk", Similarity: 0.90},
			},
		},
		// Privacy is nil — no filtering
		Settings: &stubSettingsReader{},
	}

	ps := NewPreSearcher(deps)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Content) != 1 {
		t.Errorf("expected 1 content item (no privacy filter), got %d", len(result.Content))
	}

	if len(result.RAGChunks) != 1 {
		t.Errorf("expected 1 RAG chunk (no privacy filter), got %d", len(result.RAGChunks))
	}
}

// ---------------------------------------------------------------------------
// Geo-scoped search tests
// ---------------------------------------------------------------------------

func TestPreSearch_GeoScopedSearch(t *testing.T) {
	t.Parallel()

	geoID := mustUUID(t, "00000000-0000-0000-0000-000000000010")

	deps := &ToolDeps{
		Content: &stubContentService{
			spaceSearchResults: []gen.SearchContentsBySpaceRow{
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Title: "Geo content", Slug: "geo", ContentType: "guide"},
			},
		},
		RAG: &stubRAGService{
			chunks: []RAGChunk{
				{EntityID: "00000000-0000-0000-0000-000000000001", EntityType: "content", ChunkText: "geo chunk", Similarity: 0.90},
			},
		},
		Settings: &stubSettingsReader{},
	}

	ps := NewPreSearcher(deps)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "query", &geoID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Content) != 1 {
		t.Errorf("expected 1 geo-scoped content item, got %d", len(result.Content))
	}

	if result.Content[0].Title != "Geo content" {
		t.Errorf("content title = %q, want %q", result.Content[0].Title, "Geo content")
	}
}

// ---------------------------------------------------------------------------
// Graceful degradation tests
// ---------------------------------------------------------------------------

func TestPreSearch_NilDeps(t *testing.T) {
	t.Parallel()

	ps := NewPreSearcher(nil)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Level != PreSearchNone {
		t.Errorf("level = %q, want %q", result.Level, PreSearchNone)
	}
}

func TestPreSearch_EmptyPrompt(t *testing.T) {
	t.Parallel()

	ps := NewPreSearcher(&ToolDeps{})

	_, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "", nil)
	if err == nil {
		t.Fatal("expected error for empty prompt, got nil")
	}
}

func TestPreSearch_RAGFailureNonFatal(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		Content: &stubContentService{
			searchResults: []gen.ListContentsByTenantRow{
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Title: "Content", Slug: "c", ContentType: "guide"},
			},
		},
		RAG:      &stubRAGService{err: errFakeRAG},
		Settings: &stubSettingsReader{},
	}

	ps := NewPreSearcher(deps)

	result, err := ps.PreSearch(t.Context(), pgtype.UUID{}, "query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// RAG failed but content search succeeded → PARTIAL (content exists, no RAG score).
	if result.Level != PreSearchPartial {
		t.Errorf("level = %q, want %q (content found, RAG failed)", result.Level, PreSearchPartial)
	}

	if len(result.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(result.Content))
	}

	if len(result.RAGChunks) != 0 {
		t.Errorf("expected 0 RAG chunks (RAG failed), got %d", len(result.RAGChunks))
	}
}

// ---------------------------------------------------------------------------
// Summary tests
// ---------------------------------------------------------------------------

func TestPreSearchResult_Summary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		result  PreSearchResult
		wantHas string
	}{
		{
			name:    "full",
			result:  PreSearchResult{Level: PreSearchFull, BestScore: 0.92, Content: []PreSearchContentItem{{Title: "Test"}}},
			wantHas: "FULL coverage",
		},
		{
			name:    "partial",
			result:  PreSearchResult{Level: PreSearchPartial, BestScore: 0.50, Content: []PreSearchContentItem{{Title: "Test"}}},
			wantHas: "PARTIAL coverage",
		},
		{
			name:    "none",
			result:  PreSearchResult{Level: PreSearchNone},
			wantHas: "NONE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.result.Summary()
			if !strings.Contains(summary, tt.wantHas) {
				t.Errorf("summary does not contain %q: %s", tt.wantHas, summary)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test stubs specific to presearch
// ---------------------------------------------------------------------------

var errFakeRAG = errFake("rag retrieval failed")

type errFake string

func (e errFake) Error() string { return string(e) }

// thresholdSettingsReader returns a custom float for the presearch threshold.
type thresholdSettingsReader struct {
	threshold float64
}

func (s *thresholdSettingsReader) GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string {
	return fallback
}

func (s *thresholdSettingsReader) GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int {
	return fallback
}

func (s *thresholdSettingsReader) GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64 {
	if key == "agent.presearch_full_threshold" {
		return s.threshold
	}

	return fallback
}

func (s *thresholdSettingsReader) GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool {
	return fallback
}
