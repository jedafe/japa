package rbac

import (
	"context"
	"net/http"

	"github.com/jedafe/japa/internal/auth"
)

// rbacContextKey is used to store the RBAC resolver in request context.
type rbacContextKey struct{ name string }

var ResolverKey = rbacContextKey{"rbac_resolver"}

// RequirePermission is middleware that checks if the authenticated user
// has a specific permission. It must be used after RequireAuth middleware.
//
// Usage:
//
//	r.With(rbac.RequirePermission(rbacResolver, "blog.publish")).Post("/posts", handler)
func RequirePermission(resolver *RBAC, permKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"unauthenticated","message":"Authentication required"}}`))
				return
			}

			if !resolver.HasPermission(r.Context(), user.UserID, user.TenantID, permKey) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":{"code":"forbidden","message":"You do not have permission to perform this action"}}`))
				return
			}

			ctx := context.WithValue(r.Context(), ResolverKey, resolver)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermissionWithInvalidation is middleware that checks permission and
// invalidates the user's cache after the handler completes. Use this on
// routes that modify group/role assignments (grant, revoke, moderate).
func RequirePermissionWithInvalidation(resolver *RBAC, permKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"unauthenticated","message":"Authentication required"}}`))
				return
			}

			if !resolver.HasPermission(r.Context(), user.UserID, user.TenantID, permKey) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":{"code":"forbidden","message":"You do not have permission to perform this action"}}`))
				return
			}

			ctx := context.WithValue(r.Context(), ResolverKey, resolver)
			next.ServeHTTP(w, r.WithContext(ctx))

			// After the handler completes, invalidate the requesting user's
			// cache so subsequent permission checks reflect the mutation.
			resolver.InvalidateUserCache(r.Context(), user.UserID)
		})
	}
}

// FromContext extracts the RBAC resolver from the request context.
func FromContext(ctx context.Context) *RBAC {
	r, _ := ctx.Value(ResolverKey).(*RBAC)
	return r
}
