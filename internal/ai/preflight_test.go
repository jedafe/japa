package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubFeatureChecker implements FeatureChecker with a per-key bool map.
// Keys absent from the map resolve to `fallback`.
type stubFeatureChecker struct {
	flags    map[string]bool
	fallback bool
}

func (s *stubFeatureChecker) IsEnabled(ctx context.Context, key string) bool {
	if v, ok := s.flags[key]; ok {
		return v
	}

	return s.fallback
}

// stubConsentChecker implements ConsentChecker for tests.
type stubConsentChecker struct {
	granted bool
	err     error
	gotType string // consent type seen on the last call
}

func (s *stubConsentChecker) HasConsent(ctx context.Context, userID pgtype.UUID, consentType string) (bool, error) {
	s.gotType = consentType
	return s.granted, s.err
}

// stubTierResolver implements TierResolver for tests.
type stubTierResolver struct {
	tier string
	err  error
}

func (s *stubTierResolver) ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error) {
	return s.tier, s.err
}

func validTestUserID() pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan("11111111-1111-1111-1111-111111111111")

	return id
}

// ---------------------------------------------------------------------------
// CheckAdmission — surface reachability
// ---------------------------------------------------------------------------

func TestPreflight_CheckAdmission_FlagEnabled(t *testing.T) {
	t.Parallel()

	p := NewPreflight(&stubFeatureChecker{flags: map[string]bool{"feature.ai_content_gen": true}}, nil, nil, nil, nil)

	if err := p.CheckAdmission(t.Context(), validTestUserID(), GateAssist); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPreflight_CheckAdmission_FlagDisabled(t *testing.T) {
	t.Parallel()

	p := NewPreflight(&stubFeatureChecker{flags: map[string]bool{"feature.ai_content_gen": false}}, nil, nil, nil, nil)

	err := p.CheckAdmission(t.Context(), validTestUserID(), GateAssist)
	if err == nil {
		t.Fatal("expected disabled error, got nil")
	}

	if err.Status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", err.Status, http.StatusServiceUnavailable)
	}

	if err.Code != "ai_assist_disabled" {
		t.Errorf("code = %q, want ai_assist_disabled", err.Code)
	}
}

func TestPreflight_CheckAdmission_NilCheckerFallsBackToSetting(t *testing.T) {
	t.Parallel()

	// Feature flag is authoritative; when no checker is wired the legacy
	// ai.content_assist_enabled setting is the fallback.
	settings := &agentSettingsReader{bools: map[string]bool{"ai.content_assist_enabled": false}}
	p := NewPreflight(nil, settings, nil, nil, nil)

	err := p.CheckAdmission(t.Context(), validTestUserID(), GateAssist)
	if err == nil || err.Code != "ai_assist_disabled" {
		t.Fatalf("expected ai_assist_disabled, got %v", err)
	}
}

func TestPreflight_CheckAdmission_NilDepsOpen(t *testing.T) {
	t.Parallel()

	p := NewPreflight(nil, nil, nil, nil, nil)

	if err := p.CheckAdmission(t.Context(), validTestUserID(), GateJapaBot); err != nil {
		t.Fatalf("nil deps should default open, got %v", err)
	}
}

func TestPreflight_CheckAdmission_FlagBeatsSetting(t *testing.T) {
	t.Parallel()

	// When a checker is wired the legacy setting must not override it:
	// flag enabled + setting disabled → open.
	settings := &agentSettingsReader{bools: map[string]bool{"ai.content_assist_enabled": false}}
	p := NewPreflight(&stubFeatureChecker{flags: map[string]bool{"feature.ai_content_gen": true}}, settings, nil, nil, nil)

	if err := p.CheckAdmission(t.Context(), validTestUserID(), GateAssist); err != nil {
		t.Fatalf("feature flag should beat the legacy setting, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// CheckConsent
// ---------------------------------------------------------------------------

func TestPreflight_CheckConsent_Granted(t *testing.T) {
	t.Parallel()

	p := NewPreflight(nil, nil, &stubConsentChecker{granted: true}, nil, nil)

	if err := p.CheckConsent(t.Context(), validTestUserID()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestPreflight_CheckConsent_Denied(t *testing.T) {
	t.Parallel()

	c := &stubConsentChecker{granted: false}
	p := NewPreflight(nil, nil, c, nil, nil)

	err := p.CheckConsent(t.Context(), validTestUserID())
	if err == nil {
		t.Fatal("expected consent error, got nil")
	}

	if err.Status != http.StatusForbidden || err.Code != "ai_consent_required" {
		t.Errorf("got status=%d code=%q, want 403 ai_consent_required", err.Status, err.Code)
	}

	if c.gotType != "ai_processing" {
		t.Errorf("consent type = %q, want ai_processing", c.gotType)
	}
}

func TestPreflight_CheckConsent_CheckerErrorFailsOpen(t *testing.T) {
	t.Parallel()

	p := NewPreflight(nil, nil, &stubConsentChecker{err: errors.New("gdpr store down")}, nil, nil)

	if err := p.CheckConsent(t.Context(), validTestUserID()); err != nil {
		t.Fatalf("checker error should fail open, got %v", err)
	}
}

func TestPreflight_CheckConsent_InvalidUserSkips(t *testing.T) {
	t.Parallel()

	p := NewPreflight(nil, nil, &stubConsentChecker{granted: false}, nil, nil)

	if err := p.CheckConsent(t.Context(), pgtype.UUID{}); err != nil {
		t.Fatalf("invalid userID should skip consent, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// CheckBudget — unified daily query + cost
// ---------------------------------------------------------------------------

func TestPreflight_CheckBudget_UnderLimit(t *testing.T) {
	t.Parallel()

	rl := &stubRateLimiter{count: 0}
	p := NewPreflight(nil, nil, nil, rl, &stubTierResolver{tier: "regular"})

	if err := p.CheckBudget(t.Context(), validTestUserID()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	if rl.count != 1 {
		t.Errorf("query counter = %d, want 1", rl.count)
	}

	if !rl.expired {
		t.Error("first increment should set the daily TTL")
	}
}

func TestPreflight_CheckBudget_OverLimit(t *testing.T) {
	t.Parallel()

	settings := &agentSettingsReader{ints: map[string]int{"ai.assistant.daily_queries.regular": 2}}
	rl := &stubRateLimiter{count: 2}
	p := NewPreflight(nil, settings, nil, rl, &stubTierResolver{tier: "regular"})

	err := p.CheckBudget(t.Context(), validTestUserID())
	if err == nil {
		t.Fatal("expected rate-limit error, got nil")
	}

	if err.Status != http.StatusTooManyRequests || err.Code != "ai_daily_limit" {
		t.Errorf("got status=%d code=%q, want 429 ai_daily_limit", err.Status, err.Code)
	}
}

func TestPreflight_CheckBudget_UnlimitedTier(t *testing.T) {
	t.Parallel()

	// business = -1 (unlimited) — the counter is never consulted.
	settings := &agentSettingsReader{ints: map[string]int{"ai.assistant.daily_queries.business": -1}}
	rl := &stubRateLimiter{count: 10000}
	p := NewPreflight(nil, settings, nil, rl, &stubTierResolver{tier: "business"})

	if err := p.CheckBudget(t.Context(), validTestUserID()); err != nil {
		t.Fatalf("unlimited tier should pass, got %v", err)
	}
}

func TestPreflight_CheckBudget_UsesResolvedTier(t *testing.T) {
	t.Parallel()

	// A pro user with 50 queries stays under the pro limit of 100 even
	// though the regular limit is 10.
	settings := &agentSettingsReader{
		ints: map[string]int{
			"ai.assistant.daily_queries.regular": 10,
			"ai.assistant.daily_queries.pro":     100,
		},
	}
	rl := &stubRateLimiter{count: 50}
	p := NewPreflight(nil, settings, nil, rl, &stubTierResolver{tier: "pro"})

	if err := p.CheckBudget(t.Context(), validTestUserID()); err != nil {
		t.Fatalf("pro tier should allow 51 queries, got %v", err)
	}
}

func TestPreflight_CheckBudget_TierResolveErrorFallsToRegular(t *testing.T) {
	t.Parallel()

	settings := &agentSettingsReader{ints: map[string]int{"ai.assistant.daily_queries.regular": 2}}
	rl := &stubRateLimiter{count: 2}
	p := NewPreflight(nil, settings, nil, rl, &stubTierResolver{err: errors.New("no db")})

	err := p.CheckBudget(t.Context(), validTestUserID())
	if err == nil || err.Code != "ai_daily_limit" {
		t.Fatalf("tier failure should fall back to regular limits, got %v", err)
	}
}

func TestPreflight_CheckBudget_CostCapExceeded(t *testing.T) {
	t.Parallel()

	settings := &agentSettingsReader{floats: map[string]float64{"ai.daily_cost_limit_usd": 5.00}}
	rl := &stubRateLimiter{costVal: "5.25"}
	p := NewPreflight(nil, settings, nil, rl, &stubTierResolver{tier: "business"})

	err := p.CheckBudget(t.Context(), validTestUserID())
	if err == nil {
		t.Fatal("expected cost-cap error, got nil")
	}

	if err.Status != http.StatusPaymentRequired || err.Code != "ai_daily_budget" {
		t.Errorf("got status=%d code=%q, want 402 ai_daily_budget", err.Status, err.Code)
	}
}

func TestPreflight_CheckBudget_CostCapZeroDisabled(t *testing.T) {
	t.Parallel()

	settings := &agentSettingsReader{floats: map[string]float64{"ai.daily_cost_limit_usd": 0}}
	rl := &stubRateLimiter{costVal: "9999.99"}
	p := NewPreflight(nil, settings, nil, rl, &stubTierResolver{tier: "business"})

	if err := p.CheckBudget(t.Context(), validTestUserID()); err != nil {
		t.Fatalf("0 cost cap should be disabled, got %v", err)
	}
}

func TestPreflight_CheckBudget_ValkeyFailureFailsOpen(t *testing.T) {
	t.Parallel()

	settings := &agentSettingsReader{ints: map[string]int{"ai.assistant.daily_queries.regular": 1}}
	rl := &stubRateLimiter{failIncr: true}
	p := NewPreflight(nil, settings, nil, rl, &stubTierResolver{tier: "regular"})

	if err := p.CheckBudget(t.Context(), validTestUserID()); err != nil {
		t.Fatalf("valkey outage should fail open, got %v", err)
	}
}

func TestPreflight_CheckBudget_NilBudgetSkips(t *testing.T) {
	t.Parallel()

	p := NewPreflight(nil, nil, nil, nil, nil)

	if err := p.CheckBudget(t.Context(), validTestUserID()); err != nil {
		t.Fatalf("nil budget client should skip, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RecordCost
// ---------------------------------------------------------------------------

func TestPreflight_RecordCost_Accumulates(t *testing.T) {
	t.Parallel()

	rl := &stubRateLimiter{}
	p := NewPreflight(nil, nil, nil, rl, nil)

	p.RecordCost(t.Context(), validTestUserID(), 0.0123)

	if !rl.expired {
		t.Error("first accumulation should set the daily TTL")
	}
}

func TestPreflight_RecordCost_NonPositiveNoop(t *testing.T) {
	t.Parallel()

	rl := &stubRateLimiter{failIncr: true} // would error if called
	p := NewPreflight(nil, nil, nil, rl, nil)

	p.RecordCost(t.Context(), validTestUserID(), 0)
	// no panic / no-op — pass
}

// ---------------------------------------------------------------------------
// Handler-level gates (degraded wiring — no preflight)
// ---------------------------------------------------------------------------

func TestHandleImprove_AssistDisabledByFlag(t *testing.T) {
	t.Parallel()

	h := &Handlers{checker: &stubFeatureChecker{flags: map[string]bool{"feature.ai_content_gen": false}}}
	user, platform := testUserAndPlatform()
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/improve", bytes.NewReader([]byte(`{"text":"hello"}`)))
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleImprove(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleImprove_AssistDisabledByLegacySetting(t *testing.T) {
	t.Parallel()

	// Nil checker → the deprecated ai.content_assist_enabled setting is
	// the fallback.
	h := &Handlers{Settings: &agentSettingsReader{bools: map[string]bool{"ai.content_assist_enabled": false}}}
	user, platform := testUserAndPlatform()
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/improve", bytes.NewReader([]byte(`{"text":"hello"}`)))
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleImprove(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleImprove_AssistEnabledByFlag(t *testing.T) {
	t.Parallel()

	disp := NewDispatcher()
	disp.RegisterProvider("content_generation", &mockProvider{})
	h := &Handlers{
		Dispatcher: disp,
		checker:    &stubFeatureChecker{flags: map[string]bool{"feature.ai_content_gen": true}},
	}
	user, platform := testUserAndPlatform()
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/improve", bytes.NewReader([]byte(`{"text":"hello"}`)))
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleImprove(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleImprove_ConsentRequired(t *testing.T) {
	t.Parallel()

	// Assist now enforces §19 ai_processing consent (previously only
	// /ai/chat did).
	h := &Handlers{consents: &stubConsentChecker{granted: false}}
	user, platform := testUserAndPlatform()
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/improve", bytes.NewReader([]byte(`{"text":"hello"}`)))
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleImprove(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleImprove_PreflightBudgetExceeded(t *testing.T) {
	t.Parallel()

	settings := &agentSettingsReader{ints: map[string]int{"ai.assistant.daily_queries.regular": 1}}
	rl := &stubRateLimiter{count: 1}
	h := &Handlers{
		preflight: NewPreflight(nil, settings, nil, rl, &stubTierResolver{tier: "regular"}),
	}
	user, platform := testUserAndPlatform()
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/improve", bytes.NewReader([]byte(`{"text":"hello"}`)))
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleImprove(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

// ---------------------------------------------------------------------------
// Agent tool handlers — shared assist service
// ---------------------------------------------------------------------------

// stubAssistRunner implements AssistRunner for tool-handler tests.
type stubAssistRunner struct {
	improveFn func(ctx context.Context, userID pgtype.UUID, in AssistImproveInput) (Response, error)
}

func (s *stubAssistRunner) AssistImprove(ctx context.Context, userID pgtype.UUID, in AssistImproveInput) (Response, error) {
	return s.improveFn(ctx, userID, in)
}

func (s *stubAssistRunner) AssistSummarize(ctx context.Context, userID pgtype.UUID, in AssistSummarizeInput) (Response, error) {
	return Response{}, nil
}

func (s *stubAssistRunner) AssistTranslate(ctx context.Context, userID pgtype.UUID, in AssistTranslateInput) (Response, error) {
	return Response{}, nil
}

func (s *stubAssistRunner) AssistGenerateTags(ctx context.Context, userID pgtype.UUID, in AssistTagsInput) (Response, error) {
	return Response{}, nil
}

func (s *stubAssistRunner) AssistGenerateOG(ctx context.Context, userID pgtype.UUID, in AssistOGInput) (Response, error) {
	return Response{}, nil
}

func (s *stubAssistRunner) AssistDraft(ctx context.Context, userID pgtype.UUID, in DraftInput) (DraftResult, error) {
	return DraftResult{}, nil
}

func TestAssistImproveHandler_NotConfigured(t *testing.T) {
	t.Parallel()

	handler := AssistImproveHandler(&ToolDeps{})

	res, err := handler(t.Context(), nil, validTestUserID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusError)
	}
}

func TestAssistImproveHandler_Success(t *testing.T) {
	t.Parallel()

	runner := &stubAssistRunner{
		improveFn: func(ctx context.Context, userID pgtype.UUID, in AssistImproveInput) (Response, error) {
			if in.Text != "rough draft" {
				t.Errorf("text = %q, want 'rough draft'", in.Text)
			}

			return Response{
				ProviderRef: "ref-1",
				Raw:         map[string]any{"reply": "polished draft"},
			}, nil
		},
	}

	handler := AssistImproveHandler(&ToolDeps{Assist: runner})

	res, err := handler(t.Context(), json.RawMessage(`{"text":"rough draft","tone":"casual"}`), validTestUserID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Errorf("status = %q, want %q; content: %s", res.Status, ToolCallStatusSuccess, res.Content)
	}
}

func TestAssistImproveHandler_ValidationError(t *testing.T) {
	t.Parallel()

	runner := &stubAssistRunner{
		improveFn: func(ctx context.Context, userID pgtype.UUID, in AssistImproveInput) (Response, error) {
			return Response{}, assistValidation("text is required")
		},
	}

	handler := AssistImproveHandler(&ToolDeps{Assist: runner})

	res, err := handler(t.Context(), json.RawMessage(`{}`), validTestUserID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusError)
	}
}

func TestAssistImproveHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	handler := AssistImproveHandler(&ToolDeps{Assist: &stubAssistRunner{}})

	res, err := handler(t.Context(), json.RawMessage(`{bad`), validTestUserID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %q, want %q", res.Status, ToolCallStatusError)
	}
}
