// Package ai — handlers.go provides HTTP handlers for the AI Core system.
//
// Routes:
//
//	POST /api/v1/ai/generate        — generate AI output via the gateway
//	POST /api/v1/ai/hitl             — create HITL review
//	GET  /api/v1/ai/hitl/pending     — list pending reviews
//	POST /api/v1/ai/hitl/{id}/approve — approve a review
//	POST /api/v1/ai/hitl/{id}/reject  — reject a review with reason
package ai

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for AI HTTP handlers.
type Handlers struct {
	Dispatcher *Dispatcher
	Store      *Store
}

// NewHandlers creates Handlers with the given dependencies.
func NewHandlers(dispatcher *Dispatcher, store *Store) *Handlers {
	return &Handlers{Dispatcher: dispatcher, Store: store}
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
	return int32(n)
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
	return int32(n)
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
		"prompt":   input.Prompt,
		"context":  input.Context,
		"user_id":  user.UserID.String(),
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

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
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
