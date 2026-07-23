//go:build integration

package testdb

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// WithChiURLParam returns a copy of r with a chi URL parameter injected
// into its route context, so handler-level integration tests can exercise
// chi.URLParam(r, key) without running the real router. Packages under
// test should add their own context helpers (tenant/user injection)
// alongside this one, following the existing withTenant pattern in
// internal/users/me_handlers_test.go.
func WithChiURLParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}
