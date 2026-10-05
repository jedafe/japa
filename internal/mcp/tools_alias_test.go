package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/publicapi"
)

// registerAliasTargets stubs content.search and content.read so the §120
// search/fetch aliases have a real catalog tool to dispatch to.
func registerAliasTargets(deps *testDeps) {
	deps.registry.RegisterConfig("content.search", ai.ToolConfig{
		FunctionName: "content.search",
		Description:  "Search content",
		Parameters:   map[string]any{"type": "object"},
	}, "", pgtype.UUID{})
	deps.registry.RegisterHandler("content.search", func(_ context.Context, args json.RawMessage, _ pgtype.UUID) (*ai.ToolResult, error) {
		var a struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return &ai.ToolResult{Status: ai.ToolCallStatusError, Content: `{"error":"bad args"}`}, nil
		}
		return &ai.ToolResult{
			Status:  ai.ToolCallStatusSuccess,
			Content: `{"results":[{"id":"c1","title":"Study in Canada","slug":"study-in-canada","content_type":"post"}],"query":"` + a.Query + `"}`,
		}, nil
	})

	deps.registry.RegisterConfig("content.read", ai.ToolConfig{
		FunctionName: "content.read",
		Description:  "Read content",
		Parameters:   map[string]any{"type": "object"},
	}, "", pgtype.UUID{})
	deps.registry.RegisterHandler("content.read", func(_ context.Context, args json.RawMessage, _ pgtype.UUID) (*ai.ToolResult, error) {
		var a struct {
			ContentID string `json:"content_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return &ai.ToolResult{Status: ai.ToolCallStatusError, Content: `{"error":"bad args"}`}, nil
		}
		return &ai.ToolResult{
			Status:  ai.ToolCallStatusSuccess,
			Content: `{"content":{"id":"` + a.ContentID + `","title":"Study in Canada","slug":"study-in-canada","body":"Full body text","content_type":"post","created_at":"2026-01-01T00:00:00Z"}}`,
		}, nil
	})
}

func TestMCP_AliasToolsList(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerAliasTargets(deps)
	srv.SetSiteBaseURL("https://japaconnect.com")
	ctx := withUser(t.Context(), testUserID)

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
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.(map[string]any)["name"].(string)] = true
	}
	if !names["search"] || !names["fetch"] {
		t.Fatalf("aliases missing from tools/list: %v", names)
	}
}

func TestMCP_AliasToolsListHiddenWithoutUnderlying(t *testing.T) {
	srv, _ := newTestMCP(t) // no content.search/content.read registered
	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/list",
		"params": {}
	}`)

	tools := out["result"].(map[string]any)["tools"].([]any)
	for _, tool := range tools {
		name := tool.(map[string]any)["name"].(string)
		if name == "search" || name == "fetch" {
			t.Fatalf("alias %q listed without underlying tool", name)
		}
	}
}

func TestMCP_AliasSearchCall(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerAliasTargets(deps)
	srv.SetSiteBaseURL("https://japaconnect.com")
	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/call",
		"params": {"name": "search", "arguments": {"query": "canada visa"}}
	}`)

	if out["error"] != nil {
		t.Fatalf("call failed: %v", out["error"])
	}
	res := out["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("isError result: %v", res)
	}

	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	var payload struct {
		Results []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal alias result: %v — %s", err, text)
	}
	if len(payload.Results) != 1 || payload.Results[0].ID != "c1" {
		t.Fatalf("unexpected results: %v", payload.Results)
	}
	if payload.Results[0].URL != "https://japaconnect.com/content/study-in-canada" {
		t.Fatalf("unexpected url: %q", payload.Results[0].URL)
	}
}

func TestMCP_AliasFetchCall(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerAliasTargets(deps)
	srv.SetSiteBaseURL("https://japaconnect.com")
	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 3,
		"method": "tools/call",
		"params": {"name": "fetch", "arguments": {"id": "c1"}}
	}`)

	if out["error"] != nil {
		t.Fatalf("call failed: %v", out["error"])
	}
	res := out["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("isError result: %v", res)
	}

	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	var payload struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Text  string `json:"text"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal fetch result: %v — %s", err, text)
	}
	if payload.ID != "c1" || payload.Text != "Full body text" {
		t.Fatalf("unexpected fetch payload: %v", payload)
	}
	if payload.URL != "https://japaconnect.com/content/study-in-canada" {
		t.Fatalf("unexpected url: %q", payload.URL)
	}
}

func TestMCP_AliasRequiresArgs(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerAliasTargets(deps)
	ctx := withUser(t.Context(), testUserID)

	for _, call := range []string{
		`{"name": "search", "arguments": {}}`,
		`{"name": "fetch", "arguments": {"id": " "}}`,
	} {
		_, out := postJSON(t, srv, ctx, `{
			"jsonrpc": "2.0",
			"id": 4,
			"method": "tools/call",
			"params": `+call+`
		}`)
		if out["error"] == nil {
			t.Fatalf("expected invalid-params error for %s, got %v", call, out)
		}
	}
}

func TestMCP_AliasDeniedForAPIKeyWithoutScope(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerAliasTargets(deps)
	ctx := withUser(t.Context(), testUserID)
	// API-key caller without api.read.content — alias gate must deny.
	ctx = publicapi.WithAPIKeyContext(ctx, &publicapi.APIKeyContext{
		UserID: testUserID,
		Scopes: []string{"agent.use"},
	})
	ctx = context.WithValue(ctx, apiKeyCallerKey, true)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0",
		"id": 5,
		"method": "tools/call",
		"params": {"name": "search", "arguments": {"query": "x"}}
	}`)

	if out["error"] == nil {
		t.Fatalf("expected scope denial, got %v", out)
	}
	errObj := out["error"].(map[string]any)
	detail, _ := errObj["data"].(string)
	if detail == "" {
		detail, _ = errObj["message"].(string)
	}
	if !strings.Contains(detail, "api.read.content") {
		t.Fatalf("expected scope in denial message, got %q", detail)
	}
}
