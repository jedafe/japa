//go:build integration

package spaces

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/testdb"
)

// TestListByMemberUser_OnlyPublicActiveSpaces verifies the Spaces tab returns
// only the user's public+active space memberships, excluding members-only,
// private, and non-active spaces.
func TestListByMemberUser_OnlyPublicActiveSpaces(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "spaces-tab-user"})

	publicSpace := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})           // public/active
	_ = testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{Visibility: "members"}) // excluded
	_ = testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{Visibility: "private"}) // excluded
	_ = testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{Status: "archived"})    // excluded (non-active)

	svc := NewService(database)
	h := NewHandlers(svc, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/users/"+userID.String()+"/spaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), struct{}{}, &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", userID.String())

	rec := httptest.NewRecorder()
	h.ListByMemberUser(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		Data []struct {
			ID         string `json:"id"`
			Visibility string `json:"visibility"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected exactly 1 public+active space, got %d: %s", len(resp.Data), rec.Body)
	}
	if resp.Data[0].ID != publicSpace.String() {
		t.Fatalf("returned wrong space id=%s want %s", resp.Data[0].ID, publicSpace.String())
	}
	if resp.Data[0].Visibility != "public" {
		t.Fatalf("returned non-public space visibility=%s", resp.Data[0].Visibility)
	}
}
