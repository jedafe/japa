package spaces

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Space Membership Handlers
// ---------------------------------------------------------------------------

// ToggleSpaceMembership handles POST /api/v1/spaces/{slug}/join.
// Toggles membership: joins if not a member, leaves if already a member.
func (h *Handlers) ToggleSpaceMembership(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	// §80 wiki-only space types have no joinable community surface.
	if h.rejectWikiOnly(w, r, space) {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	isMember, err := h.Service.IsMember(r.Context(), space.ID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if isMember {
		if err := h.Service.Leave(r.Context(), space.ID, user.UserID); err != nil {
			handleServiceError(w, err)
			return
		}

		if err := h.Service.IncrementSpaceCount(r.Context(), space.ID, -1); err != nil {
			handleServiceError(w, err)
			return
		}
	} else {
		// BUG-004: honor the space's joining_perm axis (Section 05 lines 358-360).
		// - 'open': immediate membership (existing behavior).
		// - 'request': insert a pending join request (status='pending') for
		//   admin approval; do NOT increment member_count until approved.
		// - 'invite': only invited users may join; reject otherwise.
		switch space.JoiningPerm {
		case JoiningRequest:
			if err := h.Service.JoinPending(r.Context(), space.ID, user.UserID); err != nil {
				handleServiceError(w, err)
				return
			}

			errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
				"data": map[string]any{
					"joined":       false,
					"pending":      true,
					"member_count": 0, // unchanged — pending requests don't count
				},
			})

			return
		case JoiningInvite:
			errpkg.WriteError(w, http.StatusForbidden, "invite_only",
				"This space is invite-only; you cannot join without an invitation")
			return
		default:
			// JoiningOpen — immediate membership.
			if err := h.Service.Join(r.Context(), space.ID, user.UserID); err != nil {
				handleServiceError(w, err)
				return
			}

			// BL-010 mirror: joining auto-follows the space server-side so
			// every join path (UI, API clients, admin grants) produces the
			// follow, not just the frontend's best-effort POST. Ensure
			// semantics make the client's duplicate call a harmless no-op.
			if h.follows != nil {
				if _, ferr := h.follows.EnsureFollow(r.Context(), user.UserID, "space", space.ID); ferr != nil {
					slog.Warn("join: auto-follow failed", "space_id", space.ID, "user_id", user.UserID, "error", ferr)
				}
			}

			if err := h.Service.IncrementSpaceCount(r.Context(), space.ID, 1); err != nil {
				handleServiceError(w, err)
				return
			}
		}
	}

	memberCount, err := h.Service.CountMembers(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"joined":       !isMember,
			"member_count": memberCount,
		},
	})
}

// Leave handles DELETE /api/v1/spaces/{slug}/leave — dedicated leave action,
// split from the join/leave toggle in ToggleSpaceMembership.
// SPACES-016: only decrement member_count if the user was an approved
// member. A pending member's count was never incremented, so decrementing
// on their leave would cause negative drift.
func (h *Handlers) Leave(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// Check if the user is an approved member before removing. Pending
	// members (status='pending') are removed but don't affect the count.
	wasApproved, err := h.Service.IsMember(r.Context(), space.ID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if err := h.Service.Leave(r.Context(), space.ID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	// SPACES-016: only decrement if the user was an approved member.
	if wasApproved {
		if err := h.Service.IncrementSpaceCount(r.Context(), space.ID, -1); err != nil {
			handleServiceError(w, err)
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"left": true}})
}

// ---------------------------------------------------------------------------
// Space Members Listing Handler
// ---------------------------------------------------------------------------

// ListMembers handles GET /api/v1/spaces/{slug}/members.
// Returns paginated members of the space with user profile data.
//
// Visibility gate: a private space's roster is only visible to its members
// (or a holder of the spaces.moderate node). Outsiders — including
// authenticated non-members and guests — receive 404 (not 403) so the
// endpoint cannot be used to probe for the existence of private spaces.
// Public spaces stay open. The member rows carry no email addresses, so no
// additional redaction is needed for non-members of public spaces.
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	if space.Visibility == VisibilityPrivate && !h.canViewPrivateMembers(r, space) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	var viewerID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		viewerID = u.UserID
	}

	members, err := h.Service.ListMembers(r.Context(), viewerID, space.ID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
}

// canViewPrivateMembers reports whether the caller may list the members of a
// private space: an approved member of the space, or a holder of the
// spaces.moderate node. The moderate bypass mirrors
// rbac.HasSpacePermission semantics: the static node, the space owner, or an
// assigned space moderator. Fail-closed: any check error denies.
func (h *Handlers) canViewPrivateMembers(r *http.Request, space *gen.GetSpaceBySlugRow) bool {
	user := auth.FromContext(r.Context())
	if user == nil || !user.UserID.Valid {
		return false
	}

	ctx := r.Context()

	isMember, err := h.Service.IsMember(ctx, space.ID, user.UserID)
	if err == nil && isMember {
		return true
	}

	if isMod, merr := h.Service.IsModerator(ctx, space.ID, user.UserID); merr == nil && isMod {
		return true
	}

	if isOwner, oerr := h.Service.IsOwner(ctx, space.ID, user.UserID); oerr == nil && isOwner {
		return true
	}

	if h.permChecker != nil && h.permChecker.HasPermission(ctx, user.UserID, "spaces.moderate") {
		return true
	}

	return false
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return 20
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 20
	}

	if n > 100 {
		return 100
	}

	return int32(n) // #nosec G109 G115 -- bounded by MaxPageSize check above
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) // #nosec G109 G115 -- bounded by non-negative check above
}

// parseMeSpacesLimit extracts the page_size query parameter for GET
// /api/v1/me/spaces. The default is intentionally high so the shell nav can
// request a small page while existing full-list callers keep working.
func parseMeSpacesLimit(r *http.Request) int32 {
	const defaultLimit = 10000

	const maxLimit = 10000

	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultLimit
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultLimit
	}

	if n > maxLimit {
		return maxLimit
	}

	return int32(n) // #nosec G109 G115 -- bounded by MaxPageSize check above
}

// parseMeSpacesOffset extracts the page_offset query parameter for GET
// /api/v1/me/spaces.
func parseMeSpacesOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) // #nosec G109 G115 -- bounded by non-negative check above
}
