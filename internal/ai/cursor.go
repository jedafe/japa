// Package ai — cursor.go implements a provider for the Cursor Cloud Agents API.
//
// Cursor (by Anysphere) is an AI agent platform, not a synchronous LLM
// inference endpoint. The API is agent-based and asynchronous:
//  1. POST /v1/agents — create an agent with a prompt, returns agent + initial run
//  2. GET  /v1/agents/{id}/runs/{runId} — poll the run until terminal status
//
// Run statuses: CREATING → RUNNING → FINISHED | ERROR | CANCELLED | EXPIRED
//
// Authentication uses Basic auth (API key as username, empty password).
// The API key is generated from the Cursor Dashboard → API Keys.
//
// This provider wraps the async flow into the synchronous Provider.Send()
// interface by polling until the run reaches a terminal state or the
// context deadline is reached.
//
// Note: Cursor agents are designed to work on repositories (creating PRs,
// making commits). When used as a general-purpose AI provider in JapaConnect
// (e.g. for content generation or chat), the agent runs without a repo
// (no-repo mode) and returns the result text from the run.
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

// Cursor API constants.
const (
	CursorProviderName = "cursor"
	CursorDefaultBase  = "https://api.cursor.com/v1"
	CursorPollInterval = 5 * time.Second
	CursorPollTimeout  = 10 * time.Minute // Cursor agents can take longer
	CursorDefaultModel = "composer-2.5"
)

// CursorProvider implements the Provider interface for the Cursor Cloud Agents API.
type CursorProvider struct {
	name       string
	baseURL    string // e.g. "https://api.cursor.com/v1"
	apiKey     string // Cursor API key from dashboard
	model      string // e.g. "composer-2.5", "claude-4-sonnet"
	httpClient *http.Client
}

// NewCursorProvider creates a Cursor Cloud Agents provider.
//   - baseURL: the API root (e.g. "https://api.cursor.com/v1"). If empty,
//     defaults to Cursor's public endpoint.
//   - apiKey: the Cursor API key (from Cursor Dashboard → API Keys).
//   - model: the model ID (e.g. "composer-2.5", "claude-4-sonnet-thinking").
//     If empty, defaults to "composer-2.5".
func NewCursorProvider(name, apiKey, baseURL, model string) *CursorProvider {
	if baseURL == "" {
		baseURL = CursorDefaultBase
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if model == "" {
		model = CursorDefaultModel
	}

	if name == "" {
		name = CursorProviderName
	}

	return &CursorProvider{
		name:       name,
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the provider identifier for logs and dashboards.
func (p *CursorProvider) Name() string {
	return p.name
}

// SetName overrides the provider name so DB-backed providers can use the
// admin-supplied display name in logs and dashboards.
func (p *CursorProvider) SetName(name string) {
	p.name = name
}

// cursorCreateAgentRequest is the body for POST /v1/agents.
type cursorCreateAgentRequest struct {
	Prompt cursorPrompt `json:"prompt"`
	Model  cursorModel  `json:"model,omitempty"`
}

// cursorPrompt contains the task prompt text.
type cursorPrompt struct {
	Text string `json:"text"`
}

// cursorModel specifies the model and optional params.
type cursorModel struct {
	ID     string             `json:"id"`
	Params []cursorModelParam `json:"params,omitempty"`
}

// cursorModelParam is a per-model parameter (e.g. fast mode).
type cursorModelParam struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// cursorCreateAgentResponse is the response from POST /v1/agents.
type cursorCreateAgentResponse struct {
	Agent cursorAgent `json:"agent"`
	Run   cursorRun   `json:"run"`
}

// cursorAgent is the durable agent record.
type cursorAgent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	LatestRunID string `json:"latestRunId"`
	URL         string `json:"url"`
}

// cursorRun is a single execution of an agent.
type cursorRun struct {
	ID         string `json:"id"`
	AgentID    string `json:"agentId"`
	Status     string `json:"status"` // CREATING, RUNNING, FINISHED, ERROR, CANCELLED, EXPIRED
	Result     string `json:"result,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

// Send creates a Cursor agent with the prompt, then polls the run until it
// reaches a terminal state or the context deadline is reached. This wraps
// the async agent-based API into the synchronous Provider.Send() interface.
func (p *CursorProvider) Send(ctx context.Context, req Request) (Response, error) {
	if p.apiKey == "" {
		return Response{}, errors.New("cursor: API key not configured")
	}

	prompt, _ := req.Payload["prompt"].(string)
	if prompt == "" {
		return Response{}, errors.New("cursor: prompt is required in payload")
	}

	// Enrich the prompt with context if provided.
	var ctxStr string
	if c, ok := req.Payload["context"].(string); ok && c != "" {
		ctxStr = c
	}

	// Apply humanization (provider-layer injection covers every generation surface).
	if level, requested := PayloadHumanize(req.Payload); requested {
		ctxStr = ApplyHumanization(ctxStr, level)
	}

	if ctxStr != "" {
		prompt = ctxStr + "\n\n" + prompt
	}

	// Step 1: Create an agent (no repo — general-purpose AI mode).
	createResp, err := p.createAgent(ctx, prompt)
	if err != nil {
		return Response{}, err
	}

	agentID := createResp.Agent.ID
	runID := createResp.Run.ID

	slog.Info("cursor: agent created",
		"provider", p.name,
		"agent_id", agentID,
		"run_id", runID,
		"status", createResp.Run.Status,
	)

	// Step 2: Poll the run until it reaches a terminal state.
	pollCtx, cancel := context.WithTimeout(ctx, CursorPollTimeout)
	defer cancel()

	ticker := time.NewTicker(CursorPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return Response{
				ProviderRef: agentID,
				Status:      "pending",
				Raw: map[string]any{
					"agent_id": agentID,
					"run_id":   runID,
					"status":   "running",
					"message":  "agent still running — poll timed out",
					"url":      createResp.Agent.URL,
				},
			}, fmt.Errorf("cursor: run %s did not complete within %s", runID, CursorPollTimeout)
		case <-ticker.C:
			current, err := p.getRun(pollCtx, agentID, runID)
			if err != nil {
				slog.Warn("cursor: poll error", "agent_id", agentID, "run_id", runID, "error", err)
				continue
			}

			switch current.Status {
			case "FINISHED":
				slog.Info("cursor: run finished",
					"agent_id", agentID,
					"run_id", runID,
					"duration_ms", current.DurationMs,
				)
				return Response{
					ProviderRef: agentID,
					Status:      "success",
					Raw: map[string]any{
						"agent_id":    agentID,
						"run_id":      runID,
						"status":      current.Status,
						"result":      current.Result,
						"duration_ms": current.DurationMs,
						"url":         createResp.Agent.URL,
					},
				}, nil
			case "ERROR":
				slog.Warn("cursor: run error", "agent_id", agentID, "run_id", runID)
				return Response{
					ProviderRef: agentID,
					Status:      "failed",
					Raw: map[string]any{
						"agent_id": agentID,
						"run_id":   runID,
						"status":   current.Status,
						"result":   current.Result,
					},
				}, fmt.Errorf("cursor: run %s failed", runID)
			case "CANCELLED", "EXPIRED":
				slog.Warn("cursor: run terminated", "agent_id", agentID, "run_id", runID, "status", current.Status)
				return Response{
					ProviderRef: agentID,
					Status:      "failed",
					Raw: map[string]any{
						"agent_id": agentID,
						"run_id":   runID,
						"status":   current.Status,
					},
				}, fmt.Errorf("cursor: run %s %s", runID, current.Status)
			default:
				// CREATING, RUNNING — keep polling.
			}
		}
	}
}

// createAgent sends POST /v1/agents to create a new Cursor agent.
func (p *CursorProvider) createAgent(ctx context.Context, prompt string) (cursorCreateAgentResponse, error) {
	body, err := json.Marshal(cursorCreateAgentRequest{
		Prompt: cursorPrompt{Text: prompt},
		Model:  cursorModel{ID: p.model},
	})
	if err != nil {
		return cursorCreateAgentResponse{}, fmt.Errorf("cursor: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/agents"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return cursorCreateAgentResponse{}, fmt.Errorf("cursor: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Cursor uses Basic auth: API key as username, empty password.
	httpReq.SetBasicAuth(p.apiKey, "")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return cursorCreateAgentResponse{}, fmt.Errorf("cursor: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return cursorCreateAgentResponse{}, fmt.Errorf("cursor: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return cursorCreateAgentResponse{}, fmt.Errorf("cursor: API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var createResp cursorCreateAgentResponse
	if err := json.Unmarshal(respBody, &createResp); err != nil {
		return cursorCreateAgentResponse{}, fmt.Errorf("cursor: parsing response: %w", err)
	}

	return createResp, nil
}

// getRun sends GET /v1/agents/{id}/runs/{runId} to poll the run status.
func (p *CursorProvider) getRun(ctx context.Context, agentID, runID string) (cursorRun, error) {
	endpoint := fmt.Sprintf("%s/agents/%s/runs/%s", p.baseURL, agentID, runID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return cursorRun{}, fmt.Errorf("cursor: creating poll request: %w", err)
	}

	httpReq.SetBasicAuth(p.apiKey, "")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return cursorRun{}, fmt.Errorf("cursor: poll request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return cursorRun{}, fmt.Errorf("cursor: reading poll response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return cursorRun{}, fmt.Errorf("cursor: poll API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var run cursorRun
	if err := json.Unmarshal(respBody, &run); err != nil {
		return cursorRun{}, fmt.Errorf("cursor: parsing poll response: %w", err)
	}

	return run, nil
}

// ChatWithTools is not supported by the Cursor provider. Cursor is an
// agent-based platform (async agent runs, not synchronous chat completions)
// and does not expose a function-calling API. The gateway will fall back to
// the next provider in the agent category.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
func (p *CursorProvider) ChatWithTools(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	return ChatWithToolsResponse{}, ErrFunctionCallingUnsupported
}

// VerifyWebhookSignature is a no-op for Cursor.
func (p *CursorProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	return nil
}

// Models returns the Cursor models this provider can serve.
func (p *CursorProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: p.model, Label: p.model + " (" + p.name + ")"},
	}
}

// FetchModels queries the Cursor /v1/models endpoint to discover available
// models at runtime. Cursor exposes a models listing endpoint that returns
// the available model IDs for the authenticated user.
func (p *CursorProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	if p.apiKey == "" {
		return nil, errors.New("cursor: API key not configured")
	}

	endpoint := p.baseURL + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("cursor: creating models request: %w", err)
	}

	// Cursor uses Basic auth.
	httpReq.SetBasicAuth(p.apiKey, "")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cursor: models request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cursor: reading models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cursor: models API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	// Cursor's models response format may vary — try OpenAI-compatible first.
	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(respBody, &modelsResp); err != nil {
		return nil, fmt.Errorf("cursor: parsing models response: %w", err)
	}

	var models []ModelInfo
	for _, m := range modelsResp.Data {
		models = append(models, ModelInfo{ID: m.ID, Label: m.ID})
	}
	for _, m := range modelsResp.Items {
		models = append(models, ModelInfo{ID: m.ID, Label: m.ID})
	}

	return models, nil
}
