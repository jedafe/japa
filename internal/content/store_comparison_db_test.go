package content

// DB integration coverage for the §43 comparison seo_indexable derivation —
// contents.seo_indexable = detailed depth + published + public + body words
// >= seo.sitemap.comparison_min_words, re-derived on every write path that
// changes one of those inputs. Runs when JC_TEST_DATABASE_URL (falls back
// to JC_DATABASE_URL) points at a migrated database; skips otherwise —
// same convention as internal/feed/foryou_db_test.go.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

func cmpSeoTestDB(t *testing.T) *db.DB {
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

func cmpSeoTestUser(t *testing.T, ctx context.Context, d *db.DB) pgtype.UUID {
	t.Helper()
	uname := fmt.Sprintf("cmpseo_%d", time.Now().UnixNano())
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

// seedComparison inserts a comparison contents row, its JCE depth metadata
// (pass depth "" to exercise the missing-metadata path), and a primary
// space assignment — publish transitions require one (§14 / BUG-007).
func seedComparison(t *testing.T, ctx context.Context, d *db.DB, author pgtype.UUID, depth, status, visibility string, words int) pgtype.UUID {
	t.Helper()
	uniq := fmt.Sprintf("cmpseo-%d", time.Now().UnixNano())

	var id pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO contents (author_id, title, slug, body, content_type, status, visibility, created_at)
		 VALUES ($1, $2, $3, $4, 'comparison', $5, $6, now()) RETURNING id`,
		author, "cmp seo "+uniq, uniq, strings.Repeat("word ", words), status, visibility).Scan(&id); err != nil {
		t.Fatalf("inserting comparison content: %v", err)
	}

	if depth != "" {
		if _, err := d.Writer.Exec(ctx,
			`INSERT INTO comparison_content_metadata (content_id, depth) VALUES ($1, $2)`, id, depth); err != nil {
			t.Fatalf("inserting comparison metadata: %v", err)
		}
	}

	var spaceTypeID, spaceID pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO space_types (id, name, slug) VALUES (gen_random_uuid(), $1, $1) RETURNING id`,
		"cmpseo-type-"+uniq).Scan(&spaceTypeID); err != nil {
		t.Fatalf("inserting space type: %v", err)
	}
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO spaces (id, space_type_id, name, slug, level, visibility, status)
		 VALUES (gen_random_uuid(), $1, $2, $3, 'custom', 'public', 'active') RETURNING id`,
		spaceTypeID, "cmpseo space "+uniq, "cmpseo-space-"+uniq).Scan(&spaceID); err != nil {
		t.Fatalf("inserting space: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO content_spaces (content_id, space_id, is_primary) VALUES ($1, $2, true)`,
		id, spaceID); err != nil {
		t.Fatalf("assigning primary space: %v", err)
	}

	t.Cleanup(func() {
		cctx := context.Background()
		d.Writer.Exec(cctx, `DELETE FROM content_spaces WHERE content_id = $1`, id)
		d.Writer.Exec(cctx, `DELETE FROM comparison_content_metadata WHERE content_id = $1`, id)
		d.Writer.Exec(cctx, `DELETE FROM contents WHERE id = $1`, id)
		d.Writer.Exec(cctx, `DELETE FROM spaces WHERE id = $1`, spaceID)
		d.Writer.Exec(cctx, `DELETE FROM space_types WHERE id = $1`, spaceTypeID)
	})

	return id
}

// TestDBComparisonSeoIndexableOnPublish proves the generic publish path
// derives seo_indexable for comparison rows — the gate that used to leave a
// published comparison private-flagged forever.
func TestDBComparisonSeoIndexableOnPublish(t *testing.T) {
	d := cmpSeoTestDB(t)
	ctx := context.Background()
	svc := NewService(d)
	author := cmpSeoTestUser(t, ctx, d)

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
		{"detailed members", "detailed", "members", 600, false},
		{"quick never indexable", "quick", "public", 600, false},
		// No metadata row = not JCE-registered (legacy wizard comparison) —
		// the seeded editorial flag (column default true) must be preserved.
		{"non-JCE comparison keeps editorial flag", "", "public", 600, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := seedComparison(t, ctx, d, author, tt.depth, StatusDraft, tt.visibility, tt.words)

			row, err := svc.UpdateContentStatus(ctx, id, StatusPublished)
			if err != nil {
				t.Fatalf("UpdateContentStatus: %v", err)
			}
			if row.SeoIndexable != tt.want {
				t.Errorf("seo_indexable = %v, want %v", row.SeoIndexable, tt.want)
			}
		})
	}
}

// TestDBComparisonSeoIndexableOnUpdate proves PATCH updates re-derive the
// flag — including the omitted-body case, where the word gate must count
// the preserved stored body rather than the empty payload.
func TestDBComparisonSeoIndexableOnUpdate(t *testing.T) {
	d := cmpSeoTestDB(t)
	ctx := context.Background()
	svc := NewService(d)
	author := cmpSeoTestUser(t, ctx, d)

	t.Run("body shrink below gate flips flag off", func(t *testing.T) {
		id := seedComparison(t, ctx, d, author, "detailed", StatusPublished, "public", 600)
		if _, err := svc.UpdateContentStatus(ctx, id, StatusPublished); err != nil {
			t.Fatalf("priming seo_indexable: %v", err)
		}

		row, err := svc.UpdateContent(ctx, id, author, UpdateContentInput{Body: "short body"}, true)
		if err != nil {
			t.Fatalf("UpdateContent: %v", err)
		}
		if row.SeoIndexable {
			t.Error("seo_indexable = true after shrinking body below the gate, want false")
		}
	})

	t.Run("omitted body preserves stored word count", func(t *testing.T) {
		id := seedComparison(t, ctx, d, author, "detailed", StatusPublished, "public", 600)
		if _, err := svc.UpdateContentStatus(ctx, id, StatusPublished); err != nil {
			t.Fatalf("priming seo_indexable: %v", err)
		}

		row, err := svc.UpdateContent(ctx, id, author, UpdateContentInput{Title: "metadata-only edit"}, true)
		if err != nil {
			t.Fatalf("UpdateContent: %v", err)
		}
		if !row.SeoIndexable {
			t.Error("seo_indexable = false on metadata-only edit, want true (stored body count must be preserved)")
		}
	})

	t.Run("publish via update derives flag", func(t *testing.T) {
		id := seedComparison(t, ctx, d, author, "detailed", StatusDraft, "public", 600)

		row, err := svc.UpdateContent(ctx, id, author, UpdateContentInput{Status: StatusPublished}, true)
		if err != nil {
			t.Fatalf("UpdateContent: %v", err)
		}
		if !row.SeoIndexable {
			t.Error("seo_indexable = false after publish via UpdateContent, want true")
		}
	})

	t.Run("unpublish flips flag off", func(t *testing.T) {
		id := seedComparison(t, ctx, d, author, "detailed", StatusPublished, "public", 600)
		if _, err := svc.UpdateContentStatus(ctx, id, StatusPublished); err != nil {
			t.Fatalf("priming seo_indexable: %v", err)
		}

		row, err := svc.UpdateContentStatus(ctx, id, StatusDraft)
		if err != nil {
			t.Fatalf("UpdateContentStatus: %v", err)
		}
		if row.SeoIndexable {
			t.Error("seo_indexable = true after unpublish, want false")
		}
	})
}
