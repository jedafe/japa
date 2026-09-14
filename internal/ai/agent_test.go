// Package ai — agent_test.go covers the Phase 5 agent loop
// (Section 103, ADR-111).
//
// Tests verify:
//   - Pre-checks: agent.enabled toggle, agent.use RBAC permission, rate limit.
//   - Session lifecycle: create → complete / fail.
//   - LLM loop: final response on no tool calls, tool call dispatch, max
//     iterations.
//   - Event emission: session_start, tool_call_start, tool_call_result,
//     progress, final_response, error.
//   - Cost limit enforcement.
//   - System prompt contains behavior rules + pre-search summary.
//
// The tests use a mock Provider that implements ChatWithTools with
// scripted responses, so no live LLM API is required.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Agent Loop)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 5, tasks 5.04-5.12)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Mock Provider — implements Provider.ChatWithTools with scripted responses
// ---------------------------------------------------------------------------

// mockAgentProvider is a Provider that returns scripted ChatWithTools
// responses in sequence. It also implements Send (returning an error) so it
// satisfies the full Provider interface.
type mockAgentProvider struct {
	name      string
	responses []ChatWithToolsResponse
	calls     int
}

func (m *mockAgentProvider) Name() string { return m.name }

func (m *mockAgentProvider) Models() []ModelInfo { return nil }

func (m *mockAgentProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) { return nil, nil }

func (m *mockAgentProvider) VerifyWebhookSignature(payload []byte, sig string) error { return nil }

func (m *mockAgentProvider) Send(ctx context.Context, req Request) (Response, error) {
	return Response{}, errors.New("mock: Send not supported")
}

func (m *mockAgentProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	if m.calls >= len(m.responses) {
		// Default: return a final response with no tool calls.
		return ChatWithToolsResponse{
			Content:      "Done.",
			FinishReason: "stop",
			Usage:        TokenUsage{TotalTokens: 100},
		}, nil
	}

	resp := m.responses[m.calls]
	m.calls++

	return resp, nil
}

// ---------------------------------------------------------------------------
// Stub session store, geo resolver, rate limiter
// ---------------------------------------------------------------------------

// stubSessionStore implements SessionStore for testing.
type stubSessionStore struct {
	created   gen.AiAgentSession
	completed *gen.AiAgentSession
	failed    *gen.AiAgentSession
	cancelled *gen.AiAgentSession
	complete  gen.CompleteAgentSessionParams
	fail      gen.FailAgentSessionParams
	cancel    pgtype.UUID
}

func (s *stubSessionStore) CreateAgentSession(ctx context.Context, arg gen.CreateAgentSessionParams) (gen.AiAgentSession, error) {
	s.created = gen.AiAgentSession{
		ID: randomUUID(),

		UserID:         arg.UserID,
		Prompt:         arg.Prompt,
		GeoSpaceID:     arg.GeoSpaceID,
		PresearchLevel: arg.PresearchLevel,
		Status:         arg.Status,
	}

	return s.created, nil
}

func (s *stubSessionStore) CompleteAgentSession(ctx context.Context, arg gen.CompleteAgentSessionParams) (gen.AiAgentSession, error) {
	s.complete = arg
	c := s.created
	c.Status = "completed"
	c.ToolCallCount = arg.ToolCallCount
	c.TotalTokens = arg.TotalTokens
	s.completed = &c

	return c, nil
}

func (s *stubSessionStore) FailAgentSession(ctx context.Context, arg gen.FailAgentSessionParams) (gen.AiAgentSession, error) {
	s.fail = arg
	c := s.created
	c.Status = "failed"
	c.ToolCallCount = arg.ToolCallCount
	c.TotalTokens = arg.TotalTokens
	s.failed = &c

	return c, nil
}

func (s *stubSessionStore) CancelAgentSession(ctx context.Context, id pgtype.UUID) (gen.AiAgentSession, error) {
	s.cancel = id
	c := s.created
	c.Status = "cancelled"
	s.cancelled = &c

	return c, nil
}

// stubGeoResolver implements GeoSpaceResolver for testing.
type stubGeoResolver struct {
	row gen.GetUserCurrentGeoSpaceRow
	err error
}

func (s *stubGeoResolver) GetUserCurrentGeoSpace(ctx context.Context, userID pgtype.UUID) (gen.GetUserCurrentGeoSpaceRow, error) {
	return s.row, s.err
}

// stubRateLimiter implements RateLimiter for testing.
type stubRateLimiter struct {
	count    int64
	expired  bool
	failIncr bool
	costVal  string // value returned by Get for the cost key; "" = not set
	getErr   error  // error to return from Get; nil = return costVal
}

func (s *stubRateLimiter) Incr(ctx context.Context, key string) (int64, error) {
	if s.failIncr {
		return 0, errors.New("valkey unavailable")
	}

	s.count++

	return s.count, nil
}

func (s *stubRateLimiter) IncrByFloat(ctx context.Context, key string, increment float64) (float64, error) {
	if s.failIncr {
		return 0, errors.New("valkey unavailable")
	}

	return increment, nil
}

func (s *stubRateLimiter) Expire(ctx context.Context, key string, ttlSeconds int) error {
	s.expired = true
	return nil
}

func (s *stubRateLimiter) Get(ctx context.Context, key string) (string, error) {
	if s.getErr != nil {
		return "", s.getErr
	}

	return s.costVal, nil
}

// agentSettingsReader is a configurable SettingsReader for agent tests.
type agentSettingsReader struct {
	bools   map[string]bool
	ints    map[string]int
	floats  map[string]float64
	strings map[string]string
}

func (s *agentSettingsReader) GetStringOr(ctx context.Context, key, fallback string) string {
	if v, ok := s.strings[key]; ok {
		return v
	}

	return fallback
}

func (s *agentSettingsReader) GetIntOr(ctx context.Context, key string, fallback int) int {
	if v, ok := s.ints[key]; ok {
		return v
	}

	return fallback
}

func (s *agentSettingsReader) GetFloatOr(ctx context.Context, key string, fallback float64) float64 {
	if v, ok := s.floats[key]; ok {
		return v
	}

	return fallback
}

func (s *agentSettingsReader) GetBoolOr(ctx context.Context, key string, fallback bool) bool {
	if v, ok := s.bools[key]; ok {
		return v
	}

	return fallback
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// collectEvents drains the channel and returns all events.
func collectEvents(t *testing.T, ch <-chan AgentEvent) []AgentEvent {
	t.Helper()

	var events []AgentEvent
	for ev := range ch {
		events = append(events, ev)
	}

	return events
}

// hasEventType checks if any event has the given type.
func hasEventType(events []AgentEvent, typ string) bool {
	for _, e := range events {
		if e.Type == typ {
			return true
		}
	}

	return false
}

// findEvent returns the first event of the given type, or nil.
func findEvent(events []AgentEvent, typ string) *AgentEvent {
	for i := range events {
		if events[i].Type == typ {
			return &events[i]
		}
	}

	return nil
}

// newTestAgentHandler builds an AgentHandler with the given mock provider
// and optional deps. The executor and registry are real (with a test handler
// registered for "content.search").
func newTestAgentHandler(t *testing.T, provider *mockAgentProvider, opts ...AgentOption) *AgentHandler {
	t.Helper()

	disp := NewDispatcher()
	disp.RegisterProvider("chat", provider)

	reg := NewToolHandlerRegistry()
	reg.RegisterConfig("content.search", ToolConfig{
		FunctionName: "content.search",
		Description:  "Search site content",
		Parameters: map[string]any{
			"type":     "object",
			"required": []any{"query"},
		},
	}, "", pgtype.UUID{})
	reg.RegisterHandler("content.search", func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		return &ToolResult{Status: ToolCallStatusSuccess, Content: `{"results":[]}`}, nil
	})

	exec := NewExecutor(
		reg,
		WithPermissionChecker(&stubPerms{granted: map[string]bool{"agent.use": true}}),
	)

	pre := NewPreSearcher(&ToolDeps{})

	h := NewAgentHandler(disp, exec, reg, pre, opts...)

	return h
}

// ---------------------------------------------------------------------------
// Pre-check tests (tasks 5.04-5.05, 5.10-5.11)
// ---------------------------------------------------------------------------

func TestHandleAgentRequest_AgentDisabled(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	h := newTestAgentHandler(
		t, provider,
		WithAgentSettings(&agentSettingsReader{
			bools: map[string]bool{"agent.enabled": false},
		}),
	)

	_, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err == nil {
		t.Fatal("expected error when agent is disabled, got nil")
	}

	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error should mention disabled: %v", err)
	}
}

func TestHandleAgentRequest_NoPermission(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	h := newTestAgentHandler(
		t, provider,
		WithAgentPerms(&stubPerms{granted: map[string]bool{}}), // no agent.use
	)

	_, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err == nil {
		t.Fatal("expected error when user lacks agent.use, got nil")
	}

	if !strings.Contains(err.Error(), "agent.use") {
		t.Errorf("error should mention agent.use: %v", err)
	}
}

func TestHandleAgentRequest_RateLimited(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	rl := &stubRateLimiter{count: 100} // already at limit
	h := newTestAgentHandler(
		t, provider,
		WithAgentRateLimiter(rl),
		WithAgentSettings(&agentSettingsReader{
			ints: map[string]int{"agent.rate_limit_per_day": 50},
		}),
	)

	_, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err == nil {
		t.Fatal("expected rate limit error, got nil")
	}

	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error should mention rate limit: %v", err)
	}
}

func TestHandleAgentRequest_RateLimiterFailsOpen(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	rl := &stubRateLimiter{failIncr: true}
	sessions := &stubSessionStore{}
	h := newTestAgentHandler(
		t, provider,
		WithAgentRateLimiter(rl),
		WithAgentSessions(sessions),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("expected request to proceed when rate limiter fails (fail-open), got error: %v", err)
	}

	events := collectEvents(t, ch)
	if !hasEventType(events, EventSessionStart) {
		t.Error("expected session_start event")
	}
}

func TestHandleAgentRequest_DailyBudgetExceeded(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	// Cost accumulator returns 6.00 USD — above the default 5.00 budget.
	rl := &stubRateLimiter{costVal: "6.00"}
	h := newTestAgentHandler(
		t, provider,
		WithAgentRateLimiter(rl),
		WithAgentSettings(&agentSettingsReader{
			floats: map[string]float64{"agent.daily_budget_usd": 5.00},
		}),
	)

	_, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err == nil {
		t.Fatal("expected budget exceeded error, got nil")
	}

	if !errors.Is(err, errAgentBudgetExceeded) {
		t.Errorf("expected errAgentBudgetExceeded, got: %v", err)
	}
}

func TestHandleAgentRequest_DailyBudgetWithinLimit(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	sessions := &stubSessionStore{}
	// Cost accumulator returns 3.00 USD — below the 5.00 budget.
	rl := &stubRateLimiter{costVal: "3.00"}
	h := newTestAgentHandler(
		t, provider,
		WithAgentRateLimiter(rl),
		WithAgentSessions(sessions),
		WithAgentSettings(&agentSettingsReader{
			floats: map[string]float64{"agent.daily_budget_usd": 5.00},
		}),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("expected request to proceed within budget, got error: %v", err)
	}

	events := collectEvents(t, ch)
	if !hasEventType(events, EventSessionStart) {
		t.Error("expected session_start event")
	}
}

func TestHandleAgentRequest_DailyBudgetDisabled(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	sessions := &stubSessionStore{}
	// Budget set to 0 disables the check even though cost exceeds any
	// reasonable limit.
	rl := &stubRateLimiter{costVal: "999.00"}
	h := newTestAgentHandler(
		t, provider,
		WithAgentRateLimiter(rl),
		WithAgentSessions(sessions),
		WithAgentSettings(&agentSettingsReader{
			floats: map[string]float64{"agent.daily_budget_usd": 0},
		}),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("expected request to proceed when budget disabled (0), got error: %v", err)
	}

	events := collectEvents(t, ch)
	if !hasEventType(events, EventSessionStart) {
		t.Error("expected session_start event")
	}
}

func TestHandleAgentRequest_DailyBudgetGetFailsOpen(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	sessions := &stubSessionStore{}
	// Valkey GET returns an error (e.g. redis.Nil on a fresh day) — the
	// budget check should fail-open and let the request proceed.
	rl := &stubRateLimiter{getErr: errors.New("redis nil")}
	h := newTestAgentHandler(
		t, provider,
		WithAgentRateLimiter(rl),
		WithAgentSessions(sessions),
		WithAgentSettings(&agentSettingsReader{
			floats: map[string]float64{"agent.daily_budget_usd": 5.00},
		}),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("expected request to proceed when budget GET fails (fail-open), got error: %v", err)
	}

	events := collectEvents(t, ch)
	if !hasEventType(events, EventSessionStart) {
		t.Error("expected session_start event")
	}
}

func TestHandleAgentRequest_EmptyPrompt(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{name: "mock"}
	h := newTestAgentHandler(t, provider)

	_, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "")
	if err == nil {
		t.Fatal("expected error for empty prompt, got nil")
	}
}

// ---------------------------------------------------------------------------
// LLM loop tests (tasks 5.08-5.09, 5.12)
// ---------------------------------------------------------------------------

func TestHandleAgentRequest_FinalResponseNoToolCalls(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{
		name: "mock",
		responses: []ChatWithToolsResponse{
			{Content: "Here is the answer.", FinishReason: "stop", Usage: TokenUsage{TotalTokens: 50}},
		},
	}
	sessions := &stubSessionStore{}
	h := newTestAgentHandler(t, provider, WithAgentSessions(sessions))

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "What is JapaConnect?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	if !hasEventType(events, EventSessionStart) {
		t.Error("expected session_start event")
	}

	if !hasEventType(events, EventFinalResponse) {
		t.Error("expected final_response event")
	}

	final := findEvent(events, EventFinalResponse)
	if final == nil {
		t.Fatal("expected final_response event")
	}

	if final.Content != "Here is the answer." {
		t.Errorf("final content = %q, want %q", final.Content, "Here is the answer.")
	}

	if final.TokensUsed != 50 {
		t.Errorf("tokens_used = %d, want 50", final.TokensUsed)
	}

	// Session should be completed.
	if sessions.completed == nil {
		t.Error("expected session to be completed")
	}

	if sessions.failed != nil {
		t.Error("did not expect session to be failed")
	}
}

func TestHandleAgentRequest_ToolCallThenFinalResponse(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{
		name: "mock",
		responses: []ChatWithToolsResponse{
			{
				Content:      "Let me search for that.",
				FinishReason: "tool_calls",
				ToolCalls: []ToolCall{
					{ID: "tc1", Name: "content.search", Arguments: json.RawMessage(`{"query":"visa"}`)},
				},
				Usage: TokenUsage{TotalTokens: 80},
			},
			{
				Content:      "Based on the search, here is the info.",
				FinishReason: "stop",
				Usage:        TokenUsage{TotalTokens: 120},
			},
		},
	}
	sessions := &stubSessionStore{}
	h := newTestAgentHandler(t, provider, WithAgentSessions(sessions))

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "Search for visa info")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	if !hasEventType(events, EventToolCallStart) {
		t.Error("expected tool_call_start event")
	}

	if !hasEventType(events, EventToolCallResult) {
		t.Error("expected tool_call_result event")
	}

	if !hasEventType(events, EventFinalResponse) {
		t.Error("expected final_response event")
	}

	// Verify tool call start event.
	tcStart := findEvent(events, EventToolCallStart)
	if tcStart == nil || tcStart.ToolName != "content.search" {
		t.Error("expected tool_call_start for content.search")
	}

	// Verify tool call result event.
	tcResult := findEvent(events, EventToolCallResult)
	if tcResult == nil || tcResult.Status != ToolCallStatusSuccess {
		t.Error("expected successful tool_call_result")
	}

	// Session completed with tool_call_count = 1.
	if sessions.completed == nil {
		t.Fatal("expected session to be completed")
	}

	if sessions.complete.ToolCallCount != 1 {
		t.Errorf("tool_call_count = %d, want 1", sessions.complete.ToolCallCount)
	}

	if sessions.complete.TotalTokens != 200 {
		t.Errorf("total_tokens = %d, want 200 (80+120)", sessions.complete.TotalTokens)
	}
}

func TestHandleAgentRequest_MaxIterationsExceeded(t *testing.T) {
	t.Parallel()

	// Provider always returns tool calls — never a final response.
	provider := &mockAgentProvider{
		name: "mock",
		responses: []ChatWithToolsResponse{
			{ToolCalls: []ToolCall{{ID: "tc1", Name: "content.search", Arguments: json.RawMessage(`{"query":"x"}`)}}, Usage: TokenUsage{TotalTokens: 50}},
			{ToolCalls: []ToolCall{{ID: "tc2", Name: "content.search", Arguments: json.RawMessage(`{"query":"x"}`)}}, Usage: TokenUsage{TotalTokens: 50}},
			{ToolCalls: []ToolCall{{ID: "tc3", Name: "content.search", Arguments: json.RawMessage(`{"query":"x"}`)}}, Usage: TokenUsage{TotalTokens: 50}},
		},
	}
	sessions := &stubSessionStore{}
	h := newTestAgentHandler(
		t, provider,
		WithAgentSessions(sessions),
		WithAgentSettings(&agentSettingsReader{
			ints: map[string]int{"agent.max_tool_calls": 2},
		}),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	// After max iterations, the loop exits with an error event (§103 says
	// session.Fail("max tool calls exceeded")).
	if !hasEventType(events, EventError) {
		t.Error("expected error event for max iterations exceeded")
	}

	errEv := findEvent(events, EventError)
	if errEv == nil || !strings.Contains(errEv.Error, "max tool calls exceeded") {
		t.Errorf("expected max tool calls error, got: %v", errEv)
	}

	// Session should be failed, not completed.
	if sessions.failed == nil {
		t.Fatal("expected session to be failed")
	}

	if sessions.completed != nil {
		t.Error("did not expect session to be completed")
	}
}

func TestHandleAgentRequest_LLMError(t *testing.T) {
	t.Parallel()

	// Override ChatWithTools to return an error.
	errorProvider := &errorAgentProvider{name: "err"}
	disp := NewDispatcher()
	disp.RegisterProvider("chat", errorProvider)

	reg := NewToolHandlerRegistry()
	exec := NewExecutor(reg)
	pre := NewPreSearcher(&ToolDeps{})
	sessions := &stubSessionStore{}

	h := NewAgentHandler(
		disp, exec, reg, pre,
		WithAgentSessions(sessions),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	if !hasEventType(events, EventError) {
		t.Error("expected error event")
	}

	if sessions.failed == nil {
		t.Error("expected session to be failed")
	}
}

// errorAgentProvider is a Provider that always returns an error from
// ChatWithTools.
type errorAgentProvider struct {
	name string
}

func (e *errorAgentProvider) Name() string                                            { return e.name }
func (e *errorAgentProvider) Models() []ModelInfo                                     { return nil }
func (e *errorAgentProvider) FetchModels(ctx context.Context) ([]ModelInfo, error)    { return nil, nil }
func (e *errorAgentProvider) VerifyWebhookSignature(payload []byte, sig string) error { return nil }
func (e *errorAgentProvider) Send(ctx context.Context, req Request) (Response, error) {
	return Response{}, errors.New("not supported")
}

func (e *errorAgentProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	return ChatWithToolsResponse{}, errors.New("LLM API is down")
}

// ---------------------------------------------------------------------------
// Cost limit test (task 5.10)
// ---------------------------------------------------------------------------

func TestHandleAgentRequest_CostLimitExceeded(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{
		name: "mock",
		responses: []ChatWithToolsResponse{
			{ToolCalls: []ToolCall{{ID: "tc1", Name: "content.search", Arguments: json.RawMessage(`{"query":"x"}`)}}, Usage: TokenUsage{TotalTokens: 100000}},
			{Content: "Done.", Usage: TokenUsage{TotalTokens: 100000}},
		},
	}
	sessions := &stubSessionStore{}
	h := newTestAgentHandler(
		t, provider,
		WithAgentSessions(sessions),
		WithAgentSettings(&agentSettingsReader{
			floats: map[string]float64{
				"agent.cost_limit_per_request_usd": 0.01, // very low limit
				"agent.cost_per_1k_tokens":         0.002,
			},
		}),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	// 100000 tokens * 0.002/1000 = 0.20 > 0.01 limit → error.
	if !hasEventType(events, EventError) {
		t.Error("expected error event for cost limit exceeded")
	}

	errEv := findEvent(events, EventError)
	if errEv == nil || !strings.Contains(errEv.Error, "cost limit") {
		t.Errorf("expected cost limit error, got: %v", errEv)
	}

	if sessions.failed == nil {
		t.Error("expected session to be failed")
	}
}

// ---------------------------------------------------------------------------
// Geo context test (task 5.05)
// ---------------------------------------------------------------------------

func TestHandleAgentRequest_GeoContextResolved(t *testing.T) {
	t.Parallel()

	cityName := "Berlin"
	countryName := "Germany"
	countryCode := "DE"
	geoID := mustUUID(t, "00000000-0000-0000-0000-000000000099")

	provider := &mockAgentProvider{
		name: "mock",
		responses: []ChatWithToolsResponse{
			{Content: "Done.", FinishReason: "stop", Usage: TokenUsage{TotalTokens: 10}},
		},
	}
	sessions := &stubSessionStore{}
	geo := &stubGeoResolver{
		row: gen.GetUserCurrentGeoSpaceRow{
			CurrentGeoSpaceID: geoID,
			CityName:          cityName,
			CountryName:       &countryName,
			CountryGeoCode:    &countryCode,
		},
	}
	h := newTestAgentHandler(
		t, provider,
		WithAgentSessions(sessions),
		WithAgentGeoResolver(geo),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	// The session_start event should carry the pre-search level.
	startEv := findEvent(events, EventSessionStart)
	if startEv == nil {
		t.Fatal("expected session_start event")
	}

	if startEv.PresearchLevel == "" {
		t.Error("expected non-empty presearch_level in session_start")
	}

	// Session should have the geo_space_id set.
	if !sessions.created.GeoSpaceID.Valid {
		t.Error("expected session to have a valid geo_space_id")
	}

	if sessions.created.GeoSpaceID.String() != geoID.String() {
		t.Errorf("geo_space_id = %s, want %s", sessions.created.GeoSpaceID, geoID)
	}
}

// ---------------------------------------------------------------------------
// Tool catalog cache test (task 5.06)
// ---------------------------------------------------------------------------

func TestHandleAgentRequest_ToolCatalogCached(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{
		name: "mock",
		responses: []ChatWithToolsResponse{
			{Content: "Done.", FinishReason: "stop", Usage: TokenUsage{TotalTokens: 10}},
		},
	}
	sessions := &stubSessionStore{}
	cache := newStubCacheClient()
	h := newTestAgentHandler(
		t, provider,
		WithAgentSessions(sessions),
		WithAgentCache(cache),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	collectEvents(t, ch)

	// The tool catalog should be cached under jc:agent:tools:{user_id}.
	found := false

	for k := range cache.store {
		if strings.HasPrefix(k, "jc:agent:tools:") {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected tool catalog to be cached in jc:agent:tools:{user_id}")
	}
}

// ---------------------------------------------------------------------------
// System prompt test (task 5.07)
// ---------------------------------------------------------------------------

func TestBuildSystemPrompt_ContainsBehaviorRules(t *testing.T) {
	t.Parallel()

	prompt := buildSystemPrompt("User's geo context: City: Berlin, Country: Germany.", &PreSearchResult{Level: PreSearchFull}, []ToolConfig{
		{FunctionName: "content.search", Description: "Search site content"},
	})

	// Check for key behavior rules from §103.
	mustContain := []string{
		"You are Japa AI",
		"BEHAVIOR RULES",
		"NEVER create duplicate content",
		"privacy_blocked",
		"User's geo context",
		"PRE-SEARCH RESULTS",
		"AVAILABLE TOOLS",
		"content.search",
	}
	for _, s := range mustContain {
		if !strings.Contains(prompt, s) {
			t.Errorf("system prompt missing %q", s)
		}
	}
}

func TestBuildSystemPrompt_NoGeoNoPreSearch(t *testing.T) {
	t.Parallel()

	prompt := buildSystemPrompt("", nil, nil)
	if !strings.Contains(prompt, "You are Japa AI") {
		t.Error("system prompt should always contain the preamble")
	}

	if strings.Contains(prompt, "PRE-SEARCH RESULTS") {
		t.Error("system prompt should not contain pre-search results when nil")
	}

	if strings.Contains(prompt, "AVAILABLE TOOLS") {
		t.Error("system prompt should not contain tools section when empty")
	}
}

// ---------------------------------------------------------------------------
// Event ordering test (task 5.12)
// ---------------------------------------------------------------------------

func TestHandleAgentRequest_EventOrdering(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{
		name: "mock",
		responses: []ChatWithToolsResponse{
			{ToolCalls: []ToolCall{{ID: "tc1", Name: "content.search", Arguments: json.RawMessage(`{"query":"x"}`)}}, Usage: TokenUsage{TotalTokens: 50}},
			{Content: "Done.", FinishReason: "stop", Usage: TokenUsage{TotalTokens: 50}},
		},
	}
	sessions := &stubSessionStore{}
	h := newTestAgentHandler(t, provider, WithAgentSessions(sessions))

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	// Expected order: session_start → progress → tool_call_start →
	// tool_call_result → progress → final_response.
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(events))
	}

	if events[0].Type != EventSessionStart {
		t.Errorf("event[0] = %q, want %q", events[0].Type, EventSessionStart)
	}

	// The last event should be final_response.
	if events[len(events)-1].Type != EventFinalResponse {
		t.Errorf("last event = %q, want %q", events[len(events)-1].Type, EventFinalResponse)
	}

	// tool_call_start should come before tool_call_result.
	startIdx, resultIdx := -1, -1

	for i, e := range events {
		if e.Type == EventToolCallStart {
			startIdx = i
		}

		if e.Type == EventToolCallResult {
			resultIdx = i
		}
	}

	if startIdx == -1 || resultIdx == -1 {
		t.Fatal("expected both tool_call_start and tool_call_result events")
	}

	if startIdx >= resultIdx {
		t.Error("tool_call_start should come before tool_call_result")
	}
}

// ---------------------------------------------------------------------------
// Timeout test (task 5.10)
// ---------------------------------------------------------------------------

func TestHandleAgentRequest_TimeoutEnforced(t *testing.T) {
	t.Parallel()

	// Provider that sleeps longer than the timeout.
	slowProvider := &slowAgentProvider{name: "slow", delay: 200 * time.Millisecond}
	disp := NewDispatcher()
	disp.RegisterProvider("chat", slowProvider)

	reg := NewToolHandlerRegistry()
	exec := NewExecutor(reg)
	pre := NewPreSearcher(&ToolDeps{})
	sessions := &stubSessionStore{}

	h := NewAgentHandler(
		disp, exec, reg, pre,
		WithAgentSessions(sessions),
		WithAgentSettings(&agentSettingsReader{
			ints: map[string]int{"agent.timeout_seconds": 0}, // 0 seconds → immediate timeout
		}),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	// The loop should time out and emit an error event.
	if !hasEventType(events, EventError) {
		t.Error("expected error event due to timeout")
	}
}

// ---------------------------------------------------------------------------
// User cancellation test (task 5.09 — CancelAgentSession)
// ---------------------------------------------------------------------------

func TestHandleAgentRequest_UserCancellation(t *testing.T) {
	t.Parallel()

	provider := &mockAgentProvider{
		name: "mock",
		responses: []ChatWithToolsResponse{
			{ToolCalls: []ToolCall{{ID: "tc1", Name: "content.search", Arguments: json.RawMessage(`{"query":"x"}`)}}, Usage: TokenUsage{TotalTokens: 50}},
			{Content: "Done.", FinishReason: "stop", Usage: TokenUsage{TotalTokens: 50}},
		},
	}
	sessions := &stubSessionStore{}
	// cancellationCache always returns "cancelled" for session-state keys,
	// so isCancelled returns true on the very first loop iteration. This
	// avoids the race between the goroutine checking isCancelled and the
	// test setting the cache key.
	cache := &cancellationCache{store: make(map[string]string)}
	h := newTestAgentHandler(
		t, provider,
		WithAgentSessions(sessions),
		WithAgentCache(cache),
	)

	ch, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{}, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectEvents(t, ch)

	// The loop should detect cancellation and emit an error event.
	if !hasEventType(events, EventError) {
		t.Error("expected error event for user cancellation")
	}

	errEv := findEvent(events, EventError)
	if errEv == nil || !strings.Contains(errEv.Error, "cancelled") {
		t.Errorf("expected cancellation error, got: %v", errEv)
	}

	// Session should be cancelled (status = 'cancelled'), not failed or completed.
	if sessions.cancelled == nil {
		t.Fatal("expected session to be cancelled")
	}

	if sessions.failed != nil {
		t.Error("did not expect session to be failed (cancellation is not failure)")
	}

	if sessions.completed != nil {
		t.Error("did not expect session to be completed")
	}
}

// cancellationCache is a CacheClient that always returns "cancelled" for
// session-state keys (jc:agent:session:*). This makes isCancelled return
// true without a race, since the goroutine can't check isCancelled before
// the cache is configured.
type cancellationCache struct {
	store map[string]string
}

func (c *cancellationCache) Get(ctx context.Context, key string) (string, error) {
	if strings.HasPrefix(key, "jc:agent:session:") {
		return "cancelled", nil
	}

	if v, ok := c.store[key]; ok {
		return v, nil
	}

	return "", errors.New("not found")
}

func (c *cancellationCache) Set(ctx context.Context, key, value string, ttlSeconds int) error {
	c.store[key] = value
	return nil
}

// slowAgentProvider is a Provider that delays its ChatWithTools response.
type slowAgentProvider struct {
	name  string
	delay time.Duration
}

func (s *slowAgentProvider) Name() string                                            { return s.name }
func (s *slowAgentProvider) Models() []ModelInfo                                     { return nil }
func (s *slowAgentProvider) FetchModels(ctx context.Context) ([]ModelInfo, error)    { return nil, nil }
func (s *slowAgentProvider) VerifyWebhookSignature(payload []byte, sig string) error { return nil }
func (s *slowAgentProvider) Send(ctx context.Context, req Request) (Response, error) {
	return Response{}, errors.New("not supported")
}

func (s *slowAgentProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	select {
	case <-time.After(s.delay):
		return ChatWithToolsResponse{Content: "Done.", Usage: TokenUsage{TotalTokens: 10}}, nil
	case <-ctx.Done():
		return ChatWithToolsResponse{}, ctx.Err()
	}
}
