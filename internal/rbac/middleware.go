package rbac

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

// rbacContextKey is used to store the RBAC resolver in request context.
type rbacContextKey struct{ name string }

// ResolverKey is the resolver key.
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
				_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"Authentication required"}}`))

				return
			}

			// Super admins bypass all permission checks.
			if resolver.IsSuperAdmin(r.Context(), user.UserID, user.TenantID) {
				ctx := context.WithValue(r.Context(), ResolverKey, resolver)
				next.ServeHTTP(w, r.WithContext(ctx))

				return
			}

			if !resolver.HasPermission(r.Context(), user.UserID, user.TenantID, permKey) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"You do not have permission to perform this action"}}`))

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
				_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"Authentication required"}}`))

				return
			}

			// Super admins bypass all permission checks.
			if resolver.IsSuperAdmin(r.Context(), user.UserID, user.TenantID) {
				ctx := context.WithValue(r.Context(), ResolverKey, resolver)
				next.ServeHTTP(w, r.WithContext(ctx))
				resolver.InvalidateUserCache(r.Context(), user.UserID)

				return
			}

			if !resolver.HasPermission(r.Context(), user.UserID, user.TenantID, permKey) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"You do not have permission to perform this action"}}`))

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

// OptionalPermission is middleware that checks if the authenticated user has
// a specific permission, but allows unauthenticated (guest) requests to pass
// through. This is used for public directory pages (GET /users, GET /spaces,
// GET /tags, GET /organizations) where guests and authed users with the
// permission may view the page, but authed users whose groups have had the
// permission revoked are denied (403). Must be used after OptionalAuth.
//
// Behavior:
//   - Guest (no user in context): allowed (preserves public/SEO access)
//   - Authed user with permission: allowed
//   - Authed user without permission: 403 forbidden
//   - Super admin: always allowed
//
// Usage:
//
//	r.With(auth.OptionalAuth(kr, vk), rbac.OptionalPermission(rbacResolver, "directory.users.view")).
//	    Get("/users", handler)
//
// Ref: japaconnect-engineering/sections/03-rbac.md (Directory visibility nodes)
func OptionalPermission(resolver *RBAC, permKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			// Guests pass through — the directory is public for SEO and
			// anonymous browsing. The permission only gates authed users
			// whose groups have had the node revoked.
			if user == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Super admins bypass all permission checks.
			if resolver.IsSuperAdmin(r.Context(), user.UserID, user.TenantID) {
				ctx := context.WithValue(r.Context(), ResolverKey, resolver)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if !resolver.HasPermission(r.Context(), user.UserID, user.TenantID, permKey) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"You do not have permission to view this directory"}}`))
				return
			}

			ctx := context.WithValue(r.Context(), ResolverKey, resolver)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SpaceIDContextKey is the context key under which a resolved space's UUID
// is stored by the spaces.SpaceResolver middleware. The rbac package reads
// this to dynamically resolve space-scoped permissions (RBAC-001).
var SpaceIDContextKey = spaceContextKey{"space_id"}

type spaceContextKey struct{ name string }

// spaceFromContext extracts a *gen.Space stored by spaces.SpaceResolver under
// SpaceIDContextKey. Returns a zero (invalid) UUID when no space was set.
func spaceFromContext(r *http.Request) pgtype.UUID {
	id, _ := r.Context().Value(SpaceIDContextKey).(pgtype.UUID)
	return id
}

// RequireSpacePermission is middleware that checks if the authenticated user
// has a space-scoped permission. It resolves the space_id from (in order):
//  1. A space resolved by the spaces.SpaceResolver middleware (stored in
//     context — used by {slug} routes)
//  2. The chi URL parameter "space_id"
//  3. The chi URL parameter "id"
//
// It then delegates to HasSpacePermission, which dynamically resolves
// "spaces.moderator.assign" via the space_moderators table (Section 03 /
// RBAC-001).
//
// Usage:
//
//	r.With(rbac.RequireSpacePermission(rbacResolver, "spaces.moderator.assign")).
//	    Delete("/spaces/{space_id}/posts/{id}", handler)
func RequireSpacePermission(resolver *RBAC, permKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"Authentication required"}}`))
				return
			}

			// Super admins bypass all permission checks.
			if resolver.IsSuperAdmin(r.Context(), user.UserID, user.TenantID) {
				ctx := context.WithValue(r.Context(), ResolverKey, resolver)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Resolve the space ID: prefer a space already resolved by
			// SpaceResolver (from {slug}), then fall back to URL params.
			spaceID := spaceFromContext(r)
			if !spaceID.Valid {
				spaceIDStr := chi.URLParam(r, "space_id")
				if spaceIDStr == "" {
					spaceIDStr = chi.URLParam(r, "id")
				}
				if spaceIDStr != "" {
					_ = spaceID.Scan(spaceIDStr)
				}
			}

			if !resolver.HasSpacePermission(r.Context(), user.UserID, user.TenantID, spaceID, permKey) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"You do not have permission to perform this action"}}`))
				return
			}

			ctx := context.WithValue(r.Context(), ResolverKey, resolver)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
