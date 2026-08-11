// Package interactions — HTTP handlers for user-facing block and mute
// operations (Sections 41, 52).
package interactions

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// MaxPageSize is the upper bound for paginated list queries.
const MaxPageSize int32 = 100

// Handlers holds dependencies for interaction HTTP handlers.
type Handlers struct {
	service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

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

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")

		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

		return nil, false
	}

	return user, true
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID

	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > int(MaxPageSize) {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // bounded
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

	return int32(n) //nolint:gosec // bounded
}

// ---------------------------------------------------------------------------
// User Blocks (Section 41 — user-facing, non-admin)
// ---------------------------------------------------------------------------

// BlockUser handles POST /api/v1/users/{user_id}/block.
// Idempotent: blocking an already-blocked user is a no-op (200).
func (h *Handlers) BlockUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	targetID, err := parseUUIDParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	if user.UserID == targetID {
		writeError(w, http.StatusBadRequest, "self_block", "Cannot block yourself")

		return
	}

	if err := h.service.BlockUser(r.Context(), tenantID, user.UserID, targetID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"blocked": true},
	})
}

// UnblockUser handles DELETE /api/v1/users/{user_id}/block.
// Idempotent: unblocking a non-blocked user is a no-op (200).
func (h *Handlers) UnblockUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	targetID, err := parseUUIDParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	if user.UserID == targetID {
		writeError(w, http.StatusBadRequest, "self_block", "Cannot block yourself")

		return
	}

	if err := h.service.UnblockUser(r.Context(), tenantID, user.UserID, targetID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"blocked": false},
	})
}

// ListBlocked handles GET /api/v1/me/blocked-users.
func (h *Handlers) ListBlocked(w http.ResponseWriter, r *http.Request) {
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

	blocked, err := h.service.ListBlockedUsers(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list blocked users")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": blocked})
}

// ---------------------------------------------------------------------------
// User Mutes (Section 52)
// ---------------------------------------------------------------------------

// MuteUser handles POST /api/v1/users/{user_id}/mute.
//
//nolint:dupl // handler pattern is intentionally consistent with UnmuteUser/MuteSpace
func (h *Handlers) MuteUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	targetID, err := parseUUIDParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	if user.UserID == targetID {
		writeError(w, http.StatusBadRequest, "self_mute", "Cannot mute yourself")

		return
	}

	if err := h.service.MuteUser(r.Context(), tenantID, user.UserID, targetID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mute user")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"muted": true},
	})
}

// UnmuteUser handles DELETE /api/v1/users/{user_id}/mute.
//
//nolint:dupl // handler pattern is intentionally consistent with MuteUser
func (h *Handlers) UnmuteUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	targetID, err := parseUUIDParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	if err := h.service.UnmuteUser(r.Context(), tenantID, user.UserID, targetID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unmute user")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"muted": false},
	})
}

// GetUserMuteStatus handles GET /api/v1/users/{user_id}/mute-status.
// Returns whether the caller has muted the given user.
func (h *Handlers) GetUserMuteStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	targetID, err := parseUUIDParam(r, "user_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	muted, err := h.service.IsUserMuted(r.Context(), tenantID, user.UserID, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to check mute status")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"muted": muted},
	})
}

// ListMutedUsers handles GET /api/v1/me/muted-users.
func (h *Handlers) ListMutedUsers(w http.ResponseWriter, r *http.Request) {
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

	muted, err := h.service.ListMutedUsers(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list muted users")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": muted})
}

// ---------------------------------------------------------------------------
// Space Mutes (Section 52)
// ---------------------------------------------------------------------------

// MuteSpace handles POST /api/v1/spaces/{space_id}/mute.
//
//nolint:dupl // handler pattern is intentionally consistent with MuteUser
func (h *Handlers) MuteSpace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	spaceID, err := parseUUIDParam(r, "space_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")

		return
	}

	if err := h.service.MuteSpace(r.Context(), tenantID, user.UserID, spaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mute space")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"muted": true},
	})
}

// UnmuteSpace handles DELETE /api/v1/spaces/{space_id}/mute.
//
//nolint:dupl // handler pattern is intentionally consistent with MuteUser
func (h *Handlers) UnmuteSpace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	spaceID, err := parseUUIDParam(r, "space_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")

		return
	}

	if err := h.service.UnmuteSpace(r.Context(), tenantID, user.UserID, spaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to unmute space")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"muted": false},
	})
}

// ListMutedSpaces handles GET /api/v1/me/muted-spaces.
func (h *Handlers) ListMutedSpaces(w http.ResponseWriter, r *http.Request) {
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

	muted, err := h.service.ListMutedSpaces(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list muted spaces")

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": muted})
}
