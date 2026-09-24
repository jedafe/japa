package publicapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// optionalScopeRouter mounts OptionalScope on /feeds/{name} the way
// server.go does — the middleware reads the chi {name} param post-routing.
func optionalScopeRouter(anonNames ...string) *chi.Mux {
	r := chi.NewRouter()
	r.With(OptionalScope("api.read.content", anonNames...)).
		Get("/feeds/{name}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	return r
}

func withKey(scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var k pgtype.UUID
			_ = k.Scan("33333333-3333-3333-3333-333333333333")
			next.ServeHTTP(w, r.WithContext(WithAPIKeyContext(r.Context(),
				&APIKeyContext{KeyID: k, UserID: k, Scopes: scopes})))
		})
	}
}

func TestOptionalScopeAnonymous(t *testing.T) {
	r := optionalScopeRouter("popular")

	// Anonymous + allowlisted name → pass.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/feeds/popular", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous popular: got %d", rec.Code)
	}

	// Anonymous + non-allowlisted name → 401.
	for _, name := range []string{"latest", "rising", "bogus"} {
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", "/feeds/"+name, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: expected 401, got %d", name, rec.Code)
		}
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil && body.Error.Code != "api_key_required" {
			t.Fatalf("anonymous %s: wrong error code %q", name, body.Error.Code)
		}
	}
}

func TestOptionalScopeKeyed(t *testing.T) {
	// Keyed requests are scope-enforced exactly like RequireScope —
	// anonymous leniency must not leak into the keyed path.
	r := chi.NewRouter()
	r.Use(withKey("api.read.me")) // key WITHOUT api.read.content
	r.With(OptionalScope("api.read.content", "popular")).
		Get("/feeds/{name}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/feeds/popular", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("keyed without scope: expected 403, got %d", rec.Code)
	}

	// Keyed WITH the scope passes on any name.
	r2 := chi.NewRouter()
	r2.Use(withKey("api.read.content"))
	r2.With(OptionalScope("api.read.content", "popular")).
		Get("/feeds/{name}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	for _, name := range []string{"popular", "latest"} {
		rec = httptest.NewRecorder()
		r2.ServeHTTP(rec, httptest.NewRequest("GET", "/feeds/"+name, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("keyed %s: expected 200, got %d", name, rec.Code)
		}
	}
}

func TestAnonymousRateLimit(t *testing.T) {
	// The inner limiter must run for anonymous requests but be skipped
	// entirely for keyed ones — stacking it would cap a keyed caller at
	// the loose IP limit below its tier.
	var innerCalls int
	inner := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			innerCalls++
			next.ServeHTTP(w, r)
		})
	}
	handler := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

	r := chi.NewRouter()
	r.With(AnonymousRateLimit(inner)).Get("/feeds/{name}", handler)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/feeds/popular", nil))
	if innerCalls != 1 {
		t.Fatalf("anonymous: inner limiter ran %d times, want 1", innerCalls)
	}

	r2 := chi.NewRouter()
	r2.Use(withKey("api.read.content"))
	r2.With(AnonymousRateLimit(inner)).Get("/feeds/{name}", handler)
	rec = httptest.NewRecorder()
	r2.ServeHTTP(rec, httptest.NewRequest("GET", "/feeds/popular", nil))
	if innerCalls != 1 {
		t.Fatalf("keyed: inner limiter ran %d times, want still 1", innerCalls)
	}
}
