package spaces

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
)

// contextKey is an unexported type used to avoid context key collisions.
type contextKey struct{ name string }

// SpaceContextKey is the context key under which a resolved space is stored
// by the SpaceResolver middleware.
var SpaceContextKey = contextKey{"space"}

// FromContext extracts the resolved space from the request context. It
// returns nil when no space was set, which happens on routes that do not
// run the SpaceResolver middleware.
func FromContext(ctx context.Context) *gen.GetSpaceBySlugRow {
	s, _ := ctx.Value(SpaceContextKey).(*gen.GetSpaceBySlugRow)
	return s
}

// SpaceResolver resolves a space from the {slug} URL parameter and stores
// it in the request context. It must run after platform and auth middleware.
// If the space is not found, it returns 404. Use this on routes that
// operate on a specific space identified by slug.
func SpaceResolver(service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := chi.URLParam(r, "slug")
			if slug == "" {
				next.ServeHTTP(w, r)
				return
			}

			space, err := service.GetSpaceBySlug(r.Context(), slug)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
					return
				}

				errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")

				return
			}

			// BUG-006: non-active spaces are offline for non-managers. The
			// resolver is on every slug-scoped route, so this single gate
			// blocks joins, rules, parents, mutes, etc. for suspended spaces.
			if space.Status != StatusActive {
				allowed := false
				if u := auth.FromContext(r.Context()); u != nil && u.UserID.Valid {
					if ok, cerr := service.CanManageSpace(r.Context(), space.ID, u.UserID); cerr == nil && ok {
						allowed = true
					}
					if !allowed {
						if res := rbac.FromContext(r.Context()); res != nil &&
							res.HasPermission(r.Context(), u.UserID, "spaces.admin") {
							allowed = true
						}
					}
				}
				if !allowed {
					errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
					return
				}
			}

			ctx := context.WithValue(r.Context(), SpaceContextKey, &space)
			// Also store the space ID under rbac.SpaceIDContextKey so the
			// rbac.RequireSpacePermission middleware can dynamically resolve
			// spaces.moderator.assign via the space_moderators table
			// (Section 03 / RBAC-001).
			ctx = context.WithValue(ctx, rbac.SpaceIDContextKey, space.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
