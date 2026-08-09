// Package calendar — handlers.go provides the HTTP handlers for the
// Community Calendar routes (§86, ADR-094).
//
// Routes:
//
//	GET    /api/v1/calendar               — list approved entries (public)
//	GET    /api/v1/calendar/:id           — single approved entry (public)
//	POST   /api/v1/calendar               — submit entry (auth, calendar.submit)
//	POST   /api/v1/calendar/:id/save      — save entry (auth)
//	DELETE /api/v1/calendar/:id/save      — unsave entry (auth)
//	POST   /api/v1/calendar/subscribe     — subscribe (auth)
//	DELETE /api/v1/calendar/subscribe     — unsubscribe (auth)
//	GET    /api/v1/admin/calendar         — admin list (auth, calendar.review)
//	PATCH  /api/v1/admin/calendar/:id     — approve/archive/flag (auth, calendar.review)
//
// Ref: japaconnect-engineering/sections/86-community-calendar.md
//      japaconnect-engineering/adr/ADR-094-community-calendar.md
package calendar

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the calendar endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates calendar handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("calendar: encode response", "error", err)
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
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n)
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (browse — anonymous)
// ---------------------------------------------------------------------------

// HandleList handles GET /calendar — list approved calendar entries with
// optional filters (dest, category, from, to). Also accepts ?destination_space_id= as legacy alias.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var destSpaceID pgtype.UUID
	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}
	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	category := r.URL.Query().Get("category")

	var from, to pgtype.Timestamptz
	if v := r.URL.Query().Get("from"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date", "Invalid from date")
			return
		}
		from = ts
	}
	if v := r.URL.Query().Get("to"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date", "Invalid to date")
			return
		}
		to = ts
	}

	page, limit := parsePageParams(r)

	entries, total, err := h.service.List(r.Context(), tenantID, destSpaceID, category, from, to, page, limit)
	if err != nil {
		slog.Error("calendar: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list calendar entries")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": entries,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGet handles GET /calendar/:id — single approved calendar entry.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	entry, err := h.service.Get(r.Context(), entryID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "entry_not_found", "Calendar entry not found")
			return
		}
		slog.Error("calendar: get failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get calendar entry")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entry})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (submit, save, subscribe)
// ---------------------------------------------------------------------------

// HandleSubmit handles POST /calendar — submit a new calendar entry for
// review. Auth required (calendar.submit permission checked at route
// registration).
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input SubmitEntryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if input.Category == "" {
		writeError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}
	if input.OccursAt == "" {
		writeError(w, http.StatusBadRequest, "missing_occurs_at", "occurs_at is required")
		return
	}

	entryID, err := h.service.Submit(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		slog.Error("calendar: submit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit calendar entry")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"entry_id": entryID,
			"status":   "pending",
		},
	})
}

// HandleSave handles POST /calendar/:id/save — bookmark a calendar entry.
func (h *Handlers) HandleSave(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Save(r.Context(), user.UserID, entryID, tenantID); err != nil {
		slog.Error("calendar: save failed", "error", err)
		writeError(w, http.StatusInternalServerError, "save_failed", "Failed to save calendar entry")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"saved": true}})
}

// HandleUnsave handles DELETE /calendar/:id/save — remove a bookmark.
func (h *Handlers) HandleUnsave(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Unsave(r.Context(), user.UserID, entryID, tenantID); err != nil {
		slog.Error("calendar: unsave failed", "error", err)
		writeError(w, http.StatusInternalServerError, "unsave_failed", "Failed to unsave calendar entry")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"saved": false}})
}

// HandleSubscribe handles POST /calendar/subscribe — subscribe to a
// category/destination combination.
func (h *Handlers) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input SubscribeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination_space_id")
		return
	}

	if err := h.service.Subscribe(r.Context(), user.UserID, tenantID, destSpaceID, input.Category); err != nil {
		slog.Error("calendar: subscribe failed", "error", err)
		writeError(w, http.StatusInternalServerError, "subscribe_failed", "Failed to subscribe")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"subscribed": true}})
}

// HandleUnsubscribe handles DELETE /calendar/subscribe — remove a
// category/destination subscription.
func (h *Handlers) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var destSpaceID pgtype.UUID
	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}
	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "missing_destination", "dest or destination_space_id is required")
		return
	}

	category := r.URL.Query().Get("category")

	if err := h.service.Unsubscribe(r.Context(), user.UserID, tenantID, destSpaceID, category); err != nil {
		slog.Error("calendar: unsubscribe failed", "error", err)
		writeError(w, http.StatusInternalServerError, "unsubscribe_failed", "Failed to unsubscribe")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"subscribed": false}})
}

// ---------------------------------------------------------------------------
// Admin handlers (review — calendar.review)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/calendar — admin list of all calendar
// entries (any status).
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var destSpaceID pgtype.UUID
	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}
	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	category := r.URL.Query().Get("category")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	var from, to pgtype.Timestamptz
	if v := r.URL.Query().Get("from"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date", "Invalid from date")
			return
		}
		from = ts
	}
	if v := r.URL.Query().Get("to"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date", "Invalid to date")
			return
		}
		to = ts
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	entries, total, err := h.service.AdminList(r.Context(), tenantID, destSpaceID, category, status, q, from, to, page, limit)
	if err != nil {
		slog.Error("calendar: admin list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list calendar entries")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": entries,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminUpdate handles PATCH /admin/calendar/:id — approve, archive,
// or flag a calendar entry.
func (h *Handlers) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input AdminUpdateEntryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Status == "" {
		writeError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	if err := h.service.AdminUpdate(r.Context(), entryID, tenantID, user.UserID, input.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "entry_not_found", "Calendar entry not found")
			return
		}
		slog.Error("calendar: admin update failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update calendar entry")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
}
