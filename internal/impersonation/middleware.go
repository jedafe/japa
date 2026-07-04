package impersonation

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

// ImpersonationMiddleware extracts the impersonation state from Valkey
// and injects it into the request context. The real admin identity is
// preserved for audit logging; downstream handlers can read the
// impersonated user via ImpersonatedFromContext or EffectiveUserID.
func ImpersonationMiddleware(store *Store) func(http.Handler) http.Handler { //nolint:revive // stuttering type name preserved for API compatibility
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
				next.ServeHTTP(w, r)
				return
			}

			targetID, err := store.GetImpersonation(r.Context(), user.UserID)
			if err != nil {
				slog.Warn("impersonation middleware: failed to get status", "error", err)
				next.ServeHTTP(w, r)

				return
			}

			if targetID == "" {
				next.ServeHTTP(w, r)
				return
			}

			var impersonatedID pgtype.UUID
			if err := impersonatedID.Scan(targetID); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), impersonatedUserKey, &ImpersonationContext{
				AdminID:        user.UserID,
				ImpersonatedID: impersonatedID,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
