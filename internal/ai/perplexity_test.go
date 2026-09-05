package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPerplexityProvider_Send_AgentAPI verifies a normal Send call targets
// /v1/agent and parses text + citations from the Agent API response shape.
func TestPerplexityProvider_Send_AgentAPI(t *testing.T) {
	t.Parallel()

	wantCitations := []string{"https://example.com/source1", "https://example.com/source2"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)

		var req perplexityAgentRequest

		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("invalid request body: %v", err)
		}

		// Legacy sonar-pro maps to the low preset.
		if req.Preset != "low" {
			t.Errorf("preset = %q, want low", req.Preset)
		}

		if req.Stream {
			t.Error("stream should be false")
		}

		if req.MaxOutputTokens != 1024 {
			t.Errorf("max_output_tokens = %d, want 1024", req.MaxOutputTokens)
		}

		resp := map[string]any{
			"id":           "resp_test_001",
			"object":       "response",
			"created_at":   1234567890,
			"completed_at": 1234567895,
			"status":       "completed",
			"model":        "perplexity/sonar",
			"output": []map[string]any{
				{
					"type": "message",
					"id":   "msg_001",
					"role": "assistant",
					"content": []map[string]any{
						{"type": "output_text", "text": "The answer is 42."},
					},
				},
				{
					"type":    "search_results",
					"queries": []string{"what is the answer"},
					"results": []map[string]any{
						{"id": 1, "url": wantCitations[0], "title": "Source 1"},
						{"id": 2, "url": wantCitations[1], "title": "Source 2"},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
				"total_tokens":  15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewPerplexityProvider("perplexity-test", "test-api-key", server.URL, "sonar-pro")

	resp, err := p.Send(t.Context(), Request{
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

	text, ok := resp.Raw["text"].(string)
	if !ok || text != "The answer is 42." {
		t.Errorf("text = %q, want \"The answer is 42.\"", text)
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

// TestPerplexityProvider_ChatWithTools_AgentAPI verifies that a tool-calling
// request maps custom tools to the Agent API format and parses function_call
// output items.
func TestPerplexityProvider_ChatWithTools_AgentAPI(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)

		var req perplexityAgentRequest

		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("invalid request body: %v", err)
		}

		if len(req.Tools) != 2 {
			t.Fatalf("tools count = %d, want 2 (web_search + custom)", len(req.Tools))
		}

		// First tool should be web_search; second is the custom function.
		custom, ok := req.Tools[1].(map[string]any)
		if !ok {
			t.Fatalf("custom tool is not a map: %#v", req.Tools[1])
		}

		if custom["type"] != "function" {
			t.Errorf("custom tool type = %q, want function", custom["type"])
		}

		if custom["name"] != "external.search" {
			t.Errorf("custom tool name = %q, want external.search", custom["name"])
		}

		resp := map[string]any{
			"id":           "resp_tools_001",
			"object":       "response",
			"created_at":   1234567890,
			"completed_at": 1234567895,
			"status":       "completed",
			"model":        "perplexity/sonar",
			"output": []map[string]any{
				{
					"type":      "function_call",
					"id":        "fc_001",
					"call_id":   "call_1",
					"name":      "external.search",
					"arguments": `{"query":"test"}`,
					"status":    "completed",
				},
			},
			"usage": map[string]any{
				"input_tokens":  20,
				"output_tokens": 10,
				"total_tokens":  30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewPerplexityProvider("perplexity-test", "test-api-key", server.URL, "perplexity/sonar")

	resp, err := p.ChatWithTools(t.Context(), ChatWithToolsRequest{
		Model: "perplexity/sonar",
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

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls count = %d, want 1", len(resp.ToolCalls))
	}

	if resp.ToolCalls[0].Name != "external.search" {
		t.Errorf("tool call name = %q, want external.search", resp.ToolCalls[0].Name)
	}

	if string(resp.ToolCalls[0].Arguments) != `{"query":"test"}` {
		t.Errorf("tool call arguments = %q, want {\"query\":\"test\"}", string(resp.ToolCalls[0].Arguments))
	}
}

// TestPerplexityProvider_FetchModels verifies FetchModels calls the Agent API
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
				{"id": "perplexity/sonar", "object": "model", "owned_by": "perplexity"},
				{"id": "openai/gpt-5.6-sol", "object": "model", "owned_by": "openai"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewPerplexityProvider("perplexity-test", "test-api-key", server.URL, "sonar-pro")

	models, err := p.FetchModels(t.Context())
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("models count = %d, want 2", len(models))
	}

	if models[0].ID != "perplexity/sonar" {
		t.Errorf("model[0].ID = %q, want perplexity/sonar", models[0].ID)
	}
}

// TestPerplexityProvider_AgentModelConfig verifies the legacy Sonar model
// mapping and provider-prefixed model handling.
func TestPerplexityProvider_AgentModelConfig(t *testing.T) {
	t.Parallel()

	p := NewPerplexityProvider("test", "", "", "")

	tests := []struct {
		model        string
		wantPreset   string
		wantModel    string
		wantToolType string
		wantChoice   any
	}{
		{"sonar-pro", "low", "", "", nil},
		{"sonar-reasoning", "low", "", "", nil},
		{"sonar-deep-research", "medium", "", "", nil},
		{"sonar", "", "perplexity/sonar", "web_search", map[string]any{"type": "web_search"}},
		{"perplexity/sonar", "", "perplexity/sonar", "web_search", map[string]any{"type": "web_search"}},
		{"low", "low", "", "", nil},
		{"high", "high", "", "", nil},
	}

	for _, tt := range tests {
		actualModel, preset, tools, toolChoice, _ := p.agentModelConfig(tt.model)
		if preset != tt.wantPreset {
			t.Errorf("agentModelConfig(%q) preset = %q, want %q", tt.model, preset, tt.wantPreset)
		}

		if actualModel != tt.wantModel {
			t.Errorf("agentModelConfig(%q) model = %q, want %q", tt.model, actualModel, tt.wantModel)
		}

		var gotToolType string

		if len(tools) > 0 {
			if tm, ok := tools[0].(map[string]any); ok {
				gotToolType, _ = tm["type"].(string)
			}
		}

		if gotToolType != tt.wantToolType {
			t.Errorf("agentModelConfig(%q) tool type = %q, want %q", tt.model, gotToolType, tt.wantToolType)
		}

		if toolChoice == nil && tt.wantChoice != nil {
			t.Errorf("agentModelConfig(%q) toolChoice = nil, want %v", tt.model, tt.wantChoice)
		} else if toolChoice != nil && tt.wantChoice == nil {
			t.Errorf("agentModelConfig(%q) toolChoice = %v, want nil", tt.model, toolChoice)
		}
	}
}
