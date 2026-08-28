// Package ai — devin.go implements a provider for the Devin agent API.
//
// Devin (by Cognition) is an AI agent platform, not a synchronous LLM
// inference endpoint. The API is session-based and asynchronous:
//  1. POST /v3/organizations/{org_id}/sessions — create a session with a prompt
//  2. GET  /v3/organizations/{org_id}/sessions/{session_id} — poll for status
//  3. GET  /v3/organizations/{org_id}/sessions/{session_id}/messages — fetch
//     the final text when structured_output is not present
//
// This provider wraps the async flow into the synchronous Provider.Send()
// interface by polling until the session reaches a terminal state
// (exit/error/suspended) or the context deadline is reached.
//
// Authentication uses Bearer tokens with the "cog_" prefix (service user
// credentials). The org_id is encoded in the base_url field of the
// ProviderConfig as the full org-scoped base URL:
//
//	https://api.devin.ai/v3/organizations/org-abc123
//
// The admin enters this as the Base URL in the provider config.
//
// Ref: japaconnect-engineering/sections/20-ai-core.md
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
	"net/url"
	"strings"
	"time"
)

// Devin API constants.
const (
	DevinProviderName = "devin"
	DevinDefaultBase  = "https://api.devin.ai/v3/organizations"
	DevinPollInterval = 5 * time.Second
	DevinPollTimeout  = 3 * time.Minute // max time to wait for session completion
)

// DevinProvider implements the Provider interface for the Devin agent API.
type DevinProvider struct {
	name         string
	baseURL      string // e.g. "https://api.devin.ai/v3/organizations/org-abc123"
	apiKey       string // "cog_..." service user token
	model        string // default devin_mode ("normal", "fast", "lite", "ultra", "fusion")
	httpClient   *http.Client
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// NewDevinProvider creates a Devin agent provider.
//   - baseURL: the org-scoped base URL, e.g.
//     "https://api.devin.ai/v3/organizations/org-abc123"
//     If empty, defaults to the base orgs endpoint (admin must set org_id).
//   - apiKey: the service user token (starts with "cog_").
//   - model: the default devin_mode. The per-request "model" payload field
//     overrides this, so a single Devin provider can serve all Devin modes.
func NewDevinProvider(name, apiKey, baseURL, model string) *DevinProvider {
	if baseURL == "" {
		baseURL = DevinDefaultBase
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if name == "" {
		name = DevinProviderName
	}

	return &DevinProvider{
		name:         name,
		baseURL:      baseURL,
		apiKey:       apiKey,
		model:        model,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		pollInterval: DevinPollInterval,
		pollTimeout:  DevinPollTimeout,
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
	Prompt                   string         `json:"prompt"`
	DevinMode                string         `json:"devin_mode,omitempty"`
	CreateAsUserID           string         `json:"create_as_user_id,omitempty"`
	StructuredOutputRequired bool           `json:"structured_output_required"`
	StructuredOutputSchema   map[string]any `json:"structured_output_schema,omitempty"`
}

// devinSessionResponse is the response from the Devin v3 sessions API.
//
// The v3 API uses "session_id" (not "devin_id") and a status enum of
// "new", "claimed", "running", "resuming", "exit", "error", and
// "suspended". The final output lives in "structured_output" when the
// session uses structured output, otherwise in the session messages.
type devinSessionResponse struct {
	SessionID        string         `json:"session_id"`
	Status           string         `json:"status"`
	StatusDetail     string         `json:"status_detail,omitempty"`
	StructuredOutput map[string]any `json:"structured_output,omitempty"`
	URL              string         `json:"url"`
	DevinMode        string         `json:"devin_mode,omitempty"`
	Title            string         `json:"title,omitempty"`
}

// devinMessagesResponse is the paginated response from /sessions/{id}/messages.
type devinMessagesResponse struct {
	Items       []devinSessionMessage `json:"items"`
	HasNextPage bool                  `json:"has_next_page"`
	EndCursor   *string               `json:"end_cursor,omitempty"`
}

// devinSessionMessage is a single message in a Devin session.
type devinSessionMessage struct {
	EventID   string `json:"event_id"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"created_at"`
}

// Send creates a Devin session with the prompt, then polls until the session
// reaches a terminal state or the context deadline is reached. This wraps the
// async session-based API into the synchronous Provider.Send() interface.
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
	session, err := p.createSession(ctx, req, prompt)
	if err != nil {
		return Response{}, err
	}

	slog.Info("devin: session created", "provider", p.name, "session_id", session.SessionID, "status", session.Status, "url", session.URL)

	// Step 2: Poll until the session reaches a terminal state.
	pollCtx, cancel := context.WithTimeout(ctx, p.pollTimeout)
	defer cancel()

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return Response{
				ProviderRef: session.SessionID,
				Status:      "pending",
				Raw: map[string]any{
					"session_id": session.SessionID,
					"url":        session.URL,
					"status":     session.Status,
					"message":    "session still running — poll timed out",
				},
			}, fmt.Errorf("devin: session %s did not complete within %s", session.SessionID, p.pollTimeout)
		case <-ticker.C:
			if err := pollCtx.Err(); err != nil {
				// pollCtx expired at the same moment the ticker fired; don't
				// start another request with an already-cancelled context.
				return Response{
					ProviderRef: session.SessionID,
					Status:      "pending",
					Raw: map[string]any{
						"session_id": session.SessionID,
						"url":        session.URL,
						"status":     session.Status,
						"message":    "session still running — poll timed out",
					},
				}, fmt.Errorf("devin: session %s did not complete within %s", session.SessionID, p.pollTimeout)
			}

			current, err := p.getSession(pollCtx, session.SessionID)
			if err != nil {
				slog.Warn("devin: poll error", "session_id", session.SessionID, "error", err)
				continue
			}

			switch current.Status {
			case "error":
				msg := coalesce(current.StatusDetail, "unknown error")
				slog.Warn("devin: session failed", "session_id", current.SessionID, "detail", msg)
				return Response{
					ProviderRef: current.SessionID,
					Status:      "failed",
					Raw: map[string]any{
						"session_id": current.SessionID,
						"url":        current.URL,
						"status":     current.Status,
						"detail":     msg,
					},
				}, fmt.Errorf("devin: session %s failed: %s", current.SessionID, msg)
			case "suspended":
				msg := coalesce(current.StatusDetail, "session suspended")
				if isDevinQuotaError(current.StatusDetail) {
					slog.Warn("devin: session suspended (quota)", "session_id", current.SessionID, "detail", msg)
					return Response{
						ProviderRef: current.SessionID,
						Status:      "failed",
						Raw: map[string]any{
							"session_id": current.SessionID,
							"url":        current.URL,
							"status":     current.Status,
							"detail":     msg,
						},
					}, fmt.Errorf("devin: session %s suspended: %s", current.SessionID, msg)
				}

				// Suspended for a non-quota reason (e.g. inactivity, user request).
				// If the agent has already provided structured output, treat it as a
				// usable result; otherwise report pending.
				if len(current.StructuredOutput) > 0 {
					slog.Info("devin: session suspended with output", "session_id", current.SessionID, "detail", msg)
					return p.buildSuccessResponse(pollCtx, current)
				}

				slog.Warn("devin: session suspended", "session_id", current.SessionID, "detail", msg)
				return Response{
					ProviderRef: current.SessionID,
					Status:      "pending",
					Raw: map[string]any{
						"session_id": current.SessionID,
						"url":        current.URL,
						"status":     current.Status,
						"detail":     msg,
					},
				}, nil
			default:
				// "new", "claimed", "running", "resuming", or "exit".
				// Devin may produce validated structured output before the session
				// reaches "exit" (e.g. while "running" with status_detail
				// "waiting_for_user"). If we have output, return it immediately so
				// the sync handler can respond without holding the request open
				// until the session finishes.
				if len(current.StructuredOutput) > 0 {
					slog.Info("devin: session output available", "session_id", current.SessionID, "status", current.Status, "detail", current.StatusDetail)
					return p.buildSuccessResponse(pollCtx, current)
				}

				if current.Status == "exit" {
					slog.Info("devin: session completed", "session_id", current.SessionID)
					return p.buildSuccessResponse(pollCtx, current)
				}

				// "new", "claimed", "running", "resuming" with no output yet.
			}
		}
	}
}

// createSession sends POST /sessions to create a new Devin session.
func (p *DevinProvider) createSession(ctx context.Context, req Request, prompt string) (devinSessionResponse, error) {
	mode := p.devinMode(req)
	wantStructured, schema := p.structuredOutputConfig(req)

	reqBody := devinCreateSessionRequest{
		Prompt:                   prompt,
		DevinMode:                mode,
		StructuredOutputRequired: wantStructured,
		StructuredOutputSchema:   schema,
	}

	if c, ok := req.Payload["create_as_user_id"].(string); ok && c != "" {
		reqBody.CreateAsUserID = c
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

	if session.SessionID == "" {
		return devinSessionResponse{}, fmt.Errorf("devin: session response missing session_id")
	}

	return session, nil
}

// getSession sends GET /sessions/{session_id} to poll the session status.
func (p *DevinProvider) getSession(ctx context.Context, sessionID string) (devinSessionResponse, error) {
	endpoint := p.baseURL + "/sessions/" + sessionID
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

// getSessionMessages fetches all messages for a session, following pagination.
func (p *DevinProvider) getSessionMessages(ctx context.Context, sessionID string) ([]devinSessionMessage, error) {
	endpoint := p.baseURL + "/sessions/" + sessionID + "/messages"
	var all []devinSessionMessage
	cursor := ""

	for {
		u := endpoint
		if cursor != "" {
			u = endpoint + "?after=" + url.QueryEscape(cursor)
		}

		httpReq, err := newValidatedHTTPRequest(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("devin: creating messages request: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

		resp, err := p.httpClient.Do(httpReq) // #nosec G704 -- endpoint validated by newValidatedHTTPRequest
		if err != nil {
			return nil, fmt.Errorf("devin: messages request failed: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("devin: reading messages response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("devin: messages API error (status %d): %s", resp.StatusCode, truncate(string(respBody), 500))
		}

		var page devinMessagesResponse
		if err := json.Unmarshal(respBody, &page); err != nil {
			return nil, fmt.Errorf("devin: parsing messages response: %w", err)
		}

		all = append(all, page.Items...)

		if !page.HasNextPage || page.EndCursor == nil || *page.EndCursor == "" {
			break
		}
		cursor = *page.EndCursor
	}

	return all, nil
}

// buildSuccessResponse converts a terminal session into the standard Response
// envelope. It prefers structured_output, then falls back to the last Devin
// message in the session.
func (p *DevinProvider) buildSuccessResponse(ctx context.Context, session devinSessionResponse) (Response, error) {
	raw := map[string]any{
		"session_id": session.SessionID,
		"url":        session.URL,
		"status":     session.Status,
		"devin_mode": session.DevinMode,
	}

	output, err := p.extractOutput(ctx, session)
	if err != nil {
		return Response{}, err
	}

	raw["generated_text"] = output
	raw["reply"] = output

	return Response{
		ProviderRef: session.SessionID,
		Status:      "success",
		Raw:         raw,
	}, nil
}

// extractOutput returns the session's final text. It first uses
// structured_output, then falls back to the most recent Devin message.
func (p *DevinProvider) extractOutput(ctx context.Context, session devinSessionResponse) (string, error) {
	if len(session.StructuredOutput) > 0 {
		// Some callers (e.g. chat) may put a plain reply key in structured output.
		if v, ok := session.StructuredOutput["reply"].(string); ok && v != "" {
			return v, nil
		}

		b, err := json.Marshal(session.StructuredOutput)
		if err != nil {
			return "", fmt.Errorf("devin: encoding structured_output: %w", err)
		}
		return string(b), nil
	}

	messages, err := p.getSessionMessages(ctx, session.SessionID)
	if err != nil {
		return "", fmt.Errorf("devin: session %s finished but messages could not be read: %w", session.SessionID, err)
	}

	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Source == "devin" {
			return messages[i].Message, nil
		}
	}

	return "", fmt.Errorf("devin: session %s finished but produced no output", session.SessionID)
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

// Models returns the Devin modes this provider can serve. The admin-supplied
// config model is the default, but the per-request "model" payload field can
// override it, so we expose all Devin agent modes here (Rule 16: the UI picker
// must be derived from registered providers, not hardcoded).
func (p *DevinProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: "normal", Label: "Normal — default Agent mode"},
		{ID: "fast", Label: "Fast — faster, lighter tasks"},
		{ID: "lite", Label: "Lite — Devin Lite mode"},
		{ID: "ultra", Label: "Ultra — maximum capability"},
		{ID: "fusion", Label: "Fusion — Fusion mode"},
	}
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

// devinMode returns the devin_mode to use for this request, preferring the
// per-request "model" payload field and falling back to the configured default.
func (p *DevinProvider) devinMode(req Request) string {
	if m, ok := req.Payload["model"].(string); ok && isDevinMode(m) {
		return m
	}
	if isDevinMode(p.model) {
		return p.model
	}
	return "normal"
}

// structuredOutputConfig decides whether to request structured output and, if
// so, which JSON Schema to supply. We require structured output for tasks that
// explicitly request a JSON response (e.g. /ai/assist/draft) and disable it
// for open-ended tasks like chat so the agent can return plain text.
func (p *DevinProvider) structuredOutputConfig(req Request) (bool, map[string]any) {
	if schema, ok := req.Payload["structured_output_schema"].(map[string]any); ok {
		return true, schema
	}

	if task, ok := req.Payload["task"].(string); ok && task == "draft" {
		return true, draftStructuredOutputSchema()
	}

	if rf, ok := req.Payload["response_format"].(map[string]any); ok {
		t, _ := rf["type"].(string)
		switch t {
		case "json_object":
			// Devin's structured_output_required default is true, but it still
			// needs a schema to know what to emit. Provide a permissive
			// object schema when the caller only asks for JSON mode.
			return true, map[string]any{"type": "object"}
		case "json_schema":
			if js, ok := rf["json_schema"].(map[string]any); ok {
				if schema, ok := js["schema"].(map[string]any); ok {
					return true, schema
				}
				// Some callers may inline the schema directly under json_schema.
				return true, js
			}
		}
	}

	return false, nil
}

// isDevinMode reports whether mode is a valid Devin agent mode.
func isDevinMode(mode string) bool {
	switch mode {
	case "normal", "fast", "lite", "ultra", "fusion":
		return true
	}
	return false
}

// isDevinQuotaError reports whether a status_detail value indicates the session
// stopped because of quota, billing, or a hard error.
func isDevinQuotaError(detail string) bool {
	switch detail {
	case "out_of_credits", "out_of_quota", "no_quota_allocation",
		"payment_declined", "org_usage_limit_exceeded",
		"user_usage_limit_exceeded", "total_session_limit_exceeded",
		"usage_limit_exceeded", "error":
		return true
	}
	return false
}

// draftStructuredOutputSchema returns a JSON Schema for the output the
// /ai/assist/draft endpoint expects. The field_values property is open-ended
// because the available custom fields vary per content type.
func draftStructuredOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":     map[string]any{"type": "string"},
			"body_html": map[string]any{"type": "string"},
			"field_values": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		},
		"required": []string{"body_html"},
	}
}

// coalesce returns the first non-empty string.
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
