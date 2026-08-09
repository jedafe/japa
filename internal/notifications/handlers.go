package notifications

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
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

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
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
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mark all notifications as read")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"success": true}})
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid notification ID format")
		return
	}

	if err := h.service.DeleteNotification(r.Context(), id, user.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete notification")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	for _, p := range input.Preferences {
		if p.Type == "" {
			writeError(w, http.StatusBadRequest, "missing_type", "each preference requires a type")
			return
		}

		if err := h.service.UpdatePreference(r.Context(), user.UserID, p.Type, p.InApp, p.Email, p.Push); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update preference")
			return
		}
	}

	prefs, err := h.service.GetPreferences(r.Context(), user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get preferences")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": prefs})
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

// AdminListNotificationTypes handles GET /api/v1/admin/notification-types —
// paginated, searchable, filterable, sortable list for the admin DataTable
// (Section 35). Returns { data, total, page, total_pages }.
func (h *Handlers) AdminListNotificationTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")

	// enabled filter: "true" → 1, "false" → 2 (sentinel: != 1 so the SQL
	// `enabled = (filter = 1)` resolves correctly), absent/empty → 0 (no filter).
	enabledFilter := int32(0)
	if ev := r.URL.Query().Get("enabled"); ev != "" {
		switch ev {
		case "true", "1":
			enabledFilter = 1
		case "false", "0":
			enabledFilter = 2
		}
	}

	sortParam := r.URL.Query().Get("sort")
	// Allowlist sortable columns; default to key.
	switch sortParam {
	case "key", "label", "enabled":
	default:
		sortParam = "key"
	}

	dir := r.URL.Query().Get("dir")
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	types, err := h.service.AdminListNotificationTypes(r.Context(), gen.AdminListNotificationTypesParams{
		Q:             q,
		EnabledFilter: enabledFilter,
		Sort:          sortParam,
		Dir:           dir,
		PageOffset:    offset,
		PageLimit:     limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list notification types")
		return
	}

	count, err := h.service.AdminCountNotificationTypes(r.Context(), gen.AdminCountNotificationTypesParams{
		Q:             q,
		EnabledFilter: enabledFilter,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to count notification types")
		return
	}

	totalPages := int32(1)
	if limit > 0 {
		totalPages = (count + limit - 1) / limit
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":        types,
		"total":       count,
		"page":        offset / limit,
		"total_pages": totalPages,
	})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "missing_endpoint", "endpoint is required")
		return
	}

	if input.P256DH == "" {
		writeError(w, http.StatusBadRequest, "missing_p256dh", "p256dh is required")
		return
	}

	if input.Auth == "" {
		writeError(w, http.StatusBadRequest, "missing_auth", "auth is required")
		return
	}

	if err := h.service.SavePushSubscription(r.Context(), user.UserID, input.Endpoint, input.P256DH, input.Auth); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to save push subscription")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"status": "subscribed"}})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "missing_endpoint", "endpoint is required")
		return
	}

	if err := h.service.DeletePushSubscription(r.Context(), user.UserID, input.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete push subscription")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// HandleGetVAPIDKey handles GET /api/v1/push/vapid-key (public, no auth).
func (h *Handlers) HandleGetVAPIDKey(w http.ResponseWriter, _ *http.Request) {
	publicKey := h.service.GetVAPIDPublicKey()
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"publicKey": publicKey}})
}

// CreateNotificationType handles POST /admin/notification-types (P5.6).
// Ref: japaconnect-engineering/sections/35-notification-system.md
func (h *Handlers) CreateNotificationType(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Key           string  `json:"key"`
		Label         string  `json:"label"`
		Description   *string `json:"description"`
		Enabled       *bool   `json:"enabled"`
		DefaultInApp  *bool   `json:"default_in_app"`
		DefaultEmail  *bool   `json:"default_email"`
		DefaultPush   *bool   `json:"default_push"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}
	if input.Key == "" || input.Label == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "key and label are required")
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	// Section 35 defaults: in_app=true, email=false, push=false
	defaultInApp := true
	if input.DefaultInApp != nil {
		defaultInApp = *input.DefaultInApp
	}
	defaultEmail := false
	if input.DefaultEmail != nil {
		defaultEmail = *input.DefaultEmail
	}
	defaultPush := false
	if input.DefaultPush != nil {
		defaultPush = *input.DefaultPush
	}
	if err := h.service.CreateNotificationType(r.Context(), input.Key, input.Label, input.Description, enabled, defaultInApp, defaultEmail, defaultPush); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create notification type")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"key": input.Key}})
}

// UpdateNotificationType handles PATCH /admin/notification-types/:key (P5.6).
// Ref: japaconnect-engineering/sections/35-notification-system.md
func (h *Handlers) UpdateNotificationType(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var input struct {
		Label         string  `json:"label"`
		Description   *string `json:"description"`
		Enabled       *bool   `json:"enabled"`
		DefaultInApp  *bool   `json:"default_in_app"`
		DefaultEmail  *bool   `json:"default_email"`
		DefaultPush   *bool   `json:"default_push"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	defaultInApp := true
	if input.DefaultInApp != nil {
		defaultInApp = *input.DefaultInApp
	}
	defaultEmail := false
	if input.DefaultEmail != nil {
		defaultEmail = *input.DefaultEmail
	}
	defaultPush := false
	if input.DefaultPush != nil {
		defaultPush = *input.DefaultPush
	}
	if err := h.service.UpdateNotificationType(r.Context(), key, input.Label, input.Description, enabled, defaultInApp, defaultEmail, defaultPush); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update notification type")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"key": key}})
}

// DeleteNotificationType handles DELETE /admin/notification-types/:key (P5.6).
func (h *Handlers) DeleteNotificationType(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := h.service.DeleteNotificationType(r.Context(), key); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete notification type")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
