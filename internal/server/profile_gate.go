package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/users"
)

// requireActiveProfileUser gates /users/{id}/* public profile tabs: it 404s
// unless {id} is an ACTIVE user in the request's tenant. This is the single
// enforcement point for "the whole profile 404s for banned/suspended/deleted
// users" — every profile tab route wraps its handler with it.
func requireActiveProfileUser(usersSvc *users.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := tenant.FromContext(r.Context()) // returns *tenant.Tenant, nil if absent
			if t == nil {
				writeProfileGateError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
				return
			}

			var userID pgtype.UUID
			if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
				writeProfileGateError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
				return
			}

			exists, err := usersSvc.ActiveUserExists(r.Context(), t.ID, userID)
			if err != nil {
				writeProfileGateError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve profile")
				return
			}
			if !exists {
				writeProfileGateError(w, http.StatusNotFound, "user_not_found", "User not found")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeProfileGateError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
