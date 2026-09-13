//go:build integration

package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

// TestGetProfileFeed_GuestAllowed verifies Section 32:12 ("profile timeline
// audience = Anyone"): GET /feed/profile/{user_id} must return the public
// rows for a guest request — no Authorization header, no *auth.AuthenticatedUser
// in context — instead of the 401 it returned while the handler required a
// viewer.
func TestGetProfileFeed_GuestAllowed(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	authorID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "profile-feed-author"})
	testdb.SeedContent(t, database, tenantID, authorID, testdb.ContentOpts{}) // defaults: published/public

	service := NewService(database, nil) // vk may be nil; GetProfileFeed never touches it
	h := NewHandlers(service)

	req := httptest.NewRequest(http.MethodGet, "/feed/profile/"+authorID.String(), nil)
	req = req.WithContext(context.WithValue(req.Context(), struct{}{}, &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "user_id", authorID.String())
	// Deliberately NOT injecting auth.UserContextKey: this request is a guest.

	rec := httptest.NewRecorder()
	h.GetProfileFeed(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("guest profile feed = %d, want 200; body=%s", rec.Code, rec.Body)
	}
}
