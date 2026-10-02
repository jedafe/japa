// Package servicedir — store_packages.go holds the data-access methods for
// §111 package items, departures, and the unified service_bookings
// pipeline. All queries are sqlc-generated
// (internal/db/queries/services.sql).
//
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
//
//	japaconnect-engineering/adr/ADR-123-service-packages.md
package servicedir

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Package items (replace-on-save)
// ---------------------------------------------------------------------------

// ReplacePackageItems swaps the full item set for a package inside one
// transaction (§111: items[] is replace-on-save on create/edit).
func (s *Store) ReplacePackageItems(ctx context.Context, packageMetaID pgtype.UUID, items []gen.CreateServicePackageItemParams) error {
	return db.WithTx(ctx, s.pool.Writer, func(q *gen.Queries) error {
		if err := q.DeleteServicePackageItems(ctx, packageMetaID); err != nil {
			return fmt.Errorf("servicedir: delete package items: %w", err)
		}

		for i := range items {
			items[i].PackageMetaID = packageMetaID
			if _, err := q.CreateServicePackageItem(ctx, items[i]); err != nil {
				return fmt.Errorf("servicedir: create package item %d: %w", i, err)
			}
		}

		return nil
	})
}

// ListPackageItems returns the ordered line items for a package.
func (s *Store) ListPackageItems(ctx context.Context, packageMetaID pgtype.UUID) ([]gen.ServicePackageItem, error) {
	rows, err := s.read.ListServicePackageItems(ctx, packageMetaID)
	if err != nil {
		return nil, fmt.Errorf("servicedir: list package items: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Package departures (replace-on-save; booked departures are preserved)
// ---------------------------------------------------------------------------

// ReplacePackageDepartures swaps the departure set for a package inside
// one transaction. Departures that already carry bookings are preserved —
// deleting them would SET NULL departure_id on service_bookings and lose
// the slot association, so the caller passes the ids returned by
// DeparturesWithBookings to keep.
func (s *Store) ReplacePackageDepartures(ctx context.Context, packageMetaID pgtype.UUID, keepIDs []pgtype.UUID, deps []gen.CreateServicePackageDepartureParams) error {
	if keepIDs == nil {
		keepIDs = []pgtype.UUID{}
	}

	return db.WithTx(ctx, s.pool.Writer, func(q *gen.Queries) error {
		if err := q.DeleteServicePackageDeparturesExcept(ctx, gen.DeleteServicePackageDeparturesExceptParams{
			PackageMetaID: packageMetaID,
			KeepIds:       keepIDs,
		}); err != nil {
			return fmt.Errorf("servicedir: delete package departures: %w", err)
		}

		for i := range deps {
			deps[i].PackageMetaID = packageMetaID
			if _, err := q.CreateServicePackageDeparture(ctx, deps[i]); err != nil {
				return fmt.Errorf("servicedir: create package departure %d: %w", i, err)
			}
		}

		return nil
	})
}

// ListPackageDepartures returns all departures for a package (edit view).
func (s *Store) ListPackageDepartures(ctx context.Context, packageMetaID pgtype.UUID) ([]gen.ServicePackageDeparture, error) {
	rows, err := s.read.ListServicePackageDepartures(ctx, packageMetaID)
	if err != nil {
		return nil, fmt.Errorf("servicedir: list package departures: %w", err)
	}

	return rows, nil
}

// ListOpenPackageDepartures returns open, not-yet-started departures for
// the public detail page.
func (s *Store) ListOpenPackageDepartures(ctx context.Context, packageMetaID pgtype.UUID) ([]gen.ServicePackageDeparture, error) {
	rows, err := s.read.ListOpenServicePackageDepartures(ctx, packageMetaID)
	if err != nil {
		return nil, fmt.Errorf("servicedir: list open package departures: %w", err)
	}

	return rows, nil
}

// GetDeparture returns a single departure row.
func (s *Store) GetDeparture(ctx context.Context, id pgtype.UUID) (gen.ServicePackageDeparture, error) {
	row, err := s.read.GetServicePackageDeparture(ctx, id)
	if err != nil {
		return gen.ServicePackageDeparture{}, fmt.Errorf("servicedir: get departure: %w", err)
	}

	return row, nil
}

// DeparturesWithBookings returns ids of departures that already carry
// bookings — the service layer keeps these when replacing the set.
func (s *Store) DeparturesWithBookings(ctx context.Context, packageMetaID pgtype.UUID) ([]pgtype.UUID, error) {
	ids, err := s.read.ListDeparturesWithBookings(ctx, packageMetaID)
	if err != nil {
		return nil, fmt.Errorf("servicedir: list departures with bookings: %w", err)
	}

	return ids, nil
}

// ---------------------------------------------------------------------------
// Booking domain rows
// ---------------------------------------------------------------------------

// BookingRow is the joined booking detail (booking fields + listing title/
// slug/kind + org display + departure dates + booker display fields).
// sqlc expands sb.* into flat columns, so the row is flat too.
type BookingRow struct {
	ID              pgtype.UUID
	ListingMetaID   pgtype.UUID
	DepartureID     pgtype.UUID
	BookerUserID    pgtype.UUID
	OrgID           pgtype.UUID
	Intent          string
	PartySize       int32
	PreferredDates  []byte
	Note            *string
	Status          string
	PurchaseMode    string
	PaymentID       pgtype.UUID
	TotalPrice      *int64
	FeeAmount       *int64
	FeeCurrency     *string
	AgentBalanceDue *int64
	OrgResponseNote *string
	DisputeReason   *string
	ExpiresAt       pgtype.Timestamptz
	ConfirmedAt     pgtype.Timestamptz
	FulfilledAt     pgtype.Timestamptz
	CancelledAt     pgtype.Timestamptz
	CancelReason    *string
	AnonymizedAt    pgtype.Timestamptz
	CreatedAt       pgtype.Timestamptz
	UpdatedAt       pgtype.Timestamptz
	ListingTitle    string
	ListingSlug     string
	ListingKind     string
	// ListingContentID is the contents.id behind the listing — the
	// jc:svc_package cache key (only populated by the detail query).
	ListingContentID pgtype.UUID
	// PackageCapacity is the package-level capacity a NULL departure
	// capacity inherits (only populated by the detail query).
	PackageCapacity   *int32
	PaymentStatus     *string // joined payment row — nil when payment_id is unset
	OrgName           *string
	OrgSlug           *string
	DepartureStartsOn pgtype.Date
	DepartureEndsOn   pgtype.Date
	BookerUsername    *string
	BookerAvatar      *string
}

// bookingFromGen projects a gen.ServiceBooking into the flat row shape.
func bookingFromGen(b gen.ServiceBooking) BookingRow {
	return BookingRow{
		ID: b.ID, ListingMetaID: b.ListingMetaID, DepartureID: b.DepartureID,
		BookerUserID: b.BookerUserID, OrgID: b.OrgID, Intent: b.Intent,
		PartySize: b.PartySize, PreferredDates: b.PreferredDates, Note: b.Note,
		Status: b.Status, PurchaseMode: b.PurchaseMode, PaymentID: b.PaymentID,
		TotalPrice: b.TotalPrice, FeeAmount: b.FeeAmount, FeeCurrency: b.FeeCurrency,
		AgentBalanceDue: b.AgentBalanceDue, OrgResponseNote: b.OrgResponseNote,
		DisputeReason: b.DisputeReason,
		ExpiresAt:     b.ExpiresAt, ConfirmedAt: b.ConfirmedAt, FulfilledAt: b.FulfilledAt,
		CancelledAt: b.CancelledAt, CancelReason: b.CancelReason,
		AnonymizedAt: b.AnonymizedAt, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
}

func bookingRowFromDetail(r gen.GetServiceBookingDetailRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason:   r.DisputeReason,
		ExpiresAt: r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.PackageCapacity = r.PackageCapacity
	row.PaymentStatus = r.PaymentStatus
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn

	return row
}

func bookingRowFromUser(r gen.ListServiceBookingsByUserRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason:   r.DisputeReason,
		ExpiresAt: r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn

	return row
}

func bookingRowFromOrg(r gen.ListServiceBookingsByOrgRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason:   r.DisputeReason,
		ExpiresAt: r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn
	row.BookerUsername = r.BookerUsername
	row.BookerAvatar = r.BookerAvatar

	return row
}

func bookingRowFromAdmin(r gen.AdminListServiceBookingsRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason:   r.DisputeReason,
		ExpiresAt: r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn
	row.BookerUsername = r.BookerUsername

	return row
}

// ---------------------------------------------------------------------------
// Bookings
// ---------------------------------------------------------------------------

// CreateBooking inserts a service_bookings row.
func (s *Store) CreateBooking(ctx context.Context, p gen.CreateServiceBookingParams) (gen.ServiceBooking, error) {
	row, err := s.write.CreateServiceBooking(ctx, p)
	if err != nil {
		return gen.ServiceBooking{}, fmt.Errorf("servicedir: create booking: %w", err)
	}

	return row, nil
}

// GetBookingDetail returns the joined booking detail by id.
func (s *Store) GetBookingDetail(ctx context.Context, id pgtype.UUID) (BookingRow, error) {
	row, err := s.read.GetServiceBookingDetail(ctx, id)
	if err != nil {
		return BookingRow{}, fmt.Errorf("servicedir: get booking detail: %w", err)
	}

	return bookingRowFromDetail(row), nil
}

// AttachBookingPayment links the platform-fee payment to a booking.
func (s *Store) AttachBookingPayment(ctx context.Context, bookingID, paymentID pgtype.UUID) error {
	_, err := s.write.AttachServiceBookingPayment(ctx, gen.AttachServiceBookingPaymentParams{
		ID: bookingID, PaymentID: paymentID,
	})
	if err != nil {
		return fmt.Errorf("servicedir: attach booking payment: %w", err)
	}

	return nil
}

// BookingsByUserFilter holds filters for the caller's booking history.
type BookingsByUserFilter struct {
	Status string
	Intent string
	Limit  int32
	Offset int32
}

// ListBookingsByUser returns the caller's bookings + inquiries.
func (s *Store) ListBookingsByUser(ctx context.Context, userID pgtype.UUID, f BookingsByUserFilter) ([]BookingRow, int64, error) {
	params := gen.ListServiceBookingsByUserParams{
		BookerUserID: userID, PageLimit: f.Limit, PageOffset: f.Offset,
	}
	if f.Status != "" {
		params.Status = &f.Status
	}

	if f.Intent != "" {
		params.Intent = &f.Intent
	}

	rows, err := s.read.ListServiceBookingsByUser(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: list bookings by user: %w", err)
	}

	total, err := s.read.CountServiceBookingsByUser(ctx, gen.CountServiceBookingsByUserParams{
		BookerUserID: userID, Status: params.Status, Intent: params.Intent,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: count bookings by user: %w", err)
	}

	out := make([]BookingRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, bookingRowFromUser(r))
	}

	return out, total, nil
}

// BookingsByOrgFilter holds filters for the org bookings inbox.
type BookingsByOrgFilter struct {
	Status           string
	Intent           string
	ListingContentID pgtype.UUID // optional ?listing_id= — the listing's public content id
	Limit            int32
	Offset           int32
}

// ListBookingsByOrg returns the org's bookings inbox. Paid bookings only
// appear once money moved (payment succeeded or refunded — enforced in
// the query).
func (s *Store) ListBookingsByOrg(ctx context.Context, orgID pgtype.UUID, f BookingsByOrgFilter) ([]BookingRow, int64, error) {
	params := gen.ListServiceBookingsByOrgParams{
		OrgID: orgID, PageLimit: f.Limit, PageOffset: f.Offset,
		ListingContentID: f.ListingContentID,
	}
	if f.Status != "" {
		params.Status = &f.Status
	}

	if f.Intent != "" {
		params.Intent = &f.Intent
	}

	rows, err := s.read.ListServiceBookingsByOrg(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: list bookings by org: %w", err)
	}

	total, err := s.read.CountServiceBookingsByOrg(ctx, gen.CountServiceBookingsByOrgParams{
		OrgID: orgID, Status: params.Status, Intent: params.Intent,
		ListingContentID: f.ListingContentID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: count bookings by org: %w", err)
	}

	out := make([]BookingRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, bookingRowFromOrg(r))
	}

	return out, total, nil
}

// BookingsAdminFilter holds filters for the admin bookings view.
type BookingsAdminFilter struct {
	Status string
	Intent string
	OrgID  pgtype.UUID
	Limit  int32
	Offset int32
}

// AdminListBookings returns all bookings (any payment state) for admins.
func (s *Store) AdminListBookings(ctx context.Context, f BookingsAdminFilter) ([]BookingRow, int64, error) {
	params := gen.AdminListServiceBookingsParams{
		OrgID: f.OrgID, PageLimit: f.Limit, PageOffset: f.Offset,
	}
	if f.Status != "" {
		params.Status = &f.Status
	}

	if f.Intent != "" {
		params.Intent = &f.Intent
	}

	rows, err := s.read.AdminListServiceBookings(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin list bookings: %w", err)
	}

	total, err := s.read.AdminCountServiceBookings(ctx, gen.AdminCountServiceBookingsParams{
		Status: params.Status, Intent: params.Intent, OrgID: f.OrgID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin count bookings: %w", err)
	}

	out := make([]BookingRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, bookingRowFromAdmin(r))
	}

	return out, total, nil
}

// ---------------------------------------------------------------------------
// Booking state transitions — each generated query guards its legal source
// status; pgx.ErrNoRows maps to ErrInvalidBookingTransition in the service
// layer.
// ---------------------------------------------------------------------------

// ConfirmBooking atomically transitions pending → confirmed. When the
// booking carries a departure_id the capacity guard + booked_count
// increment run in the same transaction (§111).
func (s *Store) ConfirmBooking(ctx context.Context, bookingID, departureID pgtype.UUID, partySize int32, note *string) (gen.ServiceBooking, error) {
	if !departureID.Valid {
		return s.write.ConfirmServiceBooking(ctx, gen.ConfirmServiceBookingParams{
			ID: bookingID, OrgResponseNote: note,
		})
	}

	return db.WithTxResult(ctx, s.pool.Writer, func(q *gen.Queries) (gen.ServiceBooking, error) {
		// Capacity check + increment + 'full' flip in one guarded update.
		if _, err := q.IncrementDepartureBookedCount(ctx, gen.IncrementDepartureBookedCountParams{
			ID: departureID, PartySize: partySize,
		}); err != nil {
			return gen.ServiceBooking{}, err // pgx.ErrNoRows → departure_full upstream
		}

		return q.ConfirmServiceBooking(ctx, gen.ConfirmServiceBookingParams{
			ID: bookingID, OrgResponseNote: note,
		})
	})
}

// DeclineBooking transitions pending → declined.
func (s *Store) DeclineBooking(ctx context.Context, id pgtype.UUID, note *string) (gen.ServiceBooking, error) {
	return s.write.DeclineServiceBooking(ctx, gen.DeclineServiceBookingParams{
		ID: id, OrgResponseNote: note,
	})
}

// FulfillBooking transitions confirmed → fulfilled.
func (s *Store) FulfillBooking(ctx context.Context, id pgtype.UUID) (gen.ServiceBooking, error) {
	return s.write.FulfillServiceBooking(ctx, id)
}

// CancelBooking transitions pending|confirmed → cancelled (booker).
func (s *Store) CancelBooking(ctx context.Context, id pgtype.UUID, reason *string) (gen.ServiceBooking, error) {
	return s.write.CancelServiceBooking(ctx, gen.CancelServiceBookingParams{
		ID: id, CancelReason: reason,
	})
}

// CancelBookingAndRelease is CancelBooking plus a booked_count decrement
// in the same transaction — the status change can't commit without
// freeing the seats. Pass an invalid departureID to skip the decrement.
func (s *Store) CancelBookingAndRelease(ctx context.Context, id, departureID pgtype.UUID, partySize int32, reason *string) (gen.ServiceBooking, error) {
	if !departureID.Valid {
		return s.CancelBooking(ctx, id, reason)
	}

	return db.WithTxResult(ctx, s.pool.Writer, func(q *gen.Queries) (gen.ServiceBooking, error) {
		b, err := q.CancelServiceBooking(ctx, gen.CancelServiceBookingParams{
			ID: id, CancelReason: reason,
		})
		if err != nil {
			return b, err
		}

		if _, err := q.DecrementDepartureBookedCount(ctx, gen.DecrementDepartureBookedCountParams{
			ID: departureID, PartySize: partySize,
		}); err != nil {
			return gen.ServiceBooking{}, err
		}

		return b, nil
	})
}

// OrgCancelBooking transitions pending|confirmed → cancelled (org).
func (s *Store) OrgCancelBooking(ctx context.Context, id pgtype.UUID, note, reason *string) (gen.ServiceBooking, error) {
	return s.write.OrgCancelServiceBooking(ctx, gen.OrgCancelServiceBookingParams{
		ID: id, OrgResponseNote: note, CancelReason: reason,
	})
}

// OrgCancelBookingAndRelease is OrgCancelBooking plus a booked_count
// decrement in the same transaction.
func (s *Store) OrgCancelBookingAndRelease(ctx context.Context, id, departureID pgtype.UUID, partySize int32, note, reason *string) (gen.ServiceBooking, error) {
	if !departureID.Valid {
		return s.OrgCancelBooking(ctx, id, note, reason)
	}

	return db.WithTxResult(ctx, s.pool.Writer, func(q *gen.Queries) (gen.ServiceBooking, error) {
		b, err := q.OrgCancelServiceBooking(ctx, gen.OrgCancelServiceBookingParams{
			ID: id, OrgResponseNote: note, CancelReason: reason,
		})
		if err != nil {
			return b, err
		}

		if _, err := q.DecrementDepartureBookedCount(ctx, gen.DecrementDepartureBookedCountParams{
			ID: departureID, PartySize: partySize,
		}); err != nil {
			return gen.ServiceBooking{}, err
		}

		return b, nil
	})
}

// AdminSetBookingStatus is the admin dispute-resolution transition — any
// target status, optional resolution note (recorded in org_response_note).
func (s *Store) AdminSetBookingStatus(ctx context.Context, id pgtype.UUID, status string, note *string) (gen.ServiceBooking, error) {
	return s.write.AdminSetServiceBookingStatus(ctx, gen.AdminSetServiceBookingStatusParams{
		ID: id, Status: status, OrgResponseNote: note,
	})
}

// AdminSetStatusAndRelease is AdminSetBookingStatus plus a booked_count
// decrement in the same transaction — used when the transition leaves
// 'confirmed'. Pass an invalid departureID to skip the decrement.
func (s *Store) AdminSetStatusAndRelease(ctx context.Context, id pgtype.UUID, status string, note *string, departureID pgtype.UUID, partySize int32) (gen.ServiceBooking, error) {
	if !departureID.Valid {
		return s.AdminSetBookingStatus(ctx, id, status, note)
	}

	return db.WithTxResult(ctx, s.pool.Writer, func(q *gen.Queries) (gen.ServiceBooking, error) {
		b, err := q.AdminSetServiceBookingStatus(ctx, gen.AdminSetServiceBookingStatusParams{
			ID: id, Status: status, OrgResponseNote: note,
		})
		if err != nil {
			return b, err
		}

		if _, err := q.DecrementDepartureBookedCount(ctx, gen.DecrementDepartureBookedCountParams{
			ID: departureID, PartySize: partySize,
		}); err != nil {
			return gen.ServiceBooking{}, err
		}

		return b, nil
	})
}

// AdminSetStatusAcquire force-transitions a booking into a seat-holding
// status (§111: confirmed|fulfilled hold seats) while acquiring departure
// seats under the same capacity guard as ConfirmBooking — the increment
// and the status write share one transaction so a failed transition
// can't leak seats.
func (s *Store) AdminSetStatusAcquire(ctx context.Context, bookingID, departureID pgtype.UUID, partySize int32, status string, note *string) (gen.ServiceBooking, error) {
	if !departureID.Valid {
		return s.AdminSetBookingStatus(ctx, bookingID, status, note)
	}

	return db.WithTxResult(ctx, s.pool.Writer, func(q *gen.Queries) (gen.ServiceBooking, error) {
		if _, err := q.IncrementDepartureBookedCount(ctx, gen.IncrementDepartureBookedCountParams{
			ID: departureID, PartySize: partySize,
		}); err != nil {
			return gen.ServiceBooking{}, err // pgx.ErrNoRows → departure_full upstream
		}

		return q.AdminSetServiceBookingStatus(ctx, gen.AdminSetServiceBookingStatusParams{
			ID: bookingID, Status: status, OrgResponseNote: note,
		})
	})
}

// DisputeBookingAndRelease transitions confirmed|fulfilled → disputed and
// releases the departure seat in the same transaction — both source
// statuses hold seats and 'disputed' holds none (§111 seat model). Pass
// an invalid departureID to skip the decrement.
func (s *Store) DisputeBookingAndRelease(ctx context.Context, id, departureID pgtype.UUID, partySize int32, reason *string) (gen.ServiceBooking, error) {
	if !departureID.Valid {
		return s.write.DisputeServiceBooking(ctx, gen.DisputeServiceBookingParams{
			ID: id, DisputeReason: reason,
		})
	}

	return db.WithTxResult(ctx, s.pool.Writer, func(q *gen.Queries) (gen.ServiceBooking, error) {
		b, err := q.DisputeServiceBooking(ctx, gen.DisputeServiceBookingParams{
			ID: id, DisputeReason: reason,
		})
		if err != nil {
			return b, err
		}

		if _, err := q.DecrementDepartureBookedCount(ctx, gen.DecrementDepartureBookedCountParams{
			ID: departureID, PartySize: partySize,
		}); err != nil {
			return gen.ServiceBooking{}, err
		}

		return b, nil
	})
}
