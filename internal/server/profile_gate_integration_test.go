//go:build integration

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
	"github.com/jedafe/japa/internal/users"
)

// TestRequireActiveProfileUser covers the shared public-profile gate: it must
// pass through for an ACTIVE user in the tenant, 404 for a non-active user
// (banned/suspended/deleted) and for a nonexistent id, and 503 when no tenant
// is resolved. This is the single enforcement point for "the whole profile
// 404s for non-active users" that every /users/{id}/* tab route depends on.
func TestRequireActiveProfileUser(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	activeID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "gate-active"})
	suspendedID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "gate-suspended", Status: "suspended"})

	usersSvc := users.NewService(database)

	var nextCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := requireActiveProfileUser(usersSvc)(next)

	newReq := func(id string, withTenant bool) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/users/"+id+"/media", nil)
		if withTenant {
			req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
		}
		return testdb.WithChiURLParam(req, "id", id)
	}

	tests := []struct {
		name       string
		id         string
		withTenant bool
		wantStatus int
		wantNext   bool
	}{
		{"active user passes", activeID.String(), true, http.StatusOK, true},
		{"suspended user 404", suspendedID.String(), true, http.StatusNotFound, false},
		{"nonexistent user 404", uuid.New().String(), true, http.StatusNotFound, false},
		{"missing tenant 503", activeID.String(), false, http.StatusServiceUnavailable, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled = false
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, newReq(tc.id, tc.withTenant))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d; body=%s", rec.Code, tc.wantStatus, rec.Body)
			}
			if nextCalled != tc.wantNext {
				t.Fatalf("nextCalled=%v want %v", nextCalled, tc.wantNext)
			}
		})
	}
}
