package notifications

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for notification HTTP handlers.
type Handlers struct {
	service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return DefaultPageSize
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return int32(n)
}

// parsePageOffset extracts the page_offset query parameter.
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
// Notification Handlers
// ---------------------------------------------------------------------------

// ListNotifications handles GET /api/v1/notifications.
func (h *Handlers) ListNotifications(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	notifications, err := h.service.GetNotifications(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": notifications})
}

// GetUnreadCount handles GET /api/v1/notifications/unread-count.
func (h *Handlers) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	count, err := h.service.GetUnreadCount(r.Context(), tenantID, user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get unread count")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]int{"count": count},
	})
}

// MarkAsRead handles PATCH /api/v1/notifications/{id}/read.
func (h *Handlers) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid notification ID format")
		return
	}

	if err := h.service.MarkAsRead(r.Context(), id, user.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mark notification as read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"read": true}})
}

// MarkAllAsRead handles POST /api/v1/notifications/read-all.
func (h *Handlers) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.service.MarkAllAsRead(r.Context(), tenantID, user.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mark all notifications as read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"success": true}})
}

// GetPreferences handles GET /api/v1/notification-preferences.
func (h *Handlers) GetPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	prefs, err := h.service.GetPreferences(r.Context(), user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get preferences")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": prefs})
}

// UpdatePreference handles PUT /api/v1/notification-preferences/{type}.
func (h *Handlers) UpdatePreference(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	nType := chi.URLParam(r, "type")
	if nType == "" {
		writeError(w, http.StatusBadRequest, "missing_type", "notification type is required")
		return
	}

	var input struct {
		InApp bool `json:"in_app"`
		Email bool `json:"email"`
		Push  bool `json:"push"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if err := h.service.UpdatePreference(r.Context(), user.UserID, nType, input.InApp, input.Email, input.Push); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update preference")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"success": true}})
}

// ListNotificationTypes handles GET /api/v1/notification-types.
func (h *Handlers) ListNotificationTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	types, err := h.service.GetNotificationTypes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get notification types")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}
