// Package ai — types.go defines the function-calling (tool use) types used
// by the AI Agent Executor (Section 103, ADR-111).
//
// The existing Provider.Send path is a simple text-completion dispatcher
// (Request{Category, Payload} → Response{ProviderRef, Status, Raw}). It does
// not support LLM function calling, which the agent executor requires. These
// types, together with Provider.ChatWithTools, add that capability in a
// backward-compatible way: Send is unchanged, ChatWithTools is a new method.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 1)
package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrFunctionCallingUnsupported is returned by providers that do not support
// LLM function calling (e.g. agent-based providers like Cursor and Devin that
// wrap asynchronous agent sessions rather than synchronous chat completions).
// The gateway treats this as a hard failure for the agent category and falls
// back to the next provider.
var ErrFunctionCallingUnsupported = errors.New("ai: function calling not supported by this provider")

// HTTPStatusError wraps a provider's non-2xx response so callers can classify
// failures — e.g. mark a BYOK user key invalid on a 401/403 rejection —
// without string-parsing. Error() preserves the legacy message format.
type HTTPStatusError struct {
	Provider string
	// Op is the operation context that appeared in the legacy message
	// ("tools", "models", "embeddings", "stream"); "" yields plain "API error".
	Op     string
	Status int
	Body   string
}

func (e *HTTPStatusError) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("%s: %s API error (status %d): %s", e.Provider, e.Op, e.Status, truncate(e.Body, 500))
	}

	return fmt.Sprintf("%s: API error (status %d): %s", e.Provider, e.Status, truncate(e.Body, 500))
}

// IsProviderAuthError reports whether err (possibly wrapped) is a provider
// 401/403 rejection — the only failure class that marks a BYOK key invalid.
func IsProviderAuthError(err error) bool {
	var he *HTTPStatusError
	if errors.As(err, &he) {
		return he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden
	}

	return false
}

// LLMToolDefinition describes a single tool the LLM may invoke during a
// function-calling conversation. The Parameters field is a JSON Schema object
// (draft 2020-12) describing the arguments the LLM must produce.
type LLMToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall represents a single tool invocation requested by the LLM.
// Arguments is the raw JSON the model produced; the executor validates it
// against the corresponding LLMToolDefinition.Parameters schema before dispatch.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ChatMessage is the unified, provider-agnostic message type for
// function-calling conversations. Each provider translates these to its
// native wire format:
//
//   - OpenAI/Xiaomi: role/content strings, assistant messages carry tool_calls,
//     tool-result messages use role "tool" with tool_call_id.
//   - Anthropic: system is top-level, user/assistant roles only, content is an
//     array of typed blocks (text, tool_use, tool_result).
type ChatMessage struct {
	// Role is one of "system", "user", "assistant", "tool".
	Role string `json:"role"`
	// Content is the text body. For assistant messages that contain only
	// tool calls this may be empty. For tool-result messages this is the
	// serialized tool result.
	Content string `json:"content,omitempty"`
	// ToolCalls is present on assistant messages when the LLM requests
	// tool invocations.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID is present on tool-result messages (role "tool") and
	// correlates the result to the originating assistant tool call.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Name is the tool name on tool-result messages (OpenAI requires this
	// on the tool role message).
	Name string `json:"name,omitempty"`
}

// ChatWithToolsRequest is the request for a function-calling chat completion.
// It carries the full conversation history (including prior tool results) so
// the LLM can continue the agent loop.
type ChatWithToolsRequest struct {
	Messages    []ChatMessage       `json:"messages"`
	Tools       []LLMToolDefinition `json:"tools,omitempty"`
	Model       string              `json:"model,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

// ChatWithToolsResponse is the result of a function-calling chat completion.
// Either Content (a final text answer) or ToolCalls (one or more tool
// invocations) is populated; both can be present when the model emits text
// alongside tool calls. Citations are populated by providers that return
// source URLs (e.g. Perplexity sonar models) so the agent can attribute
// claims.
type ChatWithToolsResponse struct {
	Content      string     `json:"content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"` // "stop", "tool_calls", "length", "content_filter", etc.
	Model        string     `json:"model,omitempty"`
	Usage        TokenUsage `json:"usage"`
	ProviderRef  string     `json:"provider_ref,omitempty"`
	ProviderName string     `json:"provider_name,omitempty"` // set by the dispatcher from the winning provider
	Citations    []string   `json:"citations,omitempty"`
}

// TokenUsage reports token consumption in a provider-agnostic way. OpenAI and
// Xiaomi use prompt/completion/total; Anthropic uses input/output. The
// providers normalize to this common shape.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// HasToolCalls reports whether the response contains any tool invocations.
func (r ChatWithToolsResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}
