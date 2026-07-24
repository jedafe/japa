package spaces

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

// contextKey is an unexported type used to avoid context key collisions.
type contextKey struct{ name string }

// SpaceContextKey is the context key under which a resolved space is stored
// by the SpaceResolver middleware.
var SpaceContextKey = contextKey{"space"}

// FromContext extracts the resolved space from the request context. It
// returns nil when no space was set, which happens on routes that do not
// run the SpaceResolver middleware.
func FromContext(ctx context.Context) *gen.Space {
	s, _ := ctx.Value(SpaceContextKey).(*gen.Space)
	return s
}

// errorResponse is the JSON body returned for error responses, following the
// JapaConnect API error format: {"error": {"code": "...", "message": "..."}}.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SpaceResolver resolves a space from the {slug} URL parameter and stores
// it in the request context. It must run after tenant and auth middleware.
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

			t := tenant.FromContext(r.Context())
			if t == nil {
				httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
				return
			}

			space, err := service.GetSpaceBySlug(r.Context(), slug, t.ID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					httpx.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
					return
				}

				httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")

				return
			}

			ctx := context.WithValue(r.Context(), SpaceContextKey, &space)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
