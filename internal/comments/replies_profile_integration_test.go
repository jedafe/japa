//go:build integration

package comments

import (
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

// TestListByAuthor_OnlyRepliesOnPublishedPublic verifies the Replies tab returns
// only the user's comments on published+public content, with the parent content
// title joined — replies on draft/private content are excluded (public-safe).
func TestListByAuthor_OnlyRepliesOnPublishedPublic(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	user := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "replies-user"})
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "replies-content-author"})

	// comment on published+public content → SHOULD appear
	pub := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Status: "published", Visibility: "public"})
	testdb.SeedComment(t, database, tenantID, user, pub, "great post")

	// comment on draft content → excluded
	draft := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Status: "draft", Visibility: "public"})
	testdb.SeedComment(t, database, tenantID, user, draft, "on a draft")

	// comment on private content → excluded
	priv := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Status: "published", Visibility: "private"})
	testdb.SeedComment(t, database, tenantID, user, priv, "on a private post")

	svc := NewService(database)
	got, total, err := svc.ListByAuthor(t.Context(), tenantID, user, 20, 0)
	if err != nil {
		t.Fatalf("ListByAuthor: %v", err)
	}
	if total != 1 || len(got) != 1 {
		t.Fatalf("got total=%d len=%d, want 1 (only replies on published+public): %+v", total, len(got), got)
	}
	// Body is now TipTap JSON (jsonb); check the text is present in the doc.
	if !strings.Contains(string(got[0].Body), "great post") {
		t.Fatalf("returned wrong comment body=%q want text containing %q", string(got[0].Body), "great post")
	}
	if got[0].ContentTitle == "" {
		t.Fatalf("expected parent-content title joined, got empty")
	}
}
