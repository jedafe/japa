// Package ai — executor_test.go covers the Phase 3 Tool Executor pipeline
// and PrivacyChecker blocklist logic (Section 103, ADR-111).
//
// These tests exercise the executor without a live database: the writer
// queries are nil (tool-call logging is a no-op), the store is nil (HITL is
// skipped), and the PrivacyChecker is a stub. The pipeline ordering, RBAC
// gate, privacy gate, blocklist, schema validation, and dispatch are all
// verified through in-memory handlers.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Executor)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 3)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// IsBlockedOperation (task 3.02)
// ---------------------------------------------------------------------------

func TestIsBlockedOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{"rbac.grant", true},
		{"rbac.revoke", true},
		{"ai_privacy.set_exempt", true},
		{"ai_providers.update", true},
		{"settings.security.update", true},
		{"user.delete", true},
		{"user.read_private", true},
		// Allowed tools.
		{"content.search", false},
		{"content.create", false},
		{"user.read_profile", false},
		{"menu.list", false},
		{"rag.search", false},
		// Empty / unrelated.
		{"", false},
		{"users.list", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := IsBlockedOperation(tc.name); got != tc.want {
				t.Errorf("IsBlockedOperation(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// validateRequiredArgs
// ---------------------------------------------------------------------------

func TestValidateRequiredArgs(t *testing.T) {
	t.Parallel()

	t.Run("no required fields accepts any json", func(t *testing.T) {
		t.Parallel()

		if err := validateRequiredArgs(json.RawMessage(`{"q":"x"}`), nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("no required fields rejects invalid json", func(t *testing.T) {
		t.Parallel()

		if err := validateRequiredArgs(json.RawMessage(`{bad`), nil); err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		t.Parallel()

		err := validateRequiredArgs(json.RawMessage(`{"q":"x"}`), []string{"q", "limit"})
		if err == nil {
			t.Fatal("expected error for missing 'limit', got nil")
		}
	})

	t.Run("all required present", func(t *testing.T) {
		t.Parallel()

		err := validateRequiredArgs(json.RawMessage(`{"q":"x","limit":5}`), []string{"q", "limit"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("non-object args with required fields", func(t *testing.T) {
		t.Parallel()

		err := validateRequiredArgs(json.RawMessage(`[1,2,3]`), []string{"q"})
		if err == nil {
			t.Fatal("expected error for non-object args, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// deriveContentType
// ---------------------------------------------------------------------------

func TestDeriveContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"content.create", "content"},
		{"menu.update", "menu"},
		{"custom_field_values.bulk_set", "custom_field_values"},
		{"rag.search", "rag"},
		{"nodot", "nodot"},
	}
	for _, tc := range tests {
		if got := deriveContentType(tc.in); got != tc.want {
			t.Errorf("deriveContentType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ToolHandlerRegistry — CatalogForUser (task 3.08)
// ---------------------------------------------------------------------------

func TestCatalogForUser_FiltersByPermissionAndBlocklist(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{FunctionName: "content.search"}, "", pgtype.UUID{})
	reg.RegisterConfig("content.create", ToolConfig{FunctionName: "content.create"}, "content.create", pgtype.UUID{})
	reg.RegisterConfig("content.delete", ToolConfig{FunctionName: "content.delete", IsBlocked: true}, "content.delete", pgtype.UUID{})
	// Hardcoded blocklist — even with no is_blocked flag and no permission.
	reg.RegisterConfig("rbac.grant", ToolConfig{FunctionName: "rbac.grant"}, "", pgtype.UUID{})

	perms := &stubPerms{granted: map[string]bool{"content.create": true}}

	got := reg.CatalogForUser(t.Context(), perms, pgtype.UUID{}, pgtype.UUID{})
	names := toolConfigNames(got)

	if !contains(names, "content.search") {
		t.Error("content.search should be in catalog (no permission required)")
	}

	if !contains(names, "content.create") {
		t.Error("content.create should be in catalog (user has permission)")
	}

	if contains(names, "content.delete") {
		t.Error("content.delete should be filtered out (is_blocked=true)")
	}

	if contains(names, "rbac.grant") {
		t.Error("rbac.grant should be filtered out (hardcoded blocklist)")
	}
}

func TestCatalogForUser_UserLacksPermission(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.create", ToolConfig{FunctionName: "content.create"}, "content.create", pgtype.UUID{})

	perms := &stubPerms{granted: map[string]bool{}}
	got := reg.CatalogForUser(t.Context(), perms, pgtype.UUID{}, pgtype.UUID{})

	if len(got) != 0 {
		t.Errorf("expected empty catalog for user without content.create, got %d tools", len(got))
	}
}

// ---------------------------------------------------------------------------
// Executor.Execute pipeline (tasks 3.03-3.07)
// ---------------------------------------------------------------------------

func newTestExecutor(t *testing.T, perms *stubPerms, privacy *stubPrivacy, reg *ToolHandlerRegistry) *Executor {
	t.Helper()

	return NewExecutor(
		reg,
		WithPermissionChecker(perms),
		WithPrivacyChecker(privacy),
	)
}

func TestExecute_Success(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{
		FunctionName: "content.search",
		Parameters: map[string]any{
			"type":     "object",
			"required": []any{"query"},
		},
	}, "", pgtype.UUID{})
	reg.RegisterHandler("content.search", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		return &ToolResult{Status: ToolCallStatusSuccess, Content: `{"results":[]}`}, nil
	})

	exec := newTestExecutor(t, &stubPerms{}, &stubPrivacy{}, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call:   ToolCall{ID: "c1", Name: "content.search", Arguments: json.RawMessage(`{"query":"visa"}`)},
		UserID: pgtype.UUID{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusSuccess)
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	exec := newTestExecutor(t, &stubPerms{}, &stubPrivacy{}, reg)

	_, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "no.such.tool", Arguments: json.RawMessage(`{}`)},
	})
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

func TestExecute_SchemaValidation_MissingRequired(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{
		FunctionName: "content.search",
		Parameters: map[string]any{
			"required": []any{"query"},
		},
	}, "", pgtype.UUID{})

	called := false

	reg.RegisterHandler("content.search", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		called = true
		return &ToolResult{Status: ToolCallStatusSuccess}, nil
	})

	exec := newTestExecutor(t, &stubPerms{}, &stubPrivacy{}, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.search", Arguments: json.RawMessage(`{"limit":5}`)},
	})
	if err == nil {
		t.Fatal("expected error for missing required field, got nil")
	}

	if called {
		t.Error("handler should not be called when schema validation fails")
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusError)
	}
}

func TestExecute_BlockedOperation(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("rbac.grant", ToolConfig{FunctionName: "rbac.grant"}, "", pgtype.UUID{})

	called := false

	reg.RegisterHandler("rbac.grant", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		called = true
		return &ToolResult{Status: ToolCallStatusSuccess}, nil
	})

	exec := newTestExecutor(t, &stubPerms{}, &stubPrivacy{}, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "rbac.grant", Arguments: json.RawMessage(`{}`)},
	})
	if err == nil {
		t.Fatal("expected error for blocked operation, got nil")
	}

	if called {
		t.Error("handler should not be called for a blocked operation")
	}

	if res.Status != ToolCallStatusUnauthorized {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusUnauthorized)
	}
}

func TestExecute_RBACUnauthorized(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.create", ToolConfig{FunctionName: "content.create"}, "content.create", pgtype.UUID{})

	called := false

	reg.RegisterHandler("content.create", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		called = true
		return &ToolResult{Status: ToolCallStatusSuccess}, nil
	})

	perms := &stubPerms{granted: map[string]bool{}} // no content.create
	exec := newTestExecutor(t, perms, &stubPrivacy{}, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.create", Arguments: json.RawMessage(`{}`)},
	})
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}

	if called {
		t.Error("handler should not be called when RBAC denies")
	}

	if res.Status != ToolCallStatusUnauthorized {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusUnauthorized)
	}
}

func TestExecute_RBACFailClosed_NilPerms(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.create", ToolConfig{FunctionName: "content.create"}, "content.create", pgtype.UUID{})

	called := false

	reg.RegisterHandler("content.create", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		called = true
		return &ToolResult{Status: ToolCallStatusSuccess}, nil
	})

	// No permission checker injected — the executor must fail-closed and
	// deny the call rather than silently allowing a gated tool through.
	exec := NewExecutor(
		reg,
		WithPrivacyChecker(&stubPrivacy{}),
		// deliberately omitting WithPermissionChecker
	)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.create", Arguments: json.RawMessage(`{}`)},
	})
	if err == nil {
		t.Fatal("expected unauthorized error for nil perms + gated tool, got nil")
	}

	if called {
		t.Error("handler should not be called when perms checker is nil and tool is gated")
	}

	if res.Status != ToolCallStatusUnauthorized {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusUnauthorized)
	}
}

func TestExecute_RBACNoPermissionRequired_NilPerms_Proceeds(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{FunctionName: "content.search"}, "", pgtype.UUID{})
	reg.RegisterHandler("content.search", stubSuccessHandler)

	// No permission checker, but the tool has no required_permission —
	// the call should proceed (fail-closed only applies to gated tools).
	exec := NewExecutor(
		reg,
		WithPrivacyChecker(&stubPrivacy{}),
	)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.search", Arguments: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusSuccess)
	}
}

func TestExecute_PrivacyBlocked(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.read", ToolConfig{FunctionName: "content.read"}, "", pgtype.UUID{})
	reg.RegisterHandler("content.read", stubSuccessHandler)
	reg.RegisterPrivacyTarget("content.read", fixedPrivacyTarget(PrivacyEntityContent, contentID))

	privacy := &stubPrivacy{exempt: true}
	exec := newTestExecutor(t, &stubPerms{}, privacy, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.read", Arguments: json.RawMessage(`{"content_id":"` + contentIDStr + `"}`)},
	})
	if err == nil {
		t.Fatal("expected privacy-blocked error, got nil")
	}

	if !errors.Is(err, ErrPrivacyBlocked) {
		t.Errorf("error should wrap ErrPrivacyBlocked, got %v", err)
	}

	if res.Status != ToolCallStatusPrivacyBlocked {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusPrivacyBlocked)
	}
}

func TestExecute_PrivacyNotExempt_Proceeds(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.read", ToolConfig{FunctionName: "content.read"}, "", pgtype.UUID{})
	reg.RegisterHandler("content.read", stubSuccessHandler)
	reg.RegisterPrivacyTarget("content.read", fixedPrivacyTarget(PrivacyEntityContent, contentID))

	privacy := &stubPrivacy{exempt: false}
	exec := newTestExecutor(t, &stubPerms{}, privacy, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.read", Arguments: json.RawMessage(`{"content_id":"` + contentIDStr + `"}`)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusSuccess)
	}
}

func TestExecute_PrivacyTargetZero_SkipsCheck(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("rag.search", ToolConfig{FunctionName: "rag.search"}, "", pgtype.UUID{})
	reg.RegisterHandler("rag.search", stubSuccessHandler)
	// Returns zero entity → executor should skip the privacy check.
	reg.RegisterPrivacyTarget("rag.search", func(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
		return "", pgtype.UUID{}, nil
	})

	// Even with exempt=true, the call proceeds because the target is zero.
	privacy := &stubPrivacy{exempt: true}
	exec := newTestExecutor(t, &stubPerms{}, privacy, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "rag.search", Arguments: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusSuccess)
	}
}

func TestExecute_HandlerError(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{FunctionName: "content.search"}, "", pgtype.UUID{})
	reg.RegisterHandler("content.search", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		return nil, errors.New("boom")
	})

	exec := newTestExecutor(t, &stubPerms{}, &stubPrivacy{}, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.search", Arguments: json.RawMessage(`{}`)},
	})
	if err == nil {
		t.Fatal("expected error from handler, got nil")
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusError)
	}
}

func TestExecute_HandlerPanic_Recovered(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{FunctionName: "content.search"}, "", pgtype.UUID{})
	reg.RegisterHandler("content.search", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		panic("kaboom")
	})

	exec := newTestExecutor(t, &stubPerms{}, &stubPrivacy{}, reg)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.search", Arguments: json.RawMessage(`{}`)},
	})
	if err == nil {
		t.Fatal("expected error from panicked handler, got nil")
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusError)
	}
}

// ---------------------------------------------------------------------------
// Test helpers + stubs
// ---------------------------------------------------------------------------

type stubPerms struct {
	granted map[string]bool
}

func (s *stubPerms) HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool {
	if s == nil {
		return false
	}

	return s.granted[permKey]
}

type stubPrivacy struct {
	exempt bool
	err    error
}

func (s *stubPrivacy) IsExempt(ctx context.Context, entityType PrivacyEntityType, tenantID, entityID pgtype.UUID) (bool, error) {
	if s == nil {
		return false, nil
	}

	return s.exempt, s.err
}

// stubSuccessHandler is a ToolHandler that always succeeds.
func stubSuccessHandler(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
	return &ToolResult{Status: ToolCallStatusSuccess, Content: `{"ok":true}`}, nil
}

// fixedPrivacyTarget returns a PrivacyTargetFunc that always reports the
// given entity type + ID, regardless of arguments.
func fixedPrivacyTarget(et PrivacyEntityType, eid pgtype.UUID) PrivacyTargetFunc {
	return func(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error) {
		return et, eid, nil
	}
}

// Fixed UUIDs used across privacy tests.
var (
	contentID    = uuidMust("11111111-1111-1111-1111-111111111111")
	contentIDStr = "11111111-1111-1111-1111-111111111111"
)

func uuidMust(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		panic(err)
	}

	return u
}

func toolConfigNames(cs []ToolConfig) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.FunctionName
	}

	return out
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}

	return false
}
