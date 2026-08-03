package spaces

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rbac"
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
func FromContext(ctx context.Context) *gen.GetSpaceBySlugRow {
	s, _ := ctx.Value(SpaceContextKey).(*gen.GetSpaceBySlugRow)
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

// writeJSON serializes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response in the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{
		Error: errorBody{Code: code, Message: message},
	})
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
				writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
				return
			}

			space, err := service.GetSpaceBySlug(r.Context(), slug, t.ID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeError(w, http.StatusNotFound, "space_not_found", "Space not found")
					return
				}

				writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")

				return
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
