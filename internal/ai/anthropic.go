// Package ai — anthropic.go implements a dedicated provider for Anthropic's
// native Messages API.
//
// Anthropic's API differs from OpenAI's in three key ways:
//  1. Endpoint is /v1/messages (not /v1/chat/completions)
//  2. Auth uses the x-api-key header (not Authorization: Bearer)
//  3. The anthropic-version header is required (e.g. "2023-06-01")
//  4. The system prompt is a top-level field, not a message with role "system"
//  5. Response content is an array of content blocks (text, tool_use, etc.)
//
// This provider also works with any Anthropic-compatible endpoint, such as
// Command Code's /provider/v1/messages endpoint (which uses Bearer auth
// instead of x-api-key — the auth header is auto-detected from the base URL).
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

// Anthropic API constants.
const (
	AnthropicProviderName = "anthropic"
	AnthropicDefaultBase  = "https://api.anthropic.com/v1"
	AnthropicAPIVersion   = "2023-06-01"
	AnthropicDefaultModel = "claude-sonnet-5"
)

// AnthropicProvider implements the Provider interface for Anthropic's
// native Messages API.
type AnthropicProvider struct {
	name       string
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewAnthropicProvider creates an Anthropic Messages API provider.
//   - baseURL: the API root (e.g. "https://api.anthropic.com/v1").
//     For Command Code's Anthropic endpoint, use "https://api.commandcode.ai/provider/v1".
//     If empty, defaults to Anthropic's public endpoint.
//   - model: the Claude model (e.g. "claude-sonnet-5", "claude-opus-5").
//     If empty, defaults to claude-sonnet-5.
//   - apiKey: the API key. For Anthropic's native API this is sent as
//     x-api-key. For proxies like Command Code that use Bearer auth,
//     the provider auto-detects based on the base URL.
func NewAnthropicProvider(name, apiKey, baseURL, model string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = AnthropicDefaultBase
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if model == "" {
		model = AnthropicDefaultModel
	}

	if name == "" {
		name = AnthropicProviderName
	}

	return &AnthropicProvider{
		name:       name,
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
}

// Name returns the provider identifier for logs and dashboards.
func (p *AnthropicProvider) Name() string {
	return p.name
}

// SetName overrides the provider name so DB-backed providers can use the
// admin-supplied display name in logs and dashboards.
func (p *AnthropicProvider) SetName(name string) {
	p.name = name
}

// anthropicMessageRequest is the request body for POST /v1/messages.
type anthropicMessageRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
}

// anthropicMessage is a single message in the Anthropic format.
// Only "user" and "assistant" roles are supported (system is top-level).
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicMessageResponse is the response from POST /v1/messages.
type anthropicMessageResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

// anthropicContentBlock is a single content block in the response.
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// anthropicUsage reports token consumption.
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// isCommandCodeOrProxy reports whether the base URL belongs to a proxy that
// uses Bearer auth instead of Anthropic's native x-api-key header.
// Command Code's Anthropic endpoint uses Bearer auth.
func (p *AnthropicProvider) usesBearerAuth() bool {
	return strings.Contains(p.baseURL, "commandcode.ai") ||
		strings.Contains(p.baseURL, "openrouter.ai")
}

// Send dispatches a Messages API request to Anthropic (or compatible proxy).
// It extracts "prompt" and optional "context" from req.Payload, builds an
// Anthropic Messages request, and parses the response.
func (p *AnthropicProvider) Send(ctx context.Context, req Request) (Response, error) {
	if p.apiKey == "" {
		return Response{}, errors.New("anthropic: API key not configured")
	}

	prompt, _ := req.Payload["prompt"].(string)
	if prompt == "" {
		return Response{}, errors.New("anthropic: prompt is required in payload")
	}

	// Build messages: Anthropic uses "user"/"assistant" roles only.
	// System context goes in the top-level "system" field, not a message.
	var systemPrompt string
	if ctxStr, ok := req.Payload["context"].(string); ok && ctxStr != "" {
		systemPrompt = ctxStr
	}

	// Apply humanization (provider-layer injection covers every generation surface).
	if level, requested := PayloadHumanize(req.Payload); requested {
		systemPrompt = ApplyHumanization(systemPrompt, level)
	}

	messages := []anthropicMessage{
		{Role: "user", Content: prompt},
	}

	chatReq := anthropicMessageRequest{
		Model:     p.model,
		Messages:  messages,
		System:    systemPrompt,
		MaxTokens: 1024,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/messages"

	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Auth: native Anthropic uses x-api-key; proxies like Command Code use Bearer.
	if p.usesBearerAuth() {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	} else {
		httpReq.Header.Set("x-api-key", p.apiKey)
		httpReq.Header.Set("anthropic-version", AnthropicAPIVersion)
	}

	httpResp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: reading response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("anthropic: API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var msgResp anthropicMessageResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return Response{}, fmt.Errorf("anthropic: parsing response: %w", err)
	}

	// Extract text from content blocks.
	var textContent string
	for _, block := range msgResp.Content {
		if block.Type == "text" {
			textContent += block.Text
		}
	}

	slog.Info(
		"anthropic: request completed",
		"provider", p.name,
		"model", msgResp.Model,
		"input_tokens", msgResp.Usage.InputTokens,
		"output_tokens", msgResp.Usage.OutputTokens,
	)

	return Response{
		ProviderRef: msgResp.ID,
		Status:      "success",
		Raw: map[string]any{
			"id":             msgResp.ID,
			"model":          msgResp.Model,
			"content":        textContent,
			"stop_reason":    msgResp.StopReason,
			"usage":          msgResp.Usage,
			"content_blocks": msgResp.Content,
		},
	}, nil
}

// VerifyWebhookSignature returns an error because Anthropic webhook
// verification is not implemented. A no-op return (nil) would silently
// accept forged payloads. Returning an error ensures any future webhook
// integration must implement real HMAC verification.
func (p *AnthropicProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	return errors.New("ai: anthropic webhook signature verification not implemented")
}

// Models returns the Claude models this provider can serve.
func (p *AnthropicProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: p.model, Label: p.model + " (" + p.name + ")"},
	}
}

// ---------------------------------------------------------------------------
// Function Calling (Section 103 — AI Agent Executor)
// ---------------------------------------------------------------------------

// anthropicToolDef is the Anthropic tools-array entry. Anthropic uses
// input_schema (JSON Schema) instead of OpenAI's parameters.
type anthropicToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// anthropicToolsRequest is the Messages API request with tools support.
// Messages use a richer shape than anthropicMessage because content can be
// an array of typed blocks (text, tool_use, tool_result).
type anthropicToolsRequest struct {
	Model     string              `json:"model"`
	Messages  []anthropicToolsMsg `json:"messages"`
	System    string              `json:"system,omitempty"`
	Tools     []anthropicToolDef  `json:"tools,omitempty"`
	MaxTokens int                 `json:"max_tokens"`
}

// anthropicToolsMsg is a message with block-array content for function calling.
type anthropicToolsMsg struct {
	Role    string                     `json:"role"` // "user" or "assistant"
	Content []anthropicContentBlockExt `json:"content"`
}

// anthropicContentBlockExt is a content block supporting all block types:
// text, tool_use (assistant), and tool_result (user). The Anthropic API
// uses different fields per block type, so this struct is a union.
type anthropicContentBlockExt struct {
	Type string `json:"type"` // "text", "tool_use", "tool_result"

	// text blocks
	Text string `json:"text,omitempty"`

	// tool_use blocks (assistant)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result blocks (user)
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"` // string or array of text blocks
}

// anthropicToolsResponse is the Messages API response with tool_use blocks.
type anthropicToolsResponse struct {
	ID         string                     `json:"id"`
	Type       string                     `json:"type"`
	Role       string                     `json:"role"`
	Content    []anthropicContentBlockExt `json:"content"`
	Model      string                     `json:"model"`
	StopReason string                     `json:"stop_reason"`
	Usage      anthropicUsage             `json:"usage"`
}

// ChatWithTools dispatches a function-calling Messages API request to
// Anthropic. It translates the unified ChatWithToolsRequest into Anthropic's
// native wire format (top-level system, block-array content, tool_use and
// tool_result blocks) and parses tool_use content blocks from the response.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
func (p *AnthropicProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	if p.apiKey == "" {
		return ChatWithToolsResponse{}, errors.New("anthropic: API key not configured")
	}
	if len(req.Messages) == 0 {
		return ChatWithToolsResponse{}, errors.New("anthropic: at least one message is required")
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	systemPrompt, msgs := toAnthropicToolsMessages(req.Messages)

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	chatReq := anthropicToolsRequest{
		Model:     model,
		Messages:  msgs,
		System:    systemPrompt,
		Tools:     toAnthropicToolDefs(req.Tools),
		MaxTokens: maxTokens,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: marshaling tools request: %w", err)
	}

	endpoint := p.baseURL + "/messages"
	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: creating tools request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.usesBearerAuth() {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	} else {
		httpReq.Header.Set("x-api-key", p.apiKey)
		httpReq.Header.Set("anthropic-version", AnthropicAPIVersion)
	}

	httpResp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: tools request failed: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: reading tools response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: tools API error (status %d): %s", httpResp.StatusCode, truncate(string(respBody), 500))
	}

	var msgResp anthropicToolsResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("anthropic: parsing tools response: %w", err)
	}

	result := ChatWithToolsResponse{
		Model:        msgResp.Model,
		ProviderRef:  msgResp.ID,
		FinishReason: anthropicStopReasonToFinish(msgResp.StopReason),
		Usage: TokenUsage{
			PromptTokens:     msgResp.Usage.InputTokens,
			CompletionTokens: msgResp.Usage.OutputTokens,
			TotalTokens:      msgResp.Usage.InputTokens + msgResp.Usage.OutputTokens,
		},
	}

	for _, block := range msgResp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			args := block.Input
			if len(args) == 0 || string(args) == "null" {
				args = json.RawMessage("{}")
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	slog.Info(
		"anthropic: tools request completed",
		"provider", p.name,
		"model", msgResp.Model,
		"stop_reason", msgResp.StopReason,
		"tool_calls", len(result.ToolCalls),
		"input_tokens", result.Usage.PromptTokens,
		"output_tokens", result.Usage.CompletionTokens,
	)

	return result, nil
}

// toAnthropicToolsMessages converts unified ChatMessages to Anthropic's wire
// format: system messages are extracted into a top-level system string, and
// all other messages become block-array content. Tool-result messages (role
// "tool") are converted to user messages with tool_result blocks.
//
// Anthropic requires alternating user/assistant roles. When multiple
// consecutive tool-result messages appear (which happens when the LLM
// requests multiple tool calls in one turn), they are merged into a single
// user message with multiple tool_result blocks to satisfy this constraint.
func toAnthropicToolsMessages(msgs []ChatMessage) (system string, out []anthropicToolsMsg) {
	for _, m := range msgs {
		switch m.Role {
		case "system":
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
			continue

		case "tool":
			// OpenAI tool-result → Anthropic user message with tool_result block.
			// Merge consecutive tool-result messages into one user message
			// (Anthropic requires alternating user/assistant roles).
			block := anthropicContentBlockExt{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}
			if n := len(out); n > 0 && out[n-1].Role == "user" && isToolResultMessage(out[n-1]) {
				out[n-1].Content = append(out[n-1].Content, block)
			} else {
				out = append(out, anthropicToolsMsg{
					Role:    "user",
					Content: []anthropicContentBlockExt{block},
				})
			}

		case "assistant":
			var blocks []anthropicContentBlockExt
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlockExt{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContentBlockExt{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
			if blocks == nil {
				blocks = []anthropicContentBlockExt{{Type: "text", Text: ""}}
			}
			out = append(out, anthropicToolsMsg{Role: "assistant", Content: blocks})

		default: // "user" and any unknown role
			block := anthropicContentBlockExt{Type: "text", Text: m.Content}
			out = append(out, anthropicToolsMsg{
				Role:    "user",
				Content: []anthropicContentBlockExt{block},
			})
		}
	}

	return system, out
}

// isToolResultMessage reports whether the given Anthropic message consists
// solely of tool_result blocks (i.e. it was synthesized from one or more
// OpenAI-style tool-result messages and can be extended with more).
func isToolResultMessage(msg anthropicToolsMsg) bool {
	if msg.Role != "user" || len(msg.Content) == 0 {
		return false
	}
	for _, b := range msg.Content {
		if b.Type != "tool_result" {
			return false
		}
	}
	return true
}

// toAnthropicToolDefs converts unified ToolDefinitions to Anthropic's tools
// array (input_schema instead of parameters).
func toAnthropicToolDefs(tools []LLMToolDefinition) []anthropicToolDef {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicToolDef, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthropicToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}
	return out
}

// anthropicStopReasonToFinish maps Anthropic stop_reason to the unified
// FinishReason vocabulary used by ChatWithToolsResponse.
func anthropicStopReasonToFinish(stopReason string) string {
	switch stopReason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	case "content_filter":
		return "content_filter"
	default:
		return stopReason
	}
}

// FetchModels queries the Anthropic /v1/models endpoint to discover available
// models at runtime. Anthropic exposes a models listing endpoint that returns
// the OpenAI-compatible { data: [{ id }] } format.
func (p *AnthropicProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	if p.apiKey == "" {
		return nil, errors.New("anthropic: API key not configured")
	}

	endpoint := p.baseURL + "/models"
	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("anthropic: creating models request: %w", err)
	}

	if p.usesBearerAuth() {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	} else {
		httpReq.Header.Set("x-api-key", p.apiKey)
		httpReq.Header.Set("anthropic-version", AnthropicAPIVersion)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: models request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: reading models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: models API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	// Anthropic uses the OpenAI-compatible models response format.
	var modelsResp struct {
		Data []struct {
			ID   string `json:"id"`
			Type string `json:"type,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &modelsResp); err != nil {
		return nil, fmt.Errorf("anthropic: parsing models response: %w", err)
	}

	var models []ModelInfo
	for _, m := range modelsResp.Data {
		models = append(models, ModelInfo{ID: m.ID, Label: m.ID})
	}

	return models, nil
}
