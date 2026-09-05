package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/tenant"
)

// requireActiveOrgByID gates public org profile tabs keyed by {id}: it 404s
// unless {id} is an ACTIVE, non-deleted org in the request's tenant. Shared
// enforcement point mirroring requireActiveProfileUser. (The header route keys
// by {slug} and self-404s via GetPublicOrganizationBySlug instead.)
func requireActiveOrgByID(orgSvc *organizations.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := tenant.FromContext(r.Context())
			if t == nil {
				writeOrgGateError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
				return
			}

			var orgID pgtype.UUID
			if err := orgID.Scan(chi.URLParam(r, "id")); err != nil {
				writeOrgGateError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID format")
				return
			}

			exists, err := orgSvc.ActiveOrgExists(r.Context(), t.ID, orgID)
			if err != nil {
				writeOrgGateError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve organization")
				return
			}

			if !exists {
				writeOrgGateError(w, http.StatusNotFound, "org_not_found", "Organization not found")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeOrgGateError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
