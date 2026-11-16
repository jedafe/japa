package worker

// DB integration coverage for the §43 seo_indexable derivation on the
// scheduled-publish path — PublishScheduledContent flips status via raw
// SQL (the status='scheduled' guard can't go through the service), so
// HandleScheduledPublish re-derives the flag for comparison rows.
// Runs when JC_TEST_DATABASE_URL (falls back to JC_DATABASE_URL) points
// at a migrated database; skips otherwise — same convention as
// internal/feed/foryou_db_test.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

func schedPubTestDB(t *testing.T) *db.DB {
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
	// RegisterDBPool wires the package-global the handlers read. Restore
	// nil so parallel worker tests still see their unwired fast path.
	RegisterDBPool(d)
	t.Cleanup(func() {
		RegisterDBPool(nil)
		d.Writer.Close()
		d.Reader.Close()
	})
	return d
}

func schedPubTestUser(t *testing.T, ctx context.Context, d *db.DB) pgtype.UUID {
	t.Helper()
	uname := fmt.Sprintf("schedpub_%d", time.Now().UnixNano())
	var id pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email) VALUES ($1, $2) RETURNING id`,
		uname, uname+"@test.dev").Scan(&id); err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

// seedScheduledContent inserts a contents row in status='scheduled' with
// publish_at in the past (so PublishScheduledContent flips it), plus a
// comparison_content_metadata row when depth is non-empty. seoIndexable
// is seeded explicitly — the column default is true, and the sweep test
// needs to distinguish "not written" from "written false".
func seedScheduledContent(t *testing.T, ctx context.Context, d *db.DB, author pgtype.UUID, contentType, depth, visibility string, words int, seoIndexable bool) pgtype.UUID {
	t.Helper()
	uniq := fmt.Sprintf("schedpub-%d", time.Now().UnixNano())

	var id pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO contents (author_id, title, slug, body, content_type, status, visibility, seo_indexable, publish_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, 'scheduled', $6, $7, now() - interval '1 minute', now()) RETURNING id`,
		author, "sched "+uniq, uniq, strings.Repeat("word ", words), contentType, visibility, seoIndexable).Scan(&id); err != nil {
		t.Fatalf("inserting scheduled content: %v", err)
	}
	if depth != "" {
		if _, err := d.Writer.Exec(ctx,
			`INSERT INTO comparison_content_metadata (content_id, depth) VALUES ($1, $2)`, id, depth); err != nil {
			t.Fatalf("inserting comparison metadata: %v", err)
		}
	}
	t.Cleanup(func() {
		cctx := context.Background()
		d.Writer.Exec(cctx, `DELETE FROM comparison_content_metadata WHERE content_id = $1`, id)
		d.Writer.Exec(cctx, `DELETE FROM contents WHERE id = $1`, id)
	})
	return id
}

func runScheduledPublish(t *testing.T, id pgtype.UUID) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"content_id": id.String()})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := HandleScheduledPublish(context.Background(), asynq.NewTask("content:publish_scheduled", payload)); err != nil {
		t.Fatalf("HandleScheduledPublish: %v", err)
	}
}

func contentStatusAndSeo(t *testing.T, ctx context.Context, d *db.DB, id pgtype.UUID) (string, bool) {
	t.Helper()
	var status string
	var seo bool
	if err := d.Reader.QueryRow(ctx,
		`SELECT status, seo_indexable FROM contents WHERE id = $1`, id).Scan(&status, &seo); err != nil {
		t.Fatalf("reading content state: %v", err)
	}
	return status, seo
}

func TestDBScheduledPublishComparisonSeoIndexable(t *testing.T) {
	d := schedPubTestDB(t)
	ctx := context.Background()
	author := schedPubTestUser(t, ctx, d)

	tests := []struct {
		name       string
		depth      string
		visibility string
		words      int
		want       bool
	}{
		{"detailed public above gate", "detailed", "public", 600, true},
		{"detailed public below gate", "detailed", "public", 100, false},
		{"detailed private", "detailed", "private", 600, false},
		{"quick never indexable", "quick", "public", 600, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := seedScheduledContent(t, ctx, d, author, "comparison", tt.depth, tt.visibility, tt.words, false)

			runScheduledPublish(t, id)

			status, seo := contentStatusAndSeo(t, ctx, d, id)
			if status != "published" {
				t.Fatalf("status = %q, want published", status)
			}
			if seo != tt.want {
				t.Errorf("seo_indexable = %v, want %v", seo, tt.want)
			}
		})
	}

	t.Run("non-comparison flag untouched", func(t *testing.T) {
		id := seedScheduledContent(t, ctx, d, author, "blog_post", "", "public", 10, false)

		runScheduledPublish(t, id)

		status, seo := contentStatusAndSeo(t, ctx, d, id)
		if status != "published" {
			t.Fatalf("status = %q, want published", status)
		}
		// Seeded false must survive — the sweep only derives the flag
		// for comparison rows.
		if seo {
			t.Error("seo_indexable flipped for non-comparison row")
		}
	})
}
