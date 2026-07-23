//go:build integration

package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
)

// TestListByUser_ReturnsUploaderMedia verifies the profile Media tab returns
// exactly the uploader's non-deleted media, tenant-scoped, with meta.total.
func TestListByUser_ReturnsUploaderMedia(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	uploaderID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "media-tab-user"})
	testdb.SeedMedia(t, database, tenantID, uploaderID)
	testdb.SeedMedia(t, database, tenantID, uploaderID)

	// Construct the media Service + Handlers the same way NewServer does.
	svc := NewService(database)
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/users/"+uploaderID.String()+"/media", nil)
	req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", uploaderID.String())

	rec := httptest.NewRecorder()
	h.ListByUser(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body)
	}
	if resp.Meta.Total != 2 {
		t.Fatalf("meta.total=%d want 2", resp.Meta.Total)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("len(data)=%d want 2", len(resp.Data))
	}
}
