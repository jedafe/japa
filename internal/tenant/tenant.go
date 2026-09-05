// Package tenant provides middleware and context helpers for resolving
// the current tenant from incoming HTTP requests.
//
// At v1 launch JapaConnect runs as a single-tenant deployment, but the
// architecture supports multi-tenancy from day one. The TenantMiddleware
// resolves the tenant from the request host header and stores it in the
// request context for downstream handlers.
package tenant

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
)

// contextKey is an unexported type used to avoid context key collisions.
type contextKey struct {
	name string
}

// TenantContextKey is the context key for the resolved tenant.
var TenantContextKey = contextKey{"tenant"}

// Tenant represents a resolved tenant from the database.
type Tenant struct {
	ID      pgtype.UUID
	Name    string
	Slug    string
	Domain  pgtype.Text
	Status  string
	BaseURL string
}

// FromContext extracts the Tenant from the request context.
// Returns nil if no tenant was set.
func FromContext(ctx context.Context) *Tenant {
	t, _ := ctx.Value(TenantContextKey).(*Tenant)
	return t
}

// TenantStore is the interface for looking up tenants by host header.
type TenantStore interface { //nolint:revive // stuttering type name preserved for API compatibility
	GetTenantByHost(ctx context.Context, host string) (*Tenant, error)
	GetDefaultTenant(ctx context.Context) (*Tenant, error)
}

// Middleware resolves the tenant from the request and stores it in context.
// It tries the Host header first, then falls back to the default tenant.
// Only active tenants are served; suspended or soft-deleted tenants are
// skipped so requests are never routed to a dead tenant.
func Middleware(store TenantStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			var t *Tenant

			host := r.Host
			if host != "" {
				resolved, err := store.GetTenantByHost(ctx, host)
				if err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error":{"code":"tenant_unavailable","message":"Tenant resolution failed"}}`))

					return
				}

				t = resolved
			}

			if t == nil {
				resolved, err := store.GetDefaultTenant(ctx)
				if err != nil || resolved == nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error":{"code":"tenant_unavailable","message":"No tenant available"}}`))

					return
				}

				t = resolved
			}

			ctx = context.WithValue(ctx, TenantContextKey, t)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SingleTenantMiddleware returns middleware that always injects the given
// tenant into the request context, with no per-request database lookup.
//
// Use this when the platform runs as a single-tenant deployment (ADR-004
// amendment): the default tenant is resolved once at boot and reused for
// every request, eliminating the per-request GetTenantByHost query that
// Middleware(store) performs. This removes a DB round-trip from every
// request's hot path.
//
// When a second tenant is provisioned in the future, swap back to
// Middleware(store) for host-based resolution.
func SingleTenantMiddleware(t *Tenant) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), TenantContextKey, t)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IDFromContext returns the tenant ID from the context as pgtype.UUID.
func IDFromContext(ctx context.Context) pgtype.UUID {
	t := FromContext(ctx)
	if t == nil {
		return pgtype.UUID{}
	}

	return t.ID
}
