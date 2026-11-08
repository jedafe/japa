package comments

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
)

// Regression coverage for §49 "Start A Conversation" (BUG-008): the derived
// discussion thread must go through the content service — the raw store path
// left field_values NULL (23502 → "A required field is missing"), skipped
// slug uniqueness, and never wrote the primary content_spaces row.
func testDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping DB integration test")
	}
	d, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("cannot connect to test DB: %v", err)
	}
	t.Cleanup(func() { d.Writer.Close(); d.Reader.Close() })
	return d
}

func newDiscussionFixture(t *testing.T, ctx context.Context, d *db.DB) (userID, spaceID, blogID pgtype.UUID) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email) VALUES ($1, $2) RETURNING id`,
		"disctest_"+suffix, "disctest_"+suffix+"@test.dev").Scan(&userID); err != nil {
		t.Fatalf("inserting user: %v", err)
	}

	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO spaces (space_type_id, name, slug, level) VALUES ($1, $2, $3, 'custom') RETURNING id`,
		"00000000-0000-0000-0000-000000000067", "disctest-"+suffix, "disctest-"+suffix).Scan(&spaceID); err != nil {
		t.Fatalf("inserting space: %v", err)
	}

	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO contents (author_id, space_id, content_type, title, slug, status, visibility)
		 VALUES ($1, $2, 'blog_post', $3, $3, 'published', 'public') RETURNING id`,
		userID, spaceID, "disctest-post-"+suffix).Scan(&blogID); err != nil {
		t.Fatalf("inserting blog content: %v", err)
	}

	t.Cleanup(func() {
		_, _ = d.Writer.Exec(ctx, `DELETE FROM content_thread_links WHERE content_id = $1`, blogID)
		_, _ = d.Writer.Exec(ctx, `DELETE FROM contents WHERE id = $1`, blogID)
		_, _ = d.Writer.Exec(ctx, `DELETE FROM spaces WHERE id = $1`, spaceID)
		_, _ = d.Writer.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	return userID, spaceID, blogID
}

func TestStartConversation_DerivesDiscussionThroughContentService(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, spaceID, blogID := newDiscussionFixture(t, ctx, d)

	svc := NewService(d)
	svc.SetContentCreator(content.NewService(d))

	res, err := svc.StartConversation(ctx, blogID, userID, StartConversationInput{})
	if err != nil {
		t.Fatalf("StartConversation: %v", err)
	}
	if !res.Link.ID.Valid || !res.CreatedThread.ID.Valid {
		t.Fatal("expected link + created thread to be populated")
	}

	var (
		contentType string
		status      string
		slug        string
		fieldValues []byte
	)
	if err := d.Reader.QueryRow(ctx,
		`SELECT content_type, status, slug, field_values FROM contents WHERE id = $1`,
		res.CreatedThread.ID).Scan(&contentType, &status, &slug, &fieldValues); err != nil {
		t.Fatalf("reading derived thread: %v", err)
	}
	if contentType != "discussion" {
		t.Errorf("content_type = %q, want discussion", contentType)
	}
	if status != "published" {
		t.Errorf("status = %q, want published", status)
	}
	if slug == "" {
		t.Error("slug must not be empty")
	}
	if fieldValues == nil {
		t.Error("field_values must not be NULL")
	}

	// The primary content_spaces row must exist — space listings and
	// follower fan-out read it (§14).
	var csCount int
	if err := d.Reader.QueryRow(ctx,
		`SELECT count(*) FROM content_spaces WHERE content_id = $1 AND space_id = $2 AND is_primary`,
		res.CreatedThread.ID, spaceID).Scan(&csCount); err != nil {
		t.Fatalf("counting content_spaces: %v", err)
	}
	if csCount != 1 {
		t.Errorf("primary content_spaces rows = %d, want 1", csCount)
	}

	// A second conversation on the same parent must not collide on the
	// (slug) unique index — the service derives a distinct slug.
	res2, err := svc.StartConversation(ctx, blogID, userID, StartConversationInput{})
	if err != nil {
		t.Fatalf("second StartConversation: %v", err)
	}

	var slug2 string
	if err := d.Reader.QueryRow(ctx, `SELECT slug FROM contents WHERE id = $1`,
		res2.CreatedThread.ID).Scan(&slug2); err != nil {
		t.Fatalf("reading second thread slug: %v", err)
	}
	if slug2 == slug {
		t.Errorf("second thread slug %q collides with first", slug2)
	}

	// Both links list against the parent content.
	links, total, err := svc.ListDiscussions(ctx, blogID, userID, true, 20, 0)
	if err != nil {
		t.Fatalf("ListDiscussions: %v", err)
	}
	if total != 2 || len(links) != 2 {
		t.Fatalf("linked discussions = %d total %d, want 2/2", len(links), total)
	}

	t.Cleanup(func() {
		_, _ = d.Writer.Exec(ctx, `DELETE FROM content_thread_links WHERE thread_id IN ($1, $2)`, res.CreatedThread.ID, res2.CreatedThread.ID)
		_, _ = d.Writer.Exec(ctx, `DELETE FROM contents WHERE id IN ($1, $2)`, res.CreatedThread.ID, res2.CreatedThread.ID)
	})
}
