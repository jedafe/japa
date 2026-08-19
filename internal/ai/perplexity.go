// Package ai — perplexity.go implements the Perplexity AI provider.
//
// Perplexity exposes an OpenAI-compatible Chat Completions API at
// https://api.perplexity.ai with the distinguishing feature that sonar models
// return a "citations" array alongside the message content. This provider
// reuses the OpenAI wire format and Bearer auth, but parses and surfaces the
// citations in the provider response so downstream consumers (agent loop,
// comparison engine, HITL verification) can attribute claims to sources.
//
// Ref: https://docs.perplexity.ai/reference/post_chat_completions
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
)

// perplexityChatResponse extends the OpenAI chat response with the citations
// array that sonar models return.
type perplexityChatResponse struct {
	openAIChatResponse
	Citations []string `json:"citations"`
}

// perplexityToolsChatResponse extends the OpenAI tools response with citations.
type perplexityToolsChatResponse struct {
	openAIToolsChatResponse
	Citations []string `json:"citations"`
}

// PerplexityProvider implements the Provider interface for the Perplexity API.
// It embeds OpenAIProvider for shared transport and overrides Send and
// ChatWithTools to capture citations.
type PerplexityProvider struct {
	*OpenAIProvider
}

// NewPerplexityProvider creates a Perplexity provider with the API endpoint
// and model default suitable for sonar models.
//   - baseURL: the API root, defaulting to "https://api.perplexity.ai".
//   - model: the sonar model, defaulting to "sonar-pro".
//   - apiKey: the Bearer token for Authorization header.
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
		OpenAIProvider: NewOpenAIProvider(name, apiKey, baseURL, model),
	}
}

// Send dispatches a chat completion to Perplexity and captures citations.
func (p *PerplexityProvider) Send(ctx context.Context, req Request) (Response, error) {
	if p.apiKey == "" {
		return Response{}, errors.New("perplexity: API key not configured")
	}

	prompt, _ := req.Payload["prompt"].(string)
	if prompt == "" {
		return Response{}, errors.New("perplexity: prompt is required in payload")
	}

	chatReq := openAIChatRequest{
		Model:       p.model,
		Messages:    buildOpenAIMessages(prompt, req.Payload),
		MaxTokens:   1024,
		Temperature: 0.2, // lower temperature for grounded, factual output
		Stream:      false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return Response{}, fmt.Errorf("perplexity: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("perplexity: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://japaconnect.com")
	httpReq.Header.Set("X-Title", "JapaConnect")

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

	var chatResp perplexityChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return Response{}, fmt.Errorf("perplexity: parsing response: %w", err)
	}

	slog.Info(
		"perplexity: request completed",
		"provider", p.name,
		"model", chatResp.Model,
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
		"citations", len(chatResp.Citations),
	)

	raw := map[string]any{
		"id":        chatResp.ID,
		"model":     chatResp.Model,
		"choices":   chatResp.Choices,
		"usage":     chatResp.Usage,
		"citations": chatResp.Citations,
	}

	return Response{
		ProviderRef: chatResp.ID,
		Status:      "success",
		Raw:         raw,
	}, nil
}

// ChatWithTools dispatches a function-calling chat completion to Perplexity
// and captures citations. Sonar models support function calling, so this
// reuses the OpenAI tools format.
func (p *PerplexityProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	if p.apiKey == "" {
		return ChatWithToolsResponse{}, errors.New("perplexity: API key not configured")
	}
	if len(req.Messages) == 0 {
		return ChatWithToolsResponse{}, errors.New("perplexity: at least one message is required")
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	chatReq := openAIToolsChatRequest{
		Model:       model,
		Messages:    toOpenAIToolsMessages(req.Messages),
		Tools:       toOpenAIToolDefs(req.Tools),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: marshaling tools request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: creating tools request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://japaconnect.com")
	httpReq.Header.Set("X-Title", "JapaConnect")

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

	var chatResp perplexityToolsChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return ChatWithToolsResponse{}, fmt.Errorf("perplexity: parsing tools response: %w", err)
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

		result.Citations = chatResp.Citations
	}

	slog.Info(
		"perplexity: tools request completed",
		"provider", p.name,
		"model", chatResp.Model,
		"finish_reason", result.FinishReason,
		"tool_calls", len(result.ToolCalls),
		"prompt_tokens", result.Usage.PromptTokens,
		"completion_tokens", result.Usage.CompletionTokens,
		"citations", len(chatResp.Citations),
	)

	return result, nil
}

// FetchModels queries the Perplexity model catalog.
func (p *PerplexityProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	if p.apiKey == "" {
		return nil, errors.New("perplexity: API key not configured")
	}

	// Perplexity exposes the model catalog under /v1/models, not the
	// OpenAI-style /models path. See:
	// https://docs.perplexity.ai/api-reference/models-get
	endpoint := p.baseURL + "/v1/models"
	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("perplexity: creating models request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://japaconnect.com")
	httpReq.Header.Set("X-Title", "JapaConnect")

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
		models = append(models, ModelInfo{ID: m.ID, Label: m.ID})
	}
	return models, nil
}
