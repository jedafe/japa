package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/jedafe/japa/internal/db/gen"
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "current_password and new_password are required")
		return
	}

	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password_too_short", "Password must be at least 8 characters")
		return
	}

	// Get current password hash.
	hashRow, err := h.Service.store.getUserPasswordHash(r.Context(), user.UserID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}
		handleServiceError(w, err)
		return
	}

	// Verify current password.
	if hashRow.PasswordHash != nil {
		if err := bcrypt.CompareHashAndPassword([]byte(*hashRow.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_password", "Current password is incorrect")
			return
		}
	}

	// Hash new password.
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to hash password")
		return
	}

	hashStr := string(newHash)
	if err := h.Service.store.updateUserPassword(r.Context(), tenantID, user.UserID, &hashStr); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	settings, err := h.Service.GetPrivacySettings(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req UpdatePrivacyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdatePrivacySettings(r.Context(), tenantID, user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	sessions, err := h.Service.ListSessions(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": sessions})
}

// RevokeSession handles DELETE /api/v1/me/sessions/{id}.
func (h *Handlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid session ID format")
		return
	}

	if err := h.Service.RevokeSession(r.Context(), tenantID, user.UserID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req RevokeAllSessionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.KeepCurrent = true
	}

	if err := h.Service.RevokeAllSessions(r.Context(), tenantID, user.UserID, req.KeepCurrent); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": accounts})
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
		writeError(w, http.StatusBadRequest, "missing_provider", "Provider is required")
		return
	}

	// Check user has a password set (prevent lockout).
	hasPassword, err := h.Service.store.userHasPassword(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !hasPassword {
		writeError(w, http.StatusBadRequest, "cannot_disconnect", "Cannot disconnect the only login method. Set a password first.")
		return
	}

	if err := h.Service.DisconnectAccount(r.Context(), tenantID, user.UserID, provider); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "disconnected"}})
}

// ContentListItem represents a content item in list responses.
type ContentListItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	ContentType string `json:"content_type"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ListMyContent handles GET /api/v1/me/content.
// Returns the caller's published content. Drafts and scheduled posts have
// their own endpoints (/me/drafts, /me/scheduled).
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

	// §50: /me/content returns published content only. The ?type= query
	// filters by content_type (blog_post, comparison, etc.).
	contentType := r.URL.Query().Get("type")
	content, err := h.Service.ListUserContentByStatus(r.Context(), tenantID, user.UserID, "published", contentType, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": content})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": content})
}

// GetMyDraft handles GET /api/v1/me/drafts/{id}. It returns the caller's own
// draft content by ID so the /create composer can restore an existing draft
// for editing. Ownership + draft status are enforced by the store query; a
// mismatch (missing, not-a-draft, or another user's content) resolves to
// pgx.ErrNoRows → 404, so the route never leaks another author's draft.
func (h *Handlers) GetMyDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")
	var contentID pgtype.UUID
	if err := contentID.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid draft ID format")
		return
	}

	draft, err := h.Service.GetUserDraft(r.Context(), tenantID, user.UserID, contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": draft})
}

// GetMyContent handles GET /api/v1/me/content/{id}. It returns the caller's
// own content by ID (any status — published, scheduled, or draft) so the
// /create composer can restore it for editing. The /me/drafts/{id} route is
// restricted to status='draft'; this route covers the rest.
//
// For non-owners, the handler falls back to a tenant-scoped (non-author-
// scoped) fetch when the caller is a super-admin or holds the RBAC
// `content.edit` permission node. This lets editors fix content they don't
// own. The backend still enforces the actual permission on PATCH /content/{id}.
func (h *Handlers) GetMyContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")
	var contentID pgtype.UUID
	if err := contentID.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID format")
		return
	}

	// Try the author-scoped fetch first — the common case (owner editing
	// their own content). pgx.ErrNoRows means the content is missing, not
	// owned by the caller, or soft-deleted.
	content, err := h.Service.GetUserContent(r.Context(), tenantID, user.UserID, contentID)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": content})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		handleServiceError(w, err)
		return
	}

	// Author-scoped fetch returned 404. Fall back to the tenant-scoped
	// fetch only for super-admins or users with the `content.edit`
	// permission node. Without a permResolver, fail closed (404).
	if h.permResolver == nil {
		handleServiceError(w, err)
		return
	}
	canEditAny := h.permResolver.IsSuperAdmin(r.Context(), user.UserID, tenantID) ||
		h.permResolver.HasPermission(r.Context(), user.UserID, tenantID, "content.edit")
	if !canEditAny {
		handleServiceError(w, err)
		return
	}

	editorContent, err := h.Service.GetContentForEditor(r.Context(), tenantID, contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": editorContent})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": content})
}

// BookmarkListItem represents a bookmark in list responses.
type BookmarkListItem struct {
	ID         string  `json:"id"`
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	Folder     string  `json:"folder"`
	CreatedAt  string  `json:"created_at"`
	Title      *string `json:"title"`
	Slug       *string `json:"slug"`
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

	writeJSON(w, http.StatusOK, map[string]any{"data": bookmarks})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": analytics})
}

// UpdateMyAccount handles PATCH /api/v1/me/account.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
// Updates username (once per 30 days), email (triggers re-verification),
// and display_name. All fields are optional (partial update).
func (h *Handlers) UpdateMyAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req UpdateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	result, err := h.Service.UpdateAccount(r.Context(), tenantID, user.UserID, &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrUsernameCooldown):
			writeError(w, http.StatusTooManyRequests, "username_cooldown", "Username can only be changed once every 30 days")
		case errors.Is(err, ErrUsernameTaken):
			writeError(w, http.StatusConflict, "username_taken", "This username is already in use")
		case errors.Is(err, ErrEmailTaken):
			writeError(w, http.StatusConflict, "email_taken", "This email is already in use")
		default:
			handleServiceError(w, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "missing_password", "Password is required to deactivate account")
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
			writeError(w, http.StatusUnauthorized, "invalid_password", "Password is incorrect")
			return
		}
	}

	if err := h.Service.DeactivateAccount(r.Context(), tenantID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deactivated"}})
}

// Ensure pgx.ErrNoRows is referenced to avoid import cycle.
var _ = pgx.ErrNoRows

// Ensure tenant package is referenced.
var _ = tenant.FromContext

// Ensure gen package is referenced.
var _ = gen.GetUserProfileParams{}

// GetMyContentAnalytics handles GET /api/v1/me/analytics/content.
// Returns per-content analytics breakdown for the authenticated user.
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
func (h *Handlers) GetMyContentAnalytics(w http.ResponseWriter, r *http.Request) {
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

	items, err := h.Service.GetUserContentAnalytics(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetMyFollowerGrowth handles GET /api/v1/me/analytics/followers.
// Returns daily follower growth for the authenticated user.
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md
func (h *Handlers) GetMyFollowerGrowth(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	days := 30
	if s := r.URL.Query().Get("days"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	growth, err := h.Service.GetUserFollowerGrowth(r.Context(), tenantID, user.UserID, days)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": growth})
}
