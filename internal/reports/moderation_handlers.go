package reports

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ModerationScope handles GET /api/v1/me/moderation/scope — the caller's
// moderation envelope; the frontend uses it to decide whether to render the
// /studio Moderation tab.
func (h *Handlers) ModerationScope(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	scope, err := h.service.ResolveModerationScope(r.Context(), user.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve moderation scope")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": scope})
}

// requireModerationScope resolves the caller's scope and writes a 403 when the
// caller has no moderation scope at all.
func (h *Handlers) requireModerationScope(w http.ResponseWriter, r *http.Request, userID pgtype.UUID) (*ModerationScope, bool) {
	scope, err := h.service.ResolveModerationScope(r.Context(), userID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve moderation scope")
		return nil, false
	}
	if !scope.Active() {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You do not have moderation access")
		return nil, false
	}
	return scope, true
}

// parseModerationFilter reads the shared queue filters from the query string.
func parseModerationFilter(r *http.Request) ModerationQueueFilter {
	var f ModerationQueueFilter

	if v := r.URL.Query().Get("status"); v != "" {
		f.Status = &v
	}
	if v := r.URL.Query().Get("entity_type"); v != "" {
		f.EntityType = &v
	}
	if v := r.URL.Query().Get("space_id"); v != "" {
		_ = f.SpaceID.Scan(v)
	}
	if v := r.URL.Query().Get("tag_id"); v != "" {
		_ = f.TagID.Scan(v)
	}
	f.AssignedOnly = r.URL.Query().Get("assigned") == "me" || r.URL.Query().Get("assigned") == "true"
	if v := r.URL.Query().Get("q"); v != "" {
		f.Q = &v
	}
	return f
}

// ListModerationQueue handles GET /api/v1/me/moderation/queue.
func (h *Handlers) ListModerationQueue(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	scope, ok := h.requireModerationScope(w, r, user.UserID)
	if !ok {
		return
	}

	filter := parseModerationFilter(r)
	limit := parsePageSize(r)
	page := int32(0)
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.ParseInt(p, 10, 32); err == nil && v >= 0 {
			page = int32(v)
		}
	}
	offset := page * limit

	items, err := h.service.ListModerationQueue(r.Context(), scope, user.UserID, filter, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list moderation queue")
		return
	}
	total, err := h.service.CountModerationQueue(r.Context(), scope, user.UserID, filter)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count moderation queue")
		return
	}

	totalPages := int32(0)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       items,
			"page":        page,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// GetModerationItem handles GET /api/v1/me/moderation/queue/{id}.
func (h *Handlers) GetModerationItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	scope, ok := h.requireModerationScope(w, r, user.UserID)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	item, err := h.service.GetModerationQueueItem(r.Context(), scope, user.UserID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// ActionModerationItem handles PATCH /api/v1/me/moderation/queue/{id}.
// Body: { action: "dismiss"|"remove"|"warn"|"ban", reason? }.
func (h *Handlers) ActionModerationItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	// Early 403 for callers with no scope at all; the service re-resolves
	// scope fresh inside ActionModerationItem for the actual authorization.
	if _, ok := h.requireModerationScope(w, r, user.UserID); !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Action == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_action", "action is required")
		return
	}

	updated, err := h.service.ActionModerationItem(r.Context(), user.UserID, id, input.Action, input.Reason)
	if err != nil {
		if errors.Is(err, ErrActionNotAllowed) {
			errpkg.WriteError(w, http.StatusForbidden, "action_not_allowed", "This action exceeds your moderation scope")
			return
		}
		if errors.Is(err, ErrAlreadyResolved) {
			errpkg.WriteError(w, http.StatusConflict, "already_resolved", "This report has already been resolved")
			return
		}
		writeServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}
