//go:build integration

package comments

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/testdb"
)

// TestVoteComment_NonDiscussionContent verifies that upvote/downvote now works
// on non-discussion content after the Section 49 amendment.
func TestVoteComment_NonDiscussionContent(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "content-author"})
	voter := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "voter"})

	// Seed a blog post, then change its content_type to 'blog' so this test
	// proves the old forum-discussion-only restriction is gone.
	contentID := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{})
	ctx := t.Context()
	if _, err := database.Writer.Exec(ctx,
		"UPDATE contents SET content_type = 'blog' WHERE id = $1 AND tenant_id = $2",
		contentID, tenantID); err != nil {
		t.Fatalf("changing content type to blog: %v", err)
	}

	commentID := testdb.SeedComment(t, database, tenantID, author, contentID, "first")

	svc := NewService(database)

	if err := svc.VoteComment(ctx, tenantID, commentID, voter, 1); err != nil {
		t.Fatalf("VoteComment on blog content: %v", err)
	}

	c, err := svc.GetComment(ctx, tenantID, commentID)
	if err != nil {
		t.Fatalf("GetComment after upvote: %v", err)
	}
	if c.UpvoteCount != 1 {
		t.Errorf("UpvoteCount = %d, want 1", c.UpvoteCount)
	}
	if c.VoteScore != 1 {
		t.Errorf("VoteScore = %d, want 1", c.VoteScore)
	}

	if err := svc.RemoveCommentVote(ctx, tenantID, commentID, voter); err != nil {
		t.Fatalf("RemoveCommentVote on blog content: %v", err)
	}

	c, err = svc.GetComment(ctx, tenantID, commentID)
	if err != nil {
		t.Fatalf("GetComment after removing vote: %v", err)
	}
	if c.UpvoteCount != 0 {
		t.Errorf("UpvoteCount after remove = %d, want 0", c.UpvoteCount)
	}
	if c.VoteScore != 0 {
		t.Errorf("VoteScore after remove = %d, want 0", c.VoteScore)
	}
}

// TestListCommentsByContent_VoteCountsAndUserVote verifies that listing
// comments returns the denormalized upvote/downvote counts and the viewer's
// own user_vote so the UI can render the active state without a second call.
func TestListCommentsByContent_VoteCountsAndUserVote(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "content-author"})
	voter := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "voter"})
	other := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "other"})

	contentID := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{})
	commentID := testdb.SeedComment(t, database, tenantID, author, contentID, "hello world")

	svc := NewService(database)
	ctx := t.Context()

	if err := svc.VoteComment(ctx, tenantID, commentID, voter, 1); err != nil {
		t.Fatalf("VoteComment: %v", err)
	}

	voterRows, err := svc.ListCommentsByContent(ctx, tenantID, contentID, voter, "top", 20, 0)
	if err != nil {
		t.Fatalf("ListCommentsByContent as voter: %v", err)
	}
	if len(voterRows) != 1 {
		t.Fatalf("got %d comments, want 1", len(voterRows))
	}

	got := voterRows[0]
	if got.UpvoteCount != 1 {
		t.Errorf("UpvoteCount = %d, want 1", got.UpvoteCount)
	}
	if got.DownvoteCount != 0 {
		t.Errorf("DownvoteCount = %d, want 0", got.DownvoteCount)
	}
	if got.VoteScore != 1 {
		t.Errorf("VoteScore = %d, want 1", got.VoteScore)
	}
	if got.UserVote != 1 {
		t.Errorf("UserVote = %d, want 1", got.UserVote)
	}

	otherRows, err := svc.ListCommentsByContent(ctx, tenantID, contentID, other, "top", 20, 0)
	if err != nil {
		t.Fatalf("ListCommentsByContent as other: %v", err)
	}
	if len(otherRows) != 1 {
		t.Fatalf("got %d comments, want 1", len(otherRows))
	}
	if otherRows[0].UpvoteCount != 1 {
		t.Errorf("other UpvoteCount = %d, want 1", otherRows[0].UpvoteCount)
	}
	if otherRows[0].UserVote != 0 {
		t.Errorf("other UserVote = %d, want 0", otherRows[0].UserVote)
	}
}

// TestListRepliesByParent_UserVote verifies the same vote-state exposure for
// lazy-loaded child replies.
func TestListRepliesByParent_UserVote(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "content-author"})
	voter := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "voter"})

	contentID := testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{})
	parentID := testdb.SeedComment(t, database, tenantID, author, contentID, "parent")
	replyID := testdb.SeedComment(t, database, tenantID, author, contentID, "reply")

	// Make the reply a child of the parent by setting parent_id.
	ctx := t.Context()
	if _, err := database.Writer.Exec(ctx,
		"UPDATE comments SET parent_id = $1, depth = 1 WHERE id = $2 AND tenant_id = $3",
		parentID, replyID, tenantID); err != nil {
		t.Fatalf("setting reply parent: %v", err)
	}

	svc := NewService(database)
	if err := svc.VoteComment(ctx, tenantID, replyID, voter, -1); err != nil {
		t.Fatalf("VoteComment: %v", err)
	}

	rows, err := svc.ListRepliesByParent(ctx, tenantID, parentID, voter, 20, 0)
	if err != nil {
		t.Fatalf("ListRepliesByParent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d replies, want 1", len(rows))
	}

	got := rows[0]
	if got.DownvoteCount != 1 {
		t.Errorf("DownvoteCount = %d, want 1", got.DownvoteCount)
	}
	if got.UpvoteCount != 0 {
		t.Errorf("UpvoteCount = %d, want 0", got.UpvoteCount)
	}
	if got.UserVote != -1 {
		t.Errorf("UserVote = %d, want -1", got.UserVote)
	}

	// Double-check zero-UUID viewer returns no user vote but still sees counts.
	var zero pgtype.UUID
	rows, err = svc.ListRepliesByParent(ctx, tenantID, parentID, zero, 20, 0)
	if err != nil {
		t.Fatalf("ListRepliesByParent anonymous: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d replies, want 1", len(rows))
	}
	if rows[0].DownvoteCount != 1 {
		t.Errorf("anonymous DownvoteCount = %d, want 1", rows[0].DownvoteCount)
	}
	if rows[0].UserVote != 0 {
		t.Errorf("anonymous UserVote = %d, want 0", rows[0].UserVote)
	}
}
