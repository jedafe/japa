// Package tenant is a single-tenant compatibility shim. It no longer
// resolves tenants from the database and always returns a zero-value tenant.
package tenant

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
)

// TenantStore is the historical interface for tenant resolution; it is
// preserved as a compatibility no-op.
type TenantStore interface {
	GetTenantByHost(ctx context.Context, host string) (*Tenant, error)
	GetDefaultTenant(ctx context.Context) (*Tenant, error)
}

// DBStore is a no-op TenantStore.
type DBStore struct{}

// NewDBStore returns a no-op tenant store.
func NewDBStore(_ any) *DBStore { return &DBStore{} }

// GetTenantByHost returns the zero tenant.
func (s *DBStore) GetTenantByHost(context.Context, string) (*Tenant, error) { return zero, nil }

// GetDefaultTenant returns the zero tenant.
func (s *DBStore) GetDefaultTenant(context.Context) (*Tenant, error) { return zero, nil }

// contextKey is an unexported type used to avoid context key collisions.
type contextKey struct {
	name string
}

// TenantContextKey is the context key for the resolved tenant.
var TenantContextKey = contextKey{"tenant"}

// Tenant is a placeholder single-tenant record.
type Tenant struct {
	ID      pgtype.UUID
	Name    string
	Slug    string
	Domain  pgtype.Text
	Status  string
	BaseURL string
}

// zero is the singleton zero-value tenant.
var zero = &Tenant{}

// FromContext extracts the Tenant from the request context.
// Returns nil if no tenant was set (preserves historical middleware checks).
func FromContext(ctx context.Context) *Tenant {
	t, _ := ctx.Value(TenantContextKey).(*Tenant)
	return t
}

// Middleware returns middleware that adds a zero-value tenant to the context.
func Middleware(_ any) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), TenantContextKey, zero)))
		})
	}
}

// SingleTenantMiddleware returns middleware that injects the supplied tenant.
func SingleTenantMiddleware(t *Tenant) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), TenantContextKey, t)))
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
