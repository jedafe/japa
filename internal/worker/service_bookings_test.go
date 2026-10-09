// service_bookings_test.go — live-DB coverage for the §111 booking
// notification handlers, specifically the 000824 provider arm: bookings
// with org_id NULL + provider_user_id set must notify the provider user
// (not fan out to an org that doesn't exist), and the booker arm must
// attribute the transition to the provider user rather than a NULL org.
//
// Skipped unless JC_DATABASE_URL is set, matching the repo's
// integration-test convention (see internal/servicedir live-DB suites).
//
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// workerLiveDB returns a real Postgres-backed *db.DB registered as the
// package pool, or skips when JC_DATABASE_URL is unset.
func workerLiveDB(t *testing.T) *db.DB {
	t.Helper()

	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	prev := dbPool
	RegisterDBPool(database)
	t.Cleanup(func() {
		dbPool = prev
		database.Writer.Close()
		database.Reader.Close()
	})

	return database
}

func workerUniq(prefix string) string {
	return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// insertWorkerUser creates a users + user_profiles pair; the notification
// rows it receives are cleaned up with the user.
func insertWorkerUser(t *testing.T, database *db.DB, username string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := database.Writer.QueryRow(t.Context(),
		`INSERT INTO users (email, username, profile_type) VALUES ($1, $2, 'regular') RETURNING id`,
		username+"@svc.test", username).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if _, err := database.Writer.Exec(t.Context(),
		`INSERT INTO user_profiles (user_id) VALUES ($1)`, id); err != nil {
		t.Fatalf("insert user_profiles: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		database.Writer.Exec(ctx, `DELETE FROM notifications WHERE recipient_id = $1 OR actor_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM service_bookings WHERE booker_user_id = $1 OR provider_user_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM user_profiles WHERE user_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})

	return id
}

// insertMentorshipBooking seeds a published mentorship-kind listing owned
// by the provider plus a pending user-provider booking (org_id NULL).
func insertMentorshipBooking(t *testing.T, database *db.DB, providerID, bookerID pgtype.UUID, uniq string) (bookingID pgtype.UUID) {
	t.Helper()
	ctx := t.Context()

	sum := sha256.Sum256([]byte(uniq))
	hash := hex.EncodeToString(sum[:])

	var contentID, metaID pgtype.UUID
	if err := database.Writer.QueryRow(ctx,
		`INSERT INTO contents (author_id, content_type, title, slug, status, published_at, content_hash)
		 VALUES ($1, 'service_listing', $2, $3, 'published', now(), $4) RETURNING id`,
		providerID, "T5 "+uniq, uniq, hash).Scan(&contentID); err != nil {
		t.Fatalf("insert content: %v", err)
	}

	if err := database.Writer.QueryRow(ctx,
		`INSERT INTO service_listing_meta (content_id, service_category, listing_kind, purchase_mode)
		 VALUES ($1, 'mentorship', 'mentorship', 'inquiry') RETURNING id`,
		contentID).Scan(&metaID); err != nil {
		t.Fatalf("insert listing meta: %v", err)
	}

	if err := database.Writer.QueryRow(ctx,
		`INSERT INTO service_bookings
			(listing_meta_id, booker_user_id, provider_user_id, intent, party_size, status, purchase_mode)
		 VALUES ($1, $2, $3, 'inquiry', 1, 'pending', 'inquiry') RETURNING id`,
		metaID, bookerID, providerID).Scan(&bookingID); err != nil {
		t.Fatalf("insert mentorship booking: %v", err)
	}

	t.Cleanup(func() {
		c := context.Background()
		database.Writer.Exec(c, `DELETE FROM notifications WHERE subject_id = $1`, bookingID)
		database.Writer.Exec(c, `DELETE FROM service_bookings WHERE listing_meta_id = $1`, metaID)
		database.Writer.Exec(c, `DELETE FROM service_listing_meta WHERE id = $1`, metaID)
		database.Writer.Exec(c, `DELETE FROM contents WHERE id = $1`, contentID)
	})

	return bookingID
}

func bookingTask(t *testing.T, taskType string, payload map[string]string) *asynq.Task {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	return asynq.NewTask(taskType, raw)
}

type notificationRow struct {
	RecipientID string
	ActorType   *string
	ActorID     *string
	ActionURL   *string
}

func notificationsFor(t *testing.T, database *db.DB, bookingID pgtype.UUID) []notificationRow {
	t.Helper()

	rows, err := database.Reader.Query(t.Context(),
		`SELECT recipient_id::text, actor_type, actor_id::text, action_url
		   FROM notifications WHERE subject_id = $1 ORDER BY created_at`, bookingID)
	if err != nil {
		t.Fatalf("query notifications: %v", err)
	}
	defer rows.Close()

	var out []notificationRow
	for rows.Next() {
		var n notificationRow
		if err := rows.Scan(&n.RecipientID, &n.ActorType, &n.ActorID, &n.ActionURL); err != nil {
			t.Fatalf("scan notification: %v", err)
		}
		out = append(out, n)
	}

	return out
}

// TestServiceBookingCreated_ProviderArm_LiveDB — an org-NULL provider
// booking must notify the provider user directly: the old org fan-out
// resolved zero members and the mentor was never told.
func TestServiceBookingCreated_ProviderArm_LiveDB(t *testing.T) {
	database := workerLiveDB(t)
	uniq := workerUniq("w5-")

	mentorID := insertWorkerUser(t, database, "w5mentor"+uniq)
	menteeID := insertWorkerUser(t, database, "w5mentee"+uniq)
	bookingID := insertMentorshipBooking(t, database, mentorID, menteeID, uniq)

	err := HandleServiceBookingCreated(t.Context(), bookingTask(t, TypeServiceBookingCreated,
		map[string]string{"booking_id": bookingID.String()}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	got := notificationsFor(t, database, bookingID)
	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}

	n := got[0]
	if n.RecipientID != mentorID.String() {
		t.Errorf("recipient = %s, want provider %s", n.RecipientID, mentorID)
	}
	if n.ActorType == nil || *n.ActorType != "user" {
		t.Errorf("actor_type = %v, want user", n.ActorType)
	}
	if n.ActorID == nil || *n.ActorID != menteeID.String() {
		t.Errorf("actor_id = %v, want booker %s", n.ActorID, menteeID)
	}
	wantURL := "/messages/mentorship?b=" + bookingID.String()
	if n.ActionURL == nil || *n.ActionURL != wantURL {
		t.Errorf("action_url = %v, want %s", n.ActionURL, wantURL)
	}
}

// TestServiceBookingStatusChanged_MentorshipBookerArm_LiveDB — the booker
// of a mentorship booking gets the transition notification with the
// provider user (not a NULL org) as actor, deep-linked to the bookings
// inbox.
func TestServiceBookingStatusChanged_MentorshipBookerArm_LiveDB(t *testing.T) {
	database := workerLiveDB(t)
	uniq := workerUniq("w5s-")

	mentorID := insertWorkerUser(t, database, "w5mentor"+uniq)
	menteeID := insertWorkerUser(t, database, "w5mentee"+uniq)
	bookingID := insertMentorshipBooking(t, database, mentorID, menteeID, uniq)

	// Move the row to the state the task reports — RespondToBooking
	// enqueues status_changed only after the transition commits.
	if _, err := database.Writer.Exec(t.Context(),
		`UPDATE service_bookings SET status = 'confirmed', confirmed_at = now() WHERE id = $1`,
		bookingID); err != nil {
		t.Fatalf("confirm booking: %v", err)
	}

	err := HandleServiceBookingStatusChanged(t.Context(), bookingTask(t, TypeServiceBookingStatusChanged,
		map[string]string{"booking_id": bookingID.String(), "status": "confirmed"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	got := notificationsFor(t, database, bookingID)
	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}

	n := got[0]
	if n.RecipientID != menteeID.String() {
		t.Errorf("recipient = %s, want booker %s", n.RecipientID, menteeID)
	}
	if n.ActorType == nil || *n.ActorType != "user" {
		t.Errorf("actor_type = %v, want user (provider)", n.ActorType)
	}
	if n.ActorID == nil || *n.ActorID != mentorID.String() {
		t.Errorf("actor_id = %v, want provider %s", n.ActorID, mentorID)
	}
	wantURL := "/messages/bookings?b=" + bookingID.String()
	if n.ActionURL == nil || *n.ActionURL != wantURL {
		t.Errorf("action_url = %v, want %s", n.ActionURL, wantURL)
	}
}

// TestServiceBookingDispute_ProviderArm_LiveDB — a booker-raised dispute
// on a mentorship booking reaches the provider (audience=org inverts to
// the provider arm), not an empty org member list.
func TestServiceBookingDispute_ProviderArm_LiveDB(t *testing.T) {
	database := workerLiveDB(t)
	uniq := workerUniq("w5d-")

	mentorID := insertWorkerUser(t, database, "w5mentor"+uniq)
	menteeID := insertWorkerUser(t, database, "w5mentee"+uniq)
	bookingID := insertMentorshipBooking(t, database, mentorID, menteeID, uniq)

	if _, err := database.Writer.Exec(t.Context(),
		`UPDATE service_bookings SET status = 'disputed', dispute_reason = 'no-show' WHERE id = $1`,
		bookingID); err != nil {
		t.Fatalf("dispute booking: %v", err)
	}

	err := HandleServiceBookingStatusChanged(t.Context(), bookingTask(t, TypeServiceBookingStatusChanged,
		map[string]string{"booking_id": bookingID.String(), "status": "disputed", "audience": "org"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	got := notificationsFor(t, database, bookingID)
	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}

	n := got[0]
	if n.RecipientID != mentorID.String() {
		t.Errorf("recipient = %s, want provider %s", n.RecipientID, mentorID)
	}
	if n.ActorID == nil || *n.ActorID != menteeID.String() {
		t.Errorf("actor_id = %v, want booker %s (the disputer)", n.ActorID, menteeID)
	}
	wantURL := "/messages/mentorship?b=" + bookingID.String()
	if n.ActionURL == nil || *n.ActionURL != wantURL {
		t.Errorf("action_url = %v, want %s", n.ActionURL, wantURL)
	}
}
