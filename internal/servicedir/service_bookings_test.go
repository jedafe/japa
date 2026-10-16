// service_bookings_test.go — unit coverage for the §111 pure/validator
// paths that don't need a database: enum validators, departure input
// validation, platform-fee computation, and the booker-cancel refund
// policy. Store-backed paths are covered by integration tests.
//
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
package servicedir

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// fakeSettings is an in-memory SettingsReader for unit tests.
type fakeSettings map[string]string

func (f fakeSettings) GetBoolOr(_ context.Context, key string, fallback bool) bool {
	if v, ok := f[key]; ok {
		return v == "true"
	}

	return fallback
}

func (f fakeSettings) GetIntOr(_ context.Context, key string, fallback int) int {
	if v, ok := f[key]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}

	return fallback
}

func (f fakeSettings) GetFloatOr(_ context.Context, key string, fallback float64) float64 {
	if v, ok := f[key]; ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}

	return fallback
}

func (f fakeSettings) GetStringOr(_ context.Context, key, fallback string) string {
	if v, ok := f[key]; ok {
		return v
	}

	return fallback
}

func TestIsValidListingKind(t *testing.T) {
	valid := []string{"service", "mentorship"}
	for _, v := range valid {
		if !IsValidListingKind(v) {
			t.Errorf("expected %q to be a valid listing_kind", v)
		}
	}

	// 'package' was removed from the feature — legacy rows stay
	// in the DB but the kind is no longer writable.
	for _, v := range []string{"", "bundle", "SERVICE", "pkg", "package"} {
		if IsValidListingKind(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestIsValidPurchaseMode(t *testing.T) {
	for _, v := range []string{"inquiry", "external", "paid"} {
		if !IsValidPurchaseMode(v) {
			t.Errorf("expected %q to be a valid purchase_mode", v)
		}
	}

	// 'full' is deliberately rejected — deferred per §111 Non-Goals.
	for _, v := range []string{"", "full", "deposit", "PAID"} {
		if IsValidPurchaseMode(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestIsValidBookingStatus(t *testing.T) {
	for _, v := range []string{"pending", "confirmed", "fulfilled", "cancelled", "declined", "disputed", "refunded"} {
		if !IsValidBookingStatus(v) {
			t.Errorf("expected %q to be a valid booking status", v)
		}
	}

	for _, v := range []string{"", "expired", "active", "PENDING"} {
		if IsValidBookingStatus(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestIsValidPricePer(t *testing.T) {
	for _, v := range []string{"person", "package", "group"} {
		if !IsValidPricePer(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}

	for _, v := range []string{"", "seat", "unit"} {
		if IsValidPricePer(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestValidatePackageDepartures(t *testing.T) {
	t.Run("empty set is valid", func(t *testing.T) {
		params, err := validatePackageDepartures(nil)
		if err != nil || params != nil {
			t.Fatalf("expected nil params, no error; got %v, %v", params, err)
		}
	})

	t.Run("requires starts_on", func(t *testing.T) {
		_, err := validatePackageDepartures([]PackageDepartureInput{{EndsOn: "2026-01-01"}})
		if err == nil {
			t.Fatal("expected error for missing starts_on")
		}
	})

	t.Run("rejects ends_on before starts_on", func(t *testing.T) {
		_, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			EndsOn:   "2026-05-01",
		}})
		if err == nil {
			t.Fatal("expected error for ends_on preceding starts_on")
		}
	})

	t.Run("rejects non-positive capacity", func(t *testing.T) {
		zero := int32(0)

		_, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			Capacity: &zero,
		}})
		if err == nil {
			t.Fatal("expected error for zero capacity")
		}
	})

	// 'full' is system-managed but accepted so a loaded row round-trips
	// unchanged through the editor — re-saving a full departure must not
	// 400. Unknown statuses still reject.
	t.Run("accepts 'full' status for round-trip", func(t *testing.T) {
		params, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			Status:   DepartureStatusFull,
		}})
		if err != nil {
			t.Fatalf("unexpected error for 'full' status: %v", err)
		}

		if *params[0].Status != DepartureStatusFull {
			t.Fatalf("status not preserved: %s", *params[0].Status)
		}
	})

	t.Run("rejects unknown status", func(t *testing.T) {
		_, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			Status:   "bogus",
		}})
		if err == nil {
			t.Fatal("expected error for unknown status")
		}
	})

	t.Run("accepts a well-formed open departure", func(t *testing.T) {
		cap := int32(12)

		params, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			EndsOn:   "2026-06-10",
			Capacity: &cap,
			Status:   DepartureStatusOpen,
		}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(params) != 1 || params[0].Capacity == nil || *params[0].Capacity != 12 {
			t.Fatalf("bad params: %+v", params)
		}
	})
}

func TestComputeFee(t *testing.T) {
	ctx := t.Context()

	t.Run("percent fee", func(t *testing.T) {
		s := &Service{settings: fakeSettings{
			"service_packages.fee_type": "percent",
			"service_packages.fee_pct":  "10",
		}}
		if fee := s.computeFee(ctx, 500_000); fee != 50_000 {
			t.Fatalf("expected 50000, got %d", fee)
		}
	})

	t.Run("fixed fee", func(t *testing.T) {
		s := &Service{settings: fakeSettings{
			"service_packages.fee_type":  "fixed",
			"service_packages.fee_fixed": "25000",
		}}
		if fee := s.computeFee(ctx, 500_000); fee != 25_000 {
			t.Fatalf("expected 25000, got %d", fee)
		}
	})

	t.Run("fee is capped at the package price", func(t *testing.T) {
		s := &Service{settings: fakeSettings{
			"service_packages.fee_type":  "fixed",
			"service_packages.fee_fixed": "999999",
		}}
		if fee := s.computeFee(ctx, 500_000); fee != 500_000 {
			t.Fatalf("expected fee capped at price 500000, got %d", fee)
		}
	})

	t.Run("defaults to 10 percent when unset", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}
		if fee := s.computeFee(ctx, 200_000); fee != 20_000 {
			t.Fatalf("expected 20000, got %d", fee)
		}
	})
}

func TestCancelFeeAction(t *testing.T) {
	ctx := t.Context()
	s := &Service{settings: fakeSettings{"service_packages.refund_window_days": "7"}}

	t.Run("inquiry bookings carry no fee", func(t *testing.T) {
		row := BookingRow{Intent: BookingIntentInquiry}
		if got := s.cancelFeeAction(ctx, row); got != "none" {
			t.Fatalf("expected none, got %q", got)
		}
	})

	t.Run("booking without payment carries no fee", func(t *testing.T) {
		row := BookingRow{Intent: BookingIntentBooking}
		if got := s.cancelFeeAction(ctx, row); got != "none" {
			t.Fatalf("expected none, got %q", got)
		}
	})

	paymentID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	t.Run("pending booking cancels always refund", func(t *testing.T) {
		row := BookingRow{Intent: BookingIntentBooking, PaymentID: paymentID, Status: BookingStatusPending}
		if got := s.cancelFeeAction(ctx, row); got != "refund" {
			t.Fatalf("expected refund, got %q", got)
		}
	})

	t.Run("confirmed inside the window refunds", func(t *testing.T) {
		row := BookingRow{
			Intent: BookingIntentBooking, PaymentID: paymentID, Status: BookingStatusConfirmed,
			ConfirmedAt: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
		}
		if got := s.cancelFeeAction(ctx, row); got != "refund" {
			t.Fatalf("expected refund, got %q", got)
		}
	})

	t.Run("confirmed outside the window forfeits the fee", func(t *testing.T) {
		row := BookingRow{
			Intent: BookingIntentBooking, PaymentID: paymentID, Status: BookingStatusConfirmed,
			ConfirmedAt: pgtype.Timestamptz{Time: time.Now().Add(-30 * 24 * time.Hour), Valid: true},
		}
		if got := s.cancelFeeAction(ctx, row); got != "retain" {
			t.Fatalf("expected retain, got %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Mentorship-kind bookings (000824 unification) — live-DB coverage.
// Skipped unless JC_DATABASE_URL is set, matching the repo's
// integration-test convention (see internal/spaces/link_geo_test.go).
// ---------------------------------------------------------------------------

// fakeFlags is an in-memory FeatureChecker for tests.
type fakeFlags map[string]bool

func (f fakeFlags) IsEnabled(_ context.Context, key string) bool { return f[key] }

// recordedTask captures an enqueued Asynq task for assertion.
type recordedTask struct {
	taskType string
	payload  []byte
}

// fakeEnqueuer records enqueued tasks instead of dispatching them.
type fakeEnqueuer struct{ tasks []recordedTask }

func (f *fakeEnqueuer) Enqueue(taskType string, payload []byte, _ ...interface{}) error {
	f.tasks = append(f.tasks, recordedTask{taskType: taskType, payload: payload})
	return nil
}

func (f *fakeEnqueuer) hasTask(taskType string) bool {
	for _, t := range f.tasks {
		if t.taskType == taskType {
			return true
		}
	}

	return false
}

// fakeDMCreator records GetOrCreateConversation calls (the §48 DM seam
// that the server wires in a later task).
type fakeDMCreator struct{ pairs [][2]pgtype.UUID }

func (f *fakeDMCreator) GetOrCreateConversation(_ context.Context, a, b pgtype.UUID) (gen.DmConversation, error) {
	f.pairs = append(f.pairs, [2]pgtype.UUID{a, b})
	return gen.DmConversation{}, nil
}

func (f *fakeDMCreator) GetConversationByParticipants(_ context.Context, _, _ pgtype.UUID) (gen.DmConversation, error) {
	return gen.DmConversation{}, pgx.ErrNoRows
}

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

// newTestService builds a Service over the live store with in-memory
// settings/flags.
func newTestService(database *db.DB, flags fakeFlags) *Service {
	return &Service{
		store:    NewStore(gen.New(database.Reader), gen.New(database.Writer), database),
		settings: fakeSettings{},
		checker:  flags,
	}
}

// insertTestUser creates a users + user_profiles pair and schedules the
// rows for deletion (bookings first — they FK to the user).
func insertTestUser(t *testing.T, database *db.DB, username string) pgtype.UUID {
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
		ctx := t.Context()
		database.Writer.Exec(ctx, `DELETE FROM service_bookings WHERE booker_user_id = $1 OR provider_user_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM user_profiles WHERE user_id = $1`, id)
		database.Writer.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})

	return id
}

// setMentor flips the mentor profile fields a mentorship listing relies on.
func setMentor(t *testing.T, database *db.DB, userID pgtype.UUID, capacity, active int16) {
	t.Helper()

	if _, err := database.Writer.Exec(t.Context(),
		`UPDATE user_profiles SET is_mentor = true, mentor_capacity = $2, active_mentee_count = $3 WHERE user_id = $1`,
		userID, capacity, active); err != nil {
		t.Fatalf("set mentor profile: %v", err)
	}
}

// insertListing creates a published service_listing contents row plus its
// service_listing_meta row of the given kind, and schedules cleanup.
func insertListing(t *testing.T, database *db.DB, authorID pgtype.UUID, kind, category, slug string) (contentID, metaID pgtype.UUID) {
	t.Helper()

	sum := sha256.Sum256([]byte(slug))
	hash := hex.EncodeToString(sum[:])

	if err := database.Writer.QueryRow(t.Context(),
		`INSERT INTO contents (author_id, content_type, title, slug, status, published_at, content_hash)
		 VALUES ($1, 'service_listing', $2, $3, 'published', now(), $4) RETURNING id`,
		authorID, "T4 "+slug, slug, hash).Scan(&contentID); err != nil {
		t.Fatalf("insert content: %v", err)
	}

	if err := database.Writer.QueryRow(t.Context(),
		`INSERT INTO service_listing_meta (content_id, service_category, listing_kind, purchase_mode)
		 VALUES ($1, $2, $3, 'inquiry') RETURNING id`,
		contentID, category, kind).Scan(&metaID); err != nil {
		t.Fatalf("insert listing meta: %v", err)
	}

	t.Cleanup(func() {
		ctx := t.Context()
		database.Writer.Exec(ctx,
			`DELETE FROM service_bookings WHERE listing_meta_id = $1`, metaID)
		database.Writer.Exec(ctx, `DELETE FROM service_listing_meta WHERE id = $1`, metaID)
		database.Writer.Exec(ctx, `DELETE FROM contents WHERE id = $1`, contentID)
	})

	return contentID, metaID
}

func uniqSlug(prefix string) string {
	return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// TestMentorshipCreateBooking_LiveDB covers the mentorship arm of
// CreateBooking: forced inquiry shape, goals → note + meta, dedupe,
// capacity, self-request, paid-input rejection, and the feature flag.
func TestMentorshipCreateBooking_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	uniq := uniqSlug("mbook-")

	svc := newTestService(database, fakeFlags{"mentorship.enabled": true})
	svc.SetEnqueuer(&fakeEnqueuer{})

	mentorID := insertTestUser(t, database, "t4mentor"+uniq)
	menteeID := insertTestUser(t, database, "t4mentee"+uniq)
	setMentor(t, database, mentorID, 3, 0)
	contentID, metaID := insertListing(t, database, mentorID, ListingKindMentorship, "mentorship", "t4-"+uniq)

	t.Run("creates a provider-side inquiry booking", func(t *testing.T) {
		res, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{
			Goals: "land a devops role",
		})
		if err != nil {
			t.Fatalf("create mentorship booking: %v", err)
		}

		b := res.Booking
		if b.ProviderUserID != mentorID.String() {
			t.Errorf("provider_user_id = %q, want %q", b.ProviderUserID, mentorID.String())
		}

		if b.OrgID != "" {
			t.Errorf("org_id = %q, want empty (user-provider booking)", b.OrgID)
		}

		if b.Intent != BookingIntentInquiry {
			t.Errorf("intent = %q, want inquiry", b.Intent)
		}

		if b.PartySize != 1 {
			t.Errorf("party_size = %d, want 1", b.PartySize)
		}

		if b.ExpiresAt != "" {
			t.Errorf("expires_at = %q, want empty (mentorship is not SLA-bound)", b.ExpiresAt)
		}

		if b.Goals != "land a devops role" {
			t.Errorf("goals = %q, want %q", b.Goals, "land a devops role")
		}

		if b.Status != BookingStatusPending {
			t.Errorf("status = %q, want pending", b.Status)
		}

		if b.ListingKind != ListingKindMentorship {
			t.Errorf("listing_kind = %q, want mentorship", b.ListingKind)
		}

		if b.ConnectionStatus != "pending" {
			t.Errorf("connection_status = %q, want pending", b.ConnectionStatus)
		}

		// goals must round-trip through meta->>'goals' on the stored row.
		row, err := svc.store.GetBookingDetail(ctx, mustUUID(t, b.ID))
		if err != nil {
			t.Fatalf("get stored booking: %v", err)
		}

		var meta map[string]any
		if err := json.Unmarshal(row.Meta, &meta); err != nil {
			t.Fatalf("meta not valid json: %v", err)
		}

		if meta["goals"] != "land a devops role" {
			t.Errorf("meta.goals = %v, want %q", meta["goals"], "land a devops role")
		}

		if row.Note == nil || *row.Note != "land a devops role" {
			t.Errorf("note = %v, want goals text", row.Note)
		}
	})

	t.Run("duplicate open request rejected", func(t *testing.T) {
		_, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{})
		if !errors.Is(err, ErrDuplicateRequest) {
			t.Fatalf("expected ErrDuplicateRequest, got %v", err)
		}
	})

	t.Run("paid inputs rejected on mentorship kind", func(t *testing.T) {
		if _, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{
			Intent: BookingIntentBooking,
		}); !errors.Is(err, ErrInvalidIntent) {
			t.Fatalf("booking intent: expected ErrInvalidIntent, got %v", err)
		}

		if _, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{
			CouponCode: "SAVE10",
		}); !errors.Is(err, ErrInvalidCoupon) {
			t.Fatalf("coupon: expected ErrInvalidCoupon, got %v", err)
		}

		if _, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{
			TierID: "11111111-1111-1111-1111-111111111111",
		}); !errors.Is(err, ErrInvalidCoupon) {
			t.Fatalf("tier: expected ErrInvalidCoupon, got %v", err)
		}
	})

	t.Run("self-request rejected", func(t *testing.T) {
		_, err := svc.CreateBooking(ctx, mentorID, contentID, CreateBookingInput{})
		if !errors.Is(err, ErrSelfMentorship) {
			t.Fatalf("expected ErrSelfMentorship, got %v", err)
		}
	})

	t.Run("mentor at capacity rejected", func(t *testing.T) {
		setMentor(t, database, mentorID, 1, 1)
		otherID := insertTestUser(t, database, "t4other"+uniq)

		_, err := svc.CreateBooking(ctx, otherID, contentID, CreateBookingInput{})
		if !errors.Is(err, ErrMentorAtCapacity) {
			t.Fatalf("expected ErrMentorAtCapacity, got %v", err)
		}

		// Room frees up → booking succeeds even with packages flag off.
		setMentor(t, database, mentorID, 3, 1)

		if _, err := svc.CreateBooking(ctx, otherID, contentID, CreateBookingInput{}); err != nil {
			t.Fatalf("booking after capacity freed (packages flag off): %v", err)
		}
	})

	t.Run("flag off returns ErrMentorshipDisabled", func(t *testing.T) {
		svcOff := newTestService(database, fakeFlags{})
		thirdID := insertTestUser(t, database, "t4third"+uniq)

		_, err := svcOff.CreateBooking(ctx, thirdID, contentID, CreateBookingInput{})
		if !errors.Is(err, ErrMentorshipDisabled) {
			t.Fatalf("expected ErrMentorshipDisabled, got %v", err)
		}
	})

	// 000825 — the partial unique index is the DB-level backstop for the
	// SELECT-then-INSERT dedupe race: a second open request on the same
	// (booker, listing_meta) pair must 23505 even when the app pre-check
	// never ran (concurrent insert path), and the error must classify as
	// the dedupe sentinel the service layer maps to ErrDuplicateRequest.
	t.Run("open duplicate insert hits the unique index", func(t *testing.T) {
		var pgErr *pgconn.PgError

		_, err := database.Writer.Exec(ctx,
			`INSERT INTO service_bookings
				(listing_meta_id, booker_user_id, provider_user_id, intent, party_size, status, purchase_mode)
			 VALUES ($1, $2, $3, 'inquiry', 1, 'pending', 'inquiry')`,
			metaID, menteeID, mentorID)
		if err == nil {
			t.Fatal("second open booking inserted — idx_svc_bookings_mentorship_open_uniq did not fire")
		}

		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("expected 23505 unique violation, got %v", err)
		}

		if pgErr.ConstraintName != "idx_svc_bookings_mentorship_open_uniq" {
			t.Fatalf("violation came from %q, want idx_svc_bookings_mentorship_open_uniq", pgErr.ConstraintName)
		}

		if !isDuplicateMentorshipBookingErr(err) {
			t.Fatal("isDuplicateMentorshipBookingErr did not classify the violation")
		}

		// Closed statuses keep multi-request freedom — a declined row on
		// the same pair must not collide.
		if _, err := database.Writer.Exec(ctx,
			`INSERT INTO service_bookings
				(listing_meta_id, booker_user_id, provider_user_id, intent, party_size, status, purchase_mode)
			 VALUES ($1, $2, $3, 'inquiry', 1, 'declined', 'inquiry')`,
			metaID, menteeID, mentorID); err != nil {
			t.Fatalf("closed-status row should not hit the open dedupe index: %v", err)
		}

		// Org-owned bookings are outside the predicate — an org booking on
		// a different meta (same booker) is untouched by the index.
		var orgID pgtype.UUID
		if err := database.Writer.QueryRow(ctx,
			`INSERT INTO organizations (slug, name, owner_user_id) VALUES ($1, $2, $3) RETURNING id`,
			"t4org"+uniq, "T4 Org "+uniq, mentorID).Scan(&orgID); err != nil {
			t.Fatalf("insert org: %v", err)
		}

		t.Cleanup(func() {
			database.Writer.Exec(t.Context(),
				`DELETE FROM service_bookings WHERE org_id = $1`, orgID)
			database.Writer.Exec(t.Context(),
				`DELETE FROM organizations WHERE id = $1`, orgID)
		})

		otherContentID, otherMetaID := insertListing(t, database, mentorID, ListingKindService, "other", "t4svc-"+uniq)
		_ = otherContentID

		if _, err := database.Writer.Exec(ctx,
			`INSERT INTO service_bookings
				(listing_meta_id, booker_user_id, org_id, intent, party_size, status, purchase_mode)
			 VALUES ($1, $2, $3, 'inquiry', 1, 'pending', 'inquiry')`,
			otherMetaID, menteeID, orgID); err != nil {
			t.Fatalf("org-owned open booking should not hit the mentorship dedupe index: %v", err)
		}
	})
}

// TestIsDuplicateMentorshipBookingErr covers the 23505 classifier — only
// the open-request index maps to ErrDuplicateRequest; other unique
// violations stay opaque errors.
func TestIsDuplicateMentorshipBookingErr(t *testing.T) {
	dup := &pgconn.PgError{Code: "23505", ConstraintName: "idx_svc_bookings_mentorship_open_uniq"}
	if !isDuplicateMentorshipBookingErr(dup) {
		t.Error("expected true for the open dedupe index")
	}

	if !isDuplicateMentorshipBookingErr(fmt.Errorf("insert: %w", dup)) {
		t.Error("expected true through wrapping")
	}

	if isDuplicateMentorshipBookingErr(&pgconn.PgError{Code: "23505", ConstraintName: "other_idx"}) {
		t.Error("other unique violations must not classify as duplicate request")
	}

	if isDuplicateMentorshipBookingErr(&pgconn.PgError{Code: "23503"}) {
		t.Error("FK violations must not classify as duplicate request")
	}

	if isDuplicateMentorshipBookingErr(errors.New("boom")) {
		t.Error("plain errors must not classify")
	}
}

// TestMentorshipRespondAndList_LiveDB covers RespondToBooking transitions,
// GetBooking provider auth, and the role=provider inbox.
func TestMentorshipRespondAndList_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	uniq := uniqSlug("mresp-")

	svc := newTestService(database, fakeFlags{"mentorship.enabled": true})
	queue := &fakeEnqueuer{}
	dm := &fakeDMCreator{}

	svc.SetEnqueuer(queue)
	svc.SetDMCreator(dm)

	mentorID := insertTestUser(t, database, "t4mentor"+uniq)
	menteeID := insertTestUser(t, database, "t4mentee"+uniq)
	strangerID := insertTestUser(t, database, "t4stranger"+uniq)
	setMentor(t, database, mentorID, 5, 0)
	contentID, _ := insertListing(t, database, mentorID, ListingKindMentorship, "mentorship", "t4-"+uniq)

	res, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{Goals: "g"})
	if err != nil {
		t.Fatalf("seed booking: %v", err)
	}

	bookingID := mustUUID(t, res.Booking.ID)

	t.Run("stranger cannot respond or view", func(t *testing.T) {
		if _, err := svc.RespondToBooking(ctx, strangerID, bookingID, "accept", ""); !errors.Is(err, ErrNotOwner) {
			t.Fatalf("respond as stranger: expected ErrNotOwner, got %v", err)
		}

		if _, err := svc.GetBooking(ctx, bookingID, strangerID, false); !errors.Is(err, ErrNotOwner) {
			t.Fatalf("get as stranger: expected ErrNotOwner, got %v", err)
		}
	})

	t.Run("provider sees booking and pending list", func(t *testing.T) {
		dto, err := svc.GetBooking(ctx, bookingID, mentorID, false)
		if err != nil {
			t.Fatalf("get as provider: %v", err)
		}

		if dto.ProviderUserID != mentorID.String() {
			t.Errorf("provider_user_id = %q, want %q", dto.ProviderUserID, mentorID.String())
		}

		rows, total, err := svc.ListProviderBookings(ctx, mentorID, "", "", ListingKindMentorship, 1, 20)
		if err != nil {
			t.Fatalf("provider inbox: %v", err)
		}

		if total < 1 || len(rows) == 0 {
			t.Fatalf("provider inbox empty, want the seeded booking")
		}

		found := false

		for _, r := range rows {
			if r.ID == res.Booking.ID {
				found = true
			}
		}

		if !found {
			t.Errorf("provider inbox missing booking %s", res.Booking.ID)
		}

		// A non-provider's inbox is empty.
		other, _, err := svc.ListProviderBookings(ctx, menteeID, "", "", "", 1, 20)
		if err != nil {
			t.Fatalf("non-provider inbox: %v", err)
		}

		if len(other) != 0 {
			t.Errorf("non-provider inbox has %d rows, want 0", len(other))
		}
	})

	t.Run("accept confirms and fires mentorship side effects", func(t *testing.T) {
		dto, err := svc.RespondToBooking(ctx, mentorID, bookingID, "accept", "happy to help")
		if err != nil {
			t.Fatalf("accept: %v", err)
		}

		if dto.Status != BookingStatusConfirmed {
			t.Errorf("status = %q, want confirmed", dto.Status)
		}

		if dto.ConnectionStatus != "active" {
			t.Errorf("connection_status = %q, want active", dto.ConnectionStatus)
		}

		if dto.OrgResponseNote != "happy to help" {
			t.Errorf("org_response_note = %q, want %q", dto.OrgResponseNote, "happy to help")
		}

		if !queue.hasTask(worker.TypeServiceBookingStatusChanged) {
			t.Error("expected service:booking_status_changed task")
		}

		if !queue.hasTask(worker.TypeMentorUpdateMeilisearch) {
			t.Error("expected mentor:update_meilisearch task on mentorship accept")
		}

		if len(dm.pairs) != 1 || dm.pairs[0][0] != mentorID || dm.pairs[0][1] != menteeID {
			t.Errorf("expected DM open (mentor, mentee), got %+v", dm.pairs)
		}
	})

	t.Run("invalid transitions rejected", func(t *testing.T) {
		if _, err := svc.RespondToBooking(ctx, mentorID, bookingID, "accept", ""); !errors.Is(err, ErrInvalidBookingTransition) {
			t.Fatalf("re-accept: expected ErrInvalidBookingTransition, got %v", err)
		}

		if _, err := svc.RespondToBooking(ctx, mentorID, bookingID, "bogus", ""); !errors.Is(err, ErrInvalidBookingTransition) {
			t.Fatalf("bogus action: expected ErrInvalidBookingTransition, got %v", err)
		}
	})

	t.Run("complete fulfils", func(t *testing.T) {
		dto, err := svc.RespondToBooking(ctx, mentorID, bookingID, "complete", "")
		if err != nil {
			t.Fatalf("complete: %v", err)
		}

		if dto.Status != BookingStatusFulfilled {
			t.Errorf("status = %q, want fulfilled", dto.Status)
		}

		if dto.ConnectionStatus != "completed" {
			t.Errorf("connection_status = %q, want completed", dto.ConnectionStatus)
		}
	})

	t.Run("decline moves pending to declined with response note", func(t *testing.T) {
		declineeID := insertTestUser(t, database, "t4declinee"+uniq)

		res, err := svc.CreateBooking(ctx, declineeID, contentID, CreateBookingInput{Goals: "g"})
		if err != nil {
			t.Fatalf("seed decline booking: %v", err)
		}

		dBookingID := mustUUID(t, res.Booking.ID)

		dto, err := svc.RespondToBooking(ctx, mentorID, dBookingID, "decline", "no capacity this season")
		if err != nil {
			t.Fatalf("decline: %v", err)
		}

		if dto.Status != BookingStatusDeclined {
			t.Errorf("status = %q, want declined", dto.Status)
		}

		if dto.ConnectionStatus != BookingStatusDeclined {
			t.Errorf("connection_status = %q, want declined passthrough", dto.ConnectionStatus)
		}

		if dto.OrgResponseNote != "no capacity this season" {
			t.Errorf("org_response_note = %q, want decline note", dto.OrgResponseNote)
		}

		// A second decline is not a valid transition out of declined.
		if _, err := svc.RespondToBooking(ctx, mentorID, dBookingID, "decline", ""); !errors.Is(err, ErrInvalidBookingTransition) {
			t.Fatalf("re-decline: expected ErrInvalidBookingTransition, got %v", err)
		}
		// Nor is decline from the fulfilled booking in the parent test.
		if _, err := svc.RespondToBooking(ctx, mentorID, bookingID, "decline", ""); !errors.Is(err, ErrInvalidBookingTransition) {
			t.Fatalf("decline fulfilled: expected ErrInvalidBookingTransition, got %v", err)
		}
	})

	t.Run("booker-side list carries provider fields and kind filter", func(t *testing.T) {
		rows, _, err := svc.ListMyBookings(ctx, menteeID, "", "", ListingKindMentorship, 1, 20)
		if err != nil {
			t.Fatalf("my bookings: %v", err)
		}

		if len(rows) != 1 {
			t.Fatalf("my bookings returned %d rows, want 1", len(rows))
		}

		if rows[0].ProviderUserID != mentorID.String() {
			t.Errorf("provider_user_id = %q, want %q", rows[0].ProviderUserID, mentorID.String())
		}

		// kind filter excludes non-matching kinds.
		rows2, _, err := svc.ListMyBookings(ctx, menteeID, "", "", ListingKindService, 1, 20)
		if err != nil {
			t.Fatalf("my bookings kind=service: %v", err)
		}

		for _, r := range rows2 {
			if r.ID == res.Booking.ID {
				t.Error("mentorship booking leaked into kind=service list")
			}
		}
	})
}

// TestListMentorshipKind_LiveDB covers the List() flag-pin matrix for
// kind=mentorship.
func TestListMentorshipKind_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	uniq := uniqSlug("mlist-")

	authorID := insertTestUser(t, database, "t4author"+uniq)
	svcContentID, _ := insertListing(t, database, authorID, ListingKindService, "other", "t4svc-"+uniq)
	menContentID, _ := insertListing(t, database, authorID, ListingKindMentorship, "mentorship", "t4men-"+uniq)
	pkgContentID, _ := insertListing(t, database, authorID, ListingKindPackage, "other", "t4pkg-"+uniq)

	// The q filter narrows to our fixtures: titles are "T4 <slug>" where
	// each slug begins with "t4" + uniq — the "T4 t4" prefix matches all
	// three and nothing else in the directory.
	listIDs := func(svc *Service, kind string) map[string]bool {
		t.Helper()

		rows, _, err := svc.List(ctx, pgtype.UUID{}, pgtype.UUID{}, "", "", nil,
			kind, nil, pgtype.Date{}, "T4 t4", "", 1, 100)
		if err != nil {
			t.Fatalf("list kind=%q: %v", kind, err)
		}

		out := map[string]bool{}
		for _, r := range rows {
			out[r.ID] = true
		}

		return out
	}

	t.Run("all flags on: unfiltered shows service+mentorship", func(t *testing.T) {
		svc := newTestService(database, fakeFlags{
			"service_packages.enabled": true,
			"mentorship.enabled":       true,
		})

		rows, _, err := svc.List(ctx, pgtype.UUID{}, pgtype.UUID{}, "", "", nil,
			"", nil, pgtype.Date{}, "T4 t4", "", 1, 100)
		if err != nil {
			t.Fatalf("list: %v", err)
		}

		got := map[string]bool{}
		for _, r := range rows {
			got[r.ID] = true
		}

		for _, id := range []pgtype.UUID{svcContentID, menContentID} {
			if !got[id.String()] {
				t.Errorf("unfiltered list missing %s", id)
			}
		}
		// The 'package' kind was removed from the feature — its
		// rows stay hidden even with service_packages.enabled on.
		if got[pkgContentID.String()] {
			t.Error("package listing leaked into unfiltered list")
		}
	})

	t.Run("packages off + mentorship on: unfiltered shows service+mentorship", func(t *testing.T) {
		svc := newTestService(database, fakeFlags{"mentorship.enabled": true})

		got := listIDs(svc, "")
		if !got[svcContentID.String()] {
			t.Error("missing service listing")
		}

		if !got[menContentID.String()] {
			t.Error("missing mentorship listing — should surface in unfiltered list")
		}

		if got[pkgContentID.String()] {
			t.Error("package listing leaked into unfiltered list")
		}
	})

	t.Run("mentorship off: unfiltered shows only service", func(t *testing.T) {
		svc := newTestService(database, fakeFlags{"service_packages.enabled": true})

		got := listIDs(svc, "")
		if got[menContentID.String()] {
			t.Error("mentorship listing leaked while mentorship.enabled off")
		}

		if !got[svcContentID.String()] {
			t.Error("missing service listing")
		}
		// The 'package' kind was removed — hidden even with the
		// packages flag on.
		if got[pkgContentID.String()] {
			t.Error("package listing leaked into unfiltered list")
		}
	})

	t.Run("both off: unfiltered shows only service", func(t *testing.T) {
		svc := newTestService(database, fakeFlags{})

		got := listIDs(svc, "")
		if !got[svcContentID.String()] {
			t.Error("missing service listing")
		}

		if got[menContentID.String()] || got[pkgContentID.String()] {
			t.Error("flag-gated listing leaked into unfiltered list")
		}
	})

	t.Run("kind=mentorship flag-gated", func(t *testing.T) {
		svcOn := newTestService(database, fakeFlags{"mentorship.enabled": true})

		got := listIDs(svcOn, ListingKindMentorship)
		if !got[menContentID.String()] || len(got) != 1 {
			t.Errorf("kind=mentorship returned %v, want only the mentorship listing", got)
		}

		svcOff := newTestService(database, fakeFlags{})

		_, _, err := svcOff.List(ctx, pgtype.UUID{}, pgtype.UUID{}, "", "", nil,
			ListingKindMentorship, nil, pgtype.Date{}, "T4 t4", "", 1, 100)
		if !errors.Is(err, ErrMentorshipDisabled) {
			t.Fatalf("kind=mentorship with flag off: expected ErrMentorshipDisabled, got %v", err)
		}
	})

	// Task 11 fix round 2 — the handler-level pin: HandleList's upfront
	// directory gate used to 404 every request while
	// service_directory.enabled was off, which would strand Task 12's
	// /mentors → /services?kind=mentorship redirect in a mentorship-only
	// deployment. An explicit kind=mentorship request now gets the
	// kind-appropriate gate; the bare list keeps the directory gate.
	t.Run("handler gate: directory off keeps kind=mentorship reachable", func(t *testing.T) {
		list := func(flags fakeFlags, rawQuery string) (int, map[string]bool) {
			t.Helper()

			h := &Handlers{service: newTestService(database, flags)}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/services?"+rawQuery, nil)
			rec := httptest.NewRecorder()
			h.HandleList(rec, req)

			got := map[string]bool{}

			if rec.Code == http.StatusOK {
				var body struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}

				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode list body: %v", err)
				}

				for _, row := range body.Data {
					got[row.ID] = true
				}
			}

			return rec.Code, got
		}

		// Directory off + mentorship on + explicit kind → mentorship rows.
		code, got := list(fakeFlags{"mentorship.enabled": true}, "kind=mentorship&q=T4+t4")
		if code != http.StatusOK {
			t.Fatalf("kind=mentorship with directory off + mentorship on: got %d, want 200", code)
		}

		if !got[menContentID.String()] || len(got) != 1 {
			t.Errorf("kind=mentorship returned %v, want only the mentorship listing", got)
		}

		// The bare list keeps the directory gate.
		if code, _ := list(fakeFlags{"mentorship.enabled": true}, "q=T4+t4"); code != http.StatusNotFound {
			t.Errorf("bare list with directory off: got %d, want 404", code)
		}

		// kind=all normalizes to the bare request — still directory-gated.
		if code, _ := list(fakeFlags{"mentorship.enabled": true}, "kind=all&q=T4+t4"); code != http.StatusNotFound {
			t.Errorf("kind=all with directory off: got %d, want 404", code)
		}

		// Explicit kind with BOTH flags off → the kind gate still applies.
		if code, _ := list(fakeFlags{}, "kind=mentorship&q=T4+t4"); code != http.StatusNotFound {
			t.Errorf("kind=mentorship with both flags off: got %d, want 404", code)
		}

		// kind=package stays directory-gated even with packages on.
		if code, _ := list(fakeFlags{"service_packages.enabled": true}, "kind=package&q=T4+t4"); code != http.StatusNotFound {
			t.Errorf("kind=package with directory off: got %d, want 404", code)
		}
	})
}

// readThroughContent satisfies the ContentService seam for tests that
// exercise EditOwn/loadListingForEdit: reads run the real contents query,
// writes fail — the mentorship-kind refusal in EditOwn fires before any
// write path is reached.
type readThroughContent struct{ q *gen.Queries }

func (r *readThroughContent) CreateContent(context.Context, content.CreateContentInput) (gen.CreateContentRow, error) {
	return gen.CreateContentRow{}, errors.New("readThroughContent: writes are unsupported in tests")
}

func (r *readThroughContent) GetContent(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error) {
	return r.q.GetContentByID(ctx, id)
}

func (r *readThroughContent) UpdateContent(context.Context, pgtype.UUID, pgtype.UUID, content.UpdateContentInput, bool) (gen.UpdateContentRow, error) {
	return gen.UpdateContentRow{}, errors.New("readThroughContent: writes are unsupported in tests")
}

func (r *readThroughContent) DeleteContent(context.Context, pgtype.UUID) error {
	return errors.New("readThroughContent: writes are unsupported in tests")
}

// TestMentorshipKindCreateEditGuard_LiveDB pins the 000824 shadow-listing
// write guards from the final review: POST /services must not mint a
// listing_kind='mentorship' row (shadow minting belongs to the mentor-
// profile opt-in path only), and PATCH /services/:id must not edit a
// shadow row — a mentor rewriting their own shadow would desync it from
// the mentor profile it mirrors.
func TestMentorshipKindCreateEditGuard_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	uniq := uniqSlug("mguard-")

	svc := newTestService(database, fakeFlags{"service_directory.enabled": true})
	svc.content = &readThroughContent{q: gen.New(database.Reader)}

	mentorID := insertTestUser(t, database, "t4mentor"+uniq)
	strangerID := insertTestUser(t, database, "t4stranger"+uniq)
	setMentor(t, database, mentorID, 3, 0)
	menContentID, _ := insertListing(t, database, mentorID, ListingKindMentorship, "mentorship", "t4men-"+uniq)
	svcContentID, _ := insertListing(t, database, mentorID, ListingKindService, "other", "t4svc-"+uniq)

	t.Run("create rejects listing_kind=mentorship", func(t *testing.T) {
		_, err := svc.Create(ctx, mentorID, CreateServiceInput{
			Title:           "T4 forged shadow",
			ServiceCategory: "other",
			ListingKind:     ListingKindMentorship,
		}, false)
		if !errors.Is(err, ErrMentorshipListingManaged) {
			t.Fatalf("expected ErrMentorshipListingManaged, got %v", err)
		}

		// No forged shadow may exist beyond the fixture row.
		row, err := svc.store.GetMentorshipListing(ctx, mentorID)
		if err != nil {
			t.Fatalf("shadow lookup: %v", err)
		}

		if row.ID != menContentID {
			t.Fatalf("mentor has a second shadow %s — forged row was written", row.ID)
		}
	})

	t.Run("create still rejects unknown kinds", func(t *testing.T) {
		_, err := svc.Create(ctx, mentorID, CreateServiceInput{
			Title:           "T4 bogus",
			ServiceCategory: "other",
			ListingKind:     "bogus",
		}, false)
		if !errors.Is(err, ErrInvalidListingKind) {
			t.Fatalf("expected ErrInvalidListingKind, got %v", err)
		}
	})

	t.Run("edit refuses the author's own shadow", func(t *testing.T) {
		title := "T4 rewritten shadow"

		_, err := svc.EditOwn(ctx, menContentID, mentorID, EditServiceInput{Title: &title}, false, false)
		if !errors.Is(err, ErrMentorshipListingManaged) {
			t.Fatalf("expected ErrMentorshipListingManaged, got %v", err)
		}
	})

	t.Run("edit refuses shadows for service.edit_any too", func(t *testing.T) {
		title := "T4 rewritten shadow"

		_, err := svc.EditOwn(ctx, menContentID, strangerID, EditServiceInput{Title: &title}, true, true)
		if !errors.Is(err, ErrMentorshipListingManaged) {
			t.Fatalf("expected ErrMentorshipListingManaged, got %v", err)
		}
	})

	t.Run("non-owner edit on a service listing still returns not_owner", func(t *testing.T) {
		title := "T4 hijack"

		_, err := svc.EditOwn(ctx, svcContentID, strangerID, EditServiceInput{Title: &title}, false, false)
		if !errors.Is(err, ErrNotOwner) {
			t.Fatalf("expected ErrNotOwner, got %v", err)
		}
	})

	t.Run("PATCH handler surfaces 400 mentorship_listing_managed", func(t *testing.T) {
		h := &Handlers{service: svc}
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/services/"+menContentID.String(),
			strings.NewReader(`{"title":"T4 rewritten shadow"}`))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey,
			&auth.AuthenticatedUser{UserID: mentorID}))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", menContentID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		h.HandleEdit(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body.String())
		}

		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}

		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode error body: %v", err)
		}

		if body.Error.Code != "mentorship_listing_managed" {
			t.Fatalf("error code = %q, want mentorship_listing_managed", body.Error.Code)
		}
	})
}

// mustUUID parses a string id into pgtype.UUID for store calls in tests.
func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

// statusChangedAudience decodes the service:booking_status_changed payloads
// a fake enqueuer captured and returns their audience fields in order.
func statusChangedAudiences(queue *fakeEnqueuer) []string {
	var out []string

	for _, task := range queue.tasks {
		if task.taskType != worker.TypeServiceBookingStatusChanged {
			continue
		}

		var payload struct {
			Audience string `json:"audience"`
		}

		if err := json.Unmarshal(task.payload, &payload); err == nil {
			out = append(out, payload.Audience)
		}
	}

	return out
}

// TestMentorshipBookingHardening_LiveDB covers the mentorship unification
// hardening round: provider-keyed dedupe, the is_mentor orphan-shadow
// recheck, non-disputable mentorship rows, the provider "cancel" respond
// action, mentee-initiated complete/cancel notifying the provider arm,
// and the admin confirmed-crossing Meilisearch reindex.
func TestMentorshipBookingHardening_LiveDB(t *testing.T) {
	ctx := t.Context()
	database := liveDB(t)
	uniq := uniqSlug("mfix-")

	svc := newTestService(database, fakeFlags{"mentorship.enabled": true})
	queue := &fakeEnqueuer{}
	svc.SetEnqueuer(queue)

	mentorID := insertTestUser(t, database, "t4mentor"+uniq)
	menteeID := insertTestUser(t, database, "t4mentee"+uniq)
	setMentor(t, database, mentorID, 5, 0)
	contentID, _ := insertListing(t, database, mentorID, ListingKindMentorship, "mentorship", "t4m-"+uniq)

	res, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{Goals: "g"})
	if err != nil {
		t.Fatalf("seed booking: %v", err)
	}

	bookingID := mustUUID(t, res.Booking.ID)

	t.Run("dedupe keys on provider, not listing meta", func(t *testing.T) {
		// A re-minted shadow is a DIFFERENT service_listing_meta row for
		// the same mentor — the old (booker, listing_meta) dedupe let a
		// second request through; the provider-keyed index must not.
		secondContentID, _ := insertListing(t, database, mentorID, ListingKindMentorship, "mentorship", "t4m2-"+uniq)
		if _, err := svc.CreateBooking(ctx, menteeID, secondContentID, CreateBookingInput{Goals: "g"}); !errors.Is(err, ErrDuplicateRequest) {
			t.Fatalf("expected ErrDuplicateRequest across re-minted shadow, got %v", err)
		}
	})

	t.Run("orphan shadow rejected when is_mentor off", func(t *testing.T) {
		if _, err := database.Writer.Exec(ctx,
			`UPDATE user_profiles SET is_mentor = false WHERE user_id = $1`, mentorID); err != nil {
			t.Fatalf("unset mentor flag: %v", err)
		}

		thirdID := insertTestUser(t, database, "t4third"+uniq)
		_, err := svc.CreateBooking(ctx, thirdID, contentID, CreateBookingInput{Goals: "g"})

		// Restore before asserting — the flag gates every later subtest,
		// and a t.Cleanup here would run with a canceled context.
		setMentor(t, database, mentorID, 5, 0)

		if !errors.Is(err, ErrBookingUnavailable) {
			t.Fatalf("expected ErrBookingUnavailable, got %v", err)
		}
	})

	t.Run("overlong goals rejected", func(t *testing.T) {
		fourthID := insertTestUser(t, database, "t4fourth"+uniq)

		_, err := svc.CreateBooking(ctx, fourthID, contentID, CreateBookingInput{Goals: strings.Repeat("x", 4001)})
		if !errors.Is(err, ErrInputTooLong) {
			t.Fatalf("expected ErrInputTooLong, got %v", err)
		}
	})

	t.Run("preferred_dates rejected on mentorship kind", func(t *testing.T) {
		// §111 hardening — mentorship requests have no date surface;
		// preferred_dates is rejected outright (400 invalid_input), not
		// silently dropped. The check runs before the dedupe guard so the
		// same mentee (with an open request) still sees invalid_input.
		_, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{
			Goals:          "g",
			PreferredDates: &PreferredDatesInput{StartOn: "2026-01-01", EndOn: "2026-01-10"},
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("mentee cannot dispute a mentorship booking", func(t *testing.T) {
		_, err := svc.DisputeMyBooking(ctx, bookingID, menteeID, "unresponsive mentor")
		if !errors.Is(err, ErrInvalidBookingTransition) {
			t.Fatalf("expected ErrInvalidBookingTransition, got %v", err)
		}
	})

	t.Run("provider cancel via respond", func(t *testing.T) {
		queue.tasks = nil

		dto, err := svc.RespondToBooking(ctx, mentorID, bookingID, "cancel", "moving abroad")
		if err != nil {
			t.Fatalf("provider cancel respond: %v", err)
		}

		if dto.Status != BookingStatusCancelled {
			t.Errorf("status = %q, want cancelled", dto.Status)
		}

		if dto.CancelReason != "moving abroad" {
			t.Errorf("cancel_reason = %q, want the provider note", dto.CancelReason)
		}

		if dto.ConnectionStatus != BookingStatusCancelled {
			t.Errorf("connection_status = %q, want cancelled passthrough", dto.ConnectionStatus)
		}
		// Provider-initiated transitions notify the booker arm.
		for _, a := range statusChangedAudiences(queue) {
			if a != "" {
				t.Errorf("status_changed audience = %q, want booker (empty)", a)
			}
		}
	})

	t.Run("mentee complete fulfils and notifies provider", func(t *testing.T) {
		res, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{Goals: "g2"})
		if err != nil {
			t.Fatalf("seed booking 2: %v", err)
		}

		booking2 := mustUUID(t, res.Booking.ID)
		if _, err := svc.RespondToBooking(ctx, mentorID, booking2, "accept", ""); err != nil {
			t.Fatalf("accept booking 2: %v", err)
		}

		queue.tasks = nil

		dto, err := svc.CompleteMyBooking(ctx, booking2, menteeID)
		if err != nil {
			t.Fatalf("mentee complete: %v", err)
		}

		if dto.Status != BookingStatusFulfilled {
			t.Errorf("status = %q, want fulfilled", dto.Status)
		}

		if dto.ConnectionStatus != "completed" {
			t.Errorf("connection_status = %q, want completed", dto.ConnectionStatus)
		}

		if !queue.hasTask(worker.TypeMentorUpdateMeilisearch) {
			t.Error("expected mentor:update_meilisearch on leaving confirmed")
		}

		for _, a := range statusChangedAudiences(queue) {
			if a != "provider" {
				t.Errorf("status_changed audience = %q, want provider", a)
			}
		}
	})

	t.Run("mentee cancel notifies provider arm", func(t *testing.T) {
		res, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{Goals: "g3"})
		if err != nil {
			t.Fatalf("seed booking 3: %v", err)
		}

		booking3 := mustUUID(t, res.Booking.ID)
		if _, err := svc.RespondToBooking(ctx, mentorID, booking3, "accept", ""); err != nil {
			t.Fatalf("accept booking 3: %v", err)
		}

		queue.tasks = nil

		dto, err := svc.CancelMyBooking(ctx, booking3, menteeID, "found another mentor")
		if err != nil {
			t.Fatalf("mentee cancel: %v", err)
		}

		if dto.Status != BookingStatusCancelled {
			t.Errorf("status = %q, want cancelled", dto.Status)
		}

		if !queue.hasTask(worker.TypeMentorUpdateMeilisearch) {
			t.Error("expected mentor:update_meilisearch on leaving confirmed")
		}

		aud := statusChangedAudiences(queue)
		if len(aud) != 1 || aud[0] != "provider" {
			t.Errorf("status_changed audiences = %v, want exactly one provider task", aud)
		}
	})

	t.Run("admin transition across confirmed reindexes mentor", func(t *testing.T) {
		res, err := svc.CreateBooking(ctx, menteeID, contentID, CreateBookingInput{Goals: "g4"})
		if err != nil {
			t.Fatalf("seed booking 4: %v", err)
		}

		booking4 := mustUUID(t, res.Booking.ID)

		queue.tasks = nil
		// pending → confirmed enters the active set: reindex.
		if _, err := svc.AdminSetBookingStatus(ctx, booking4, AdminBookingActionInput{Status: BookingStatusConfirmed}); err != nil {
			t.Fatalf("admin confirm: %v", err)
		}

		if !queue.hasTask(worker.TypeMentorUpdateMeilisearch) {
			t.Error("expected mentor:update_meilisearch when admin moves booking into confirmed")
		}

		queue.tasks = nil
		// confirmed → cancelled leaves the active set: reindex again.
		if _, err := svc.AdminSetBookingStatus(ctx, booking4, AdminBookingActionInput{Status: BookingStatusCancelled, ResolutionNote: "policy"}); err != nil {
			t.Fatalf("admin cancel: %v", err)
		}

		if !queue.hasTask(worker.TypeMentorUpdateMeilisearch) {
			t.Error("expected mentor:update_meilisearch when admin moves booking out of confirmed")
		}
	})
}
