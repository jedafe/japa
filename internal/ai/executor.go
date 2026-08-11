// Package ai — executor.go implements the Agent Tool Executor pipeline
// (Section 103, ADR-111).
//
// Every tool call the LLM emits during the agent loop is routed through
// Executor.Execute before it reaches an internal service function. The
// pipeline enforces the §103 Tool Executor checks in an order that is
// strictly safer than the spec's textual listing — the security blocklist
// runs before RBAC and privacy so a blocked operation never consumes a
// cache lookup or DB query:
//
//  1. Schema validation — are the arguments valid per the tool's JSON schema?
//  2. Security boundary — is this a blocked operation? (§103 step 4, moved first)
//  3. RBAC check — can THIS user perform THIS operation? (§103 step 2)
//  4. Privacy check — is the target data AI-exempt? (§103 step 3)
//  5. Execute — dispatch to the registered handler. (§103 step 5)
//  6. HITL — if requires_hitl is true, create an ai_hitl_reviews record. (§103 step 7)
//  7. Log — record in ai_agent_tool_calls with status + latency_ms. (§103 step 6, deferred)
//
// The executor never holds a database transaction open across an external
// call. Each step that needs the DB uses a short, scoped query.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Executor)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 3, tasks 3.03-3.08)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Result + handler types
// ---------------------------------------------------------------------------

// ToolResult is the unified return value of every tool handler. Content is a
// JSON string the agent loop feeds back to the LLM as the tool-result message
// body. Status is one of the ToolCallStatus* constants below. Metadata is
// reserved for executor-internal bookkeeping (e.g. the created content_id)
// and is not sent to the LLM.
type ToolResult struct {
	Content  string         `json:"content"`
	Status   string         `json:"status"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Tool call status constants — match the ai_agent_tool_calls.status CHECK
// constraint from migration 000372: pending, success, error, unauthorized,
// privacy_blocked, hitl_queued.
const (
	ToolCallStatusPending        = "pending"
	ToolCallStatusSuccess        = "success"
	ToolCallStatusError          = "error"
	ToolCallStatusUnauthorized   = "unauthorized"
	ToolCallStatusPrivacyBlocked = "privacy_blocked"
	ToolCallStatusHITLQueued     = "hitl_queued"
)

// ToolHandler is the function signature every registered tool implements.
// args is the raw JSON the LLM produced (already validated against the
// tool's required fields by the executor). The handler returns a ToolResult
// whose Content is relayed to the LLM.
//
// Handlers perform their own entity-level privacy checks via the injected
// PrivacyChecker when they target a specific content/cfv/user row that the
// executor-level PrivacyTargetFunc hook did not cover.
type ToolHandler func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error)

// PrivacyTargetFunc extracts the target entity (type + ID) for the §103
// step-3 privacy check from a tool call's arguments. The executor calls it
// before dispatch; when the returned entity is ai_exempt the call is blocked
// with ToolCallStatusPrivacyBlocked. Returning a zero entity type or ID
// signals "no privacy target" and the executor skips the exemption check —
// useful for tools like rag.search that filter exempt rows at the SQL layer
// rather than targeting a single row.
type PrivacyTargetFunc func(ctx context.Context, args json.RawMessage, tenantID pgtype.UUID) (PrivacyEntityType, pgtype.UUID, error)

// ---------------------------------------------------------------------------
// Tool config (parsed from ai_tools.config JSONB)
// ---------------------------------------------------------------------------

// ToolConfig is the parsed shape of the ai_tools.config JSONB column per §103
// "Tool Catalog Expansion". The function_name matches the key the LLM emits
// in a ToolCall; handler is the registry key used to look up the Go handler.
// Behavioral flags (requires_hitl, is_blocked, reads_user_data,
// writes_user_data) drive executor decisions.
type ToolConfig struct {
	FunctionName   string         `json:"function_name"`
	Description    string         `json:"description,omitempty"`
	Parameters     map[string]any `json:"parameters,omitempty"`
	Handler        string         `json:"handler,omitempty"`
	ReadsUserData  bool           `json:"reads_user_data,omitempty"`
	WritesUserData bool           `json:"writes_user_data,omitempty"`
	RequiresHITL   bool           `json:"requires_hitl,omitempty"`
	IsBlocked      bool           `json:"is_blocked,omitempty"`
}

// requiredFields returns the top-level "required" property names from the
// tool's JSON Schema parameters, or nil if the schema declares none. Used by
// the executor's lightweight schema-validation step. Full JSON Schema
// validation (types, formats, additionalProperties) is deferred to the LLM
// providers, which validate tool arguments natively before emitting them.
func (c ToolConfig) requiredFields() []string {
	if c.Parameters == nil {
		return nil
	}
	raw, ok := c.Parameters["required"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tool handler registry (task 3.08)
// ---------------------------------------------------------------------------

// ToolHandlerRegistry maps tool function names to their Go handlers and
// parsed configs. It is the runtime counterpart of the ai_tools table: the
// table stores the declarative catalog (function schema, behavioral flags,
// required_permission), and the registry binds each function_name to the
// Go function that implements it.
//
// required_permission lives on the ai_tools row itself (not the config JSONB)
// and is loaded alongside the config so CatalogForUser can filter by the
// calling user's RBAC grants.
type ToolHandlerRegistry struct {
	handlers map[string]toolEntry
	configs  map[string]loadedTool
}

// toolEntry binds a Go handler to its optional privacy-target extractor.
type toolEntry struct {
	handler       ToolHandler
	privacyTarget PrivacyTargetFunc
}

type loadedTool struct {
	config             ToolConfig
	requiredPermission string // from ai_tools.required_permission; empty = no permission gate
}

// NewToolHandlerRegistry creates an empty registry.
func NewToolHandlerRegistry() *ToolHandlerRegistry {
	return &ToolHandlerRegistry{
		handlers: make(map[string]toolEntry),
		configs:  make(map[string]loadedTool),
	}
}

// RegisterHandler binds a Go handler to a tool function name. The handler
// must match the function_name in the ai_tools config JSONB. Registering a
// duplicate name replaces the prior handler (used by tests and hot-reload).
func (r *ToolHandlerRegistry) RegisterHandler(functionName string, handler ToolHandler) {
	if r == nil || handler == nil {
		return
	}
	existing := r.handlers[functionName]
	r.handlers[functionName] = toolEntry{handler: handler, privacyTarget: existing.privacyTarget}
}

// RegisterPrivacyTarget binds a privacy-target extractor to a tool function
// name. The extractor is called by the executor before dispatch to determine
// whether the targeted entity is ai_exempt. Must be called after
// RegisterHandler for the same name; calling it first is a no-op.
func (r *ToolHandlerRegistry) RegisterPrivacyTarget(functionName string, fn PrivacyTargetFunc) {
	if r == nil || fn == nil {
		return
	}
	existing, ok := r.handlers[functionName]
	if !ok {
		return
	}
	r.handlers[functionName] = toolEntry{handler: existing.handler, privacyTarget: fn}
}

// RegisterConfig records the parsed config + required_permission for a tool
// loaded from the ai_tools table. Called by LoadFromDB for each enabled row.
func (r *ToolHandlerRegistry) RegisterConfig(functionName string, cfg ToolConfig, requiredPermission string) {
	if r == nil {
		return
	}
	r.configs[functionName] = loadedTool{config: cfg, requiredPermission: requiredPermission}
}

// LoadFromDB loads enabled ai_tools rows for the tenant, parses each row's
// config JSONB into a ToolConfig, and registers the config. The Go handlers
// are registered separately via RegisterHandler (the table stores a handler
// *reference* string, not the function itself). Tools whose config JSONB
// fails to parse are skipped with a warning log — a malformed row must not
// break the whole catalog.
func (r *ToolHandlerRegistry) LoadFromDB(ctx context.Context, registry *ToolRegistry, tenantID pgtype.UUID) error {
	if r == nil || registry == nil {
		return errors.New("ai: registry and loader are required")
	}

	tools, err := registry.ListTools(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("loading ai_tools for catalog: %w", err)
	}

	for _, t := range tools {
		if !t.IsEnabled {
			continue
		}

		var cfg ToolConfig
		if len(t.Config) > 0 {
			if err := json.Unmarshal(t.Config, &cfg); err != nil {
				slog.Warn("ai: skipping tool with unparseable config",
					"tool", t.Name, "error", err)
				continue
			}
		}
		// The ai_tools.name column holds the function_name (e.g.
		// "content.search"). The config JSONB may repeat it; prefer the
		// column value as the source of truth so a stale config field
		// cannot shadow the canonical name.
		functionName := t.Name
		if cfg.FunctionName == "" {
			cfg.FunctionName = functionName
		}

		requiredPerm := ""
		if t.RequiredPermission != nil {
			requiredPerm = *t.RequiredPermission
		}

		r.RegisterConfig(functionName, cfg, requiredPerm)
	}

	return nil
}

// CatalogForUser returns the tool configs available to the given user,
// filtered by (a) the hardcoded blocklist (IsBlockedOperation), (b) the
// per-tool is_blocked flag, and (c) the user's RBAC grants against each
// tool's required_permission. Tools with no required_permission are
// available to every user who holds the agent.use node (the agent loop
// checks agent.use at the door; the catalog filter is per-tool).
func (r *ToolHandlerRegistry) CatalogForUser(ctx context.Context, perms PermissionChecker, userID, tenantID pgtype.UUID) []ToolConfig {
	if r == nil {
		return nil
	}

	out := make([]ToolConfig, 0, len(r.configs))
	for name, lt := range r.configs {
		if IsBlockedOperation(name) || lt.config.IsBlocked {
			continue
		}
		if lt.requiredPermission != "" && perms != nil {
			if !perms.HasPermission(ctx, userID, tenantID, lt.requiredPermission) {
				continue
			}
		}
		out = append(out, lt.config)
	}
	// Deterministic order by function name so the LLM tool list is stable
	// across calls (important for prompt reproducibility and test stability).
	sort.Slice(out, func(i, j int) bool {
		return out[i].FunctionName < out[j].FunctionName
	})
	return out
}

// Handler returns the registered Go handler for a function name, or nil.
func (r *ToolHandlerRegistry) Handler(functionName string) ToolHandler {
	if r == nil {
		return nil
	}
	if e, ok := r.handlers[functionName]; ok {
		return e.handler
	}
	return nil
}

// privacyTargetFor returns the registered privacy-target extractor for a
// function name, or nil.
func (r *ToolHandlerRegistry) privacyTargetFor(functionName string) PrivacyTargetFunc {
	if r == nil {
		return nil
	}
	if e, ok := r.handlers[functionName]; ok {
		return e.privacyTarget
	}
	return nil
}

// Config returns the loaded config for a function name. ok is false when the
// name is not in the registry.
func (r *ToolHandlerRegistry) Config(functionName string) (ToolConfig, bool) {
	if r == nil {
		return ToolConfig{}, false
	}
	lt, ok := r.configs[functionName]
	return lt.config, ok
}

// loadedConfig returns the full loadedTool (config + required_permission) for
// a function name. Used by the executor's RBAC step to access the
// required_permission that lives on the ai_tools row, not in the config JSONB.
func (r *ToolHandlerRegistry) loadedConfig(functionName string) (loadedTool, bool) {
	if r == nil {
		return loadedTool{}, false
	}
	lt, ok := r.configs[functionName]
	return lt, ok
}

// ---------------------------------------------------------------------------
// Executor (tasks 3.03-3.07)
// ---------------------------------------------------------------------------

// PrivacyExemptionChecker is the subset of PrivacyChecker the executor
// needs. Defined as an interface so the executor can be tested with a stub
// instead of a live gen.Queries. The concrete *PrivacyChecker satisfies it.
type PrivacyExemptionChecker interface {
	IsExempt(ctx context.Context, entityType PrivacyEntityType, tenantID, entityID pgtype.UUID) (bool, error)
}

// Executor runs the §103 tool-call pipeline. It is the single chokepoint
// between the LLM's function-calling output and the platform's internal
// service functions. Every tool call is authorized, privacy-checked,
// blocklisted, dispatched, logged, and (conditionally) queued for HITL here.
type Executor struct {
	registry *ToolHandlerRegistry
	privacy  PrivacyExemptionChecker
	perms    PermissionChecker
	store    *Store       // for HITL review creation
	write    *gen.Queries // for ai_agent_tool_calls logging
}

// ExecutorOption configures an Executor. PrivacyChecker and PermissionChecker
// are required; Store and write Queries are required for HITL and logging.
type ExecutorOption func(*Executor)

// NewExecutor creates an Executor with the given registry and applies the
// options. The registry, privacy checker, permission checker, store, and
// write queries must be supplied via options.
func NewExecutor(registry *ToolHandlerRegistry, opts ...ExecutorOption) *Executor {
	e := &Executor{registry: registry}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	return e
}

// WithPrivacyChecker injects the PrivacyChecker used for step 3.
func WithPrivacyChecker(c PrivacyExemptionChecker) ExecutorOption {
	return func(e *Executor) { e.privacy = c }
}

// WithPermissionChecker injects the RBAC checker used for step 2.
func WithPermissionChecker(p PermissionChecker) ExecutorOption {
	return func(e *Executor) { e.perms = p }
}

// WithStore injects the Store used for HITL review creation (step 7).
func WithStore(s *Store) ExecutorOption {
	return func(e *Executor) { e.store = s }
}

// WithWriteQueries injects the gen.Queries (backed by the writer pool) used
// for ai_agent_tool_calls logging (step 6).
func WithWriteQueries(q *gen.Queries) ExecutorOption {
	return func(e *Executor) { e.write = q }
}

// ExecuteParams carries the per-call context the executor needs that is not
// part of the ToolCall itself: the owning session (for the
// ai_agent_tool_calls.session_id FK) and the calling user + tenant.
type ExecuteParams struct {
	Call      ToolCall
	SessionID pgtype.UUID
	UserID    pgtype.UUID
	TenantID  pgtype.UUID
}

// Execute runs the full §103 tool-call pipeline for a single LLM tool call
// and returns the handler's result. Every outcome — success, error,
// unauthorized, privacy_blocked, hitl_queued — is recorded as an
// ai_agent_tool_calls row with status and latency_ms.
//
// The method never panics: a handler panic is recovered and recorded as an
// error result so one bad tool cannot crash the agent loop. The return
// values are named so the recovery defer can set them before the logging
// defer persists the final status.
func (e *Executor) Execute(ctx context.Context, p ExecuteParams) (res *ToolResult, err error) {
	if e == nil {
		return nil, errors.New("ai: executor not configured")
	}
	if e.registry == nil {
		return nil, errors.New("ai: executor has no tool registry")
	}

	start := time.Now()
	call := p.Call

	// logToolCall writes the ai_agent_tool_calls row with the final status
	// and latency. It is deferred so every return path is logged. The
	// initial row is created with status "pending"; the deferred update
	// flips it to the terminal status. When the writer is nil (e.g. in
	// unit tests that only exercise the pipeline logic), logging is skipped.
	var (
		resultStatus  = ToolCallStatusError
		resultContent string
		resultErr     error
		hitlReviewID  pgtype.UUID
	)
	defer func() {
		e.finalizeToolCall(ctx, p, call, resultStatus, resultContent, resultErr, hitlReviewID, start)
	}()
	defer func() {
		if rcv := recover(); rcv != nil {
			resultStatus = ToolCallStatusError
			resultErr = fmt.Errorf("ai: tool handler panic for %q: %v", call.Name, rcv)
			res = e.errorResult(resultErr)
			err = resultErr
			slog.Error("ai: tool handler panic", "tool", call.Name, "panic", rcv)
		}
	}()

	// Step 1 — Schema validation (lightweight: required top-level fields).
	cfg, ok := e.registry.Config(call.Name)
	if !ok {
		resultErr = fmt.Errorf("ai: unknown tool %q", call.Name)
		return e.errorResult(resultErr), resultErr
	}
	if err := validateRequiredArgs(call.Arguments, cfg.requiredFields()); err != nil {
		resultErr = fmt.Errorf("ai: invalid arguments for %q: %w", call.Name, err)
		return e.errorResult(resultErr), resultErr
	}

	// Step 2 — Security boundary (§103 step 4, moved before RBAC/privacy so
	// a blocked op never reaches those checks — defense-in-depth: even a
	// misconfigured catalog row cannot let a blocked tool through).
	if IsBlockedOperation(call.Name) || cfg.IsBlocked {
		resultStatus = ToolCallStatusUnauthorized
		resultErr = fmt.Errorf("ai: operation %q is not available through the agent", call.Name)
		return e.unauthorizedResult(resultErr), resultErr
	}

	// Step 3 — RBAC check. Fail-closed: if a tool requires a permission but
	// no permission checker is configured, the call is denied. A nil perms
	// checker is a misconfiguration that must never silently allow a gated
	// tool through.
	if lt, hasCfg := e.registry.loadedConfig(call.Name); hasCfg && lt.requiredPermission != "" {
		if e.perms == nil {
			resultStatus = ToolCallStatusUnauthorized
			resultErr = fmt.Errorf("ai: tool %q requires permission %q but no permission checker is configured", call.Name, lt.requiredPermission)
			return e.unauthorizedResult(resultErr), resultErr
		}
		if !e.perms.HasPermission(ctx, p.UserID, p.TenantID, lt.requiredPermission) {
			resultStatus = ToolCallStatusUnauthorized
			resultErr = fmt.Errorf("ai: user lacks permission %q for tool %q", lt.requiredPermission, call.Name)
			return e.unauthorizedResult(resultErr), resultErr
		}
	}

	// Step 4 — Privacy check (only when the handler exposes a target).
	handler := e.registry.Handler(call.Name)
	if handler == nil {
		resultErr = fmt.Errorf("ai: no handler registered for tool %q", call.Name)
		return e.errorResult(resultErr), resultErr
	}
	if e.privacy != nil {
		if pt := e.registry.privacyTargetFor(call.Name); pt != nil {
			et, eid, err := pt(ctx, call.Arguments, p.TenantID)
			if err != nil {
				resultErr = fmt.Errorf("ai: resolving privacy target for %q: %w", call.Name, err)
				return e.errorResult(resultErr), resultErr
			}
			if et != "" && eid != (pgtype.UUID{}) {
				exempt, err := e.privacy.IsExempt(ctx, et, p.TenantID, eid)
				if err != nil {
					resultErr = fmt.Errorf("ai: privacy check for %q: %w", call.Name, err)
					return e.errorResult(resultErr), resultErr
				}
				if exempt {
					resultStatus = ToolCallStatusPrivacyBlocked
					resultErr = fmt.Errorf("%w: tool %q targets an ai_exempt entity", ErrPrivacyBlocked, call.Name)
					return e.privacyBlockedResult(resultErr), resultErr
				}
			}
		}
	}

	// Step 5 — Dispatch (§103 step 5).
	hres, herr := handler(ctx, call.Arguments, p.UserID, p.TenantID)
	if herr != nil {
		resultErr = fmt.Errorf("ai: tool %q failed: %w", call.Name, herr)
		return e.errorResult(resultErr), resultErr
	}
	if hres == nil {
		hres = &ToolResult{Status: ToolCallStatusSuccess, Content: "{}"}
	}

	// Step 6 — HITL (§103 step 7, when the tool config requests it).
	// A handler may pre-create its own HITL review (e.g. content.create_draft
	// sets source='agent_draft_request' + content_id + persona context the
	// executor does not have) and surface the review ID via
	// hres.Metadata["hitl_review_id"]. When present, the executor honors it
	// and skips its own createHITL — avoiding a duplicate review row.
	if cfg.RequiresHITL && e.store != nil {
		if preID, ok := hitlReviewIDFromMeta(hres); ok {
			hitlReviewID = preID
			resultStatus = ToolCallStatusHITLQueued
			hres.Status = ToolCallStatusHITLQueued
		} else {
			review, hitlErr := e.createHITL(ctx, p, call, cfg, hres)
			if hitlErr != nil {
				slog.Warn("ai: HITL review creation failed; tool result returned without review",
					"tool", call.Name, "error", hitlErr)
				// Non-fatal: the tool call still succeeded; the HITL queue
				// simply did not get a row. The admin can still see the tool
				// call in ai_agent_tool_calls.
			} else {
				hitlReviewID = review.ID
				resultStatus = ToolCallStatusHITLQueued
				hres.Status = ToolCallStatusHITLQueued
				hres.Metadata = appendMeta(hres.Metadata, "hitl_review_id", review.ID)
			}
		}
	}

	resultStatus = hres.Status
	if resultStatus == "" {
		resultStatus = ToolCallStatusSuccess
	}
	resultContent = hres.Content
	return hres, nil
}

// createHITL builds and persists an ai_hitl_reviews row for a tool call that
// requested human review. content_type is derived from the tool name's
// namespace (e.g. "content.create" → "content"); ai_output is the tool
// result JSON. content_id is left null — the agent may create a HITL review
// for an operation that has no backing content row (e.g. a bulk field set).
//
// The source is set to 'agent_tool' (migration 000395) so the admin panel
// can filter tool-triggered HITL reviews from agent draft requests
// ('agent_draft_request') and manual/admin reviews ('manual'). The
// content.create_draft tool sets its own source='agent_draft_request'
// inside the handler because it has the content_id + persona context the
// executor does not.
func (e *Executor) createHITL(ctx context.Context, p ExecuteParams, call ToolCall, cfg ToolConfig, res *ToolResult) (HITLReview, error) {
	contentType := deriveContentType(call.Name)
	aiOutput := []byte(res.Content)
	if len(aiOutput) == 0 {
		aiOutput = []byte("{}")
	}

	review := HITLReview{
		TenantID:    p.TenantID,
		ContentType: contentType,
		AIOutput:    aiOutput,
		Source:      HITLSourceAgentTool,
	}
	return CreateHITLReview(ctx, e.store, review)
}

// deriveContentType extracts the HITL content_type from a tool function name.
// The function name is namespaced (e.g. "content.create", "menu.update",
// "custom_field_values.bulk_set"); the leading segment before the first dot
// is the content type. Tools with no dot fall back to the full name.
func deriveContentType(functionName string) string {
	if i := strings.IndexByte(functionName, '.'); i > 0 {
		return functionName[:i]
	}
	return functionName
}

// appendMeta returns metadata with key set to value, allocating the map if
// nil. Keeps handler code free of nil-map guards.
func appendMeta(m map[string]any, key string, value any) map[string]any {
	if m == nil {
		m = make(map[string]any, 1)
	}
	m[key] = value
	return m
}

// hitlReviewIDFromMeta extracts a pre-created HITL review ID from a tool
// result's Metadata. Handlers that create their own HITL review (e.g.
// content.create_draft, which needs content_id + persona context the
// executor lacks) set Metadata["hitl_review_id"] to the review's UUID so
// the executor skips its own createHITL and records the existing review ID
// on the ai_agent_tool_calls row. Returns (id, false) when not set.
func hitlReviewIDFromMeta(res *ToolResult) (pgtype.UUID, bool) {
	if res == nil || res.Metadata == nil {
		return pgtype.UUID{}, false
	}
	v, ok := res.Metadata["hitl_review_id"]
	if !ok {
		return pgtype.UUID{}, false
	}
	id, ok := v.(pgtype.UUID)
	if !ok || !id.Valid {
		return pgtype.UUID{}, false
	}
	return id, true
}

// ---------------------------------------------------------------------------
// Result constructors — set the status the deferred logger will persist.
// ---------------------------------------------------------------------------

func (e *Executor) errorResult(err error) *ToolResult {
	return &ToolResult{Status: ToolCallStatusError, Content: errorJSON(err)}
}

func (e *Executor) unauthorizedResult(err error) *ToolResult {
	return &ToolResult{Status: ToolCallStatusUnauthorized, Content: errorJSON(err)}
}

func (e *Executor) privacyBlockedResult(err error) *ToolResult {
	return &ToolResult{Status: ToolCallStatusPrivacyBlocked, Content: errorJSON(err)}
}

// errorJSON marshals an error into a {"error": "..."} JSON string the LLM
// can read as the tool-result message body.
func errorJSON(err error) string {
	b, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(b)
}

// ---------------------------------------------------------------------------
// Tool-call logging (task 3.05)
// ---------------------------------------------------------------------------

// finalizeToolCall writes the ai_agent_tool_calls lifecycle: a pending row
// is created at call start (when a writer is configured), and updated with
// the terminal status, latency, result, error, and hitl_review_id at call
// end. When no writer is configured (unit tests), logging is a no-op. All
// logging errors are warned, never propagated — a logging failure must not
// mask the tool's actual result.
//
// The logging context is derived from the request context via
// context.WithoutCancel so that audit records are persisted even when the
// agent request times out or the client disconnects. A 5-second timeout
// bounds the write so a stuck DB connection does not leak a goroutine.
func (e *Executor) finalizeToolCall(ctx context.Context, p ExecuteParams, call ToolCall, status, content string, callErr error, hitlReviewID pgtype.UUID, start time.Time) {
	if e == nil || e.write == nil {
		return
	}

	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	latency := int32(time.Since(start).Milliseconds())

	args := []byte(call.Arguments)
	if len(args) == 0 {
		args = []byte("{}")
	}

	row, err := e.write.CreateAgentToolCall(logCtx, gen.CreateAgentToolCallParams{
		TenantID:       p.TenantID,
		SessionID:      p.SessionID,
		ToolName:       call.Name,
		Arguments:      args,
		Status:         ToolCallStatusPending,
		RbacChecked:    e.perms != nil,
		PrivacyChecked: e.privacy != nil,
	})
	if err != nil {
		slog.Warn("ai: failed to create ai_agent_tool_calls row", "tool", call.Name, "error", err)
		return
	}

	var errMsg *string
	if callErr != nil {
		s := callErr.Error()
		errMsg = &s
	}
	var result []byte
	if content != "" {
		result = []byte(content)
	}

	updateParams := gen.UpdateAgentToolCallResultParams{
		Result:       result,
		Status:       status,
		ErrorMessage: errMsg,
		LatencyMs:    &latency,
		ID:           row.ID,
		TenantID:     p.TenantID,
	}
	if hitlReviewID != (pgtype.UUID{}) {
		updateParams.HitlReviewID = hitlReviewID
	}

	if _, err := e.write.UpdateAgentToolCallResult(logCtx, updateParams); err != nil {
		slog.Warn("ai: failed to update ai_agent_tool_calls result", "tool", call.Name, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Schema validation helper
// ---------------------------------------------------------------------------

// validateRequiredArgs unmarshals the LLM-produced arguments and checks that
// every name in required is present at the top level. Full JSON Schema
// validation (types, formats, additionalProperties) is deferred to the LLM
// providers, which validate tool arguments natively before emitting them;
// this guard catches malformed/missing arguments from a misbehaving model
// without pulling a JSON Schema library into the module.
func validateRequiredArgs(args json.RawMessage, required []string) error {
	if len(required) == 0 {
		// Still verify the arguments are valid JSON.
		var probe any
		if err := json.Unmarshal(args, &probe); err != nil {
			return fmt.Errorf("arguments are not valid JSON: %w", err)
		}
		return nil
	}

	var obj map[string]any
	if err := json.Unmarshal(args, &obj); err != nil {
		return fmt.Errorf("arguments are not a JSON object: %w", err)
	}
	var missing []string
	for _, name := range required {
		if _, ok := obj[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required field(s): %s", strings.Join(missing, ", "))
	}
	return nil
}
