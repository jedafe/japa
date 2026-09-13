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
// // Usage:
// //	r.With(rbac.RequirePermission(rbacResolver, "blog.publish")).Post("/posts", handler)
func RequirePermission(resolver *RBAC, permKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
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

// RequireAnyPermission is middleware that checks if the authenticated user
// has at least one of the given permissions. This is used for routes that
// accept either an "own" scope (e.g. comparison.edit_own) or an "any" scope
// (e.g. comparison.edit_any). The handler is responsible for enforcing
// ownership when the user only has the "own" scope. Must be used after
// RequireAuth middleware.
// // Usage:
// //	r.With(rbac.RequireAnyPermission(rbacResolver, "comparison.edit_own", "comparison.edit_any")).
// Patch("/comparisons/{id}", handler)
func RequireAnyPermission(resolver *RBAC, permKeys ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
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

			for _, key := range permKeys {
				if resolver.HasPermission(r.Context(), user.UserID, user.TenantID, key) {
					ctx := context.WithValue(r.Context(), ResolverKey, resolver)
					next.ServeHTTP(w, r.WithContext(ctx))

					return
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"You do not have permission to perform this action"}}`))
		})
	}
}

// RequirePermissionWithInvalidation is middleware that checks permission for
// routes that modify group/role assignments (grant, revoke, moderate).
// // Note: This middleware does NOT invalidate the requesting user's cache —
// the requesting user's permissions do not change when they assign a
// moderator or modify a role/group. The handler that processes the mutation
// is responsible for invalidating the TARGET user's cache (e.g., the spaces
// handler calls invalidateTargetPermCache after a moderator assignment).
// This middleware is kept as a semantic marker for mutation routes; its
// permission-check behavior is identical to RequirePermission.
func RequirePermissionWithInvalidation(resolver *RBAC, permKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
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
// // Behavior:
// - Guest (no user in context): allowed (preserves public/SEO access)
// - Authed user with permission: allowed
// - Authed user without permission: 403 forbidden
// - Super admin: always allowed
// // Usage:
// //	r.With(auth.OptionalAuth(kr, vk), rbac.OptionalPermission(rbacResolver, "directory.users.view")).
// Get("/users", handler)
// // Ref: japaconnect-engineering/sections/03-rbac.md (Directory visibility nodes).
func OptionalPermission(resolver *RBAC, permKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			// Guests (no user or invalid user ID) pass through — the
			// directory is public for SEO and anonymous browsing. The
			// permission only gates authed users whose groups have had
			// the node revoked.
			if user == nil || !user.UserID.Valid {
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
// 1. A space resolved by the spaces.SpaceResolver middleware (stored in
// context — used by {slug} routes)
// 2. The chi URL parameter "space_id"
// 3. The chi URL parameter "id"
// // It then delegates to HasSpacePermission, which dynamically resolves
// "spaces.moderate" via the space_moderators table (Section 03 / RBAC-001):
// an assigned moderator can moderate the specific space they were assigned
// to without holding the static "spaces.moderate" node.
// // Usage:
// //	r.With(rbac.RequireSpacePermission(rbacResolver, "spaces.moderate")).
// Delete("/spaces/{slug}/rules/{ruleId}", handler)
func RequireSpacePermission(resolver *RBAC, permKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if user == nil || !user.UserID.Valid {
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
