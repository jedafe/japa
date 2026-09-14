// Package ai — agent_handlers.go implements the HTTP handlers for the AI
// Agent Executor API surface (Section 103, ADR-111, Phase 6).
// // Routes wired by internal/server:
// //	POST /api/v1/ai/agent — agent request (SSE stream)
// GET /api/v1/ai/agent/capabilities — tool catalog for current user
// GET /api/v1/ai/agent/sessions — user session history (paginated)
// GET /api/v1/ai/agent/sessions/{id} — session detail + tool call log
// POST /api/v1/ai/agent/sessions/{id}/cancel — cancel a running session
// POST /api/v1/content/{id}/reassign-author — reassign content authorship
// GET /api/v1/me/ai-privacy — view own AI privacy exemptions
// PUT /api/v1/me/ai-privacy — set AI privacy exemption on own data
// GET /api/v1/admin/agent/sessions — admin: all sessions (paginated)
// GET /api/v1/admin/agent/sessions/{id} — admin: session detail + audit trail
// GET /api/v1/admin/agent/tool-stats — admin: tool usage statistics
// GET /api/v1/admin/agent/cost-report — admin: cost breakdown by user
// // SSE streaming is new to the codebase (no existing pattern). The agent
// endpoint uses standard Go http.Flusher with Content-Type text/event-stream
// per §103 "SSE Streaming". Each AgentEvent from the agent loop is marshaled
// to JSON and written as a `data: {json}\n\n` line, flushed immediately.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (API Routes, SSE Streaming)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6, tasks 6.01-6.10)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
)

// ---------------------------------------------------------------------------
// AgentHTTPHandlers
// ---------------------------------------------------------------------------

// AgentSessionQueries is the subset of gen.Queries the agent HTTP handlers
// use for session history, detail, cancellation, admin aggregates, and
// privacy-exemption writes. *gen.Queries satisfies this.
type AgentSessionQueries interface {
	// User-facing session history + detail.
	GetAgentSessionForUser(ctx context.Context, arg gen.GetAgentSessionForUserParams) (gen.AiAgentSession, error)
	ListAgentSessionsByUser(ctx context.Context, arg gen.ListAgentSessionsByUserParams) ([]gen.AiAgentSession, error)
	CountAgentSessionsByUser(ctx context.Context, userID pgtype.UUID) (int64, error)
	ListAgentToolCallsBySession(ctx context.Context, sessionID pgtype.UUID) ([]gen.AiAgentToolCall, error)
	CancelAgentSession(ctx context.Context, id pgtype.UUID) (gen.AiAgentSession, error)

	// Admin session listing + detail.
	GetAgentSession(ctx context.Context, id pgtype.UUID) (gen.AiAgentSession, error)
	ListAgentSessionsForAdmin(ctx context.Context, arg gen.ListAgentSessionsForAdminParams) ([]gen.AiAgentSession, error)
	CountAgentSessionsForAdmin(ctx context.Context, statusFilter string) (int64, error)

	// Admin aggregates.
	GetAgentToolStats(ctx context.Context, arg gen.GetAgentToolStatsParams) ([]gen.GetAgentToolStatsRow, error)
	GetAgentCostReport(ctx context.Context, arg gen.GetAgentCostReportParams) ([]gen.GetAgentCostReportRow, error)

	// Privacy exemption reads (GET /me/ai-privacy).
	GetContentAIExempt(ctx context.Context, id pgtype.UUID) (bool, error)
	GetCustomFieldValueAIExempt(ctx context.Context, id pgtype.UUID) (bool, error)
	GetCustomFieldValueEntity(ctx context.Context, id pgtype.UUID) (gen.GetCustomFieldValueEntityRow, error)
	GetUserAIExempt(ctx context.Context, id pgtype.UUID) (bool, error)

	// Privacy exemption writes (PUT /me/ai-privacy).
	SetContentAIExempt(ctx context.Context, arg gen.SetContentAIExemptParams) (gen.SetContentAIExemptRow, error)
	SetCustomFieldValueAIExempt(ctx context.Context, arg gen.SetCustomFieldValueAIExemptParams) (gen.SetCustomFieldValueAIExemptRow, error)
	SetUserAIExempt(ctx context.Context, arg gen.SetUserAIExemptParams) (gen.SetUserAIExemptRow, error)
}

// ContentAuthorQueries is the subset of gen.Queries the reassign-author
// handler uses: read the current content row (for field_values + ownership
// check), verify the target author exists in the tenant, and update
// author_id + field_values. *gen.Queries satisfies this.
type ContentAuthorQueries interface {
	GetContentByID(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error)
	ReassignSingleContentAuthor(ctx context.Context, arg gen.ReassignSingleContentAuthorParams) (gen.ReassignSingleContentAuthorRow, error)
	GetUserByID(ctx context.Context, userID pgtype.UUID) (gen.GetUserByIDRow, error)
}

// AgentHTTPHandlers holds dependencies for the Phase 6 agent HTTP handlers.
// The AgentHandler (Phase 5) runs the agent loop; these handlers wrap it
// with HTTP concerns (auth context, SSE streaming, JSON responses, RBAC
// already enforced by router middleware).
type AgentHTTPHandlers struct {
	agent     *AgentHandler
	registry  *ToolHandlerRegistry
	perms     PermissionChecker
	sessions  AgentSessionQueries
	content   ContentAuthorQueries
	cache     CacheClient // for jc:agent:session:{id} cancel flag
	engine    *ComparisonEngine
	preSearch *PreSearcher
}

// AgentHTTPOption configures an AgentHTTPHandlers.
type AgentHTTPOption func(*AgentHTTPHandlers)

// NewAgentHTTPHandlers creates the Phase 6 HTTP handler set. The agent
// handler and session queries are required; the registry, perms, content
// store, and cache are optional but should be provided by the wiring layer.
func NewAgentHTTPHandlers(agent *AgentHandler, sessions AgentSessionQueries, opts ...AgentHTTPOption) *AgentHTTPHandlers {
	h := &AgentHTTPHandlers{agent: agent, sessions: sessions}

	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}

	return h
}

// WithAgentRegistry injects the tool handler registry (for capabilities).
func WithAgentRegistry(r *ToolHandlerRegistry) AgentHTTPOption {
	return func(h *AgentHTTPHandlers) { h.registry = r }
}

// WithAgentHTTPPerms injects the RBAC permission checker (for capabilities).
func WithAgentHTTPPerms(p PermissionChecker) AgentHTTPOption {
	return func(h *AgentHTTPHandlers) { h.perms = p }
}

// WithAgentContentStore injects the content queries (for reassign-author).
func WithAgentContentStore(c ContentAuthorQueries) AgentHTTPOption {
	return func(h *AgentHTTPHandlers) { h.content = c }
}

// WithAgentHTTPCache injects the Valkey cache (for session cancel flag).
func WithAgentHTTPCache(c CacheClient) AgentHTTPOption {
	return func(h *AgentHTTPHandlers) { h.cache = c }
}

// WithAgentComparisonEngine injects the ComparisonEngine (Phase 7) for the
// direct /api/v1/ai/compare-entities endpoint.
func WithAgentComparisonEngine(e *ComparisonEngine) AgentHTTPOption {
	return func(h *AgentHTTPHandlers) { h.engine = e }
}

// WithAgentPreSearcher injects the PreSearcher (Phase 5) for the direct
// /api/v1/ai/compare-entities endpoint's knowledge-gathering step.
func WithAgentPreSearcher(p *PreSearcher) AgentHTTPOption {
	return func(h *AgentHTTPHandlers) { h.preSearch = p }
}

// ---------------------------------------------------------------------------
// Task 6.01 — POST /api/v1/ai/agent (SSE streaming)
// ---------------------------------------------------------------------------

// HandleAgent is the SSE streaming endpoint for agent requests. It reads
// the prompt from the request body, calls AgentHandler.HandleAgentRequest,
// and streams AgentEvents back as Server-Sent Events.
// // The request body accepts { prompt: string, conversation_id?: uuid } per
// §103. The conversation_id field is accepted for forward compatibility
// with multi-turn conversations but is not yet used by the agent loop
// (Phase 7+ will add conversation context persistence).
// // SSE wire format per §103 "SSE Streaming":
// //	Content-Type: text/event-stream
// Cache-Control: no-cache
// Connection: keep-alive
// // Each event is written as `data: {json}\n\n` and flushed immediately so the
// frontend receives progress incrementally. Pre-check failures (agent
// disabled, no permission, rate limited) return a standard JSON error
// response with the appropriate status code before the stream opens.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (SSE Streaming, API Routes).
func (h *AgentHTTPHandlers) HandleAgent(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Prompt         string `json:"prompt"`
		ConversationID string `json:"conversation_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Prompt == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "prompt is required")
		return
	}

	// HandleAgentRequest runs synchronous pre-checks (enabled, permission,
	// rate limit, pre-search, session creation) and returns a channel. Pre-
	// check failures come back as errors here — return them as JSON before
	// opening the SSE stream so the client gets a proper status code.
	events, err := h.agent.HandleAgentRequest(r.Context(), user.UserID, tenantID, input.Prompt)
	if err != nil {
		writeAgentError(w, err)
		return
	}

	// Set SSE headers and get the flusher. Per §103, the codebase has no
	// existing SSE endpoints — this is the reference implementation.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable Nginx buffering
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	if flusher == nil {
		// No flusher — the server doesn't support streaming. Log and fall
		// back to writing all events at once (the client still gets them,
		// just not incrementally).
		slog.Warn("ai: response writer does not support flushing, SSE will not stream incrementally")
	}

	// Drain the event channel, writing each event as an SSE line. The agent
	// loop closes the channel when it finishes (completion, failure, or
	// cancellation), so this range exits naturally.
	for ev := range events {
		data, marshalErr := json.Marshal(ev)
		if marshalErr != nil {
			slog.Warn("ai: failed to marshal agent event", "type", ev.Type, "error", marshalErr)
			continue
		}

		if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", data); writeErr != nil {
			// Client disconnected — stop draining. The agent loop detects
			// ctx cancellation and stops on its own.
			return
		}

		if flusher != nil {
			flusher.Flush()
		}
	}
}

// writeAgentError maps an agent pre-check error to the appropriate HTTP
// status code and writes a standard error response (Rule 5).
func writeAgentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAgentDisabled):
		errpkg.WriteError(w, http.StatusForbidden, "agent_disabled", "The AI agent is currently disabled")
	case errors.Is(err, errAgentNoPermission):
		errpkg.WriteError(w, http.StatusForbidden, "agent_permission_required", "You do not have permission to use the AI agent")
	case errors.Is(err, errAgentRateLimited):
		errpkg.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Daily agent request limit exceeded")
	case errors.Is(err, errAgentBudgetExceeded):
		errpkg.WriteError(w, http.StatusPaymentRequired, "budget_exceeded", "Daily AI spend budget exceeded")
	case errors.Is(err, errAgentEmptyPrompt):
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "prompt is required")
	case errors.Is(err, errAgentNotConfigured):
		errpkg.WriteError(w, http.StatusServiceUnavailable, "agent_not_configured", "The AI agent is not configured")
	default:
		slog.Warn("ai: agent request failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "agent_error", "The agent request could not be started")
	}
}

// Sentinel errors for agent pre-check failures. These are set on the
// AgentHandler's error returns via the wrapAgentError helper below so the
// HTTP layer can map them to status codes without string matching.
var (
	errAgentDisabled       = errors.New("ai: agent is disabled")
	errAgentNoPermission   = errors.New("ai: user lacks agent.use permission")
	errAgentRateLimited    = errors.New("ai: daily agent rate limit exceeded")
	errAgentBudgetExceeded = errors.New("ai: daily agent budget exceeded")
	errAgentEmptyPrompt    = errors.New("ai: prompt is required")
	errAgentNotConfigured  = errors.New("ai: agent handler not configured")
)

// ---------------------------------------------------------------------------
// Task 6.02 — GET /api/v1/ai/agent/capabilities
// ---------------------------------------------------------------------------

// HandleGetCapabilities returns the tool catalog available to the current
// user, filtered by RBAC, plus the user's geo context. Per §103 API Routes:
// "Returns: { tools: [...], geo_context: {...} }".
func (h *AgentHTTPHandlers) HandleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	// Build the catalog filtered by the user's RBAC grants. Reuses the
	// agent's buildToolCatalog which reads from the jc:agent:tools:{user_id}
	// cache (10-min TTL) when a cache is configured.
	tools := h.agent.buildToolCatalog(r.Context(), user.UserID)

	// Resolve geo context for the response. The agent handler's
	// resolveGeoContext returns (geoSpaceID, summary); we surface the
	// summary as geo_context.
	_, geoSummary := h.agent.resolveGeoContext(r.Context(), user.UserID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"tools":       tools,
			"geo_context": geoSummary,
		},
	})
}

// ---------------------------------------------------------------------------
// Task 6.03 — GET /api/v1/ai/agent/sessions (paginated history)
// ---------------------------------------------------------------------------

// HandleListSessions returns the current user's agent session history,
// offset-paginated per the API response format convention
// (?page_size=N&page_offset=N).
func (h *AgentHTTPHandlers) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	sessions, err := h.sessions.ListAgentSessionsByUser(r.Context(), gen.ListAgentSessionsByUserParams{

		UserID:     user.UserID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		slog.Warn("ai: listing agent sessions", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list agent sessions")

		return
	}

	total, err := h.sessions.CountAgentSessionsByUser(r.Context(), user.UserID)
	if err != nil {
		slog.Warn("ai: counting agent sessions", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count agent sessions")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": sessionsToJSON(sessions),
		"meta": map[string]any{
			"total":       total,
			"page_size":   limit,
			"page_offset": offset,
		},
	})
}

// ---------------------------------------------------------------------------
// Task 6.04 — GET /api/v1/ai/agent/sessions/{id} (detail + tool calls)
// ---------------------------------------------------------------------------

// HandleGetSession returns a single agent session's detail plus its tool
// call audit log. Ownership is enforced at the DB layer via
// GetAgentSessionForUser ( + user_id + id) so a user cannot read
// another user's session.
func (h *AgentHTTPHandlers) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	sessionID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}

	session, err := h.sessions.GetAgentSessionForUser(r.Context(), gen.GetAgentSessionForUserParams{
		ID: sessionID,

		UserID: user.UserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "session_not_found", "Agent session not found")
			return
		}

		slog.Warn("ai: getting agent session", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve agent session")

		return
	}

	toolCalls, err := h.sessions.ListAgentToolCallsBySession(r.Context(), sessionID)
	if err != nil {
		slog.Warn("ai: listing agent tool calls", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve tool call log")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"session":    sessionToJSON(session),
			"tool_calls": toolCallsToJSON(toolCalls),
		},
	})
}

// ---------------------------------------------------------------------------
// Task 6.05 — POST /api/v1/ai/agent/sessions/{id}/cancel
// ---------------------------------------------------------------------------

// HandleCancelSession cancels a running agent session. It sets the
// jc:agent:session:{id} Valkey key to "cancelled" (the agent loop checks
// this key each iteration and breaks) and marks the session row as
// 'cancelled' via CancelAgentSession. Ownership is enforced at the DB
// layer via GetAgentSessionForUser.
func (h *AgentHTTPHandlers) HandleCancelSession(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	sessionID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}

	// Verify ownership before cancelling. A user can only cancel their own
	// running session.
	session, err := h.sessions.GetAgentSessionForUser(r.Context(), gen.GetAgentSessionForUserParams{
		ID: sessionID,

		UserID: user.UserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "session_not_found", "Agent session not found")
			return
		}

		slog.Warn("ai: getting agent session for cancel", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve agent session")

		return
	}

	if session.Status != "running" {
		errpkg.WriteError(w, http.StatusConflict, "session_not_running", "Session is not running and cannot be cancelled")
		return
	}

	// Set the Valkey cancel flag so the running loop breaks on its next
	// iteration check. The key already exists (set to "running" by the
	// agent loop); overwrite it with "cancelled".
	if h.cache != nil {
		key := valkey.MustKey("agent:session:%s", sessionID.String())
		if err := h.cache.Set(r.Context(), key, "cancelled", int(agentSessionStateTTL.Seconds())); err != nil {
			slog.Warn("ai: failed to set cancel flag", "error", err)
			// Non-fatal — the DB update below still marks the session.
		}
	}

	// Mark the session row as cancelled. CancelAgentSession only updates
	// rows with status = 'running' (race-safe: if the loop completes
	// between the ownership check and here, the row is already terminal).
	_, cancelErr := h.sessions.CancelAgentSession(r.Context(), sessionID)
	if cancelErr != nil {
		if errors.Is(cancelErr, pgx.ErrNoRows) {
			// The session completed between the ownership check and the
			// cancel — it is no longer running. The Valkey flag was set
			// but the loop already exited. Report the actual state.
			errpkg.WriteError(w, http.StatusConflict, "session_not_running",
				"Session is not running and cannot be cancelled")
			return
		}

		slog.Warn("ai: cancelling agent session row", "error", cancelErr)
		// The Valkey flag is set; the loop will still break. Report success
		// since the cancellation signal was delivered.
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"status": "cancelled"},
	})
}

// ---------------------------------------------------------------------------
// Task 6.06 — POST /api/v1/content/{id}/reassign-author
// ---------------------------------------------------------------------------

// HandleReassignAuthor reassigns a content row's author_id and appends
// authorship provenance to field_values._ai_provenance. Requires the
// content.assign_author permission (enforced by router middleware).
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Authorship Assignment).
func (h *AgentHTTPHandlers) HandleReassignAuthor(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	contentID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}

	var input struct {
		NewAuthorID string `json:"new_author_id"`
		Reason      string `json:"reason,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.NewAuthorID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "new_author_id is required")
		return
	}

	newAuthorID, err := scanUUID(input.NewAuthorID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "new_author_id must be a valid UUID")
		return
	}

	if h.content == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "content_store_unavailable", "Content store not configured")
		return
	}

	// Verify the target author is a valid user in the same tenant (spec:
	// "Verify tenant ownership and target author validity"). This prevents
	// orphaned author_id references and cross-tenant assignments.
	if _, err := h.content.GetUserByID(r.Context(),

		newAuthorID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_author", "Target author not found in this tenant")
			return
		}

		slog.Warn("ai: verifying target author", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify target author")

		return
	}

	// Read the current content row to get field_values for the provenance
	// merge. GetContentByID is tenant-scoped (defense-in-depth, BUG-014).
	current, err := h.content.GetContentByID(r.Context(), contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "content_not_found", "Content not found")
			return
		}

		slog.Warn("ai: reading content for reassign", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read content")

		return
	}

	// Merge _ai_provenance into field_values. field_values is a JSONB
	// column ([]byte). We unmarshal, update the _ai_provenance sub-object,
	// and re-marshal.
	updatedFieldValues, err := mergeAuthorProvenance(current.FieldValues, user.UserID, input.Reason)
	if err != nil {
		slog.Warn("ai: merging author provenance", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update authorship provenance")

		return
	}

	updated, err := h.content.ReassignSingleContentAuthor(r.Context(), gen.ReassignSingleContentAuthorParams{
		AuthorID:    newAuthorID,
		FieldValues: updatedFieldValues,
		ID:          contentID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "content_not_found", "Content not found")
			return
		}

		slog.Warn("ai: reassigning content author", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to reassign author")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"content_id":    updated.ID,
			"author_id":     updated.AuthorID,
			"reassigned_by": user.UserID,
		},
	})
}

// mergeAuthorProvenance reads the current field_values JSONB, updates the
// _ai_provenance sub-object with author_assigned_by and
// author_assignment_reason, and returns the updated JSONB bytes. If
// field_values is empty/null, a new object is created.
func mergeAuthorProvenance(fieldValues []byte, assignedBy pgtype.UUID, reason string) ([]byte, error) {
	var fv map[string]any
	if len(fieldValues) > 0 {
		if err := json.Unmarshal(fieldValues, &fv); err != nil {
			return nil, fmt.Errorf("unmarshaling field_values: %w", err)
		}
	}

	if fv == nil {
		fv = map[string]any{}
	}

	prov, _ := fv["_ai_provenance"].(map[string]any)
	if prov == nil {
		prov = map[string]any{}
	}

	prov["author_assigned_by"] = assignedBy.String()
	if reason != "" {
		prov["author_assignment_reason"] = reason
	}

	prov["author_assigned_at"] = time.Now().UTC().Format(time.RFC3339)
	fv["_ai_provenance"] = prov

	out, err := json.Marshal(fv)
	if err != nil {
		return nil, fmt.Errorf("marshaling updated field_values: %w", err)
	}

	return out, nil
}

// cfvEntityOwnedByUser checks whether the calling user owns the entity a
// custom_field_value is attached to. Returns true for:
// - entity_type "content" → contents.author_id == userID (requires content store)
// - entity_type "user" or "user_profile" → entity_id == userID
// - other entity types → false (conservative deny)
// // The content store is optional; if it's nil and the entity is content, the
// function returns false (deny by default when ownership can't be verified).
func cfvEntityOwnedByUser(entity gen.GetCustomFieldValueEntityRow, userID pgtype.UUID, content ContentAuthorQueries, ctx context.Context) bool {
	switch entity.EntityType {
	case "user", "user_profile":
		return entity.EntityID == userID
	case "content":
		if content == nil {
			return false
		}

		row, err := content.GetContentByID(ctx, entity.EntityID)
		if err != nil {
			return false
		}

		return row.AuthorID == userID
	default:
		// Conservative deny for entity types we don't know how to verify
		// ownership for. This prevents a user from setting ai_exempt on
		// field values attached to entities they don't own (e.g. spaces,
		// organizations) without an explicit ownership check.
		return false
	}
}

// ---------------------------------------------------------------------------
// Task 6.07 — GET/PUT /api/v1/me/ai-privacy
// ---------------------------------------------------------------------------

// HandleGetAIPrivacy returns the current user's AI privacy exemption status.
// Per §103, users can set ai_exempt on their own data (profile, content,
// custom_field_value).
// // The current implementation returns the profile-level exemption flag
// (users.ai_exempt). Listing all exempt content/field values for a user
// would require additional list queries (e.g. "ListExemptContentByAuthor")
// which are not part of Phase 6. The frontend can determine per-entity
// exemption status by checking the ai_exempt field on individual content
// rows and field values via existing content/CFV endpoints.
func (h *AgentHTTPHandlers) HandleGetAIPrivacy(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	// Profile-level exemption (users.ai_exempt).
	profileExempt, err := h.sessions.GetUserAIExempt(r.Context(), user.UserID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("ai: reading user ai_exempt", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read privacy status")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"profile_ai_exempt": profileExempt,
		},
	})
}

// HandleSetAIPrivacy sets the ai_exempt flag on the user's own data. Per
// §103, a user may only set the flag on their OWN data:
// - entity_type "profile" → users.ai_exempt (entity_id must be own user ID)
// - entity_type "content" → contents.ai_exempt (entity_id must be own content)
// - entity_type "custom_field_value" → custom_field_values.ai_exempt
// // Ownership for "content" and "custom_field_value" is verified by checking
// that the content's author_id matches the user (for content) — the SQL
// queries are tenant-scoped for defense-in-depth.
func (h *AgentHTTPHandlers) HandleSetAIPrivacy(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		AIExempt   bool   `json:"ai_exempt"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.EntityID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "entity_id is required")
		return
	}

	entityID, err := scanUUID(input.EntityID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "entity_id must be a valid UUID")
		return
	}

	switch input.EntityType {
	case "profile":
		// A user may only set the profile exemption on their own user row.
		if entityID != user.UserID {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only set AI privacy on your own profile")
			return
		}

		row, err := h.sessions.SetUserAIExempt(r.Context(), gen.SetUserAIExemptParams{
			AiExempt: input.AIExempt,
			ID:       entityID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
				return
			}

			slog.Warn("ai: setting user ai_exempt", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to set privacy exemption")

			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"entity_type": "profile",
			"entity_id":   row.ID,
			"ai_exempt":   row.AiExempt,
		}})

	case "content":
		// Verify ownership: the content's author_id must match the user.
		if h.content == nil {
			errpkg.WriteError(w, http.StatusServiceUnavailable, "content_store_unavailable", "Content store not configured")
			return
		}

		current, err := h.content.GetContentByID(r.Context(), entityID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "content_not_found", "Content not found")
				return
			}

			slog.Warn("ai: reading content for privacy", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to read content")

			return
		}

		if current.AuthorID != user.UserID {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only set AI privacy on your own content")
			return
		}

		row, err := h.sessions.SetContentAIExempt(r.Context(), gen.SetContentAIExemptParams{
			AiExempt: input.AIExempt,
			ID:       entityID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "content_not_found", "Content not found")
				return
			}

			slog.Warn("ai: setting content ai_exempt", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to set privacy exemption")

			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"entity_type": "content",
			"entity_id":   row.ID,
			"ai_exempt":   row.AiExempt,
		}})

	case "custom_field_value":
		// custom_field_values has no author_id column. We verify ownership
		// transitively: look up the CFV's entity_type and entity_id, then
		// check that the calling user owns that entity.
		// - entity_type "content" → contents.author_id must match user
		// - entity_type "user"/"user_profile" → entity_id must match user
		// - other entity types → deny (conservative default)
		// The SetCustomFieldValueAIExempt query is tenant-scoped via the
		// custom_fields JOIN for defense-in-depth (BUG-014).
		entity, err := h.sessions.GetCustomFieldValueEntity(r.Context(), entityID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "field_value_not_found", "Custom field value not found")
				return
			}

			slog.Warn("ai: reading cfv entity for ownership check", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify ownership")

			return
		}

		if !cfvEntityOwnedByUser(entity, user.UserID, h.content, r.Context()) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner",
				"You can only set AI privacy on your own data")
			return
		}

		row, err := h.sessions.SetCustomFieldValueAIExempt(r.Context(), gen.SetCustomFieldValueAIExemptParams{
			AiExempt: input.AIExempt,
			ID:       entityID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "field_value_not_found", "Custom field value not found")
				return
			}

			slog.Warn("ai: setting cfv ai_exempt", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to set privacy exemption")

			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"entity_type": "custom_field_value",
			"entity_id":   row.ID,
			"ai_exempt":   row.AiExempt,
		}})

	default:
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error",
			"entity_type must be 'profile', 'content', or 'custom_field_value'")
	}
}

// ---------------------------------------------------------------------------
// Task 6.08 — Admin routes
// ---------------------------------------------------------------------------

// HandleAdminListSessions returns all agent sessions for the tenant,
// paginated, with an optional status filter (?status=running|completed|...).
func (h *AgentHTTPHandlers) HandleAdminListSessions(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	statusFilter := r.URL.Query().Get("status")

	sessions, err := h.sessions.ListAgentSessionsForAdmin(r.Context(), gen.ListAgentSessionsForAdminParams{

		StatusFilter: statusFilter,
		PageOffset:   offset,
		PageLimit:    limit,
	})
	if err != nil {
		slog.Warn("ai: admin listing agent sessions", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list agent sessions")

		return
	}

	total, err := h.sessions.CountAgentSessionsForAdmin(r.Context(), statusFilter)
	if err != nil {
		slog.Warn("ai: admin counting agent sessions", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count agent sessions")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": sessionsToJSON(sessions),
		"meta": map[string]any{
			"total":         total,
			"page_size":     limit,
			"page_offset":   offset,
			"status_filter": statusFilter,
		},
	})
}

// HandleAdminGetSession returns any session in the tenant (not just the
// caller's) plus its full tool call audit trail. Admin-only.
func (h *AgentHTTPHandlers) HandleAdminGetSession(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	sessionID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}

	session, err := h.sessions.GetAgentSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "session_not_found", "Agent session not found")
			return
		}

		slog.Warn("ai: admin getting agent session", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve agent session")

		return
	}

	toolCalls, err := h.sessions.ListAgentToolCallsBySession(r.Context(), sessionID)
	if err != nil {
		slog.Warn("ai: admin listing tool calls", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve tool call log")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"session":    sessionToJSON(session),
			"tool_calls": toolCallsToJSON(toolCalls),
		},
	})
}

// HandleAdminToolStats returns per-tool usage statistics (call count,
// success rate, average latency) for the tenant, optionally filtered by
// date range (?from=YYYY-MM-DD&to=YYYY-MM-DD).
func (h *AgentHTTPHandlers) HandleAdminToolStats(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	fromDate, toDate, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	stats, err := h.sessions.GetAgentToolStats(r.Context(), gen.GetAgentToolStatsParams{

		FromDate: fromDate,
		ToDate:   toDate,
	})
	if err != nil {
		slog.Warn("ai: admin tool stats", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve tool statistics")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toolStatsToJSON(stats)})
}

// HandleAdminCostReport returns a cost breakdown by user for the tenant,
// optionally filtered by date range.
func (h *AgentHTTPHandlers) HandleAdminCostReport(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	fromDate, toDate, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	report, err := h.sessions.GetAgentCostReport(r.Context(), gen.GetAgentCostReportParams{

		FromDate: fromDate,
		ToDate:   toDate,
	})
	if err != nil {
		slog.Warn("ai: admin cost report", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve cost report")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": costReportToJSON(report)})
}

// ---------------------------------------------------------------------------
// Task 7.07 — POST /api/v1/ai/compare-entities (direct comparison endpoint)
// ---------------------------------------------------------------------------

// compareEntitiesRequest is the body shape for POST /api/v1/ai/compare-entities
// per §103 API Routes:
// //	{ entity_type, entity_ids: uuid[], comparison_fields?: string[],
// focus?: string, geo_space_id?: uuid, create_post?: bool }
// // entity_type is the slug of a comparison_type_definition (e.g.
// "universities"). entity_ids are the comparison_items to compare
// (referenced by their comparison_items.id). When create_post is true and
// the tier is unstructured, the engine creates a new comparison content
// row (status = "draft") and returns its content_id.
type compareEntitiesRequest struct {
	EntityType       string   `json:"entity_type"`
	EntityIDs        []string `json:"entity_ids"`
	ComparisonFields []string `json:"comparison_fields,omitempty"`
	Focus            string   `json:"focus,omitempty"`
	GeoSpaceID       string   `json:"geo_space_id,omitempty"`
	CreatePost       bool     `json:"create_post,omitempty"`
}

// HandleCompareEntities is the direct comparison endpoint. It orchestrates
// the ComparisonEngine (Phase 7): classifies the request into a tier
// (structured / semi-structured / unstructured), gathers knowledge via
// pre-search (RAG + content search), and — for Tier 3 with create_post —
// creates a new comparison content row. The actual value synthesis is left
// to the agent loop (which has the LLM); this endpoint prepares the
// structure and returns the tier + knowledge + existing/new comparison.
// // Per §103: "agent-accessible, also directly callable". Gated by agent.use.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine, API Routes)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7, task 7.07).
func (h *AgentHTTPHandlers) HandleCompareEntities(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input compareEntitiesRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.EntityType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "entity_type is required")
		return
	}

	if len(input.EntityIDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "entity_ids is required (at least one)")
		return
	}

	if h.engine == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "engine_unavailable", "Comparison engine is not configured")
		return
	}

	// Classify the request into a tier.
	tier, typeRow, existingDims, missing, err := h.engine.ClassifyRequest(
		r.Context(), input.EntityType, input.ComparisonFields,
	)
	if err != nil {
		slog.Warn("ai: classifying comparison request", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "classification_failed", "Failed to classify comparison request")

		return
	}

	// Gather knowledge via pre-search (RAG + content search). The focus
	// field guides the retrieval; entity_type + entity_ids form the query.
	var preSearchResult *PreSearchResult

	if h.preSearch != nil {
		query := buildComparisonQuery(input.EntityType, input.EntityIDs, input.Focus)

		var geoSpaceID *pgtype.UUID

		if input.GeoSpaceID != "" {
			id, gidErr := scanUUID(input.GeoSpaceID)
			if gidErr == nil && id.Valid {
				geoSpaceID = &id
			}
		}

		ps, psErr := h.preSearch.PreSearch(r.Context(), tenantID, query, geoSpaceID)
		if psErr != nil {
			slog.Warn("ai: comparison pre-search failed (non-fatal)", "error", psErr)
		} else {
			preSearchResult = ps
		}
	}

	// Build the response. The tier drives what the frontend/agent does next.
	resp := map[string]any{
		"tier":                tier.String(),
		"entity_type":         input.EntityType,
		"entity_ids":          input.EntityIDs,
		"comparison_fields":   input.ComparisonFields,
		"existing_dimensions": existingDims,
		"missing_dimensions":  missing,
	}
	if typeRow != nil {
		resp["type_id"] = uuidString(typeRow.ID)
		resp["type_name"] = typeRow.Name
		resp["type_slug"] = typeRow.Slug
	}

	if preSearchResult != nil {
		resp["presearch_level"] = preSearchResult.Level
		resp["presearch_summary"] = preSearchResult.Summary
	}

	// For Tier 3 with create_post, create a new comparison content row.
	// The status is always "draft" — the agent never publishes directly.
	if tier == TierUnstructured && input.CreatePost {
		// Build items from entity_ids. The entity_ids are opaque
		// identifiers (e.g. university names or UUIDs); the engine
		// creates comparison_items with those as titles.
		items := make([]ComparisonItemInput, 0, len(input.EntityIDs))

		for _, eid := range input.EntityIDs {
			if eid == "" {
				continue
			}

			items = append(items, ComparisonItemInput{Name: eid})
		}

		// Dimensions come from comparison_fields when provided; otherwise
		// the LLM (agent loop) will propose them via comparison.add_dimension.
		dims := input.ComparisonFields

		res, createErr := h.engine.CreateComparison(r.Context(), tenantID, user.UserID, CreateComparisonInput{
			CreateType: false, // no new type — Tier 3 prose-only or LLM proposes type
			Title:      buildComparisonTitle(input.EntityType, input.EntityIDs),
			Body:       "",
			Items:      items,
			Dimensions: dims,
		})
		if createErr != nil {
			slog.Warn("ai: creating comparison content for compare-entities", "error", createErr)
			errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create comparison content")

			return
		}

		resp["content_id"] = uuidString(res.ContentID)
		resp["content_status"] = "draft"
		resp["dimension_ids"] = uuidsToStrings(res.DimensionIDs)
		resp["item_ids"] = uuidsToStrings(res.ItemIDs)

		// Read back the comparison object so the caller gets the full
		// structure (§103 response: { comparison: {...}, content_id?: uuid }).
		comp, readErr := h.engine.ReadComparison(r.Context(), res.ContentID)
		if readErr != nil {
			slog.Warn("ai: reading back comparison for compare-entities response",
				"content_id", res.ContentID, "error", readErr)
		} else {
			resp["comparison"] = comp
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// buildComparisonQuery constructs a RAG query string from the entity type +
// entity IDs + focus for the pre-search knowledge-gathering step.
func buildComparisonQuery(entityType string, entityIDs []string, focus string) string {
	parts := make([]string, 0, 2+len(entityIDs))
	parts = append(parts, "compare", entityType)
	parts = append(parts, entityIDs...)

	if focus != "" {
		parts = append(parts, focus)
	}

	return strings.Join(parts, " ")
}

// buildComparisonTitle constructs a default title for a new comparison
// content row from the entity type + entity IDs.
func buildComparisonTitle(entityType string, entityIDs []string) string {
	if len(entityIDs) == 0 {
		return entityType + " comparison"
	}

	if len(entityIDs) == 1 {
		return entityType + ": " + entityIDs[0]
	}
	// "X vs Y vs Z"
	title := entityIDs[0]
	for _, eid := range entityIDs[1:] {
		title += " vs " + eid
	}

	return title
}

// ---------------------------------------------------------------------------
// JSON serialization helpers
// ---------------------------------------------------------------------------

// sessionToJSON converts a gen.AiAgentSession to a JSON-friendly map. The
// pgtype.UUID/Timestamptz/Numeric types don't serialize to clean JSON by
// default, so we extract the values explicitly.
func sessionToJSON(s gen.AiAgentSession) map[string]any {
	out := map[string]any{
		"id":              s.ID.String(),
		"user_id":         s.UserID.String(),
		"prompt":          s.Prompt,
		"presearch_level": s.PresearchLevel,
		"status":          s.Status,
		"tool_call_count": s.ToolCallCount,
		"total_tokens":    s.TotalTokens,
		"started_at":      tsToISO(s.StartedAt),
		"created_at":      tsToISO(s.CreatedAt),
	}
	if s.GeoSpaceID.Valid {
		out["geo_space_id"] = s.GeoSpaceID.String()
	}

	if s.ResultSummary != nil {
		out["result_summary"] = *s.ResultSummary
	}

	if s.ErrorMessage != nil {
		out["error_message"] = *s.ErrorMessage
	}

	if s.CompletedAt.Valid {
		out["completed_at"] = tsToISO(s.CompletedAt)
	}
	// estimated_cost is numeric(10,4); extract the float for JSON.
	out["estimated_cost"] = numericToFloat(s.EstimatedCost)

	return out
}

// sessionsToJSON converts a slice of sessions to JSON-friendly maps.
func sessionsToJSON(sessions []gen.AiAgentSession) []map[string]any {
	out := make([]map[string]any, 0, len(sessions))
	for i := range sessions {
		out = append(out, sessionToJSON(sessions[i]))
	}

	return out
}

// toolCallToJSON converts a gen.AiAgentToolCall to a JSON-friendly map.
func toolCallToJSON(tc gen.AiAgentToolCall) map[string]any {
	out := map[string]any{
		"id":              tc.ID.String(),
		"session_id":      tc.SessionID.String(),
		"tool_name":       tc.ToolName,
		"status":          tc.Status,
		"rbac_checked":    tc.RbacChecked,
		"privacy_checked": tc.PrivacyChecked,
		"created_at":      tsToISO(tc.CreatedAt),
	}
	// arguments/result are JSONB ([]byte); decode to generic any so they
	// serialize as nested JSON rather than base64 strings.
	if len(tc.Arguments) > 0 {
		var args any
		if json.Unmarshal(tc.Arguments, &args) == nil {
			out["arguments"] = args
		} else {
			out["arguments"] = string(tc.Arguments)
		}
	}

	if len(tc.Result) > 0 {
		var res any
		if json.Unmarshal(tc.Result, &res) == nil {
			out["result"] = res
		} else {
			out["result"] = string(tc.Result)
		}
	}

	if tc.ErrorMessage != nil {
		out["error_message"] = *tc.ErrorMessage
	}

	if tc.LatencyMs != nil {
		out["latency_ms"] = *tc.LatencyMs
	}

	if tc.HitlReviewID.Valid {
		out["hitl_review_id"] = tc.HitlReviewID.String()
	}

	return out
}

// toolCallsToJSON converts a slice of tool calls to JSON-friendly maps.
func toolCallsToJSON(tcs []gen.AiAgentToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(tcs))
	for i := range tcs {
		out = append(out, toolCallToJSON(tcs[i]))
	}

	return out
}

// toolStatsToJSON converts tool stats rows to JSON-friendly maps. The
// aggregate columns (avg_latency_ms, max_latency_ms) come back as
// interface{} from sqlc; we coerce them to float64 for clean JSON.
func toolStatsToJSON(rows []gen.GetAgentToolStatsRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))

	for _, r := range rows {
		successRate := 0.0
		if r.TotalCalls > 0 {
			successRate = float64(r.SuccessCount) / float64(r.TotalCalls)
		}

		out = append(out, map[string]any{
			"tool_name":             r.ToolName,
			"total_calls":           r.TotalCalls,
			"success_count":         r.SuccessCount,
			"error_count":           r.ErrorCount,
			"unauthorized_count":    r.UnauthorizedCount,
			"privacy_blocked_count": r.PrivacyBlockedCount,
			"hitl_queued_count":     r.HitlQueuedCount,
			"success_rate":          successRate,
			"avg_latency_ms":        toFloat(r.AvgLatencyMs),
			"max_latency_ms":        toFloat(r.MaxLatencyMs),
		})
	}

	return out
}

// costReportToJSON converts cost report rows to JSON-friendly maps.
func costReportToJSON(rows []gen.GetAgentCostReportRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))

	for _, r := range rows {
		entry := map[string]any{
			"user_id":          r.UserID.String(),
			"session_count":    r.SessionCount,
			"total_tokens":     toInt64(r.TotalTokens),
			"total_tool_calls": toInt64(r.TotalToolCalls),
			"total_cost_usd":   toFloat(r.TotalCostUsd),
		}
		if r.Username != nil {
			entry["username"] = *r.Username
		}

		out = append(out, entry)
	}

	return out
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

// parseURLUUID extracts and validates a UUID from a chi URL parameter.
// Writes a 400 error and returns ok=false on failure.
func parseURLUUID(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", param+" is required")
		return pgtype.UUID{}, false
	}

	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", param+" must be a valid UUID")
		return pgtype.UUID{}, false
	}

	return id, true
}

// parseDateRange parses optional ?from and ?to date query params
// (YYYY-MM-DD) into pgtype.Timestamptz values. "from" is inclusive (start
// of day), "to" is exclusive (start of the next day). Writes a 400 error
// on invalid dates.
func parseDateRange(w http.ResponseWriter, r *http.Request) (pgtype.Timestamptz, pgtype.Timestamptz, bool) {
	var from, to pgtype.Timestamptz

	if raw := r.URL.Query().Get("from"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "'from' must be YYYY-MM-DD")
			return pgtype.Timestamptz{}, pgtype.Timestamptz{}, false
		}

		from = pgtype.Timestamptz{Time: t, Valid: true}
	}

	if raw := r.URL.Query().Get("to"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "'to' must be YYYY-MM-DD")
			return pgtype.Timestamptz{}, pgtype.Timestamptz{}, false
		}
		// 'to' is exclusive — use the start of the next day so a single
		// date covers the full day.
		to = pgtype.Timestamptz{Time: t.AddDate(0, 0, 1), Valid: true}
	}

	return from, to, true
}

// tsToISO converts a pgtype.Timestamptz to an ISO8601 string, or "" if invalid.
func tsToISO(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}

	return ts.Time.UTC().Format(time.RFC3339)
}

// numericToFloat extracts a float64 from a pgtype.Numeric. Returns 0 on
// parse failure (the column is NOT NULL with a default of 0, so a parse
// failure is a degenerate case).
func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}

	f, err := n.Float64Value()
	if err != nil {
		return 0
	}

	return f.Float64
}

// toFloat coerces an interface{} (from sqlc aggregate columns) to float64.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

// toInt64 coerces an interface{} (from sqlc aggregate columns) to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case string:
		v, _ := strconv.ParseInt(n, 10, 64)
		return v
	default:
		return 0
	}
}
