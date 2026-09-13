// go:build integration

package organizations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

func TestGetPublicOrganization_ActiveLean_NonActive404(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active"})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "shady", Status: "suspended"})

	h := NewHandlers(NewService(database), nil, nil)

	call := func(slug string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/organizations/"+slug, nil)
		req = req.WithContext(context.WithValue(req.Context(), struct{}{}, &struct {
			ID   pgtype.UUID
			Name string
			Slug string
		}{ID: tenantID}))
		req = testdb.WithChiURLParam(req, "slug", slug)
		rec := httptest.NewRecorder()
		h.GetPublicOrganization(rec, req)
		return rec
	}

	// active → 200 lean (no internal columns)
	rec := call("acme")
	if rec.Code != http.StatusOK {
		t.Fatalf("active: status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, leaked := range []string{"tenant_id", "owner_user_id", "field_values"} {
		if strings.Contains(body, leaked) {
			t.Errorf("active org body leaks %q: %s", leaked, body)
		}
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.Data["slug"] != "acme" {
		t.Fatalf("slug=%v want acme", envelope.Data["slug"])
	}

	// suspended → 404
	if rec := call("shady"); rec.Code != http.StatusNotFound {
		t.Fatalf("suspended: status=%d want 404; body=%s", rec.Code, rec.Body)
	}
	// missing → 404
	if rec := call("nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing: status=%d want 404; body=%s", rec.Code, rec.Body)
	}
}

func TestListPublicMembers_ActiveOnly_Lean(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active"})

	activeUser := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "active-mem", DisplayName: "Active Mem"})
	suspendedUser := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "susp-mem", Status: "suspended"})
	testdb.SeedOrgMembership(t, database, orgID, activeUser, "admin")
	testdb.SeedOrgMembership(t, database, orgID, suspendedUser, "member")

	h := NewHandlers(NewService(database), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String()+"/members", nil)
	req = req.WithContext(context.WithValue(req.Context(), struct{}{}, &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", orgID.String())
	rec := httptest.NewRecorder()
	h.ListPublicMembers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "invited_by") {
		t.Errorf("members body leaks invited_by: %s", body)
	}
	var envelope struct {
		Data []struct {
			Role string `json:"role"`
			User struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(envelope.Data) != 1 {
		t.Fatalf("got %d members, want 1 (active only)", len(envelope.Data))
	}
	if envelope.Data[0].User.Username != "active-mem" || envelope.Data[0].Role != "admin" {
		t.Fatalf("unexpected member: %+v", envelope.Data[0])
	}
}

func TestListPublicOrganizations_VerifiedActiveOnly(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "v1", Status: "active", Verified: true})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "v2", Status: "active", Verified: true})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "unverified", Status: "active", Verified: false})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "suspended", Status: "suspended", Verified: true})

	h := NewHandlers(NewService(database), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/organizations", nil)
	req = req.WithContext(context.WithValue(req.Context(), struct{}{}, &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{ID: tenantID}))
	rec := httptest.NewRecorder()
	h.ListPublicOrganizations(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	var envelope struct {
		Data []struct {
			Slug string `json:"slug"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.Meta.Total != 2 || len(envelope.Data) != 2 {
		t.Fatalf("got total=%d len=%d, want 2 verified+active only", envelope.Meta.Total, len(envelope.Data))
	}
}
