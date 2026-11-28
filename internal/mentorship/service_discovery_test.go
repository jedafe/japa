// service_discovery_test.go — live-DB coverage for the mentor discovery
// availability filter (§102/§111 hardening): at-capacity mentors are
// hidden by default and surface only with include_unavailable.
// Skipped unless JC_DATABASE_URL is set.
package mentorship

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// liveDB returns a real Postgres-backed *db.DB, or skips the test when
// JC_DATABASE_URL is unset (same convention as servicedir's live tests).
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

// insertMentor seeds a users + user_profiles pair with is_mentor on and
// the given capacity/active counts, plus the published mentorship-kind
// shadow listing the discovery EXISTS gate requires. Cleans up on test end.
func insertMentor(t *testing.T, database *db.DB, username string, capacity, active int16) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := database.Writer.QueryRow(t.Context(),
		`INSERT INTO users (email, username, profile_type) VALUES ($1, $2, 'regular') RETURNING id`,
		username+"@svc.test", username).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if _, err := database.Writer.Exec(t.Context(),
		`INSERT INTO user_profiles (user_id, is_mentor, mentor_capacity, active_mentee_count, mentor_since)
		 VALUES ($1, true, $2, $3, now())`,
		id, capacity, active); err != nil {
		t.Fatalf("insert user_profiles: %v", err)
	}

	sum := sha256.Sum256([]byte(username))
	hash := hex.EncodeToString(sum[:])

	var contentID pgtype.UUID
	if err := database.Writer.QueryRow(t.Context(),
		`INSERT INTO contents (author_id, content_type, title, slug, status, published_at, content_hash)
		 VALUES ($1, 'service_listing', $2, $3, 'published', now(), $4) RETURNING id`,
		id, "M "+username, "mentor-"+username, hash).Scan(&contentID); err != nil {
		t.Fatalf("insert shadow content: %v", err)
	}

	if _, err := database.Writer.Exec(t.Context(),
		`INSERT INTO service_listing_meta (content_id, service_category, listing_kind, purchase_mode)
		 VALUES ($1, 'mentorship', 'mentorship', 'inquiry')`, contentID); err != nil {
		t.Fatalf("insert shadow meta: %v", err)
	}

	t.Cleanup(func() {
		ctx := t.Context()
		database.Writer.Exec(ctx, `DELETE FROM service_listing_meta WHERE content_id = $1`, contentID)
		database.Writer.Exec(ctx, `DELETE FROM contents WHERE id = $1`, contentID)
		database.Writer.Exec(ctx, `DELETE FROM user_profiles WHERE user_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})

	return id
}

// TestListMentorsIncludeUnavailable_LiveDB — an at-capacity mentor
// (available_slots <= 0) is excluded from discovery by default and
// included when include_unavailable is set, in both the plain-list and
// embedding-match query arms.
func TestListMentorsIncludeUnavailable_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	uniq := strconv.FormatInt(time.Now().UnixNano(), 36)

	svc := NewService(database)

	full := insertMentor(t, database, "t5full"+uniq, 1, 1) // at capacity
	open := insertMentor(t, database, "t5open"+uniq, 2, 0) // has slots

	contains := func(mentors []MentorSummary, id pgtype.UUID) bool {
		for _, m := range mentors {
			if m.UserID == id {
				return true
			}
		}

		return false
	}

	t.Run("default excludes at-capacity mentors", func(t *testing.T) {
		mentors, _, err := svc.ListMentors(ctx, pgtype.UUID{}, nil, "newest", nil, false, 100, 0)
		if err != nil {
			t.Fatalf("list mentors: %v", err)
		}

		if contains(mentors, full) {
			t.Error("at-capacity mentor visible without include_unavailable")
		}

		if !contains(mentors, open) {
			t.Error("available mentor missing from default list")
		}
	})

	t.Run("include_unavailable keeps at-capacity mentors", func(t *testing.T) {
		mentors, _, err := svc.ListMentors(ctx, pgtype.UUID{}, nil, "newest", nil, true, 100, 0)
		if err != nil {
			t.Fatalf("list mentors: %v", err)
		}

		if !contains(mentors, full) {
			t.Error("at-capacity mentor missing with include_unavailable")
		}
	})
}

// TestMentorProfileMissingRow_LiveDB — an account with no user_profiles
// row resolves a default profile from GET (schema defaults, not a 500)
// and PATCH upserts the row on first save; a later PATCH takes the
// conflict arm and preserves mentor_since (audit GAP-01).
func TestMentorProfileMissingRow_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	uniq := strconv.FormatInt(time.Now().UnixNano(), 36)

	svc := NewService(database)

	var id pgtype.UUID
	if err := database.Writer.QueryRow(ctx,
		`INSERT INTO users (email, username, profile_type) VALUES ($1, $2, 'regular') RETURNING id`,
		"noprof"+uniq+"@svc.test", "noprof"+uniq).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	t.Cleanup(func() {
		database.Writer.Exec(ctx, `DELETE FROM contents WHERE author_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM user_profiles WHERE user_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})

	// GET resolves a default view instead of surfacing ErrNoRows as a 500.
	prof, err := svc.GetMentorProfile(ctx, id)
	if err != nil {
		t.Fatalf("GetMentorProfile: %v", err)
	}

	if prof.IsMentor == nil || *prof.IsMentor {
		t.Errorf("is_mentor = %v, want false default", prof.IsMentor)
	}

	if prof.MentorCapacity == nil || *prof.MentorCapacity != 3 {
		t.Errorf("mentor_capacity = %v, want schema default 3", prof.MentorCapacity)
	}

	// PATCH upserts the row — the update arm must not rely on a preexisting
	// user_profiles row.
	on := true
	var cap5 int16 = 5

	prof, err = svc.UpdateMentorProfile(ctx, id, &on, &cap5, []string{"visa"}, nil, MentorCardInput{})
	if err != nil {
		t.Fatalf("UpdateMentorProfile (insert arm): %v", err)
	}

	if prof.IsMentor == nil || !*prof.IsMentor ||
		prof.MentorCapacity == nil || *prof.MentorCapacity != 5 {
		t.Errorf("post-upsert profile wrong: %+v", prof)
	}

	// Second PATCH takes the conflict arm; mentor_since must survive an
	// opt-out (restored intact on re-enable).
	off := false

	prof, err = svc.UpdateMentorProfile(ctx, id, &off, nil, nil, nil, MentorCardInput{})
	if err != nil {
		t.Fatalf("UpdateMentorProfile (conflict arm): %v", err)
	}

	if prof.IsMentor == nil || *prof.IsMentor {
		t.Errorf("is_mentor = %v, want false", prof.IsMentor)
	}

	if !prof.MentorSince.Valid {
		t.Error("mentor_since cleared on opt-out — expected preserved")
	}
}
