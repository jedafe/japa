// Package ai — phase11_hardening_test.go covers the Phase 11 Hardening &
// Audit tasks from ROADMAP-AI-AGENT.md:
//
//   - 11.01a: executor fail-closed when privacy checker is nil but a
//     PrivacyTargetFunc is registered (defense-in-depth matching the RBAC
//     fail-closed pattern).
//   - 11.02a: category-based unconditional HITL routing for high-stakes
//     guide categories (visa, banking, grants, health_insurance, admission)
//     via ToolResult.ForceHITL.
//   - 11.03a: tool name normalization (trim + lowercase) before blocklist
//     check so an LLM cannot bypass the blocklist via casing or whitespace.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 11)
package ai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// 11.01a — Privacy fail-closed when checker is nil but target is registered
// ---------------------------------------------------------------------------

func TestExecute_PrivacyFailClosed_NilChecker(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.read", ToolConfig{FunctionName: "content.read"}, "", pgtype.UUID{})

	called := false

	reg.RegisterHandler("content.read", func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		called = true
		return &ToolResult{Status: ToolCallStatusSuccess}, nil
	})
	reg.RegisterPrivacyTarget("content.read", fixedPrivacyTarget(PrivacyEntityContent, contentID))

	// No privacy checker injected — the executor must fail-closed and
	// deny the call rather than silently allowing a privacy-gated tool
	// through. This mirrors the RBAC fail-closed pattern.
	exec := NewExecutor(
		reg,
		WithPermissionChecker(&stubPerms{granted: map[string]bool{"content.read": true}}),
		// deliberately omitting WithPrivacyChecker
	)

	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "content.read", Arguments: json.RawMessage(`{"content_id":"` + contentIDStr + `"}`)},
	})
	if err == nil {
		t.Fatal("expected privacy-blocked error for nil checker + registered target, got nil")
	}

	if called {
		t.Error("handler should not be called when privacy checker is nil and target is registered")
	}

	if res.Status != ToolCallStatusPrivacyBlocked {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusPrivacyBlocked)
	}
}

func TestExecute_PrivacyNoTarget_NilChecker_Proceeds(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{FunctionName: "content.search"}, "", pgtype.UUID{})
	reg.RegisterHandler("content.search", stubSuccessHandler)
	// No privacy target registered — nil checker should NOT block.

	exec := NewExecutor(
		reg,
		WithPermissionChecker(&stubPerms{granted: map[string]bool{"content.search": true}}),
		// no privacy checker
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

// ---------------------------------------------------------------------------
// 11.02a — High-stakes category HITL routing (IsHighStakesCategory + ForceHITL)
// ---------------------------------------------------------------------------

func TestIsHighStakesCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		category string
		want     bool
	}{
		// High-stakes per §103.
		{"visa", true},
		{"banking", true},
		{"grants", true},
		{"health_insurance", true},
		{"admission", true},
		// Case-insensitive + whitespace tolerance.
		{"Visa", true},
		{"  BANKING  ", true},
		{"Grants", true},
		// Non-high-stakes categories.
		{"travel_routes", false},
		{"scholarship", false},
		{"accommodation", false},
		{"work_career", false},
		{"culture_lifestyle", false},
		// Empty / unrelated.
		{"", false},
		{"general", false},
	}
	for _, tc := range tests {
		t.Run(tc.category, func(t *testing.T) {
			t.Parallel()

			if got := IsHighStakesCategory(tc.category); got != tc.want {
				t.Errorf("IsHighStakesCategory(%q) = %v, want %v", tc.category, got, tc.want)
			}
		})
	}
}

// TestContentCreateHandler_ForceHITL_HighStakesCategory verifies that the
// content.create handler sets ForceHITL=true on the ToolResult when the
// guide_category is a §103 high-stakes category. The executor checks this
// flag after dispatch and creates a HITL review even when the tool's
// requires_hitl flag is false.
func TestContentCreateHandler_ForceHITL_HighStakesCategory(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, input ContentCreateInput) (ContentCreateRow, error) {
			return ContentCreateRow{
				ID:            uuidMust("44444444-4444-4444-4444-444444444444"),
				Title:         input.Title,
				Slug:          "visa-guide",
				Status:        "draft",
				ContentType:   input.ContentType,
				GuideCategory: input.GuideCategory,
			}, nil
		},
	}

	handler := ContentCreateHandler(deps)

	args, _ := json.Marshal(contentCreateArgs{
		Title:         "Visa Application Guide",
		Body:          "How to apply for a visa...",
		ContentType:   "guide",
		GuideCategory: "visa",
	})

	res, err := handler(t.Context(), args, uuidMust("55555555-5555-5555-5555-555555555555"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.ForceHITL {
		t.Error("expected ForceHITL=true for high-stakes guide_category 'visa'")
	}
}

func TestContentCreateHandler_NoForceHITL_NonHighStakesCategory(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, input ContentCreateInput) (ContentCreateRow, error) {
			return ContentCreateRow{
				ID:            uuidMust("44444444-4444-4444-4444-444444444444"),
				Title:         input.Title,
				Slug:          "culture-guide",
				Status:        "draft",
				ContentType:   input.ContentType,
				GuideCategory: input.GuideCategory,
			}, nil
		},
	}

	handler := ContentCreateHandler(deps)

	args, _ := json.Marshal(contentCreateArgs{
		Title:         "Culture & Lifestyle Guide",
		Body:          "Living abroad...",
		ContentType:   "guide",
		GuideCategory: "culture_lifestyle",
	})

	res, err := handler(t.Context(), args, uuidMust("55555555-5555-5555-5555-555555555555"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ForceHITL {
		t.Error("ForceHITL should be false for non-high-stakes guide_category 'culture_lifestyle'")
	}
}

func TestContentCreateHandler_NoForceHITL_NoCategory(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, input ContentCreateInput) (ContentCreateRow, error) {
			return ContentCreateRow{
				ID:          uuidMust("44444444-4444-4444-4444-444444444444"),
				Title:       input.Title,
				Slug:        "general-post",
				Status:      "draft",
				ContentType: input.ContentType,
			}, nil
		},
	}

	handler := ContentCreateHandler(deps)

	args, _ := json.Marshal(contentCreateArgs{
		Title:       "General Post",
		Body:        "Hello world...",
		ContentType: "article",
	})

	res, err := handler(t.Context(), args, uuidMust("55555555-5555-5555-5555-555555555555"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ForceHITL {
		t.Error("ForceHITL should be false when no guide_category is provided")
	}
}

// TestContentCreateDraftHandler_ForceHITL_HighStakesCategory verifies the
// same behavior for the content.create_draft handler.
func TestContentCreateDraftHandler_ForceHITL_HighStakesCategory(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, input ContentCreateInput) (ContentCreateRow, error) {
			return ContentCreateRow{
				ID:            uuidMust("44444444-4444-4444-4444-444444444444"),
				Title:         input.Title,
				Slug:          "banking-guide",
				Status:        "draft",
				ContentType:   input.ContentType,
				GuideCategory: input.GuideCategory,
			}, nil
		},
	}

	handler := ContentCreateDraftHandler(deps)

	args, _ := json.Marshal(contentCreateDraftArgs{
		Title:          "Banking Guide for Expats",
		Body:           "How to open a bank account...",
		ContentType:    "guide",
		GuideCategory:  "banking",
		OriginalPrompt: "Write a banking guide",
	})

	res, err := handler(t.Context(), args, uuidMust("55555555-5555-5555-5555-555555555555"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.ForceHITL {
		t.Error("expected ForceHITL=true for high-stakes guide_category 'banking'")
	}
}

// ---------------------------------------------------------------------------
// 11.03a — Tool name normalization before blocklist check
// ---------------------------------------------------------------------------

func TestIsBlockedOperation_CaseInsensitive(t *testing.T) {
	t.Parallel()

	// Uppercase / mixed-case variants of blocked tools should be caught.
	blockedVariants := []string{
		"RBAC.grant",
		"Rbac.Revoke",
		"AI_PRIVACY.set_exempt",
		"Ai_Providers.update",
		"SETTINGS.SECURITY.update",
		"USER.DELETE",
		"User.Read_Private",
	}
	for _, name := range blockedVariants {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !IsBlockedOperation(name) {
				t.Errorf("IsBlockedOperation(%q) = false, want true (case-insensitive)", name)
			}
		})
	}
}

func TestIsBlockedOperation_WhitespaceTrimmed(t *testing.T) {
	t.Parallel()

	// Whitespace-padded blocked tools should be caught.
	blockedPadded := []string{
		" rbac.grant ",
		"  user.delete",
		"user.read_private\t",
		"\nai_privacy.set_exempt\n",
	}
	for _, name := range blockedPadded {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !IsBlockedOperation(name) {
				t.Errorf("IsBlockedOperation(%q) = false, want true (whitespace trimmed)", name)
			}
		})
	}
}

func TestExecute_NormalizesToolName_BlockedUppercase(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	// Register a legitimate tool that the normalized name would match.
	reg.RegisterConfig("content.search", ToolConfig{FunctionName: "content.search"}, "", pgtype.UUID{})
	reg.RegisterHandler("content.search", stubSuccessHandler)

	exec := newTestExecutor(t, &stubPerms{granted: map[string]bool{"content.search": true}}, &stubPrivacy{}, reg)

	// An LLM emitting "RBAC.GRANT" should be blocked by the blocklist
	// even though the casing differs from the lowercase seed.
	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "RBAC.GRANT", Arguments: json.RawMessage(`{}`)},
	})
	if err == nil {
		t.Fatal("expected blocked error for uppercase RBAC.GRANT, got nil")
	}

	if res.Status != ToolCallStatusUnauthorized {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusUnauthorized)
	}
}

func TestExecute_NormalizesToolName_LegitimateUppercase(t *testing.T) {
	t.Parallel()

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{
		FunctionName: "content.search",
		Parameters: map[string]any{
			"type":     "object",
			"required": []any{"query"},
		},
	}, "", pgtype.UUID{})
	reg.RegisterHandler("content.search", stubSuccessHandler)

	exec := newTestExecutor(t, &stubPerms{granted: map[string]bool{"content.search": true}}, &stubPrivacy{}, reg)

	// An LLM emitting "CONTENT.SEARCH" should be normalized to
	// "content.search" and dispatched successfully.
	res, err := exec.Execute(t.Context(), ExecuteParams{
		Call: ToolCall{ID: "c1", Name: "CONTENT.SEARCH", Arguments: json.RawMessage(`{"query":"test"}`)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusSuccess)
	}
}
