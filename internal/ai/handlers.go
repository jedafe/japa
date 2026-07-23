// Package ai — handlers.go provides HTTP handlers for the AI Core system.
//
// Routes:
//
//	POST /api/v1/ai/generate                      — generate AI output via the gateway
//	POST /api/v1/ai/chat                           — JapaBot conversational AI
//	POST /api/v1/ai/improve                        — improve writing quality
//	POST /api/v1/ai/summarize                      — summarize text
//	POST /api/v1/ai/translate                      — translate text
//	POST /api/v1/ai/generate-tags                  — generate content tags
//	POST /api/v1/ai/generate-og                    — generate OG metadata
//	POST /api/v1/ai/hitl                           — create HITL review
//	GET  /api/v1/ai/hitl/pending                   — list pending reviews
//	POST /api/v1/ai/hitl/{id}/approve              — approve a review
//	POST /api/v1/ai/hitl/{id}/reject               — reject a review with reason
//	GET  /admin/ai/context/{user_id}               — view user's AI context (Honcho)
//	DELETE /admin/ai/context/{user_id}/{entry_id}   — delete a specific context entry
//	GET  /admin/ai/usage                           — list AI usage logs (stub)
package ai

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/honcho"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handlers holds dependencies for AI HTTP handlers.
type Handlers struct {
	Dispatcher  *Dispatcher
	Store       *Store
	Honcho      *honcho.Client
	AsynqClient TaskEnqueueClient
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
// Request body: { "message": "user message", "context": {optional context} }
// Response: { "data": { "response": "AI response", "provider_ref": "...", "status": "..." } }.
func (h *Handlers) HandleChat(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Message string         `json:"message"`
		Context map[string]any `json:"context,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")

		return
	}

	if input.Message == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "message is required")

		return
	}

	payload := map[string]any{
		"prompt":    input.Message,
		"context":   input.Context,
		"user_id":   user.UserID.String(),
		"tenant_id": tenantID.String(),
		"task":      "chat",
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

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// ---------------------------------------------------------------------------
// Content AI Assist Handlers
// ---------------------------------------------------------------------------

// HandleImprove handles POST /api/v1/ai/improve — improve writing quality.
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

// HandleSummarize handles POST /api/v1/ai/summarize — summarize text.
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

// HandleTranslate handles POST /api/v1/ai/translate — translate text.
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

// HandleGenerateTags handles POST /api/v1/ai/generate-tags — generate content tags.
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

// HandleGenerateOG handles POST /api/v1/ai/generate-og — generate OG metadata.
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
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())

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

// HandleApproveHITL approves a pending HITL review.
func (h *Handlers) HandleApproveHITL(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var reviewID pgtype.UUID
	if err := reviewID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid review ID")

		return
	}

	if err := ApproveReview(r.Context(), h.Store, reviewID, user.UserID); err != nil {
		slog.Error("approving HITL review", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "approved"}})
}

// HandleRejectHITL rejects a pending HITL review with a reason.
func (h *Handlers) HandleRejectHITL(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
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

	if err := RejectReview(r.Context(), h.Store, reviewID, user.UserID, input.Reason); err != nil {
		slog.Error("rejecting HITL review", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "rejected"}})
}

// ---------------------------------------------------------------------------
// Admin AI Context Handlers
// ---------------------------------------------------------------------------

// HandleGetUserContext returns the AI context entries for a specific user.
// GET /admin/ai/context/{user_id}.
func (h *Handlers) HandleGetUserContext(w http.ResponseWriter, r *http.Request) {
	if h.Honcho == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "user_id is required")

		return
	}

	entries, err := h.Honcho.GetContext(r.Context(), userID)
	if err != nil {
		slog.Error("getting user AI context", "error", err, "user_id", userID)
		writeError(w, http.StatusBadGateway, "honcho_error", "Failed to retrieve user context")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":        entries,
		"total_count": len(entries),
	})
}

// HandleDeleteUserContextEntry deletes a specific AI context entry for a user.
// DELETE /admin/ai/context/{user_id}/{entry_id}.
func (h *Handlers) HandleDeleteUserContextEntry(w http.ResponseWriter, r *http.Request) {
	if h.Honcho == nil {
		writeError(w, http.StatusServiceUnavailable, "honcho_unavailable", "Honcho client not configured")

		return
	}

	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "user_id is required")

		return
	}

	entryID := chi.URLParam(r, "entry_id")
	if entryID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "entry_id is required")

		return
	}

	if err := h.Honcho.DeleteEntry(r.Context(), userID, entryID); err != nil {
		slog.Error("deleting user context entry", "error", err, "user_id", userID, "entry_id", entryID)
		writeError(w, http.StatusBadGateway, "honcho_error", "Failed to delete context entry")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Admin AI Usage Handler (stub — ai_usage_log table not yet created)
// ---------------------------------------------------------------------------

// HandleGetAIUsage returns AI usage logs. Currently a stub that returns an
// empty dataset until the ai_usage_log migration is added.
// GET /admin/ai/usage.
func (h *Handlers) HandleGetAIUsage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data":        []any{},
		"total_count": 0,
		"note":        "ai_usage_log table not yet implemented",
	})
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
// GET /admin/ai/models.
func (h *Handlers) HandleListModels(w http.ResponseWriter, _ *http.Request) {
	// Models are derived from the registered providers in the dispatcher.
	// In the current setup, the Xiaomi provider is the primary one.
	// The admin can select "Default" (auto-select) or a specific model.
	models := []Model{
		{ID: "", Label: "Default"},
		{ID: "mimo-flash", Label: "MiMo Flash (Xiaomi)"},
		{ID: "gpt-4", Label: "GPT-4"},
		{ID: "gpt-3.5-turbo", Label: "GPT-3.5 Turbo"},
		{ID: "claude-3", Label: "Claude 3"},
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": models})
}

// ─── P5.6 AI Conversations ───────────────────────────────────────────────

// HandleListConversations handles GET /ai/conversations (P5.6).
func (h *Handlers) HandleListConversations(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return
	}
	// Conversation history is stored in Honcho — list context entries as conversations
	entries, err := h.Honcho.GetContext(r.Context(), user.UserID.String())
	if err != nil {
		// Honcho may not be configured — return empty list
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
}

// HandleGetConversation handles GET /ai/conversations/:id (P5.6).
func (h *Handlers) HandleGetConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return
	}
	// Conversation history is stored in Honcho — return context entries
	entries, err := h.Honcho.GetContext(r.Context(), user.UserID.String())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
}

// HandleListCategories handles GET /ai/categories — returns available AI categories.
func (h *Handlers) HandleListCategories(w http.ResponseWriter, _ *http.Request) {
	categories := []map[string]string{
		{"value": "content_generation", "label": "Content Generation", "description": "Generate blog posts, articles, and other content"},
		{"value": "chat", "label": "Chat", "description": "Conversational AI assistant"},
		{"value": "moderation", "label": "Moderation", "description": "Content moderation and safety"},
		{"value": "embedding", "label": "Embedding", "description": "Vector embeddings for search and recommendations"},
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": categories})
}
