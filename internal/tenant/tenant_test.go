package tenant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type mockStore struct {
	tenant *Tenant
	err    error
}

func (m *mockStore) GetTenantByHost(_ context.Context, _ string) (*Tenant, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.tenant, nil
}

func (m *mockStore) GetDefaultTenant(_ context.Context) (*Tenant, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.tenant, nil
}

func TestMiddleware_ResolvesTenant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		store      TenantStore
		host       string
		wantStatus int
		wantTenant bool
	}{
		{
			name:       "tenant found by host",
			store:      &mockStore{tenant: &Tenant{Name: "Test", Slug: "test"}},
			host:       "test.japaconnect.com",
			wantStatus: http.StatusOK,
			wantTenant: true,
		},
		{
			name:       "fallback to default tenant",
			store:      &mockStore{tenant: &Tenant{Name: "Default", Slug: "japaconnect"}},
			host:       "unknown.com",
			wantStatus: http.StatusOK,
			wantTenant: true,
		},
		{
			name:       "no tenant available - 503",
			store:      &mockStore{err: context.Canceled},
			host:       "nope.com",
			wantStatus: http.StatusServiceUnavailable,
			wantTenant: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := Middleware(tt.store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tenant := FromContext(r.Context())
				if tt.wantTenant && tenant == nil {
					t.Error("expected tenant in context, got nil")
				}

				if !tt.wantTenant && tenant != nil {
					t.Error("expected no tenant, got one")
				}

				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.host != "" {
				req.Host = tt.host
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestFromContext_NoTenant(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	if tenant := FromContext(ctx); tenant != nil {
		t.Error("expected nil tenant from empty context")
	}
}

func TestIDFromContext_WithTenant(t *testing.T) {
	t.Parallel()

	id := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	ctx := context.WithValue(t.Context(), TenantContextKey, &Tenant{
		ID:   id,
		Name: "Test",
		Slug: "test",
	})

	got := IDFromContext(ctx)
	if !got.Valid {
		t.Error("expected valid UUID")
	}
}
