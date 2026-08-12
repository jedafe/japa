// Package ai — devin.go implements a provider for the Devin agent API.
//
// Devin (by Cognition) is an AI agent platform, not a synchronous LLM
// inference endpoint. The API is session-based and asynchronous:
//  1. POST /v3/organizations/{org_id}/sessions — create a session with a prompt
//  2. GET  /v3/organizations/{org_id}/sessions/{devin_id} — poll for completion
//
// This provider wraps the async flow into the synchronous Provider.Send()
// interface by polling until the session reaches a terminal state
// (completed/failed) or the context deadline is reached.
//
// Authentication uses Bearer tokens with the "cog_" prefix (service user
// credentials). The org_id is encoded in the base_url field of the
// ProviderConfig as the full org-scoped base URL:
//
//	https://api.devin.ai/v3/organizations/org-abc123
//
// The admin enters this as the Base URL in the provider config.
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

// Devin API constants.
const (
	DevinProviderName = "devin"
	DevinDefaultBase  = "https://api.devin.ai/v3/organizations"
	DevinPollInterval = 5 * time.Second
	DevinPollTimeout  = 5 * time.Minute // max time to wait for session completion
)

// DevinProvider implements the Provider interface for the Devin agent API.
type DevinProvider struct {
	name       string
	baseURL    string // e.g. "https://api.devin.ai/v3/organizations/org-abc123"
	apiKey     string // "cog_..." service user token
	model      string // unused — Devin selects the model internally
	httpClient *http.Client
}

// NewDevinProvider creates a Devin agent provider.
//   - baseURL: the org-scoped base URL, e.g.
//     "https://api.devin.ai/v3/organizations/org-abc123"
//     If empty, defaults to the base orgs endpoint (admin must set org_id).
//   - apiKey: the service user token (starts with "cog_").
//   - model: unused (Devin manages model selection internally); kept for
//     interface compatibility.
func NewDevinProvider(name, apiKey, baseURL, model string) *DevinProvider {
	if baseURL == "" {
		baseURL = DevinDefaultBase
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if name == "" {
		name = DevinProviderName
	}

	return &DevinProvider{
		name:       name,
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the provider identifier for logs and dashboards.
func (p *DevinProvider) Name() string {
	return p.name
}

// SetName overrides the provider name so DB-backed providers can use the
// admin-supplied display name in logs and dashboards.
func (p *DevinProvider) SetName(name string) {
	p.name = name
}

// devinCreateSessionRequest is the body for POST /sessions.
type devinCreateSessionRequest struct {
	Prompt         string `json:"prompt"`
	DevinMode      string `json:"devin_mode,omitempty"` // "normal", "fast", "lite", "ultra", "fusion"
	CreateAsUserID string `json:"create_as_user_id,omitempty"`
}

// devinSessionResponse is the response from session create/get endpoints.
type devinSessionResponse struct {
	DevinID  string         `json:"devin_id"`
	Status   string         `json:"status"` // "created", "running", "blocked", "completed", "failed"
	Message  string         `json:"message,omitempty"`
	Output   string         `json:"output,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Send creates a Devin session with the prompt, then polls until the session
// completes or the context deadline is reached. This wraps the async
// session-based API into the synchronous Provider.Send() interface.
func (p *DevinProvider) Send(ctx context.Context, req Request) (Response, error) {
	if p.apiKey == "" {
		return Response{}, errors.New("devin: API key not configured")
	}

	prompt, _ := req.Payload["prompt"].(string)
	if prompt == "" {
		return Response{}, errors.New("devin: prompt is required in payload")
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

	// Step 1: Create a session.
	session, err := p.createSession(ctx, prompt)
	if err != nil {
		return Response{}, err
	}

	slog.Info("devin: session created", "provider", p.name, "devin_id", session.DevinID, "status", session.Status)

	// Step 2: Poll until the session reaches a terminal state.
	pollCtx, cancel := context.WithTimeout(ctx, DevinPollTimeout)
	defer cancel()

	ticker := time.NewTicker(DevinPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return Response{
				ProviderRef: session.DevinID,
				Status:      "pending",
				Raw: map[string]any{
					"devin_id": session.DevinID,
					"status":   session.Status,
					"message":  "session still running — poll timed out",
				},
			}, fmt.Errorf("devin: session %s did not complete within %s", session.DevinID, DevinPollTimeout)
		case <-ticker.C:
			current, err := p.getSession(pollCtx, session.DevinID)
			if err != nil {
				slog.Warn("devin: poll error", "devin_id", session.DevinID, "error", err)
				continue
			}

			switch current.Status {
			case "completed":
				slog.Info("devin: session completed", "devin_id", current.DevinID)
				return Response{
					ProviderRef: current.DevinID,
					Status:      "success",
					Raw: map[string]any{
						"devin_id": current.DevinID,
						"status":   current.Status,
						"output":   current.Output,
						"message":  current.Message,
					},
				}, nil
			case "failed":
				slog.Warn("devin: session failed", "devin_id", current.DevinID, "message", current.Message)
				return Response{
					ProviderRef: current.DevinID,
					Status:      "failed",
					Raw: map[string]any{
						"devin_id": current.DevinID,
						"status":   current.Status,
						"message":  current.Message,
					},
				}, fmt.Errorf("devin: session %s failed: %s", current.DevinID, current.Message)
			case "blocked":
				// "blocked" means Devin needs human input — treat as pending.
				slog.Warn("devin: session blocked (needs input)", "devin_id", current.DevinID)
				return Response{
					ProviderRef: current.DevinID,
					Status:      "pending",
					Raw: map[string]any{
						"devin_id": current.DevinID,
						"status":   current.Status,
						"message":  "session blocked — Devin needs human input",
					},
				}, nil
			default:
				// "created", "running" — keep polling.
			}
		}
	}
}

// createSession sends POST /sessions to create a new Devin session.
// The model field from the config is used as the devin_mode parameter
// (normal, fast, lite, ultra, fusion). If empty, the org default is used.
func (p *DevinProvider) createSession(ctx context.Context, prompt string) (devinSessionResponse, error) {
	reqBody := devinCreateSessionRequest{
		Prompt:    prompt,
		DevinMode: p.model, // model field maps to devin_mode
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/sessions"
	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return devinSessionResponse{}, fmt.Errorf("devin: API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var session devinSessionResponse
	if err := json.Unmarshal(respBody, &session); err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: parsing response: %w", err)
	}

	return session, nil
}

// getSession sends GET /sessions/{devin_id} to poll the session status.
func (p *DevinProvider) getSession(ctx context.Context, devinID string) (devinSessionResponse, error) {
	endpoint := p.baseURL + "/sessions/" + devinID
	httpReq, err := newValidatedHTTPRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: creating poll request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
	if err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: poll request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: reading poll response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return devinSessionResponse{}, fmt.Errorf("devin: poll API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var session devinSessionResponse
	if err := json.Unmarshal(respBody, &session); err != nil {
		return devinSessionResponse{}, fmt.Errorf("devin: parsing poll response: %w", err)
	}

	return session, nil
}

// ChatWithTools is not supported by the Devin provider. Devin is an
// agent-based platform (async sessions, not synchronous chat completions)
// and does not expose a function-calling API. The gateway will fall back to
// the next provider in the agent category.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
func (p *DevinProvider) ChatWithTools(_ context.Context, _ ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	return ChatWithToolsResponse{}, ErrFunctionCallingUnsupported
}

// VerifyWebhookSignature is a no-op for Devin.
func (p *DevinProvider) VerifyWebhookSignature(_ []byte, _ string) error {
	return nil
}

// Models returns the Devin modes this provider can serve. Devin doesn't have
// traditional model selection — instead it has agent modes that control
// speed/capability tradeoffs. The model field in the config maps to devin_mode.
func (p *DevinProvider) Models() []ModelInfo {
	if p.model == "" {
		return []ModelInfo{{ID: "normal", Label: "Normal (default)"}}
	}
	return []ModelInfo{{ID: p.model, Label: p.model + " (" + p.name + ")"}}
}

// FetchModels returns the available Devin agent modes. Devin doesn't expose a
// /v1/models endpoint, but the devin_mode parameter accepts a fixed set of
// values: normal, fast, lite, ultra, fusion. We return these as the "models"
// so the admin can pick from the dropdown in the UI.
func (p *DevinProvider) FetchModels(_ context.Context) ([]ModelInfo, error) {
	return []ModelInfo{
		{ID: "normal", Label: "Normal — default Agent mode"},
		{ID: "fast", Label: "Fast — faster, lighter tasks"},
		{ID: "lite", Label: "Lite — Devin Lite mode"},
		{ID: "ultra", Label: "Ultra — maximum capability"},
		{ID: "fusion", Label: "Fusion — Fusion mode"},
	}, nil
}
