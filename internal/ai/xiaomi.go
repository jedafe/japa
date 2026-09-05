// Package ai — xiaomi.go implements the Xiaomi MiMo AI provider.
//
// MiMo exposes an OpenAI-compatible chat completions API. Authentication
// uses an api-key header (not Bearer token). The base URL is selected
// automatically from the key prefix: tp- for Token Plan, sk- for Pay-as-you-go.
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

// Provider name and model constants.
const (
	XiaomiProviderName     = "xiaomi"
	XiaomiModelFlash       = "mimo-v2-flash"
	XiaomiModelV25         = "mimo-v2.5"
	XiaomiModelV25Pro      = "mimo-v2.5-pro"
	XiaomiBasePayg         = "https://api.xiaomimimo.com/v1"            // #nosec G101 -- API endpoint URL, not a credential
	XiaomiBaseTokenPlanCN  = "https://token-plan-cn.xiaomimimo.com/v1"  // #nosec G101 -- API endpoint URL, not a credential
	XiaomiBaseTokenPlanSGP = "https://token-plan-sgp.xiaomimimo.com/v1" // #nosec G101 -- API endpoint URL, not a credential
	XiaomiBaseTokenPlanAMS = "https://token-plan-ams.xiaomimimo.com/v1" // #nosec G101 -- API endpoint URL, not a credential
)

// mimoChatRequest is the OpenAI-compatible request body for MiMo.
type mimoChatRequest struct {
	Model               string        `json:"model"`
	Messages            []mimoMessage `json:"messages"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
	Temperature         float64       `json:"temperature,omitempty"`
	TopP                float64       `json:"top_p,omitempty"`
	Stream              bool          `json:"stream"`
}

// mimoMessage is a single chat message.
type mimoMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// mimoChatResponse is the OpenAI-compatible response from MiMo.
type mimoChatResponse struct {
	ID      string       `json:"id"`
	Choices []mimoChoice `json:"choices"`
	Model   string       `json:"model"`
	Usage   mimoUsage    `json:"usage"`
}

// mimoChoice is a single completion choice.
type mimoChoice struct {
	FinishReason string      `json:"finish_reason"`
	Index        int         `json:"index"`
	Message      mimoMessage `json:"message"`
}

// mimoUsage reports token consumption.
type mimoUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// XiaomiProvider implements the Provider interface for Xiaomi MiMo.
type XiaomiProvider struct {
	name       string
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewXiaomiProvider creates a Xiaomi MiMo provider.
// The base URL is selected automatically from the key prefix:
//   - tp- → Token Plan (CN endpoint by default)
//   - sk- → Pay-as-you-go
//
// If model is empty, it defaults to "mimo-v2-flash".
func NewXiaomiProvider(apiKey, model string) *XiaomiProvider {
	if apiKey == "" {
		slog.Warn("xiaomi: empty API key, provider will fail on Send")
	}

	baseURL := selectXiaomiBaseURL(apiKey)

	if model == "" {
		model = XiaomiModelFlash
	}

	return &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// selectXiaomiBaseURL picks the correct base URL based on the API key prefix.
func selectXiaomiBaseURL(apiKey string) string {
	if strings.HasPrefix(apiKey, "tp-") {
		return XiaomiBaseTokenPlanCN
	}

	return XiaomiBasePayg
}

// Name returns the provider identifier for logs and dashboards.
func (p *XiaomiProvider) Name() string {
	return p.name
}

// SetName overrides the provider name so DB-backed providers can use the
// admin-supplied display name in logs and dashboards.
func (p *XiaomiProvider) SetName(name string) {
	p.name = name
}

// Send dispatches a chat completion request to the MiMo API.
// It extracts "prompt" and optional "context" from req.Payload,
// builds an OpenAI-compatible request, and parses the response.
func (p *XiaomiProvider) Send(ctx context.Context, req Request) (Response, error) {
	if p.apiKey == "" {
		return Response{}, errors.New("xiaomi: API key not configured")
	}

	prompt, _ := req.Payload["prompt"].(string)
	if prompt == "" {
		return Response{}, errors.New("xiaomi: prompt is required in payload")
	}

	// Build messages: system context + user prompt.
	messages := buildMimoMessages(prompt, req.Payload)

	// Allow per-request model override (ADR-114: per-request model selection).
	model := p.model
	if m, ok := req.Payload["model"].(string); ok && m != "" {
		model = m
	}

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

	chatReq := mimoChatRequest{
		Model:               model,
		Messages:            messages,
		MaxCompletionTokens: maxTokens,
		Temperature:         0.7,
		Stream:              false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return Response{}, fmt.Errorf("xiaomi: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("xiaomi: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.apiKey)

	httpResp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return Response{}, fmt.Errorf("xiaomi: request failed: %w", err)
	}

	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("xiaomi: reading response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("xiaomi: API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp mimoChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return Response{}, fmt.Errorf("xiaomi: parsing response: %w", err)
	}

	slog.Info(
		"xiaomi: request completed",
		"model", chatResp.Model,
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
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

// buildMimoMessages constructs the messages array from prompt and optional context.
func buildMimoMessages(prompt string, payload map[string]any) []mimoMessage {
	var messages []mimoMessage

	// Inject system context if provided.
	var sysCtx string
	if ctx, ok := payload["context"].(string); ok && ctx != "" {
		sysCtx = ctx
	}
	// Shared persona voice prompt (ADR-114).
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
		messages = append(messages, mimoMessage{Role: "system", Content: sysCtx})
	}

	messages = append(messages, mimoMessage{Role: "user", Content: prompt})

	return messages
}

// VerifyWebhookSignature returns an error because MiMo does not support
// webhook callbacks. A no-op return (nil) would silently accept forged
// payloads if a webhook handler were ever wired in. Returning an error
// ensures any future webhook integration must implement real verification.
func (p *XiaomiProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	return errors.New("ai: xiaomi provider does not support webhook callbacks")
}

// Models returns the MiMo models this provider can serve, for the admin model
// picker (Rule 16: derived from registered providers, not hardcoded).
func (p *XiaomiProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: XiaomiModelFlash, Label: "MiMo Flash (Xiaomi)"},
		{ID: XiaomiModelV25, Label: "MiMo v2.5 (Xiaomi)"},
		{ID: XiaomiModelV25Pro, Label: "MiMo v2.5 Pro (Xiaomi)"},
	}
}

// ---------------------------------------------------------------------------
// Function Calling (Section 103 — AI Agent Executor)
// ---------------------------------------------------------------------------

// mimoToolDef is the OpenAI-compatible tools-array entry for MiMo.
type mimoToolDef struct {
	Type     string           `json:"type"` // "function"
	Function mimoToolFunction `json:"function"`
}

// mimoToolFunction is the inner function definition.
type mimoToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// mimoToolCall is a tool call in the assistant message response.
type mimoToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function mimoToolCallFunc `json:"function"`
}

// mimoToolCallFunc is the inner function payload of a tool call.
type mimoToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// mimoToolsChatRequest is the chat completions request with tools support.
type mimoToolsChatRequest struct {
	Model               string         `json:"model"`
	Messages            []mimoToolsMsg `json:"messages"`
	Tools               []mimoToolDef  `json:"tools,omitempty"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	Temperature         float64        `json:"temperature,omitempty"`
	TopP                float64        `json:"top_p,omitempty"`
	Stream              bool           `json:"stream"`
}

// mimoToolsMsg is a chat message supporting function-calling fields.
type mimoToolsMsg struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []mimoToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

// mimoToolsChatResponse is the chat completions response with tool calls.
type mimoToolsChatResponse struct {
	ID      string            `json:"id"`
	Choices []mimoToolsChoice `json:"choices"`
	Model   string            `json:"model"`
	Usage   mimoUsage         `json:"usage"`
}

// mimoToolsChoice is a single completion choice with tool-call support.
type mimoToolsChoice struct {
	FinishReason string       `json:"finish_reason"`
	Index        int          `json:"index"`
	Message      mimoToolsMsg `json:"message"`
}

// ChatWithTools dispatches a function-calling chat completion to the MiMo
// API. MiMo is OpenAI-compatible, so the tools array and tool_calls response
// follow the same wire format as openai.go. The only difference is auth
// (api-key header instead of Bearer token).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md.
func (p *XiaomiProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	if p.apiKey == "" {
		return ChatWithToolsResponse{}, errors.New("xiaomi: API key not configured")
	}

	if len(req.Messages) == 0 {
		return ChatWithToolsResponse{}, errors.New("xiaomi: at least one message is required")
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	chatReq := mimoToolsChatRequest{
		Model:               model,
		Messages:            toMimoToolsMessages(req.Messages),
		Tools:               toMimoToolDefs(req.Tools),
		Temperature:         req.Temperature,
		MaxCompletionTokens: req.MaxTokens,
		Stream:              false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("xiaomi: marshaling tools request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("xiaomi: creating tools request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.apiKey)

	httpResp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("xiaomi: tools request failed: %w", err)
	}

	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("xiaomi: reading tools response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return ChatWithToolsResponse{}, fmt.Errorf("xiaomi: tools API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp mimoToolsChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("xiaomi: parsing tools response: %w", err)
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
		"xiaomi: tools request completed",
		"model", chatResp.Model,
		"finish_reason", result.FinishReason,
		"tool_calls", len(result.ToolCalls),
		"prompt_tokens", result.Usage.PromptTokens,
		"completion_tokens", result.Usage.CompletionTokens,
	)

	return result, nil
}

// toMimoToolsMessages converts unified ChatMessages to the MiMo wire format.
func toMimoToolsMessages(msgs []ChatMessage) []mimoToolsMsg {
	out := make([]mimoToolsMsg, 0, len(msgs))

	for _, m := range msgs {
		mm := mimoToolsMsg{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			mm.ToolCalls = append(mm.ToolCalls, mimoToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: mimoToolCallFunc{
					Name:      tc.Name,
					Arguments: string(tc.Arguments),
				},
			})
		}

		out = append(out, mm)
	}

	return out
}

// toMimoToolDefs converts unified ToolDefinitions to the MiMo tools array.
func toMimoToolDefs(tools []LLMToolDefinition) []mimoToolDef {
	if len(tools) == 0 {
		return nil
	}

	out := make([]mimoToolDef, 0, len(tools))
	for _, t := range tools {
		out = append(out, mimoToolDef{
			Type:     "function",
			Function: mimoToolFunction(t),
		})
	}

	return out
}

// FetchModels queries the Xiaomi MiMo /v1/models endpoint to discover
// available models at runtime. Xiaomi exposes an OpenAI-compatible models
// endpoint, so this reuses the same parsing format.
func (p *XiaomiProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	if p.apiKey == "" {
		return nil, errors.New("xiaomi: API key not configured")
	}

	endpoint := p.baseURL + "/models"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("xiaomi: creating models request: %w", err)
	}

	httpReq.Header.Set("api-key", p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("xiaomi: models request failed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("xiaomi: reading models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xiaomi: models API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	// Xiaomi uses the OpenAI-compatible models response format.
	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &modelsResp); err != nil {
		return nil, fmt.Errorf("xiaomi: parsing models response: %w", err)
	}

	var models []ModelInfo
	for _, m := range modelsResp.Data {
		models = append(models, ModelInfo{ID: m.ID, Label: m.ID})
	}

	return models, nil
}

// truncate shortens s to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "..."
}
