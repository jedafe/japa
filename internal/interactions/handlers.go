// Package interactions — HTTP handlers for user-facing block and mute
// operations (Sections 41, 52).
package interactions

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
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

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")

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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
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

	// #nosec G109 // value bounded by MaxPageSize check
	// #nosec G115 // value bounded by caller
	return int32(n)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	if user.UserID == targetID {
		errpkg.WriteError(w, http.StatusBadRequest, "self_block", "Cannot block yourself")

		return
	}

	if err := h.service.BlockUser(r.Context(), tenantID, user.UserID, targetID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	if user.UserID == targetID {
		errpkg.WriteError(w, http.StatusBadRequest, "self_block", "Cannot block yourself")

		return
	}

	if err := h.service.UnblockUser(r.Context(), tenantID, user.UserID, targetID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list blocked users")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": blocked})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	if user.UserID == targetID {
		errpkg.WriteError(w, http.StatusBadRequest, "self_mute", "Cannot mute yourself")

		return
	}

	if err := h.service.MuteUser(r.Context(), tenantID, user.UserID, targetID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to mute user")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"muted": true},
	})
}

// UnmuteUser handles DELETE /api/v1/users/{user_id}/mute.
//

// UnmuteUser is an exported method.
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	if err := h.service.UnmuteUser(r.Context(), tenantID, user.UserID, targetID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to unmute user")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")

		return
	}

	muted, err := h.service.IsUserMuted(r.Context(), tenantID, user.UserID, targetID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check mute status")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list muted users")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": muted})
}

// ---------------------------------------------------------------------------
// Space Mutes (Section 52) — list only
// ---------------------------------------------------------------------------
// Space mute/unmute *actions* (POST/DELETE /spaces/{slug}/mute) are handled by
// internal/spaces, which resolves the space by slug before calling the shared
// sqlc queries. This package only owns the list endpoint below.

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

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	level := strings.TrimSpace(r.URL.Query().Get("level"))

	// Optional space_type filter. Empty or invalid values fall back to the
	// zero UUID sentinel, which the query treats as "no type filter".
	var spaceTypeID pgtype.UUID
	if err := spaceTypeID.Scan(r.URL.Query().Get("space_type")); err != nil || !spaceTypeID.Valid {
		spaceTypeID = pgtype.UUID{Valid: true}
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	muted, err := h.service.ListMutedSpaces(r.Context(), tenantID, user.UserID, q, level, spaceTypeID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list muted spaces")

		return
	}

	if muted == nil {
		muted = []MutedSpaceInfo{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": muted})
}
