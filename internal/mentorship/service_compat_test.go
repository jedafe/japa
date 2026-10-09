// service_compat_test.go — unit coverage for the 000824 compatibility
// layer: the legacy↔booking vocabulary translation, the BookingRow →
// Connection mapping, servicedir sentinel mapping, and the delegation
// guards in the connection lifecycle methods. Store-backed behavior is
// covered by internal/servicedir's live-DB tests.
package mentorship

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/servicedir"
)

// fakeBookingBackend is an in-memory BookingBackend recording calls and
// replaying scripted results — enough to exercise the compat guards
// without a database.
type fakeBookingBackend struct {
	listingID  pgtype.UUID
	listingErr error

	rows   map[string]servicedir.BookingRow
	rowErr error

	created       []servicedir.CreateBookingInput
	createBooking servicedir.BookingResult
	createErr     error

	responded    []string // actions seen, "accept|decline|complete"
	respondErr   error
	afterRespond func() // hook run after RespondToBooking (flips fake state)
	cancelled    []string
	cancelErr    error
	providerCanc []string
	completeMine []pgtype.UUID
	adminStatus  []string
	adminErr     error

	ensures []servicedir.MentorShadowInput
	retires []pgtype.UUID
}

func newFakeBackend() *fakeBookingBackend {
	return &fakeBookingBackend{rows: map[string]servicedir.BookingRow{}}
}

func (f *fakeBookingBackend) MentorshipListingContentID(_ context.Context, _ pgtype.UUID) (pgtype.UUID, error) {
	if f.listingErr != nil {
		return pgtype.UUID{}, f.listingErr
	}

	return f.listingID, nil
}

func (f *fakeBookingBackend) CreateBooking(_ context.Context, bookerID, _ pgtype.UUID, in servicedir.CreateBookingInput) (servicedir.BookingResult, error) {
	f.created = append(f.created, in)
	if f.createErr != nil {
		return servicedir.BookingResult{}, f.createErr
	}

	return f.createBooking, nil
}

func (f *fakeBookingBackend) MentorshipBookingRow(_ context.Context, id pgtype.UUID) (servicedir.BookingRow, error) {
	if f.rowErr != nil {
		return servicedir.BookingRow{}, f.rowErr
	}

	row, ok := f.rows[id.String()]
	if !ok {
		return servicedir.BookingRow{}, pgx.ErrNoRows
	}

	return row, nil
}

func (f *fakeBookingBackend) ListMentorshipBookingsByBooker(_ context.Context, _ pgtype.UUID, status string, _, _ int32) ([]servicedir.BookingRow, error) {
	out := make([]servicedir.BookingRow, 0, len(f.rows))
	for _, r := range f.rows {
		if status == "" || r.Status == status {
			out = append(out, r)
		}
	}

	return out, nil
}

func (f *fakeBookingBackend) ListMentorshipBookingsByProvider(_ context.Context, providerID pgtype.UUID, status string, _, _ int32) ([]servicedir.BookingRow, error) {
	out := make([]servicedir.BookingRow, 0, len(f.rows))
	for _, r := range f.rows {
		if r.ProviderUserID != providerID {
			continue
		}
		if status == "" || r.Status == status {
			out = append(out, r)
		}
	}

	return out, nil
}

func (f *fakeBookingBackend) ListMentorshipBookingsAdmin(_ context.Context, status string, providerID, bookerID pgtype.UUID, _, _ int32) ([]servicedir.BookingRow, error) {
	out := make([]servicedir.BookingRow, 0, len(f.rows))
	for _, r := range f.rows {
		if status != "" && r.Status != status {
			continue
		}
		if providerID.Valid && r.ProviderUserID != providerID {
			continue
		}
		if bookerID.Valid && r.BookerUserID != bookerID {
			continue
		}
		out = append(out, r)
	}

	return out, nil
}

func (f *fakeBookingBackend) RespondToBooking(_ context.Context, _, bookingID pgtype.UUID, action, _ string) (servicedir.BookingDTO, error) {
	f.responded = append(f.responded, action)
	if f.afterRespond != nil {
		f.afterRespond()
	}
	if f.respondErr != nil {
		return servicedir.BookingDTO{}, f.respondErr
	}

	return servicedir.BookingDTO{ID: bookingID.String()}, nil
}

func (f *fakeBookingBackend) CompleteMyBooking(_ context.Context, bookingID, _ pgtype.UUID) (servicedir.BookingDTO, error) {
	f.completeMine = append(f.completeMine, bookingID)
	return servicedir.BookingDTO{ID: bookingID.String()}, nil
}

func (f *fakeBookingBackend) CancelMyBooking(_ context.Context, bookingID, _ pgtype.UUID, reason string) (servicedir.BookingDTO, error) {
	f.cancelled = append(f.cancelled, reason)
	if f.cancelErr != nil {
		return servicedir.BookingDTO{}, f.cancelErr
	}

	return servicedir.BookingDTO{ID: bookingID.String()}, nil
}

func (f *fakeBookingBackend) CancelProviderBooking(_ context.Context, _, bookingID pgtype.UUID, reason string) (servicedir.BookingDTO, error) {
	f.providerCanc = append(f.providerCanc, reason)
	if f.cancelErr != nil {
		return servicedir.BookingDTO{}, f.cancelErr
	}

	return servicedir.BookingDTO{ID: bookingID.String()}, nil
}

func (f *fakeBookingBackend) AdminSetBookingStatus(_ context.Context, bookingID pgtype.UUID, in servicedir.AdminBookingActionInput) (servicedir.BookingDTO, error) {
	f.adminStatus = append(f.adminStatus, in.Status)
	if f.adminErr != nil {
		return servicedir.BookingDTO{}, f.adminErr
	}

	return servicedir.BookingDTO{ID: bookingID.String(), Status: in.Status}, nil
}

func (f *fakeBookingBackend) EnsureMentorShadowListing(_ context.Context, _ pgtype.UUID, in servicedir.MentorShadowInput) (pgtype.UUID, error) {
	f.ensures = append(f.ensures, in)
	return f.listingID, nil
}

func (f *fakeBookingBackend) RetireMentorShadowListing(_ context.Context, id pgtype.UUID) error {
	f.retires = append(f.retires, id)
	return nil
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}

	return id
}

func TestBookingStatusTranslation(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"active":    "confirmed",
		"completed": "fulfilled",
		"pending":   "pending",
		"declined":  "declined",
		"cancelled": "cancelled",
		"":          "",
	}
	for in, want := range cases {
		if got := bookingStatus(in); got != want {
			t.Errorf("bookingStatus(%q) = %q, want %q", in, got, want)
		}
		if got := connectionStatus(want); want == "confirmed" || want == "fulfilled" {
			if got != in {
				t.Errorf("connectionStatus(%q) = %q, want %q", want, got, in)
			}
		}
	}
}

func TestMapBookingErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   error
		want error
	}{
		{pgx.ErrNoRows, ErrConnectionNotFound},
		{fmt.Errorf("wrap: %w", pgx.ErrNoRows), ErrConnectionNotFound},
		{servicedir.ErrNotOwner, ErrNotAuthorized},
		{servicedir.ErrInvalidBookingTransition, ErrInvalidStatusChange},
		{servicedir.ErrDuplicateRequest, ErrDuplicateRequest},
		{servicedir.ErrMentorAtCapacity, ErrMentorAtCapacity},
		{servicedir.ErrSelfMentorship, ErrCannotSelfMentor},
		{servicedir.ErrBookingUnavailable, ErrMentorNotFound},
		{servicedir.ErrInvalidID, ErrInvalidDestination},
		{servicedir.ErrMentorshipDisabled, servicedir.ErrMentorshipDisabled}, // passthrough
	}
	for _, tc := range cases {
		if got := mapBookingErr(tc.in); !errors.Is(got, tc.want) {
			t.Errorf("mapBookingErr(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}

	// Unknown errors pass through untouched.
	other := errors.New("boom")
	if got := mapBookingErr(other); !errors.Is(got, other) {
		t.Errorf("mapBookingErr(boom) = %v, want passthrough", got)
	}
}

func TestConnectionFromBookingRow(t *testing.T) {
	t.Parallel()

	mentorID := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	menteeID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	destID := mustUUID(t, "33333333-3333-3333-3333-333333333333")
	now := time.Now().UTC()
	ts := pgtype.Timestamptz{Time: now, Valid: true}
	mentorUser := "mentor_u"
	menteeUser := "mentee_u"
	reason := "moved on"

	row := servicedir.BookingRow{
		ID:                 mustUUID(t, "44444444-4444-4444-4444-444444444444"),
		ProviderUserID:     mentorID,
		BookerUserID:       menteeID,
		DestinationSpaceID: destID,
		Status:             servicedir.BookingStatusConfirmed,
		ListingKind:        servicedir.ListingKindMentorship,
		Meta:               []byte(`{"goals":"ship it","responded_at":"` + now.Format(time.RFC3339Nano) + `"}`),
		ConfirmedAt:        ts,
		CancelledAt:        ts,
		CancelReason:       &reason,
		CreatedAt:          ts,
		UpdatedAt:          ts,
		ProviderUsername:   &mentorUser,
		BookerUsername:     &menteeUser,
	}

	c := connectionFromBookingRow(row)
	if c.MentorID != mentorID || c.MenteeID != menteeID {
		t.Errorf("ids wrong: mentor=%v mentee=%v", c.MentorID, c.MenteeID)
	}
	if c.Status != "active" {
		t.Errorf("status = %q, want active", c.Status)
	}
	if c.Goals == nil || *c.Goals != "ship it" {
		t.Errorf("goals = %v, want meta goals", c.Goals)
	}
	if c.RespondedAt != ts {
		t.Errorf("responded_at = %v, want confirmed_at %v", c.RespondedAt, ts)
	}
	if c.DestinationSpaceID != destID || c.CancelReason == nil || *c.CancelReason != reason {
		t.Errorf("dest/reason mapping wrong: %+v", c)
	}
	if c.MentorUsername == nil || *c.MentorUsername != mentorUser ||
		c.MenteeUsername == nil || *c.MenteeUsername != menteeUser {
		t.Errorf("username joins wrong: %+v", c)
	}

	// fulfilled → completed + completed_at set.
	row.Status = servicedir.BookingStatusFulfilled
	row.FulfilledAt = ts
	c = connectionFromBookingRow(row)
	if c.Status != "completed" || !c.CompletedAt.Valid {
		t.Errorf("fulfilled mapping wrong: status=%q completed_at=%v", c.Status, c.CompletedAt)
	}

	// Declined rows: responded_at rides meta (no confirmed_at).
	row.Status = servicedir.BookingStatusDeclined
	row.ConfirmedAt = pgtype.Timestamptz{}
	c = connectionFromBookingRow(row)
	if c.Status != "declined" || !c.RespondedAt.Valid {
		t.Errorf("declined mapping wrong: status=%q responded_at=%v", c.Status, c.RespondedAt)
	}

	// Note fallback: migrated rows carry goals only in the note column.
	note := "legacy goals"
	row.Meta = nil
	row.Note = &note
	c = connectionFromBookingRow(row)
	if c.Goals == nil || *c.Goals != note {
		t.Errorf("note fallback wrong: goals=%v", c.Goals)
	}
}

func TestConnectionMethodsRequireBackend(t *testing.T) {
	t.Parallel()

	svc := &Service{} // bookings seam unwired
	ctx := context.Background()
	id := mustUUID(t, "55555555-5555-5555-5555-555555555555")

	if _, err := svc.CreateConnection(ctx, id, mustUUID(t, "66666666-6666-6666-6666-666666666666"), nil, pgtype.UUID{}); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("CreateConnection = %v, want errBackendUnavailable", err)
	}
	if _, err := svc.ListOutgoingConnections(ctx, id, "", 10, 0); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("ListOutgoingConnections = %v, want errBackendUnavailable", err)
	}
	if err := svc.CancelOutgoingConnection(ctx, id, id); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("CancelOutgoingConnection = %v, want errBackendUnavailable", err)
	}
	if _, err := svc.ListIncomingConnections(ctx, id, "", 10, 0); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("ListIncomingConnections = %v, want errBackendUnavailable", err)
	}
	if _, err := svc.AcceptConnection(ctx, id, id); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("AcceptConnection = %v, want errBackendUnavailable", err)
	}
	if _, err := svc.CompleteConnection(ctx, id, id); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("CompleteConnection = %v, want errBackendUnavailable", err)
	}
	if _, err := svc.CancelConnection(ctx, id, id, nil); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("CancelConnection = %v, want errBackendUnavailable", err)
	}
	if _, err := svc.ListAllConnections(ctx, "", pgtype.UUID{}, pgtype.UUID{}, 10, 0); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("ListAllConnections = %v, want errBackendUnavailable", err)
	}
	if _, err := svc.AdminUpdateConnection(ctx, id, "active"); !errors.Is(err, errBackendUnavailable) {
		t.Errorf("AdminUpdateConnection = %v, want errBackendUnavailable", err)
	}
}

func TestCreateConnectionDelegates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mentorID := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	menteeID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	listingID := mustUUID(t, "33333333-3333-3333-3333-333333333333")
	bookingID := mustUUID(t, "44444444-4444-4444-4444-444444444444")
	goals := "learn devops"

	backend := newFakeBackend()
	backend.listingID = listingID
	backend.createBooking = servicedir.BookingResult{Booking: servicedir.BookingDTO{ID: bookingID.String()}}
	backend.rows[bookingID.String()] = servicedir.BookingRow{
		ID:             bookingID,
		ProviderUserID: mentorID,
		BookerUserID:   menteeID,
		Status:         servicedir.BookingStatusPending,
		ListingKind:    servicedir.ListingKindMentorship,
	}

	svc := &Service{bookings: backend}
	conn, err := svc.CreateConnection(ctx, mentorID, menteeID, &goals, pgtype.UUID{})
	if err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}

	if conn.ID != bookingID || conn.Status != "pending" {
		t.Errorf("conn = %+v, want id=%s status=pending", conn, bookingID)
	}
	if len(backend.created) != 1 {
		t.Fatalf("CreateBooking calls = %d, want 1", len(backend.created))
	}
	if backend.created[0].Intent != servicedir.BookingIntentInquiry || backend.created[0].Goals != goals {
		t.Errorf("input = %+v, want inquiry + goals", backend.created[0])
	}
}

func TestCreateConnectionSelfMentor(t *testing.T) {
	t.Parallel()

	svc := &Service{bookings: newFakeBackend()}
	id := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	if _, err := svc.CreateConnection(context.Background(), id, id, nil, pgtype.UUID{}); !errors.Is(err, ErrCannotSelfMentor) {
		t.Errorf("CreateConnection self = %v, want ErrCannotSelfMentor", err)
	}
}

func TestCreateConnectionNoListing(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	backend.listingErr = pgx.ErrNoRows
	svc := &Service{bookings: backend}

	_, err := svc.CreateConnection(context.Background(),
		mustUUID(t, "11111111-1111-1111-1111-111111111111"),
		mustUUID(t, "22222222-2222-2222-2222-222222222222"), nil, pgtype.UUID{})
	if !errors.Is(err, ErrMentorNotFound) {
		t.Errorf("CreateConnection = %v, want ErrMentorNotFound", err)
	}
}

func TestAcceptConnectionGuards(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mentorID := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	menteeID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	other := mustUUID(t, "99999999-9999-9999-9999-999999999999")
	connID := mustUUID(t, "44444444-4444-4444-4444-444444444444")

	pending := servicedir.BookingRow{
		ID:             connID,
		ProviderUserID: mentorID,
		BookerUserID:   menteeID,
		Status:         servicedir.BookingStatusPending,
		ListingKind:    servicedir.ListingKindMentorship,
	}

	t.Run("not the mentor → ErrNotAuthorized", func(t *testing.T) {
		backend := newFakeBackend()
		backend.rows[connID.String()] = pending
		svc := &Service{bookings: backend}

		if _, err := svc.AcceptConnection(ctx, other, connID); !errors.Is(err, ErrNotAuthorized) {
			t.Errorf("got %v, want ErrNotAuthorized", err)
		}
	})

	t.Run("not pending → ErrConnectionNotFound", func(t *testing.T) {
		backend := newFakeBackend()
		r := pending
		r.Status = servicedir.BookingStatusConfirmed
		backend.rows[connID.String()] = r
		svc := &Service{bookings: backend}

		if _, err := svc.AcceptConnection(ctx, mentorID, connID); !errors.Is(err, ErrConnectionNotFound) {
			t.Errorf("got %v, want ErrConnectionNotFound", err)
		}
	})

	t.Run("accepts and maps to legacy vocabulary", func(t *testing.T) {
		backend := newFakeBackend()
		backend.rows[connID.String()] = pending
		svc := &Service{bookings: backend}

		// The post-transition read replays the same row — flip it to
		// confirmed so the returned DTO reflects the accepted state.
		backend.responds(func() {
			r := pending
			r.Status = servicedir.BookingStatusConfirmed
			r.ConfirmedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			backend.rows[connID.String()] = r
		})
		conn, err := svc.AcceptConnection(ctx, mentorID, connID)
		if err != nil {
			t.Fatalf("AcceptConnection: %v", err)
		}
		if conn.Status != "active" {
			t.Errorf("status = %q, want active", conn.Status)
		}
		if len(backend.responded) != 1 || backend.responded[0] != "accept" {
			t.Errorf("responded = %v, want [accept]", backend.responded)
		}
	})
}

func TestCancelOutgoingConnectionGuard(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	menteeID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	connID := mustUUID(t, "44444444-4444-4444-4444-444444444444")

	backend := newFakeBackend()
	backend.rows[connID.String()] = servicedir.BookingRow{
		ID:           connID,
		BookerUserID: menteeID,
		Status:       servicedir.BookingStatusConfirmed, // already active
		ListingKind:  servicedir.ListingKindMentorship,
	}
	svc := &Service{bookings: backend}

	if err := svc.CancelOutgoingConnection(ctx, menteeID, connID); !errors.Is(err, ErrConnectionNotFound) {
		t.Errorf("active cancel via outgoing = %v, want ErrConnectionNotFound", err)
	}
}

// responds installs a hook run after RespondToBooking is called — the
// fake flips stored state so the compat layer's re-read sees the
// transitioned row.
func (f *fakeBookingBackend) responds(fn func()) { f.afterRespond = fn }
