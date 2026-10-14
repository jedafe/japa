package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/honcho"
	"github.com/jedafe/japa/internal/rag"
	"github.com/jedafe/japa/internal/worker"
)

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// PermissionChecker checks whether a user holds a permission node. The
// rbac.RBAC struct satisfies this interface. Used by the ADR-025 context
// builder to check each ai.context.* category node before including that
// category in a user's AI context.
type PermissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// ConsentChecker checks whether a user has granted a specific consent type.
// The gdpr.Service satisfies this interface. When nil, consent checks are
// skipped (graceful degradation — the wiring layer SHOULD provide one).
// Used to verify ai_processing consent before sending user prompts to
// external AI providers (§19, ADR-014).
type ConsentChecker interface {
	HasConsent(ctx context.Context, userID pgtype.UUID, consentType string) (bool, error)
}

// PersonaVoiceResolver resolves a shared persona voice prompt for the AI
// handlers. It is satisfied by internal/personas.Service. The userID argument
// is the calling user so the resolver can enforce voice visibility (system
// voices or voices created by the caller are visible; ADR-114, BUG-009).
type PersonaVoiceResolver interface {
	GetVoicePrompt(ctx context.Context, userID, voiceID pgtype.UUID) (string, error)
}

// UserContextProvider fetches the per-category site activity that feeds
// buildUserAIContext (ADR-025). It is satisfied by a wiring-layer adapter that
// calls the real wallet, academy, mentorship, forum, referral, and moderation
// services/stores (BUG-001).
type UserContextProvider interface {
	GetUserAIContextData(ctx context.Context, userID pgtype.UUID, categories []string) (map[string]any, error)
}

// StudioReviewHook is called when an ai_hitl_reviews row with source='studio'
// is approved or rejected, so studio_jobs and the linked tool-table row can be
// transitioned (ADR-076 Amendment 1, Approach B).
type StudioReviewHook interface {
	OnStudioReviewApproved(ctx context.Context, reviewID, reviewerID pgtype.UUID) error
	OnStudioReviewRejected(ctx context.Context, reviewID, reviewerID pgtype.UUID) error
}

// DraftPublishHook publishes the target of a non-studio ai_hitl_reviews row
// when it is approved (§103: "Approve → publish" for agent_draft_request
// drafts and manual submissions that carry a content_id). target_table
// disambiguates: 'contents' rows get status='published'; tool-table targets
// route through the dedicated tool publisher.
type DraftPublishHook interface {
	PublishHITLTarget(ctx context.Context, reviewerID pgtype.UUID, targetTable string, targetID pgtype.UUID) error
}

// Handlers holds dependencies for AI HTTP handlers.
type Handlers struct {
	Dispatcher           *Dispatcher
	Store                *Store
	Honcho               *honcho.Client
	AsynqClient          TaskEnqueueClient
	Settings             SettingsReader // hot-reloadable system_settings (sub-toggles, quotas)
	checker              FeatureChecker // feature_flags reachability (feature.japabot); nil → settings fallback
	Perms                PermissionChecker
	consents             ConsentChecker       // §19: ai_processing consent gate (nil = skip check)
	preflight            *Preflight           // §20 amendment: shared admission + unified daily budget (nil = degrade to checker/Settings)
	ragService           RAGRetriever         // Section 69: RAG retrieval at chat time (nil = non-RAG fallback)
	HTTPClient           *http.Client         // nil → http.DefaultClient (external search / HITL verify)
	cache                CacheClient          // nil → ai.cache_enabled must be true to have any effect
	costs                *CostEstimator       // estimates per-request USD cost from token usage
	PersonaVoiceResolver                      // optional voice prompt resolver (persona_voices)
	StudioReviewHook                          // optional hook for studio HITL review lifecycle
	DraftPublishHook                          // optional hook — publish non-studio review targets on approve
	userContext          UserContextProvider  // per-category user activity data (BUG-001)
	Convos               *ConversationService // ADR-130 unified transcript store (nil = unified surface off)
	agentSessions        AgentSessionQueries  // tool-call reader for conversation detail (ADR-130)
}

// RAGRetriever is the interface for the RAG retrieval step (Section 69,
// ADR-069). The rag.Service satisfies this. When nil, HandleChat falls back
// to a non-RAG LLM call.
type RAGRetriever interface {
	Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID, viewerID pgtype.UUID) ([]rag.RetrievedChunk, error)
	FormatContext(chunks []rag.RetrievedChunk) string
}

// SetRAGService injects the RAG service for retrieval-augmented chat
// (Section 69, ADR-069). When set, HandleChat runs the retrieval step
// before the LLM call if rag.enabled is true.
func (h *Handlers) SetRAGService(r RAGRetriever) {
	h.ragService = r
}

// SetStudioReviewHook injects the studio hook so AI HITL review
// approve/reject can transition studio_jobs and linked tool-table rows.
func (h *Handlers) SetStudioReviewHook(sh StudioReviewHook) {
	h.StudioReviewHook = sh
}

// SetDraftPublishHook injects the publisher that takes a non-studio HITL
// review's target live on approve (§103: agent_draft_request drafts enter
// the queue unpublished; approve publishes them).
func (h *Handlers) SetDraftPublishHook(dh DraftPublishHook) {
	h.DraftPublishHook = dh
}

// SetConsentChecker injects the consent checker used to verify
// ai_processing consent before sending user prompts to external AI
// providers (§19, ADR-014). When nil, consent checks are skipped.
func (h *Handlers) SetConsentChecker(c ConsentChecker) {
	h.consents = c
}

// SetPreflight injects the shared §20 AI admission pipeline (feature gate
// → ai_processing consent → unified daily budget). When nil, handlers
// degrade to their own feature checker / settings / consent fields.
func (h *Handlers) SetPreflight(p *Preflight) {
	h.preflight = p
}

// enqueueAIProcessTask enqueues an async AI processing task.
// Fire-and-forget: errors are logged but never propagated.
func (h *Handlers) enqueueAIProcessTask(taskType, prompt, modelID, requestID string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"task_type":  taskType,
		"prompt":     prompt,
		"model_id":   modelID,
		"request_id": requestID,
	})
	if err != nil {
		slog.Warn("ai: failed to marshal process task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeAITaskProcessing, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeAITaskProcessing)),
	); err != nil {
		slog.Warn("ai: failed to enqueue process task", "task_type", taskType, "error", err)
	}
}

// SetPersonaVoiceResolver injects the shared voice resolver used by AI
// handlers to translate a persona_voices ID into a voice prompt.
func (h *Handlers) SetPersonaVoiceResolver(r PersonaVoiceResolver) {
	h.PersonaVoiceResolver = r
}

// SetUserContextProvider injects the per-category site-activity provider used
// by buildUserAIContext (ADR-025). When nil, the context builder only emits
// enabled_categories booleans and prior memory (BUG-001).
func (h *Handlers) SetUserContextProvider(p UserContextProvider) {
	h.userContext = p
}

// resolveVoicePrompt looks up the shared persona voice prompt for voiceID for
// the given calling user. Returns an empty string with no error when voiceID is
// blank or the resolver is not wired. Invisible voices are treated as no-op
// prompts (return "") rather than errors (BUG-009).
func (h *Handlers) resolveVoicePrompt(ctx context.Context, userID pgtype.UUID, voiceID string) (string, error) {
	if voiceID == "" || h.PersonaVoiceResolver == nil {
		return "", nil
	}

	var vid pgtype.UUID
	if err := vid.Scan(voiceID); err != nil {
		return "", err
	}

	return h.GetVoicePrompt(ctx, userID, vid)
}

// applyModelAndVoice resolves and validates a per-request model/voice override
// and mutates payload. It writes a 400 error to w and returns false on
// validation failure. Pass defaultKey as the system_settings key for the
// platform/category default (e.g. "ai.model.default.content_generation").
// // Note: assist endpoints (improve, summarize, translate, generate-tags,
// generate-og, draft) share the "content_generation" category so a single
// platform default model can power all content assistant surfaces.
func (h *Handlers) applyModelAndVoice(
	w http.ResponseWriter,
	r *http.Request,

	payload map[string]any,
	model, voiceID, category, defaultKey string,
) bool {
	// Visibility: only system voices or voices created by the caller may
	// be applied. Invisible voices resolve to an empty prompt (no-op)
	// instead of an error so the request continues without the override
	// (BUG-009).
	caller := auth.FromContext(r.Context())

	var userID pgtype.UUID

	if caller != nil {
		userID = caller.UserID
	}

	if err := h.resolveModelAndVoice(r.Context(), userID, payload, model, voiceID, category, defaultKey); err != nil {
		var ae *AssistError
		if errors.As(err, &ae) {
			errpkg.WriteError(w, ae.Status, ae.Code, ae.Message)
		} else {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
		}

		return false
	}

	return true
}

// NewHandlers creates Handlers with the given dependencies.
func NewHandlers(dispatcher *Dispatcher, store *Store) *Handlers {
	return &Handlers{Dispatcher: dispatcher, Store: store}
}

// SetHonchoClient injects the Honcho client for admin context operations.
func (h *Handlers) SetHonchoClient(c *honcho.Client) {
	h.Honcho = c
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability (feature.japabot).
// *features.Checker satisfies it structurally.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// SetSettings injects the hot-reloadable system_settings reader (Rule 3).
func (h *Handlers) SetSettings(s SettingsReader) {
	h.Settings = s
	h.costs = NewCostEstimator(s)
}

// SetFeatureChecker injects the feature flag checker so AI surface
// reachability reads the same feature_flags keys the route gates enforce
// (feature.japabot, feature.ai_content_gen) instead of the legacy
// system_settings keys (kept as the nil-checker fallback).
func (h *Handlers) SetFeatureChecker(c FeatureChecker) {
	h.checker = c
}

// SetCacheClient injects the Valkey cache client used for the
// jc:ai:response:* semantic cache. nil disables caching.
func (h *Handlers) SetCacheClient(c CacheClient) {
	h.cache = c
}

// SetRBAC injects the permission checker used by the ADR-025 context builder
// to gate which ai.context.* categories feed a given user's AI context.
func (h *Handlers) SetRBAC(p PermissionChecker) {
	h.Perms = p
}

// aiContextCategories maps each ai.context.* category node (ADR-025) to the
// key under which the category's data is placed in the AI context payload.
// A category is only included when the user holds the corresponding node.
var aiContextCategories = map[string]string{
	"ai.context.points_balance":      "points_balance",
	"ai.context.course_progress":     "course_progress",
	"ai.context.mentorship_activity": "mentorship_activity",
	"ai.context.forum_activity":      "forum_activity",
	"ai.context.referral_activity":   "referral_activity",
	"ai.context.moderation_history":  "moderation_history",
}

// buildUserAIContext assembles the AI context payload for a JapaBot chat
// request (ADR-025). It merges:
// 1. Prior persistent memory from Honcho (Section 20) — retrieved via
// h.Honcho.GetContext and included as "prior_memory".
// 2. Per-category site-activity context, gated by the ai.context.* nodes
// (ADR-025). Each category is only included when the user holds the
// corresponding permission node (all default OFF, including super_admin).
// // Caller-supplied input.Context is merged last (lowest precedence) so the
// server remains the authority over what the AI can see.
func (h *Handlers) buildUserAIContext(ctx context.Context, userID pgtype.UUID, clientContext map[string]any) map[string]any {
	out := map[string]any{}

	// 1. Prior Honcho memory (Section 20 persistent context).
	if h.Honcho != nil {
		if entries, err := h.Honcho.GetContext(ctx, userID.String()); err == nil {
			mem := make([]map[string]any, 0, len(entries))
			for _, e := range entries {
				mem = append(mem, map[string]any{
					"id":         e.ID,
					"content":    e.Content,
					"created_at": e.CreatedAt,
				})
			}

			out["prior_memory"] = mem
		} else {
			slog.Warn("ai: loading prior honcho memory for context", "error", err, "user_id", userID)
		}
	}

	// 2. Per-category activity context gated by ai.context.* nodes (ADR-025).
	enabledKeys := make([]string, 0, len(aiContextCategories))

	if h.Perms != nil {
		enabled := map[string]bool{}

		for node, key := range aiContextCategories {
			if h.Perms.HasPermission(ctx, userID, node) {
				enabled[key] = true

				enabledKeys = append(enabledKeys, key)
			}
		}

		out["enabled_categories"] = enabled

		// Fetch actual data for the enabled categories (BUG-001).
		if h.userContext != nil && len(enabledKeys) > 0 {
			if data, err := h.userContext.GetUserAIContextData(ctx, userID, enabledKeys); err == nil {
				for k, v := range data {
					out[k] = v
				}
			} else {
				slog.Warn("ai: loading user context data", "error", err, "user_id", userID)
			}
		}
	}

	// 3. Merge caller-supplied context (lowest precedence).
	for k, v := range clientContext {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}

	return out
}

// persistChatTurn stores the user message and AI reply to Honcho as persistent
// memory (Section 20). Fire-and-forget: errors are logged, never propagated,
// so a Honcho outage never blocks the chat response (Rule 8 spirit).
func (h *Handlers) persistChatTurn(ctx context.Context, userID, message, reply, conversationID string) {
	if h.Honcho == nil {
		return
	}

	meta := map[string]any{"role": "user"}
	if conversationID != "" {
		meta["conversation_id"] = conversationID
	}

	if _, err := h.Honcho.AddMemory(ctx, userID, honcho.AddMemoryInput{
		Content:  message,
		Metadata: meta,
	}); err != nil {
		slog.Warn("ai: persisting user message to honcho", "error", err, "user_id", userID)
	}

	replyMeta := map[string]any{"role": "assistant"}
	if conversationID != "" {
		replyMeta["conversation_id"] = conversationID
	}

	if _, err := h.Honcho.AddMemory(ctx, userID, honcho.AddMemoryInput{
		Content:  reply,
		Metadata: replyMeta,
	}); err != nil {
		slog.Warn("ai: persisting assistant reply to honcho", "error", err, "user_id", userID)
	}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return nil, false
	}

	return user, true
}

// requireAIAdmission runs the shared §20 admission gates for a user-facing
// AI surface — surface reachability (feature_flags authoritative, legacy
// system_settings as the nil-checker fallback) then ai_processing consent
// (§19/ADR-014). With a wired preflight the checks run through the shared
// pipeline; without one the handler's own checker/Settings/consents fields
// reproduce the same semantics.
func (h *Handlers) requireAIAdmission(ctx context.Context, w http.ResponseWriter, userID pgtype.UUID, gate SurfaceGate) bool {
	if h.preflight != nil {
		if perr := h.preflight.CheckAdmission(ctx, userID, gate); perr != nil {
			errpkg.WriteError(w, perr.Status, perr.Code, perr.Message)
			return false
		}

		return true
	}

	// Degraded wiring (tests construct Handlers without a preflight):
	// resolve reachability + consent from the handler's own fields with
	// the same semantics as Preflight.CheckAdmission.
	enabled := true

	if h.checker != nil {
		enabled = h.checker.IsEnabled(ctx, gate.FeatureKey)
	} else if h.Settings != nil {
		enabled = h.Settings.GetBoolOr(ctx, gate.SettingKey, gate.SettingDefault)
	}

	if !enabled {
		errpkg.WriteError(w, http.StatusServiceUnavailable, gate.DisabledCode, gate.DisabledMsg)
		return false
	}

	if h.consents != nil && userID.Valid {
		consented, err := h.consents.HasConsent(ctx, userID, "ai_processing")
		if err != nil {
			slog.Warn("ai: consent check failed, allowing request", "error", err)
		} else if !consented {
			errpkg.WriteError(w, http.StatusForbidden, "ai_consent_required", "You must consent to AI processing before using AI features")
			return false
		}
	}

	return true
}

// requireAIBudget runs the shared §20 unified daily budget check
// (jc:ai:queries + jc:ai:cost). Call after input validation so malformed
// requests do not consume quota. Nil preflight skips the check.
func (h *Handlers) requireAIBudget(ctx context.Context, w http.ResponseWriter, userID pgtype.UUID) bool {
	if h.preflight == nil {
		return true
	}

	if perr := h.preflight.CheckBudget(ctx, userID); perr != nil {
		errpkg.WriteError(w, perr.Status, perr.Code, perr.Message)
		return false
	}

	return true
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return 20
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 20
	}

	if n > 100 {
		return 100
	}

	return int32(n) // #nosec G109 G115 -- bounded by MaxPageSize check above
}

func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) // #nosec G109 G115 -- bounded by non-negative check above
}

// allowedContextKeys is the allowlist of keys a client may supply in the
// optional "context" object on /ai/chat and /ai/generate. Unknown keys and
// nested objects are dropped to prevent prompt injection, cache poisoning,
// and the leakage of server-authoritative context (ADR-010).
var allowedContextKeys = map[string]bool{
	"title":            true,
	"slug":             true,
	"audience":         true,
	"related_features": true,
	"doc_category":     true,
	"doc_subcategory":  true,
	"language":         true,
	"content_id":       true,
	"space_id":         true,
	"geo_space_id":     true,
	"organization_id":  true,
	"tags":             true,
	"tone":             true,
	"style":            true,
	"format":           true,
	"max_length":       true,
	"max_words":        true,
	"target_locale":    true,
	"text":             true,
	"body":             true,
	"content_type":     true,
	"include_sources":  true,
	"source_count":     true,
}

// isValidContextValue reports whether a client-supplied context value is a
// scalar or a flat array of scalars. Nested maps are rejected.
func isValidContextValue(v any) bool {
	switch val := v.(type) {
	case string, bool, float64, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	case []string:
		return true
	case []any:
		for _, e := range val {
			if !isValidContextValue(e) {
				return false
			}
		}

		return true
	default:
		return false
	}
}

// sanitizeClientContext strips the client-supplied context map to the allowed
// key/value types used by the AI handlers. Dropped keys are logged (key only,
// never the value) so callers can debug missing context.
func sanitizeClientContext(ctx map[string]any) map[string]any {
	if ctx == nil {
		return nil
	}

	out := make(map[string]any, len(ctx))

	for k, v := range ctx {
		if !allowedContextKeys[k] {
			slog.Warn("ai: dropping disallowed client context key", "key", k)
			continue
		}

		if !isValidContextValue(v) {
			slog.Warn("ai: dropping client context key with unsupported value type", "key", k)
			continue
		}

		out[k] = v
	}

	return out
}

// toInt32 converts a JSON number (parsed as float64) to int32 with a
// saturating clamp. Provider token counts are never negative and practically
// never exceed int32 range; this helper lets us keep the database columns as
// integer without gosec G115 false positives.
func toInt32(v float64) int32 {
	const maxInt32 = 1<<31 - 1
	if v > maxInt32 {
		return maxInt32
	}

	if v < 0 {
		return 0
	}

	return int32(v)
}

// dispatchAndRecordUsage dispatches through the gateway and, on success,
// writes a row to ai_usage_log (Section 20 — admin AI usage monitoring).
// The usage write is fire-and-forget: a failure to log never fails the
// request. The provider name, model, and token counts are extracted from
// the Response. Cost is estimated from token usage using the per-provider
// price settings (ai.* and agent.cost_per_1k_tokens).
func (h *Handlers) dispatchAndRecordUsage(ctx context.Context, userID pgtype.UUID, taskType, category string, req Request) (Response, error) {
	cacheEnabled := false
	cacheTTL := 0

	if h.Settings != nil {
		cacheEnabled = h.Settings.GetBoolOr(ctx, "ai.cache_enabled", false)
		cacheTTL = h.Settings.GetIntOr(ctx, "ai.cache_ttl_seconds", 300)
	}

	cacheKey := ""
	if cacheEnabled && h.cache != nil {
		cacheKey = aiResponseCacheKey(userID, category, req.Payload)
		if cached, ok := lookupCachedResponse(ctx, h.cache, cacheKey); ok {
			// Cache hit: log as a zero-cost, zero-token cached call so the
			// admin dashboard can still see the volume and the savings.
			if h.Store != nil {
				if lerr := h.Store.RecordAIUsage(ctx, userID, "cache", "", taskType, "", 0, 0, 0, 0); lerr != nil {
					slog.Warn("ai: failed to record cache usage log", "task_type", taskType, "error", lerr)
				}
			}

			return cached, nil
		}
	}

	resp, err := h.Dispatcher.Dispatch(ctx, category, req)
	if err != nil {
		return resp, err
	}

	// Extract model + token counts from the provider response. The Raw map
	// is OpenAI-compatible: { "model": "...", "usage": { prompt_tokens,
	// completion_tokens, total_tokens } }.
	model, _ := resp.Raw["model"].(string)

	var promptTokens, completionTokens, totalTokens int32
	if usage, ok := resp.Raw["usage"].(mimoUsage); ok {
		promptTokens = toInt32(float64(usage.PromptTokens))
		completionTokens = toInt32(float64(usage.CompletionTokens))
		totalTokens = toInt32(float64(usage.TotalTokens))
	} else if usage, ok := resp.Raw["usage"].(openAIUsage); ok {
		promptTokens = toInt32(float64(usage.PromptTokens))
		completionTokens = toInt32(float64(usage.CompletionTokens))
		totalTokens = toInt32(float64(usage.TotalTokens))
	} else if usageRaw, ok := resp.Raw["usage"].(map[string]any); ok {
		if v, ok := usageRaw["prompt_tokens"].(float64); ok {
			promptTokens = toInt32(v)
		}

		if v, ok := usageRaw["completion_tokens"].(float64); ok {
			completionTokens = toInt32(v)
		}

		if v, ok := usageRaw["total_tokens"].(float64); ok {
			totalTokens = toInt32(v)
		}
	}

	// Estimate cost from token usage. Falls back to a conservative default
	// if no per-provider price is configured.
	var costUSD float64
	if h.costs != nil {
		costUSD = h.costs.Estimate(ctx, resp.ProviderName, model, int(totalTokens))
	}

	// Accumulate the unified daily AI spend (jc:ai:cost:{user}:{date}) so the
	// §20 shared budget covers every surface dispatched through this path —
	// generate, chat, and all assist ops.
	if h.preflight != nil {
		h.preflight.RecordCost(ctx, userID, costUSD)
	}

	// Fire-and-forget usage log. A failure here must not fail the request.
	if h.Store != nil {
		if lerr := h.Store.RecordAIUsage(ctx, userID, resp.ProviderName, model, taskType, resp.ProviderRef, promptTokens, completionTokens, totalTokens, costUSD); lerr != nil {
			slog.Warn("ai: failed to record usage log", "task_type", taskType, "error", lerr)
		}
	}

	// Write the response to the semantic cache so identical future prompts
	// can skip the LLM call entirely.
	if cacheEnabled && h.cache != nil && cacheKey != "" {
		cacheResponse(ctx, h.cache, cacheKey, resp, cacheTTL)
	}

	return resp, nil
}

// isValidModelForCategory reports whether model is exposed by any provider
// registered for the given category. The admin model picker is derived from
// the same source (Rule 16), so this keeps per-request overrides consistent
// with the UI options.
func (h *Handlers) isValidModelForCategory(model, category string) bool {
	if h.Dispatcher == nil {
		return false
	}

	for _, m := range h.Dispatcher.RegisteredModelsByCategory(category) {
		if m.ID == model {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// AI Generate Handler
// ---------------------------------------------------------------------------

// HandleGenerate dispatches an AI request through the gateway and returns
// the provider response. The request body specifies category, prompt, and
// optional context.
func (h *Handlers) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Category      string         `json:"category"`
		Prompt        string         `json:"prompt"`
		Context       map[string]any `json:"context,omitempty"`
		Humanize      *bool          `json:"humanize,omitempty"`
		HumanizeLevel string         `json:"humanize_level,omitempty"`
		Model         string         `json:"model,omitempty"`
		VoiceID       string         `json:"voice_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Category == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "category is required")

		return
	}

	if input.Prompt == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "prompt is required")

		return
	}

	// Sanitize client-supplied context to prevent prompt injection and cache
	// poisoning (ADR-010, BUG-007). Only allowed scalar keys survive; the rest
	// are dropped with a warning.
	input.Context = sanitizeClientContext(input.Context)

	// TASK-004: docs page generation is fire-and-forget. Enqueue a background
	// task and return 202 Accepted with the task reference instead of blocking
	// on the AI gateway call synchronously.
	if input.Category == "docs" {
		// Extract title, slug, audience and related_features from the context map (§44).
		// Title is required so the worker can create a real doc page draft.
		title, _ := input.Context["title"].(string)
		slug, _ := input.Context["slug"].(string)

		audience, _ := input.Context["audience"].(string)
		if audience == "" {
			audience = "users"
		}

		relatedFeatures := []string{}

		if rf, ok := input.Context["related_features"].([]any); ok {
			for _, v := range rf {
				if s, ok := v.(string); ok {
					relatedFeatures = append(relatedFeatures, s)
				}
			}
		}

		if title == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "title is required for docs generation")
			return
		}

		docCategory, _ := input.Context["doc_category"].(string)
		docSubcategory, _ := input.Context["doc_subcategory"].(string)

		payload, err := json.Marshal(map[string]any{
			"prompt":           input.Prompt,
			"title":            title,
			"slug":             slug,
			"user_id":          user.UserID.String(),
			"audience":         audience,
			"related_features": relatedFeatures,
			"doc_category":     docCategory,
			"doc_subcategory":  docSubcategory,
		})
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "marshal_error", "Failed to marshal task payload")
			return
		}

		if h.AsynqClient == nil {
			errpkg.WriteError(w, http.StatusServiceUnavailable, "no_worker", "Background task client unavailable")
			return
		}

		// DOC-014: deterministic task ID so repeated identical generation requests
		// de-duplicate. Keyed by platform + audience + slug (or title hash when no slug provided).
		idSeed := slug
		if idSeed == "" {
			sum := sha256.Sum256([]byte(title))
			idSeed = hex.EncodeToString(sum[:])
		}

		taskID := fmt.Sprintf(
			"docs:generate_page:%s:%s:%s",
			pgtype.UUID{}.String(),
			audience,
			idSeed,
		)

		info, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeDocsGeneratePage, payload),
			asynq.Queue(worker.QueueForTask(worker.TypeDocsGeneratePage)),
			asynq.TaskID(taskID),
		)
		if err != nil {
			slog.Error("ai: failed to enqueue docs:generate_page", "error", err)
			errpkg.WriteError(w, http.StatusBadGateway, "enqueue_error", "Failed to enqueue generation task")

			return
		}

		// Record a usage log entry for the docs generation request so the
		// admin AI usage dashboard can track volume even though the actual
		// gateway call happens asynchronously in the docs worker.
		if h.Store != nil {
			if lerr := h.Store.RecordAIUsage(r.Context(), user.UserID, "docs", "", "generate", info.ID, 0, 0, 0, 0); lerr != nil {
				slog.Warn("ai: failed to record docs usage log", "error", lerr)
			}
		}

		errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
			"data": map[string]any{
				"status":    "accepted",
				"task_id":   info.ID,
				"task_type": worker.TypeDocsGeneratePage,
			},
		})

		return
	}

	payload := map[string]any{
		"prompt":  input.Prompt,
		"context": input.Context,
		"user_id": user.UserID.String(),
	}

	if !h.applyModelAndVoice(w, r, payload, input.Model, input.VoiceID, input.Category, "ai.model.default."+input.Category) {
		return
	}

	// Humanization: merge global ai.humanize.* setting with per-request override.
	hEnabled, hLevel := h.resolveHandlerHumanize(r.Context(), input.Humanize, input.HumanizeLevel)
	writeHumanizePayload(payload, hEnabled, hLevel)

	resp, err := h.dispatchAndRecordUsage(r.Context(), user.UserID, "generate", input.Category, Request{
		Category: input.Category,
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai generate failed", "error", err, "category", input.Category)
		errpkg.WriteError(w, http.StatusBadGateway, "ai_error", "AI generation failed")

		return
	}

	// Enqueue async AI process task for background logging/analytics — fire-and-forget.
	h.enqueueAIProcessTask(input.Category, input.Prompt, "", "")

	// Map the provider Response to the frontend GenerateResult contract.
	// The provider returns raw data in Response.Raw; we extract the fields
	// the admin UI needs (generated_text, model, tokens_used).
	genResult := mapGenerateResult(resp)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": genResult,
	})
}

// mapGenerateResult extracts the frontend-expected fields from a provider
// Response. The provider stores the raw API response in Response.Raw; this
// function normalizes the OpenAI-compatible structure (choices[0].message.content,
// model, usage.total_tokens) into the {generated_text, model, tokens_used}
// contract that the admin GenerateResult TypeScript interface expects.
func mapGenerateResult(resp Response) map[string]any {
	result := map[string]any{
		"generated_text": "",
		"model":          "",
		"tokens_used":    0,
	}

	if resp.Raw == nil {
		return result
	}

	// Extract model.
	if model, ok := resp.Raw["model"].(string); ok {
		result["model"] = model
	}

	// Extract generated_text from choices[0].message.content or message.reasoning.
	if choices, ok := resp.Raw["choices"].([]mimoChoice); ok && len(choices) > 0 {
		result["generated_text"] = choices[0].Message.Content
	} else if choices, ok := resp.Raw["choices"].([]openAIChoice); ok && len(choices) > 0 {
		msg := choices[0].Message
		if msg.Content != "" {
			result["generated_text"] = msg.Content
		} else {
			result["generated_text"] = msg.Reasoning
		}
	} else if choicesRaw, ok := resp.Raw["choices"].([]any); ok && len(choicesRaw) > 0 {
		if choice, ok := choicesRaw[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if content, ok := msg["content"].(string); ok && content != "" {
					result["generated_text"] = content
				} else if reasoning, ok := msg["reasoning"].(string); ok {
					result["generated_text"] = reasoning
				}
			}
		}
	}

	// Extract tokens_used from usage.total_tokens.
	if usage, ok := resp.Raw["usage"].(mimoUsage); ok {
		result["tokens_used"] = usage.TotalTokens
	} else if usage, ok := resp.Raw["usage"].(openAIUsage); ok {
		result["tokens_used"] = usage.TotalTokens
	} else if usageRaw, ok := resp.Raw["usage"].(map[string]any); ok {
		if total, ok := usageRaw["total_tokens"].(float64); ok {
			// toInt32 clamps to int32 range; widen back to int for the
			// JSON response contract (DraftResult.TokensUsed is int).
			result["tokens_used"] = int(toInt32(total))
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// JapaBot Chat Handler
// ---------------------------------------------------------------------------

// HandleChat handles POST /api/v1/ai/chat — JapaBot conversational AI.
// Request body (Section 20): { "message": string, "conversation_id?": uuid }
// Response (Section 20): { "data": { "reply": string, "conversation_id": uuid, "sources?": [] } }.
// Honors the `ai.japabot_enabled` system_settings global toggle (Rule 3).
func (h *Handlers) HandleChat(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	// Shared §20 admission: feature.japabot reachability (legacy
	// ai.japabot_enabled setting is the nil-checker fallback) then
	// §19/ADR-014 ai_processing consent — short-circuits before any
	// provider call.
	if !h.requireAIAdmission(r.Context(), w, user.UserID, GateJapaBot) {
		return
	}

	var input struct {
		Message        string         `json:"message"`
		ConversationID string         `json:"conversation_id,omitempty"`
		Context        map[string]any `json:"context,omitempty"`
		Humanize       *bool          `json:"humanize,omitempty"`
		HumanizeLevel  string         `json:"humanize_level,omitempty"`
		Model          string         `json:"model,omitempty"`
		VoiceID        string         `json:"voice_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Message == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "message is required")

		return
	}

	// Sanitize client-supplied context to prevent prompt injection and cache
	// poisoning (ADR-010, BUG-007). Only allowed scalar keys survive.
	input.Context = sanitizeClientContext(input.Context)

	// Shared §20 unified daily budget (jc:ai:queries + jc:ai:cost). Checked
	// after validation so malformed requests don't consume quota.
	if !h.requireAIBudget(r.Context(), w, user.UserID) {
		return
	}

	// Keep the raw user text for memory persistence — input.Message is
	// rewritten below with the RAG context prefix, and persisting that
	// augmented prompt would pollute Honcho long-term memory with retrieved
	// passages on every turn.
	rawUserMessage := input.Message

	// Build the AI context payload (ADR-025): prior Honcho memory (Section 20)
	// + per-category site-activity context gated by ai.context.* nodes. The
	// server is the authority over what the AI can see — client-supplied
	// context is merged at lowest precedence.
	aiContext := h.buildUserAIContext(r.Context(), user.UserID, input.Context)

	// RAG retrieval step (Section 69, ADR-069): when rag.enabled is true and
	// a RAG service is configured, embed the user's query and retrieve the
	// top-k most similar content chunks from content_embeddings (pgvector
	// HNSW). The retrieved passages are injected as sourced context into the
	// LLM system prompt. If RAG is disabled or the retriever is nil, the
	// chat falls back to a non-RAG LLM call.
	var ragSources []map[string]any

	if h.ragService != nil {
		// BUG-003 fix: resolve the user's preferred language from the
		// Accept-Language header so RAG retrieval filters by language
		// (ADR-069 §3 step 3). Falls back to empty (no filter) if the
		// header is absent.
		ragLang := resolveAcceptLanguage(r)

		chunks, err := h.ragService.Retrieve(r.Context(), input.Message, ragLang, nil, user.UserID)
		if err != nil {
			slog.Warn("ai chat: RAG retrieval failed, falling back to non-RAG", "error", err)
		} else if len(chunks) > 0 {
			ragContext := h.ragService.FormatContext(chunks)
			if ragContext != "" {
				// Prepend RAG context to the prompt so the LLM grounds its
				// answer in the retrieved passages.
				input.Message = ragContext + "\n\nUser question: " + input.Message
			}

			for _, c := range chunks {
				// §69 Citation Source Contract: emit title/slug/url so the
				// frontend can link each citation to the entity's canonical
				// route (empty when the entity no longer resolves).
				ragSources = append(ragSources, map[string]any{
					"entity_type": c.EntityType,
					"entity_id":   c.EntityID,
					"similarity":  c.Similarity,
					"tier":        c.Tier,
					"title":       c.Title,
					"slug":        c.Slug,
					"url":         c.URL,
				})
			}
		}
	}

	payload := map[string]any{
		"prompt":          input.Message,
		"context":         aiContext,
		"user_id":         user.UserID.String(),
		"task":            "chat",
		"conversation_id": input.ConversationID,
	}

	if !h.applyModelAndVoice(w, r, payload, input.Model, input.VoiceID, "chat", "ai.model.default.chat") {
		return
	}

	// Humanization: merge global ai.humanize.* setting with per-request override.
	hEnabled, hLevel := h.resolveHandlerHumanize(r.Context(), input.Humanize, input.HumanizeLevel)
	writeHumanizePayload(payload, hEnabled, hLevel)

	resp, err := h.dispatchAndRecordUsage(r.Context(), user.UserID, "chat", "chat", Request{
		Category: "chat",
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai chat failed", "error", err)
		errpkg.WriteError(w, http.StatusBadGateway, "ai_error", "JapaBot is unavailable")

		return
	}

	// Shape the response to the Section 20 contract: { reply, conversation_id, sources? }.
	// The provider's text lives in resp.Raw; extract the reply text and echo the
	// conversation_id (existing or newly generated by the provider/Honcho).
	reply := ExtractReplyText(resp)

	conversationID := input.ConversationID
	if cid, ok := resp.Raw["conversation_id"].(string); ok && cid != "" {
		conversationID = cid
	}

	// Merge RAG sources with any provider-returned sources.
	sources := resp.Raw["sources"]
	if ragSources != nil {
		if sources == nil {
			sources = ragSources
		} else if existing, ok := sources.([]any); ok {
			merged := make([]any, 0, len(existing)+len(ragSources))
			merged = append(merged, existing...)

			for _, rs := range ragSources {
				merged = append(merged, rs)
			}

			sources = merged
		} else {
			sources = ragSources
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"reply":           reply,
			"conversation_id": conversationID,
			"provider_ref":    resp.ProviderRef,
			"sources":         sources,
		},
	})

	// Persist the full chat turn to Honcho only after the response has been
	// successfully shaped and written to the client (BUG-017). Fire-and-forget:
	// errors are logged, never propagated (Rule 8 spirit).
	h.persistChatTurn(r.Context(), user.UserID.String(), rawUserMessage, reply, conversationID)
}

// ExtractReplyText pulls the human-readable reply string out of a provider
// Response. OpenAI-compatible providers (Xiaomi MiMo) return choices[].message.content.
// It is exported so other packages (e.g. video summaries) can shape provider
// output without duplicating the parsing logic.
func ExtractReplyText(resp Response) string {
	if resp.Raw == nil {
		return ""
	}

	if s, ok := resp.Raw["reply"].(string); ok {
		return s
	}

	if rawChoices, ok := resp.Raw["choices"]; ok {
		if choices, ok := rawChoices.([]any); ok && len(choices) > 0 {
			if first, ok := choices[0].(map[string]any); ok {
				if msg, ok := first["message"].(map[string]any); ok {
					if content, ok := msg["content"].(string); ok {
						return content
					}
				}
			}
		}
	}

	return ""
}

// ---------------------------------------------------------------------------
// Content AI Assist Handlers
// ---------------------------------------------------------------------------

// HandleImprove handles POST /api/v1/ai/assist/improve — improve writing quality.
// Request body: { "text": "...", "tone": "professional|casual|academic" }.
// Thin adapter over AssistImprove — the shared service also backs the
// content.improve agent tool.
func (h *Handlers) HandleImprove(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.requireAIAdmission(r.Context(), w, user.UserID, GateAssist) {
		return
	}

	var input AssistImproveInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Text == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "text is required")

		return
	}

	if !h.requireAIBudget(r.Context(), w, user.UserID) {
		return
	}

	resp, err := h.AssistImprove(r.Context(), user.UserID, input)
	if err != nil {
		writeAssistError(w, "AI improvement failed", err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleSummarize handles POST /api/v1/ai/assist/summarize — summarize text.
// Request body: { "text": "...", "max_words": 100 }.
func (h *Handlers) HandleSummarize(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.requireAIAdmission(r.Context(), w, user.UserID, GateAssist) {
		return
	}

	var input AssistSummarizeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Text == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "text is required")

		return
	}

	if !h.requireAIBudget(r.Context(), w, user.UserID) {
		return
	}

	resp, err := h.AssistSummarize(r.Context(), user.UserID, input)
	if err != nil {
		writeAssistError(w, "AI summarization failed", err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleTranslate handles POST /api/v1/ai/assist/translate — translate text.
// Request body: { "text": "...", "target_locale": "fr" }.
func (h *Handlers) HandleTranslate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.requireAIAdmission(r.Context(), w, user.UserID, GateAssist) {
		return
	}

	var input AssistTranslateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Text == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "text is required")

		return
	}

	if input.TargetLocale == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "target_locale is required")

		return
	}

	if !h.requireAIBudget(r.Context(), w, user.UserID) {
		return
	}

	resp, err := h.AssistTranslate(r.Context(), user.UserID, input)
	if err != nil {
		writeAssistError(w, "AI translation failed", err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleGenerateTags handles POST /api/v1/ai/assist/generate-tags — generate content tags.
// Request body: { "title": "...", "text": "..." }.
func (h *Handlers) HandleGenerateTags(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.requireAIAdmission(r.Context(), w, user.UserID, GateAssist) {
		return
	}

	var input AssistTagsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Title == "" && input.Text == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "title or text is required")

		return
	}

	if !h.requireAIBudget(r.Context(), w, user.UserID) {
		return
	}

	resp, err := h.AssistGenerateTags(r.Context(), user.UserID, input)
	if err != nil {
		writeAssistError(w, "AI tag generation failed", err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleGenerateOG handles POST /api/v1/ai/assist/generate-og — generate OG metadata.
// Request body: { "title": "...", "text": "...", "content_type": "post" }.
func (h *Handlers) HandleGenerateOG(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.requireAIAdmission(r.Context(), w, user.UserID, GateAssist) {
		return
	}

	var input AssistOGInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "title is required")

		return
	}

	if !h.requireAIBudget(r.Context(), w, user.UserID) {
		return
	}

	resp, err := h.AssistGenerateOG(r.Context(), user.UserID, input)
	if err != nil {
		writeAssistError(w, "AI OG generation failed", err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// FieldSchema is an exported type.
type FieldSchema struct {
	Key     string        `json:"key"`
	Type    string        `json:"type"`
	Label   string        `json:"label"`
	Options []FieldOption `json:"options,omitempty"`
}

// FieldOption is an exported type.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// DraftOptions lets users tailor the generated body.
type DraftOptions struct {
	MinWords                *int   `json:"min_words,omitempty"`
	ParagraphCount          *int   `json:"paragraph_count,omitempty"`
	HeadingCount            *int   `json:"heading_count,omitempty"`
	MinParagraphsPerHeading *int   `json:"min_paragraphs_per_heading,omitempty"`
	ExternalLinks           *int   `json:"external_links,omitempty"`
	InternalLinks           *int   `json:"internal_links,omitempty"`
	Mode                    string `json:"mode,omitempty"`
	Framework               string `json:"framework,omitempty"`
	IncludeIntro            bool   `json:"include_intro,omitempty"`
	IncludeConclusion       bool   `json:"include_conclusion,omitempty"`
	IncludeFAQs             bool   `json:"include_faqs,omitempty"`
	IncludeInlineImage      bool   `json:"include_inline_image,omitempty"`
	IncludeSteps            bool   `json:"include_steps,omitempty"`
	IncludeKeyTakeaways     bool   `json:"include_key_takeaways,omitempty"`
}

// DraftInput is an exported type.
type DraftInput struct {
	ContentType   string         `json:"content_type"`
	Title         string         `json:"title"`
	ExistingBody  string         `json:"existing_body"`
	Instructions  string         `json:"instructions"`
	Tone          string         `json:"tone"`
	Humanize      *bool          `json:"humanize,omitempty"`
	HumanizeLevel string         `json:"humanize_level,omitempty"`
	FieldSchema   []FieldSchema  `json:"field_schema,omitempty"`
	Context       map[string]any `json:"context,omitempty"`
	Options       DraftOptions   `json:"options,omitempty"`
	VoiceID       string         `json:"voice_id,omitempty"`
	Model         string         `json:"model,omitempty"`
}

// DraftResult is an exported type.
type DraftResult struct {
	Title       string         `json:"title"`
	BodyHTML    string         `json:"body_html"`
	FieldValues map[string]any `json:"field_values"`
	TokensUsed  int            `json:"tokens_used"`
}

// HandleDraft handles POST /api/v1/ai/assist/draft — generate or improve a body
// and optionally fill custom fields. Thin adapter over AssistDraft — the
// shared service also backs the content.generate_draft agent tool.
func (h *Handlers) HandleDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.requireAIAdmission(r.Context(), w, user.UserID, GateAssist) {
		return
	}

	var input DraftInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.ContentType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "content_type is required")
		return
	}

	if !h.requireAIBudget(r.Context(), w, user.UserID) {
		return
	}

	result, err := h.AssistDraft(r.Context(), user.UserID, input)
	if err != nil {
		writeAssistError(w, "AI draft failed", err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// normalizeFieldValues coerces AI-generated field values so they match the
// expected schema types and select options. This reduces UI warnings and
// makes sure select boxes actually show a selected value.
func normalizeFieldValues(values map[string]any, schema []FieldSchema) map[string]any {
	if values == nil {
		return map[string]any{}
	}

	schemaByKey := make(map[string]FieldSchema, len(schema))
	for _, f := range schema {
		schemaByKey[f.Key] = f
	}

	normalized := make(map[string]any, len(values))

	for key, v := range values {
		field, ok := schemaByKey[key]
		if !ok {
			normalized[key] = v
			continue
		}

		switch field.Type {
		case "select":
			s, _ := v.(string)
			if s == "" {
				normalized[key] = v
				continue
			}

			if match := matchSelectOption(s, field.Options); match != "" {
				normalized[key] = match
			} else {
				normalized[key] = v
			}
		case "boolean":
			normalized[key] = toBool(v)
		case "number", "currency":
			if n, ok := toNumber(v); ok {
				normalized[key] = n
			} else {
				normalized[key] = v
			}
		default:
			normalized[key] = v
		}
	}

	return normalized
}

// matchSelectOption returns the option value that best matches the input
// string. It tries exact, case-insensitive, substring, acronym, and then
// Levenshtein distance.
func matchSelectOption(input string, options []FieldOption) string {
	if input == "" {
		return ""
	}

	inputLower := strings.ToLower(strings.TrimSpace(input))

	var (
		bestMatch string
		bestScore int
	)

	const maxDistance = 3

	for _, opt := range options {
		optVal := strings.TrimSpace(opt.Value)
		optValLower := strings.ToLower(optVal)

		// Exact (case-insensitive).
		if optValLower == inputLower {
			return opt.Value
		}

		// Option value contains the input (e.g. "kingdom" in "United Kingdom").
		if strings.Contains(optValLower, inputLower) {
			return opt.Value
		}

		// Input contains the option value (e.g. "United Kingdom" in a longer phrase).
		if strings.Contains(inputLower, optValLower) && len(optValLower) > bestScore {
			bestMatch = opt.Value
			bestScore = len(optValLower)

			continue
		}

		// Acronym of option words (e.g. "UK" for "United Kingdom", "US" for "United States").
		if acronym := optionAcronym(optValLower); acronym == inputLower {
			return opt.Value
		}
	}

	if bestMatch != "" {
		return bestMatch
	}

	// No good substring/acronym match; try Levenshtein distance.
	for _, opt := range options {
		optValLower := strings.ToLower(strings.TrimSpace(opt.Value))
		if d := levenshteinDistance(inputLower, optValLower); d <= maxDistance && (bestMatch == "" || d < bestScore) {
			bestMatch = opt.Value
			bestScore = d
		}
	}

	return bestMatch
}

// optionAcronym returns the first letter of each word in a select option,
// lowercased and without separators. "United Kingdom" -> "uk".
func optionAcronym(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '&'
	})

	var b strings.Builder

	for _, w := range words {
		if w == "" {
			continue
		}

		rw := []rune(w)
		b.WriteRune(unicode.ToLower(rw[0]))
	}

	return b.String()
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		s := strings.ToLower(strings.TrimSpace(val))
		return s == "true" || s == "yes" || s == "1" || s == "on"
	case int, int8, int16, int32, int64:
		return val != 0
	case uint, uint8, uint16, uint32, uint64:
		return val != 0
	case float32, float64:
		return val != 0
	default:
		return false
	}
}

func toNumber(v any) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case float32:
		return float64(val), true
	case float64:
		return val, true
	case string:
		// Strip common currency/unit noise and parse.
		s := strings.TrimSpace(val)
		s = strings.TrimPrefix(s, "$")
		s = strings.TrimPrefix(s, "£")
		s = strings.TrimPrefix(s, "€")
		s = strings.TrimSuffix(s, "USD")
		s = strings.TrimSuffix(s, "GBP")
		s = strings.TrimSpace(s)

		if s == "" {
			return 0, true
		}

		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return n, true
		}
	}

	return 0, false
}

// levenshteinDistance computes the minimum number of single-character edits
// required to change one string into the other.
func levenshteinDistance(a, b string) int {
	if a == "" {
		return len(b)
	}

	if b == "" {
		return len(a)
	}

	// Convert to rune slices for Unicode safety.
	ra, rb := []rune(a), []rune(b)
	m, n := len(ra), len(rb)

	prev := make([]int, n+1)
	for j := 0; j <= n; j++ {
		prev[j] = j
	}

	curr := make([]int, n+1)

	for i := 1; i <= m; i++ {
		curr[0] = i

		for j := 1; j <= n; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}

			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}

		prev, curr = curr, prev
	}

	return prev[n]
}

func buildDraftPrompt(input DraftInput) string {
	var b strings.Builder

	b.WriteString("You are a content editor for an immigration community platform.\n")
	b.WriteString("Content type: " + input.ContentType + "\n")

	if input.Title != "" {
		b.WriteString("Current title: " + input.Title + "\n")
	}

	if input.ExistingBody != "" {
		b.WriteString("Existing body: " + input.ExistingBody + "\n")
	}

	b.WriteString("Tone: " + input.Tone + "\n")

	if input.Instructions != "" {
		b.WriteString("Instructions: " + input.Instructions + "\n")
	}

	// Tone override.
	if input.Tone != "" {
		b.WriteString(fmt.Sprintf("Tone: %s.\n", input.Tone))
	}

	// Content mode (generate / improve / extend / shorten).
	opt := input.Options
	switch opt.Mode {
	case "improve":
		b.WriteString("Mode: improve. Rewrite and improve the existing body while keeping its meaning.\n")
	case "extend":
		b.WriteString("Mode: extend. Expand and add more detail to the existing body.\n")
	case "shorten":
		b.WriteString("Mode: shorten. Condense the existing body while keeping the key points.\n")
	default:
		b.WriteString("Mode: generate. Create a new body from the title and context.\n")
	}

	// Marketing / copywriting framework override.
	switch strings.ToLower(opt.Framework) {
	case "aida":
		b.WriteString("Marketing framework: AIDA. Structure the output in four clear stages: Attention (hook the reader), Interest (build relevance), Desire (create emotional/informational want), and Action (clear next step or CTA).\n")
	case "pas":
		b.WriteString("Marketing framework: PAS. Structure the output in three clear stages: Problem (identify the reader's pain point), Agitation (amplify the problem and its stakes), and Solution (present the answer and desired outcome).\n")
	}

	// Content tailoring options.
	if opt.MinWords != nil && *opt.MinWords > 0 {
		b.WriteString(fmt.Sprintf("Minimum word count: %d\n", *opt.MinWords))
	}

	if opt.ParagraphCount != nil && *opt.ParagraphCount > 0 {
		b.WriteString(fmt.Sprintf("Number of paragraphs: %d\n", *opt.ParagraphCount))
	}

	if opt.HeadingCount != nil && *opt.HeadingCount > 0 {
		b.WriteString(fmt.Sprintf("Include at least %d section headings (excluding introduction, conclusion, FAQs, and key takeaways).\n", *opt.HeadingCount))
	}

	if opt.MinParagraphsPerHeading != nil && *opt.MinParagraphsPerHeading > 0 {
		b.WriteString(fmt.Sprintf("Minimum paragraphs per heading section: %d\n", *opt.MinParagraphsPerHeading))
	}

	if opt.ExternalLinks != nil && *opt.ExternalLinks > 0 {
		b.WriteString(fmt.Sprintf("Include at least %d external links to authority domains (e.g. gov.uk, nhs.uk, official bank or university sites) as HTML <a href=\"...\"> anchor tags.\n", *opt.ExternalLinks))
	}

	if opt.InternalLinks != nil && *opt.InternalLinks > 0 {
		b.WriteString(fmt.Sprintf("Include at least %d internal links to related content using relevant keywords as anchor text, as HTML <a href=\"/...\"> anchor tags.\n", *opt.InternalLinks))
	}

	if opt.IncludeIntro {
		b.WriteString("Include an introduction section.\n")
	}

	if opt.IncludeConclusion {
		b.WriteString("Include a conclusion section.\n")
	}

	if opt.IncludeFAQs {
		b.WriteString("Include a FAQ section at the end.\n")
	}

	if opt.IncludeInlineImage {
		b.WriteString("Include an inline image placeholder with a descriptive alt attribute and a relevant src path.\n")
	}

	if opt.IncludeSteps {
		b.WriteString("Format the content as a clear numbered or bulleted step-by-step guide.\n")
	}

	if opt.IncludeKeyTakeaways {
		b.WriteString("Include a key takeaways section at the end.\n")
	}

	if len(input.FieldSchema) > 0 {
		b.WriteString("Also return values for these fields as JSON under field_values:\n")

		for _, f := range input.FieldSchema {
			b.WriteString("- " + f.Key + " (" + f.Type + "): " + f.Label + "\n")
		}

		b.WriteString("For 'number' and 'currency' fields, return only a numeric value (no currency symbols or units). For 'boolean' fields, return true or false. For 'select' fields, use one of the provided option values.\n")
	}

	b.WriteString("Return a JSON object with keys: title (optional), body_html (HTML, not plain text), field_values (optional). Do not include markdown code fences and do not include any chain-of-thought or reasoning text.\n")

	return b.String()
}

// extractTrailingJSONObject finds the last top-level JSON object in a string
// and returns it. This is useful for reasoning models that output a chain-of-
// thought narrative followed by a final JSON payload. Returns the empty string
// if no valid object is found.
func extractTrailingJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// Find the last '}' in the string.
	end := strings.LastIndex(text, "}")
	if end == -1 {
		return ""
	}

	// Scan backwards to find the matching '{'.
	nest := 0
	start := -1

	for i := end; i >= 0; i-- {
		c := text[i]
		if c == '}' {
			nest++
		} else if c == '{' {
			nest--
			if nest == 0 {
				start = i
				break
			}
		}
	}

	if start == -1 {
		return ""
	}

	candidate := text[start : end+1]

	var obj map[string]any

	if err := json.Unmarshal([]byte(candidate), &obj); err == nil {
		return candidate
	}

	return ""
}

// plainTextToHTML converts a block of plain text into a minimal HTML
// paragraph structure. Double line breaks become <p> boundaries, single
// line breaks become <br> so it renders in a WYSIWYG editor.
func plainTextToHTML(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	// If the text already looks like HTML, leave it alone.
	if strings.HasPrefix(text, "<") {
		return text
	}

	paragraphs := strings.Split(text, "\n\n")
	for i, p := range paragraphs {
		p = strings.TrimSpace(p)
		p = strings.ReplaceAll(p, "\n", "<br>")

		if p != "" {
			paragraphs[i] = "<p>" + p + "</p>"
		} else {
			paragraphs[i] = ""
		}
	}

	return strings.Join(paragraphs, "")
}

// firstReasoningText returns the reasoning text from the first choice, if any.
func firstReasoningText(resp Response) string {
	if resp.Raw == nil {
		return ""
	}

	if choices, ok := resp.Raw["choices"].([]openAIChoice); ok && len(choices) > 0 {
		return choices[0].Message.Reasoning
	}

	if choicesRaw, ok := resp.Raw["choices"].([]any); ok && len(choicesRaw) > 0 {
		if choice, ok := choicesRaw[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if r, ok := msg["reasoning"].(string); ok {
					return r
				}
			}
		}
	}

	return ""
}

func parseDraftResponse(resp Response) (DraftResult, error) {
	result := DraftResult{Title: "", BodyHTML: "", FieldValues: map[string]any{}, TokensUsed: 0}

	raw := mapGenerateResult(resp)
	if v, ok := raw["tokens_used"].(int); ok {
		result.TokensUsed = v
	}

	content := raw["generated_text"].(string)

	// Some providers (e.g. Tencent/Hy3) set a degenerate/empty content
	// string while putting the actual output in the reasoning field. If the
	// primary content is empty or unhelpful, try the reasoning trace.
	if content == "" {
		content = firstReasoningText(resp)
	}

	if content == "" {
		return result, nil
	}

	// Some providers wrap JSON in markdown code fences; strip them before parsing.
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Try to parse the generated text as a JSON object. If that fails,
	// treat the whole content as body_html.
	var parsed struct {
		Title       string         `json:"title"`
		Body        string         `json:"body"`
		BodyHTML    string         `json:"body_html"`
		Content     string         `json:"content"`
		FieldValues map[string]any `json:"field_values"`
	}

	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		result.Title = parsed.Title
		if parsed.BodyHTML != "" {
			result.BodyHTML = parsed.BodyHTML
		} else if parsed.Body != "" {
			result.BodyHTML = plainTextToHTML(parsed.Body)
		} else if parsed.Content != "" {
			result.BodyHTML = plainTextToHTML(parsed.Content)
		}

		result.FieldValues = parsed.FieldValues
	} else if trailing := extractTrailingJSONObject(content); trailing != "" {
		// Reasoning models often emit a chain-of-thought followed by a final
		// JSON object. Try to extract and use that object.
		if err := json.Unmarshal([]byte(trailing), &parsed); err == nil {
			result.Title = parsed.Title
			if parsed.BodyHTML != "" {
				result.BodyHTML = parsed.BodyHTML
			} else if parsed.Body != "" {
				result.BodyHTML = plainTextToHTML(parsed.Body)
			} else if parsed.Content != "" {
				result.BodyHTML = plainTextToHTML(parsed.Content)
			}

			result.FieldValues = parsed.FieldValues
		} else {
			result.BodyHTML = plainTextToHTML(content)
		}
	} else {
		result.BodyHTML = plainTextToHTML(content)
	}

	// If the primary content parsed to nothing useful, try the reasoning
	// trace as a fallback. Some providers put a degenerate content JSON
	// (e.g. {"": ""}) while the actual output is in message.reasoning.
	if result.Title == "" && result.BodyHTML == "" && len(result.FieldValues) == 0 {
		if reasoning := firstReasoningText(resp); reasoning != "" {
			// Reset and re-parse using the reasoning trace.
			result = DraftResult{Title: "", BodyHTML: "", FieldValues: map[string]any{}, TokensUsed: result.TokensUsed}
			reasoning = strings.TrimSpace(reasoning)
			reasoning = strings.TrimPrefix(reasoning, "```json")
			reasoning = strings.TrimPrefix(reasoning, "```")
			reasoning = strings.TrimSuffix(reasoning, "```")
			reasoning = strings.TrimSpace(reasoning)

			if err := json.Unmarshal([]byte(reasoning), &parsed); err == nil {
				result.Title = parsed.Title
				if parsed.BodyHTML != "" {
					result.BodyHTML = parsed.BodyHTML
				} else if parsed.Body != "" {
					result.BodyHTML = plainTextToHTML(parsed.Body)
				} else if parsed.Content != "" {
					result.BodyHTML = plainTextToHTML(parsed.Content)
				}

				result.FieldValues = parsed.FieldValues
			} else if trailing := extractTrailingJSONObject(reasoning); trailing != "" {
				if err := json.Unmarshal([]byte(trailing), &parsed); err == nil {
					result.Title = parsed.Title
					if parsed.BodyHTML != "" {
						result.BodyHTML = parsed.BodyHTML
					} else if parsed.Body != "" {
						result.BodyHTML = plainTextToHTML(parsed.Body)
					} else if parsed.Content != "" {
						result.BodyHTML = plainTextToHTML(parsed.Content)
					}

					result.FieldValues = parsed.FieldValues
				}
			}
		}
	}

	if result.FieldValues == nil {
		result.FieldValues = map[string]any{}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// HITL Handlers
// ---------------------------------------------------------------------------

// decodeAIOutput converts a JSONB ai_output blob to a display string. If the
// blob is a JSON string literal (e.g. "some text"), the unquoted string is
// returned. Otherwise the raw JSON is returned as a string.
func decodeAIOutput(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	// Try to unwrap a JSON string literal first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Not a string literal — return the raw JSON text.
	return string(raw)
}

// HandleCreateHITL creates a new HITL review for AI output. Accepts two
// request formats:
// - Legacy: {content_id, content_type, ai_output} — used when a content row
// already exists.
// - Admin generator: {content, content_type, source} — used by the admin
// "Send to Review" feature which submits generated text directly without
// a backing content row (content_id is nullable per migration 000275).
func (h *Handlers) HandleCreateHITL(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentID   string `json:"content_id"`
		ContentType string `json:"content_type"`
		AIOutput    any    `json:"ai_output"`
		// Admin generator fields (frontend aiApi.createHitlReview).
		Content string `json:"content"`
		Source  string `json:"source"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.ContentType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "content_type is required")

		return
	}

	// Resolve the content_id — optional when using the admin generator format.
	var contentID pgtype.UUID
	if input.ContentID != "" {
		if err := contentID.Scan(input.ContentID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid content_id")

			return
		}
	}

	// Resolve ai_output: prefer the explicit ai_output field, fall back to
	// the content string from the admin generator format.
	var aiOutputBytes []byte

	if input.AIOutput != nil {
		var err error

		aiOutputBytes, err = json.Marshal(input.AIOutput)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid ai_output")

			return
		}
	} else if input.Content != "" {
		aiOutputBytes, _ = json.Marshal(input.Content)
	}

	if len(aiOutputBytes) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "either ai_output or content is required")

		return
	}

	review, err := CreateHITLReview(r.Context(), h.Store, HITLReview{

		ContentID:   contentID,
		ContentType: input.ContentType,
		AIOutput:    aiOutputBytes,
	})
	if err != nil {
		slog.Error("creating HITL review", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create HITL review")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": review})
}

// HandleListPendingHITL lists pending HITL reviews for the current platform.
func (h *Handlers) HandleListPendingHITL(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	sourceFilter := r.URL.Query().Get("source")

	reviews, err := ListPendingReviews(r.Context(), h.Store, sourceFilter, limit, offset)
	if err != nil {
		slog.Error("listing pending HITL reviews", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list reviews")

		return
	}

	total, err := CountPendingReviews(r.Context(), h.Store, sourceFilter)
	if err != nil {
		slog.Error("counting pending HITL reviews", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count reviews")

		return
	}

	// Map to the frontend-friendly DTO. The frontend HitlReview interface
	// expects {id, content, content_type, source, status, created_at, ...}.
	// The backend stores the AI output in ai_output (JSONB); decode it to a
	// string for the content field.
	type hitlReviewDTO struct {
		ID          string `json:"id"`
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
		Source      string `json:"source"`
		Status      string `json:"status"`
		CreatedAt   string `json:"created_at"`
		ReviewedAt  string `json:"reviewed_at,omitempty"`
		ReviewerID  string `json:"reviewer_id,omitempty"`
		Reason      string `json:"reason,omitempty"`
	}

	dtos := make([]hitlReviewDTO, 0, len(reviews))

	for _, rev := range reviews {
		dto := hitlReviewDTO{
			ID:          rev.ID.String(),
			Content:     decodeAIOutput(rev.AIOutput),
			ContentType: rev.ContentType,
			Source:      rev.Source,
			Status:      rev.Status,
			CreatedAt:   rev.CreatedAt.Format(time.RFC3339),
		}
		if rev.ReviewedAt != nil {
			dto.ReviewedAt = rev.ReviewedAt.Format(time.RFC3339)
		}

		if rev.ReviewerID.Valid {
			dto.ReviewerID = rev.ReviewerID.String()
		}

		if rev.Reason != "" {
			dto.Reason = rev.Reason
		}

		dtos = append(dtos, dto)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"reviews": dtos,
			"total":   total,
		},
	})
}

// HandleApproveHITL approves a pending HITL review. The review is scoped to
// the caller's platform to prevent cross-platform approve (Section 09).
func (h *Handlers) HandleApproveHITL(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var reviewID pgtype.UUID
	if err := reviewID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid review ID")

		return
	}

	review, err := h.Store.GetHITLReview(r.Context(), reviewID)
	if err != nil {
		slog.Error("fetching HITL review before approve", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")

		return
	}

	// For studio-sourced reviews, run the studio hook first and only commit
	// the review status if the hook succeeds (BUG-004). This prevents a
	// published/rejected studio job from being paired with a review that never
	// actually transitioned due to a hook failure.
	if review.Source == HITLSourceStudio && h.StudioReviewHook != nil {
		if err := h.OnStudioReviewApproved(r.Context(), reviewID, user.UserID); err != nil {
			slog.Error("studio review approve hook failed", "review_id", reviewID, "error", err)
			// Surface the underlying publish failure (e.g. missing primary
			// space) so the reviewer can fix it instead of guessing why the
			// approval cannot complete.
			errpkg.WriteError(w, http.StatusBadGateway, "studio_hook_failed",
				fmt.Sprintf("The studio publish step failed: %v. The review has not been approved", err))

			return
		}
	}

	// Non-studio reviews carrying a publishable target (agent_draft_request
	// drafts, manual submissions) publish on approve — §103 "Approve →
	// publish". Fail closed like the studio hook: a publish failure leaves
	// the review pending instead of reporting an approved-but-unpublished
	// draft.
	if review.Source != HITLSourceStudio && review.ContentID.Valid && h.DraftPublishHook != nil {
		if err := h.PublishHITLTarget(r.Context(), user.UserID, review.TargetTable, review.ContentID); err != nil {
			slog.Error("draft publish hook failed", "review_id", reviewID, "content_id", review.ContentID, "target_table", review.TargetTable, "error", err)
			errpkg.WriteError(w, http.StatusBadGateway, "publish_failed",
				fmt.Sprintf("The publish step failed: %v. The review has not been approved", err))

			return
		}
	}

	if err := ApproveReview(r.Context(), h.Store, reviewID, user.UserID); err != nil {
		slog.Error("approving HITL review", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "approved"}})
}

// HandleRejectHITL rejects a pending HITL review with a reason. The review is
// scoped to the caller's platform to prevent cross-platform reject (Section 09).
func (h *Handlers) HandleRejectHITL(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var reviewID pgtype.UUID
	if err := reviewID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "invalid review ID")

		return
	}

	var input struct {
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	review, err := h.Store.GetHITLReview(r.Context(), reviewID)
	if err != nil {
		slog.Error("fetching HITL review before reject", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")

		return
	}

	// For studio-sourced reviews, run the studio hook first and only commit
	// the review status if the hook succeeds (BUG-004).
	if review.Source == HITLSourceStudio && h.StudioReviewHook != nil {
		if err := h.OnStudioReviewRejected(r.Context(), reviewID, user.UserID); err != nil {
			slog.Error("studio review reject hook failed", "review_id", reviewID, "error", err)
			errpkg.WriteError(w, http.StatusBadGateway, "studio_hook_failed", "The studio reject step failed; the review has not been rejected")

			return
		}
	}

	if err := RejectReview(r.Context(), h.Store, reviewID, user.UserID, input.Reason); err != nil {
		slog.Error("rejecting HITL review", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "rejected"}})
}

// ---------------------------------------------------------------------------
// HITL Verification Handler
// ---------------------------------------------------------------------------

// HandleVerifyHITL grounds a claim using the platform's configured external
// search provider (Perplexity, Tavily, or Brave). It is intended for human
// reviewers to verify a factual claim in a guide, comparison, or other
// AI-generated content before approval.
// // POST /api/v1/ai/hitl/verify
// Body: { "claim": "...", "context": "..." }
// Response: { "data": { "answer": "...", "citations": ["..."], "provider": "perplexity" } }.
func (h *Handlers) HandleVerifyHITL(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Claim   string `json:"claim"`
		Context string `json:"context,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Claim == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_claim", "claim is required")
		return
	}

	if h.Settings == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "settings_unavailable", "Settings reader is not configured")
		return
	}

	client := h.httpClient()

	// Build a verification query from the claim and optional context.
	query := input.Claim
	if input.Context != "" {
		query = fmt.Sprintf("%s\nContext: %s", input.Claim, input.Context)
	}

	provider, results, err := SearchWeb(r.Context(), client, h.Settings, query, 5)
	if err != nil {
		slog.Warn("ai: HITL verification search failed", "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "verification_unavailable", "The verification service is temporarily unavailable")

		return
	}

	var answer string

	var citations []string

	if len(results) > 0 {
		answer = results[0].Content
		citations = results[0].Citations
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"claim":     input.Claim,
			"answer":    answer,
			"citations": citations,
			"provider":  provider,
		},
	})
}

// httpClient returns the handler's HTTP client, defaulting to
// http.DefaultClient. Used for external search calls from handlers.
func (h *Handlers) httpClient() *http.Client {
	if h.HTTPClient != nil {
		return h.HTTPClient
	}

	return http.DefaultClient
}

// ---------------------------------------------------------------------------
// Admin AI Context Handlers
// ---------------------------------------------------------------------------

// HandleGetUserContext returns the AI context entries for a specific user.
// GET /admin/users/{id}/ai-context (catalog route).
// Reads the user id from either the {user_id} or {id} chi param so the
// handler works against both the catalog registration and any legacy drift
// registration without breaking.
func (h *Handlers) HandleGetUserContext(w http.ResponseWriter, r *http.Request) {
	if h.Honcho == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		userID = chi.URLParam(r, "id")
	}

	if userID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "user_id is required")

		return
	}

	// Platform scoping (§9, ADR-027): verify the target user belongs to the
	// admin's platform before exposing their Honcho memory. Prevents cross-platform
	// AI-context access.
	if h.Store == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "store_unavailable", "User store not configured")
		return
	}

	ok, err := h.Store.UserExists(r.Context(), userID)
	if err != nil {
		slog.Error("verifying user existence", "error", err, "user_id", userID)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify user")

		return
	}

	if !ok {
		errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	entries, err := h.Honcho.GetContext(r.Context(), userID)
	if err != nil {
		slog.Error("getting user AI context", "error", err, "user_id", userID)
		errpkg.WriteError(w, http.StatusBadGateway, "honcho_error", "Failed to retrieve user context")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": entries,
		"meta": map[string]any{"total_count": len(entries)},
	})
}

// HandleDeleteUserContextEntry deletes a specific AI context entry for a user.
// DELETE /admin/users/{id}/ai-context/{entryId} (catalog route).
// Reads params from either {user_id}/{entry_id} or {id}/{entryId} chi params.
func (h *Handlers) HandleDeleteUserContextEntry(w http.ResponseWriter, r *http.Request) {
	if h.Honcho == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "honcho_unavailable", "Honcho client not configured")

		return
	}

	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		userID = chi.URLParam(r, "id")
	}

	if userID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "user_id is required")

		return
	}

	entryID := chi.URLParam(r, "entry_id")
	if entryID == "" {
		entryID = chi.URLParam(r, "entryId")
	}

	if entryID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "entry_id is required")

		return
	}

	// Platform scoping (§9, ADR-027): verify the target user belongs to the
	// admin's platform before deleting their Honcho memory entry.
	if h.Store == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "store_unavailable", "User store not configured")
		return
	}

	ok, err := h.Store.UserExists(r.Context(), userID)
	if err != nil {
		slog.Error("verifying user existence", "error", err, "user_id", userID)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify user")

		return
	}

	if !ok {
		errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	if err := h.Honcho.DeleteEntry(r.Context(), userID, entryID); err != nil {
		slog.Error("deleting user context entry", "error", err, "user_id", userID, "entry_id", entryID)
		errpkg.WriteError(w, http.StatusBadGateway, "honcho_error", "Failed to delete context entry")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Admin AI Usage Handler (Section 20 — admin AI usage monitoring)
// ---------------------------------------------------------------------------

// HandleGetAIUsage returns aggregated AI usage for the platform.
// GET /admin/ai/usage?period=7d|30d&provider=openai|ollama.
// Returns the Section 20 contract: { total_cost_usd, tokens_used, by_task_type }.
func (h *Handlers) HandleGetAIUsage(w http.ResponseWriter, r *http.Request) {

	periodDays := 30

	if p := r.URL.Query().Get("period"); p != "" {
		switch p {
		case "7d":
			periodDays = 7
		case "30d":
			periodDays = 30
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "period must be '7d' or '30d'")
			return
		}
	}

	provider := r.URL.Query().Get("provider") // optional filter

	summary, err := h.Store.GetAIUsageSummary(r.Context(), periodDays, provider)
	if err != nil {
		slog.Error("getting ai usage summary", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve AI usage")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": summary})
}

// ---------------------------------------------------------------------------
// AI Models Handler
// ---------------------------------------------------------------------------

// Model represents an available AI model for the admin UI.
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// HandleListModels returns the list of available AI models.
// GET /admin/ai/models[?category=embedding]. Models are derived from the
// providers actually registered with the dispatcher (Rule 16: no
// hardcoded/mock data). A "Default" entry (empty id) is always present for
// auto-selection. When the optional ?category= query param is supplied, only
// models from providers registered for that category are returned (e.g.
// ?category=embedding returns only embedding-capable models).
func (h *Handlers) HandleListModels(w http.ResponseWriter, r *http.Request) {
	models := []Model{{ID: "", Label: "Default"}}

	if h.Dispatcher != nil {
		category := r.URL.Query().Get("category")

		var registered []Model

		if category != "" {
			registered = h.Dispatcher.RegisteredModelsByCategory(category)
		} else {
			registered = h.Dispatcher.RegisteredModels()
		}

		slog.Info("ai: HandleListModels registered", "category", category, "count", len(registered))

		models = append(models, registered...)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": models})
}

// ─── P5.6 AI Conversations ───────────────────────────────────────────────

// HandleListConversations handles GET /ai/conversations (P5.6).
// Returns the user's JapaBot conversation history, grouped by conversation_id
// from Honcho context entries (Section 20). Each conversation summary includes
// the conversation_id, the latest message preview, and the turn count. Returns
// an empty list when Honcho is not configured or the user has no entries.
// Supports ?page_size and ?page_offset pagination (BUG-016).
func (h *Handlers) HandleListConversations(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return
	}

	if h.Honcho == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	pageSize := parsePageSize(r)
	pageOffset := parsePageOffset(r)

	entries, err := h.Honcho.GetContext(r.Context(), user.UserID.String())
	if err != nil {
		// Honcho may be unreachable — degrade to an empty list rather than 500.
		slog.Warn("ai: listing conversations from honcho", "error", err)
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})

		return
	}

	// Group entries by conversation_id (metadata) into conversation summaries.
	// The list DTO intentionally does not include full entries (BUG-005); the
	// detail endpoint /ai/conversations/:id returns the complete turn history.
	type convoSummary struct {
		ConversationID string    `json:"conversation_id"`
		Preview        string    `json:"preview"`
		TurnCount      int       `json:"turn_count"`
		LastEntryAt    time.Time `json:"last_entry_at"`
	}

	convos := map[string]*convoSummary{}

	for _, e := range entries {
		cid := ""

		if e.Metadata != nil {
			if v, ok := e.Metadata["conversation_id"].(string); ok {
				cid = v
			}
		}

		if cid == "" {
			// Entries without a conversation_id are standalone context rows;
			// surface them as their own pseudo-conversation keyed by entry id.
			cid = e.ID
		}

		c, exists := convos[cid]
		if !exists {
			c = &convoSummary{ConversationID: cid}
			convos[cid] = c
		}

		c.TurnCount++
		if e.CreatedAt.After(c.LastEntryAt) {
			c.LastEntryAt = e.CreatedAt
			c.Preview = e.Content
		}
	}

	out := make([]convoSummary, 0, len(convos))
	for _, c := range convos {
		out = append(out, *c)
	}

	// Sort by most recent activity first, then apply offset/limit pagination.
	sort.Slice(out, func(i, j int) bool { return out[i].LastEntryAt.After(out[j].LastEntryAt) })

	total := int64(len(out))

	if int(pageOffset) >= len(out) {
		out = out[:0]
	} else {
		out = out[pageOffset:]
		if int(pageSize) > 0 && int(pageSize) < len(out) {
			out = out[:pageSize]
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{
			"total":       total,
			"page_size":   pageSize,
			"page_offset": pageOffset,
		},
	})
}

// HandleGetConversation handles GET /ai/conversations/:id (P5.6).
// Returns all Honcho context entries belonging to the conversation_id given
// in :id. Returns 404 when the conversation has no entries.
func (h *Handlers) HandleGetConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return
	}

	conversationID := chi.URLParam(r, "id")
	if conversationID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "conversation id is required")
		return
	}

	if h.Honcho == nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Conversation not found")
		return
	}

	entries, err := h.Honcho.GetContext(r.Context(), user.UserID.String())
	if err != nil {
		slog.Warn("ai: loading conversations from honcho", "error", err)
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Conversation not found")

		return
	}

	// Collect all entries whose metadata.conversation_id matches the requested
	// id. Entries that lack a conversation_id are surfaced in the list response
	// as pseudo-conversations keyed by entry id — match on entry id as a
	// fallback so those stay openable.
	var matched []honcho.ContextEntry

	for _, e := range entries {
		if e.Metadata != nil {
			if cid, ok := e.Metadata["conversation_id"].(string); ok && cid == conversationID {
				matched = append(matched, e)
				continue
			}
		}

		if e.ID == conversationID {
			matched = append(matched, e)
		}
	}

	if len(matched) == 0 {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Conversation not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"conversation_id": conversationID,
		"entries":         matched,
		"turn_count":      len(matched),
	}})
}

// humanizeCategory derives a display label and short description from a
// dispatcher category slug. It keeps HandleListCategories free of hardcoded
// label lists (Rule 16); the only source of truth is the provider registration.
func humanizeCategory(cat string) (label, description string) {
	// Replace common separators and title-case each word.
	words := strings.FieldsFunc(cat, func(r rune) bool { return r == '_' || r == '-' })
	for i, w := range words {
		if len(w) == 0 {
			continue
		}

		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}

	label = strings.Join(words, " ")
	if label == "" {
		label = cat
	}

	description = label

	return label, description
}

// HandleListCategories handles GET /ai/categories — returns available AI
// categories derived from the providers actually registered with the
// dispatcher (Rule 16: no hardcoded/mock data).
func (h *Handlers) HandleListCategories(w http.ResponseWriter, _ *http.Request) {
	categories := []map[string]string{}

	if h.Dispatcher != nil {
		for _, cat := range h.Dispatcher.RegisteredCategories() {
			label, description := humanizeCategory(cat)
			categories = append(categories, map[string]string{
				"value":       cat,
				"label":       label,
				"description": description,
			})
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": categories})
}

// resolveAcceptLanguage extracts the primary 2-letter language tag from the
// Accept-Language header for RAG retrieval filtering (ADR-069 §3 step 3).
// Returns "" when the header is absent so the retrieval language filter is
// skipped (backward-compatible with the pre-filter behavior).
func resolveAcceptLanguage(r *http.Request) string {
	accept := r.Header.Get("Accept-Language")
	if accept == "" {
		return ""
	}
	// "en-US,en;q=0.9,fr;q=0.8" → "en"
	parts := strings.SplitN(accept, ",", 2)

	tag := strings.TrimSpace(parts[0])
	if idx := strings.Index(tag, "-"); idx > 0 {
		tag = tag[:idx]
	}

	if len(tag) == 2 {
		return strings.ToLower(tag)
	}

	return ""
}
