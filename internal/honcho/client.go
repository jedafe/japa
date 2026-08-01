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

// Health pings the Honcho service to report connectivity for the Control
// Panel infrastructure health group (ADR-027). Returns nil when the service
// responds with any 2xx/4xx status (reachable), and an error only when the
// HTTP call itself fails (unreachable).
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("honcho: creating health request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("honcho: health check: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	// 5xx means the service is up but unhealthy; anything else (including 404)
	// means the service is reachable.
	if resp.StatusCode >= 500 {
		return fmt.Errorf("honcho: health check returned status %d", resp.StatusCode)
	}

	return nil
}
