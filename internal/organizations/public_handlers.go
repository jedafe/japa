package organizations

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetPublicOrganization handles GET /organizations/{slug} (public). Returns the
// lean org header for ACTIVE orgs; non-active/deleted/missing → 404 (the query
// returns no row → handleServiceError maps pgx.ErrNoRows to 404).
func (h *Handlers) GetPublicOrganization(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	org, err := h.service.GetPublicOrganizationBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toPublicOrg(org)})
}

// ListPublicMembers handles GET /organizations/{id}/members (public). The
// active-org gate (requireActiveOrgByID) has already 404'd non-active orgs.
func (h *Handlers) ListPublicMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenantID(w, r)
	if !ok {
		return
	}
	orgID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	rows, err := h.service.ListPublicMembers(r.Context(), orgID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toOrgMembers(rows)})
}
