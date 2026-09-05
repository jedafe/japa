// Package ai — gateway_test.go covers the function-calling extension to the
// Dispatcher (Section 103, Phase 1). The tests use mock providers to verify
// that ChatWithTools dispatches to the correct category, falls back on
// failure, and parses tool-call responses correctly.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 1, Task 1.08)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// ChatWithTools — Dispatch Routing
// ---------------------------------------------------------------------------

func TestChatWithToolsRoutesToCorrectCategory(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	agentProvider := &mockProvider{
		name: "agent-provider",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{
				ProviderRef:  "agent-ref",
				Content:      "agent result",
				FinishReason: "stop",
			}, nil
		},
	}
	modProvider := &mockProvider{
		name: "mod-provider",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{ProviderRef: "mod-ref", FinishReason: "stop"}, nil
		},
	}

	d.RegisterProvider("agent", agentProvider)
	d.RegisterProvider("moderation", modProvider)

	resp, err := d.ChatWithTools(t.Context(), "agent", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatWithTools(agent) error: %v", err)
	}

	if resp.ProviderRef != "agent-ref" {
		t.Errorf("ProviderRef = %q, want %q", resp.ProviderRef, "agent-ref")
	}

	if resp.Content != "agent result" {
		t.Errorf("Content = %q, want %q", resp.Content, "agent result")
	}

	if agentProvider.chatWithToolsCount != 1 {
		t.Errorf("agent provider call count = %d, want 1", agentProvider.chatWithToolsCount)
	}

	if modProvider.chatWithToolsCount != 0 {
		t.Errorf("mod provider should not have been called, got %d", modProvider.chatWithToolsCount)
	}
}

func TestChatWithToolsNoProvidersForCategory(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	_, err := d.ChatWithTools(t.Context(), "nonexistent", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("ChatWithTools() expected error for unregistered category, got nil")
	}
}

// ---------------------------------------------------------------------------
// ChatWithTools — Circuit-Breaker Fallback
// ---------------------------------------------------------------------------

func TestChatWithToolsFallsBackOnFailure(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	failing := &mockProvider{
		name: "failing",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{}, errors.New("provider down")
		},
	}
	working := &mockProvider{
		name: "working",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{
				ProviderRef:  "working-ref",
				Content:      "fallback result",
				FinishReason: "stop",
			}, nil
		},
	}

	d.RegisterProvider("agent", failing)
	d.RegisterProvider("agent", working)

	resp, err := d.ChatWithTools(t.Context(), "agent", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("ChatWithTools() fallback error: %v", err)
	}

	if resp.ProviderRef != "working-ref" {
		t.Errorf("ProviderRef = %q, want %q (fallback provider)", resp.ProviderRef, "working-ref")
	}

	if failing.chatWithToolsCount != 1 {
		t.Errorf("failing provider should have been tried once, got %d", failing.chatWithToolsCount)
	}

	if working.chatWithToolsCount != 1 {
		t.Errorf("working provider should have been tried once, got %d", working.chatWithToolsCount)
	}
}

func TestChatWithToolsAllProvidersFail(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	lastErrSentinel := errors.New("p2 error")
	p1 := &mockProvider{
		name: "p1",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{}, errors.New("p1 error")
		},
	}
	p2 := &mockProvider{
		name: "p2",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{}, lastErrSentinel
		},
	}

	d.RegisterProvider("agent", p1)
	d.RegisterProvider("agent", p2)

	_, err := d.ChatWithTools(t.Context(), "agent", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("ChatWithTools() expected error when all providers fail, got nil")
	}

	// The gateway wraps the last provider's error with %w.
	if !errors.Is(err, lastErrSentinel) {
		t.Errorf("expected error to wrap last provider error %q, got: %v", lastErrSentinel, err)
	}
}

// ---------------------------------------------------------------------------
// ChatWithTools — Tool Call Parsing
// ---------------------------------------------------------------------------

func TestChatWithToolsParsesToolCalls(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	toolCallArgs := json.RawMessage(`{"query":"scholarships","limit":5}`)
	provider := &mockProvider{
		name: "tool-provider",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{
				ProviderRef:  "tool-ref",
				FinishReason: "tool_calls",
				ToolCalls: []ToolCall{
					{
						ID:        "call_001",
						Name:      "content.search",
						Arguments: toolCallArgs,
					},
				},
			}, nil
		},
	}

	d.RegisterProvider("agent", provider)

	resp, err := d.ChatWithTools(t.Context(), "agent", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "find scholarships"}},
		Tools: []LLMToolDefinition{
			{
				Name:        "content.search",
				Description: "Search site content",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
					},
					"required": []string{"query"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ChatWithTools() error: %v", err)
	}

	if !resp.HasToolCalls() {
		t.Fatal("HasToolCalls() = false, want true")
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}

	tc := resp.ToolCalls[0]
	if tc.ID != "call_001" {
		t.Errorf("ToolCall ID = %q, want %q", tc.ID, "call_001")
	}

	if tc.Name != "content.search" {
		t.Errorf("ToolCall Name = %q, want %q", tc.Name, "content.search")
	}

	var args map[string]any
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		t.Fatalf("unmarshaling tool call arguments: %v", err)
	}

	if args["query"] != "scholarships" {
		t.Errorf("ToolCall args[query] = %v, want %q", args["query"], "scholarships")
	}
}

func TestChatWithToolsParsesContentResponse(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	provider := &mockProvider{
		name: "text-provider",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{
				ProviderRef:  "text-ref",
				Content:      "Here is the answer.",
				FinishReason: "stop",
				Usage: TokenUsage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
			}, nil
		},
	}

	d.RegisterProvider("agent", provider)

	resp, err := d.ChatWithTools(t.Context(), "agent", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "what is 2+2"}},
	})
	if err != nil {
		t.Fatalf("ChatWithTools() error: %v", err)
	}

	if resp.HasToolCalls() {
		t.Error("HasToolCalls() = true, want false for text-only response")
	}

	if resp.Content != "Here is the answer." {
		t.Errorf("Content = %q, want %q", resp.Content, "Here is the answer.")
	}

	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "stop")
	}

	if resp.Usage.TotalTokens != 15 {
		t.Errorf("Usage.TotalTokens = %d, want 15", resp.Usage.TotalTokens)
	}
}

// ---------------------------------------------------------------------------
// ChatWithTools — ErrFunctionCallingUnsupported Fallback
// ---------------------------------------------------------------------------

func TestChatWithToolsFallsBackFromUnsupportedProvider(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	// Simulate an agent-based provider (like Cursor/Devin) that doesn't
	// support function calling.
	unsupported := &mockProvider{
		name: "agent-based",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{}, ErrFunctionCallingUnsupported
		},
	}
	supported := &mockProvider{
		name: "llm-based",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{
				ProviderRef:  "llm-ref",
				Content:      "result from llm",
				FinishReason: "stop",
			}, nil
		},
	}

	d.RegisterProvider("agent", unsupported)
	d.RegisterProvider("agent", supported)

	resp, err := d.ChatWithTools(t.Context(), "agent", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("ChatWithTools() should fall back from unsupported, got error: %v", err)
	}

	if resp.ProviderRef != "llm-ref" {
		t.Errorf("ProviderRef = %q, want %q (fallback to supported)", resp.ProviderRef, "llm-ref")
	}

	if unsupported.chatWithToolsCount != 1 {
		t.Errorf("unsupported provider should have been tried once, got %d", unsupported.chatWithToolsCount)
	}

	if supported.chatWithToolsCount != 1 {
		t.Errorf("supported provider should have been tried once, got %d", supported.chatWithToolsCount)
	}
}

// ---------------------------------------------------------------------------
// ChatWithTools — Request Propagation
// ---------------------------------------------------------------------------

func TestChatWithToolsPropagatesRequestToProvider(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	var capturedReq ChatWithToolsRequest

	provider := &mockProvider{
		name: "capture",
		chatWithToolsFn: func(_ context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			capturedReq = req
			return ChatWithToolsResponse{FinishReason: "stop"}, nil
		},
	}

	d.RegisterProvider("agent", provider)

	expectedMessages := []ChatMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Compare TUM and Stuttgart"},
	}
	expectedTools := []LLMToolDefinition{
		{Name: "rag.search", Description: "Search knowledge base", Parameters: map[string]any{"type": "object"}},
	}

	_, err := d.ChatWithTools(t.Context(), "agent", ChatWithToolsRequest{
		Messages:    expectedMessages,
		Tools:       expectedTools,
		Model:       "gpt-4o",
		Temperature: 0.3,
		MaxTokens:   2048,
	})
	if err != nil {
		t.Fatalf("ChatWithTools() error: %v", err)
	}

	if len(capturedReq.Messages) != 2 {
		t.Errorf("captured messages len = %d, want 2", len(capturedReq.Messages))
	}

	if capturedReq.Messages[1].Content != "Compare TUM and Stuttgart" {
		t.Errorf("captured user message = %q, want %q", capturedReq.Messages[1].Content, "Compare TUM and Stuttgart")
	}

	if len(capturedReq.Tools) != 1 {
		t.Errorf("captured tools len = %d, want 1", len(capturedReq.Tools))
	}

	if capturedReq.Tools[0].Name != "rag.search" {
		t.Errorf("captured tool name = %q, want %q", capturedReq.Tools[0].Name, "rag.search")
	}

	if capturedReq.Model != "gpt-4o" {
		t.Errorf("captured model = %q, want %q", capturedReq.Model, "gpt-4o")
	}

	if capturedReq.Temperature != 0.3 {
		t.Errorf("captured temperature = %v, want 0.3", capturedReq.Temperature)
	}

	if capturedReq.MaxTokens != 2048 {
		t.Errorf("captured max_tokens = %d, want 2048", capturedReq.MaxTokens)
	}
}

// ---------------------------------------------------------------------------
// ChatWithTools — Context Cancellation
// ---------------------------------------------------------------------------

func TestChatWithToolsRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	provider := &mockProvider{
		name: "ctx-provider",
		chatWithToolsFn: func(ctx context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			<-ctx.Done()
			return ChatWithToolsResponse{}, ctx.Err()
		},
	}

	d.RegisterProvider("agent", provider)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := d.ChatWithTools(ctx, "agent", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Fatal("ChatWithTools() expected error from cancelled context, got nil")
	}
}
