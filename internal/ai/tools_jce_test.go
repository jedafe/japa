// Package ai — tools_jce_test.go covers the JCE Sub-spec 4 AI advisor and
// refresh tool handlers.
//
// The handlers are tested with stub services that implement the JCEService
// and PermissionChecker interfaces. No live database is required.
//
// Ref: docs/superpowers/plans/2026-08-30-jce-subspec-4.md (Task 8)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubJCEService implements JCEService for testing.
type stubJCEService struct {
	result map[string]any
	err    error

	analyzeCalled   bool
	rankCalled      bool
	explainCalled   bool
	recommendCalled bool
	refreshCalled   bool

	lastContentID  pgtype.UUID
	lastProfileID  pgtype.UUID
	lastItemID     pgtype.UUID
	lastFactorKey  string
	lastQuestion   string
	lastSourceKind *string
}

func (s *stubJCEService) Analyze(ctx context.Context, contentID, userID, profileID pgtype.UUID, question string) (map[string]any, error) {
	s.analyzeCalled = true
	s.lastContentID = contentID
	s.lastProfileID = profileID
	s.lastQuestion = question

	return s.result, s.err
}

func (s *stubJCEService) Rank(ctx context.Context, contentID, userID, profileID pgtype.UUID) (map[string]any, error) {
	s.rankCalled = true
	s.lastContentID = contentID
	s.lastProfileID = profileID

	return s.result, s.err
}

func (s *stubJCEService) Explain(ctx context.Context, contentID, itemID, userID, profileID pgtype.UUID, factorKey, question string) (map[string]any, error) {
	s.explainCalled = true
	s.lastContentID = contentID
	s.lastItemID = itemID
	s.lastProfileID = profileID
	s.lastFactorKey = factorKey
	s.lastQuestion = question

	return s.result, s.err
}

func (s *stubJCEService) Recommend(ctx context.Context, contentID, userID, profileID pgtype.UUID) (map[string]any, error) {
	s.recommendCalled = true
	s.lastContentID = contentID
	s.lastProfileID = profileID

	return s.result, s.err
}

func (s *stubJCEService) Refresh(ctx context.Context, contentID, tenantID, userID pgtype.UUID, sourceKind *string) (map[string]any, error) {
	s.refreshCalled = true
	s.lastContentID = contentID
	s.lastSourceKind = sourceKind

	return s.result, s.err
}

// stubJCEPermissionChecker implements PermissionChecker for testing.
type stubJCEPermissionChecker struct {
	allowed map[string]bool
}

func (s *stubJCEPermissionChecker) HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool {
	return s.allowed[permKey]
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func TestJCEAnalyzeHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	svc := &stubJCEService{result: map[string]any{"summary": "TUM wins"}}
	deps := &ToolDeps{JCE: svc}

	handler := JCEAnalyzeHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","question":"which is best?"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["summary"] != "TUM wins" {
		t.Errorf("summary = %v, want %q", m["summary"], "TUM wins")
	}

	if svc.lastContentID != contentID {
		t.Errorf("content_id mismatch")
	}

	if svc.lastQuestion != "which is best?" {
		t.Errorf("question = %q, want %q", svc.lastQuestion, "which is best?")
	}
}

func TestJCERankHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	svc := &stubJCEService{result: map[string]any{"ranking": []any{"a", "b"}}}
	deps := &ToolDeps{JCE: svc}

	handler := JCERankHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)

	ranking, _ := m["ranking"].([]any)
	if len(ranking) != 2 {
		t.Errorf("ranking length = %d, want 2", len(ranking))
	}

	if svc.lastContentID != contentID {
		t.Errorf("content_id mismatch")
	}
}

func TestJCEExplainHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	itemID := mustUUID(t, "00000000-0000-0000-0000-000000000002")
	svc := &stubJCEService{result: map[string]any{"explanation": "low cost"}}
	deps := &ToolDeps{JCE: svc}

	handler := JCEExplainHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","item_id":"00000000-0000-0000-0000-000000000002","factor_key":"cost","question":"why?"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["explanation"] != "low cost" {
		t.Errorf("explanation = %v, want %q", m["explanation"], "low cost")
	}

	if svc.lastContentID != contentID {
		t.Errorf("content_id mismatch")
	}

	if svc.lastItemID != itemID {
		t.Errorf("item_id mismatch")
	}

	if svc.lastFactorKey != "cost" {
		t.Errorf("factor_key = %q, want %q", svc.lastFactorKey, "cost")
	}
}

func TestJCERecommendHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	svc := &stubJCEService{result: map[string]any{"next_actions": []any{"apply"}}}
	deps := &ToolDeps{JCE: svc}

	handler := JCERecommendHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)

	actions, _ := m["next_actions"].([]any)
	if len(actions) != 1 {
		t.Errorf("actions length = %d, want 1", len(actions))
	}

	if svc.lastContentID != contentID {
		t.Errorf("content_id mismatch")
	}
}

func TestJCERefreshHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	svc := &stubJCEService{result: map[string]any{"status": "pending"}}
	deps := &ToolDeps{
		JCE:        svc,
		Permission: &stubJCEPermissionChecker{allowed: map[string]bool{"jce.refresh.trigger": true}},
	}

	handler := JCERefreshHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","source_kind":"official"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["status"] != "pending" {
		t.Errorf("status = %v, want %q", m["status"], "pending")
	}

	if svc.lastContentID != contentID {
		t.Errorf("content_id mismatch")
	}

	if svc.lastSourceKind == nil || *svc.lastSourceKind != "official" {
		t.Errorf("source_kind = %v, want %q", svc.lastSourceKind, "official")
	}
}

func TestJCERefreshHandler_WithoutSourceKind(t *testing.T) {
	t.Parallel()

	svc := &stubJCEService{result: map[string]any{"status": "pending"}}
	deps := &ToolDeps{
		JCE:        svc,
		Permission: &stubJCEPermissionChecker{allowed: map[string]bool{"jce.refresh.trigger": true}},
	}

	handler := JCERefreshHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	if svc.lastSourceKind != nil {
		t.Errorf("source_kind = %v, want nil", svc.lastSourceKind)
	}
}

func TestJCERefreshHandler_Unauthorized(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		JCE:        &stubJCEService{result: map[string]any{"status": "pending"}},
		Permission: &stubJCEPermissionChecker{allowed: map[string]bool{}},
	}

	handler := JCERefreshHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusUnauthorized {
		t.Fatalf("status = %q, want unauthorized", tr.Status)
	}
}

func TestJCERefreshHandler_NoPermissionChecker(t *testing.T) {
	t.Parallel()

	svc := &stubJCEService{result: map[string]any{"status": "pending"}}
	deps := &ToolDeps{JCE: svc}

	handler := JCERefreshHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
}

func TestJCEHandlers_NoJCE(t *testing.T) {
	t.Parallel()

	handler := JCEAnalyzeHandler(&ToolDeps{})

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestJCEHandlers_InvalidContentID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{JCE: &stubJCEService{}}
	handler := JCEAnalyzeHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"not-a-uuid"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestJCEHandlers_ServiceError(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{JCE: &stubJCEService{err: errors.New("boom")}}
	handler := JCEAnalyzeHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func TestJCE_Registration(t *testing.T) {
	t.Parallel()

	registry := NewToolHandlerRegistry()
	deps := &ToolDeps{JCE: &stubJCEService{}}
	RegisterPhase7ComparisonTools(registry, deps)

	names := []string{
		"comparison.analyze",
		"comparison.rank",
		"comparison.explain",
		"comparison.recommend_next_step",
		"comparison.refresh",
	}

	for _, name := range names {
		if registry.Handler(name) == nil {
			t.Errorf("handler for %q not registered", name)
		}

		if registry.privacyTargetFor(name) == nil {
			t.Errorf("privacy target for %q not registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Privacy targets
// ---------------------------------------------------------------------------

func TestJCEPrivacyTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fn      PrivacyTargetFunc
		content string
		wantErr bool
	}{
		{
			name:    "analyze",
			fn:      JCEAnalyzePrivacyTarget,
			content: "00000000-0000-0000-0000-000000000001",
		},
		{
			name:    "rank",
			fn:      JCERankPrivacyTarget,
			content: "00000000-0000-0000-0000-000000000002",
		},
		{
			name:    "explain",
			fn:      JCEExplainPrivacyTarget,
			content: "00000000-0000-0000-0000-000000000003",
		},
		{
			name:    "recommend",
			fn:      JCERecommendPrivacyTarget,
			content: "00000000-0000-0000-0000-000000000004",
		},
		{
			name:    "refresh",
			fn:      JCERefreshPrivacyTarget,
			content: "00000000-0000-0000-0000-000000000005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := json.RawMessage(`{"content_id":"` + tt.content + `"}`)

			et, id, err := tt.fn(t.Context(), args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if et != PrivacyEntityContent {
				t.Errorf("entity type = %q, want %q", et, PrivacyEntityContent)
			}

			if !id.Valid || id.String() != tt.content {
				t.Errorf("id = %v, want %q", id, tt.content)
			}
		})
	}
}

func TestJCEPrivacyTarget_MissingContentID(t *testing.T) {
	t.Parallel()

	et, id, err := JCEAnalyzePrivacyTarget(t.Context(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if et != "" {
		t.Errorf("entity type = %q, want empty", et)
	}

	if id.Valid {
		t.Errorf("expected invalid UUID")
	}
}

func TestJCEPrivacyTarget_InvalidContentID(t *testing.T) {
	t.Parallel()

	et, id, err := JCEAnalyzePrivacyTarget(t.Context(), json.RawMessage(`{"content_id":"not-a-uuid"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if et != "" {
		t.Errorf("entity type = %q, want empty", et)
	}

	if id.Valid {
		t.Errorf("expected invalid UUID")
	}
}

// ---------------------------------------------------------------------------
// ToolDeps setter
// ---------------------------------------------------------------------------

func TestJCE_WithJCE(t *testing.T) {
	t.Parallel()

	svc := &stubJCEService{}
	deps := NewToolDeps().WithJCE(svc)

	if deps.JCE != svc {
		t.Error("WithJCE did not set JCE field")
	}
}
