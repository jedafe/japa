// service_mentorship_test.go — live-DB coverage for the 000824 mentorship
// shadow-listing erasure gap: AnonymizeUserProfile severs the
// user_profiles link but the shadow contents row (name/bio/username PII)
// stayed published until AnonymizeMentorshipShadowListings was added.
// Skipped unless JC_DATABASE_URL is set, matching the repo's
// integration-test convention (see internal/spaces/link_geo_test.go).
package gdpr

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
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
		ctx := t.Context()
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
// service_listing contents row plus a listing_kind meta row. kind=” maps
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

	// Opt-out tombstone: SoftDeleteMentorshipShadow only sets deleted_at
	// and leaves title/body/slug intact. The scrub must de-PII it while
	// preserving the original tombstone timestamp.
	tombID, _ := insertShadowListing(t, database, mentorID, "", "mentor-"+uniq+"a-old")
	if _, err := database.Writer.Exec(ctx,
		`UPDATE contents SET deleted_at = now() - interval '7 days' WHERE id = $1`,
		tombID); err != nil {
		t.Fatalf("soft-delete tombstone shadow: %v", err)
	}

	var tombDeletedAt pgtype.Timestamptz
	if err := database.Writer.QueryRow(ctx,
		`SELECT deleted_at FROM contents WHERE id = $1`, tombID).
		Scan(&tombDeletedAt); err != nil {
		t.Fatalf("read tombstone deleted_at: %v", err)
	}

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

	// Opt-out tombstone: PII scrubbed, original deleted_at preserved
	// (COALESCE — the scrub must not rewrite opt-out history).
	tomb := readShadowState(t, database, tombID)
	if !tomb.DeletedAt.Valid {
		t.Error("tombstone lost its deleted_at")
	} else if !tomb.DeletedAt.Time.Equal(tombDeletedAt.Time) {
		t.Errorf("tombstone deleted_at rewritten: got %v, want %v",
			tomb.DeletedAt.Time, tombDeletedAt.Time)
	}

	if tomb.Title != "" || tomb.Body != "" || tomb.BodyHTML != "" {
		t.Errorf("tombstone PII not scrubbed: title=%q body=%q", tomb.Title, tomb.Body)
	}

	if tomb.Slug != "mentor-deleted-"+tombID.String() {
		t.Errorf("tombstone slug = %q, want mentor-deleted-%s", tomb.Slug, tombID.String())
	}

	if tomb.Status != "archived" {
		t.Errorf("tombstone status = %q, want archived", tomb.Status)
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

	// Idempotent: a second run rewrites the same deterministic values
	// (slug/hash are derived from the row id, deleted_at is COALESCEd),
	// so a retried deletion job — or the hardDeleteUser tx running the
	// scrub again — stays safe.
	if err := q.AnonymizeMentorshipShadowListings(ctx, mentorID); err != nil {
		t.Fatalf("second AnonymizeMentorshipShadowListings: %v", err)
	}

	again := readShadowState(t, database, shadowID)
	if !again.DeletedAt.Valid || again.Title != "" || again.Slug != wantSlug {
		t.Error("second run mutated the tombstone row")
	}

	againTomb := readShadowState(t, database, tombID)
	if !againTomb.DeletedAt.Time.Equal(tombDeletedAt.Time) {
		t.Error("second run rewrote the opt-out tombstone timestamp")
	}
}

// TestAnonymizeShadowMediaFields_LiveDB proves the §14 follow-up to the
// erasure scrub: a body embed that pins a media row through
// content_media_fields releases it once the shadow is anonymized and its
// refs re-derived. Mirrors the sync block inside the anonymize and
// hard-delete paths.
func TestAnonymizeShadowMediaFields_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	q := gen.New(database.Writer)
	uniq := uniqTag("gdpr-cmf-")

	mentorID := insertGDPRTestUser(t, database, uniq+"m")

	var mediaID pgtype.UUID
	mediaPath := "gdpr-cmf/" + uniq + ".jpg"
	if err := database.Writer.QueryRow(ctx,
		`INSERT INTO media (uploader_user_id, original_filename, mime_type, file_size_bytes, storage_path, storage_bucket)
		 VALUES ($1, 'bio.jpg', 'image/jpeg', 1, $2, 'japaconnect') RETURNING id`,
		mentorID, mediaPath).Scan(&mediaID); err != nil {
		t.Fatalf("insert media: %v", err)
	}

	shadowID, _ := insertShadowListing(t, database, mentorID, "", "mentor-"+uniq+"m")
	if _, err := database.Writer.Exec(ctx,
		`UPDATE contents SET body = 'bio with photo ![p](media/serve/japaconnect/`+mediaPath+`)' WHERE id = $1`,
		shadowID); err != nil {
		t.Fatalf("embed media in shadow body: %v", err)
	}

	// The shadow write bypasses content.Service, so its media refs are
	// registered by the store-level sync — same call, direct.
	if err := content.SyncMediaRefs(ctx, q, q, shadowID, content.MediaRefSource{
		ContentType: "service_listing",
		Body:        "bio with photo ![p](media/serve/japaconnect/" + mediaPath + ")",
	}); err != nil {
		t.Fatalf("initial media sync: %v", err)
	}

	var fieldKey string
	if err := database.Writer.QueryRow(ctx,
		`SELECT field_key FROM content_media_fields WHERE content_id = $1 AND media_id = $2`,
		shadowID, mediaID).Scan(&fieldKey); err != nil {
		t.Fatalf("expected tracked body_block ref: %v", err)
	}
	if fieldKey != "body_block" {
		t.Fatalf("field_key = %q, want body_block", fieldKey)
	}

	// Mirror the service: list survivors, scrub, re-sync each.
	rows, err := q.ListMentorshipShadowMediaFields(ctx, mentorID)
	if err != nil {
		t.Fatalf("ListMentorshipShadowMediaFields: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != shadowID {
		t.Fatalf("list returned %d rows, want the one shadow", len(rows))
	}
	if err := q.AnonymizeMentorshipShadowListings(ctx, mentorID); err != nil {
		t.Fatalf("AnonymizeMentorshipShadowListings: %v", err)
	}
	for _, row := range rows {
		if err := content.SyncMediaRefs(ctx, q, q, row.ID, content.MediaRefSource{
			ContentType: "service_listing",
			FieldValues: row.FieldValues,
		}); err != nil {
			t.Fatalf("post-scrub media sync: %v", err)
		}
	}

	var n int
	if err := database.Writer.QueryRow(ctx,
		`SELECT count(*) FROM content_media_fields WHERE content_id = $1`,
		shadowID).Scan(&n); err != nil {
		t.Fatalf("count refs: %v", err)
	}
	if n != 0 {
		t.Fatalf("%d media refs survived the scrub, want 0", n)
	}
}
