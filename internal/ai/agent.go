// Package ai — agent.go implements the AI Agent loop (Section 103, ADR-111).
//
// The AgentHandler is the orchestration layer that ties together the Phase 1
// gateway function-calling extension, the Phase 3 tool executor, the Phase 4
// tool catalog, and the Phase 5 pre-search step. It is the entry point for
// POST /api/v1/ai/agent (wired in Phase 6).
//
// The flow per §103 "Agent Loop":
//
//  1. Pre-checks: agent.enabled toggle, agent.use RBAC permission, daily
//     rate limit (jc:agent:rate:{user_id}:{date}).
//  2. Resolve the user's geo context from users.current_geo_space_id.
//  3. Run the pre-search (content + RAG, privacy-filtered, classified).
//  4. Create an ai_agent_sessions row with the pre-search level.
//  5. Build the tool catalog filtered by the user's RBAC grants (cached in
//     jc:agent:tools:{user_id} for 10 minutes).
//  6. Build the system prompt with behavior rules + geo context + pre-search.
//  7. LLM function-calling loop via gateway.ChatWithTools, dispatching each
//     tool call through the Executor. Enforced guardrails: max tool calls
//     (agent.max_tool_calls), timeout (agent.timeout_seconds), per-request
//     cost limit (agent.cost_limit_per_request_usd).
//  8. Emit AgentEvents to a channel for SSE streaming.
//  9. Complete or fail the ai_agent_sessions row at the end.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Agent Loop, System Prompt Behavior Rules)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 5, tasks 5.04-5.12)
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
)

// ---------------------------------------------------------------------------
// Agent events (task 5.12 — SSE streaming)
// ---------------------------------------------------------------------------

// AgentEvent is a single event emitted on the agent event channel. The Phase
// 6 SSE handler marshals each event to JSON and writes it as an SSE
// `data: {json}\n\n` line. The Type field drives the frontend rendering.
type AgentEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`

	// session_start / progress
	PresearchLevel string `json:"presearch_level,omitempty"`
	Iteration      int    `json:"iteration,omitempty"`
	Message        string `json:"message,omitempty"`

	// tool_call_start / tool_call_result
	ToolName  string `json:"tool_name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Status    string `json:"status,omitempty"`

	// tool_call_result / final_response / error
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`

	// final_response
	TokensUsed    int     `json:"tokens_used,omitempty"`
	EstimatedCost float64 `json:"estimated_cost,omitempty"`
}

// Agent event type constants.
const (
	EventSessionStart   = "session_start"
	EventToolCallStart  = "tool_call_start"
	EventToolCallResult = "tool_call_result"
	EventProgress       = "progress"
	EventFinalResponse  = "final_response"
	EventError          = "error"
)

// ---------------------------------------------------------------------------
// Dependency interfaces (not already defined elsewhere in the package)
// ---------------------------------------------------------------------------

// SessionStore is the subset of gen.Queries the agent loop uses for
// ai_agent_sessions lifecycle management. *gen.Queries satisfies this.
type SessionStore interface {
	CreateAgentSession(ctx context.Context, arg gen.CreateAgentSessionParams) (gen.AiAgentSession, error)
	CompleteAgentSession(ctx context.Context, arg gen.CompleteAgentSessionParams) (gen.AiAgentSession, error)
	FailAgentSession(ctx context.Context, arg gen.FailAgentSessionParams) (gen.AiAgentSession, error)
	CancelAgentSession(ctx context.Context, id pgtype.UUID) (gen.AiAgentSession, error)
}

// GeoSpaceResolver resolves the user's current geo space. *gen.Queries
// satisfies this via GetUserCurrentGeoSpace.
type GeoSpaceResolver interface {
	GetUserCurrentGeoSpace(ctx context.Context, userID pgtype.UUID) (gen.GetUserCurrentGeoSpaceRow, error)
}

// RateLimiter is the subset of Valkey commands the agent uses for the
// jc:agent:rate:{user_id}:{date} daily counter and the
// jc:agent:cost:{user_id}:{date} daily cost accumulator.
// valkeycompat.Cmdable satisfies this.
type RateLimiter interface {
	Incr(ctx context.Context, key string) (int64, error)
	IncrByFloat(ctx context.Context, key string, increment float64) (float64, error)
	Expire(ctx context.Context, key string, ttlSeconds int) error
	Get(ctx context.Context, key string) (string, error)
}

// ---------------------------------------------------------------------------
// AgentHandler
// ---------------------------------------------------------------------------

// AgentHandler orchestrates the agent loop. It is the single entry point for
// agent requests; the Phase 6 HTTP handler calls HandleAgentRequest and
// streams the returned event channel over SSE.
type AgentHandler struct {
	gateway     *Dispatcher
	executor    *Executor
	registry    *ToolHandlerRegistry
	presearch   *PreSearcher
	perms       PermissionChecker
	settings    SettingsReader
	sessions    SessionStore
	geo         GeoSpaceResolver
	rateLimiter RateLimiter
	cache       CacheClient     // for jc:agent:tools:{user_id} catalog cache
	persona     PersonaResolver // Phase 8: resolve agent.default_persona_id → user_id for draft requests

	// llmCategory is the gateway provider category used for ChatWithTools
	// dispatch. Defaults to "chat" — the agent uses the same chat-capable
	// providers as the existing /ai/chat endpoint. The wiring layer can
	// override this to route the agent to a dedicated category.
	llmCategory string
}

// AgentOption configures an AgentHandler.
type AgentOption func(*AgentHandler)

// NewAgentHandler creates an AgentHandler. The gateway, executor, registry,
// presearcher, permission checker, and settings reader are required; the
// session store, geo resolver, rate limiter, and cache are optional but
// should be provided by the wiring layer for full functionality.
func NewAgentHandler(gateway *Dispatcher, executor *Executor, registry *ToolHandlerRegistry, presearch *PreSearcher, opts ...AgentOption) *AgentHandler {
	h := &AgentHandler{
		gateway:     gateway,
		executor:    executor,
		registry:    registry,
		presearch:   presearch,
		llmCategory: "chat",
	}

	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}

	return h
}

// WithAgentPerms injects the RBAC permission checker.
func WithAgentPerms(p PermissionChecker) AgentOption {
	return func(h *AgentHandler) { h.perms = p }
}

// WithAgentSettings injects the hot-reloadable settings reader.
func WithAgentSettings(s SettingsReader) AgentOption {
	return func(h *AgentHandler) { h.settings = s }
}

// WithAgentSessions injects the session store (gen.Queries).
func WithAgentSessions(s SessionStore) AgentOption {
	return func(h *AgentHandler) { h.sessions = s }
}

// WithAgentGeoResolver injects the geo space resolver.
func WithAgentGeoResolver(r GeoSpaceResolver) AgentOption {
	return func(h *AgentHandler) { h.geo = r }
}

// WithAgentRateLimiter injects the Valkey rate limiter.
func WithAgentRateLimiter(r RateLimiter) AgentOption {
	return func(h *AgentHandler) { h.rateLimiter = r }
}

// WithAgentCache injects the Valkey cache for the tool catalog cache.
func WithAgentCache(c CacheClient) AgentOption {
	return func(h *AgentHandler) { h.cache = c }
}

// WithAgentLLMCategory overrides the gateway provider category.
func WithAgentLLMCategory(cat string) AgentOption {
	return func(h *AgentHandler) { h.llmCategory = cat }
}

// WithAgentPersona injects the AI persona resolver (Phase 8). The agent
// loop uses it to resolve agent.default_persona_id → the persona's backing
// users.id when a user without content.create prompts the agent about
// content that doesn't exist (pre-search = NONE). The resolved user_id is
// injected into the system prompt so the LLM passes it to the
// content.create_draft tool as the author_id.
func WithAgentPersona(p PersonaResolver) AgentOption {
	return func(h *AgentHandler) { h.persona = p }
}

// ---------------------------------------------------------------------------
// HandleAgentRequest (tasks 5.04-5.12)
// ---------------------------------------------------------------------------

// agentRateLimitTTL is the TTL for jc:agent:rate:{user_id}:{date} keys.
// The counter lives until midnight UTC. We approximate this with a 24-hour
// TTL — the date component in the key ensures a new key is used each day,
// so the TTL is a safety upper bound, not the exact midnight boundary.
const agentRateLimitTTL = 24 * time.Hour

// agentCostAccumulatorTTL is the TTL for jc:agent:cost:{user_id}:{date}
// keys. Per the Valkey keys catalog: until midnight UTC. We approximate
// this with a 24-hour TTL — the date component in the key ensures a new
// key is used each day, so the TTL is a safety upper bound.
const agentCostAccumulatorTTL = 24 * time.Hour

// agentCatalogCacheTTL is the TTL for jc:agent:tools:{user_id} keys.
// Per the Valkey keys catalog: 10 minutes.
const agentCatalogCacheTTL = 10 * time.Minute

// agentSessionStateTTL is the TTL for jc:agent:session:{session_id} keys.
// Per the Valkey keys catalog: 5 minutes. Used for cancellation checks.
const agentSessionStateTTL = 5 * time.Minute

// agentDefaults are the §103 system_settings defaults used when the
// settings reader is nil or a key is absent.
const (
	defaultMaxToolCalls    = 20
	defaultTimeoutSeconds  = 60
	defaultCostLimitUSD    = 0.50
	defaultRateLimitPerDay = 50
	defaultDailyBudgetUSD  = 5.00
	defaultLLMTemperature  = 0.3
)

// HandleAgentRequest runs the agent loop for a single user prompt. It
// performs the synchronous pre-checks (enabled, permission, rate limit),
// resolves geo context, runs the pre-search, creates the session record,
// and returns a channel of AgentEvents. The LLM loop runs in a goroutine
// that emits events to the channel and closes it when the loop finishes.
//
// The returned error is non-nil only for pre-check failures (agent disabled,
// no permission, rate limited, pre-search failure, session creation
// failure). Errors that occur during the LLM loop are emitted as
// EventError events on the channel, not returned here.
func (h *AgentHandler) HandleAgentRequest(ctx context.Context, userID, tenantID pgtype.UUID, prompt string) (<-chan AgentEvent, error) {
	if h == nil {
		return nil, errAgentNotConfigured
	}

	if prompt == "" {
		return nil, errAgentEmptyPrompt
	}

	// --- Task 5.10: agent.enabled toggle ---
	if !h.getBoolSetting(ctx, tenantID, "agent.enabled", true) {
		return nil, errAgentDisabled
	}

	// --- Task 5.05: agent.use RBAC permission ---
	if h.perms != nil {
		if !h.perms.HasPermission(ctx, userID, tenantID, "agent.use") {
			return nil, errAgentNoPermission
		}
	}

	// --- Task 5.11: rate limiting (jc:agent:rate:{user_id}:{date}) ---
	if err := h.checkRateLimit(ctx, userID, tenantID); err != nil {
		return nil, errAgentRateLimited
	}

	// --- Guide §19: daily budget enforcement (jc:agent:cost:{user_id}:{date}) ---
	// Rejects the request if the user's accumulated daily AI spend has
	// reached agent.daily_budget_usd. A budget of 0 disables the check.
	if err := h.checkDailyBudget(ctx, userID, tenantID); err != nil {
		return nil, errAgentBudgetExceeded
	}

	// --- Task 5.05: resolve geo context from users.current_geo_space_id ---
	geoSpaceID, geoSummary := h.resolveGeoContext(ctx, userID, tenantID)

	// --- Tasks 5.01-5.03: pre-search ---
	var preSearchResult *PreSearchResult

	if h.presearch != nil {
		var err error

		preSearchResult, err = h.presearch.PreSearch(ctx, tenantID, prompt, geoSpaceID)
		if err != nil {
			slog.Warn("ai: pre-search failed, proceeding without it", "error", err)

			preSearchResult = &PreSearchResult{Level: PreSearchNone}
		}
	} else {
		preSearchResult = &PreSearchResult{Level: PreSearchNone}
	}

	// --- Task 5.09: create ai_agent_sessions record ---
	session, err := h.createSession(ctx, userID, tenantID, prompt, geoSpaceID, preSearchResult.Level)
	if err != nil {
		return nil, fmt.Errorf("ai: creating agent session: %w", err)
	}

	// --- Set session state in Valkey for cancellation checks ---
	h.setSessionState(ctx, tenantID, session.ID)

	// --- Phase 8: resolve the draft-request context (Content Request Flow).
	// When pre-search = NONE and the user has agent.draft_request but not
	// content.create, the agent should search externally and create a draft
	// for admin review. The persona user_id is resolved here so the system
	// prompt can instruct the LLM to pass it to content.create_draft. ---
	draftCtx := h.resolveDraftRequestContext(ctx, userID, tenantID, preSearchResult.Level)

	// --- Return channel and run the loop in a goroutine ---
	events := make(chan AgentEvent, 32)

	go h.runLoop(ctx, session, userID, tenantID, prompt, geoSpaceID, geoSummary, preSearchResult, draftCtx, events)

	return events, nil
}

// draftRequestContext holds the Phase 8 Content Request Flow context. When
// Active is true, the agent loop injects draft-request instructions + the
// persona user_id into the system prompt so the LLM uses external.search
// + content.create_draft instead of content.create.
type draftRequestContext struct {
	Active        bool   // true when the draft-request path applies
	PersonaUserID string // the persona's backing users.id (UUID string)
	PersonaID     string // the agent.default_persona_id (UUID string)
}

// resolveDraftRequestContext determines whether the current request is on
// the Phase 8 draft-request path (§103 Content Request Flow). The path is
// active when ALL of the following hold:
//   - pre-search level is NONE (no existing content covers the request)
//   - the user has agent.draft_request permission
//   - the user does NOT have content.create permission (users with
//     content.create use the normal content.create tool with themselves as
//     author)
//   - agent.default_persona_id is configured and resolves to a persona
//
// When active, the persona's backing users.id is resolved and returned so
// the system prompt can instruct the LLM to pass it to content.create_draft
// as the author_id. Resolution failures (no persona configured, persona not
// found) degrade gracefully — the path stays inactive and the LLM falls
// back to presenting external results without creating a draft.
func (h *AgentHandler) resolveDraftRequestContext(ctx context.Context, userID, tenantID pgtype.UUID, level PreSearchLevel) draftRequestContext {
	if level != PreSearchNone || h.perms == nil || h.persona == nil {
		return draftRequestContext{}
	}

	if !h.perms.HasPermission(ctx, userID, tenantID, "agent.draft_request") {
		return draftRequestContext{}
	}

	if h.perms.HasPermission(ctx, userID, tenantID, "content.create") {
		// User has full create capability — they use content.create, not
		// the draft-request path.
		return draftRequestContext{}
	}

	personaIDStr := h.getStringSetting(ctx, tenantID, "agent.default_persona_id", "")
	if personaIDStr == "" || personaIDStr == "\"\"" {
		return draftRequestContext{}
	}

	personaID, err := scanUUID(personaIDStr)
	if err != nil || !personaID.Valid {
		return draftRequestContext{}
	}

	pid, err := h.persona.GetPersonaUserID(ctx, tenantID, personaID)
	if err != nil || !pid.Valid {
		slog.Warn("ai: draft-request persona resolution failed", "persona_id", personaIDStr, "error", err)
		return draftRequestContext{}
	}

	return draftRequestContext{
		Active:        true,
		PersonaUserID: pid.String(),
		PersonaID:     personaIDStr,
	}
}

// getStringSetting reads a string setting with fallback.
func (h *AgentHandler) getStringSetting(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string {
	if h.settings == nil {
		return fallback
	}

	return h.settings.GetStringOr(ctx, tenantID, key, fallback)
}

// ---------------------------------------------------------------------------
// runLoop — the LLM function-calling loop (tasks 5.06-5.10, 5.12)
// ---------------------------------------------------------------------------

func (h *AgentHandler) runLoop(
	ctx context.Context,
	session gen.AiAgentSession,
	userID, tenantID pgtype.UUID,
	prompt string,
	geoSpaceID *pgtype.UUID,
	geoSummary string,
	preSearchResult *PreSearchResult,
	draftCtx draftRequestContext,
	events chan<- AgentEvent,
) {
	defer close(events)

	// Enforce timeout (agent.timeout_seconds). The loop ctx is derived from
	// the request ctx with the configured timeout.
	timeout := time.Duration(h.getIntSetting(ctx, tenantID, "agent.timeout_seconds", defaultTimeoutSeconds)) * time.Second

	loopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	maxToolCalls := h.getIntSetting(ctx, tenantID, "agent.max_tool_calls", defaultMaxToolCalls)
	costLimit := h.getFloatSetting(ctx, tenantID, "agent.cost_limit_per_request_usd", defaultCostLimitUSD)
	temperature := h.getFloatSetting(ctx, tenantID, "agent.temperature", defaultLLMTemperature)

	// Cost estimator uses per-provider/model price settings so the agent's
	// per-request cost limit reflects the actual configured LLM price.
	costEstimator := NewCostEstimator(h.settings)

	// sendEvent writes an event to the channel, respecting the original
	// request context cancellation. If the SSE consumer stops reading
	// (client disconnect → ctx cancelled), the loop unblocks via
	// ctx.Done() instead of blocking forever. The loopCtx timeout does
	// NOT close the event channel — the timeout only cancels LLM calls.
	// This ensures the terminal error/final_response events can still be
	// sent after the timeout fires, as long as the consumer is reading.
	sendEvent := func(ev AgentEvent) bool {
		select {
		case events <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Emit session_start event.
	if !sendEvent(AgentEvent{
		Type:           EventSessionStart,
		SessionID:      session.ID.String(),
		PresearchLevel: string(preSearchResult.Level),
		Message:        preSearchResult.Summary(),
	}) {
		h.failSession(ctx, session, loopCtx.Err(), 0, 0, 0)
		return
	}

	// --- Task 5.06: build tool catalog (cached) ---
	tools := h.buildToolCatalog(loopCtx, userID, tenantID)
	toolDefs := configsToToolDefinitions(tools)

	// --- Task 5.07: build system prompt ---
	systemPrompt := buildSystemPrompt(geoSummary, preSearchResult, tools)

	// --- Phase 8: when the draft-request path is active, append the
	// persona user_id + draft-request instructions so the LLM uses
	// external.search + content.create_draft instead of content.create. ---
	if draftCtx.Active {
		systemPrompt += buildDraftRequestPromptSection(draftCtx)
	}

	// --- Task 5.08: LLM function-calling loop ---
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	var (
		totalTokens   int
		estimatedCost float64
		toolCallCount int
		finalResponse string
		loopErr       error
		cancelled     bool
		maxedOut      bool
	)

	for iteration := range maxToolCalls {
		// Check for user-initiated cancellation (jc:agent:session:{tenant_id}:{id} = "cancelled").
		if h.isCancelled(loopCtx, tenantID, session.ID) {
			cancelled = true
			break
		}

		// Progress event.
		if !sendEvent(AgentEvent{
			Type:      EventProgress,
			SessionID: session.ID.String(),
			Iteration: iteration + 1,
			Message:   fmt.Sprintf("LLM iteration %d", iteration+1),
		}) {
			loopErr = loopCtx.Err()
			break
		}

		resp, err := h.gateway.ChatWithTools(loopCtx, h.llmCategory, ChatWithToolsRequest{
			Messages:    messages,
			Tools:       toolDefs,
			Temperature: temperature,
		})
		if err != nil {
			loopErr = fmt.Errorf("agent: LLM call failed: %w", err)
			break
		}

		// Accumulate token usage + cost using the per-provider/model price.
		totalTokens += resp.Usage.TotalTokens
		estimatedCost += costEstimator.Estimate(loopCtx, tenantID, resp.ProviderName, resp.Model, resp.Usage.TotalTokens)

		// --- Cost limit check (task 5.10) ---
		if estimatedCost > costLimit {
			loopErr = fmt.Errorf("agent: cost limit exceeded (%.4f > %.4f USD)", estimatedCost, costLimit)
			break
		}

		// No tool calls → final response.
		if !resp.HasToolCalls() {
			finalResponse = resp.Content
			break
		}

		// Append the assistant message with tool calls to the conversation.
		messages = append(messages, ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Dispatch each tool call through the executor.
		for _, call := range resp.ToolCalls {
			toolCallCount++

			// Emit tool_call_start event.
			if !sendEvent(AgentEvent{
				Type:      EventToolCallStart,
				SessionID: session.ID.String(),
				ToolName:  call.Name,
				Arguments: string(call.Arguments),
			}) {
				loopErr = loopCtx.Err()
				break
			}

			result, execErr := h.executor.Execute(loopCtx, ExecuteParams{
				Call:      call,
				SessionID: session.ID,
				UserID:    userID,
			})

			// The executor returns (result, err). When both are non-nil,
			// the result already carries the structured error content
			// (with code + message) — prefer it over a generic errorJSON.
			// Only synthesize error content when result is nil.
			var resultContent, resultStatus string
			if result != nil {
				resultContent = result.Content
				resultStatus = result.Status
			} else if execErr != nil {
				resultStatus = ToolCallStatusError
				resultContent = errorJSON(execErr)
			}

			// Emit tool_call_result event.
			if !sendEvent(AgentEvent{
				Type:      EventToolCallResult,
				SessionID: session.ID.String(),
				ToolName:  call.Name,
				Status:    resultStatus,
				Content:   resultContent,
			}) {
				loopErr = loopCtx.Err()
				break
			}

			// Append the tool result to the conversation for the next LLM call.
			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    resultContent,
				ToolCallID: call.ID,
				Name:       call.Name,
			})
		}

		// If the inner loop broke due to sendEvent failure, break the outer loop.
		if loopErr != nil {
			break
		}

		// If this was the last allowed iteration and the model still wants
		// tools, the next iteration won't run — mark as maxed out.
		if iteration == maxToolCalls-1 && resp.HasToolCalls() {
			maxedOut = true
		}
	}

	// --- Task 5.09: complete, cancel, or fail the session ---

	// User-initiated cancellation → CancelAgentSession (status = 'cancelled').
	if cancelled {
		h.cancelSession(ctx, session)
		h.trackCost(ctx, userID, estimatedCost)
		sendEvent(AgentEvent{
			Type:      EventError,
			SessionID: session.ID.String(),
			Error:     "agent: session cancelled by user",
		})

		return
	}

	if loopErr != nil {
		h.failSession(ctx, session, loopErr, toolCallCount, totalTokens, estimatedCost)
		h.trackCost(ctx, userID, estimatedCost)
		sendEvent(AgentEvent{
			Type:      EventError,
			SessionID: session.ID.String(),
			Error:     loopErr.Error(),
		})

		return
	}

	// Max iterations exhausted without a final answer → fail per §103.
	if maxedOut || finalResponse == "" {
		loopErr = fmt.Errorf("agent: max tool calls exceeded (%d)", maxToolCalls)
		h.failSession(ctx, session, loopErr, toolCallCount, totalTokens, estimatedCost)
		h.trackCost(ctx, userID, estimatedCost)
		sendEvent(AgentEvent{
			Type:      EventError,
			SessionID: session.ID.String(),
			Error:     loopErr.Error(),
		})

		return
	}

	h.completeSession(ctx, session, finalResponse, toolCallCount, totalTokens, estimatedCost)
	h.trackCost(ctx, userID, estimatedCost)
	sendEvent(AgentEvent{
		Type:          EventFinalResponse,
		SessionID:     session.ID.String(),
		Content:       finalResponse,
		TokensUsed:    totalTokens,
		EstimatedCost: estimatedCost,
	})
}

// ---------------------------------------------------------------------------
// Helpers — settings, geo, rate limit, catalog, session lifecycle
// ---------------------------------------------------------------------------

// getIntSetting reads an int setting with fallback.
func (h *AgentHandler) getIntSetting(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int {
	if h.settings == nil {
		return fallback
	}

	return h.settings.GetIntOr(ctx, tenantID, key, fallback)
}

// getFloatSetting reads a float setting with fallback.
func (h *AgentHandler) getFloatSetting(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64 {
	if h.settings == nil {
		return fallback
	}

	return h.settings.GetFloatOr(ctx, tenantID, key, fallback)
}

// getBoolSetting reads a bool setting with fallback.
func (h *AgentHandler) getBoolSetting(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool {
	if h.settings == nil {
		return fallback
	}

	return h.settings.GetBoolOr(ctx, tenantID, key, fallback)
}

// resolveGeoContext fetches the user's current_geo_space_id and builds a
// human-readable summary for the system prompt. Returns (nil, "") when the
// geo resolver is not configured or the user has no geo space set.
func (h *AgentHandler) resolveGeoContext(ctx context.Context, userID, tenantID pgtype.UUID) (*pgtype.UUID, string) {
	if h.geo == nil {
		return nil, ""
	}

	row, err := h.geo.GetUserCurrentGeoSpace(ctx,
		userID,
	)
	if err != nil {
		slog.Warn("ai: failed to resolve user geo context", "error", err)
		return nil, ""
	}

	var geoSpaceID *pgtype.UUID
	if row.CurrentGeoSpaceID.Valid {
		geoSpaceID = &row.CurrentGeoSpaceID
	}

	var parts []string
	if row.CityName != "" {
		parts = append(parts, "City: "+row.CityName)
	}

	if row.CountryName != nil {
		parts = append(parts, "Country: "+*row.CountryName)
	}

	if row.CountryGeoCode != nil {
		parts = append(parts, "Country code: "+*row.CountryGeoCode)
	}

	summary := ""
	if len(parts) > 0 {
		summary = fmt.Sprintf("User's geo context: %s.", strings.Join(parts, ", "))
	}

	return geoSpaceID, summary
}

// checkRateLimit increments the jc:agent:rate:{user_id}:{date} counter and
// rejects the request if the daily limit is exceeded. When the rate limiter
// is nil, rate limiting is skipped (graceful degradation — the wiring layer
// SHOULD provide one).
func (h *AgentHandler) checkRateLimit(ctx context.Context, userID, tenantID pgtype.UUID) error {
	if h.rateLimiter == nil {
		return nil
	}

	key := valkey.MustKey("agent:rate:%s:%s", userID.String(), time.Now().UTC().Format("2006-01-02"))

	count, err := h.rateLimiter.Incr(ctx, key)
	if err != nil {
		slog.Warn("ai: rate limit check failed, allowing request", "error", err)
		return nil // fail-open on Valkey error — don't block the user
	}

	// Set TTL on first increment so the key expires.
	if count == 1 {
		if err := h.rateLimiter.Expire(ctx, key, int(agentRateLimitTTL.Seconds())); err != nil {
			slog.Warn("ai: failed to set rate limit key TTL", "error", err)
		}
	}

	limit := h.getIntSetting(ctx, tenantID, "agent.rate_limit_per_day", defaultRateLimitPerDay)
	if int(count) > limit {
		return fmt.Errorf("ai: daily agent rate limit exceeded (%d/%d)", count, limit)
	}

	return nil
}

// checkDailyBudget reads the jc:agent:cost:{user_id}:{date} accumulator and
// rejects the request if the user's accumulated daily AI spend has reached
// agent.daily_budget_usd (Guide §19 — budget enforcement). A budget of 0
// disables the check. When the rate limiter is nil or the Valkey GET fails,
// the check is skipped (fail-open) so a Valkey outage does not lock users
// out of the agent — the per-request cost limit (agent.cost_limit_per_request_usd)
// still applies as a per-session guard.
func (h *AgentHandler) checkDailyBudget(ctx context.Context, userID, tenantID pgtype.UUID) error {
	if h.rateLimiter == nil {
		return nil
	}

	budget := h.getFloatSetting(ctx, tenantID, "agent.daily_budget_usd", defaultDailyBudgetUSD)
	if budget <= 0 {
		return nil // 0 = disabled
	}

	key := valkey.MustKey("agent:cost:%s:%s", userID.String(), time.Now().UTC().Format("2006-01-02"))

	raw, err := h.rateLimiter.Get(ctx, key)
	if err != nil {
		// redis.Nil on a fresh day is expected — no cost accumulated yet.
		// Any other error is a Valkey issue; fail-open so the agent stays
		// usable. The per-request cost limit still guards the session.
		return nil
	}

	if raw == "" {
		return nil
	}

	total, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		slog.Warn("ai: daily budget check, unparseable cost value", "key", key, "value", raw, "error", err)
		return nil
	}

	if total >= budget {
		return fmt.Errorf("ai: daily agent budget exceeded (%.4f/%.2f USD)", total, budget)
	}

	return nil
}

// trackCost accumulates the session's estimated cost into the
// jc:agent:cost:{user_id}:{date} Valkey key (valkey-keys-catalog.md:237).
// This enables per-user daily cost monitoring and budget enforcement beyond
// the per-request cost limit. Best-effort: a Valkey error logs a warning but
// does not fail the session.
func (h *AgentHandler) trackCost(ctx context.Context, userID pgtype.UUID, cost float64) {
	if h.rateLimiter == nil || cost <= 0 {
		return
	}

	key := valkey.MustKey("agent:cost:%s:%s", userID.String(), time.Now().UTC().Format("2006-01-02"))

	total, err := h.rateLimiter.IncrByFloat(ctx, key, cost)
	if err != nil {
		slog.Warn("ai: failed to track agent cost", "error", err)
		return
	}
	// Set TTL on first increment so the key expires.
	if total == cost {
		if err := h.rateLimiter.Expire(ctx, key, int(agentCostAccumulatorTTL.Seconds())); err != nil {
			slog.Warn("ai: failed to set agent cost key TTL", "error", err)
		}
	}
}

// buildToolCatalog returns the tool configs available to the user, filtered
// by RBAC. The result is cached in jc:agent:tools:{user_id} for 10 minutes
// (task 5.06). When the registry or perms checker is nil, returns nil.
func (h *AgentHandler) buildToolCatalog(ctx context.Context, userID, tenantID pgtype.UUID) []ToolConfig {
	if h.registry == nil {
		return nil
	}

	// Try cache first.
	cacheKey := valkey.MustKey("agent:tools:%s", userID.String())
	if h.cache != nil {
		if cached, err := h.cache.Get(ctx, cacheKey); err == nil && cached != "" {
			var tools []ToolConfig
			if err := json.Unmarshal([]byte(cached), &tools); err == nil {
				return tools
			}
		}
	}

	tools := h.registry.CatalogForUser(ctx, h.perms, userID, tenantID)

	// Cache the result.
	if h.cache != nil && len(tools) > 0 {
		if data, err := json.Marshal(tools); err == nil {
			if err := h.cache.Set(ctx, cacheKey, string(data), int(agentCatalogCacheTTL.Seconds())); err != nil {
				slog.Warn("ai: failed to cache tool catalog", "error", err)
			}
		}
	}

	return tools
}

// createSession inserts an ai_agent_sessions row with status "running" and
// the pre-search level. Returns the created session.
func (h *AgentHandler) createSession(ctx context.Context, userID, tenantID pgtype.UUID, prompt string, geoSpaceID *pgtype.UUID, level PreSearchLevel) (gen.AiAgentSession, error) {
	if h.sessions == nil {
		// No session store configured — return a zero-value session so
		// the loop can proceed (useful for tests).
		return gen.AiAgentSession{ID: randomUUID(), UserID: userID, Prompt: prompt, PresearchLevel: string(level), Status: "running"}, nil
	}

	var geoID pgtype.UUID
	if geoSpaceID != nil {
		geoID = *geoSpaceID
	}

	return h.sessions.CreateAgentSession(ctx, gen.CreateAgentSessionParams{

		UserID:         userID,
		Prompt:         prompt,
		GeoSpaceID:     geoID,
		PresearchLevel: string(level),
		Status:         "running",
	})
}

// completeSession marks the session as completed with the final response,
// tool call count, token usage, and estimated cost.
func (h *AgentHandler) completeSession(ctx context.Context, session gen.AiAgentSession, summary string, toolCallCount, totalTokens int, cost float64) {
	if h.sessions == nil {
		return
	}

	summaryPtr := &summary

	_, err := h.sessions.CompleteAgentSession(context.WithoutCancel(ctx), gen.CompleteAgentSessionParams{
		ResultSummary: summaryPtr,
		ToolCallCount: int32(toolCallCount), // #nosec G115 -- bounded by single session tool calls
		TotalTokens:   int32(totalTokens),   // #nosec G115 -- bounded by single session token count
		EstimatedCost: floatToNumeric(cost),
		ID:            session.ID,
	})
	if err != nil {
		slog.Warn("ai: failed to complete agent session", "session_id", session.ID, "error", err)
	}
}

// failSession marks the session as failed with the error message.
func (h *AgentHandler) failSession(ctx context.Context, session gen.AiAgentSession, loopErr error, toolCallCount, totalTokens int, cost float64) {
	if h.sessions == nil {
		return
	}

	errMsg := loopErr.Error()

	_, err := h.sessions.FailAgentSession(context.WithoutCancel(ctx), gen.FailAgentSessionParams{
		ErrorMessage:  &errMsg,
		ToolCallCount: int32(toolCallCount), // #nosec G115 -- bounded by single session tool calls
		TotalTokens:   int32(totalTokens),   // #nosec G115 -- bounded by single session token count
		EstimatedCost: floatToNumeric(cost),
		ID:            session.ID,
	})
	if err != nil {
		slog.Warn("ai: failed to fail agent session", "session_id", session.ID, "error", err)
	}
}

// cancelSession marks the session as cancelled. Distinct from failSession
// because cancellation is a user action, not a system error — the
// ai_agent_sessions.status column is set to 'cancelled' (not 'failed') so
// admin dashboards can distinguish the two.
func (h *AgentHandler) cancelSession(ctx context.Context, session gen.AiAgentSession) {
	if h.sessions == nil {
		return
	}

	_, err := h.sessions.CancelAgentSession(context.WithoutCancel(ctx), session.ID)
	if err != nil {
		slog.Warn("ai: failed to cancel agent session", "session_id", session.ID, "error", err)
	}
}

// setSessionState writes the session state to Valkey for cancellation
// checks. The key jc:agent:session:{tenant_id}:{session_id} is set to
// "running" with a 5-minute TTL. The cancel endpoint (Phase 6) sets it to
// "cancelled". Tenant-scoping prevents cross-tenant session cancellation
// (Rule 4 defense-in-depth, complementing the DB ownership check).
func (h *AgentHandler) setSessionState(ctx context.Context, tenantID, sessionID pgtype.UUID) {
	if h.cache == nil {
		return
	}

	key := valkey.MustKey("agent:session:%s", sessionID.String())
	if err := h.cache.Set(ctx, key, "running", int(agentSessionStateTTL.Seconds())); err != nil {
		slog.Warn("ai: failed to set session state", "error", err)
	}
}

// isCancelled checks whether the session has been cancelled via the
// jc:agent:session:{tenant_id}:{session_id} Valkey key.
func (h *AgentHandler) isCancelled(ctx context.Context, tenantID, sessionID pgtype.UUID) bool {
	if h.cache == nil {
		return false
	}

	key := valkey.MustKey("agent:session:%s", sessionID.String())

	val, err := h.cache.Get(ctx, key)
	if err != nil {
		return false
	}

	return val == "cancelled"
}

// ---------------------------------------------------------------------------
// System prompt (task 5.07)
// ---------------------------------------------------------------------------

// buildSystemPrompt constructs the LLM system prompt with the §103 behavior
// rules, the user's geo context, the pre-search results, and the available
// tool list.
func buildSystemPrompt(geoSummary string, preSearch *PreSearchResult, tools []ToolConfig) string {
	var b strings.Builder

	b.WriteString(systemPromptPreamble)

	if geoSummary != "" {
		fmt.Fprintf(&b, "\n\n%s", geoSummary)
	}

	if preSearch != nil {
		fmt.Fprintf(&b, "\n\nPRE-SEARCH RESULTS:\n%s", preSearch.Summary())
		fmt.Fprintf(&b, "\n\nPre-search data (JSON):\n%s", preSearchJSON(preSearch))
	}

	if len(tools) > 0 {
		b.WriteString("\n\nAVAILABLE TOOLS:\n")

		for _, t := range tools {
			fmt.Fprintf(&b, "- %s: %s\n", t.FunctionName, t.Description)
		}
	}

	return b.String()
}

// buildDraftRequestPromptSection appends the Phase 8 Content Request Flow
// instructions to the system prompt when the draft-request path is active
// (pre-search = NONE + user has agent.draft_request + persona resolved).
// The LLM is instructed to use external.search + content.create_draft with
// the resolved persona user_id as the author_id, and to never use
// content.create (which the user lacks permission for anyway, but the
// explicit instruction prevents the LLM from attempting it).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, task 8.01).
func buildDraftRequestPromptSection(d draftRequestContext) string {
	return fmt.Sprintf(`

DRAFT-REQUEST MODE (Phase 8 Content Request Flow):
Pre-search found no existing content covering this request, and the user does
not have content.create permission. They do have agent.draft_request, which
means you may create a draft for admin review.

When the request is suitable for content creation:
1. Call content_types.list to discover the valid content_type and its field_group_entity_class. If the type has categories (e.g. guide or comparison), call the category_list_tool (guide_categories.list or comparison.types.list) to get the specific category/type and field group.
2. Call custom_field_groups.list with the entity_class and then custom_fields.list with the group_id to discover the structured fields.
3. Call external.search to gather relevant external information.
4. Summarize the external results for the user in your response.
5. If the draft is high-stakes (visa, tax, banking, housing, cost_of_living,
   relocation, health_insurance, grants, admission), call external.search AGAIN
   with specific queries for the current facts (fees, rates, rent ranges,
   requirements, processing times) before writing the body. Cite the most
   relevant source URL(s).
6. Call content.create_draft with:
   - author_id: "%s"  (the AI persona's backing user — do NOT use the calling user)
   - ai_persona_id: "%s"
   - content_type and category from the discovery step above
   - original_prompt: the user's original request text
   - field_values: free-form JSONB only (page builder blocks, _ai_provenance); structured custom field values go through custom_field_values.bulk_set after creation
   - status is forced to "draft" by the tool; do not attempt to publish.
4. Tell the user the draft has been submitted for admin review and will be
   published after a human reviewer approves it.

Do NOT call content.create — the user lacks permission and the call will be
rejected. Use content.create_draft exclusively for the draft-request path.
`, d.PersonaUserID, d.PersonaID)
}

// systemPromptPreamble is the §103 behavior rules block, verbatim from the
// spec. The LLM receives these instructions in every agent request.
const systemPromptPreamble = `You are Japa AI, the intelligence layer of JapaConnect.

BEHAVIOR RULES:
1. You have been given pre-search results. Use them.

2. If FULL results exist:
   - Present the existing content to the user with a summary and link
   - Do NOT create duplicate content
   - Offer to update or expand if the user wants more detail

3. If PARTIAL results exist:
   - Tell the user what already exists
   - Identify what's missing
   - If you can fill the gaps (via tools + RAG + external search), do so
   - Present the updated content

4. If NONE exist:
   - If you can create the content (authorized + sufficient knowledge):
     create it, then present it
   - If you need more information from the user: ask targeted questions
   - If you need external knowledge: search externally, summarize,
     and offer to create content from the findings
   - If you cannot create it (unauthorized or impossible): explain why
     and suggest what the user can do

5. NEVER create duplicate content. Always check pre-search results first.

6. For comparisons specifically:
   - Check if a comparison of these subjects already exists
   - If it exists but is outdated, offer to update it
   - If it exists and is current, present it
   - If it doesn't exist, determine the tier (structured/semi/unstructured)
     and execute the appropriate comparison strategy
   - When filling comparison values from external.search, set source_ref to the
     most relevant URL from the result's citations array.

7. For guide and comparison creation that depends on current facts (visa fees,
   rent ranges, processing times, tax rates, bank requirements, embassy rules):
   - Call external.search first and, when citations are returned, cite the most
     relevant source URL in the body or set source_ref on comparison values.
   - Do not fabricate current figures. If no external source is available, say so
     or ask the user for the current value.

8. You are operating as the user who prompted you. You can only do what
   they can do. If a tool returns "unauthorized", do not retry — inform
   the user.

9. Respect privacy. If a tool returns "privacy_blocked", do not attempt
   to bypass — inform the user that the data is private.

10. For ANY content creation request:
    - Call content_types.list first to discover the valid content_type, whether it has categories, the category discovery tool, and the field_group_entity_class to use. Only content types the user is permitted to create will appear in the list (filtered by agent.content_type.* grants).
    - If content_type has categories (e.g. guide), call the category_list_tool (e.g. guide_categories.list or comparison.types.list) to get the specific field_group_id or entity_class.
    - Call custom_field_groups.list with the field_group_entity_class to find the field group ID and slug. Then call custom_fields.list with the group_id to discover required/optional field keys and types.
    - The field_values parameter in content.create/content.create_draft is only for free-form contents.field_values JSONB (e.g. page builder blocks, _ai_provenance). It is NOT for structured custom field values. After creating the content, use custom_field_values.bulk_set with the new content_id to set the content type's structured custom fields.
    - For numeric custom fields (e.g. salary_min, salary_max), pass the value as value_number in custom_field_values.set and custom_field_values.bulk_set. Pass booleans as value_bool, dates as value_date, and text/select/textarea values as value_text.
    - High-stakes content (visa, banking, grants, health_insurance, admission, and relocation/finance/cost-of-living advice) requires current facts. Call external.search and cite the most relevant source URL. Do not fabricate current fees, processing times, rent ranges, or exchange rates.

11. When the user asks to create or publish MULTIPLE content items at once:
    - Use content.bulk_create to create many similar content rows in a single tool call. Each item can set its own title, content_type, author_id, organization_id, publish_at, guide_category, and field_values.
    - Use content.bulk_publish to transition many content rows to published (or another status) in a single tool call.
    - Both bulk tools are HITL-queued and return per-item success/failure, so partial success is fine. Report the created and failed counts to the user.

12. Author assignment rules:
    - For content.create and content.bulk_create, the default author is the calling user. To set author_id to another user or to an AI persona's backing user, the caller must have content.assign_author.
    - For content.create_draft, author_id should be the AI persona's backing user resolved by the agent. Do not override it with an arbitrary user.
`

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

// configsToToolDefinitions converts the internal ToolConfig slice to the
// LLMToolDefinition slice the gateway expects.
func configsToToolDefinitions(configs []ToolConfig) []LLMToolDefinition {
	out := make([]LLMToolDefinition, 0, len(configs))
	for _, c := range configs {
		out = append(out, LLMToolDefinition{
			Name:        c.FunctionName,
			Description: c.Description,
			Parameters:  c.Parameters,
		})
	}

	return out
}

// floatToNumeric converts a float64 to a pgtype.Numeric suitable for the
// ai_agent_sessions.estimated_cost column.
func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%.4f", f)); err != nil {
		// Fallback: zero numeric with Valid=true so we never send SQL NULL
		// to a NOT NULL column.
		return pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	}

	return n
}

// randomUUID generates a new UUID. Used only as a fallback when no session
// store is configured (tests). In production the session store generates
// the UUID via PostgreSQL's gen_random_uuid().
func randomUUID() pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(uuid.NewString())

	return u
}
