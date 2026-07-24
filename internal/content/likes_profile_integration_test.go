//go:build integration

package content

import (
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

// TestListContentLikedByUser_OnlyPublishedPublic verifies the Likes tab returns
// only published+public content the user reacted to — draft and private content
// the user also reacted to must be excluded (public-safe).
func TestListContentLikedByUser_OnlyPublishedPublic(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	user := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "likes-user"})
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "likes-author"})

	// published+public content the user reacted to → SHOULD appear
	good := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Status: "published", Visibility: "public"})
	testdb.SeedReaction(t, database, tenantID, user, "content", good, "like")
	// reacting a second time with a different key must NOT duplicate the row
	testdb.SeedReaction(t, database, tenantID, user, "content", good, "love")

	// draft content the user reacted to → excluded
	draft := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Status: "draft", Visibility: "public"})
	testdb.SeedReaction(t, database, tenantID, user, "content", draft, "like")

	// private content the user reacted to → excluded
	priv := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Status: "published", Visibility: "private"})
	testdb.SeedReaction(t, database, tenantID, user, "content", priv, "like")

	svc := NewService(database)
	got, err := svc.ListContentLikedByUser(t.Context(), tenantID, user, 20, 0)
	if err != nil {
		t.Fatalf("ListContentLikedByUser: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d liked items, want exactly 1 (published+public, de-duplicated): %+v", len(got), got)
	}
	if got[0].ID != good {
		t.Fatalf("returned wrong content id=%v want %v", got[0].ID, good)
	}
}
