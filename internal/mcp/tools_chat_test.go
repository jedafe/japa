package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/db/gen"
)

// chatStubProvider is a minimal ai.Provider for japa.chat tests — the loop
// calls ChatWithTools once and takes the canned reply.
type chatStubProvider struct{ reply string }

func (p *chatStubProvider) Name() string { return "stub" }
func (p *chatStubProvider) Models() []ai.ModelInfo {
	return nil
}
func (p *chatStubProvider) FetchModels(context.Context) ([]ai.ModelInfo, error) {
	return nil, nil
}
func (p *chatStubProvider) VerifyWebhookSignature([]byte, string) error { return nil }
func (p *chatStubProvider) Send(context.Context, ai.Request) (ai.Response, error) {
	return ai.Response{}, errors.New("stub: Send not supported")
}
func (p *chatStubProvider) ChatWithTools(context.Context, ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error) {
	return ai.ChatWithToolsResponse{
		Content:      p.reply,
		FinishReason: "stop",
		Usage:        ai.TokenUsage{TotalTokens: 7},
	}, nil
}

type chatStubPresearch struct{}

func (chatStubPresearch) PreSearch(context.Context, string, *pgtype.UUID, pgtype.UUID) (*ai.PreSearchResult, error) {
	return &ai.PreSearchResult{Level: ai.PreSearchNone}, nil
}

// chatStubSessions satisfies ai.SessionStore (the MCP surface shares the
// §103 audit contract — the engine creates its own session per turn).
type chatStubSessions struct{ id pgtype.UUID }

func (s *chatStubSessions) CreateAgentSession(context.Context, gen.CreateAgentSessionParams) (gen.AiAgentSession, error) {
	return gen.AiAgentSession{ID: s.id, Status: "running"}, nil
}
func (s *chatStubSessions) CompleteAgentSession(_ context.Context, arg gen.CompleteAgentSessionParams) (gen.AiAgentSession, error) {
	return gen.AiAgentSession{ID: arg.ID, Status: "completed"}, nil
}
func (s *chatStubSessions) FailAgentSession(_ context.Context, arg gen.FailAgentSessionParams) (gen.AiAgentSession, error) {
	return gen.AiAgentSession{ID: arg.ID, Status: "failed"}, nil
}
func (s *chatStubSessions) CancelAgentSession(_ context.Context, id pgtype.UUID) (gen.AiAgentSession, error) {
	return gen.AiAgentSession{ID: id, Status: "cancelled"}, nil
}

// newChatServer builds an MCP Server with the unified engine wired: an
// AgentHandler on a stub provider, empty registry (pure chat turn), and
// perms granting ai.context.use.
func newChatServer(t *testing.T) (*Server, *testDeps) {
	t.Helper()
	srv, deps := newTestMCP(t)

	disp := ai.NewDispatcher()
	disp.RegisterProvider("chat", &chatStubProvider{reply: "hello from japa"})
	reg := ai.NewToolHandlerRegistry()
	exec := ai.NewExecutor(reg)

	deps.perms.granted["ai.context.use"] = true

	agent := ai.NewAgentHandler(disp, exec, reg, chatStubPresearch{},
		ai.WithAgentPerms(deps.perms),
		ai.WithAgentSessions(&chatStubSessions{id: mustUUID("00000000-0000-0000-0000-0000000000aa")}),
	)
	srv.SetAgentHandler(agent)
	return srv, deps
}

func TestMCP_JapaChat_ListedForJWTCaller(t *testing.T) {
	srv, _ := newChatServer(t)
	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1,
		"method": "tools/list", "params": {}
	}`)

	tools := out["result"].(map[string]any)["tools"].([]any)
	found := false
	for _, tool := range tools {
		if tool.(map[string]any)["name"].(string) == "japa.chat" {
			found = true
		}
	}
	if !found {
		t.Fatal("japa.chat missing from tools/list for a JWT caller")
	}
}

func TestMCP_JapaChat_HiddenFromAPIKeyCaller(t *testing.T) {
	srv, _ := newChatServer(t)
	ctx := context.WithValue(withUser(t.Context(), testUserID), apiKeyCallerKey, true)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1,
		"method": "tools/list", "params": {}
	}`)

	tools := out["result"].(map[string]any)["tools"].([]any)
	for _, tool := range tools {
		if tool.(map[string]any)["name"].(string) == "japa.chat" {
			t.Fatal("japa.chat must not be listed for API-key/OAuth callers")
		}
	}

	// And calling it is denied before dispatch.
	_, out = postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 2,
		"method": "tools/call",
		"params": {"name": "japa.chat", "arguments": {"message": "hi"}}
	}`)
	if out["error"] == nil {
		t.Fatal("API-key caller must not invoke japa.chat")
	}
	errObj := out["error"].(map[string]any)
	if !strings.Contains(errObj["data"].(string), "not available") {
		t.Fatalf("unexpected denial: %v", errObj)
	}
}

func TestMCP_JapaChat_DrainsUnifiedEngine(t *testing.T) {
	srv, _ := newChatServer(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 3,
		"method": "tools/call",
		"params": {"name": "japa.chat", "arguments": {"message": "what is japa?"}}
	}`)

	if status != http.StatusOK || out["error"] != nil {
		t.Fatalf("call failed: status=%d out=%v", status, out)
	}
	res := out["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("isError result: %v", res)
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)

	var payload struct {
		Reply          string `json:"reply"`
		ConversationID string `json:"conversation_id"`
		SessionID      string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal chat result: %v — %s", err, text)
	}
	if payload.Reply != "hello from japa" {
		t.Fatalf("reply mismatch: %q", payload.Reply)
	}
	if payload.SessionID == "" {
		t.Fatal("session_id missing — §103 audit contract broken")
	}
}

func TestMCP_JapaChat_NilAgent_NotAvailable(t *testing.T) {
	srv, _ := newTestMCP(t) // no SetAgentHandler — engine unwired
	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1,
		"method": "tools/list", "params": {}
	}`)
	for _, tool := range out["result"].(map[string]any)["tools"].([]any) {
		if tool.(map[string]any)["name"].(string) == "japa.chat" {
			t.Fatal("japa.chat listed without an engine")
		}
	}

	_, out = postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 2,
		"method": "tools/call",
		"params": {"name": "japa.chat", "arguments": {"message": "hi"}}
	}`)
	if out["error"] == nil {
		t.Fatal("japa.chat call should fail when the engine is unwired")
	}
}

func TestMCP_JapaChat_MessageRequired(t *testing.T) {
	srv, _ := newChatServer(t)
	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 4,
		"method": "tools/call",
		"params": {"name": "japa.chat", "arguments": {}}
	}`)
	errObj := out["error"].(map[string]any)
	if !strings.Contains(errObj["data"].(string), "message is required") {
		t.Fatalf("expected message-required error, got %v", errObj)
	}
}

func TestMCP_JapaChat_PermissionDenied(t *testing.T) {
	srv, deps := newChatServer(t)
	delete(deps.perms.granted, "ai.context.use")
	delete(deps.perms.granted, "agent.use")
	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 5,
		"method": "tools/call",
		"params": {"name": "japa.chat", "arguments": {"message": "hi"}}
	}`)
	if out["error"] == nil {
		t.Fatal("unified admission gate must deny callers without ai.context.use/agent.use")
	}
}
