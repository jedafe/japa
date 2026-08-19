package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPerplexityProvider_Send_Citations verifies that a normal Send call parses
// the top-level citations array and exposes it in Response.Raw.
func TestPerplexityProvider_Send_Citations(t *testing.T) {
	t.Parallel()

	wantCitations := []string{"https://example.com/source1", "https://example.com/source2"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var req openAIChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("invalid request body: %v", err)
		}
		if req.Model != "sonar-pro" {
			t.Errorf("model = %q, want sonar-pro", req.Model)
		}

		resp := map[string]any{
			"id":        "perplexity-test-id",
			"object":    "chat.completion",
			"created":   1234567890,
			"model":     "sonar-pro",
			"citations": wantCitations,
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": "The answer is 42.",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewPerplexityProvider("perplexity-test", "test-api-key", server.URL, "sonar-pro")
	resp, err := p.Send(context.Background(), Request{
		Category: "chat",
		Payload: map[string]any{
			"prompt": "What is the answer?",
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("status = %q, want success", resp.Status)
	}

	citations, ok := resp.Raw["citations"].([]string)
	if !ok {
		t.Fatalf("citations not found or wrong type in Raw: %#v", resp.Raw["citations"])
	}
	if len(citations) != len(wantCitations) {
		t.Fatalf("citations count = %d, want %d", len(citations), len(wantCitations))
	}
	for i, c := range citations {
		if c != wantCitations[i] {
			t.Errorf("citation[%d] = %q, want %q", i, c, wantCitations[i])
		}
	}
}

// TestPerplexityProvider_ChatWithTools_Citations verifies that tool-calling
// responses also surface the Perplexity citations array.
func TestPerplexityProvider_ChatWithTools_Citations(t *testing.T) {
	t.Parallel()

	wantCitations := []string{"https://example.com/tool-source"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]any{
			"id":      "perplexity-tools-id",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "sonar-pro",
			"citations": wantCitations,
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":         "assistant",
						"content":      "",
						"tool_calls":   []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "external.search",
									"arguments": `{"query":"test"}`,
								},
							},
						},
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     20,
				"completion_tokens": 10,
				"total_tokens":      30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewPerplexityProvider("perplexity-test", "test-api-key", server.URL, "sonar-pro")
	resp, err := p.ChatWithTools(context.Background(), ChatWithToolsRequest{
		Model: "sonar-pro",
		Messages: []ChatMessage{
			{Role: "user", Content: "Search for the answer"},
		},
		Tools: []LLMToolDefinition{
			{
				Name:        "external.search",
				Description: "Search the web",
				Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
	})
	if err != nil {
		t.Fatalf("ChatWithTools failed: %v", err)
	}

	if len(resp.Citations) != len(wantCitations) {
		t.Fatalf("citations count = %d, want %d", len(resp.Citations), len(wantCitations))
	}
	if resp.Citations[0] != wantCitations[0] {
		t.Errorf("citation[0] = %q, want %q", resp.Citations[0], wantCitations[0])
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls count = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "external.search" {
		t.Errorf("tool call name = %q, want external.search", resp.ToolCalls[0].Name)
	}
}

// TestPerplexityProvider_FetchModels verifies FetchModels calls the Perplexity
// /v1/models catalog endpoint and parses the OpenAI-compatible response.
func TestPerplexityProvider_FetchModels(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}

		resp := map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "sonar-pro", "object": "model"},
				{"id": "sonar-reasoning", "object": "model"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewPerplexityProvider("perplexity-test", "test-api-key", server.URL, "sonar-pro")
	models, err := p.FetchModels(context.Background())
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models count = %d, want 2", len(models))
	}
	if models[0].ID != "sonar-pro" {
		t.Errorf("model[0].ID = %q, want sonar-pro", models[0].ID)
	}
}
