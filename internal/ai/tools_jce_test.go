// Package ai — tools_jce_test.go covers the JCE refresh tool handler.
//
// The handler is tested with stub services that implement the JCEService
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

	refreshCalled bool

	lastContentID  pgtype.UUID
	lastSourceKind *string
}

func (s *stubJCEService) Refresh(ctx context.Context, contentID, userID pgtype.UUID, sourceKind *string) (map[string]any, error) {
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
// Handler
// ---------------------------------------------------------------------------

func TestJCERefreshHandler_Success(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	svc := &stubJCEService{result: map[string]any{"status": "pending"}}
	deps := &ToolDeps{
		JCE:        svc,
		Permission: &stubJCEPermissionChecker{allowed: map[string]bool{"jce.refresh.trigger": true}},
	}

	handler := JCERefreshHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","source_kind":"official"}`), pgtype.UUID{})
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

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
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

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
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

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
}

func TestJCEHandlers_NoJCE(t *testing.T) {
	t.Parallel()

	handler := JCERefreshHandler(&ToolDeps{})

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
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
	handler := JCERefreshHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"not-a-uuid"}`), pgtype.UUID{})
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
	handler := JCERefreshHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
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

	if registry.Handler("comparison.refresh") == nil {
		t.Error("handler for \"comparison.refresh\" not registered")
	}

	if registry.privacyTargetFor("comparison.refresh") == nil {
		t.Error("privacy target for \"comparison.refresh\" not registered")
	}

	for _, name := range []string{"comparison.analyze", "comparison.rank", "comparison.explain", "comparison.recommend_next_step"} {
		if registry.Handler(name) != nil {
			t.Errorf("removed advisor handler %q is still registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Privacy target
// ---------------------------------------------------------------------------

func TestJCEPrivacyTarget(t *testing.T) {
	t.Parallel()

	content := "00000000-0000-0000-0000-000000000005"
	args := json.RawMessage(`{"content_id":"` + content + `"}`)

	et, id, err := JCERefreshPrivacyTarget(t.Context(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if et != PrivacyEntityContent {
		t.Errorf("entity type = %q, want %q", et, PrivacyEntityContent)
	}

	if !id.Valid || id.String() != content {
		t.Errorf("id = %v, want %q", id, content)
	}
}

func TestJCEPrivacyTarget_MissingContentID(t *testing.T) {
	t.Parallel()

	et, id, err := JCERefreshPrivacyTarget(t.Context(), json.RawMessage(`{}`))
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

	et, id, err := JCERefreshPrivacyTarget(t.Context(), json.RawMessage(`{"content_id":"not-a-uuid"}`))
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
