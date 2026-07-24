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
	handler := requireActiveProfileUser(usersSvc, "id")(next)

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

	// The profile-feed route (GET /feed/profile/{user_id}) reuses this gate with a
	// DIFFERENT param key. Verify the gate reads the configured key, not a hardcoded
	// "id": with paramKey="user_id" an active user passes and a suspended one 404s.
	t.Run("honors custom param key user_id", func(t *testing.T) {
		userIDHandler := requireActiveProfileUser(usersSvc, "user_id")(next)

		newFeedReq := func(id string) *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/feed/profile/"+id, nil)
			req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
			return testdb.WithChiURLParam(req, "user_id", id)
		}

		nextCalled = false
		rec := httptest.NewRecorder()
		userIDHandler.ServeHTTP(rec, newFeedReq(activeID.String()))
		if rec.Code != http.StatusOK || !nextCalled {
			t.Fatalf("active via user_id: status=%d nextCalled=%v; want 200/true; body=%s", rec.Code, nextCalled, rec.Body)
		}

		nextCalled = false
		rec = httptest.NewRecorder()
		userIDHandler.ServeHTTP(rec, newFeedReq(suspendedID.String()))
		if rec.Code != http.StatusNotFound || nextCalled {
			t.Fatalf("suspended via user_id: status=%d nextCalled=%v; want 404/false; body=%s", rec.Code, nextCalled, rec.Body)
		}
	})
}
