package spaces

// Ref: .jeos/sections/05-geo-first-space-system.md
// (Amendment addendum 2026-11-27: Space Hub Redesign — sidebar data
// endpoints for the card-driven right rail.)

import (
	"net/http"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// isPrimarySpace reports whether the space row is platform-run (geo levels,
// universities) as opposed to a member-created custom community — mirrors
// the (creator_user_id IS NULL OR level <> 'custom') is_primary column.
func isPrimarySpace(space *gen.GetSpaceBySlugRow) bool {
	if space.IsPrimary != nil {
		return *space.IsPrimary
	}

	// Older shapes may not carry the column — fall back to the level check.
	return space.Level != LevelCustom
}

// ---------------------------------------------------------------------------
// Contributors (public — the hub's "Top contributors" card)
// ---------------------------------------------------------------------------

// ListContributors handles GET /api/v1/spaces/{slug}/contributors?limit=.
func (h *Handlers) ListContributors(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	// BUG-002 parity with ListSpaceContent: contributor names inside a
	// restricted space are restricted too.
	if space.Visibility != VisibilityPublic && !h.canViewRestrictedSpace(r, space.ID) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return
	}

	limit := parsePageSize(r)

	rows, err := h.Service.ListSpaceContributors(r.Context(), space.ID, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list space contributors")
		return
	}
	if rows == nil {
		rows = []gen.ListSpaceContributorsRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// ---------------------------------------------------------------------------
// Leaders (public — the hub's "Space leaders" card)
// ---------------------------------------------------------------------------

// ListLeaders handles GET /api/v1/spaces/{slug}/leaders — owners first,
// then moderators, each row labelled by role.
func (h *Handlers) ListLeaders(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	if space.Visibility != VisibilityPublic && !h.canViewRestrictedSpace(r, space.ID) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return
	}

	limit := parsePageSize(r)

	rows, err := h.Service.ListSpaceLeaders(r.Context(), space.ID, limit)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list space leaders")
		return
	}
	if rows == nil {
		rows = []gen.ListSpaceLeadersRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}
