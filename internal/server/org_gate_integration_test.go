//go:build integration

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/testdb"
)

func TestRequireActiveOrgByID(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	activeOrg := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "gate-active", Status: "active"})
	suspendedOrg := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "gate-suspended", Status: "suspended"})

	orgSvc := organizations.NewService(database)

	var nextCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := requireActiveOrgByID(orgSvc)(next)

	newReq := func(id string, withTenant bool) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/organizations/"+id+"/members", nil)
		if withTenant {
			req = req.WithContext(context.WithValue(req.Context(), struct{}{}, &struct {
				ID   pgtype.UUID
				Name string
				Slug string
			}{ID: tenantID}))
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
		{"active org passes", activeOrg.String(), true, http.StatusOK, true},
		{"suspended org 404", suspendedOrg.String(), true, http.StatusNotFound, false},
		{"nonexistent org 404", uuid.New().String(), true, http.StatusNotFound, false},
		{"missing tenant 503", activeOrg.String(), false, http.StatusServiceUnavailable, false},
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
