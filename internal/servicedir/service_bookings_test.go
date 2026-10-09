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
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

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
	valid := []string{"service", "package", "mentorship"}
	for _, v := range valid {
		if !IsValidListingKind(v) {
			t.Errorf("expected %q to be a valid listing_kind", v)
		}
	}

	for _, v := range []string{"", "bundle", "SERVICE", "pkg"} {
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
	ctx := context.Background()

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
	ctx := context.Background()
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
		ctx := context.Background()
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
		ctx := context.Background()
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
			database.Writer.Exec(context.Background(),
				`DELETE FROM service_bookings WHERE org_id = $1`, orgID)
			database.Writer.Exec(context.Background(),
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

	t.Run("all flags on: unfiltered shows every kind", func(t *testing.T) {
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
		for _, id := range []pgtype.UUID{svcContentID, menContentID, pkgContentID} {
			if !got[id.String()] {
				t.Errorf("unfiltered list missing %s", id)
			}
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
			t.Error("package listing leaked while service_packages.enabled off")
		}
	})

	t.Run("packages on + mentorship off: unfiltered hides mentorship", func(t *testing.T) {
		svc := newTestService(database, fakeFlags{"service_packages.enabled": true})
		got := listIDs(svc, "")
		if got[menContentID.String()] {
			t.Error("mentorship listing leaked while mentorship.enabled off")
		}
		if !got[pkgContentID.String()] {
			t.Error("missing package listing")
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
