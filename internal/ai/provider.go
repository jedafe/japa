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
	// ChatWithTools dispatches a function-calling chat completion. It accepts
	// tool definitions and the full conversation history (including prior tool
	// results) and returns either a final text answer or one or more tool
	// invocations. Providers that do not support function calling (e.g.
	// agent-based providers) return ErrFunctionCallingUnsupported.
	//
	// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
	ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error)
	// VerifyWebhookSignature validates an incoming webhook callback.
	VerifyWebhookSignature(payload []byte, sig string) error
	// Models returns the model ids this provider serves, for the admin
	// model picker (Rule 16: derived from registered providers, not hardcoded).
	Models() []ModelInfo
	// FetchModels queries the provider's /v1/models endpoint (or equivalent)
	// to discover available models at runtime. Used by the AI Providers
	// management page so the admin can pick from a dropdown instead of
	// typing model names manually. Returns an empty list if the provider
	// does not expose a models endpoint (e.g. Devin).
	FetchModels(ctx context.Context) ([]ModelInfo, error)
}

// ModelInfo describes a single model exposed by a provider.
type ModelInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
