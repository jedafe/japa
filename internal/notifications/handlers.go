package notifications

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

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
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get notifications")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": notifications})
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
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get unread count")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid notification ID format")
		return
	}

	if err := h.service.MarkAsRead(r.Context(), id, user.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to mark notification as read")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"read": true}})
}

// MarkAllAsRead handles POST /api/v1/me/notifications/read-all.
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
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to mark all notifications as read")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"success": true}})
}

// DeleteNotification handles DELETE /api/v1/me/notifications/{id}.
func (h *Handlers) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")

	var id pgtype.UUID

	if err := id.Scan(idStr); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid notification ID format")
		return
	}

	if err := h.service.DeleteNotification(r.Context(), id, user.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete notification")
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// GetPreferences handles GET /api/v1/notification-preferences.
func (h *Handlers) GetPreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	prefs, err := h.service.GetPreferences(r.Context(), user.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get preferences")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": prefs})
}

// UpdatePreference handles PUT /api/v1/notification-preferences/{type}.
func (h *Handlers) UpdatePreference(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	nType := chi.URLParam(r, "type")
	if nType == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_type", "notification type is required")
		return
	}

	var input struct {
		InApp bool `json:"in_app"`
		Email bool `json:"email"`
		Push  bool `json:"push"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if err := h.service.UpdatePreference(r.Context(), user.UserID, nType, input.InApp, input.Email, input.Push); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update preference")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"success": true}})
}

// UpdatePreferences handles PATCH /api/v1/me/notification-preferences — a
// bulk update across multiple notification types in one request. Thin
// orchestration over the existing per-type UpdatePreference; no new
// service-layer logic.
func (h *Handlers) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Preferences []struct {
			Type  string `json:"type"`
			InApp bool   `json:"in_app"`
			Email bool   `json:"email"`
			Push  bool   `json:"push"`
		} `json:"preferences"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	for _, p := range input.Preferences {
		if p.Type == "" {
			httpx.WriteError(w, http.StatusBadRequest, "missing_type", "each preference requires a type")
			return
		}

		if err := h.service.UpdatePreference(r.Context(), user.UserID, p.Type, p.InApp, p.Email, p.Push); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update preference")
			return
		}
	}

	prefs, err := h.service.GetPreferences(r.Context(), user.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get preferences")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": prefs})
}

// ListNotificationTypes handles GET /api/v1/notification-types.
func (h *Handlers) ListNotificationTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	types, err := h.service.GetNotificationTypes(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get notification types")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// ---------------------------------------------------------------------------
// Push Subscription Handlers
// ---------------------------------------------------------------------------

// HandlePushSubscribe handles POST /api/v1/push/subscribe.
func (h *Handlers) HandlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Endpoint string `json:"endpoint"`
		P256DH   string `json:"p256dh"`
		Auth     string `json:"auth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Endpoint == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_endpoint", "endpoint is required")
		return
	}

	if input.P256DH == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_p256dh", "p256dh is required")
		return
	}

	if input.Auth == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_auth", "auth is required")
		return
	}

	if err := h.service.SavePushSubscription(r.Context(), user.UserID, input.Endpoint, input.P256DH, input.Auth); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to save push subscription")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"status": "subscribed"}})
}

// HandlePushUnsubscribe handles DELETE /api/v1/push/subscribe.
func (h *Handlers) HandlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Endpoint string `json:"endpoint"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Endpoint == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_endpoint", "endpoint is required")
		return
	}

	if err := h.service.DeletePushSubscription(r.Context(), user.UserID, input.Endpoint); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete push subscription")
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// HandleGetVAPIDKey handles GET /api/v1/push/vapid-key (public, no auth).
func (h *Handlers) HandleGetVAPIDKey(w http.ResponseWriter, _ *http.Request) {
	publicKey := h.service.GetVAPIDPublicKey()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"publicKey": publicKey}})
}

// CreateNotificationType handles POST /admin/notification-types (P5.6).
func (h *Handlers) CreateNotificationType(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Key         string  `json:"key"`
		Label       string  `json:"label"`
		Description *string `json:"description"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}
	if input.Key == "" || input.Label == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "key and label are required")
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if err := h.service.CreateNotificationType(r.Context(), input.Key, input.Label, input.Description, enabled); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create notification type")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"key": input.Key}})
}

// UpdateNotificationType handles PATCH /admin/notification-types/:key (P5.6).
func (h *Handlers) UpdateNotificationType(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var input struct {
		Label       string  `json:"label"`
		Description *string `json:"description"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if err := h.service.UpdateNotificationType(r.Context(), key, input.Label, input.Description, enabled); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update notification type")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key}})
}

// DeleteNotificationType handles DELETE /admin/notification-types/:key (P5.6).
func (h *Handlers) DeleteNotificationType(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := h.service.DeleteNotificationType(r.Context(), key); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete notification type")
		return
	}
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}
