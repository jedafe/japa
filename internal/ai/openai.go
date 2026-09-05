// Package ai — openai.go implements a generic OpenAI-compatible provider.
//
// This provider supports any backend that implements the OpenAI Chat
// Completions API format (/v1/chat/completions) with Bearer token auth.
// This covers:
//   - OpenAI (api.openai.com)
//   - OpenRouter (openrouter.ai)
//   - OpenCode Zen
//   - Groq, Together AI, Anyscale, LiteLLM, LocalAI, vLLM, Ollama (with OpenAI compat)
//   - Any other OpenAI-compatible gateway
//
// The base URL and model are configurable via the ProviderConfig, so the
// admin can point at any endpoint without code changes.
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

// OpenAIProvider implements the Provider interface for any OpenAI-compatible API.
type OpenAIProvider struct {
	name                   string
	baseURL                string
	apiKey                 string
	model                  string
	httpClient             *http.Client
	useMaxCompletionTokens bool
}

// NewOpenAIProvider creates an OpenAI-compatible provider.
//   - baseURL: the API root (e.g. "https://api.openai.com/v1", "https://openrouter.ai/api/v1")
//     If empty, defaults to OpenAI's public endpoint.
//   - model: the default model to use (e.g. "gpt-5.4", "anthropic/claude-sonnet-5" for OpenRouter)
//     If empty, defaults to "gpt-4o-mini".
//   - apiKey: the Bearer token for Authorization header.
func NewOpenAIProvider(name, apiKey, baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	// Trim trailing slash so endpoint construction is predictable.
	baseURL = strings.TrimRight(baseURL, "/")

	if model == "" {
		model = "gpt-4o-mini"
	}

	if name == "" {
		name = "openai"
	}

	return &OpenAIProvider{
		name:       name,
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Name returns the provider identifier for logs and dashboards.
func (p *OpenAIProvider) Name() string {
	return p.name
}

// SetName overrides the provider name so DB-backed providers can use the
// admin-supplied display name in logs and dashboards.
func (p *OpenAIProvider) SetName(name string) {
	p.name = name
}

// UseMaxCompletionTokens switches the wire request to use the current
// "max_completion_tokens" field instead of the deprecated "max_tokens" field.
// Call this for providers that have adopted the OpenAI 2025 schema
// (OpenAI, Groq, OpenRouter, Together AI, etc.). Leave it unset for
// Ollama, custom proxies, and other providers that may still require
// "max_tokens".
func (p *OpenAIProvider) UseMaxCompletionTokens() {
	p.useMaxCompletionTokens = true
}

// isReasoningModel reports whether the model identifier belongs to an
// OpenAI reasoning family (o1, o3, o4, and their variants). These models
// require max_completion_tokens and reject temperature/top_p.
func isReasoningModel(model string) bool {
	if model == "" {
		return false
	}
	// Strip any provider/organization prefix, e.g. "openai/o1-mini".
	base := model
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}

	switch {
	case strings.HasPrefix(base, "o1"), strings.HasPrefix(base, "o3"), strings.HasPrefix(base, "o4"):
		return true
	}

	return false
}

// openAIChatRequest is the standard OpenAI chat completions request body.
// The request supports both "max_tokens" and "max_completion_tokens" but
// only one is emitted per request (see the wire-token helper in Send).
type openAIChatRequest struct {
	Model               string          `json:"model"`
	Messages            []openAIMessage `json:"messages"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         float64         `json:"temperature,omitempty"`
	TopP                float64         `json:"top_p,omitempty"`
	Stream              bool            `json:"stream"`
	ResponseFormat      any             `json:"response_format,omitempty"`
	Extra               map[string]any  `json:"-"`
}

// openAIMessage is a single chat message in the OpenAI format.
// Some providers (e.g. StepFun /step-*-flash) put the generated output in
// the "reasoning" field and leave "content" empty; we capture both.
type openAIMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning"`
}

// openAIChatResponse is the standard OpenAI chat completions response.
type openAIChatResponse struct {
	ID      string         `json:"id"`
	Choices []openAIChoice `json:"choices"`
	Model   string         `json:"model"`
	Usage   openAIUsage    `json:"usage"`
}

// openAIChoice is a single completion choice.
type openAIChoice struct {
	FinishReason string        `json:"finish_reason"`
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
}

// openAIUsage reports token consumption.
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Send dispatches a chat completion request to the OpenAI-compatible API.
// It extracts "prompt" and optional "context" from req.Payload, builds an
// OpenAI-compatible request, and parses the response.
func (p *OpenAIProvider) Send(ctx context.Context, req Request) (Response, error) {
	if p.apiKey == "" {
		return Response{}, errors.New("openai: API key not configured")
	}

	prompt, _ := req.Payload["prompt"].(string)
	if prompt == "" {
		return Response{}, errors.New("openai: prompt is required in payload")
	}

	messages := buildOpenAIMessages(prompt, req.Payload)

	maxTokens := 1024
	if mc, ok := req.Payload["max_completion_tokens"].(int); ok && mc > 0 {
		maxTokens = mc
	} else if mc, ok := req.Payload["max_completion_tokens"].(float64); ok && mc > 0 {
		maxTokens = int(mc)
	} else if mt, ok := req.Payload["max_tokens"].(int); ok && mt > 0 {
		maxTokens = mt
	} else if mt, ok := req.Payload["max_tokens"].(float64); ok && mt > 0 {
		maxTokens = int(mt)
	}

	// Allow the caller to override the provider's default model for this
	// request (ADR-114: per-request model selection).
	model := p.model
	if m, ok := req.Payload["model"].(string); ok && m != "" {
		model = m
	}

	chatReq := openAIChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	// Reasoning models (o1/o3/o4) require max_completion_tokens and do not
	// support temperature/top_p. For other models, prefer max_completion_tokens
	// when the provider advertises current schema support (OpenAI, Groq, etc.);
	// otherwise fall back to the deprecated max_tokens for Ollama/custom proxies.
	if isReasoningModel(model) || p.useMaxCompletionTokens {
		chatReq.MaxCompletionTokens = maxTokens
	} else {
		chatReq.MaxTokens = maxTokens
	}

	if !isReasoningModel(model) {
		chatReq.Temperature = 0.7
	}

	if rf, ok := req.Payload["response_format"]; ok {
		chatReq.ResponseFormat = rf
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("openai: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// OpenAI-compatible APIs use Bearer token auth (unlike Xiaomi which uses api-key header).
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// OpenRouter recommends these headers for attribution/routing; they are
	// harmless on other OpenAI-compatible endpoints and ignored if unknown.
	httpReq.Header.Set("HTTP-Referer", "https://japaconnect.com")
	httpReq.Header.Set("X-Title", "JapaConnect")

	httpResp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return Response{}, fmt.Errorf("openai: request failed: %w", err)
	}

	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("openai: reading response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("openai: API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return Response{}, fmt.Errorf("openai: parsing response: %w", err)
	}

	contentPreview := ""

	if len(chatResp.Choices) > 0 {
		msg := chatResp.Choices[0].Message
		if msg.Content != "" {
			contentPreview = strings.TrimSpace(msg.Content)
		} else {
			contentPreview = strings.TrimSpace(msg.Reasoning)
		}

		if len(contentPreview) > 200 {
			contentPreview = contentPreview[:200]
		}
	}

	slog.Info(
		"openai: request completed",
		"provider", p.name,
		"model", chatResp.Model,
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
		"content_len", len(contentPreview),
		"content_preview", contentPreview,
	)

	return Response{
		ProviderRef: chatResp.ID,
		Status:      "success",
		Raw: map[string]any{
			"id":      chatResp.ID,
			"model":   chatResp.Model,
			"choices": chatResp.Choices,
			"usage":   chatResp.Usage,
		},
	}, nil
}

// buildOpenAIMessages constructs the messages array from prompt and optional
// context. When the payload requests humanization (or the caller pre-merged
// the global setting into the payload), the humanization instructions are
// appended to the system context so the model steers toward natural output.
func buildOpenAIMessages(prompt string, payload map[string]any) []openAIMessage {
	var messages []openAIMessage

	var sysCtx string
	if ctx, ok := payload["context"].(string); ok && ctx != "" {
		sysCtx = ctx
	}
	// Shared persona voice prompt (ADR-114): when the caller supplies a
	// voice_prompt, it is injected as the outer system context so it guides
	// the whole response.
	if voice, ok := payload["voice_prompt"].(string); ok && voice != "" {
		if sysCtx != "" {
			sysCtx = voice + "\n\n" + sysCtx
		} else {
			sysCtx = voice
		}
	}

	// Apply humanization (provider-layer injection covers every generation surface).
	if level, requested := PayloadHumanize(payload); requested {
		sysCtx = ApplyHumanization(sysCtx, level)
	}

	if sysCtx != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: sysCtx})
	}

	messages = append(messages, openAIMessage{Role: "user", Content: prompt})

	return messages
}

// VerifyWebhookSignature is a no-op for OpenAI-compatible providers.
// Webhook callbacks (if any) are provider-specific and not part of the
// standard OpenAI Chat Completions API.
func (p *OpenAIProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	return nil
}

// Models returns the model list for this provider. For OpenAI-compatible
// providers, the model is admin-configured (not a fixed catalog), so we
// return the configured model. If the admin wants to expose multiple
// models, they can register multiple provider configs with different
// model values.
func (p *OpenAIProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: p.model, Label: p.model + " (" + p.name + ")"},
	}
}

// openAIModelsResponse is the response from GET /v1/models (OpenAI format).
type openAIModelsResponse struct {
	Data []openAIModelEntry `json:"data"`
}

// openAIModelEntry is a single model in the /v1/models response.
type openAIModelEntry struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// ---------------------------------------------------------------------------
// Function Calling (Section 103 — AI Agent Executor)
// ---------------------------------------------------------------------------

// openAIToolDef is the OpenAI tools-array entry: { type: "function", function: {...} }.
type openAIToolDef struct {
	Type     string             `json:"type"` // always "function"
	Function openAIToolFunction `json:"function"`
}

// openAIToolFunction is the inner function definition.
type openAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// openAIToolCall is a tool call in the assistant message response.
type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"` // "function"
	Function openAIToolCallFunc `json:"function"`
}

// openAIToolCallFunc is the inner function payload of a tool call.
type openAIToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

// openAIToolsChatRequest is the chat completions request with tools support.
// Messages use a richer shape than openAIMessage because assistant messages
// can carry tool_calls and tool-result messages need tool_call_id + name.
type openAIToolsChatRequest struct {
	Model               string           `json:"model"`
	Messages            []openAIToolsMsg `json:"messages"`
	Tools               []openAIToolDef  `json:"tools,omitempty"`
	MaxTokens           int              `json:"max_tokens,omitempty"`
	MaxCompletionTokens int              `json:"max_completion_tokens,omitempty"`
	Temperature         float64          `json:"temperature,omitempty"`
	TopP                float64          `json:"top_p,omitempty"`
	Stream              bool             `json:"stream"`
}

// openAIToolsMsg is a chat message supporting function-calling fields.
type openAIToolsMsg struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

// openAIToolsChatResponse is the chat completions response with tool calls.
type openAIToolsChatResponse struct {
	ID      string              `json:"id"`
	Choices []openAIToolsChoice `json:"choices"`
	Model   string              `json:"model"`
	Usage   openAIUsage         `json:"usage"`
}

// openAIToolsChoice is a single completion choice with tool-call support.
type openAIToolsChoice struct {
	FinishReason string         `json:"finish_reason"`
	Index        int            `json:"index"`
	Message      openAIToolsMsg `json:"message"`
}

// ChatWithTools dispatches a function-calling chat completion to the
// OpenAI-compatible API. It translates the unified ChatWithToolsRequest into
// the OpenAI wire format (tools array, multi-turn messages with tool_calls
// and tool-result roles) and parses tool_calls from the response.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md.
func (p *OpenAIProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	if p.apiKey == "" {
		return ChatWithToolsResponse{}, errors.New("openai: API key not configured")
	}

	if len(req.Messages) == 0 {
		return ChatWithToolsResponse{}, errors.New("openai: at least one message is required")
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	chatReq := openAIToolsChatRequest{
		Model:    model,
		Messages: toOpenAIToolsMessages(req.Messages),
		Tools:    toOpenAIToolDefs(req.Tools),
		Stream:   false,
	}
	if isReasoningModel(model) || p.useMaxCompletionTokens {
		chatReq.MaxCompletionTokens = req.MaxTokens
	} else {
		chatReq.MaxTokens = req.MaxTokens
	}

	if !isReasoningModel(model) {
		chatReq.Temperature = req.Temperature
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("openai: marshaling tools request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("openai: creating tools request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://japaconnect.com")
	httpReq.Header.Set("X-Title", "JapaConnect")

	httpResp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("openai: tools request failed: %w", err)
	}

	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("openai: reading tools response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return ChatWithToolsResponse{}, fmt.Errorf("openai: tools API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp openAIToolsChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("openai: parsing tools response: %w", err)
	}

	result := ChatWithToolsResponse{
		Model:       chatResp.Model,
		ProviderRef: chatResp.ID,
		Usage: TokenUsage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]
		result.FinishReason = choice.FinishReason
		result.Content = choice.Message.Content

		for _, tc := range choice.Message.ToolCalls {
			args := json.RawMessage(tc.Function.Arguments)
			if len(args) == 0 || string(args) == "null" {
				args = json.RawMessage("{}")
			}

			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
	}

	slog.Info(
		"openai: tools request completed",
		"provider", p.name,
		"model", chatResp.Model,
		"finish_reason", result.FinishReason,
		"tool_calls", len(result.ToolCalls),
		"prompt_tokens", result.Usage.PromptTokens,
		"completion_tokens", result.Usage.CompletionTokens,
	)

	return result, nil
}

// toOpenAIToolsMessages converts unified ChatMessages to the OpenAI wire format.
func toOpenAIToolsMessages(msgs []ChatMessage) []openAIToolsMsg {
	out := make([]openAIToolsMsg, 0, len(msgs))

	for _, m := range msgs {
		om := openAIToolsMsg{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, openAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openAIToolCallFunc{
					Name:      tc.Name,
					Arguments: string(tc.Arguments),
				},
			})
		}

		out = append(out, om)
	}

	return out
}

// toOpenAIToolDefs converts unified ToolDefinitions to the OpenAI tools array.
func toOpenAIToolDefs(tools []LLMToolDefinition) []openAIToolDef {
	if len(tools) == 0 {
		return nil
	}

	out := make([]openAIToolDef, 0, len(tools))
	for _, t := range tools {
		out = append(out, openAIToolDef{
			Type:     "function",
			Function: openAIToolFunction(t),
		})
	}

	return out
}

// FetchModels queries the provider's GET /v1/models endpoint to discover
// available models at runtime. This works for OpenAI, OpenRouter, Command
// Code, Ollama, and any other OpenAI-compatible provider that implements
// the models endpoint.
func (p *OpenAIProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	if p.apiKey == "" {
		return nil, errors.New("openai: API key not configured")
	}

	endpoint := p.baseURL + "/models"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("openai: creating models request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://japaconnect.com")
	httpReq.Header.Set("X-Title", "JapaConnect")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: models request failed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: reading models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: models API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var modelsResp openAIModelsResponse
	if err := json.Unmarshal(respBody, &modelsResp); err != nil {
		return nil, fmt.Errorf("openai: parsing models response: %w", err)
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
