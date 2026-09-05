package spaces

import (
	"net/http"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// =====================
// Follow / Unfollow
// =====================

// ToggleFollow handles POST/DELETE /api/v1/spaces/{slug}/follow.
// Toggles the authenticated user's follow status for the space.
func (h *Handlers) ToggleFollow(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	// H3: route through the generic follows service when available, so
	// auto-join for open spaces (BL-010) and notification dispatch are
	// applied consistently. Fall back to the space-specific path only when
	// the generic service is not wired (backward compatibility).
	var followed bool

	var err error
	if h.follows != nil {
		followed, err = h.follows.ToggleFollow(r.Context(), tenantID, user.UserID, "space", space.ID)
	} else {
		followed, err = h.Service.ToggleFollowSpace(r.Context(), tenantID, user.UserID, space.ID)
	}

	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Return the actual post-toggle state. The `followed` bool is true when
	// the user is now following, false when now unfollowed. Hardcoding false
	// on DELETE would misreport the state when the user wasn't following
	// before (the toggle follows them but the response says false).
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"following": followed}})
}

// =====================
// Mute / Unmute
// =====================

// MuteSpace handles POST /api/v1/spaces/{slug}/mute.
func (h *Handlers) MuteSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.Service.MuteSpace(r.Context(), tenantID, user.UserID, space.ID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"muted": true}})
}

// UnmuteSpace handles DELETE /api/v1/spaces/{slug}/mute.
func (h *Handlers) UnmuteSpace(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.Service.UnmuteSpace(r.Context(), tenantID, user.UserID, space.ID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"muted": false}})
}

// GetMuteStatus handles GET /api/v1/spaces/{slug}/mute-status.
func (h *Handlers) GetMuteStatus(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"muted": false}})
		return
	}

	muted, err := h.Service.IsSpaceMuted(r.Context(), tenantID, user.UserID, space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"muted": muted}})
}
