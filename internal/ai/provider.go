// Package ai implements the AI Core system (Section 20).
//
// Provider abstraction for swappable AI backends, a Gateway with
// circuit-breaker fallback across providers per category, and
// Human-in-the-Loop (HITL) review for high-stakes AI output.
package ai

import "context"

// Request is the standard envelope sent to any AI provider.
type Request struct {
	Category string         // e.g. "content_generation", "moderation", "embedding"
	Payload  map[string]any // provider-specific parameters
}

// Response is the standard envelope returned by any AI provider.
type Response struct {
	ProviderRef string         // provider-assigned reference ID for audit trail
	Status      string         // "success", "pending", "failed"
	Raw         map[string]any // provider-specific response data
}

// Provider defines the contract every AI backend must implement.
// Business logic never calls a vendor SDK directly; it always goes
// through this interface so providers can be swapped or fallback can
// be applied without touching callers.
type Provider interface {
	// Name returns a human-readable identifier for logs and dashboards.
	Name() string
	// Send dispatches a request to the AI backend and returns the response.
	Send(ctx context.Context, req Request) (Response, error)
	// VerifyWebhookSignature validates an incoming webhook callback.
	VerifyWebhookSignature(payload []byte, sig string) error
}
