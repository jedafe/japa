//go:build integration

package feed

import (
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

// TestGetForYouFeed_NullSpaceIDDoesNotFailScan verifies that a published content
// item without an assigned space (space_id IS NULL) can be returned by the For-You
// feed candidate query. Before the fix, the SQL expression that produces
// is_space_followed evaluated to NULL when c.space_id was NULL, and pgx could not
// scan NULL into the non-nullable *bool column, causing:
//
//	"can't scan into dest[22] (col: is_space_followed): cannot scan NULL into *bool"
func TestGetForYouFeed_NullSpaceIDDoesNotFailScan(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	authorID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{})
	viewerID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{})

	contentID := testdb.SeedContent(t, database, tenantID, authorID, testdb.ContentOpts{})

	ctx := t.Context()
	_, err := database.Writer.Exec(ctx,
		`INSERT INTO content_scores (content_id, velocity_score) VALUES ($1, 10)`,
		contentID)
	if err != nil {
		t.Fatalf("testdb: seeding content_scores: %v", err)
	}

	service := NewService(database, nil)

	items, err := service.GetForYouFeed(ctx, tenantID, viewerID, SortRelevant, 20, 0, nil)
	if err != nil {
		t.Fatalf("GetForYouFeed returned an error: %v", err)
	}

	found := false
	for _, item := range items {
		if item.ID == contentID.String() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected content %s in For-You feed, got %+v", contentID.String(), items)
	}
}
