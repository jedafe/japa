package middleware

import (
	"net/http"
	"strings"
)

// LegacyForumPathRewrite rewrites the legacy /api/v1/forum* request path to
// the canonical /api/v1/thread* before chi routing. Forum routes were
// renamed /forum → /thread (thread-rename spec); existing API consumers —
// the mobile app, integrations, bookmarks — keep working because both the
// exact path and every subpath are transparently rewritten.
//
// Placed on the root mux (before platform resolution and the /api/v1 mount)
// so the rewrite happens before route matching; downstream middleware and
// handlers only ever see the canonical /thread path.
func LegacyForumPathRewrite(next http.Handler) http.Handler {
	const (
		legacy    = "/api/v1/forum"
		canonical = "/api/v1/thread"
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == legacy:
			r.URL.Path = canonical
		case strings.HasPrefix(r.URL.Path, legacy+"/"):
			r.URL.Path = canonical + r.URL.Path[len(legacy):]
		}
		next.ServeHTTP(w, r)
	})
}
