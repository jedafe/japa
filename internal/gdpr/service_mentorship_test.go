// service_mentorship_test.go — live-DB coverage for the 000824 mentorship
// shadow-listing erasure gap: AnonymizeUserProfile severs the
// user_profiles link but the shadow contents row (name/bio/username PII)
// stayed published until AnonymizeMentorshipShadowListings was added.
// Skipped unless JC_DATABASE_URL is set, matching the repo's
// integration-test convention (see internal/spaces/link_geo_test.go).
package gdpr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// liveDB returns a real Postgres-backed *db.DB, or skips the test when
// JC_DATABASE_URL is unset (never runs in CI without a database).
func liveDB(t *testing.T) *db.DB {
	t.Helper()

	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	t.Cleanup(func() {
		database.Writer.Close()
		database.Reader.Close()
	})

	return database
}

func uniqTag(prefix string) string {
	return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// insertGDPRTestUser creates a users + user_profiles pair and schedules
// the rows for deletion (children first — they FK to the user).
func insertGDPRTestUser(t *testing.T, database *db.DB, username string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := database.Writer.QueryRow(t.Context(),
		`INSERT INTO users (email, username, profile_type) VALUES ($1, $2, 'regular') RETURNING id`,
		username+"@gdpr.test", username).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if _, err := database.Writer.Exec(t.Context(),
		`INSERT INTO user_profiles (user_id) VALUES ($1)`, id); err != nil {
		t.Fatalf("insert user_profiles: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		database.Writer.Exec(ctx,
			`DELETE FROM service_bookings WHERE booker_user_id = $1 OR provider_user_id = $1`, id)
		database.Writer.Exec(ctx,
			`DELETE FROM service_listing_meta WHERE content_id IN (SELECT id FROM contents WHERE author_id = $1)`, id)
		database.Writer.Exec(ctx, `DELETE FROM contents WHERE author_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM user_profiles WHERE user_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})

	return id
}

// insertShadowListing creates the 000824 shape: a published
// service_listing contents row plus a listing_kind meta row. kind='' maps
// to 'mentorship'; pass another kind for a non-mentorship control row.
func insertShadowListing(t *testing.T, database *db.DB, authorID pgtype.UUID, kind, slug string) (contentID, metaID pgtype.UUID) {
	t.Helper()
	if kind == "" {
		kind = "mentorship"
	}

	sum := sha256.Sum256([]byte(slug))
	hash := hex.EncodeToString(sum[:])

	if err := database.Writer.QueryRow(t.Context(),
		`INSERT INTO contents (author_id, content_type, title, slug, body, body_html, status, published_at, content_hash)
		 VALUES ($1, 'service_listing', $2, $3, 'mentor bio text', '', 'published', now(), $4) RETURNING id`,
		authorID, "Mentor Name — Mentorship", slug, hash).Scan(&contentID); err != nil {
		t.Fatalf("insert shadow content: %v", err)
	}

	if err := database.Writer.QueryRow(t.Context(),
		`INSERT INTO service_listing_meta (content_id, org_id, service_category, is_remote, listing_kind, purchase_mode)
		 VALUES ($1, NULL, $2, true, $3, 'inquiry') RETURNING id`,
		contentID, kind, kind).Scan(&metaID); err != nil {
		t.Fatalf("insert shadow meta: %v", err)
	}

	return contentID, metaID
}

type shadowState struct {
	DeletedAt   pgtype.Timestamptz
	Status      string
	Title       string
	Body        string
	BodyHTML    string
	Slug        string
	ContentHash *string
}

func readShadowState(t *testing.T, database *db.DB, contentID pgtype.UUID) shadowState {
	t.Helper()

	var s shadowState
	if err := database.Writer.QueryRow(t.Context(),
		`SELECT deleted_at, status, title, body, body_html, slug, content_hash
		 FROM contents WHERE id = $1`, contentID).
		Scan(&s.DeletedAt, &s.Status, &s.Title, &s.Body, &s.BodyHTML, &s.Slug, &s.ContentHash); err != nil {
		t.Fatalf("read content row: %v", err)
	}

	return s
}

// TestAnonymizeMentorshipShadowListings_LiveDB proves the erasure scrub
// retires and de-PIIs the mentor shadow while leaving other listings and
// other users' shadows untouched.
func TestAnonymizeMentorshipShadowListings_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	q := gen.New(database.Writer)
	uniq := uniqTag("gdpr-mentor-")

	mentorID := insertGDPRTestUser(t, database, uniq+"a")
	otherID := insertGDPRTestUser(t, database, uniq+"b")

	shadowID, _ := insertShadowListing(t, database, mentorID, "", "mentor-"+uniq+"a")
	serviceID, _ := insertShadowListing(t, database, mentorID, "service", "svc-"+uniq+"a")
	otherShadowID, _ := insertShadowListing(t, database, otherID, "", "mentor-"+uniq+"b")

	if err := q.AnonymizeMentorshipShadowListings(ctx, mentorID); err != nil {
		t.Fatalf("AnonymizeMentorshipShadowListings: %v", err)
	}

	wantHash := sha256.Sum256([]byte("mentor-deleted:" + shadowID.String()))
	wantSlug := "mentor-deleted-" + shadowID.String()

	got := readShadowState(t, database, shadowID)
	if !got.DeletedAt.Valid {
		t.Error("shadow listing deleted_at not set — still live")
	}
	if got.Status != "archived" {
		t.Errorf("status = %q, want archived", got.Status)
	}
	if got.Title != "" || got.Body != "" || got.BodyHTML != "" {
		t.Errorf("PII columns not scrubbed: title=%q body=%q body_html=%q", got.Title, got.Body, got.BodyHTML)
	}
	if got.Slug != wantSlug {
		t.Errorf("slug = %q, want %q", got.Slug, wantSlug)
	}
	wantHashHex := hex.EncodeToString(wantHash[:])
	if got.ContentHash == nil || *got.ContentHash != wantHashHex {
		t.Errorf("content_hash = %v, want %q", got.ContentHash, wantHashHex)
	}

	// Non-mentorship service listing by the same author must survive —
	// the scrub is scoped by listing_kind='mentorship'.
	svc := readShadowState(t, database, serviceID)
	if svc.DeletedAt.Valid || svc.Status != "published" || svc.Title == "" {
		t.Errorf("non-mentorship listing touched: deleted=%v status=%q title=%q",
			svc.DeletedAt.Valid, svc.Status, svc.Title)
	}

	// Another mentor's shadow must survive.
	other := readShadowState(t, database, otherShadowID)
	if other.DeletedAt.Valid || other.Status != "published" {
		t.Errorf("other user's shadow touched: deleted=%v status=%q",
			other.DeletedAt.Valid, other.Status)
	}

	// Idempotent: a second run is a no-op (scrubbed rows are filtered by
	// deleted_at IS NULL), so a retried deletion job stays safe.
	if err := q.AnonymizeMentorshipShadowListings(ctx, mentorID); err != nil {
		t.Fatalf("second AnonymizeMentorshipShadowListings: %v", err)
	}
	again := readShadowState(t, database, shadowID)
	if !again.DeletedAt.Valid || again.Title != "" || again.Slug != wantSlug {
		t.Error("second run mutated the tombstone row")
	}
}
