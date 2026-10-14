// Package ai — stream_test.go covers ADR-130 Phase 2 token-level streaming:
// dispatcher fallback semantics (pre-delta failure degrades to single-shot
// then next provider; post-delta failure is terminal), OpenAI-compatible
// SSE chunk parsing (deltas, tool-call fragments, usage chunk), Anthropic
// Messages SSE parsing, and the token-estimation fallback.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Phases 2–4)
package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockStreamProvider extends mockProvider with an optional ChatStream
// implementation so dispatcher tests can mix streaming and non-streaming
// providers.
type mockStreamProvider struct {
	mockProvider
	chatStreamFn    func(ctx context.Context, req ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error)
	chatStreamCount int
}

func (m *mockStreamProvider) ChatStream(ctx context.Context, req ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error) {
	m.chatStreamCount++
	if m.chatStreamFn != nil {
		return m.chatStreamFn(ctx, req, sink)
	}

	return ChatWithToolsResponse{FinishReason: "stop"}, nil
}

// ---------------------------------------------------------------------------
// Dispatcher fallback semantics
// ---------------------------------------------------------------------------

func TestChatWithToolsStreamNonStreamingProvider(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	d.RegisterProvider("chat", &mockProvider{
		name: "single-shot",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{Content: "plain reply", FinishReason: "stop"}, nil
		},
	})

	var deltas []string
	resp, err := d.ChatWithToolsStream(t.Context(), "chat", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatalf("ChatWithToolsStream error: %v", err)
	}
	if resp.Content != "plain reply" {
		t.Errorf("Content = %q, want %q", resp.Content, "plain reply")
	}
	if resp.ProviderName != "single-shot" {
		t.Errorf("ProviderName = %q, want %q", resp.ProviderName, "single-shot")
	}
	if len(deltas) != 0 {
		t.Errorf("non-streaming provider emitted %d deltas, want 0", len(deltas))
	}
}

func TestChatWithToolsStreamEmitsDeltas(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	d.RegisterProvider("chat", &mockStreamProvider{mockProvider: mockProvider{
		name: "streamer",
	},
		chatStreamFn: func(_ context.Context, _ ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error) {
			sink("Hel")
			sink("lo")
			return ChatWithToolsResponse{Content: "Hello", FinishReason: "stop"}, nil
		}})

	var deltas []string
	resp, err := d.ChatWithToolsStream(t.Context(), "chat", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatalf("ChatWithToolsStream error: %v", err)
	}
	if strings.Join(deltas, "") != "Hello" {
		t.Errorf("deltas = %v, want [Hel lo]", deltas)
	}
	if resp.Content != "Hello" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello")
	}
}

func TestChatWithToolsStreamMidStreamFailureIsTerminal(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	next := &mockProvider{
		name: "next",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{Content: "fallback"}, nil
		},
	}
	d.RegisterProvider("chat", &mockStreamProvider{mockProvider: mockProvider{
		name: "streamer",
	},
		chatStreamFn: func(_ context.Context, _ ChatWithToolsRequest, sink DeltaSink) (ChatWithToolsResponse, error) {
			sink("partial")
			return ChatWithToolsResponse{}, errors.New("connection reset")
		}})
	d.RegisterProvider("chat", next)

	var deltas []string
	_, err := d.ChatWithToolsStream(t.Context(), "chat", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, func(s string) { deltas = append(deltas, s) })
	if err == nil {
		t.Fatal("expected mid-stream error, got nil")
	}
	if !strings.Contains(err.Error(), "mid-response") {
		t.Errorf("error = %q, want mid-response failure", err)
	}
	if next.chatWithToolsCount != 0 {
		t.Errorf("next provider called %d times — must not retry after emitted deltas", next.chatWithToolsCount)
	}
}

func TestChatWithToolsStreamPreDeltaFailureDegradesToSingleShot(t *testing.T) {
	t.Parallel()

	p := &mockStreamProvider{mockProvider: mockProvider{
		name: "flaky",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{Content: "non-streamed reply", FinishReason: "stop"}, nil
		},
	},
		chatStreamFn: func(_ context.Context, _ ChatWithToolsRequest, _ DeltaSink) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{}, fmt.Errorf("%w (stream_options rejected)", ErrStreamingUnsupported)
		}}

	d := NewDispatcher()
	d.RegisterProvider("chat", p)

	resp, err := d.ChatWithToolsStream(t.Context(), "chat", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, func(string) {})
	if err != nil {
		t.Fatalf("expected single-shot fallback, got error: %v", err)
	}
	if resp.Content != "non-streamed reply" {
		t.Errorf("Content = %q, want %q", resp.Content, "non-streamed reply")
	}
	if p.chatStreamCount != 1 || p.chatWithToolsCount != 1 {
		t.Errorf("stream=%d single=%d, want 1 each", p.chatStreamCount, p.chatWithToolsCount)
	}
}

func TestChatWithToolsStreamFallsThroughToNextProvider(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	d.RegisterProvider("chat", &mockStreamProvider{mockProvider: mockProvider{
		name: "dead",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{}, errors.New("down")
		},
	},
		chatStreamFn: func(_ context.Context, _ ChatWithToolsRequest, _ DeltaSink) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{}, errors.New("stream refused")
		}})
	d.RegisterProvider("chat", &mockProvider{
		name: "backup",
		chatWithToolsFn: func(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
			return ChatWithToolsResponse{Content: "backup reply"}, nil
		},
	})

	resp, err := d.ChatWithToolsStream(t.Context(), "chat", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatalf("expected fallback to next provider, got: %v", err)
	}
	if resp.ProviderName != "backup" || resp.Content != "backup reply" {
		t.Errorf("resp = %+v, want backup provider reply", resp)
	}
}

func TestChatWithToolsStreamNoProviders(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	_, err := d.ChatWithToolsStream(t.Context(), "chat", ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, nil)
	if !errors.Is(err, ErrNoProviders) {
		t.Errorf("err = %v, want ErrNoProviders", err)
	}
}

// ---------------------------------------------------------------------------
// OpenAI-compatible SSE parsing (OpenAIProvider.ChatStream)
// ---------------------------------------------------------------------------

// sseResponse writes an SSE body of `data: {json}\n\n` events + [DONE].
func sseResponse(chunks ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

func TestOpenAIChatStreamParsesDeltasUsageAndTools(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		sseResponse(
			`{"id":"c1","model":"gpt-x","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
			`{"id":"c1","choices":[{"index":0,"delta":{"content":"Hel"}}]}`,
			`{"id":"c1","choices":[{"index":0,"delta":{"content":"lo "}}]}`,
			`{"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"content.search","arguments":"{\"que"}}]}}]}`,
			`{"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ry\":\"x\"}"}}]}}]}`,
			`{"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		)(w, r)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "test-key", srv.URL, "gpt-x")

	var deltas []string
	resp, err := p.ChatStream(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}

	if strings.Join(deltas, "") != "Hello " {
		t.Errorf("deltas = %v, want Hello ", deltas)
	}
	if resp.Content != "Hello " {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello ")
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.FinishReason)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("Usage.TotalTokens = %d, want 15", resp.Usage.TotalTokens)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "content.search" {
		t.Errorf("ToolCall = %+v, want call_1/content.search", tc)
	}
	if string(tc.Arguments) != `{"query":"x"}` {
		t.Errorf("Arguments = %s, want {\"query\":\"x\"}", tc.Arguments)
	}
	if resp.ProviderRef != "c1" || resp.Model != "gpt-x" {
		t.Errorf("ref/model = %q/%q", resp.ProviderRef, resp.Model)
	}
}

func TestOpenAIChatStreamAcceptsEOFAfterFinishReason(t *testing.T) {
	t.Parallel()

	// Some compatible servers close the body without a [DONE] sentinel.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "k", srv.URL, "m")

	var deltas []string
	resp, err := p.ChatStream(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}
	if resp.Content != "done" || resp.FinishReason != "stop" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestOpenAIChatStreamErrorsOnAbruptEOF(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"trunc\"}}]}\n\n")
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "k", srv.URL, "m")
	_, err := p.ChatStream(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, nil)
	if err == nil {
		t.Fatal("expected error on abrupt EOF without finish_reason")
	}
}

func TestOpenAIChatStreamHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"bad request"}}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "k", srv.URL, "m")
	_, err := p.ChatStream(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("err = %v, want status 400 error", err)
	}
}

func TestOpenAIChatStreamContextCancel(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"x\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // stream stalls until the client cancels
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test", "k", srv.URL, "m")
	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		_, err := p.ChatStream(ctx, ChatWithToolsRequest{
			Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		}, func(string) {})
		errCh <- err
	}()

	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error after cancel")
		}
	case <-t.Context().Done():
		t.Fatal("ChatStream did not return after context cancel")
	}
}

// ---------------------------------------------------------------------------
// Anthropic Messages SSE parsing (AnthropicProvider.ChatStream)
// ---------------------------------------------------------------------------

func TestAnthropicChatStreamParsesEvents(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "ak" {
			t.Errorf("x-api-key = %q, want ak", got)
		}
		if got := r.Header.Get("anthropic-version"); got != AnthropicAPIVersion {
			t.Errorf("anthropic-version = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":25,"output_tokens":1}}}`,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"He"}}`,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"llo"}}`,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu_1","name":"content.search"}}`,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"x\"}"}}`,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":1}`,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":42}}`,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
		}
		for _, e := range events {
			fmt.Fprintln(w, e)
		}
		fmt.Fprintln(w)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test", "ak", srv.URL, "claude-x")

	var deltas []string
	resp, err := p.ChatStream(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}

	if strings.Join(deltas, "") != "Hello" {
		t.Errorf("deltas = %v, want Hello", deltas)
	}
	if resp.Content != "Hello" {
		t.Errorf("Content = %q, want Hello", resp.Content)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.FinishReason)
	}
	if resp.Usage.PromptTokens != 25 || resp.Usage.CompletionTokens != 42 || resp.Usage.TotalTokens != 67 {
		t.Errorf("Usage = %+v, want 25/42/67", resp.Usage)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "tu_1" || resp.ToolCalls[0].Name != "content.search" {
		t.Errorf("ToolCall = %+v", resp.ToolCalls[0])
	}
	if string(resp.ToolCalls[0].Arguments) != `{"q":"x"}` {
		t.Errorf("Arguments = %s, want {\"q\":\"x\"}", resp.ToolCalls[0].Arguments)
	}
	if resp.ProviderRef != "msg_1" || resp.Model != "claude-x" {
		t.Errorf("ref/model = %q/%q", resp.ProviderRef, resp.Model)
	}
}

func TestAnthropicChatStreamErrorEvent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `event: error`)
		fmt.Fprintln(w, `data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test", "ak", srv.URL, "claude-x")
	_, err := p.ChatStream(t.Context(), ChatWithToolsRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "Overloaded") {
		t.Errorf("err = %v, want Overloaded error", err)
	}
}

// ---------------------------------------------------------------------------
// Usage-estimate fallback
// ---------------------------------------------------------------------------

func TestEstimateTurnTokens(t *testing.T) {
	t.Parallel()

	prompt, completion := estimateTurnTokens(
		[]ChatMessage{
			{Role: "system", Content: "You are helpful."},      // 16 chars → 4
			{Role: "user", Content: "Tell me about Lagos."},    // 20 chars → 5
			{Role: "tool", Content: "result", ToolCallID: "c"}, // 7 chars → 1
		},
		"a forty-char-ish reply that is about xx chars", // 45 chars → 11
	)
	if prompt != 11 || completion != 12 {
		t.Errorf("estimate = %d/%d, want 11/12", prompt, completion)
	}
}
