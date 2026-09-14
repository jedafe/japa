//go:build integration

package content

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/testdb"
)

func TestListByOrganization_OnlyOrgPublishedPublic(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "auth"})
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active", OwnerUserID: author})
	otherOrg := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "other", Status: "active"})

	// org published-public (included) ; org draft (excluded) ; other org (excluded) ; personal (excluded)
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "orgpub", Status: "published", Visibility: "public", OrganizationID: orgID})
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "orgdraft", Status: "draft", Visibility: "public", OrganizationID: orgID})
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "otherorg", Status: "published", Visibility: "public", OrganizationID: otherOrg})
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "personal", Status: "published", Visibility: "public"})

	h := NewHandlers(NewService(database))
	req := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String()+"/content", nil)
	req = req.WithContext(context.WithValue(req.Context(), struct{}{}, &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", orgID.String())
	rec := httptest.NewRecorder()
	h.ListByOrganization(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	var env struct {
		Data []struct {
			Title        string `json:"title"`
			Organization *struct {
				Slug string `json:"slug"`
			} `json:"organization"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Data) != 1 || env.Data[0].Title != "orgpub" {
		t.Fatalf("want 1 org published-public post 'orgpub', got %+v", env.Data)
	}
	if env.Data[0].Organization == nil || env.Data[0].Organization.Slug != "acme" {
		t.Fatalf("org badge missing/wrong: %+v", env.Data[0].Organization)
	}
}
