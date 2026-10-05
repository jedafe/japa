package spaces

import (
	"net/http"
	"strconv"

	"github.com/jedafe/japa/internal/auth"
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
	if h.Service.IsWikiOnlyType(r.Context(), space.SpaceTypeID) {
		errpkg.WriteError(w, http.StatusConflict, "wiki_only_space",
			"This space is wiki-only and does not accept members")
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
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	members, err := h.Service.ListMembers(r.Context(), space.ID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
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
