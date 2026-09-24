package users

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
)

// ChangePasswordRequest is the input for changing password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword handles POST /api/v1/me/change-password.
// Validates the current password, enforces the platform's password policy
// (from system_settings), checks password history, hashes with bcrypt cost 12,
// records the new hash in password_history, and invalidates all sessions
// to force re-login on all devices.
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md.
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "current_password and new_password are required")
		return
	}

	// Build the password policy from system_settings (auth.password.* keys).
	// Falls back to defaults when settings are unavailable.
	policy := auth.DefaultPasswordPolicy()
	if h.authSettings != nil {
		policy = auth.PasswordPolicyFromSettings(r.Context(), h.authSettings)
	}

	// Validate the new password against the full policy (length, complexity,
	// breach check) — not just a hardcoded min length.
	if err := auth.ValidatePassword(req.NewPassword, policy); err != nil {
		var breached *auth.BreachedError
		if errors.As(err, &breached) {
			slog.Warn("change-password blocked: breached password",
				"user_id", user.UserID.String(),
				"breach_count", breached.Count)
			errpkg.WriteError(w, http.StatusBadRequest, "password_breached",
				"This password has appeared in a data breach and cannot be used. Please choose a different password.")

			return
		}

		errpkg.WriteError(w, http.StatusBadRequest, "weak_password", err.Error())

		return
	}

	// Get current password hash.
	hashRow, err := h.Service.store.getUserPasswordHash(r.Context(), user.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			return
		}

		handleServiceError(w, err)

		return
	}

	// Verify current password.
	if hashRow.PasswordHash != nil {
		if err := bcrypt.CompareHashAndPassword([]byte(*hashRow.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_password", "Current password is incorrect")
			return
		}
	} else {
		// OAuth-only accounts (password_hash IS NULL) cannot use password change.
		errpkg.WriteError(w, http.StatusBadRequest, "no_password_set", "This account does not have a password set. Use password reset instead.")
		return
	}

	// Check password history to prevent reuse of recent passwords.
	if err := auth.CheckPasswordHistory(r.Context(), h.Service.store, user.UserID, req.NewPassword, policy); err != nil {
		var reused *auth.PasswordReusedError
		if errors.As(err, &reused) {
			errpkg.WriteError(w, http.StatusBadRequest, "password_reused",
				"New password cannot be the same as a recent password. Please choose a different password.")
			return
		}
	}

	// Hash new password with bcrypt cost 12 (auth.BcryptCost), not
	// bcrypt.DefaultCost (10).
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to hash password")
		return
	}

	if err := h.Service.store.updateUserPassword(r.Context(), user.UserID, &newHash); err != nil {
		handleServiceError(w, err)
		return
	}

	// Record the new password in history so it cannot be reused later.
	if err := h.Service.store.insertPasswordHistory(r.Context(), user.UserID, newHash); err != nil {
		slog.Warn("change-password: failed to record password history", "error", err)
	}

	// Invalidate all sessions for this user — force re-login on all devices.
	// Ref: §02 "Session invalidation on password change"
	if err := h.Service.store.revokeAllRefreshTokens(r.Context(), user.UserID); err != nil {
		slog.Error("change-password: failed to invalidate sessions",
			"user_id", user.UserID.String(), "error", err)
	}

	// Revoke the current access token's JTI so it is immediately invalid,
	// not just after the 15-minute TTL expires. Same pattern as the logout
	// handler (internal/auth/handlers.go Logout).
	if h.keyRot != nil && h.Redis != nil {
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				if claims, err := h.keyRot.ValidateAccessToken(parts[1]); err == nil {
					if err := auth.RevokeToken(r.Context(), claims.JTI, claims.ExpiresAt.Time, h.Redis); err != nil {
						slog.Error("change-password: revoke access token JTI", "error", err)
					}
				}
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// PrivacySettings represents user privacy settings.
type PrivacySettings struct {
	DmPrivacy          string `json:"dm_privacy"`
	ProfileVisibility  string `json:"profile_visibility"`
	SearchIndexable    bool   `json:"search_indexable"`
	ShowOnlineStatus   bool   `json:"show_online_status"`
	ShowActivity       bool   `json:"show_activity"`
	ShowPointsPublicly bool   `json:"show_points_publicly"`
}

// GetPrivacyOptions handles GET /api/v1/me/privacy/options.
func (h *Handlers) GetPrivacyOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	opts := h.Service.GetPrivacyOptions(r.Context())
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": opts})
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": settings})
}

// UpdatePrivacyRequest is the input for updating privacy settings.
type UpdatePrivacyRequest struct {
	DmPrivacy          *string `json:"dm_privacy,omitempty"`
	ProfileVisibility  *string `json:"profile_visibility,omitempty"`
	SearchIndexable    *bool   `json:"search_indexable,omitempty"`
	ShowOnlineStatus   *bool   `json:"show_online_status,omitempty"`
	ShowActivity       *bool   `json:"show_activity,omitempty"`
	ShowPointsPublicly *bool   `json:"show_points_publicly,omitempty"`
}

// UpdatePrivacy handles PATCH /api/v1/me/privacy.
func (h *Handlers) UpdatePrivacy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var req UpdatePrivacyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if err := h.Service.UpdatePrivacySettings(r.Context(), user.UserID, &req); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// SessionInfo represents a user session.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md.
type SessionInfo struct {
	ID         string `json:"id"`
	DeviceName string `json:"device_name"`
	IpAddress  string `json:"ip_address"`
	LastUsedAt string `json:"last_used_at"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	Current    bool   `json:"current"`
}

// ListSessions handles GET /api/v1/me/sessions.
func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	// Resolve the current session from the jc_refresh cookie so the frontend
	// can flag it. Best-effort: a missing/unmatched cookie yields a zero UUID,
	// which means no session is marked current (graceful degradation).
	currentID := h.resolveCurrentSessionID(r.Context(), user.UserID, r)

	sessions, err := h.Service.ListSessions(r.Context(), user.UserID, currentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sessions})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid session ID format")
		return
	}

	if err := h.Service.RevokeSession(r.Context(), user.UserID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

// RevokeAllSessionsRequest is the input for revoking all sessions.
type RevokeAllSessionsRequest struct {
	KeepCurrent bool `json:"keep_current"`
}

// RevokeAllSessions handles DELETE /api/v1/me/sessions.
// Revokes all sessions except the current one (identified by the jc_refresh
// cookie). Per JEOS §50: "revoke all other sessions (keep current one)".
func (h *Handlers) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	currentID := h.resolveCurrentSessionID(r.Context(), user.UserID, r)

	if err := h.Service.RevokeAllSessions(r.Context(), user.UserID, currentID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

// resolveCurrentSessionID reads the jc_refresh cookie, hashes it, and resolves
// the corresponding refresh_tokens.id. Returns a zero UUID when the cookie is
// missing or the token is not found — callers degrade gracefully (no session
// marked current / all sessions revoked).
func (h *Handlers) resolveCurrentSessionID(ctx context.Context, userID pgtype.UUID, r *http.Request) pgtype.UUID {
	cookie, err := r.Cookie("jc_refresh")
	if err != nil || cookie.Value == "" {
		return pgtype.UUID{}
	}

	tokenHash := auth.HashRefreshToken(cookie.Value)

	id, err := h.Service.store.getSessionIDByHash(ctx, userID, tokenHash)
	if err != nil {
		return pgtype.UUID{}
	}

	return id
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

	accounts, err := h.Service.ListConnectedAccounts(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": accounts})
}

// DisconnectAccount handles DELETE /api/v1/me/connected-accounts/{provider}.
func (h *Handlers) DisconnectAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	provider := chi.URLParam(r, "provider")
	if provider == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_provider", "Provider is required")
		return
	}

	// Check user has a password set (prevent lockout).
	hasPassword, err := h.Service.store.userHasPassword(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !hasPassword {
		errpkg.WriteError(w, http.StatusBadRequest, "cannot_disconnect", "Cannot disconnect the only login method. Set a password first.")
		return
	}

	if err := h.Service.DisconnectAccount(r.Context(), user.UserID, provider); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "disconnected"}})
}

// ContentListItem represents a content item in list responses.
type ContentListItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	ContentType  string `json:"content_type"`
	Status       string `json:"status"`
	ViewCount    int32  `json:"view_count"`
	CommentCount int32  `json:"comment_count"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ListMyContent handles GET /api/v1/me/content.
// Returns the caller's published content. Drafts and scheduled posts have
// their own endpoints (/me/drafts, /me/scheduled).
func (h *Handlers) ListMyContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	// §50: /me/content returns published content only. The ?type= query
	// filters by content_type (blog_post, comparison, etc.), ?q= is a
	// free-text search over title/slug, and ?sort= orders the list.
	contentType := r.URL.Query().Get("type")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")

	content, err := h.Service.ListUserContentByStatus(r.Context(), user.UserID, "published", contentType, q, sort, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &content)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// ListMyDrafts handles GET /api/v1/me/drafts.
func (h *Handlers) ListMyDrafts(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	contentType := r.URL.Query().Get("type")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")

	content, err := h.Service.ListUserContentByStatus(r.Context(), user.UserID, "draft", contentType, q, sort, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &content)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
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

	idStr := chi.URLParam(r, "id")

	var contentID pgtype.UUID

	if err := contentID.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid draft ID format")
		return
	}

	draft, err := h.Service.GetUserDraft(r.Context(), user.UserID, contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": draft})
}

// GetMyContent handles GET /api/v1/me/content/{id}. It returns the caller's
// own content by ID (any status — published, scheduled, or draft) so the
// /create composer can restore it for editing. The /me/drafts/{id} route is
// restricted to status='draft'; this route covers the rest.
//
// For non-owners, the handler falls back to a platform-scoped (non-author-
// scoped) fetch when the caller is a super-admin or holds the RBAC
// `content.edit` permission node. This lets editors fix content they don't
// own. The backend still enforces the actual permission on PATCH /content/{id}.
func (h *Handlers) GetMyContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")

	var contentID pgtype.UUID

	if err := contentID.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid content ID format")
		return
	}

	// Try the author-scoped fetch first — the common case (owner editing
	// their own content). pgx.ErrNoRows means the content is missing, not
	// owned by the caller, or soft-deleted.
	content, err := h.Service.GetUserContent(r.Context(), user.UserID, contentID)
	if err == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
		return
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		handleServiceError(w, err)
		return
	}

	// Author-scoped fetch returned 404. Fall back to the platform-scoped
	// fetch only for super-admins or users with the `content.edit_any`
	// permission node. Without a permResolver, fail closed (404).
	if h.permResolver == nil {
		handleServiceError(w, err)
		return
	}

	editorContent, err := h.Service.GetContentForEditor(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	canEditAny := h.permResolver.IsSuperAdmin(r.Context(), user.UserID) ||
		h.permResolver.HasPermission(r.Context(), user.UserID, "content.edit_any")
	if !canEditAny {
		switch editorContent.ContentType {
		case "story":
			canEditAny = h.permResolver.HasPermission(r.Context(), user.UserID, "story.edit_any")
		case "comparison":
			canEditAny = h.permResolver.HasPermission(r.Context(), user.UserID, "comparison.edit_any")
		}
	}
	if !canEditAny {
		handleServiceError(w, pgx.ErrNoRows)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": editorContent})
}

// ListMyScheduled handles GET /api/v1/me/scheduled.
func (h *Handlers) ListMyScheduled(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	contentType := r.URL.Query().Get("type")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")

	content, err := h.Service.ListUserContentByStatus(r.Context(), user.UserID, "scheduled", contentType, q, sort, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &content)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// ListMyInReview handles GET /api/v1/me/in-review. Content held for HITL
// review (e.g. high-stakes categories forced to in_review on save) is not
// a draft and not scheduled, so it needs its own listing — otherwise the
// author cannot see it anywhere in the content manager.
func (h *Handlers) ListMyInReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	contentType := r.URL.Query().Get("type")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")

	content, err := h.Service.ListUserContentByStatus(r.Context(), user.UserID, "in_review", contentType, q, sort, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &content)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// BookmarkListItem represents a bookmark in list responses.
type BookmarkListItem struct {
	ID         string  `json:"id"`
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	Folder     string  `json:"folder"`
	FolderID   *string `json:"folder_id"`
	CreatedAt  string  `json:"created_at"`
	Title      *string `json:"title"`
	Slug       *string `json:"slug"`
	Audience   *string `json:"audience"`
}

// ListMyBookmarks handles GET /api/v1/me/bookmarks.
// Supports optional ?folder_id=uuid filter (BL-002, §50) and
// ?entity_type=<type> filter (so the learn app can request only learning
// entities).
func (h *Handlers) ListMyBookmarks(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	var folderID pgtype.UUID
	if fid := r.URL.Query().Get("folder_id"); fid != "" {
		if err := folderID.Scan(fid); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_folder_id", "Invalid folder_id format")
			return
		}
	}

	entityType := r.URL.Query().Get("entity_type")

	// The entity_type filter accepts a comma-separated list (e.g.
	// ?entity_type=learning_track,content) so the learn app can request
	// multiple learning-related entity types in one call. When a single
	// value is passed, it filters to that one type. When empty, all types
	// are returned.
	var entityTypes []string
	if entityType != "" {
		entityTypes = strings.Split(entityType, ",")
	}

	bookmarks, err := h.Service.ListUserBookmarks(r.Context(), user.UserID, folderID, entityTypes, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &bookmarks)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": bookmarks})
}

// CreateMyBookmark handles POST /api/v1/me/bookmarks.
// Creates a polymorphic bookmark for any entity type (content, doc_page,
// forum_thread, space, etc.). Returns 201 Created with the saved bookmark.
func (h *Handlers) CreateMyBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Folder     string `json:"folder"`
		FolderID   string `json:"folder_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.EntityType == "" || input.EntityID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "entity_type and entity_id are required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(input.EntityID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id format")
		return
	}

	var folderID pgtype.UUID
	if input.FolderID != "" {
		if err := folderID.Scan(input.FolderID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_folder_id", "Invalid folder_id format")
			return
		}
	}

	bm, err := h.Service.CreateBookmark(r.Context(), user.UserID, entityID, folderID, input.EntityType, input.Folder)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": bm})
}

// DeleteMyBookmark handles DELETE /api/v1/me/bookmarks/{id}.
// Removes a bookmark by its row ID — polymorphic, works for any entity_type
// (content, forum_thread, space, comment, learning_track, etc.). Platform +
// owner scoped; returns 404 if the bookmark doesn't exist or belongs to
// another user.
func (h *Handlers) DeleteMyBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")

	var bookmarkID pgtype.UUID

	if err := bookmarkID.Scan(idStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteBookmarkByID(r.Context(), bookmarkID, user.UserID); err != nil {
		if errors.Is(err, errpkg.ErrNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Bookmark not found")
			return
		}

		slog.Warn("delete my bookmark failed", "error", err, "user_id", user.UserID, "bookmark_id", bookmarkID)
		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// GetMyAnalytics handles GET /api/v1/me/analytics.
// Supports period presets (7d, 30d, 90d, custom) for engagement metrics.
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md.
func (h *Handlers) GetMyAnalytics(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	now := time.Now()

	var from, to time.Time

	q := r.URL.Query()

	switch q.Get("period") {
	case "7d":
		from = now.AddDate(0, 0, -7)
	case "90d":
		from = now.AddDate(0, 0, -90)
	case "custom":
		if start, err := time.Parse(time.RFC3339, q.Get("start")); err == nil {
			from = start
		}

		if end, err := time.Parse(time.RFC3339, q.Get("end")); err == nil {
			to = end
		}

		if to.IsZero() {
			to = now
		}
	default:
		from = now.AddDate(0, 0, -30)
	}

	if to.IsZero() {
		to = now
	}

	var fromPtr, toPtr *time.Time
	if !from.IsZero() {
		fromPtr = &from
	}

	if !to.IsZero() {
		toPtr = &to
	}

	analytics, err := h.Service.GetUserAnalytics(r.Context(), user.UserID, fromPtr, toPtr)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": analytics})
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

	var req UpdateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	result, err := h.Service.UpdateAccount(r.Context(), user.UserID, &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrUsernameCooldown):
			errpkg.WriteError(w, http.StatusTooManyRequests, "username_cooldown", "Username can only be changed once every 30 days")
		case errors.Is(err, ErrUsernameTaken):
			errpkg.WriteError(w, http.StatusConflict, "username_taken", "This username is already in use")
		case errors.Is(err, ErrEmailTaken):
			errpkg.WriteError(w, http.StatusConflict, "email_taken", "This email is already in use")
		default:
			handleServiceError(w, err)
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
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

	var req DeactivateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Password == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_password", "Password is required to deactivate account")
		return
	}

	// Verify password.
	hashRow, err := h.Service.store.getUserPasswordHash(r.Context(), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if hashRow.PasswordHash != nil {
		if err := bcrypt.CompareHashAndPassword([]byte(*hashRow.PasswordHash), []byte(req.Password)); err != nil {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_password", "Password is incorrect")
			return
		}
	} else {
		// OAuth-only accounts (password_hash IS NULL) cannot verify
		// ownership via password. Reject to prevent deactivation by a
		// stolen session token without credential verification.
		errpkg.WriteError(w, http.StatusBadRequest, "no_password_set", "This account does not have a password set. Use password reset to set a password before deactivating.")
		return
	}

	if err := h.Service.DeactivateAccount(r.Context(), user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Invalidate all sessions for this user — a deactivated user must not
	// continue to use the platform on existing tokens. Ref: §02 (Session
	// invalidation), Rule 6 (perm cache invalidation).
	if err := h.Service.store.revokeAllRefreshTokens(r.Context(), user.UserID); err != nil {
		slog.Error("deactivate: failed to invalidate sessions",
			"user_id", user.UserID.String(), "error", err)
	}
	// Invalidate the permission cache so cached permissions don't outlive
	// the deactivation (Rule 6).
	if h.Redis != nil {
		_ = h.Redis.Del(r.Context(), valkey.MustKey("perm:%s", user.UserID.String())).Err()
	}
	// Revoke the current access token's JTI so it is immediately invalid.
	if h.keyRot != nil && h.Redis != nil {
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				if claims, err := h.keyRot.ValidateAccessToken(parts[1]); err == nil {
					if err := auth.RevokeToken(r.Context(), claims.JTI, claims.ExpiresAt.Time, h.Redis); err != nil {
						slog.Error("deactivate: revoke access token JTI", "error", err)
					}
				}
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deactivated"}})
}

// Ensure pgx.ErrNoRows is referenced to avoid import cycle.
var _ = pgx.ErrNoRows

// Ensure gen package is referenced.
var _ = gen.GetUserProfileRow{}

// GetMyContentAnalytics handles GET /api/v1/me/analytics/content.
// Returns per-content analytics breakdown for the authenticated user.
// Supports period presets (7d, 30d, 90d, custom) via the same semantics as
// GET /me/analytics. Without a period, returns all-time ranking.
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md.
func (h *Handlers) GetMyContentAnalytics(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	q := r.URL.Query()
	period := q.Get("period")

	now := time.Now()

	var from, to time.Time

	switch period {
	case "7d":
		from = now.AddDate(0, 0, -7)
	case "30d":
		from = now.AddDate(0, 0, -30)
	case "90d":
		from = now.AddDate(0, 0, -90)
	case "custom":
		if start, err := time.Parse(time.RFC3339, q.Get("start")); err == nil {
			from = start
		}

		if end, err := time.Parse(time.RFC3339, q.Get("end")); err == nil {
			to = end
		}

		if to.IsZero() {
			to = now
		}
	}

	if to.IsZero() {
		to = now
	}

	fromTs := pgtype.Timestamptz{}
	toTs := pgtype.Timestamptz{}

	if !from.IsZero() {
		fromTs = pgtype.Timestamptz{Time: from, Valid: true}
	}

	if !to.IsZero() {
		toTs = pgtype.Timestamptz{Time: to, Valid: true}
	}

	items, err := h.Service.GetUserContentByPeriod(r.Context(), user.UserID, fromTs, toTs, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if items == nil {
		items = []gen.GetUserContentByPeriodRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetMyFollowerGrowth handles GET /api/v1/me/analytics/followers.
// Returns daily follower growth for the authenticated user.
// Ref: japaconnect-engineering/sections/40-platform-analytics-site-reports.md.
func (h *Handlers) GetMyFollowerGrowth(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	days := 30

	if s := r.URL.Query().Get("days"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	growth, err := h.Service.GetUserFollowerGrowth(r.Context(), user.UserID, days)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if growth == nil {
		growth = []gen.GetUserFollowerGrowthRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": growth})
}
