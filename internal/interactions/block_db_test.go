package interactions

// DB integration coverage for the §41 block write path (BLK-020).
// Runs when JC_TEST_DATABASE_URL (falls back to JC_DATABASE_URL) points at a
// migrated database; skips otherwise — same convention as
// internal/tips/settle_db_test.go.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

func blockTestDB(t *testing.T) *db.DB {
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

func blockTestUser(t *testing.T, ctx context.Context, d *db.DB, tag string) pgtype.UUID {
	t.Helper()
	uname := fmt.Sprintf("blocktester_%s_%d", tag, time.Now().UnixNano())
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email) VALUES ($1, $2) RETURNING id`,
		uname, uname+"@test.dev").Scan(&id)
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO user_profiles (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, id); err != nil {
		t.Fatalf("inserting profile: %v", err)
	}
	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

// TestDBBlockUserSideEffects proves the block transaction leaves all five
// consequences correct: user_blocks row, mutual follow deletion, synced
// follower_count, and declined pending DM requests.
func TestDBBlockUserSideEffects(t *testing.T) {
	d := blockTestDB(t)
	ctx := context.Background()
	svc := NewService(d)

	a := blockTestUser(t, ctx, d, "a")
	b := blockTestUser(t, ctx, d, "b")

	// Seed: mutual follows + inflated counts + one pending DM request.
	for _, pair := range [][2]pgtype.UUID{{a, b}, {b, a}} {
		if _, err := d.Writer.Exec(ctx,
			`INSERT INTO follows (follower_id, entity_type, entity_id) VALUES ($1, 'user', $2)`,
			pair[0], pair[1]); err != nil {
			t.Fatalf("seeding follow: %v", err)
		}
	}
	if _, err := d.Writer.Exec(ctx,
		`UPDATE users SET follower_count = 1 WHERE id IN ($1, $2)`, a, b); err != nil {
		t.Fatalf("seeding counts: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO dm_requests (requester_id, recipient_id, status) VALUES ($1, $2, 'pending')`, a, b); err != nil {
		t.Fatalf("seeding dm_request: %v", err)
	}

	if err := svc.BlockUser(ctx, a, b); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}

	var n int
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2`, a, b).Scan(&n); err != nil || n != 1 {
		t.Fatalf("user_blocks row missing (n=%d, err=%v)", n, err)
	}
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM follows WHERE entity_type = 'user' AND
		 ((follower_id = $1 AND entity_id = $2) OR (follower_id = $2 AND entity_id = $1))`, a, b).Scan(&n); err != nil || n != 0 {
		t.Fatalf("mutual follows not removed (n=%d, err=%v)", n, err)
	}
	for _, u := range []pgtype.UUID{a, b} {
		var c int
		if err := d.Reader.QueryRow(ctx, `SELECT follower_count FROM users WHERE id = $1`, u).Scan(&c); err != nil || c != 0 {
			t.Fatalf("follower_count not synced for %v (c=%d, err=%v)", u, c, err)
		}
	}
	var st string
	if err := d.Reader.QueryRow(ctx,
		`SELECT status FROM dm_requests WHERE requester_id = $1 AND recipient_id = $2`, a, b).Scan(&st); err != nil || st != "declined" {
		t.Fatalf("dm_request not declined (status=%q, err=%v)", st, err)
	}
}

// TestDBGetBlockStatus proves directional semantics: blocked reports the
// viewer→target direction only; blocked_by reports target→viewer.
func TestDBGetBlockStatus(t *testing.T) {
	d := blockTestDB(t)
	ctx := context.Background()
	svc := NewService(d)

	a := blockTestUser(t, ctx, d, "va")
	b := blockTestUser(t, ctx, d, "vb")

	if err := svc.BlockUser(ctx, a, b); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}

	st, err := svc.GetBlockStatus(ctx, a, b)
	if err != nil {
		t.Fatalf("GetBlockStatus: %v", err)
	}
	if !st.Blocked || st.BlockedBy {
		t.Fatalf("status = %+v, want {Blocked:true BlockedBy:false}", st)
	}

	st, err = svc.GetBlockStatus(ctx, b, a)
	if err != nil {
		t.Fatalf("GetBlockStatus reverse: %v", err)
	}
	if st.Blocked || !st.BlockedBy {
		t.Fatalf("reverse status = %+v, want {Blocked:false BlockedBy:true}", st)
	}
}
