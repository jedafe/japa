// Package ai — perplexity.go implements the Perplexity Agent API provider.
//
// Perplexity retired the Sonar Chat Completions endpoint and now exposes the
// Agent API at https://api.perplexity.ai/v1/agent. This provider targets the
// Responses-style request/response format: the request uses "input" instead of
// "messages", "max_output_tokens" instead of "max_tokens", provider-prefixed
// model slugs, and a "tools" array that includes built-in web search. The
// response text is extracted from the "output" array and citations are
// collected from "search_results" output items.
//
// Ref: https://docs.perplexity.ai/api-reference/agent-post
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// PerplexityProvider implements the Provider interface for the Perplexity
// Agent (Responses) API.
type PerplexityProvider struct {
	name       string
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewPerplexityProvider creates a Perplexity Agent API provider.
//   - baseURL: the API root, defaulting to "https://api.perplexity.ai".
//   - model: the model or preset. Legacy Sonar names ("sonar-pro", etc.) are
//     mapped to their Agent API equivalents. Provider-prefixed model IDs
//     ("perplexity/sonar", "openai/gpt-5.6-sol") are used as-is. Preset names
//     ("fast", "low", "medium", "high", "xhigh") are used as presets.
//   - apiKey: the Bearer token for the Authorization header.
func NewPerplexityProvider(name, apiKey, baseURL, model string) *PerplexityProvider {
	if baseURL == "" {
		baseURL = "https://api.perplexity.ai"
	}

	baseURL = strings.TrimRight(baseURL, "/")

	if model == "" {
		model = "sonar-pro"
	}

	if name == "" {
		name = "perplexity"
	}

	return &PerplexityProvider{
		name:       name,
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Name returns the provider identifier for logs and dashboards.
func (p *PerplexityProvider) Name() string {
	return p.name
}

// SetName overrides the provider name so DB-backed providers can use the
// admin-supplied display name in logs and dashboards.
func (p *PerplexityProvider) SetName(name string) {
	p.name = name
}

// VerifyWebhookSignature is a no-op for Perplexity.
func (p *PerplexityProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	return nil
}

// perplexityAgentRequest is the body for POST /v1/agent.
type perplexityAgentRequest struct {
	Input           any     `json:"input"`
	Instructions    string  `json:"instructions,omitempty"`
	Model           string  `json:"model,omitempty"`
	Preset          string  `json:"preset,omitempty"`
	Tools           []any   `json:"tools,omitempty"`
	ToolChoice      any     `json:"tool_choice,omitempty"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"top_p,omitempty"`
	Stream          bool    `json:"stream"`
	ResponseFormat  any     `json:"response_format,omitempty"`
	MaxSteps        int     `json:"max_steps,omitempty"`
}

// perplexityInputItem is a single item in the Agent API "input" array.
type perplexityInputItem struct {
	Type      string `json:"type"`
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

// perplexityFunctionTool is a custom function declaration for the Agent API.
// Unlike OpenAI's tools array, the name/description live at the top level.
type perplexityFunctionTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}

// perplexityAgentResponse is the non-streaming response from POST /v1/agent.
type perplexityAgentResponse struct {
	ID                string                       `json:"id"`
	Object            string                       `json:"object"`
	CreatedAt         int64                        `json:"created_at"`
	CompletedAt       int64                        `json:"completed_at"`
	Status            string                       `json:"status"`
	Model             string                       `json:"model"`
	Error             *perplexityError             `json:"error,omitempty"`
	Output            []perplexityOutputItem       `json:"output"`
	Usage             perplexityUsage              `json:"usage"`
	IncompleteDetails *perplexityIncompleteDetails `json:"incomplete_details,omitempty"`
}

type perplexityError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
}

type perplexityIncompleteDetails struct {
	Reason string `json:"reason,omitempty"`
}

// perplexityOutputItem is a single item in the Agent API "output" array.
type perplexityOutputItem struct {
	Type      string                   `json:"type"`
	ID        string                   `json:"id,omitempty"`
	Status    string                   `json:"status,omitempty"`
	Role      string                   `json:"role,omitempty"`
	Content   []perplexityContentItem  `json:"content,omitempty"`
	Name      string                   `json:"name,omitempty"`
	CallID    string                   `json:"call_id,omitempty"`
	Arguments string                   `json:"arguments,omitempty"`
	Queries   []string                 `json:"queries,omitempty"`
	Results   []perplexitySearchResult `json:"results,omitempty"`
}

type perplexityContentItem struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Annotations []any  `json:"annotations,omitempty"`
}

type perplexitySearchResult struct {
	ID          any    `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	Date        string `json:"date,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
	Source      string `json:"source,omitempty"`
}

// perplexityUsage maps the Agent API usage object.
type perplexityUsage struct {
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	TotalTokens  int             `json:"total_tokens"`
	Cost         *perplexityCost `json:"cost,omitempty"`
}

type perplexityCost struct {
	TotalCost float64 `json:"total_cost"`
}

// Send dispatches a Perplexity Agent API request and captures text output and
// citations.
func (p *PerplexityProvider) Send(ctx context.Context, req Request) (Response, error) {
	if p.apiKey == "" {
		return Response{}, errors.New("perplexity: API key not configured")
	}

	prompt, _ := req.Payload["prompt"].(string)
	if prompt == "" {
		return Response{}, errors.New("perplexity: prompt is required in payload")
	}

	input := p.buildInputFromPrompt(prompt, req.Payload)

	agentReq, err := p.buildAgentRequest(req, input, nil)
	if err != nil {
		return Response{}, err
	}

	return p.doAgentRequest(ctx, agentReq)
}

// ChatWithTools dispatches a function-calling request to the Perplexity Agent
// API. Custom functions are declared in the Agent API format; the response is
// parsed for assistant messages and function calls.
func (p *PerplexityProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	if p.apiKey == "" {
		return ChatWithToolsResponse{}, errors.New("perplexity: API key not configured")
	}

	if len(req.Messages) == 0 {
		return ChatWithToolsResponse{}, errors.New("perplexity: at least one message is required")
	}

	input := p.buildInputFromMessages(req.Messages)

	agentReq, err := p.buildAgentRequest(req, input, req.Tools)
	if err != nil {
		return ChatWithToolsResponse{}, err
	}

	return p.doAgentToolsRequest(ctx, agentReq)
}

// buildInputFromPrompt converts a prompt + optional context into Agent API
// input items.
func (p *PerplexityProvider) buildInputFromPrompt(prompt string, payload map[string]any) []perplexityInputItem {
	var input []perplexityInputItem

	var sysCtx string
	if ctx, ok := payload["context"].(string); ok && ctx != "" {
		sysCtx = ctx
	}

	if voice, ok := payload["voice_prompt"].(string); ok && voice != "" {
		if sysCtx != "" {
			sysCtx = voice + "\n\n" + sysCtx
		} else {
			sysCtx = voice
		}
	}

	if level, requested := PayloadHumanize(payload); requested {
		sysCtx = ApplyHumanization(sysCtx, level)
	}

	if sysCtx != "" {
		input = append(input, perplexityInputItem{Type: "message", Role: "system", Content: sysCtx})
	}

	input = append(input, perplexityInputItem{Type: "message", Role: "user", Content: prompt})

	return input
}

// buildInputFromMessages converts unified ChatMessages into Agent API input
// items. Assistant tool calls become function_call items; tool results become
// function_call_output items.
func (p *PerplexityProvider) buildInputFromMessages(msgs []ChatMessage) []perplexityInputItem {
	var input []perplexityInputItem

	for _, m := range msgs {
		switch m.Role {
		case "tool":
			input = append(input, perplexityInputItem{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Name:   m.Name,
				Output: m.Content,
			})
		case "assistant":
			if m.Content != "" {
				input = append(input, perplexityInputItem{Type: "message", Role: "assistant", Content: m.Content})
			}

			for _, tc := range m.ToolCalls {
				input = append(input, perplexityInputItem{
					Type:      "function_call",
					CallID:    tc.ID,
					Name:      tc.Name,
					Arguments: string(tc.Arguments),
				})
			}
		default:
			role := m.Role
			if role == "" {
				role = "user"
			}

			input = append(input, perplexityInputItem{Type: "message", Role: role, Content: m.Content})
		}
	}

	return input
}

// buildAgentRequest assembles the shared Perplexity Agent API request from
// caller overrides and provider defaults.
func (p *PerplexityProvider) buildAgentRequest(reqBase any, input []perplexityInputItem, tools []LLMToolDefinition) (*perplexityAgentRequest, error) {
	var req Request

	var isChat bool

	switch r := reqBase.(type) {
	case Request:
		req = r
	case ChatWithToolsRequest:
		// ChatWithToolsRequest has no Prompt/Payload, so we create a Request with
		// an empty Payload for the helpers below. The fields we need (Temperature,
		// MaxTokens, Model, ResponseFormat) are on ChatWithToolsRequest directly.
		req = Request{Payload: map[string]any{}}
		// Inject chat-specific overrides.
		if r.Temperature != 0 {
			req.Payload["temperature"] = r.Temperature
		}

		if r.MaxTokens != 0 {
			req.Payload["max_output_tokens"] = r.MaxTokens
		}

		if r.Model != "" {
			req.Payload["model"] = r.Model
		}

		isChat = true
	}

	model := p.model
	if m, ok := req.Payload["model"].(string); ok && m != "" {
		model = m
	}

	actualModel, preset, baseTools, toolChoice, _ := p.agentModelConfig(model)

	// Merge custom function tools. For chat, these are provided by the agent loop.
	if len(tools) > 0 {
		for _, t := range tools {
			baseTools = append(baseTools, perplexityFunctionTool{
				Type:        "function",
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
				Strict:      true,
			})
		}
		// When we have custom functions, let the model choose rather than forcing
		// web_search, which would prevent custom tool use.
		toolChoice = "auto"
	}

	maxOutputTokens := 1024
	if mc, ok := req.Payload["max_output_tokens"].(int); ok && mc > 0 {
		maxOutputTokens = mc
	} else if mc, ok := req.Payload["max_output_tokens"].(float64); ok && mc > 0 {
		maxOutputTokens = int(mc)
	} else if mt, ok := req.Payload["max_tokens"].(int); ok && mt > 0 {
		maxOutputTokens = mt
	} else if mt, ok := req.Payload["max_tokens"].(float64); ok && mt > 0 {
		maxOutputTokens = int(mt)
	}

	temperature := 0.2
	if t, ok := req.Payload["temperature"].(float64); ok && t >= 0 {
		temperature = t
	}

	var topP float64
	if t, ok := req.Payload["top_p"].(float64); ok && t >= 0 {
		topP = t
	}

	agentReq := &perplexityAgentRequest{
		Input:           input,
		Model:           actualModel,
		Preset:          preset,
		Tools:           baseTools,
		ToolChoice:      toolChoice,
		MaxOutputTokens: maxOutputTokens,
		Temperature:     temperature,
		TopP:            topP,
		Stream:          false,
	}

	if isChat {
		// For function-calling, give the agent a few research loop steps so it
		// can initialize and run any built-in tools.
		agentReq.MaxSteps = 3
	}

	if rf, ok := req.Payload["response_format"]; ok {
		agentReq.ResponseFormat = p.mapResponseFormat(rf)
	}

	return agentReq, nil
}

// agentModelConfig converts a configured model/preset string into the pieces
// needed by the Agent API. It maps legacy Sonar model names to their modern
// equivalents and returns the web_search tool/choice when a direct model is
// used (presets bundle their own tools).
func (p *PerplexityProvider) agentModelConfig(model string) (actualModel, preset string, tools []any, toolChoice any, forceSearch bool) {
	m := strings.ToLower(strings.TrimSpace(model))

	// Preset names are single lowercase words without a provider slash.
	if !strings.Contains(m, "/") {
		switch m {
		case "", "sonar-pro", "sonar-reasoning", "sonar-reasoning-pro":
			return "", "low", nil, nil, true
		case "sonar":
			return "perplexity/sonar", "", []any{map[string]any{"type": "web_search"}}, map[string]any{"type": "web_search"}, true
		case "sonar-deep-research":
			return "", "medium", nil, nil, true
		case "fast", "low", "medium", "high", "xhigh":
			return "", m, nil, nil, true
		}
		// Unknown non-prefixed slug: fall back to the default Sonar behavior.
		return "perplexity/sonar", "", []any{map[string]any{"type": "web_search"}}, map[string]any{"type": "web_search"}, true
	}

	// Provider-prefixed model ID (e.g. perplexity/sonar, openai/gpt-5.6-sol).
	// Perplexity is search-native, so we enable web_search for direct models.
	return m, "", []any{map[string]any{"type": "web_search"}}, map[string]any{"type": "web_search"}, true
}

// mapResponseFormat converts OpenAI-style response_format to the Agent API
// json_schema shape, which is the only structured-output format Perplexity
// Agent API exposes.
func (p *PerplexityProvider) mapResponseFormat(rf any) any {
	rfMap, ok := rf.(map[string]any)
	if !ok {
		return nil
	}

	t, _ := rfMap["type"].(string)
	switch t {
	case "json_object":
		return map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "response",
				"schema": map[string]any{"type": "object"},
			},
		}
	case "json_schema":
		if js, ok := rfMap["json_schema"].(map[string]any); ok {
			return map[string]any{"type": "json_schema", "json_schema": js}
		}
	}

	return nil
}

// doAgentRequest sends the request and parses the Agent API response.
func (p *PerplexityProvider) doAgentRequest(ctx context.Context, agentReq *perplexityAgentRequest) (Response, error) {
	body, err := json.Marshal(agentReq)
	if err != nil {
		return Response{}, fmt.Errorf("perplexity: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/v1/agent"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("perplexity: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	httpResp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return Response{}, fmt.Errorf("perplexity: request failed: %w", err)
	}

	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("perplexity: reading response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("perplexity: API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var agentResp perplexityAgentResponse
	if err := json.Unmarshal(respBody, &agentResp); err != nil {
		return Response{}, fmt.Errorf("perplexity: parsing response: %w", err)
	}

	if agentResp.Status == "failed" {
		msg := "unknown error"
		if agentResp.Error != nil && agentResp.Error.Message != "" {
			msg = agentResp.Error.Message
		}

		return Response{}, fmt.Errorf("perplexity: agent run failed: %s", msg)
	}

	if agentResp.Status != "completed" {
		return Response{}, fmt.Errorf("perplexity: agent run finished with status %q", agentResp.Status)
	}

	text := p.extractText(&agentResp)
	citations := p.extractCitations(&agentResp)

	slog.Info(
		"perplexity: request completed",
		"provider", p.name,
		"model", agentResp.Model,
		"input_tokens", agentResp.Usage.InputTokens,
		"output_tokens", agentResp.Usage.OutputTokens,
		"citations", len(citations),
	)

	raw := map[string]any{
		"id":        agentResp.ID,
		"model":     agentResp.Model,
		"output":    agentResp.Output,
		"usage":     agentResp.Usage,
		"citations": citations,
		"text":      text,
		"reply":     text,
	}

	return Response{
		ProviderRef: agentResp.ID,
		Status:      "success",
		Raw:         raw,
	}, nil
}

// doAgentToolsRequest sends a function-calling Agent API request and parses
// tool calls and text content.
func (p *PerplexityProvider) doAgentToolsRequest(ctx context.Context, agentReq *perplexityAgentRequest) (ChatWithToolsResponse, error) {
	body, err := json.Marshal(agentReq)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: marshaling tools request: %w", err)
	}

	endpoint := p.baseURL + "/v1/agent"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: creating tools request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	httpResp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: tools request failed: %w", err)
	}

	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: reading tools response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: tools API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var agentResp perplexityAgentResponse
	if err := json.Unmarshal(respBody, &agentResp); err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: parsing tools response: %w", err)
	}

	if agentResp.Status == "failed" {
		msg := "unknown error"
		if agentResp.Error != nil && agentResp.Error.Message != "" {
			msg = agentResp.Error.Message
		}

		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: agent run failed: %s", msg)
	}

	if agentResp.Status != "completed" {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: agent run finished with status %q", agentResp.Status)
	}

	result := ChatWithToolsResponse{
		Model:       agentResp.Model,
		ProviderRef: agentResp.ID,
		Usage: TokenUsage{
			PromptTokens:     agentResp.Usage.InputTokens,
			CompletionTokens: agentResp.Usage.OutputTokens,
			TotalTokens:      agentResp.Usage.TotalTokens,
		},
	}

	for _, item := range agentResp.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					result.Content += c.Text
				}
			}
		case "function_call":
			args := json.RawMessage(item.Arguments)
			if len(args) == 0 || string(args) == "null" {
				args = json.RawMessage("{}")
			}

			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        item.CallID,
				Name:      item.Name,
				Arguments: args,
			})
		}
	}

	result.Citations = p.extractCitations(&agentResp)

	slog.Info(
		"perplexity: tools request completed",
		"provider", p.name,
		"model", agentResp.Model,
		"tool_calls", len(result.ToolCalls),
		"prompt_tokens", result.Usage.PromptTokens,
		"completion_tokens", result.Usage.CompletionTokens,
		"citations", len(result.Citations),
	)

	return result, nil
}

// extractText concatenates all output_text content from assistant message
// output items.
func (p *PerplexityProvider) extractText(resp *perplexityAgentResponse) string {
	var sb strings.Builder

	for _, item := range resp.Output {
		if item.Type != "message" || item.Role != "assistant" {
			continue
		}

		for _, c := range item.Content {
			if c.Type == "output_text" {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}

				sb.WriteString(c.Text)
			}
		}
	}

	return strings.TrimSpace(sb.String())
}

// extractCitations collects URLs from search_results output items.
func (p *PerplexityProvider) extractCitations(resp *perplexityAgentResponse) []string {
	seen := make(map[string]bool)

	var citations []string

	for _, item := range resp.Output {
		if item.Type != "search_results" {
			continue
		}

		for _, r := range item.Results {
			if r.URL != "" && !seen[r.URL] {
				seen[r.URL] = true

				citations = append(citations, r.URL)
			}
		}
	}

	return citations
}

// Models returns a fallback model list for the admin picker when the live
// catalog cannot be fetched.
func (p *PerplexityProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: "sonar-pro", Label: "Sonar Pro (legacy → Agent API preset low)"},
		{ID: "perplexity/sonar", Label: "Perplexity Sonar"},
		{ID: "low", Label: "Agent API preset: low"},
		{ID: "medium", Label: "Agent API preset: medium"},
		{ID: "high", Label: "Agent API preset: high"},
		{ID: "fast", Label: "Agent API preset: fast"},
		{ID: "xhigh", Label: "Agent API preset: xhigh"},
	}
}

// FetchModels queries the Perplexity Agent API model catalog.
func (p *PerplexityProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	if p.apiKey == "" {
		return nil, errors.New("perplexity: API key not configured")
	}

	endpoint := p.baseURL + "/v1/models"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("perplexity: creating models request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("perplexity: models request failed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("perplexity: reading models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("perplexity: models API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var modelsResp openAIModelsResponse
	if err := json.Unmarshal(respBody, &modelsResp); err != nil {
		return nil, fmt.Errorf("perplexity: parsing models response: %w", err)
	}

	var models []ModelInfo

	for _, m := range modelsResp.Data {
		label := m.ID
		if m.OwnedBy != "" {
			label += " (" + m.OwnedBy + ")"
		}

		models = append(models, ModelInfo{ID: m.ID, Label: label})
	}

	return models, nil
}
