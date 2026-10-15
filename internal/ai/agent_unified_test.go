package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// recordingProvider captures the ChatWithToolsRequest of each call so tests
// can assert on the message list and tool definitions the loop sent.
type recordingProvider struct {
	name  string
	reply string
	reqs  []ChatWithToolsRequest
}

func (m *recordingProvider) Name() string        { return m.name }
func (m *recordingProvider) Models() []ModelInfo { return nil }
func (m *recordingProvider) FetchModels(context.Context) ([]ModelInfo, error) {
	return nil, nil
}
func (m *recordingProvider) VerifyWebhookSignature([]byte, string) error { return nil }
func (m *recordingProvider) Send(context.Context, Request) (Response, error) {
	return Response{}, errors.New("mock: Send not supported")
}
func (m *recordingProvider) ChatWithTools(_ context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	m.reqs = append(m.reqs, req)
	return ChatWithToolsResponse{
		Content:      m.reply,
		FinishReason: "stop",
		Usage:        TokenUsage{TotalTokens: 42},
	}, nil
}

// gatedProvider blocks ChatWithTools until release is closed; a cancelled
// ctx aborts the call like a real LLM client would. Needed for detach
// tests — the instant recordingProvider makes cancel races
// nondeterministic.
type gatedProvider struct {
	recordingProvider
	release chan struct{}
}

func (p *gatedProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	p.reqs = append(p.reqs, req)
	select {
	case <-p.release:
	case <-ctx.Done():
		return ChatWithToolsResponse{}, ctx.Err()
	}
	return ChatWithToolsResponse{
		Content:      p.reply,
		FinishReason: "stop",
		Usage:        TokenUsage{TotalTokens: 42},
	}, nil
}

// countingRateLimiter tracks Incr/IncrByFloat calls per key so tests can
// assert which jc:agent:* / jc:ai:* counters did (not) fire.
type countingRateLimiter struct {
	incrKeys  map[string]int64
	floatKeys map[string]float64
}

func newCountingRateLimiter() *countingRateLimiter {
	return &countingRateLimiter{incrKeys: map[string]int64{}, floatKeys: map[string]float64{}}
}

func (s *countingRateLimiter) Incr(_ context.Context, key string) (int64, error) {
	s.incrKeys[key]++
	return s.incrKeys[key], nil
}
func (s *countingRateLimiter) IncrByFloat(_ context.Context, key string, v float64) (float64, error) {
	s.floatKeys[key] += v
	return s.floatKeys[key], nil
}
func (s *countingRateLimiter) Expire(context.Context, string, int) error { return nil }
func (s *countingRateLimiter) Get(context.Context, string) (string, error) {
	return "", errors.New("not set")
}

// stubPreSearcher returns a canned pre-search result.
type stubPreSearcher struct{ res *PreSearchResult }

func (s *stubPreSearcher) PreSearch(context.Context, string, *pgtype.UUID, pgtype.UUID) (*PreSearchResult, error) {
	return s.res, nil
}

// newUnifiedHandler builds an AgentHandler for unified-path tests with a
// stub conversation store, recording provider, and given perms.
func newUnifiedHandler(t *testing.T, provider Provider, perms *stubPerms, opts ...AgentOption) (*AgentHandler, *stubConvoStore, *countingRateLimiter) {
	t.Helper()

	disp := NewDispatcher()
	disp.RegisterProvider("chat", provider)

	reg := NewToolHandlerRegistry()
	reg.RegisterConfigFull("content.search", ToolConfig{
		FunctionName: "content.search",
		Description:  "Search site content",
	}, "", pgtype.UUID{}, "content")

	exec := NewExecutor(reg)
	convos := NewConversationService(newStubConvoStore(), nil)
	limiter := newCountingRateLimiter()

	all := append([]AgentOption{
		WithAgentPerms(perms),
		WithAgentConversations(convos),
		WithAgentRateLimiter(limiter),
		WithAgentSessions(&stubSessionStore{}),
	}, opts...)

	h := NewAgentHandler(disp, exec, reg, &stubPreSearcher{res: &PreSearchResult{Level: PreSearchNone}}, all...)

	st := convos.store.(*stubConvoStore)
	return h, st, limiter
}

func agentRateKeyCount(l *countingRateLimiter) int64 {
	for k, v := range l.incrKeys {
		if len(k) > 4 && k[:4] != "" && containsStr(k, "agent:rate:") {
			return v
		}
	}
	return 0
}

// floatKeyTotal sums IncrByFloat values across keys containing substr.
func floatKeyTotal(l *countingRateLimiter, substr string) float64 {
	var total float64
	for k, v := range l.floatKeys {
		if containsStr(k, substr) {
			total += v
		}
	}
	return total
}

func containsStr(hay, needle string) bool {
	return len(needle) > 0 && len(hay) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(hay); i++ {
			if hay[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func TestUnifiedChat_MultiTurnHistory(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "first answer"}
	h, st, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	ev1, err := h.HandleUnifiedChat(t.Context(), uid, UnifiedChatRequest{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, ev1)
	convEvent := findEvent(events, EventConversation)
	if convEvent == nil || convEvent.ConversationID == "" {
		t.Fatal("expected conversation event with conversation_id")
	}
	final := findEvent(events, EventFinalResponse)
	if final == nil || final.MessageID == "" {
		t.Fatal("expected final_response with message_id")
	}
	convID, _ := scanUUID(convEvent.ConversationID)

	// Second turn on the same thread — provider must see turn 1 in history.
	provider.reply = "second answer"
	ev2, err := h.HandleUnifiedChat(t.Context(), uid, UnifiedChatRequest{Prompt: "follow-up", ConversationID: &convID})
	if err != nil {
		t.Fatal(err)
	}
	collectEvents(t, ev2)

	if len(provider.reqs) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(provider.reqs))
	}
	msgs := provider.reqs[1].Messages
	// system + user1 + assistant1 + user2
	if len(msgs) != 4 || msgs[1].Content != "hello" || msgs[2].Content != "first answer" || msgs[3].Content != "follow-up" {
		t.Fatalf("history not threaded: %+v", msgs)
	}

	// Transcript now holds 4 messages.
	if got := len(st.msgs[uuidStr(convID)]); got != 4 {
		t.Fatalf("expected 4 transcript rows, got %d", got)
	}
}

func TestUnifiedChat_ToolsDisabledSkipsAgentCounters(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	// User holds agent.use, so eligibility is on — but the per-message
	// override forces tools off.
	h, _, limiter := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"agent.use": true}})
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	off := false
	ev, err := h.HandleUnifiedChat(t.Context(), uid, UnifiedChatRequest{
		Prompt: "just chat",
		Tools:  &ToolSelection{Enabled: &off},
	})
	if err != nil {
		t.Fatal(err)
	}
	collectEvents(t, ev)

	if len(provider.reqs) != 1 {
		t.Fatalf("expected 1 LLM call, got %d", len(provider.reqs))
	}
	if len(provider.reqs[0].Tools) != 0 {
		t.Fatalf("expected empty tool defs, got %+v", provider.reqs[0].Tools)
	}
	if got := agentRateKeyCount(limiter); got != 0 {
		t.Fatalf("jc:agent:rate must not increment on pure-chat turns, got %d", got)
	}
}

func TestUnifiedChat_ToolEnabledTurnRunsAgentCounters(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, limiter := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"agent.use": true}})

	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "search stuff"})
	if err != nil {
		t.Fatal(err)
	}
	collectEvents(t, ev)

	if len(provider.reqs) == 0 || len(provider.reqs[0].Tools) != 1 {
		t.Fatalf("expected 1 tool def, got %+v", provider.reqs)
	}
	if got := agentRateKeyCount(limiter); got != 1 {
		t.Fatalf("expected jc:agent:rate increment, got %d", got)
	}
}

func TestUnifiedChat_SourcesBeforeFinal(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	h.presearch = &stubPreSearcher{res: &PreSearchResult{
		Level: PreSearchPartial,
		RAGChunks: []RAGChunk{
			{EntityType: "content", EntityID: "x", Title: "Visa Guide", Slug: "visa", URL: "/wiki/visa", Similarity: 0.9},
		},
	}}

	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, ev)

	srcIdx, finIdx := -1, -1
	for i, e := range events {
		if e.Type == EventSources && srcIdx == -1 {
			srcIdx = i
		}
		if e.Type == EventFinalResponse {
			finIdx = i
		}
	}
	if srcIdx == -1 || finIdx == -1 || srcIdx > finIdx {
		t.Fatalf("sources must precede final_response: %+v", events)
	}
	src := events[srcIdx]
	if len(src.Sources) != 1 || src.Sources[0].URL != "/wiki/visa" {
		t.Fatalf("bad sources payload: %+v", src.Sources)
	}
}

func TestUnifiedChat_PermissionOrGate(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}

	// agent.use-only user (no ai.context.use) is admitted — legacy /ai/agent
	// only required agent.use.
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"agent.use": true}})
	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("agent-only user must be admitted: %v", err)
	}
	collectEvents(t, ev)

	// ai.context.use-only user (no agent.use) — chat allowed, tools empty.
	provider2 := &recordingProvider{name: "mock", reply: "ok"}
	h2, _, limiter := newUnifiedHandler(t, provider2, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	ev2, err := h2.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("chat-only user must be admitted: %v", err)
	}
	collectEvents(t, ev2)
	if len(provider2.reqs) == 0 || len(provider2.reqs[0].Tools) != 0 {
		t.Fatal("chat-only user must get an empty catalog")
	}
	if agentRateKeyCount(limiter) != 0 {
		t.Fatal("chat-only turn must not consume jc:agent:rate")
	}

	// Neither permission → denied.
	h3, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{}})
	_, err = h3.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{3}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if !errors.Is(err, errAINoPermission) {
		t.Fatalf("expected errAINoPermission, got %v", err)
	}
}

func TestUnifiedChat_DeniedDoesNotConsumeQueries(t *testing.T) {
	// ADR-130: the permission OR-gate runs before CheckBudget — a
	// permissionless request must not increment jc:ai:queries.
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, limiter := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{}})
	h.preflight = NewPreflight(nil, nil, nil, limiter, nil)

	_, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if !errors.Is(err, errAINoPermission) {
		t.Fatalf("expected errAINoPermission, got %v", err)
	}
	for k := range limiter.incrKeys {
		if containsStr(k, "ai:queries:") {
			t.Fatalf("denied request consumed shared query quota: %s", k)
		}
	}
}

func TestUnifiedChat_ConversationBusy(t *testing.T) {
	// A second send while a turn is running on the same thread → 409-shaped
	// error, no quota consumed (busy check runs before CheckBudget).
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, st, limiter := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	h.preflight = NewPreflight(nil, nil, nil, limiter, nil)

	convID := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	st.busyConvs = map[string]bool{uuidStr(convID): true}

	_, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{
		Prompt: "hi again", ConversationID: &convID,
	})
	if !errors.Is(err, errConversationBusy) {
		t.Fatalf("expected errConversationBusy, got %v", err)
	}
	for k := range limiter.incrKeys {
		if containsStr(k, "ai:queries:") {
			t.Fatalf("busy rejection consumed shared query quota: %s", k)
		}
	}
}

func TestUnifiedChat_SessionCreateUniqueViolationMapsToBusy(t *testing.T) {
	// A send that slips past the busy pre-check still 409s: the unique index
	// on ai_agent_sessions surfaces as a pg unique violation.
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	h.sessions = &stubSessionStore{createErr: &pgconn.PgError{Code: "23505"}}

	_, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if !errors.Is(err, errConversationBusy) {
		t.Fatalf("unique violation should map to errConversationBusy, got %v", err)
	}
}

func TestUnifiedChat_SessionCreateSweepsStaleRunningRow(t *testing.T) {
	// A crashed turn leaves a 'running' session row that trips the
	// one-running-per-conversation unique index even though the busy
	// pre-check treats it as stale. The handler must sweep stale rows and
	// retry once — the send succeeds instead of 409ing forever.
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	swept := int64(1)
	h.sessions = &stubSessionStore{
		createErr:     &pgconn.PgError{Code: "23505"},
		createErrOnce: true,
		staleFail:     &swept,
	}

	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("stale-running sweep should let the send proceed, got %v", err)
	}
	if !hasEventType(collectEvents(t, ev), EventFinalResponse) {
		t.Fatal("expected final_response after stale sweep + retry")
	}
}

func TestUnifiedChat_TurnCountEvents(t *testing.T) {
	// conversation + final_response carry the server-side persisted message
	// count so the sidebar assigns rather than increments.
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, st, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})

	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	// Seed a thread with two prior turns.
	conv, err := st.CreateAIConversation(t.Context(), gen.CreateAIConversationParams{UserID: uid, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	st.InsertAIMessage(t.Context(), gen.InsertAIMessageParams{ConversationID: conv.ID, Role: "user", Content: "u1"})
	st.InsertAIMessage(t.Context(), gen.InsertAIMessageParams{ConversationID: conv.ID, Role: "assistant", Content: "a1"})

	ev, err := h.HandleUnifiedChat(t.Context(), uid, UnifiedChatRequest{Prompt: "next", ConversationID: &conv.ID})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, ev)

	var convEv, finalEv *AgentEvent
	for i := range events {
		switch events[i].Type {
		case EventConversation:
			convEv = &events[i]
		case EventFinalResponse:
			finalEv = &events[i]
		}
	}
	if convEv == nil || convEv.TurnCount != 3 {
		t.Fatalf("conversation event turn_count should be history(2)+user(1)=3, got %+v", convEv)
	}
	if finalEv == nil || finalEv.TurnCount != 4 {
		t.Fatalf("final_response turn_count should be 4, got %+v", finalEv)
	}
}

func TestUnifiedChat_NilRegistryDegrade(t *testing.T) {
	// A nil tool registry must degrade to pure chat, not panic — the
	// handler dereferences registry.Categories() during catalog resolution.
	provider := &recordingProvider{name: "mock", reply: "ok"}
	disp := NewDispatcher()
	disp.RegisterProvider("chat", provider)
	exec := NewExecutor(NewToolHandlerRegistry())

	h := NewAgentHandler(disp, exec, nil, &stubPreSearcher{res: &PreSearchResult{Level: PreSearchNone}},
		WithAgentPerms(&stubPerms{granted: map[string]bool{"agent.use": true}}),
		WithAgentConversations(NewConversationService(newStubConvoStore(), nil)),
		WithAgentSessions(&stubSessionStore{}))

	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("nil registry should degrade to chat, got %v", err)
	}
	events := collectEvents(t, ev)
	if !hasEventType(events, EventFinalResponse) {
		t.Fatalf("expected final_response, got %+v", events)
	}
	if len(provider.reqs) == 0 || len(provider.reqs[0].Tools) != 0 {
		t.Fatal("nil registry must produce an empty catalog")
	}
}

func TestUnifiedChat_GlobalToolsKillSwitch(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, limiter := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"agent.use": true}},
		WithAgentSettings(&agentSettingsReader{bools: map[string]bool{"agent.enabled": false}}))

	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("feature.ai_agent off must still allow chat: %v", err)
	}
	collectEvents(t, ev)
	if len(provider.reqs) == 0 || len(provider.reqs[0].Tools) != 0 {
		t.Fatal("kill-switch must empty the catalog")
	}
	if agentRateKeyCount(limiter) != 0 {
		t.Fatal("no tools → no agent counter")
	}
}

func TestUnifiedChat_PureChatRecordsSharedCostOnly(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, limiter := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"agent.use": true}})

	// Wire the shared §20 preflight with the same stub limiter so RecordCost
	// lands on observable keys.
	h.preflight = NewPreflight(nil, nil, nil, limiter, nil)

	off := false
	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{
		Prompt: "just chat",
		Tools:  &ToolSelection{Enabled: &off},
	})
	if err != nil {
		t.Fatal(err)
	}
	collectEvents(t, ev)

	if got := floatKeyTotal(limiter, "agent:cost:"); got != 0 {
		t.Fatalf("pure chat must not write jc:agent:cost, got %.4f", got)
	}
	if got := floatKeyTotal(limiter, "ai:cost:"); got <= 0 {
		t.Fatalf("pure chat must record shared jc:ai:cost, got %.4f", got)
	}
}

func TestUnifiedChat_ToolTurnRecordsAgentAndSharedCost(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, limiter := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"agent.use": true}})
	h.preflight = NewPreflight(nil, nil, nil, limiter, nil)

	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "use tools"})
	if err != nil {
		t.Fatal(err)
	}
	collectEvents(t, ev)

	if got := floatKeyTotal(limiter, "agent:cost:"); got <= 0 {
		t.Fatalf("tool-enabled turn must write jc:agent:cost, got %.4f", got)
	}
	if got := floatKeyTotal(limiter, "ai:cost:"); got <= 0 {
		t.Fatalf("tool-enabled turn must record shared jc:ai:cost, got %.4f", got)
	}
}

func TestUnifiedChat_PersistsVoiceID(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, st, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})

	ev, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{
		Prompt:  "hi",
		VoiceID: "voice-abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, ev)
	final := findEvent(events, EventFinalResponse)
	if final == nil || final.MessageID == "" {
		t.Fatal("expected persisted assistant message")
	}
	var found *string
	for _, msgs := range st.msgs {
		for i := range msgs {
			if msgs[i].ID.String() == final.MessageID {
				found = msgs[i].VoiceID
			}
		}
	}
	if found == nil || *found != "voice-abc" {
		t.Fatalf("voice_id not persisted: %+v", found)
	}
}

func TestUnifiedChat_InvalidModel(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}},
		WithAgentModelValidator(func(model, category string) bool { return model == "good-model" }))

	_, err := h.HandleUnifiedChat(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, UnifiedChatRequest{Prompt: "hi", Model: "bogus"})
	if !errors.Is(err, errInvalidModel) {
		t.Fatalf("expected errInvalidModel, got %v", err)
	}
}

func TestHandleAgentRequest_LegacyPathStillWorks(t *testing.T) {
	// Legacy wrapper: no unified deps needed; agent.use required.
	provider := &mockAgentProvider{name: "mock"}
	h := newTestAgentHandler(t, provider,
		WithAgentPerms(&stubPerms{granted: map[string]bool{"agent.use": true}}))

	ev, err := h.HandleAgentRequest(t.Context(), pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, "hi")
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, ev)
	if !hasEventType(events, EventSessionStart) || !hasEventType(events, EventFinalResponse) {
		t.Fatalf("legacy turn missing events: %+v", events)
	}
}

// A tools-on turn (agent.use → non-empty catalog → detached) survives
// client disconnect: the loop completes and the terminal event still
// lands for a connected reader.
func TestUnifiedChat_DetachedTurnCompletesAfterCancel(t *testing.T) {
	provider := &gatedProvider{recordingProvider: recordingProvider{name: "mock", reply: "done"}, release: make(chan struct{})}
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"agent.use": true}})
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	ctx, cancel := context.WithCancel(context.Background())
	events, err := h.HandleUnifiedChat(ctx, uid, UnifiedChatRequest{Prompt: "run a task"})
	if err != nil {
		t.Fatal(err)
	}

	// session_start proves the loop is parked on the gated LLM call.
	if first := <-events; first.Type != EventSessionStart {
		t.Fatalf("expected session_start first, got %v", first.Type)
	}
	cancel()                // client disconnects
	close(provider.release) // the (detached) LLM call can now return

	got := []AgentEvent{}
	done := make(chan struct{})
	go func() {
		for ev := range events {
			got = append(got, ev)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("detached loop stalled")
	}
	if findEvent(got, EventFinalResponse) == nil {
		t.Fatalf("expected final_response after disconnect, got %+v", got)
	}
}

// Control: a pure-chat turn (ai.context.use only → empty catalog →
// attached) stays request-bound — cancelling mid-loop fails the run
// instead of finishing it.
func TestUnifiedChat_ChatTurnStillAbortsOnCancel(t *testing.T) {
	provider := &gatedProvider{recordingProvider: recordingProvider{name: "mock", reply: "done"}, release: make(chan struct{})}
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	ctx, cancel := context.WithCancel(context.Background())
	events, err := h.HandleUnifiedChat(ctx, uid, UnifiedChatRequest{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	<-events // session_start — loop is parked on the gated call
	cancel()
	close(provider.release) // provider sees ctx.Done and fails the call

	got := []AgentEvent{}
	for ev := range events {
		got = append(got, ev)
	}
	if findEvent(got, EventFinalResponse) != nil {
		t.Fatal("attached chat turn must not complete after disconnect")
	}
}
