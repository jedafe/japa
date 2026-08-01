// Package ai — handlers.go provides HTTP handlers for the AI Core system.
//
// Routes:
//
//	POST /api/v1/ai/generate                      — generate AI output via the gateway
//	POST /api/v1/ai/chat                           — JapaBot conversational AI
//	POST /api/v1/ai/assist/improve                 — improve writing quality
//	POST /api/v1/ai/assist/summarize               — summarize text
//	POST /api/v1/ai/assist/translate               — translate text
//	POST /api/v1/ai/assist/generate-tags           — generate content tags
//	POST /api/v1/ai/assist/generate-og             — generate OG metadata
//	POST /api/v1/ai/hitl                           — create HITL review
//	GET  /api/v1/ai/hitl/pending                   — list pending reviews
//	POST /api/v1/ai/hitl/{id}/approve              — approve a review
//	POST /api/v1/ai/hitl/{id}/reject               — reject a review with reason
//	GET  /api/v1/ai/conversations                  — list user conversations
//	GET  /api/v1/ai/conversations/{id}             — get a specific conversation
//	GET  /api/v1/ai/categories                     — list AI categories
//	GET  /admin/ai/context/{user_id}               — view user's AI context (Honcho)
//	DELETE /admin/ai/context/{user_id}/{entry_id}  — delete a specific context entry
//	GET  /admin/ai/usage                           — aggregated AI usage (Section 20)
//	GET  /admin/ai/models                          — list available AI models
package ai

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/honcho"
	"github.com/jedafe/japa/internal/tenant"
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
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// Handlers holds dependencies for AI HTTP handlers.
type Handlers struct {
	Dispatcher  *Dispatcher
	Store       *Store
	Honcho      *honcho.Client
	AsynqClient TaskEnqueueClient
	Settings    *config.Settings // hot-reloadable system_settings (ai.japabot_enabled, etc.)
	Perms       PermissionChecker
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

// NewHandlers creates Handlers with the given dependencies.
func NewHandlers(dispatcher *Dispatcher, store *Store) *Handlers {
	return &Handlers{Dispatcher: dispatcher, Store: store}
}

// SetHonchoClient injects the Honcho client for admin context operations.
func (h *Handlers) SetHonchoClient(c *honcho.Client) {
	h.Honcho = c
}

// SetSettings injects the hot-reloadable system_settings reader (Rule 3).
// Used by HandleChat to honor the `ai.japabot_enabled` global toggle.
func (h *Handlers) SetSettings(s *config.Settings) {
	h.Settings = s
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
//  1. Prior persistent memory from Honcho (Section 20) — retrieved via
//     h.Honcho.GetContext and included as "prior_memory".
//  2. Per-category site-activity context, gated by the ai.context.* nodes
//     (ADR-025). Each category is only included when the user holds the
//     corresponding permission node (all default OFF, including super_admin).
//
// Caller-supplied input.Context is merged last (lowest precedence) so the
// server remains the authority over what the AI can see.
func (h *Handlers) buildUserAIContext(ctx context.Context, userID, tenantID pgtype.UUID, clientContext map[string]any) map[string]any {
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
	if h.Perms != nil {
		enabled := map[string]bool{}
		for node := range aiContextCategories {
			if h.Perms.HasPermission(ctx, userID, tenantID, node) {
				enabled[aiContextCategories[node]] = true
			}
		}
		out["enabled_categories"] = enabled
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")

		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return nil, false
	}

	return user, true
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

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Category string         `json:"category"`
		Prompt   string         `json:"prompt"`
		Context  map[string]any `json:"context,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Category == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "category is required")

		return
	}

	if input.Prompt == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "prompt is required")

		return
	}

	// TASK-004: docs page generation is fire-and-forget. Enqueue a background
	// task and return 202 Accepted with the task reference instead of blocking
	// on the AI gateway call synchronously.
	if input.Category == "docs" {
		payload, err := json.Marshal(map[string]string{
			"prompt":    input.Prompt,
			"user_id":   user.UserID.String(),
			"tenant_id": tenantID.String(),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "marshal_error", "Failed to marshal task payload")
			return
		}

		if h.AsynqClient == nil {
			writeError(w, http.StatusServiceUnavailable, "no_worker", "Background task client unavailable")
			return
		}

		info, err := h.AsynqClient.Enqueue(
			asynq.NewTask(worker.TypeDocsGeneratePage, payload),
			asynq.Queue(worker.QueueForTask(worker.TypeDocsGeneratePage)),
		)
		if err != nil {
			slog.Error("ai: failed to enqueue docs:generate_page", "error", err)
			writeError(w, http.StatusBadGateway, "enqueue_error", "Failed to enqueue generation task")
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]any{
			"data": map[string]any{
				"status":   "accepted",
				"task_id":  info.ID,
				"task_type": worker.TypeDocsGeneratePage,
			},
		})
		return
	}

	payload := map[string]any{
		"prompt":    input.Prompt,
		"context":   input.Context,
		"user_id":   user.UserID.String(),
		"tenant_id": tenantID.String(),
	}

	resp, err := h.Dispatcher.Dispatch(r.Context(), input.Category, Request{
		Category: input.Category,
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai generate failed", "error", err, "category", input.Category)
		writeError(w, http.StatusBadGateway, "ai_error", "AI generation failed")

		return
	}

	// Enqueue async AI process task for background logging/analytics — fire-and-forget.
	h.enqueueAIProcessTask(input.Category, input.Prompt, "", "")

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Honor the global JapaBot toggle (system_settings 'ai.japabot_enabled',
	// default true). A disabled toggle short-circuits before any provider call.
	if h.Settings != nil {
		if enabled, err := h.Settings.GetSettingBool(r.Context(), tenantID, "ai.japabot_enabled"); err == nil && !enabled {
			writeError(w, http.StatusServiceUnavailable, "japabot_disabled", "JapaBot is currently disabled")
			return
		}
	}

	var input struct {
		Message        string         `json:"message"`
		ConversationID string         `json:"conversation_id,omitempty"`
		Context        map[string]any `json:"context,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Message == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "message is required")

		return
	}

	// Build the AI context payload (ADR-025): prior Honcho memory (Section 20)
	// + per-category site-activity context gated by ai.context.* nodes. The
	// server is the authority over what the AI can see — client-supplied
	// context is merged at lowest precedence.
	aiContext := h.buildUserAIContext(r.Context(), user.UserID, tenantID, input.Context)

	payload := map[string]any{
		"prompt":           input.Message,
		"context":          aiContext,
		"user_id":          user.UserID.String(),
		"tenant_id":        tenantID.String(),
		"task":             "chat",
		"conversation_id":  input.ConversationID,
	}

	resp, err := h.Dispatcher.Dispatch(r.Context(), "chat", Request{
		Category: "chat",
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai chat failed", "error", err)
		writeError(w, http.StatusBadGateway, "ai_error", "JapaBot is unavailable")

		return
	}

	// Shape the response to the Section 20 contract: { reply, conversation_id, sources? }.
	// The provider's text lives in resp.Raw; extract the reply text and echo the
	// conversation_id (existing or newly generated by the provider/Honcho).
	reply := extractReplyText(resp)
	conversationID := input.ConversationID
	if cid, ok := resp.Raw["conversation_id"].(string); ok && cid != "" {
		conversationID = cid
	}

	// Persist the chat turn to Honcho as persistent memory (Section 20).
	// Fire-and-forget: errors are logged, never propagated (Rule 8 spirit).
	h.persistChatTurn(r.Context(), user.UserID.String(), input.Message, reply, conversationID)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"reply":           reply,
			"conversation_id": conversationID,
			"provider_ref":    resp.ProviderRef,
			"sources":         resp.Raw["sources"],
		},
	})
}

// extractReplyText pulls the human-readable reply string out of a provider
// Response. OpenAI-compatible providers (Xiaomi MiMo) return choices[].message.content.
func extractReplyText(resp Response) string {
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
func (h *Handlers) HandleImprove(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Text string `json:"text"`
		Tone string `json:"tone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Text == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "text is required")

		return
	}

	tone := input.Tone
	if tone == "" {
		tone = "professional"
	}

	payload := map[string]any{
		"prompt":    "Improve the following text for clarity, grammar, and style. Tone: " + tone + ".\n\n" + input.Text,
		"user_id":   user.UserID.String(),
		"tenant_id": tenantID.String(),
		"task":      "improve",
		"tone":      tone,
	}

	resp, err := h.Dispatcher.Dispatch(r.Context(), "content_generation", Request{
		Category: "content_generation",
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai improve failed", "error", err)
		writeError(w, http.StatusBadGateway, "ai_error", "AI improvement failed")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleSummarize handles POST /api/v1/ai/assist/summarize — summarize text.
// Request body: { "text": "...", "max_words": 100 }.
func (h *Handlers) HandleSummarize(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Text     string `json:"text"`
		MaxWords int    `json:"max_words"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Text == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "text is required")

		return
	}

	maxWords := input.MaxWords
	if maxWords <= 0 {
		maxWords = 100
	}

	payload := map[string]any{
		"prompt":    "Summarize the following text in at most " + strconv.Itoa(maxWords) + " words.\n\n" + input.Text,
		"user_id":   user.UserID.String(),
		"tenant_id": tenantID.String(),
		"task":      "summarize",
		"max_words": maxWords,
	}

	resp, err := h.Dispatcher.Dispatch(r.Context(), "content_generation", Request{
		Category: "content_generation",
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai summarize failed", "error", err)
		writeError(w, http.StatusBadGateway, "ai_error", "AI summarization failed")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleTranslate handles POST /api/v1/ai/assist/translate — translate text.
// Request body: { "text": "...", "target_locale": "fr" }.
func (h *Handlers) HandleTranslate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Text         string `json:"text"`
		TargetLocale string `json:"target_locale"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Text == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "text is required")

		return
	}

	if input.TargetLocale == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "target_locale is required")

		return
	}

	payload := map[string]any{
		"prompt":        "Translate the following text to locale " + input.TargetLocale + ".\n\n" + input.Text,
		"user_id":       user.UserID.String(),
		"tenant_id":     tenantID.String(),
		"task":          "translate",
		"target_locale": input.TargetLocale,
	}

	resp, err := h.Dispatcher.Dispatch(r.Context(), "content_generation", Request{
		Category: "content_generation",
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai translate failed", "error", err)
		writeError(w, http.StatusBadGateway, "ai_error", "AI translation failed")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleGenerateTags handles POST /api/v1/ai/assist/generate-tags — generate content tags.
// Request body: { "title": "...", "text": "..." }.
func (h *Handlers) HandleGenerateTags(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Title == "" && input.Text == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "title or text is required")

		return
	}

	payload := map[string]any{
		"prompt":    "Generate relevant content tags for the following. Return as a JSON array of strings.\n\nTitle: " + input.Title + "\n\nText: " + input.Text,
		"user_id":   user.UserID.String(),
		"tenant_id": tenantID.String(),
		"task":      "generate_tags",
	}

	resp, err := h.Dispatcher.Dispatch(r.Context(), "content_generation", Request{
		Category: "content_generation",
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai generate-tags failed", "error", err)
		writeError(w, http.StatusBadGateway, "ai_error", "AI tag generation failed")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleGenerateOG handles POST /api/v1/ai/assist/generate-og — generate OG metadata.
// Request body: { "title": "...", "text": "...", "content_type": "post" }.
func (h *Handlers) HandleGenerateOG(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Title       string `json:"title"`
		Text        string `json:"text"`
		ContentType string `json:"content_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "title is required")

		return
	}

	contentType := input.ContentType
	if contentType == "" {
		contentType = "post"
	}

	payload := map[string]any{
		"prompt":       "Generate Open Graph metadata (og:title, og:description, og:image alt text) for the following " + contentType + ". Return as JSON.\n\nTitle: " + input.Title + "\n\nText: " + input.Text,
		"user_id":      user.UserID.String(),
		"tenant_id":    tenantID.String(),
		"task":         "generate_og",
		"content_type": contentType,
	}

	resp, err := h.Dispatcher.Dispatch(r.Context(), "content_generation", Request{
		Category: "content_generation",
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai generate-og failed", "error", err)
		writeError(w, http.StatusBadGateway, "ai_error", "AI OG generation failed")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// ---------------------------------------------------------------------------
// HITL Handlers
// ---------------------------------------------------------------------------

// HandleCreateHITL creates a new HITL review for AI output.
func (h *Handlers) HandleCreateHITL(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentID   string `json:"content_id"`
		ContentType string `json:"content_type"`
		AIOutput    any    `json:"ai_output"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid content_id")

		return
	}

	aiOutputBytes, err := json.Marshal(input.AIOutput)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid ai_output")

		return
	}

	review, err := CreateHITLReview(r.Context(), h.Store, HITLReview{
		TenantID:    tenantID,
		ContentID:   contentID,
		ContentType: input.ContentType,
		AIOutput:    aiOutputBytes,
	})
	if err != nil {
		slog.Error("creating HITL review", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create HITL review")

		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": review})
}

// HandleListPendingHITL lists pending HITL reviews for the current tenant.
func (h *Handlers) HandleListPendingHITL(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	reviews, err := ListPendingReviews(r.Context(), h.Store, tenantID, limit, offset)
	if err != nil {
		slog.Error("listing pending HITL reviews", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list reviews")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": reviews})
}

// HandleApproveHITL approves a pending HITL review. The review is scoped to
// the caller's tenant to prevent cross-tenant approve (Section 09).
func (h *Handlers) HandleApproveHITL(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var reviewID pgtype.UUID
	if err := reviewID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid review ID")

		return
	}

	if err := ApproveReview(r.Context(), h.Store, tenantID, reviewID, user.UserID); err != nil {
		slog.Error("approving HITL review", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "approved"}})
}

// HandleRejectHITL rejects a pending HITL review with a reason. The review is
// scoped to the caller's tenant to prevent cross-tenant reject (Section 09).
func (h *Handlers) HandleRejectHITL(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var reviewID pgtype.UUID
	if err := reviewID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid review ID")

		return
	}

	var input struct {
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if err := RejectReview(r.Context(), h.Store, tenantID, reviewID, user.UserID, input.Reason); err != nil {
		slog.Error("rejecting HITL review", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "rejected"}})
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
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		userID = chi.URLParam(r, "id")
	}
	if userID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "user_id is required")

		return
	}

	// Tenant scoping (§9, ADR-027): verify the target user belongs to the
	// admin's tenant before exposing their Honcho memory. Prevents cross-tenant
	// AI-context access.
	if h.Store != nil {
		t := tenant.FromContext(r.Context())
		if t != nil {
			ok, err := h.Store.UserBelongsToTenant(r.Context(), t.ID, userID)
			if err != nil {
				slog.Error("verifying user tenant membership", "error", err, "user_id", userID)
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to verify user")

				return
			}
			if !ok {
				writeError(w, http.StatusNotFound, "user_not_found", "User not found")

				return
			}
		}
	}

	entries, err := h.Honcho.GetContext(r.Context(), userID)
	if err != nil {
		slog.Error("getting user AI context", "error", err, "user_id", userID)
		writeError(w, http.StatusBadGateway, "honcho_error", "Failed to retrieve user context")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": entries,
		"meta": map[string]any{"total_count": len(entries)},
	})
}

// HandleDeleteUserContextEntry deletes a specific AI context entry for a user.
// DELETE /admin/users/{id}/ai-context/{entryId} (catalog route).
// Reads params from either {user_id}/{entry_id} or {id}/{entryId} chi params.
func (h *Handlers) HandleDeleteUserContextEntry(w http.ResponseWriter, r *http.Request) {
	if h.Honcho == nil {
		writeError(w, http.StatusServiceUnavailable, "honcho_unavailable", "Honcho client not configured")

		return
	}

	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		userID = chi.URLParam(r, "id")
	}
	if userID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "user_id is required")

		return
	}

	entryID := chi.URLParam(r, "entry_id")
	if entryID == "" {
		entryID = chi.URLParam(r, "entryId")
	}
	if entryID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "entry_id is required")

		return
	}

	// Tenant scoping (§9, ADR-027): verify the target user belongs to the
	// admin's tenant before deleting their Honcho memory entry.
	if h.Store != nil {
		t := tenant.FromContext(r.Context())
		if t != nil {
			ok, err := h.Store.UserBelongsToTenant(r.Context(), t.ID, userID)
			if err != nil {
				slog.Error("verifying user tenant membership", "error", err, "user_id", userID)
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to verify user")

				return
			}
			if !ok {
				writeError(w, http.StatusNotFound, "user_not_found", "User not found")

				return
			}
		}
	}

	if err := h.Honcho.DeleteEntry(r.Context(), userID, entryID); err != nil {
		slog.Error("deleting user context entry", "error", err, "user_id", userID, "entry_id", entryID)
		writeError(w, http.StatusBadGateway, "honcho_error", "Failed to delete context entry")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Admin AI Usage Handler (Section 20 — admin AI usage monitoring)
// ---------------------------------------------------------------------------

// HandleGetAIUsage returns aggregated AI usage for the tenant.
// GET /admin/ai/usage?period=7d|30d&provider=openai|ollama.
// Returns the Section 20 contract: { total_cost_usd, tokens_used, by_task_type }.
func (h *Handlers) HandleGetAIUsage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	periodDays := 30

	if p := r.URL.Query().Get("period"); p != "" {
		switch p {
		case "7d":
			periodDays = 7
		case "30d":
			periodDays = 30
		default:
			writeError(w, http.StatusBadRequest, "validation_error", "period must be '7d' or '30d'")
			return
		}
	}

	provider := r.URL.Query().Get("provider") // optional filter

	summary, err := h.Store.GetAIUsageSummary(r.Context(), tenantID, periodDays, provider)
	if err != nil {
		slog.Error("getting ai usage summary", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve AI usage")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": summary})
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
// GET /admin/ai/models. Models are derived from the providers actually
// registered with the dispatcher (Rule 16: no hardcoded/mock data). A
// "Default" entry (empty id) is always present for auto-selection.
func (h *Handlers) HandleListModels(w http.ResponseWriter, _ *http.Request) {
	models := []Model{{ID: "", Label: "Default"}}

	if h.Dispatcher != nil {
		for _, m := range h.Dispatcher.RegisteredModels() {
			models = append(models, m)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": models})
}

// ─── P5.6 AI Conversations ───────────────────────────────────────────────

// HandleListConversations handles GET /ai/conversations (P5.6).
// Returns the user's JapaBot conversation history, grouped by conversation_id
// from Honcho context entries (Section 20). Each conversation summary includes
// the conversation_id, the latest message preview, and the turn count. Returns
// an empty list when Honcho is not configured or the user has no entries.
func (h *Handlers) HandleListConversations(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return
	}

	if h.Honcho == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	entries, err := h.Honcho.GetContext(r.Context(), user.UserID.String())
	if err != nil {
		// Honcho may be unreachable — degrade to an empty list rather than 500.
		slog.Warn("ai: listing conversations from honcho", "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	// Group entries by conversation_id (metadata) into conversation summaries.
	type convoSummary struct {
		ConversationID string         `json:"conversation_id"`
		Preview        string         `json:"preview"`
		TurnCount      int            `json:"turn_count"`
		LastEntryAt    time.Time      `json:"last_entry_at"`
		Entries        []honcho.ContextEntry `json:"entries,omitempty"`
	}

	convos := map[string]*convoSummary{}
	var order []string

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
			order = append(order, cid)
		}

		c.TurnCount++
		c.Entries = append(c.Entries, e)
		if e.CreatedAt.After(c.LastEntryAt) {
			c.LastEntryAt = e.CreatedAt
			c.Preview = e.Content
		}
	}

	out := make([]convoSummary, 0, len(order))
	for _, cid := range order {
		out = append(out, *convos[cid])
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// HandleGetConversation handles GET /ai/conversations/:id (P5.6).
// Returns all Honcho context entries belonging to the conversation_id given
// in :id. Returns 404 when the conversation has no entries.
func (h *Handlers) HandleGetConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return
	}

	conversationID := chi.URLParam(r, "id")
	if conversationID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "conversation id is required")
		return
	}

	if h.Honcho == nil {
		writeError(w, http.StatusNotFound, "not_found", "Conversation not found")
		return
	}

	entries, err := h.Honcho.GetContext(r.Context(), user.UserID.String())
	if err != nil {
		slog.Warn("ai: loading conversations from honcho", "error", err)
		writeError(w, http.StatusNotFound, "not_found", "Conversation not found")
		return
	}

	// Collect all entries whose metadata.conversation_id matches the requested id.
	var matched []honcho.ContextEntry
	for _, e := range entries {
		if e.Metadata != nil {
			if cid, ok := e.Metadata["conversation_id"].(string); ok && cid == conversationID {
				matched = append(matched, e)
			}
		}
	}

	if len(matched) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Conversation not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"conversation_id": conversationID,
		"entries":         matched,
		"turn_count":      len(matched),
	}})
}

// HandleListCategories handles GET /ai/categories — returns available AI
// categories derived from the providers actually registered with the
// dispatcher (Rule 16: no hardcoded/mock data).
func (h *Handlers) HandleListCategories(w http.ResponseWriter, _ *http.Request) {
	categories := []map[string]string{}

	if h.Dispatcher != nil {
		for _, cat := range h.Dispatcher.RegisteredCategories() {
			categories = append(categories, map[string]string{
				"value": cat,
				"label": cat,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": categories})
}
