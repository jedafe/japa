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
	XiaomiModelPro         = "mimo-v2-pro"
	XiaomiModelV25Pro      = "mimo-v2.5-pro"
	XiaomiModelOmni        = "mimo-v2-omni"
	XiaomiBasePayg         = "https://api.xiaomimimo.com/v1"            //nolint:gosec // G101: API endpoint URL, not a credential
	XiaomiBaseTokenPlanCN  = "https://token-plan-cn.xiaomimimo.com/v1"  //nolint:gosec // G101: API endpoint URL, not a credential
	XiaomiBaseTokenPlanSGP = "https://token-plan-sgp.xiaomimimo.com/v1" //nolint:gosec // G101: API endpoint URL, not a credential
	XiaomiBaseTokenPlanAMS = "https://token-plan-ams.xiaomimimo.com/v1" //nolint:gosec // G101: API endpoint URL, not a credential
)

// mimoChatRequest is the OpenAI-compatible request body for MiMo.
type mimoChatRequest struct {
	Model       string        `json:"model"`
	Messages    []mimoMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	Stream      bool          `json:"stream"`
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

	chatReq := mimoChatRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   1024,
		Temperature: 0.7,
		Stream:      false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return Response{}, fmt.Errorf("xiaomi: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("xiaomi: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.apiKey)

	httpResp, err := p.httpClient.Do(httpReq)
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
	if ctx, ok := payload["context"].(string); ok && ctx != "" {
		messages = append(messages, mimoMessage{Role: "system", Content: ctx})
	}

	messages = append(messages, mimoMessage{Role: "user", Content: prompt})

	return messages
}

// VerifyWebhookSignature is a no-op. MiMo does not currently send webhooks.
func (p *XiaomiProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	// MiMo does not send webhook callbacks — signature verification is a no-op.
	return nil
}

// truncate shortens s to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "..."
}
