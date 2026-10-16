package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/publicapi"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	os.Exit(m.Run())
}

var testUserID = mustUUID("00000000-0000-0000-0000-000000000001")

type testDeps struct {
	registry *ai.ToolHandlerRegistry
	executor *ai.Executor
	perms    *stubPerms
	privacy  *stubPrivacy
	sessions *stubSessions
}

func newTestMCP(t *testing.T) (*Server, *testDeps) {
	t.Helper()

	registry := ai.NewToolHandlerRegistry()
	perms := &stubPerms{granted: map[string]bool{}}
	privacy := &stubPrivacy{}
	executor := ai.NewExecutor(registry, ai.WithPermissionChecker(perms), ai.WithPrivacyChecker(privacy))
	sessions := &stubSessions{}

	// Register a simple echo tool and an HITL-emitting tool.
	registry.RegisterConfig("test.echo", ai.ToolConfig{
		FunctionName: "test.echo",
		Description:  "Echo the provided message",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
			"required": []any{"message"},
		},
	}, "", pgtype.UUID{})
	registry.RegisterHandler("test.echo", func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ai.ToolResult, error) {
		var a struct {
			Message string `json:"message"`
		}

		if err := json.Unmarshal(args, &a); err != nil {
			return &ai.ToolResult{Status: ai.ToolCallStatusError, Content: fmt.Sprintf(`{"error":%q}`, err.Error())}, nil
		}

		return &ai.ToolResult{Status: ai.ToolCallStatusSuccess, Content: fmt.Sprintf(`{"echo":%q}`, a.Message)}, nil
	})

	registry.RegisterConfig("test.hitl", ai.ToolConfig{
		FunctionName: "test.hitl",
		Description:  "Queue for HITL review",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, "", pgtype.UUID{})

	reviewID := mustUUID("00000000-0000-0000-0000-000000000003")

	registry.RegisterHandler("test.hitl", func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ai.ToolResult, error) {
		return &ai.ToolResult{
			Status:  ai.ToolCallStatusHITLQueued,
			Content: `{"created":true}`,
			Metadata: map[string]any{
				"hitl_review_id": reviewID,
			},
		}, nil
	})

	return NewServer(executor, registry, sessions, perms), &testDeps{
		registry: registry,
		executor: executor,
		perms:    perms,
		privacy:  privacy,
		sessions: sessions,
	}
}

func mustUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		panic(err)
	}

	return u
}

func withUser(ctx context.Context, userID pgtype.UUID) context.Context {
	return context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: userID,
	})
}

func postJSON(t *testing.T, handler http.Handler, ctx context.Context, body string) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var out map[string]any
	if rr.Body.Len() > 0 {
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
	}

	return rr.Code, out
}

func TestMCP_Initialize(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"}
		}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	if out["id"].(float64) != 1 {
		t.Fatalf("expected id 1, got %v", out["id"])
	}

	res := out["result"].(map[string]any)
	if res["protocolVersion"] != "2024-11-05" {
		t.Fatalf("unexpected protocol version: %v", res["protocolVersion"])
	}

	caps := res["capabilities"].(map[string]any)

	tools := caps["tools"].(map[string]any)
	if tools["listChanged"] != false {
		t.Fatalf("expected listChanged=false, got %v", tools["listChanged"])
	}
}

func TestMCP_InitializedNotification(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"method": "notifications/initialized"
	}`)

	if status != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %v", status, out)
	}

	if out != nil {
		t.Fatalf("notification should not return a body: %v", out)
	}
}

func TestMCP_ToolsList(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/list",
		"params": {}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	res := out["result"].(map[string]any)

	tools := res["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	echo := tools[0].(map[string]any)
	if echo["name"] != "test.echo" {
		t.Fatalf("expected test.echo, got %v", echo["name"])
	}

	schema := echo["inputSchema"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("expected schema type object, got %v", schema["type"])
	}
}

func TestMCP_ToolsCall(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 3,
		"method": "tools/call",
		"params": {
			"name": "test.echo",
			"arguments": {"message": "hello"}
		}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	res := out["result"].(map[string]any)

	content := res["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}

	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"hello"`) {
		t.Fatalf("expected echo hello in text, got %s", text)
	}

	if res["isError"] == true {
		t.Fatalf("expected isError false, got %v", res["isError"])
	}
}

func TestMCP_ToolsCall_UnknownTool(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 4,
		"method": "tools/call",
		"params": {
			"name": "test.unknown",
			"arguments": {}
		}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("expected -32602, got %v", errObj["code"])
	}
}

func TestMCP_ToolsCall_HITLQueued(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 5,
		"method": "tools/call",
		"params": {
			"name": "test.hitl",
			"arguments": {}
		}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	res := out["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)

	var payload map[string]any

	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("text is not valid JSON: %v", err)
	}

	if payload["status"] != "hitl_queued" {
		t.Fatalf("expected status hitl_queued, got %v", payload["status"])
	}

	if payload["hitl_review_id"] == "" {
		t.Fatalf("expected hitl_review_id in payload")
	}

	if res["isError"] == true {
		t.Fatalf("expected isError false for HITL-queued result")
	}
}

func TestMCP_Unauthenticated(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := t.Context()

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 6,
		"method": "tools/list",
		"params": {}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32001 {
		t.Fatalf("expected -32001, got %v", errObj["code"])
	}
}

func TestMCP_UnknownMethod(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 7,
		"method": "foo.bar",
		"params": {}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32601 {
		t.Fatalf("expected -32601, got %v", errObj["code"])
	}
}

func TestMCP_InvalidJSON(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 8,
		"method": `)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32700 {
		t.Fatalf("expected -32700, got %v", errObj["code"])
	}
}

func TestMCP_GETStream(t *testing.T) {
	srv, _ := newTestMCP(t)

	ctx, cancel := context.WithCancel(withUser(t.Context(), testUserID))
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	// The handler blocks on the request context, so cancel it after a short
	// delay so the test does not hang.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	srv.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	if !strings.Contains(rr.Body.String(), "event: endpoint") {
		t.Fatalf("expected endpoint event in body, got %q", rr.Body.String())
	}
}

// DELETE is the streamable-HTTP session-termination verb; a stateless
// server answers 405 rather than a 200-wrapped JSON-RPC error.
func TestMCP_UnsupportedMethod(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}

	if allow := rr.Header().Get("Allow"); allow != "GET, POST" {
		t.Fatalf("expected Allow: GET, POST, got %q", allow)
	}
}

func TestMCP_Initialize_2025_11_25(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2025-11-25",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"}
		}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}

	result := out["result"].(map[string]any)
	// A supported request version is echoed back (spec negotiation).
	if result["protocolVersion"] != "2025-11-25" {
		t.Fatalf("expected negotiated protocol version 2025-11-25, got %q", result["protocolVersion"])
	}

	if result["instructions"] == "" {
		t.Fatal("expected initialize to advertise server instructions")
	}
}

// Claude/ChatGPT connectors request 2025-06-18; an unknown version must
// still negotiate to a supported one rather than erroring (MCP lifecycle).
func TestMCP_Initialize_2025_06_18_AndFallback(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": {"protocolVersion": "2025-06-18", "capabilities": {},
			"clientInfo": {"name": "claude", "version": "1.0"}}
	}`)
	if res := out["result"].(map[string]any); res["protocolVersion"] != "2025-06-18" {
		t.Fatalf("expected 2025-06-18 echoed, got %q", res["protocolVersion"])
	}

	// Unknown version → server answers with its newest supported version.
	_, out = postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 2, "method": "initialize",
		"params": {"protocolVersion": "2099-01-01", "capabilities": {},
			"clientInfo": {"name": "future", "version": "1.0"}}
	}`)

	res := out["result"].(map[string]any)
	if res["protocolVersion"] != latestProtocolVersion {
		t.Fatalf("expected fallback to %q, got %q", latestProtocolVersion, res["protocolVersion"])
	}
}

func TestMCP_PingAndNotificationHandling(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	status, out := postJSON(t, srv, ctx, `{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	if status != http.StatusOK {
		t.Fatalf("ping: expected 200, got %d", status)
	}

	if _, ok := out["result"]; !ok {
		t.Fatalf("ping: expected empty result, got %v", out)
	}

	// Unknown notifications must be silently accepted, never error-responded.
	status, out = postJSON(t, srv, ctx, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{}}`)
	if status != http.StatusAccepted || out != nil {
		t.Fatalf("notification: expected 202/empty, got %d %v", status, out)
	}
}

// Streamable HTTP permits JSON-RPC batches; verify mixed request +
// notification batches produce a response array and notification-only
// batches produce 202.
func TestMCP_BatchRequests(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(`[
		{"jsonrpc":"2.0","method":"notifications/initialized"},
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
	]`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("batch: expected 200, got %d", rec.Code)
	}

	var batch []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&batch); err != nil {
		t.Fatalf("batch: decode: %v", err)
	}

	if len(batch) != 2 {
		t.Fatalf("batch: expected 2 responses (notification skipped), got %d", len(batch))
	}

	// Notification-only batch → 202, no body.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(`[
		{"jsonrpc":"2.0","method":"notifications/initialized"},
		{"jsonrpc":"2.0","method":"notifications/cancelled","params":{}}
	]`))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("notification batch: expected 202, got %d", rec.Code)
	}

	// DELETE is the streamable session-termination verb → 405, not a
	// JSON-RPC error body.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/mcp", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE: expected 405, got %d", rec.Code)
	}
}

func TestAPIKeyCallerGetsReadOnlyCatalog(t *testing.T) {
	srv, deps := newTestMCP(t)
	_ = deps

	// register a write tool to prove it is filtered
	deps.registry.RegisterConfig("content.create", ai.ToolConfig{FunctionName: "content.create"}, "", pgtype.UUID{})
	deps.registry.RegisterConfig("content.read", ai.ToolConfig{FunctionName: "content.read"}, "", pgtype.UUID{})
	deps.registry.RegisterConfig("destination.brief", ai.ToolConfig{FunctionName: "destination.brief"}, "", pgtype.UUID{})
	deps.registry.RegisterConfig("persona_voices.list", ai.ToolConfig{FunctionName: "persona_voices.list"}, "", pgtype.UUID{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AuthenticatedUser{UserID: testUserID})
	ctx = context.WithValue(ctx, apiKeyCallerKey, true)
	// API key with only api.read.content — no api.read.brief scope.
	ctx = publicapi.WithAPIKeyContext(ctx, &publicapi.APIKeyContext{
		UserID: testUserID, Scopes: []string{"agent.use", "api.read.content"},
	})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req.WithContext(ctx))

	var resp map[string]any

	json.NewDecoder(rec.Body).Decode(&resp)
	tools := resp["result"].(map[string]any)["tools"].([]any)
	seen := map[string]bool{}

	for _, tool := range tools {
		name := tool.(map[string]any)["name"].(string)
		seen[name] = true

		if name == "content.create" {
			t.Fatal("write tool leaked to API-key caller")
		}

		if name == "destination.brief" {
			t.Fatal("premium tool leaked to under-scoped key")
		}
	}

	if !seen["content.read"] {
		t.Fatal("expected content.read to remain visible")
	}

	if !seen["persona_voices.list"] {
		t.Fatal("expected persona_voices.list to remain visible to agent.use-scoped key")
	}
}

// stubTierResolver is a stub publicapi.TierResolver for premium-gate tests.
type stubTierResolver struct{ tier string }

func (s stubTierResolver) ResolveUserTier(_ context.Context, _ pgtype.UUID) (string, error) {
	return s.tier, nil
}

// apiKeyCtx marks ctx as an API-key caller carrying the given scopes.
func apiKeyCtx(ctx context.Context, scopes ...string) context.Context {
	ctx = context.WithValue(ctx, apiKeyCallerKey, true)

	return publicapi.WithAPIKeyContext(ctx, &publicapi.APIKeyContext{
		UserID: testUserID, Scopes: scopes,
	})
}

func registerPhase2Tools(deps *testDeps) {
	for _, name := range []string{"feeds.explore", "feeds.for_you", "scam.list", "scam.get", "search.semantic", "rag.search"} {
		deps.registry.RegisterConfig(name, ai.ToolConfig{FunctionName: name}, "", pgtype.UUID{})
	}
}

// Phase 2 (spec 2026-09-24-agent-data-api-phase2): the five new tools appear
// in an API-key caller's catalog only when the key carries the tool's scope.
func TestPhase2ToolGateScopeFiltering(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerPhase2Tools(deps)

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "api.read.content")

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/list",
		"params": {}
	}`)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	tools := out["result"].(map[string]any)["tools"].([]any)
	seen := map[string]bool{}

	for _, tool := range tools {
		seen[tool.(map[string]any)["name"].(string)] = true
	}

	if !seen["feeds.explore"] {
		t.Fatal("expected feeds.explore visible to api.read.content key")
	}

	for _, name := range []string{"feeds.for_you", "scam.list", "scam.get", "search.semantic"} {
		if seen[name] {
			t.Fatalf("tool %s leaked to under-scoped key", name)
		}
	}
}

// tools/call on a tool whose scope the key lacks is denied before execution.
func TestPhase2ToolGateScopeDenial(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerPhase2Tools(deps)

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "api.read.content")

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {"name": "scam.list", "arguments": {}}
	}`)

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("expected -32602, got %v", errObj["code"])
	}

	data, _ := errObj["data"].(string)
	if !strings.Contains(data, "api.read.safety") {
		t.Fatalf("expected scope denial naming api.read.safety, got %q", data)
	}
}

// search.semantic is premium: an API key carrying api.read.search still hits
// the minTier gate when the owner resolves to regular.
func TestPhase2SemanticTierDenialAPIKey(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerPhase2Tools(deps)
	srv.SetTierResolver(stubTierResolver{tier: "regular"})

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "api.read.search")

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {"name": "search.semantic", "arguments": {"query": "visa"}}
	}`)

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("expected -32602, got %v", errObj["code"])
	}

	data, _ := errObj["data"].(string)
	if !strings.Contains(data, "pro") {
		t.Fatalf("expected tier denial naming pro, got %q", data)
	}
}

// The minTier gate applies to JWT callers too — a regular-tier session user
// cannot reach search.semantic even though API-key scope checks don't apply.
func TestPhase2SemanticTierDenialJWT(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerPhase2Tools(deps)
	srv.SetTierResolver(stubTierResolver{tier: "regular"})

	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {"name": "search.semantic", "arguments": {"query": "visa"}}
	}`)

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("expected -32602, got %v", errObj["code"])
	}

	data, _ := errObj["data"].(string)
	if !strings.Contains(data, "pro") {
		t.Fatalf("expected tier denial naming pro, got %q", data)
	}
}

// rag.search is restored to the API-key allowlist under the same gate as
// search.semantic (api.read.search + pro) now that retrieval is
// viewer-scoped to the key owner (§69 scoped-retrieval amendment).
func TestRAGSearchGateForAPIKeys(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerPhase2Tools(deps)

	// Under-scoped key (content only): not visible in the catalog.
	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "api.read.content")

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}
	}`)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	for _, tool := range out["result"].(map[string]any)["tools"].([]any) {
		if tool.(map[string]any)["name"] == "rag.search" {
			t.Fatal("rag.search leaked to api.read.content-scoped key")
		}
	}

	// Scoped but regular tier: tools/call denied on minTier.
	srv.SetTierResolver(stubTierResolver{tier: "regular"})

	ctx = apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "api.read.search")
	_, out = postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": {"name": "rag.search", "arguments": {"query": "visa"}}
	}`)

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("expected -32602, got %v", errObj["code"])
	}

	if data, _ := errObj["data"].(string); !strings.Contains(data, "pro") {
		t.Fatalf("expected tier denial naming pro, got %q", data)
	}

	// Scoped + pro: the call reaches the handler.
	srv.SetTierResolver(stubTierResolver{tier: "pro"})
	deps.registry.RegisterHandler("rag.search", func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ai.ToolResult, error) {
		return &ai.ToolResult{Status: ai.ToolCallStatusSuccess, Content: `{"results":[]}`}, nil
	})

	_, out = postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": {"name": "rag.search", "arguments": {"query": "visa"}}
	}`)
	if out["error"] != nil {
		t.Fatalf("expected rag.search call to pass the gate, got %v", out["error"])
	}

	if res := out["result"].(map[string]any); res["isError"] == true {
		t.Fatalf("expected successful result, got %v", res)
	}
}

// A pro-tier JWT caller passes the gate — the call reaches the executor
// (which then fails only because no handler is registered in the test rig).
func TestPhase2SemanticTierAllowed(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerPhase2Tools(deps)
	srv.SetTierResolver(stubTierResolver{tier: "pro"})
	deps.registry.RegisterHandler("search.semantic", func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ai.ToolResult, error) {
		return &ai.ToolResult{Status: ai.ToolCallStatusSuccess, Content: `{"result":{"data":[]}}`}, nil
	})

	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {"name": "search.semantic", "arguments": {"query": "visa"}}
	}`)

	if out["error"] != nil {
		t.Fatalf("expected tool call to pass the gate, got %v", out["error"])
	}

	res := out["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("expected successful result, got %v", res)
	}
}

// §119 v1.3: an API key carrying the admin-only agent.write scope bypasses
// the read-only gate map and sees the owner's full RBAC-filtered catalog —
// write tools included, no api.read.* scope requirements.
func TestAgentWriteKeyGetsFullCatalog(t *testing.T) {
	srv, deps := newTestMCP(t)

	deps.registry.RegisterConfig("content.create", ai.ToolConfig{FunctionName: "content.create"}, "", pgtype.UUID{})
	deps.registry.RegisterConfig("content.read", ai.ToolConfig{FunctionName: "content.read"}, "", pgtype.UUID{})
	deps.registry.RegisterConfig("scam.list", ai.ToolConfig{FunctionName: "scam.list"}, "", pgtype.UUID{})

	// Note: no api.read.* scopes at all — elevation replaces the gate map.
	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "agent.write")

	status, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}
	}`)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, out)
	}

	seen := map[string]bool{}
	for _, tool := range out["result"].(map[string]any)["tools"].([]any) {
		seen[tool.(map[string]any)["name"].(string)] = true
	}

	for _, name := range []string{"content.create", "content.read", "scam.list"} {
		if !seen[name] {
			t.Fatalf("expected %s visible to agent.write key", name)
		}
	}
}

// The elevated key can execute write tools through the executor — the call
// is gated by the owner's RBAC, not the fixed allowlist.
func TestAgentWriteKeyCanCallWriteTool(t *testing.T) {
	srv, deps := newTestMCP(t)

	deps.registry.RegisterConfig("content.create", ai.ToolConfig{FunctionName: "content.create"}, "", pgtype.UUID{})
	deps.registry.RegisterHandler("content.create", func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ai.ToolResult, error) {
		return &ai.ToolResult{Status: ai.ToolCallStatusSuccess, Content: `{"content":{"id":"abc"}}`}, nil
	})
	// A tool the owner's RBAC does not permit stays unreachable even with
	// agent.write — elevation is not an RBAC bypass.
	deps.registry.RegisterConfig("admin.thing", ai.ToolConfig{FunctionName: "admin.thing"}, "admin.thing", pgtype.UUID{})

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "agent.write")

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": {"name": "content.create", "arguments": {"title": "t", "body": "b"}}
	}`)
	if out["error"] != nil {
		t.Fatalf("expected write call to pass the gate, got %v", out["error"])
	}

	if res := out["result"].(map[string]any); res["isError"] == true {
		t.Fatalf("expected successful write result, got %v", res)
	}

	_, out = postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": {"name": "admin.thing", "arguments": {}}
	}`)

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("expected -32602 for RBAC-denied tool, got %v", errObj)
	}
}

// Elevation does not lift premium tier gates: an agent.write key owned by a
// regular-tier user is still denied minTier tools.
func TestAgentWriteKeyStillTierGated(t *testing.T) {
	srv, deps := newTestMCP(t)
	deps.registry.RegisterConfig("destination.brief", ai.ToolConfig{FunctionName: "destination.brief"}, "", pgtype.UUID{})
	srv.SetTierResolver(stubTierResolver{tier: "regular"})

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "agent.write")

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": {"name": "destination.brief", "arguments": {}}
	}`)

	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("expected -32602, got %v", errObj)
	}

	if data, _ := errObj["data"].(string); !strings.Contains(data, "pro") {
		t.Fatalf("expected tier denial naming pro, got %q", data)
	}
}

// agent.write must stay an AdminOnly catalog scope: it is what keeps it off
// self-serve mints and OAuth grants. If it ever becomes non-admin, DCR
// clients could consent-mint write-capable jcpat_* tokens — this test fails
// before that regression ships (§119 v1.3, §120 scope ceiling).
func TestAgentWriteStaysAdminOnly(t *testing.T) {
	if !publicapi.AnyAdminOnly([]string{"agent.write"}) {
		t.Fatal("agent.write must be AdminOnly in the scope catalog")
	}

	for _, s := range publicapi.SelfServiceScopes() {
		if s.Value == "agent.write" {
			t.Fatal("agent.write leaked into the self-serve scope catalog")
		}
	}

	for _, s := range oauthGrantableScopes() {
		if s == "agent.write" {
			t.Fatal("agent.write leaked into OAuth grantable scopes")
		}
	}

	granted := filterGrantableScopes([]string{"agent.use", "agent.write", "*"})
	for _, s := range granted {
		if s == "agent.write" {
			t.Fatal("agent.write must never be grantable via OAuth DCR")
		}
	}
}

type stubPerms struct {
	granted map[string]bool
}

func (s *stubPerms) HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool {
	return s.granted[permKey]
}

type stubPrivacy struct{}

func (s *stubPrivacy) IsExempt(ctx context.Context, entityType ai.PrivacyEntityType, entityID pgtype.UUID) (bool, error) {
	return false, nil
}

type stubSessions struct {
	created    int
	completed  int
	failed     int
	sessionIDs []pgtype.UUID
}

func (s *stubSessions) CreateAgentSession(ctx context.Context, arg gen.CreateAgentSessionParams) (gen.AiAgentSession, error) {
	s.created++
	id := mustUUID(uuid.NewString())
	s.sessionIDs = append(s.sessionIDs, id)

	return gen.AiAgentSession{ID: id, UserID: arg.UserID, Status: "running"}, nil
}

func (s *stubSessions) CompleteAgentSession(ctx context.Context, arg gen.CompleteAgentSessionParams) (gen.AiAgentSession, error) {
	s.completed++
	return gen.AiAgentSession{ID: arg.ID, Status: "completed"}, nil
}

func (s *stubSessions) FailAgentSession(ctx context.Context, arg gen.FailAgentSessionParams) (gen.AiAgentSession, error) {
	s.failed++
	return gen.AiAgentSession{ID: arg.ID, Status: "failed"}, nil
}
