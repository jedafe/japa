//go:build integration

package content

import (
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

// TestListPublishedByAuthor_ScopesAndPublicSafe verifies the Posts tab (a) scopes
// to the {id} author (another author's content must not leak — the original bug
// where the route returned ALL tenant content) and (b) is public-safe (draft and
// private content excluded).
func TestListPublishedByAuthor_ScopesAndPublicSafe(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	authorA := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "posts-author-a"})
	authorB := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "posts-author-b"})

	// authorA: 2 published+public → appear
	testdb.SeedContent(t, database, tenantID, authorA, testdb.ContentOpts{Status: "published", Visibility: "public"})
	testdb.SeedContent(t, database, tenantID, authorA, testdb.ContentOpts{Status: "published", Visibility: "public"})
	// authorA: draft + private → excluded
	testdb.SeedContent(t, database, tenantID, authorA, testdb.ContentOpts{Status: "draft", Visibility: "public"})
	testdb.SeedContent(t, database, tenantID, authorA, testdb.ContentOpts{Status: "published", Visibility: "private"})
	// authorB: 3 published+public → must NOT leak into authorA's tab
	testdb.SeedContent(t, database, tenantID, authorB, testdb.ContentOpts{Status: "published", Visibility: "public"})
	testdb.SeedContent(t, database, tenantID, authorB, testdb.ContentOpts{Status: "published", Visibility: "public"})
	testdb.SeedContent(t, database, tenantID, authorB, testdb.ContentOpts{Status: "published", Visibility: "public"})

	svc := NewService(database)
	got, err := svc.ListPublishedByAuthor(t.Context(), tenantID, authorA, 20, 0)
	if err != nil {
		t.Fatalf("ListPublishedByAuthor: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items for authorA, want 2 (author-scoped, published+public only)", len(got))
	}
	for _, c := range got {
		if c.AuthorID != authorA {
			t.Fatalf("leaked content from another author: %v", c.AuthorID)
		}
		if c.Status != "published" || c.Visibility != "public" {
			t.Fatalf("leaked non-public content: status=%s visibility=%s", c.Status, c.Visibility)
		}
	}
}
