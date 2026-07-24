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

// TestListByUser_OnlyMediaFromPublishedPublicPosts verifies the Media tab returns
// ONLY media that is (a) featured/header image of the user's published+public post,
// (b) confirmed, (c) universal (user_group_id IS NULL). Everything else is excluded.
func TestListByUser_OnlyMediaFromPublishedPublicPosts(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "media-posts-author"})

	// SHOULD appear: confirmed universal media, featured on a published public post.
	good := testdb.SeedMedia(t, database, tenantID, author)
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{
		Status: "published", Visibility: "public", FeaturedImageID: good,
	})

	// SHOULD be excluded: media featured only on a DRAFT post.
	draftMedia := testdb.SeedMedia(t, database, tenantID, author)
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{
		Status: "draft", Visibility: "public", FeaturedImageID: draftMedia,
	})

	// SHOULD be excluded: media featured only on a PRIVATE post.
	privMedia := testdb.SeedMedia(t, database, tenantID, author)
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{
		Status: "published", Visibility: "private", FeaturedImageID: privMedia,
	})

	// SHOULD be excluded: media not referenced by any post (raw upload).
	_ = testdb.SeedMedia(t, database, tenantID, author)

	svc := NewService(database)
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/users/"+author.String()+"/media", nil)
	req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", author.String())

	rec := httptest.NewRecorder()
	h.ListByUser(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body)
	}
	if resp.Meta.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("expected exactly 1 item (the featured image of the published public post), got total=%d len=%d body=%s",
			resp.Meta.Total, len(resp.Data), rec.Body)
	}
	if resp.Data[0].ID != good.String() {
		t.Fatalf("returned wrong media id=%s want %s", resp.Data[0].ID, good.String())
	}
}
