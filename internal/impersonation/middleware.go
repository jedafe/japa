package impersonation

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

// adminAPIPrefix is the path prefix for admin routes. The impersonation
// middleware skips these so an admin can still manage impersonation
// sessions (start/end/status) with their real identity (Section 46:
// "wraps all non-admin routes").
const adminAPIPrefix = "/api/v1/admin/"

// ImpersonationMiddleware extracts the impersonation state from Valkey
// and injects it into the request context. The real admin identity stays in
// auth.UserContextKey for audit logging; the resolved effective user
// (target when impersonating, else the real user) is additionally placed
// under EffectiveUserKey for downstream enforcement (ban checks, RBAC).
//
// Must be registered AFTER auth middleware (OptionalAuth/RequireAuth) so
// the admin's identity is populated in context before the Valkey lookup.
// Skips /api/v1/admin/ routes so the admin retains their real identity
// for impersonation management endpoints.
func ImpersonationMiddleware(store *Store) func(http.Handler) http.Handler { //nolint:revive // stuttering type name preserved for API compatibility
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip admin routes — the admin must act as themselves to
			// manage impersonation sessions (Section 46).
			if strings.HasPrefix(r.URL.Path, adminAPIPrefix) {
				next.ServeHTTP(w, r)
				return
			}

			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
				next.ServeHTTP(w, r)
				return
			}

			// Default effective identity: the real user. Always set so
			// downstream enforcement can rely on EffectiveUserKey.
			ctx := context.WithValue(r.Context(), EffectiveUserKey, user.UserID)

			targetID, err := store.GetImpersonation(r.Context(), user.UserID)
			if err != nil {
				slog.Warn("impersonation middleware: failed to get status", "error", err)
				next.ServeHTTP(w, r.WithContext(ctx))

				return
			}

			if targetID == "" {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			var impersonatedID pgtype.UUID
			if err := impersonatedID.Scan(targetID); err != nil {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			ctx = context.WithValue(ctx, impersonatedUserKey, &ImpersonationContext{
				AdminID:        user.UserID,
				ImpersonatedID: impersonatedID,
			})
			// Effective identity switches to the impersonated target; the
			// real admin remains in auth.UserContextKey for audit.
			ctx = context.WithValue(ctx, EffectiveUserKey, impersonatedID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
