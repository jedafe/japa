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
	name       string
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIProvider creates an OpenAI-compatible provider.
//   - baseURL: the API root (e.g. "https://api.openai.com/v1", "https://openrouter.ai/api/v1")
//     If empty, defaults to OpenAI's public endpoint.
//   - model: the default model to use (e.g. "gpt-4o", "anthropic/claude-3.5-sonnet" for OpenRouter)
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

// openAIChatRequest is the standard OpenAI chat completions request body.
type openAIChatRequest struct {
	Model       string            `json:"model"`
	Messages    []openAIMessage   `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	TopP        float64           `json:"top_p,omitempty"`
	Stream      bool              `json:"stream"`
	Extra       map[string]any    `json:"-"`
}

// openAIMessage is a single chat message in the OpenAI format.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse is the standard OpenAI chat completions response.
type openAIChatResponse struct {
	ID      string           `json:"id"`
	Choices []openAIChoice   `json:"choices"`
	Model   string           `json:"model"`
	Usage   openAIUsage      `json:"usage"`
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

	chatReq := openAIChatRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   1024,
		Temperature: 0.7,
		Stream:      false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
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

	httpResp, err := p.httpClient.Do(httpReq)
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

	slog.Info(
		"openai: request completed",
		"provider", p.name,
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

// buildOpenAIMessages constructs the messages array from prompt and optional context.
func buildOpenAIMessages(prompt string, payload map[string]any) []openAIMessage {
	var messages []openAIMessage

	if ctx, ok := payload["context"].(string); ok && ctx != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: ctx})
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

// FetchModels queries the provider's GET /v1/models endpoint to discover
// available models at runtime. This works for OpenAI, OpenRouter, Command
// Code, Ollama, and any other OpenAI-compatible provider that implements
// the models endpoint.
func (p *OpenAIProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	if p.apiKey == "" {
		return nil, errors.New("openai: API key not configured")
	}

	endpoint := p.baseURL + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
