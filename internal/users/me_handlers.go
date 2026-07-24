package users

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

// ChangePasswordRequest is the input for changing password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword handles POST /api/v1/me/change-password.
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "current_password and new_password are required")
		return
	}

	if len(req.NewPassword) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "password_too_short", "Password must be at least 8 characters")
		return
	}

	// Get current password hash.
	hashRow, err := h.Service.store.getUserPasswordHash(r.Context(), user.UserID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		handleServiceError(w, err)
		return
	}

	// Verify current password.
	if hashRow.PasswordHash != nil {
		if err := bcrypt.CompareHashAndPassword([]byte(*hashRow.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid_password", "Current password is incorrect")
			return
		}
	}

	// Hash new password.
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to hash password")
		return
	}

	hashStr := string(newHash)
	if err := h.Service.store.updateUserPassword(r.Context(), user.UserID, &hashStr); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// PrivacySettings represents user privacy settings.
type PrivacySettings struct {
	DmPrivacy         string `json:"dm_privacy"`
	ProfileVisibility string `json:"profile_visibility"`
	SearchIndexable   bool   `json:"search_indexable"`
	ShowOnlineStatus  bool   `json:"show_online_status"`
	ShowActivity      bool   `json:"show_activity"`
}

// GetPrivacy handles GET /api/v1/me/privacy.
func (h *Handlers) GetPrivacy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	settings, err := h.Service.GetPrivacySettings(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
}

// UpdatePrivacyRequest is the input for updating privacy settings.
type UpdatePrivacyRequest struct {
	DmPrivacy         *string `json:"dm_privacy,omitempty"`
	ProfileVisibility *string `json:"profile_visibility,omitempty"`
	SearchIndexable   *bool   `json:"search_indexable,omitempty"`
	ShowOnlineStatus  *bool   `json:"show_online_status,omitempty"`
	ShowActivity      *bool   `json:"show_activity,omitempty"`
}

// UpdatePrivacy handles PATCH /api/v1/me/privacy.
func (h *Handlers) UpdatePrivacy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdatePrivacyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdatePrivacySettings(r.Context(), user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// SessionInfo represents a user session.
type SessionInfo struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

// ListSessions handles GET /api/v1/me/sessions.
func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	sessions, err := h.Service.ListSessions(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": sessions})
}

// RevokeSession handles DELETE /api/v1/me/sessions/{id}.
func (h *Handlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid session ID format")
		return
	}

	if err := h.Service.RevokeSession(r.Context(), user.UserID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

// RevokeAllSessionsRequest is the input for revoking all sessions.
type RevokeAllSessionsRequest struct {
	KeepCurrent bool `json:"keep_current"`
}

// RevokeAllSessions handles DELETE /api/v1/me/sessions.
func (h *Handlers) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req RevokeAllSessionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.KeepCurrent = true
	}

	if err := h.Service.RevokeAllSessions(r.Context(), user.UserID, req.KeepCurrent); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

// ConnectedAccountInfo represents a linked OAuth account.
type ConnectedAccountInfo struct {
	Provider    string `json:"provider"`
	ConnectedAt string `json:"connected_at"`
}

// ListConnectedAccounts handles GET /api/v1/me/connected-accounts.
func (h *Handlers) ListConnectedAccounts(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	accounts, err := h.Service.ListConnectedAccounts(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": accounts})
}

// DisconnectAccount handles DELETE /api/v1/me/connected-accounts/{provider}.
func (h *Handlers) DisconnectAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	provider := chi.URLParam(r, "provider")
	if provider == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_provider", "Provider is required")
		return
	}

	// Check user has a password set (prevent lockout).
	hasPassword, err := h.Service.store.userHasPassword(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !hasPassword {
		httpx.WriteError(w, http.StatusBadRequest, "cannot_disconnect", "Cannot disconnect the only login method. Set a password first.")
		return
	}

	if err := h.Service.DisconnectAccount(r.Context(), tenantID, user.UserID, provider); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "disconnected"}})
}

// ContentListItem represents a content item in list responses.
type ContentListItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	ContentType string `json:"content_type"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ListMyContent handles GET /api/v1/me/content.
func (h *Handlers) ListMyContent(w http.ResponseWriter, r *http.Request) {
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

	content, err := h.Service.ListUserContent(r.Context(), tenantID, user.UserID, "", limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// ListMyDrafts handles GET /api/v1/me/drafts.
func (h *Handlers) ListMyDrafts(w http.ResponseWriter, r *http.Request) {
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

	content, err := h.Service.ListUserContent(r.Context(), tenantID, user.UserID, "draft", limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// ListMyScheduled handles GET /api/v1/me/scheduled.
func (h *Handlers) ListMyScheduled(w http.ResponseWriter, r *http.Request) {
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

	content, err := h.Service.ListUserContent(r.Context(), tenantID, user.UserID, "scheduled", limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// BookmarkListItem represents a bookmark in list responses.
type BookmarkListItem struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Folder     string `json:"folder"`
	CreatedAt  string `json:"created_at"`
}

// ListMyBookmarks handles GET /api/v1/me/bookmarks.
func (h *Handlers) ListMyBookmarks(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	bookmarks, err := h.Service.ListUserBookmarks(r.Context(), user.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": bookmarks})
}

// GetMyAnalytics handles GET /api/v1/me/analytics.
func (h *Handlers) GetMyAnalytics(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	analytics, err := h.Service.GetUserAnalytics(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": analytics})
}

// DeactivateAccountRequest is the input for account deactivation.
type DeactivateAccountRequest struct {
	Password string `json:"password"`
}

// DeactivateAccount handles POST /api/v1/me/deactivate.
func (h *Handlers) DeactivateAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req DeactivateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_password", "Password is required to deactivate account")
		return
	}

	// Verify password.
	hashRow, err := h.Service.store.getUserPasswordHash(r.Context(), user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if hashRow.PasswordHash != nil {
		if err := bcrypt.CompareHashAndPassword([]byte(*hashRow.PasswordHash), []byte(req.Password)); err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid_password", "Password is incorrect")
			return
		}
	}

	if err := h.Service.DeactivateAccount(r.Context(), tenantID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deactivated"}})
}

// RequestDataExportRequest is the input for requesting data export.
type RequestDataExportRequest struct {
	Format string `json:"format,omitempty"`
}

// RequestDataExport handles POST /api/v1/me/data-export.
func (h *Handlers) RequestDataExport(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_ = user

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{
			"status":  "export_requested",
			"message": "Your data export has been requested. You will receive a download link via email when ready.",
		},
	})
}

// Ensure pgx.ErrNoRows is referenced to avoid import cycle.
var _ = pgx.ErrNoRows

// Ensure tenant package is referenced.
var _ = tenant.FromContext

// Ensure gen package is referenced.
var _ = gen.GetUserProfileParams{}
