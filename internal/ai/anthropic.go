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
	AnthropicProviderName  = "anthropic"
	AnthropicDefaultBase   = "https://api.anthropic.com/v1"
	AnthropicAPIVersion    = "2023-06-01"
	AnthropicDefaultModel  = "claude-sonnet-4-20250514"
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
//   - model: the Claude model (e.g. "claude-sonnet-4-20250514", "claude-opus-4-20250514").
//     If empty, defaults to claude-sonnet-4.
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
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

	httpResp, err := p.httpClient.Do(httpReq)
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
			"id":           msgResp.ID,
			"model":        msgResp.Model,
			"content":      textContent,
			"stop_reason":  msgResp.StopReason,
			"usage":        msgResp.Usage,
			"content_blocks": msgResp.Content,
		},
	}, nil
}

// VerifyWebhookSignature is a no-op for Anthropic.
func (p *AnthropicProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	return nil
}

// Models returns the Claude models this provider can serve.
func (p *AnthropicProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: p.model, Label: p.model + " (" + p.name + ")"},
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
