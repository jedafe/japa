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
	ID     pgtype.UUID
	Name   string
	Slug   string
	Domain pgtype.Text
}

// FromContext extracts the Tenant from the request context.
// Returns nil if no tenant was set.
func FromContext(ctx context.Context) *Tenant {
	t, _ := ctx.Value(TenantContextKey).(*Tenant)
	return t
}

// TenantStore is the interface for looking up tenants by host header.
type TenantStore interface {
	GetTenantByHost(ctx context.Context, host string) (*Tenant, error)
	GetDefaultTenant(ctx context.Context) (*Tenant, error)
}

// Middleware resolves the tenant from the request and stores it in context.
// It tries the Host header first, then falls back to the default tenant.
func Middleware(store TenantStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			var tenant *Tenant

			host := r.Host
			if host != "" {
				t, err := store.GetTenantByHost(ctx, host)
				if err == nil {
					tenant = t
				}
			}

			if tenant == nil {
				t, err := store.GetDefaultTenant(ctx)
				if err != nil || t == nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					w.Write([]byte(`{"error":{"code":"tenant_unavailable","message":"No tenant available"}}`))
					return
				}
				tenant = t
			}

			ctx = context.WithValue(ctx, TenantContextKey, tenant)
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
