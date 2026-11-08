package feed

// DB integration coverage for the author-points reputation signal
// (Section 32 — For-You relevant ranking). Runs when JC_TEST_DATABASE_URL
// (falls back to JC_DATABASE_URL) points at a migrated database; skips
// otherwise — same convention as internal/interactions/block_db_test.go.
//
// The author_points CTE aggregates wallet_ledger per DISTINCT candidate
// author. The regression this guards: joining candidate contents inside the
// aggregation multiplied each author's balance by their candidate count
// (an author with K rows in the pool reported K×balance).

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

func feedTestDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("JC_DATABASE_URL")
	}
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

func feedTestUser(t *testing.T, ctx context.Context, d *db.DB, tag string, bot bool) pgtype.UUID {
	t.Helper()
	uname := fmt.Sprintf("feedtester_%s_%d", tag, time.Now().UnixNano())
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email, is_bot) VALUES ($1, $2, $3) RETURNING id`,
		uname, uname+"@test.dev", bot).Scan(&id)
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE author_id = $1`, id)
		d.Writer.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

func feedTestPoints(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID, amounts ...int64) {
	t.Helper()
	for _, amt := range amounts {
		if _, err := d.Writer.Exec(ctx,
			`INSERT INTO wallet_ledger (user_id, ledger_type, operation, amount, resulting_balance) VALUES ($1, 'points', 'earn', $2, 0)`,
			userID, amt); err != nil {
			t.Fatalf("inserting ledger row: %v", err)
		}
	}
}

func feedTestContent(t *testing.T, ctx context.Context, d *db.DB, authorID pgtype.UUID, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := d.Writer.Exec(ctx,
			`INSERT INTO contents (author_id, title, slug, status, created_at)
			 VALUES ($1, $2, $3, 'published', now())`,
			authorID, fmt.Sprintf("rep-test post %d", i), fmt.Sprintf("rep-test-%d-%d", i, time.Now().UnixNano())); err != nil {
			t.Fatalf("inserting content: %v", err)
		}
	}
}

func feedTestCandidates(t *testing.T, ctx context.Context, d *db.DB, viewer pgtype.UUID, authorIDs []pgtype.UUID) []gen.GetForYouCandidatesRow {
	t.Helper()
	var windowStart pgtype.Timestamptz
	if err := windowStart.Scan(time.Now().Add(-24 * time.Hour)); err != nil {
		t.Fatalf("scanning window start: %v", err)
	}
	empty := ""
	rows, err := gen.New(d.Reader).GetForYouCandidates(ctx, gen.GetForYouCandidatesParams{
		AuthorIds:          authorIDs,
		SpaceIds:           []pgtype.UUID{},
		TagIds:             []pgtype.UUID{},
		CandidateCap:       600,
		WindowStart:        windowStart,
		ExcludeAuthorID:    viewer,
		AffinityLimit:      300,
		VelocityLimit:      300,
		VisitorCountryCode: &empty,
		VisitorRegion:      &empty,
	})
	if err != nil {
		t.Fatalf("GetForYouCandidates: %v", err)
	}
	return rows
}

func authorPointsFor(rows []gen.GetForYouCandidatesRow, authorID pgtype.UUID) (points int64, found int) {
	for _, r := range rows {
		if uuidToString(r.AuthorID) == uuidToString(authorID) {
			found++
			points = r.AuthorPoints
		}
	}
	return points, found
}

// TestDBGetForYouCandidatesAuthorPoints proves author_points equals the
// author's real ledger balance — not multiplied by the author's candidate
// count (the pre-fix CTE joined contents inside the aggregation).
func TestDBGetForYouCandidatesAuthorPoints(t *testing.T) {
	d := feedTestDB(t)
	ctx := context.Background()

	author := feedTestUser(t, ctx, d, "reputed", false)
	viewer := feedTestUser(t, ctx, d, "viewer", false)

	// 250 points across two ledger rows — proves SUM aggregation.
	feedTestPoints(t, ctx, d, author, 200, 50)

	// Three candidates from the same author; the pre-fix bug reported 3×250.
	feedTestContent(t, ctx, d, author, 3)

	rows := feedTestCandidates(t, ctx, d, viewer, []pgtype.UUID{author})

	points, found := authorPointsFor(rows, author)
	if found == 0 {
		t.Fatal("author's candidates absent from pool — fixture did not land")
	}
	if found < 3 {
		t.Fatalf("expected all 3 author candidates in pool, found %d", found)
	}
	if points != 250 {
		t.Fatalf("author_points = %d, want 250 (any multiplier = join-duplication regression)", points)
	}
}

// TestDBGetForYouCandidatesAuthorPointsEligibility proves ineligible authors
// carry zero reputation weight — same eligibility as the §64 leaderboard:
// bots and points-hidden users contribute rep 0.
func TestDBGetForYouCandidatesAuthorPointsEligibility(t *testing.T) {
	d := feedTestDB(t)
	ctx := context.Background()

	bot := feedTestUser(t, ctx, d, "bot", true)
	hidden := feedTestUser(t, ctx, d, "hidden", false)
	viewer := feedTestUser(t, ctx, d, "viewer2", false)

	feedTestPoints(t, ctx, d, bot, 100)
	feedTestPoints(t, ctx, d, hidden, 50)

	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO user_privacy_settings (user_id, show_points_publicly) VALUES ($1, false)`,
		hidden); err != nil {
		t.Fatalf("inserting privacy settings: %v", err)
	}

	feedTestContent(t, ctx, d, bot, 1)
	feedTestContent(t, ctx, d, hidden, 1)

	rows := feedTestCandidates(t, ctx, d, viewer, []pgtype.UUID{bot, hidden})

	if pts, found := authorPointsFor(rows, bot); found == 0 {
		t.Fatal("bot author's candidate absent from pool — fixture did not land")
	} else if pts != 0 {
		t.Fatalf("bot author_points = %d, want 0 (bots carry no reputation weight)", pts)
	}

	if pts, found := authorPointsFor(rows, hidden); found == 0 {
		t.Fatal("hidden author's candidate absent from pool — fixture did not land")
	} else if pts != 0 {
		t.Fatalf("opted-out author_points = %d, want 0 (show_points_publicly=false)", pts)
	}
}
