//go:build integration

package organizations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
)

func TestGetPublicOrganization_ActiveLean_NonActive404(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active"})
	testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "shady", Status: "suspended"})

	h := NewHandlers(NewService(database))

	call := func(slug string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/organizations/"+slug, nil)
		req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
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
