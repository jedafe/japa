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
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	os.Exit(m.Run())
}

var (
	testUserID   = mustUUID("00000000-0000-0000-0000-000000000001")
	testTenantID = mustUUID("00000000-0000-0000-0000-000000000002")
)

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
	registry.RegisterHandler("test.echo", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ai.ToolResult, error) {
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
	registry.RegisterHandler("test.hitl", func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ai.ToolResult, error) {
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

func withUser(ctx context.Context, userID, tenantID pgtype.UUID) context.Context {
	return context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID:   userID,
		TenantID: tenantID,
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
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	ctx := context.Background()

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
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	ctx, cancel := context.WithCancel(withUser(context.Background(), testUserID, testTenantID))
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

func TestMCP_UnsupportedMethod(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(context.Background(), testUserID, testTenantID)

	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj := out["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32600 {
		t.Fatalf("expected -32600, got %v", errObj["code"])
	}
}

func TestMCP_Initialize_2025_11_25(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(context.Background(), testUserID, testTenantID)

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
	if result["protocolVersion"] != protocolVersion {
		t.Fatalf("expected advertised protocol version %q, got %q", protocolVersion, result["protocolVersion"])
	}
}

type stubPerms struct {
	granted map[string]bool
}

func (s *stubPerms) HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool {
	return s.granted[permKey]
}

type stubPrivacy struct{}

func (s *stubPrivacy) IsExempt(ctx context.Context, entityType ai.PrivacyEntityType, tenantID, entityID pgtype.UUID) (bool, error) {
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
	return gen.AiAgentSession{ID: id, TenantID: arg.TenantID, UserID: arg.UserID, Status: "running"}, nil
}

func (s *stubSessions) CompleteAgentSession(ctx context.Context, arg gen.CompleteAgentSessionParams) (gen.AiAgentSession, error) {
	s.completed++
	return gen.AiAgentSession{ID: arg.ID, TenantID: arg.TenantID, Status: "completed"}, nil
}

func (s *stubSessions) FailAgentSession(ctx context.Context, arg gen.FailAgentSessionParams) (gen.AiAgentSession, error) {
	s.failed++
	return gen.AiAgentSession{ID: arg.ID, TenantID: arg.TenantID, Status: "failed"}, nil
}
