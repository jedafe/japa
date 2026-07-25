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
