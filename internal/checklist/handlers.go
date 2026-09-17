// Package checklist — handlers.go provides the HTTP handlers for the Visa
// Document Checklist routes (Section 70, ADR-070).
//
// Routes:
//
//	GET   /api/v1/checklist/selector                       — country + visa-type options (public)
//	GET   /api/v1/checklist/{guide_id}                     — checklist items for a guide (public)
//	GET   /api/v1/checklist/{guide_id}/progress            — user's progress (auth)
//	PATCH /api/v1/checklist/{guide_id}/progress/{item_key} — mark complete/incomplete + note (auth)
//	DELETE /api/v1/checklist/{guide_id}/progress           — reset progress (auth)
//	GET   /api/v1/admin/checklist/progress                 — all users' progress (admin)
//	GET   /api/v1/admin/checklist/completion-stats         — completion rates by guide (admin)
//
// Ref: japaconnect-engineering/sections/70-visa-checklist.md
//
//	japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md
package checklist

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the checklist endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates checklist handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go)
// ---------------------------------------------------------------------------

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

// parsePageParams reads page/limit query params with sensible defaults.
func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (no auth required — anonymous browse per Section 70)
// ---------------------------------------------------------------------------

// HandleSelector handles GET /checklist/selector — country + visa-type
// options from published visa guides. Public, no auth required.
func (h *Handlers) HandleSelector(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "checklist_unavailable", "Checklist service not configured")
		return
	}

	if !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusForbidden, "checklist_disabled", "Checklist tool is disabled")
		return
	}

	result, err := h.service.GetSelector(r.Context())
	if err != nil {
		slog.Error("checklist: selector failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "selector_failed", "Failed to get selector options")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// HandleGetItems handles GET /checklist/{guide_id} — the checklist guide
// (guide_id + title + items extracted from required_documents). Public, no
// auth required (anonymous browse). Returns the ChecklistGuideResult shape
// that matches the frontend ChecklistGuide type (§70 contract).
func (h *Handlers) HandleGetItems(w http.ResponseWriter, r *http.Request) {

	guideID, ok := scanUUIDParam(w, r, "guide_id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "checklist_unavailable", "Checklist service not configured")
		return
	}

	if !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusForbidden, "checklist_disabled", "Checklist tool is disabled")
		return
	}

	guide, err := h.service.GetGuide(r.Context(), guideID)
	if err != nil {
		slog.Error("checklist: get guide failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "items_failed", "Failed to get checklist items")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guide})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (progress — auth required)
// ---------------------------------------------------------------------------

// HandleGetProgress handles GET /checklist/{guide_id}/progress — the
// authenticated user's progress on this checklist.
func (h *Handlers) HandleGetProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	guideID, ok := scanUUIDParam(w, r, "guide_id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "checklist_unavailable", "Checklist service not configured")
		return
	}

	items, err := h.service.GetProgress(r.Context(), user.UserID, guideID)
	if err != nil {
		slog.Error("checklist: get progress failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "progress_failed", "Failed to get progress")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// updateProgressBody is the PATCH request body for marking an item. Both
// fields are pointers so a note-only update (completed omitted) preserves the
// existing completed state, and a toggle-only update (notes omitted) preserves
// the existing notes.
type updateProgressBody struct {
	Completed *bool   `json:"completed"`
	Notes     *string `json:"notes"`
}

// HandleUpdateProgress handles PATCH
// /checklist/{guide_id}/progress/{item_key} — mark an item complete/incomplete
// with an optional note. Auth required. A nil completed preserves the existing
// completion state (note-only update); a nil notes preserves existing notes.
func (h *Handlers) HandleUpdateProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	guideID, ok := scanUUIDParam(w, r, "guide_id")
	if !ok {
		return
	}

	itemKey := chi.URLParam(r, "item_key")
	if itemKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_item_key", "Item key is required")
		return
	}

	var body updateProgressBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "checklist_unavailable", "Checklist service not configured")
		return
	}

	row, err := h.service.UpsertProgress(r.Context(), user.UserID, guideID, itemKey, body.Completed, body.Notes)
	if err != nil {
		slog.Error("checklist: update progress failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update progress")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":           row.ID.String(),
			"item_key":     row.ItemKey,
			"completed":    row.Completed,
			"completed_at": formatTs(row.CompletedAt),
			"notes":        ptrString(row.Notes),
			"updated_at":   formatTs(row.UpdatedAt),
		},
	})
}

// HandleResetProgress handles DELETE /checklist/{guide_id}/progress — reset
// all progress for this checklist. Auth required.
func (h *Handlers) HandleResetProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	guideID, ok := scanUUIDParam(w, r, "guide_id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "checklist_unavailable", "Checklist service not configured")
		return
	}

	if err := h.service.ResetProgress(r.Context(), user.UserID, guideID); err != nil {
		slog.Error("checklist: reset progress failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reset_failed", "Failed to reset progress")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"reset": true}})
}

// ---------------------------------------------------------------------------
// Admin handlers (checklist.manage permission, checked at route registration)
// ---------------------------------------------------------------------------

// HandleAdminListProgress handles GET /admin/checklist/progress — all users'
// progress, filterable by guide_id and user_id. Admin-only.
func (h *Handlers) HandleAdminListProgress(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "checklist_unavailable", "Checklist service not configured")
		return
	}

	var guideID pgtype.UUID
	if v := r.URL.Query().Get("guide_id"); v != "" {
		if err := guideID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid guide_id")
			return
		}
	}

	var userID pgtype.UUID
	if v := r.URL.Query().Get("user_id"); v != "" {
		if err := userID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user_id")
			return
		}
	}

	page, limit := parsePageParams(r)

	rows, total, err := h.service.AdminListProgress(r.Context(), guideID, userID, page, limit)
	if err != nil {
		slog.Error("checklist: admin list progress failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list progress")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminCompletionStats handles GET /admin/checklist/completion-stats —
// completion rates by guide. Admin-only.
func (h *Handlers) HandleAdminCompletionStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "checklist_unavailable", "Checklist service not configured")
		return
	}

	page, limit := parsePageParams(r)

	stats, err := h.service.AdminCompletionStats(r.Context(), page, limit)
	if err != nil {
		slog.Error("checklist: admin completion stats failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "stats_failed", "Failed to get completion stats")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": stats,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
		},
	})
}
