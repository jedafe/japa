// Package honcho provides an HTTP client for the Honcho agent-memory service.
// It supports user context retrieval, memory entry management, and user
// erasure during GDPR account deletion.
package honcho

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ContextEntry represents a single AI context entry stored in Honcho.
type ContextEntry struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// AddMemoryInput is the payload for adding a memory entry to a user's context.
type AddMemoryInput struct {
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Client calls the Honcho agent-memory service over HTTP.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Client that targets the given base URL. Trailing slashes
// are stripped so callers can pass either "http://localhost:8001" or
// "http://localhost:8001/".
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DeleteUser sends DELETE {baseURL}/users/{userID} to the Honcho service.
// Returns an error for any non-2xx response so the caller (the GDPR erasure
// job) can propagate it and let Asynq retry.
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("honcho: userID is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/users/"+userID, nil)
	if err != nil {
		return fmt.Errorf("honcho: creating delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("honcho: calling delete user %s: %w", userID, err)
	}

	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	// 404 means the user was already purged from Honcho — treat as success.
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("honcho: delete user %s returned status %d", userID, resp.StatusCode)
	}

	return nil
}

// GetContext retrieves AI context entries for a user from Honcho.
// Returns an empty slice (not nil) when the user has no entries.
func (c *Client) GetContext(ctx context.Context, userID string) ([]ContextEntry, error) {
	if userID == "" {
		return nil, errors.New("honcho: userID is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/users/"+userID+"/context", nil)
	if err != nil {
		return nil, fmt.Errorf("honcho: creating get context request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("honcho: getting context for user %s: %w", userID, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return []ContextEntry{}, nil
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("honcho: get context for user %s returned status %d", userID, resp.StatusCode)
	}

	var entries []ContextEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("honcho: decoding context entries: %w", err)
	}

	return entries, nil
}

// AddMemory adds a memory entry to a user's AI context in Honcho.
func (c *Client) AddMemory(ctx context.Context, userID string, input AddMemoryInput) (*ContextEntry, error) {
	if userID == "" {
		return nil, errors.New("honcho: userID is required")
	}

	if input.Content == "" {
		return nil, errors.New("honcho: content is required")
	}

	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("honcho: marshalling add memory input: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/users/"+userID+"/context", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("honcho: creating add memory request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("honcho: adding memory for user %s: %w", userID, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("honcho: add memory for user %s returned status %d", userID, resp.StatusCode)
	}

	var entry ContextEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return nil, fmt.Errorf("honcho: decoding context entry: %w", err)
	}

	return &entry, nil
}

// DeleteEntry deletes a specific context entry from Honcho.
func (c *Client) DeleteEntry(ctx context.Context, userID, entryID string) error {
	if userID == "" {
		return errors.New("honcho: userID is required")
	}

	if entryID == "" {
		return errors.New("honcho: entryID is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/users/"+userID+"/context/"+entryID, nil)
	if err != nil {
		return fmt.Errorf("honcho: creating delete entry request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("honcho: deleting entry %s for user %s: %w", entryID, userID, err)
	}

	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	// 404 means the entry was already deleted — treat as success (idempotent),
	// consistent with DeleteUser's 404 handling.
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("honcho: delete entry %s for user %s returned status %d", entryID, userID, resp.StatusCode)
	}

	return nil
}

// HealthReport carries the result of a Honcho health probe, including
// optional sync-lag and store-size metrics that ADR-027 / Section 21 ask
// the Control Panel health group to surface. The Honcho /health endpoint
// currently returns only {"status":"ok"}, so SyncLag and StoreSize are nil
// when the server does not report them — the caller treats nil as "metric
// not available" and the health check degrades to connectivity-only.
//
// Ref: japaconnect-engineering/sections/21-control-panel.md (line 104:
// application: { go_api, meilisearch, honcho }) and ADR-027 (connectivity,
// sync lag, store size).
type HealthReport struct {
	// Healthy is true when the service responds with a non-5xx status and
	// the body's "status" field (when present) is not an error sentinel.
	Healthy bool `json:"healthy"`

	// SyncLag is the number of pending/background work units that have not
	// yet been processed — a proxy for how far behind the reasoning queue
	// is. nil when the server does not report it.
	SyncLag *int `json:"sync_lag,omitempty"`

	// StoreSize is the total number of stored entries (users/messages) the
	// Honcho instance holds. nil when the server does not report it.
	StoreSize *int64 `json:"store_size,omitempty"`

	// Details is a human-readable summary shown in the Control Panel
	// health dashboard (Section 21 "details?: string").
	Details string `json:"details,omitempty"`
}

// Health pings the Honcho service and returns a HealthReport for the Control
// Panel application health group (ADR-027, Section 21 line 104). The report
// includes connectivity plus any sync-lag / store-size metrics the server
// exposes in the /health response body. When the server does not report a
// metric, the corresponding pointer field is nil and Details notes that only
// connectivity was confirmed.
//
// A 5-second timeout is enforced so that an unreachable Honcho service does
// not stall the entire /control/health endpoint (the default httpClient timeout
// is 30s, which would delay all infrastructure health results).
func (c *Client) Health(ctx context.Context) (*HealthReport, error) {
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(healthCtx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("honcho: creating health request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("honcho: health check: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	// 5xx means the service is up but unhealthy; anything else (including
	// 404) means the service is reachable.
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("honcho: health check returned status %d", resp.StatusCode)
	}

	report := &HealthReport{Healthy: true}

	// Parse the response body for any metrics the server chooses to expose.
	// The Honcho /health endpoint currently returns {"status":"ok"}, but we
	// read the body defensively so that if a future version adds sync-lag or
	// store-size fields (e.g. pending_work_units, total_users) they are
	// surfaced automatically without a client change.
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		// Body is not JSON (or empty) — connectivity is confirmed, no
		// metrics to extract.
		report.Details = "connectivity ok (no metrics reported)"
		return report, nil
	}

	// Confirm health from the status field when present.
	if status, ok := body["status"].(string); ok {
		switch strings.ToLower(status) {
		case "ok", "healthy", "up":
			report.Healthy = true
		case "error", "unhealthy", "down":
			report.Healthy = false
			report.Details = fmt.Sprintf("honcho: health status=%q", status)

			return report, nil
		}
	}

	// Extract sync lag — accept any of the known field names the Honcho
	// queue-status API or a future /health body might use.
	report.SyncLag = extractInt(body, "pending_work_units", "sync_lag", "pending")

	// Extract store size — accept any of the known field names.
	report.StoreSize = extractInt64(body, "total_users", "store_size", "total_entries", "total_messages")

	report.Details = buildHealthDetails(report)

	return report, nil
}

// extractInt looks up the first matching key in body that decodes to a
// JSON number, returning a pointer to the int value. Returns nil when no
// matching key is present or the value is not a number.
func extractInt(body map[string]any, keys ...string) *int {
	for _, k := range keys {
		if v, ok := body[k]; ok {
			switch n := v.(type) {
			case float64:
				i := int(n)
				return &i
			case int:
				return &n
			}
		}
	}

	return nil
}

// extractInt64 is the int64 variant of extractInt.
func extractInt64(body map[string]any, keys ...string) *int64 {
	for _, k := range keys {
		if v, ok := body[k]; ok {
			switch n := v.(type) {
			case float64:
				i := int64(n)
				return &i
			case int:
				i := int64(n)
				return &i
			}
		}
	}

	return nil
}

// buildHealthDetails renders a human-readable summary for the Control Panel
// health dashboard (Section 21 "details?: string").
func buildHealthDetails(r *HealthReport) string {
	if r.SyncLag == nil && r.StoreSize == nil {
		return "connectivity ok (metrics not reported)"
	}

	var parts []string
	if r.SyncLag != nil {
		parts = append(parts, fmt.Sprintf("sync_lag=%d", *r.SyncLag))
	}

	if r.StoreSize != nil {
		parts = append(parts, fmt.Sprintf("store_size=%d", *r.StoreSize))
	}

	return "connectivity ok, " + strings.Join(parts, ", ")
}
