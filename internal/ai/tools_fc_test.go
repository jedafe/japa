// Package ai — tools_fc_test.go covers the function-calling wire-format
// translation functions and provider-level ChatWithTools HTTP parsing
// (Section 103, Phase 1).
//
// These tests complement gateway_test.go (which tests the Dispatcher) by
// exercising the actual message/tool conversion logic and the HTTP
// request/response cycle in each provider.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 1, Task 1.08)
package ai

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// toOpenAIToolsMessages — Wire Format Translation
// ---------------------------------------------------------------------------

func TestToOpenAIToolsMessagesSystemRole(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hi"},
	}

	out := toOpenAIToolsMessages(msgs)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}

	if out[0].Role != "system" {
		t.Errorf("out[0] role = %q, want %q", out[0].Role, "system")
	}

	if out[0].Content != "You are helpful." {
		t.Errorf("out[0] content = %q, want %q", out[0].Content, "You are helpful.")
	}
}

func TestToOpenAIToolsMessagesAssistantWithToolCalls(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{
			Role:    "assistant",
			Content: "Let me search.",
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "content.search", Arguments: json.RawMessage(`{"q":"test"}`)},
			},
		},
	}

	out := toOpenAIToolsMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}

	if out[0].Role != "assistant" {
		t.Errorf("role = %q, want %q", out[0].Role, "assistant")
	}

	if out[0].Content != "Let me search." {
		t.Errorf("content = %q, want %q", out[0].Content, "Let me search.")
	}

	if len(out[0].ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(out[0].ToolCalls))
	}

	if out[0].ToolCalls[0].ID != "call_1" {
		t.Errorf("tool_call id = %q, want %q", out[0].ToolCalls[0].ID, "call_1")
	}

	if out[0].ToolCalls[0].Type != "function" {
		t.Errorf("tool_call type = %q, want %q", out[0].ToolCalls[0].Type, "function")
	}

	if out[0].ToolCalls[0].Function.Name != "content.search" {
		t.Errorf("tool_call name = %q, want %q", out[0].ToolCalls[0].Function.Name, "content.search")
	}

	if out[0].ToolCalls[0].Function.Arguments != `{"q":"test"}` {
		t.Errorf("tool_call args = %q, want %q", out[0].ToolCalls[0].Function.Arguments, `{"q":"test"}`)
	}
}

func TestToOpenAIToolsMessagesToolResultRole(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{Role: "tool", Content: `{"results":[]}`, ToolCallID: "call_1", Name: "content.search"},
	}

	out := toOpenAIToolsMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}

	if out[0].Role != "tool" {
		t.Errorf("role = %q, want %q", out[0].Role, "tool")
	}

	if out[0].Content != `{"results":[]}` {
		t.Errorf("content = %q, want %q", out[0].Content, `{"results":[]}`)
	}

	if out[0].ToolCallID != "call_1" {
		t.Errorf("tool_call_id = %q, want %q", out[0].ToolCallID, "call_1")
	}

	if out[0].Name != "content.search" {
		t.Errorf("name = %q, want %q", out[0].Name, "content.search")
	}
}

func TestToOpenAIToolsMessagesEmpty(t *testing.T) {
	t.Parallel()

	out := toOpenAIToolsMessages(nil)
	if len(out) != 0 {
		t.Errorf("len = %d, want 0 for nil input", len(out))
	}
}

// ---------------------------------------------------------------------------
// toOpenAIToolDefs — Tool Definition Conversion
// ---------------------------------------------------------------------------

func TestToOpenAIToolDefsEmpty(t *testing.T) {
	t.Parallel()

	out := toOpenAIToolDefs(nil)
	if out != nil {
		t.Errorf("expected nil for empty input, got %v", out)
	}
}

func TestToOpenAIToolDefsConversion(t *testing.T) {
	t.Parallel()

	tools := []LLMToolDefinition{
		{
			Name:        "rag.search",
			Description: "Search knowledge base",
			Parameters:  map[string]any{"type": "object"},
		},
	}

	out := toOpenAIToolDefs(tools)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}

	if out[0].Type != "function" {
		t.Errorf("type = %q, want %q", out[0].Type, "function")
	}

	if out[0].Function.Name != "rag.search" {
		t.Errorf("function name = %q, want %q", out[0].Function.Name, "rag.search")
	}

	if out[0].Function.Description != "Search knowledge base" {
		t.Errorf("function description = %q, want %q", out[0].Function.Description, "Search knowledge base")
	}
}

// ---------------------------------------------------------------------------
// toAnthropicToolsMessages — Wire Format Translation
// ---------------------------------------------------------------------------

func TestToAnthropicToolsMessagesSystemExtraction(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{Role: "system", Content: "You are Japa AI."},
		{Role: "user", Content: "Hello"},
	}

	system, out := toAnthropicToolsMessages(msgs)
	if system != "You are Japa AI." {
		t.Errorf("system = %q, want %q", system, "You are Japa AI.")
	}

	if len(out) != 1 {
		t.Fatalf("messages len = %d, want 1 (system extracted)", len(out))
	}

	if out[0].Role != "user" {
		t.Errorf("out[0] role = %q, want %q", out[0].Role, "user")
	}
}

func TestToAnthropicToolsMessagesMultipleSystemConcatenated(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{Role: "system", Content: "Rule 1."},
		{Role: "system", Content: "Rule 2."},
		{Role: "user", Content: "Hi"},
	}

	system, out := toAnthropicToolsMessages(msgs)
	if system != "Rule 1.\n\nRule 2." {
		t.Errorf("system = %q, want %q", system, "Rule 1.\n\nRule 2.")
	}

	if len(out) != 1 {
		t.Fatalf("messages len = %d, want 1", len(out))
	}
}

func TestToAnthropicToolsMessagesAssistantWithToolUse(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{
			Role:    "assistant",
			Content: "Searching now.",
			ToolCalls: []ToolCall{
				{ID: "tu_1", Name: "rag.search", Arguments: json.RawMessage(`{"q":"test"}`)},
			},
		},
	}

	_, out := toAnthropicToolsMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("messages len = %d, want 1", len(out))
	}

	if out[0].Role != "assistant" {
		t.Errorf("role = %q, want %q", out[0].Role, "assistant")
	}

	// Should have 2 blocks: text + tool_use
	if len(out[0].Content) != 2 {
		t.Fatalf("content blocks len = %d, want 2", len(out[0].Content))
	}

	if out[0].Content[0].Type != "text" {
		t.Errorf("block[0] type = %q, want %q", out[0].Content[0].Type, "text")
	}

	if out[0].Content[0].Text != "Searching now." {
		t.Errorf("block[0] text = %q, want %q", out[0].Content[0].Text, "Searching now.")
	}

	if out[0].Content[1].Type != "tool_use" {
		t.Errorf("block[1] type = %q, want %q", out[0].Content[1].Type, "tool_use")
	}

	if out[0].Content[1].ID != "tu_1" {
		t.Errorf("block[1] id = %q, want %q", out[0].Content[1].ID, "tu_1")
	}

	if out[0].Content[1].Name != "rag.search" {
		t.Errorf("block[1] name = %q, want %q", out[0].Content[1].Name, "rag.search")
	}
}

func TestToAnthropicToolsMessagesAssistantWithOnlyToolCalls(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{
			Role:    "assistant",
			Content: "", // no text, only tool calls
			ToolCalls: []ToolCall{
				{ID: "tu_1", Name: "rag.search", Arguments: json.RawMessage(`{}`)},
			},
		},
	}

	_, out := toAnthropicToolsMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("messages len = %d, want 1", len(out))
	}
	// Should have 1 block: tool_use only (no empty text block)
	if len(out[0].Content) != 1 {
		t.Fatalf("content blocks len = %d, want 1 (tool_use only)", len(out[0].Content))
	}

	if out[0].Content[0].Type != "tool_use" {
		t.Errorf("block[0] type = %q, want %q", out[0].Content[0].Type, "tool_use")
	}
}

func TestToAnthropicToolsMessagesAssistantEmpty(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{Role: "assistant", Content: "", ToolCalls: nil},
	}

	_, out := toAnthropicToolsMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("messages len = %d, want 1", len(out))
	}
	// Should have 1 block: empty text (Anthropic requires content)
	if len(out[0].Content) != 1 {
		t.Fatalf("content blocks len = %d, want 1", len(out[0].Content))
	}

	if out[0].Content[0].Type != "text" {
		t.Errorf("block[0] type = %q, want %q", out[0].Content[0].Type, "text")
	}
}

func TestToAnthropicToolsMessagesToolResultMerged(t *testing.T) {
	t.Parallel()

	// Multiple consecutive tool-result messages should be merged into one
	// user message with multiple tool_result blocks (Anthropic requires
	// alternating user/assistant roles).
	msgs := []ChatMessage{
		{
			Role:       "tool",
			Content:    `{"results":[1]}`,
			ToolCallID: "tu_1",
			Name:       "rag.search",
		},
		{
			Role:       "tool",
			Content:    `{"results":[2]}`,
			ToolCallID: "tu_2",
			Name:       "content.search",
		},
	}

	_, out := toAnthropicToolsMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("messages len = %d, want 1 (merged)", len(out))
	}

	if out[0].Role != "user" {
		t.Errorf("role = %q, want %q", out[0].Role, "user")
	}

	if len(out[0].Content) != 2 {
		t.Fatalf("content blocks len = %d, want 2 (merged tool_results)", len(out[0].Content))
	}

	if out[0].Content[0].Type != "tool_result" {
		t.Errorf("block[0] type = %q, want %q", out[0].Content[0].Type, "tool_result")
	}

	if out[0].Content[0].ToolUseID != "tu_1" {
		t.Errorf("block[0] tool_use_id = %q, want %q", out[0].Content[0].ToolUseID, "tu_1")
	}

	if out[0].Content[1].Type != "tool_result" {
		t.Errorf("block[1] type = %q, want %q", out[0].Content[1].Type, "tool_result")
	}

	if out[0].Content[1].ToolUseID != "tu_2" {
		t.Errorf("block[1] tool_use_id = %q, want %q", out[0].Content[1].ToolUseID, "tu_2")
	}
}

func TestToAnthropicToolsMessagesToolResultNotMergedWithUserText(t *testing.T) {
	t.Parallel()

	// A tool-result after a user text message should NOT merge — it starts
	// a new user message.
	msgs := []ChatMessage{
		{Role: "user", Content: "Please search."},
		{Role: "tool", Content: `{"results":[]}`, ToolCallID: "tu_1", Name: "rag.search"},
	}

	_, out := toAnthropicToolsMessages(msgs)
	if len(out) != 2 {
		t.Fatalf("messages len = %d, want 2 (not merged)", len(out))
	}

	if out[0].Content[0].Type != "text" {
		t.Errorf("block[0] type = %q, want %q", out[0].Content[0].Type, "text")
	}

	if out[1].Content[0].Type != "tool_result" {
		t.Errorf("block[1] type = %q, want %q", out[1].Content[0].Type, "tool_result")
	}
}

func TestToAnthropicToolsMessagesFullConversation(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{Role: "system", Content: "You are Japa AI."},
		{Role: "user", Content: "Compare TUM and Stuttgart."},
		{
			Role:    "assistant",
			Content: "Let me search.",
			ToolCalls: []ToolCall{
				{ID: "tu_1", Name: "rag.search", Arguments: json.RawMessage(`{"q":"TUM"}`)},
				{ID: "tu_2", Name: "rag.search", Arguments: json.RawMessage(`{"q":"Stuttgart"}`)},
			},
		},
		{Role: "tool", Content: `{"chunks":[...]}`, ToolCallID: "tu_1", Name: "rag.search"},
		{Role: "tool", Content: `{"chunks":[...]}`, ToolCallID: "tu_2", Name: "rag.search"},
	}

	system, out := toAnthropicToolsMessages(msgs)
	if system != "You are Japa AI." {
		t.Errorf("system = %q, want %q", system, "You are Japa AI.")
	}
	// Expected: user, assistant (2 blocks), user (2 merged tool_results)
	if len(out) != 3 {
		t.Fatalf("messages len = %d, want 3", len(out))
	}

	if out[0].Role != "user" {
		t.Errorf("out[0] role = %q, want user", out[0].Role)
	}

	if out[1].Role != "assistant" {
		t.Errorf("out[1] role = %q, want assistant", out[1].Role)
	}

	if out[2].Role != "user" {
		t.Errorf("out[2] role = %q, want user", out[2].Role)
	}
	// Merged tool results
	if len(out[2].Content) != 2 {
		t.Errorf("out[2] content blocks = %d, want 2 (merged)", len(out[2].Content))
	}
}

// ---------------------------------------------------------------------------
// toAnthropicToolDefs — Tool Definition Conversion
// ---------------------------------------------------------------------------

func TestToAnthropicToolDefsEmpty(t *testing.T) {
	t.Parallel()

	out := toAnthropicToolDefs(nil)
	if out != nil {
		t.Errorf("expected nil for empty input, got %v", out)
	}
}

func TestToAnthropicToolDefsConversion(t *testing.T) {
	t.Parallel()

	tools := []LLMToolDefinition{
		{
			Name:        "rag.search",
			Description: "Search knowledge base",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}

	out := toAnthropicToolDefs(tools)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}

	if out[0].Name != "rag.search" {
		t.Errorf("name = %q, want %q", out[0].Name, "rag.search")
	}

	if out[0].InputSchema == nil {
		t.Error("input_schema should not be nil")
	}

	if out[0].InputSchema["type"] != "object" {
		t.Errorf("input_schema[type] = %v, want %q", out[0].InputSchema["type"], "object")
	}
}

// ---------------------------------------------------------------------------
// anthropicStopReasonToFinish — Stop Reason Mapping
// ---------------------------------------------------------------------------

func TestAnthropicStopReasonToFinish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		stopReason string
		want       string
	}{
		{"end_turn", "stop"},
		{"stop_sequence", "stop"},
		{"tool_use", "tool_calls"},
		{"max_tokens", "length"},
		{"content_filter", "content_filter"},
		{"unknown_reason", "unknown_reason"}, // passthrough
	}

	for _, tt := range tests {
		t.Run(tt.stopReason, func(t *testing.T) {
			t.Parallel()

			got := anthropicStopReasonToFinish(tt.stopReason)
			if got != tt.want {
				t.Errorf("anthropicStopReasonToFinish(%q) = %q, want %q", tt.stopReason, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// toMimoToolsMessages + toMimoToolDefs — Xiaomi Wire Format
// ---------------------------------------------------------------------------

func TestToMimoToolsMessagesToolResultRole(t *testing.T) {
	t.Parallel()

	msgs := []ChatMessage{
		{Role: "tool", Content: `{"ok":true}`, ToolCallID: "call_1", Name: "rag.search"},
	}

	out := toMimoToolsMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}

	if out[0].Role != "tool" {
		t.Errorf("role = %q, want %q", out[0].Role, "tool")
	}

	if out[0].ToolCallID != "call_1" {
		t.Errorf("tool_call_id = %q, want %q", out[0].ToolCallID, "call_1")
	}
}

func TestToMimoToolDefsEmpty(t *testing.T) {
	t.Parallel()

	out := toMimoToolDefs(nil)
	if out != nil {
		t.Errorf("expected nil for empty input, got %v", out)
	}
}

// ---------------------------------------------------------------------------
// OpenAIProvider.ChatWithTools — HTTP Response Parsing (httptest)
// ---------------------------------------------------------------------------

func TestOpenAIChatWithToolsParsesToolCallResponse(t *testing.T) {
	t.Parallel()

	// Mock OpenAI API returning a tool_calls response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer test-key")
		}

		var reqBody openAIToolsChatRequest

		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Errorf("unmarshaling request: %v", err)
		}

		if len(reqBody.Tools) != 1 {
			t.Errorf("request tools len = %d, want 1", len(reqBody.Tools))
		}

		if reqBody.Tools[0].Function.Name != "content.search" {
			t.Errorf("request tool name = %q, want content.search", reqBody.Tools[0].Function.Name)
		}

		resp := `{
			"id": "chatcmpl-001",
			"model": "gpt-4o",
			"choices": [{
				"finish_reason": "tool_calls",
				"index": 0,
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [{
						"id": "call_abc",
						"type": "function",
						"function": {
							"name": "content.search",
							"arguments": "{\"query\":\"scholarships\",\"limit\":5}"
						}
					}]
				}
			}],
			"usage": {"prompt_tokens": 50, "completion_tokens": 20, "total_tokens": 70}
		}`

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "test-key", srv.URL, "gpt-4o")

	resp, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "find scholarships"}},
		Tools: []LLMToolDefinition{
			{Name: "content.search", Description: "Search content", Parameters: map[string]any{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}

	if !resp.HasToolCalls() {
		t.Fatal("HasToolCalls() = false, want true")
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}

	tc := resp.ToolCalls[0]
	if tc.ID != "call_abc" {
		t.Errorf("ToolCall ID = %q, want call_abc", tc.ID)
	}

	if tc.Name != "content.search" {
		t.Errorf("ToolCall Name = %q, want content.search", tc.Name)
	}

	var args map[string]any
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		t.Fatalf("unmarshaling arguments: %v", err)
	}

	if args["query"] != "scholarships" {
		t.Errorf("args[query] = %v, want scholarships", args["query"])
	}

	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.FinishReason)
	}

	if resp.Usage.TotalTokens != 70 {
		t.Errorf("Usage.TotalTokens = %d, want 70", resp.Usage.TotalTokens)
	}
}

func TestOpenAIChatWithToolsParsesTextResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := `{
			"id": "chatcmpl-002",
			"model": "gpt-4o",
			"choices": [{
				"finish_reason": "stop",
				"index": 0,
				"message": {"role": "assistant", "content": "Here is your answer."}
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "test-key", srv.URL, "gpt-4o")

	resp, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}

	if resp.HasToolCalls() {
		t.Error("HasToolCalls() = true, want false")
	}

	if resp.Content != "Here is your answer." {
		t.Errorf("Content = %q, want %q", resp.Content, "Here is your answer.")
	}

	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", resp.FinishReason)
	}
}

func TestOpenAIChatWithToolsEmptyArgumentsDefaultsToEmptyObject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := `{
			"id": "chatcmpl-003",
			"model": "gpt-4o",
			"choices": [{
				"finish_reason": "tool_calls",
				"index": 0,
				"message": {
					"role": "assistant",
					"tool_calls": [{
						"id": "call_empty",
						"type": "function",
						"function": {
							"name": "tools.list",
							"arguments": ""
						}
					}]
				}
			}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 1, "total_tokens": 6}
		}`

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "test-key", srv.URL, "gpt-4o")

	resp, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "list tools"}},
		Tools: []LLMToolDefinition{
			{Name: "tools.list", Description: "List tools", Parameters: map[string]any{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}

	// Empty arguments should default to "{}" not ""
	args := string(resp.ToolCalls[0].Arguments)
	if args != "{}" {
		t.Errorf("Arguments = %q, want %q (empty should default to {})", args, "{}")
	}

	// Verify it's valid JSON
	var m map[string]any
	if err := json.Unmarshal(resp.ToolCalls[0].Arguments, &m); err != nil {
		t.Errorf("Arguments should be valid JSON: %v", err)
	}
}

func TestOpenAIChatWithToolsNoAPIKey(t *testing.T) {
	t.Parallel()

	p := NewOpenAIProvider("test", "", "", "")

	_, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestOpenAIChatWithToolsNoMessages(t *testing.T) {
	t.Parallel()

	p := NewOpenAIProvider("test", "key", "", "")

	_, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{})
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
}

func TestOpenAIChatWithToolsAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "bad-key", srv.URL, "gpt-4o")

	_, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for API error response")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should contain status 401, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// AnthropicProvider.ChatWithTools — HTTP Response Parsing (httptest)
// ---------------------------------------------------------------------------

func TestAnthropicChatWithToolsParsesToolUseResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("path = %q, want /messages", r.URL.Path)
		}

		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q, want test-key", r.Header.Get("x-api-key"))
		}

		if r.Header.Get("anthropic-version") != AnthropicAPIVersion {
			t.Errorf("anthropic-version = %q, want %q", r.Header.Get("anthropic-version"), AnthropicAPIVersion)
		}

		var reqBody anthropicToolsRequest

		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Errorf("unmarshaling request: %v", err)
		}

		if reqBody.System != "You are Japa AI." {
			t.Errorf("system = %q, want %q", reqBody.System, "You are Japa AI.")
		}

		if len(reqBody.Tools) != 1 {
			t.Errorf("tools len = %d, want 1", len(reqBody.Tools))
		}

		if reqBody.Tools[0].InputSchema == nil {
			t.Error("input_schema should not be nil")
		}

		resp := `{
			"id": "msg_001",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "text", "text": "Let me search."},
				{"type": "tool_use", "id": "toolu_001", "name": "rag.search", "input": {"query": "scholarships"}}
			],
			"model": "claude-sonnet-5",
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 30, "output_tokens": 15}
		}`

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test", "test-key", srv.URL, "claude-sonnet-5")

	resp, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "You are Japa AI."},
			{Role: "user", Content: "find scholarships"},
		},
		Tools: []LLMToolDefinition{
			{Name: "rag.search", Description: "Search RAG", Parameters: map[string]any{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}

	if !resp.HasToolCalls() {
		t.Fatal("HasToolCalls() = false, want true")
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}

	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_001" {
		t.Errorf("ToolCall ID = %q, want toolu_001", tc.ID)
	}

	if tc.Name != "rag.search" {
		t.Errorf("ToolCall Name = %q, want rag.search", tc.Name)
	}

	var args map[string]any
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		t.Fatalf("unmarshaling arguments: %v", err)
	}

	if args["query"] != "scholarships" {
		t.Errorf("args[query] = %v, want scholarships", args["query"])
	}

	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.FinishReason)
	}

	if resp.Content != "Let me search." {
		t.Errorf("Content = %q, want %q", resp.Content, "Let me search.")
	}

	if resp.Usage.TotalTokens != 45 {
		t.Errorf("Usage.TotalTokens = %d, want 45 (30+15)", resp.Usage.TotalTokens)
	}
}

func TestAnthropicChatWithToolsEmptyInputDefaultsToEmptyObject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := `{
			"id": "msg_002",
			"type": "message",
			"role": "assistant",
			"content": [
				{"type": "tool_use", "id": "toolu_002", "name": "tools.list", "input": null}
			],
			"model": "claude-sonnet-5",
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 5, "output_tokens": 1}
		}`

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test", "test-key", srv.URL, "claude-sonnet-5")

	resp, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "list tools"}},
	})
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}

	args := string(resp.ToolCalls[0].Arguments)
	if args != "{}" {
		t.Errorf("Arguments = %q, want %q (null input should default to {})", args, "{}")
	}
}

func TestAnthropicChatWithToolsNoAPIKey(t *testing.T) {
	t.Parallel()

	p := NewAnthropicProvider("test", "", "", "")

	_, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestAnthropicChatWithToolsNoMessages(t *testing.T) {
	t.Parallel()

	p := NewAnthropicProvider("test", "key", "", "")

	_, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{})
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
}

// ---------------------------------------------------------------------------
// XiaomiProvider.ChatWithTools — HTTP Response Parsing (httptest)
// ---------------------------------------------------------------------------

func TestXiaomiChatWithToolsParsesToolCallResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}

		if r.Header.Get("api-key") != "sk-test" {
			t.Errorf("api-key = %q, want sk-test", r.Header.Get("api-key"))
		}

		resp := `{
			"id": "mimo-001",
			"model": "mimo-v2-flash",
			"choices": [{
				"finish_reason": "tool_calls",
				"index": 0,
				"message": {
					"role": "assistant",
					"tool_calls": [{
						"id": "call_xyz",
						"type": "function",
						"function": {
							"name": "rag.search",
							"arguments": "{\"query\":\"visa\"}"
						}
					}]
				}
			}],
			"usage": {"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30}
		}`

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	defer srv.Close()

	p := NewXiaomiProvider("sk-test", "mimo-v2-flash")
	// Override base URL to point at the test server.
	p.baseURL = srv.URL

	resp, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "search visa info"}},
		Tools: []LLMToolDefinition{
			{Name: "rag.search", Description: "Search RAG", Parameters: map[string]any{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}

	if !resp.HasToolCalls() {
		t.Fatal("HasToolCalls() = false, want true")
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}

	tc := resp.ToolCalls[0]
	if tc.ID != "call_xyz" {
		t.Errorf("ToolCall ID = %q, want call_xyz", tc.ID)
	}

	if tc.Name != "rag.search" {
		t.Errorf("ToolCall Name = %q, want rag.search", tc.Name)
	}

	var args map[string]any
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		t.Fatalf("unmarshaling arguments: %v", err)
	}

	if args["query"] != "visa" {
		t.Errorf("args[query] = %v, want visa", args["query"])
	}
}

func TestXiaomiChatWithToolsEmptyArgumentsDefaultsToEmptyObject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := `{
			"id": "mimo-002",
			"model": "mimo-v2-flash",
			"choices": [{
				"finish_reason": "tool_calls",
				"index": 0,
				"message": {
					"role": "assistant",
					"tool_calls": [{
						"id": "call_empty",
						"type": "function",
						"function": {"name": "tools.list", "arguments": ""}
					}]
				}
			}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 1, "total_tokens": 6}
		}`

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	defer srv.Close()

	p := NewXiaomiProvider("sk-test", "mimo-v2-flash")
	p.baseURL = srv.URL

	resp, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "list tools"}},
	})
	if err != nil {
		t.Fatalf("ChatWithTools error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}

	args := string(resp.ToolCalls[0].Arguments)
	if args != "{}" {
		t.Errorf("Arguments = %q, want %q", args, "{}")
	}
}

// ---------------------------------------------------------------------------
// Cursor + Devin — ErrFunctionCallingUnsupported
// ---------------------------------------------------------------------------

func TestCursorChatWithToolsUnsupported(t *testing.T) {
	t.Parallel()

	p := NewCursorProvider("test", "key", "", "")

	_, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if !errors.Is(err, ErrFunctionCallingUnsupported) {
		t.Errorf("error = %v, want ErrFunctionCallingUnsupported", err)
	}
}

func TestDevinChatWithToolsUnsupported(t *testing.T) {
	t.Parallel()

	p := NewDevinProvider("test", "key", "", "")

	_, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if !errors.Is(err, ErrFunctionCallingUnsupported) {
		t.Errorf("error = %v, want ErrFunctionCallingUnsupported", err)
	}
}
