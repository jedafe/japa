// Package honcho provides a minimal HTTP client for the Honcho agent-memory
// service. The only operation JapaConnect needs is user erasure during GDPR
// account deletion — Honcho must delete all AI context for the user or the
// erasure job retries.
package honcho

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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

	if resp.StatusCode >= 400 {
		return fmt.Errorf("honcho: delete user %s returned status %d", userID, resp.StatusCode)
	}

	return nil
}
