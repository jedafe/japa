package gates

import (
	"net/http"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// RequireSurfaceOpen returns middleware enforcing an api_surface lockdown:
// while the named lockdown flag is enabled, every request under the wrapped
// routes answers 503 surface_locked (§123). Mounted on the /control route
// group before auth so a locked surface reveals nothing — not even the
// shape of its auth requirement.
func RequireSurfaceOpen(flags FlagIO, flag string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if flags != nil && flags.IsEnabled(r.Context(), flag) {
				errpkg.WriteError(w, http.StatusServiceUnavailable, "surface_locked", "This surface is temporarily offline")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
