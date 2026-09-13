//go:build integration

package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

func TestListByOrganization_OrgPublicPostMedia(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "mauth"})
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active", OwnerUserID: author})

	mediaID := testdb.SeedMedia(t, database, tenantID, author) // confirmed, universal
	// org published-public post featuring the media → media included
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{
		Title: "orgpub", Status: "published", Visibility: "public",
		OrganizationID: orgID, FeaturedImageID: mediaID,
	})
	// org DRAFT featuring media → excluded (not published)
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{
		Title: "orgdraft", Status: "draft", Visibility: "public",
		OrganizationID: orgID, FeaturedImageID: testdb.SeedMedia(t, database, tenantID, author),
	})

	h := NewHandlers(NewService(database))
	req := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String()+"/media", nil)
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
			ID string `json:"id"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Meta.Total != 1 || len(env.Data) != 1 || env.Data[0].ID != mediaID.String() {
		t.Fatalf("want exactly the org public post media, got total=%d data=%+v", env.Meta.Total, env.Data)
	}
}
