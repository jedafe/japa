// service_bookings.go — the §111 unified inquiry/booking pipeline.
// One table (service_bookings), one state machine, two intents:
//
//	intent='inquiry' — lead capture for purchase_mode='inquiry' listings.
//	    No payment; the org sees it in the bookings inbox and responds via
//	    the org_response_note / DM.
//	intent='booking' — reservation on a purchase_mode='paid' package. The
//	    booker pays the platform fee on-platform (§17,
//	    purpose_type='service_booking_fee'); the agent collects
//	    agent_balance_due directly. Fee settles on 'fulfilled', refunds on
//	    declined/expired/org-cancelled/disputed-resolved-for-booker, and is
//	    forfeited on late booker-cancel (§111 refund policy).
//
// purchase_mode='external' listings don't accept bookings — they link out
// via booking_url.
//
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
//
//	japaconnect-engineering/adr/ADR-123-service-packages.md
package servicedir

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	mw "github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/worker"
)

// BookingDTO is the API response shape for one service_bookings row.
type BookingDTO struct {
	ID              string               `json:"id"`
	ListingMetaID   string               `json:"listing_meta_id"`
	ListingID       string               `json:"listing_id,omitempty"` // the listing's public content id — the ?listing_id= filter key
	ListingTitle    string               `json:"listing_title"`
	ListingSlug     string               `json:"listing_slug"`
	ListingKind     string               `json:"listing_kind"`
	DepartureID     string               `json:"departure_id,omitempty"`
	DepartureStart  string               `json:"departure_starts_on,omitempty"`
	DepartureEnd    string               `json:"departure_ends_on,omitempty"`
	BookerUserID    string               `json:"booker_user_id,omitempty"`
	BookerUsername  string               `json:"booker_username,omitempty"`
	BookerAvatar    string               `json:"booker_avatar,omitempty"`
	OrgID           string               `json:"org_id"`
	OrgName         string               `json:"org_name,omitempty"`
	OrgSlug         string               `json:"org_slug,omitempty"`
	Intent          string               `json:"intent"`
	PartySize       int32                `json:"party_size"`
	PreferredDates  *PreferredDatesInput `json:"preferred_dates,omitempty"`
	Note            string               `json:"note,omitempty"`
	Status          string               `json:"status"`
	PurchaseMode    string               `json:"purchase_mode"`
	PaymentID       string               `json:"payment_id,omitempty"`
	TotalPrice      *int64               `json:"total_price,omitempty"`
	FeeAmount       *int64               `json:"fee_amount,omitempty"`
	FeeCurrency     string               `json:"fee_currency,omitempty"`
	AgentBalanceDue *int64               `json:"agent_balance_due,omitempty"`
	OrgResponseNote string               `json:"org_response_note,omitempty"`
	ExpiresAt       string               `json:"expires_at,omitempty"`
	ConfirmedAt     string               `json:"confirmed_at,omitempty"`
	FulfilledAt     string               `json:"fulfilled_at,omitempty"`
	CancelledAt     string               `json:"cancelled_at,omitempty"`
	CancelReason    string               `json:"cancel_reason,omitempty"`
	CreatedAt       string               `json:"created_at"`
}

// BookingResult is the POST /services/:id/bookings response — the booking
// plus a nested payment object when a platform-fee checkout was opened
// (§111: intent='booking' returns { booking, payment: {checkout_url} }).
type BookingResult struct {
	Booking BookingDTO      `json:"booking"`
	Payment *BookingPayment `json:"payment,omitempty"`
}

// BookingPayment carries the §17 fee-checkout details back to the client.
type BookingPayment struct {
	ID          string `json:"id"`
	CheckoutURL string `json:"checkout_url"`
}

// PreferredDatesInput is the flexible-date object for non-departure
// package bookings (§111 schema: {start_on, end_on, flexible}).
type PreferredDatesInput struct {
	StartOn  string `json:"start_on"`
	EndOn    string `json:"end_on"`
	Flexible bool   `json:"flexible"`
}

// CreateBookingInput is the request body for POST /services/:id/bookings.
// provider/email/return_url are required when intent='booking' on a
// purchase_mode='paid' listing (the fee checkout fields).
type CreateBookingInput struct {
	Intent         string               `json:"intent"` // inquiry | booking (default derived from purchase_mode)
	DepartureID    string               `json:"departure_id"`
	PartySize      int32                `json:"party_size"`
	PreferredDates *PreferredDatesInput `json:"preferred_dates"`
	Note           string               `json:"note"`
	Provider       string               `json:"provider"`
	Email          string               `json:"email"`
	ReturnURL      string               `json:"return_url"`
}

// OrgBookingActionInput is the PATCH /org/:org_id/service-bookings/:id body.
type OrgBookingActionInput struct {
	Status          string `json:"status"` // confirmed | declined | fulfilled | cancelled
	OrgResponseNote string `json:"org_response_note"`
	CancelReason    string `json:"cancel_reason"`
}

// AdminBookingActionInput is the PATCH /admin/service-bookings/:id body
// (dispute resolution).
type AdminBookingActionInput struct {
	Status         string `json:"status"`
	ResolutionNote string `json:"resolution_note"`
}

// bookingToDTO maps the joined store row to the API shape.
func bookingToDTO(r BookingRow) BookingDTO {
	dto := BookingDTO{
		ID:              r.ID.String(),
		ListingMetaID:   r.ListingMetaID.String(),
		ListingTitle:    r.ListingTitle,
		ListingSlug:     r.ListingSlug,
		ListingKind:     r.ListingKind,
		Intent:          r.Intent,
		PartySize:       r.PartySize,
		Status:          r.Status,
		PurchaseMode:    r.PurchaseMode,
		TotalPrice:      r.TotalPrice,
		FeeAmount:       r.FeeAmount,
		AgentBalanceDue: r.AgentBalanceDue,
		ExpiresAt:       formatTs(r.ExpiresAt),
		ConfirmedAt:     formatTs(r.ConfirmedAt),
		FulfilledAt:     formatTs(r.FulfilledAt),
		CancelledAt:     formatTs(r.CancelledAt),
		CreatedAt:       formatTs(r.CreatedAt),
	}

	if r.ListingContentID.Valid {
		dto.ListingID = r.ListingContentID.String()
	}

	if r.DepartureID.Valid {
		dto.DepartureID = r.DepartureID.String()
	}

	if r.DepartureStartsOn.Valid {
		dto.DepartureStart = r.DepartureStartsOn.Time.Format("2006-01-02")
	}

	if r.DepartureEndsOn.Valid {
		dto.DepartureEnd = r.DepartureEndsOn.Time.Format("2006-01-02")
	}

	// GDPR-erased bookings keep the financial row but drop the booker.
	if r.BookerUserID.Valid {
		dto.BookerUserID = r.BookerUserID.String()
	}

	if r.BookerUsername != nil {
		dto.BookerUsername = *r.BookerUsername
	}

	if r.BookerAvatar != nil {
		dto.BookerAvatar = *r.BookerAvatar
	}

	if r.OrgID.Valid {
		dto.OrgID = r.OrgID.String()
	}

	if r.OrgName != nil {
		dto.OrgName = *r.OrgName
	}

	if r.OrgSlug != nil {
		dto.OrgSlug = *r.OrgSlug
	}

	if r.Note != nil {
		dto.Note = *r.Note
	}

	if r.PaymentID.Valid {
		dto.PaymentID = r.PaymentID.String()
	}

	if r.FeeCurrency != nil {
		dto.FeeCurrency = *r.FeeCurrency
	}

	if r.OrgResponseNote != nil {
		dto.OrgResponseNote = *r.OrgResponseNote
	}

	if r.CancelReason != nil {
		dto.CancelReason = *r.CancelReason
	}

	if len(r.PreferredDates) > 0 {
		var dates PreferredDatesInput
		if err := json.Unmarshal(r.PreferredDates, &dates); err == nil {
			dto.PreferredDates = &dates
		}
	}

	return dto
}

// validatePreferredDates enforces the §111 {start_on, end_on, flexible}
// shape — both dates optional but well-formed, and start_on <= end_on.
func validatePreferredDates(d *PreferredDatesInput) error {
	var start, end time.Time

	if d.StartOn != "" {
		t, err := time.Parse("2006-01-02", d.StartOn)
		if err != nil {
			return ErrInvalidPreferredDates
		}
		start = t
	}

	if d.EndOn != "" {
		t, err := time.Parse("2006-01-02", d.EndOn)
		if err != nil {
			return ErrInvalidPreferredDates
		}
		end = t
	}

	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return ErrInvalidPreferredDates
	}

	return nil
}

// computeFee calculates the §111 platform fee for a package price (both
// minor units). fee_type='percent' → price × fee_pct / 100; 'fixed' →
// fee_fixed. The fee never exceeds the package price.
func (s *Service) computeFee(ctx context.Context, priceAmount int64) int64 {
	if s.settings.GetStringOr(ctx, "service_packages.fee_type", "percent") == "fixed" {
		fee := int64(s.settings.GetIntOr(ctx, "service_packages.fee_fixed", 0))
		if fee > priceAmount {
			return priceAmount
		}

		return fee
	}

	pct := int64(s.settings.GetIntOr(ctx, "service_packages.fee_pct", 10))

	fee := priceAmount * pct / 100
	if fee > priceAmount {
		return priceAmount
	}

	return fee
}

// bookingExpiresAt computes the pending SLA deadline.
func (s *Service) bookingExpiresAt(ctx context.Context) pgtype.Timestamptz {
	sla := s.settings.GetIntOr(ctx, "service_packages.org_response_sla_hours", 72)

	return pgtype.Timestamptz{Time: time.Now().Add(time.Duration(sla) * time.Hour), Valid: true}
}

// CreateBooking creates a service inquiry or paid booking against a
// listing (POST /services/:id/bookings). For intent='booking' on a
// purchase_mode='paid' package it snapshots total/fee/balance and opens
// the §17 fee checkout; the org never sees paid bookings until the fee
// payment succeeds (query-level filter + notification deferred to the
// payment-success path).
func (s *Service) CreateBooking(ctx context.Context, bookerID pgtype.UUID, listingContentID pgtype.UUID, input CreateBookingInput) (BookingResult, error) {
	if !s.PackagesEnabled(ctx) {
		return BookingResult{}, ErrPackagesDisabled
	}

	listing, err := s.store.GetByContentID(ctx, listingContentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingResult{}, pgx.ErrNoRows
		}

		return BookingResult{}, fmt.Errorf("servicedir: booking: get listing: %w", err)
	}

	if listing.Status != content.StatusPublished {
		return BookingResult{}, ErrBookingUnavailable
	}

	// §111 availability window — a package that hasn't opened yet or is
	// past available_until doesn't accept bookings. Date columns compare
	// against the UTC civil date (pgtype.Date stores UTC midnight).
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if (listing.AvailableFrom.Valid && listing.AvailableFrom.Time.After(today)) ||
		(listing.AvailableUntil.Valid && listing.AvailableUntil.Time.Before(today)) {
		return BookingResult{}, ErrBookingUnavailable
	}

	// service_bookings.org_id is NOT NULL — the booking lands in the org
	// inbox, so a user-authored listing with no owning org has nowhere to
	// deliver a lead. Reject cleanly rather than failing on the insert.
	if !listing.OrgID.Valid {
		return BookingResult{}, ErrBookingUnavailable
	}

	// Resolve intent: explicit input wins; otherwise derive from the
	// listing's purchase mode.
	intent := input.Intent
	if intent == "" {
		if listing.PurchaseMode == PurchaseModePaid {
			intent = BookingIntentBooking
		} else {
			intent = BookingIntentInquiry
		}
	}

	if intent != BookingIntentInquiry && intent != BookingIntentBooking {
		return BookingResult{}, ErrInvalidIntent
	}

	switch listing.PurchaseMode {
	case PurchaseModeExternal:
		// External listings link out via booking_url — no on-platform bookings.
		return BookingResult{}, ErrBookingUnavailable
	case PurchaseModePaid:
		if intent != BookingIntentBooking {
			return BookingResult{}, ErrInvalidIntent
		}
	default: // inquiry
		if intent != BookingIntentInquiry {
			return BookingResult{}, ErrInvalidIntent
		}
	}

	// Rule 9 — fee checkout is a financial transaction: Idempotency-Key is
	// required when the resolved intent initiates a payment. Checked before
	// the booking row exists so a rejected request writes nothing.
	if intent == BookingIntentBooking && mw.IdempotencyKeyFromContext(ctx) == "" {
		return BookingResult{}, ErrIdempotencyRequired
	}

	partySize := input.PartySize
	if partySize <= 0 {
		partySize = 1
	}

	var departureID pgtype.UUID
	if input.DepartureID != "" {
		if err := departureID.Scan(input.DepartureID); err != nil {
			return BookingResult{}, ErrInvalidDeparture
		}

		dep, err := s.store.GetDeparture(ctx, departureID)
		if err != nil {
			return BookingResult{}, ErrInvalidDeparture
		}

		if dep.PackageMetaID != listing.MetaID || dep.Status != DepartureStatusOpen {
			return BookingResult{}, ErrInvalidDeparture
		}
	}

	var preferredDates []byte
	if input.PreferredDates != nil {
		if err := validatePreferredDates(input.PreferredDates); err != nil {
			return BookingResult{}, err
		}

		preferredDates, err = json.Marshal(input.PreferredDates)
		if err != nil {
			return BookingResult{}, fmt.Errorf("invalid preferred_dates: %w", err)
		}
	}

	// Checkout fields are validated before the booking row is written —
	// a rejected paid booking must not leave an orphaned pending row.
	if intent == BookingIntentBooking && (input.Provider == "" || input.Email == "") {
		return BookingResult{}, ErrCheckoutFieldsRequired
	}

	params := gen.CreateServiceBookingParams{
		ListingMetaID:  listing.MetaID,
		DepartureID:    departureID,
		BookerUserID:   bookerID,
		OrgID:          listing.OrgID,
		Intent:         intent,
		PartySize:      &partySize,
		PreferredDates: preferredDates,
		Status:         strPtr(BookingStatusPending),
		PurchaseMode:   listing.PurchaseMode,
		ExpiresAt:      s.bookingExpiresAt(ctx),
	}

	if input.Note != "" {
		params.Note = &input.Note
	}

	// §111 monetary snapshot for paid bookings — values frozen at booking
	// time so later price/fee-setting changes don't mutate open bookings.
	if intent == BookingIntentBooking {
		if listing.PriceAmount == nil || listing.PriceCurrency == nil {
			return BookingResult{}, ErrMissingPackagePrice
		}

		total := *listing.PriceAmount
		fee := s.computeFee(ctx, total)
		balance := total - fee

		params.TotalPrice = &total
		params.FeeAmount = &fee
		params.FeeCurrency = listing.PriceCurrency
		params.AgentBalanceDue = &balance
	}

	booking, err := s.store.CreateBooking(ctx, params)
	if err != nil {
		return BookingResult{}, err
	}

	result := BookingResult{Booking: bookingToDTO(bookingFromGen(booking))}

	if intent == BookingIntentBooking {
		// Open the §17 fee checkout. The booking row stays 'pending' — the
		// payment-success path (worker onPaymentSucceeded) attaches nothing
		// further; the org-side visibility + notification gate on the
		// payment status join.
		if s.payments == nil {
			return BookingResult{}, errors.New("payments service unavailable")
		}

		pay, err := s.payments.CreatePayment(ctx, bookerID, PaymentInput{
			Provider:    input.Provider,
			PurposeType: "service_booking_fee",
			PurposeID:   booking.ID.String(),
			Amount:      *params.FeeAmount,
			Currency:    *params.FeeCurrency,
			Email:       input.Email,
			ReturnURL:   input.ReturnURL,
			Metadata: map[string]interface{}{
				"booking_id":   booking.ID.String(),
				"listing_slug": listing.Slug,
			},
		})
		if err != nil {
			// Don't leave a dangling paid-mode pending booking with no
			// checkout — cancel it immediately.
			reason := "payment initiation failed"
			if _, cerr := s.store.CancelBooking(ctx, booking.ID, &reason); cerr != nil {
				slog.Error("servicedir: failed to cancel booking after payment init failure",
					"booking_id", booking.ID, "error", cerr)
			}

			return BookingResult{}, fmt.Errorf("servicedir: create fee payment: %w", err)
		}

		var paymentID pgtype.UUID
		if err := paymentID.Scan(pay.PaymentID); err == nil {
			if err := s.store.AttachBookingPayment(ctx, booking.ID, paymentID); err != nil {
				slog.Error("servicedir: attach booking payment failed",
					"booking_id", booking.ID, "payment_id", pay.PaymentID, "error", err)
			}
		}

		result.Payment = &BookingPayment{ID: pay.PaymentID, CheckoutURL: pay.CheckoutURL}
		result.Booking.PaymentID = pay.PaymentID
	} else {
		// Inquiries notify the org immediately — no payment gate.
		s.enqueueBookingCreated(booking.ID)
	}

	return result, nil
}

// GetBooking returns the booking detail for the booker, a member of the
// owning org, or an admin (checked at route level for admin).
func (s *Service) GetBooking(ctx context.Context, bookingID, viewerID pgtype.UUID, isAdmin bool) (BookingDTO, error) {
	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: get booking: %w", err)
	}

	if !isAdmin && row.BookerUserID != viewerID {
		isMember, err := s.store.IsOrgMember(ctx, row.OrgID, viewerID)
		if err != nil {
			return BookingDTO{}, fmt.Errorf("servicedir: booking: org membership check: %w", err)
		}

		if !isMember {
			return BookingDTO{}, ErrNotOwner
		}

		// Org-side visibility gates paid bookings on money moved — same
		// rule as the inbox query (succeeded|refunded). A pending/failed
		// fee payment means the booking never landed in the org's queue.
		if row.Intent == BookingIntentBooking && !paymentSettled(row.PaymentStatus) {
			return BookingDTO{}, pgx.ErrNoRows
		}
	}

	return bookingToDTO(row), nil
}

// ListMyBookings returns the caller's bookings + inquiries.
func (s *Service) ListMyBookings(ctx context.Context, userID pgtype.UUID, status, intent string, page, limit int32) ([]BookingDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	rows, total, err := s.store.ListBookingsByUser(ctx, userID, BookingsByUserFilter{
		Status: status, Intent: intent, Limit: limit, Offset: (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, err
	}

	out := make([]BookingDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, bookingToDTO(r))
	}

	return out, total, nil
}

// ListOrgBookings returns the org's bookings inbox. The caller must be an
// org member (service.booking.manage also checked at route level).
func (s *Service) ListOrgBookings(ctx context.Context, orgID, userID pgtype.UUID, status, intent, listingID string, page, limit int32) ([]BookingDTO, int64, error) {
	isMember, err := s.store.IsOrgMember(ctx, orgID, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: org bookings: membership check: %w", err)
	}

	if !isMember {
		return nil, 0, ErrNotOrgMember
	}

	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	var listingContentID pgtype.UUID
	if listingID != "" {
		if err := listingContentID.Scan(listingID); err != nil {
			return nil, 0, ErrInvalidID
		}
	}

	rows, total, err := s.store.ListBookingsByOrg(ctx, orgID, BookingsByOrgFilter{
		Status: status, Intent: intent, ListingContentID: listingContentID,
		Limit: limit, Offset: (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, err
	}

	out := make([]BookingDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, bookingToDTO(r))
	}

	return out, total, nil
}

// ListOrgPackages returns the org's package listings. Anonymous callers
// and non-members see published packages only; org members see every
// status — the dashboard view includes drafts/in_review (§111).
func (s *Service) ListOrgPackages(ctx context.Context, orgID, viewerID pgtype.UUID, page, limit int32) ([]ServiceDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	status := content.StatusPublished

	if viewerID.Valid {
		if member, err := s.store.IsOrgMember(ctx, orgID, viewerID); err == nil && member {
			status = "" // member: all statuses
		}
	}

	rows, err := s.store.ListByOrg(ctx, orgID, status, ListingKindPackage, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}

	out := make([]ServiceDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToDTO(r))
	}

	return out, int64(len(out)), nil
}

// OrgTransition applies the org-facing status transition
// (confirmed|declined|fulfilled|cancelled) with org-membership and
// state-machine guards. Side effects (notifications, fee settlement,
// seat release) are enqueued after the row commits.
func (s *Service) OrgTransition(ctx context.Context, orgID, bookingID, userID pgtype.UUID, input OrgBookingActionInput) (BookingDTO, error) {
	if !s.PackagesEnabled(ctx) {
		return BookingDTO{}, ErrPackagesDisabled
	}

	isMember, err := s.store.IsOrgMember(ctx, orgID, userID)
	if err != nil {
		return BookingDTO{}, fmt.Errorf("servicedir: org transition: membership check: %w", err)
	}

	if !isMember {
		return BookingDTO{}, ErrNotOrgMember
	}

	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: org transition: get booking: %w", err)
	}

	if row.OrgID != orgID {
		return BookingDTO{}, ErrNotOwner
	}

	// Paid bookings only become actionable once money moved — the inbox
	// hides them before then, but a PATCH by ID would otherwise let the
	// org confirm a booking whose fee was never captured.
	if row.Intent == BookingIntentBooking && !paymentSettled(row.PaymentStatus) {
		return BookingDTO{}, pgx.ErrNoRows
	}

	var note *string
	if input.OrgResponseNote != "" {
		note = &input.OrgResponseNote
	}

	switch input.Status {
	case BookingStatusConfirmed:
		_, err = s.store.ConfirmBooking(ctx, bookingID, row.DepartureID, row.PartySize, note)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Distinguish the capacity guard (departure full/closed —
				// the increment rejects when booked_count + party exceeds
				// capacity) from an illegal status transition on the
				// booking itself.
				if s.departureFull(ctx, row) {
					return BookingDTO{}, ErrDepartureFull
				}

				return BookingDTO{}, ErrInvalidBookingTransition
			}

			return BookingDTO{}, fmt.Errorf("servicedir: confirm booking: %w", err)
		}
	case BookingStatusDeclined:
		_, err = s.store.DeclineBooking(ctx, bookingID, note)
	case BookingStatusFulfilled:
		_, err = s.store.FulfillBooking(ctx, bookingID)
	case BookingStatusCancelled:
		var reason *string
		if input.CancelReason != "" {
			reason = &input.CancelReason
		}

		// Release seats in the same tx as the status write when leaving
		// 'confirmed' — a post-commit decrement can be lost on failure.
		releaseDep := pgtype.UUID{}
		if row.Status == BookingStatusConfirmed {
			releaseDep = row.DepartureID
		}

		_, err = s.store.OrgCancelBookingAndRelease(ctx, bookingID, releaseDep, row.PartySize, note, reason)
	default:
		return BookingDTO{}, ErrInvalidBookingTransition
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, ErrInvalidBookingTransition
		}

		return BookingDTO{}, fmt.Errorf("servicedir: booking transition %s: %w", input.Status, err)
	}

	s.afterBookingTransition(ctx, row, input.Status)

	// booked_count moved on confirm and on seat release — drop the cached
	// package detail so the public page doesn't show stale availability.
	if row.DepartureID.Valid &&
		(input.Status == BookingStatusConfirmed || input.Status == BookingStatusCancelled) {
		s.invalidatePackageCache(row.ListingContentID)
	}

	return s.GetBooking(ctx, bookingID, userID, true)
}

// CancelMyBooking lets the booker cancel a pending or confirmed booking.
// If the booking held departure seats they are released. The fee refund
// decision (window check) is deferred to the settle task — the service
// computes it and stamps it in the payload.
func (s *Service) CancelMyBooking(ctx context.Context, bookingID, userID pgtype.UUID, reason string) (BookingDTO, error) {
	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: cancel: get booking: %w", err)
	}

	if row.BookerUserID != userID {
		return BookingDTO{}, ErrNotOwner
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	// Leaving 'confirmed' frees departure seats in the same transaction —
	// a lost decrement would strand booked_count forever (§111 seats are
	// counted iff status='confirmed').
	releaseDep := pgtype.UUID{}
	if row.Status == BookingStatusConfirmed {
		releaseDep = row.DepartureID
	}

	if _, err := s.store.CancelBookingAndRelease(ctx, bookingID, releaseDep, row.PartySize, reasonPtr); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, ErrInvalidBookingTransition
		}

		return BookingDTO{}, fmt.Errorf("servicedir: cancel booking: %w", err)
	}

	if row.Status == BookingStatusConfirmed && row.DepartureID.Valid {
		s.invalidatePackageCache(row.ListingContentID)
	}

	s.enqueueStatusChanged(row.ID, BookingStatusCancelled)
	s.enqueueFeeSettle(row.ID, s.cancelFeeAction(ctx, row))

	return s.GetBooking(ctx, bookingID, userID, false)
}

// AdminListBookings returns all bookings for the admin view.
func (s *Service) AdminListBookings(ctx context.Context, status, intent, orgIDStr string, page, limit int32) ([]BookingDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	var orgID pgtype.UUID
	if orgIDStr != "" {
		if err := orgID.Scan(orgIDStr); err != nil {
			return nil, 0, ErrInvalidID
		}
	}

	rows, total, err := s.store.AdminListBookings(ctx, BookingsAdminFilter{
		Status: status, Intent: intent, OrgID: orgID,
		Limit: limit, Offset: (page - 1) * limit,
	})
	if err != nil {
		return nil, 0, err
	}

	out := make([]BookingDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, bookingToDTO(r))
	}

	return out, total, nil
}

// AdminSetBookingStatus is the admin dispute-resolution transition. Any
// target status is allowed; the resolution note lands in
// org_response_note. Seat release + fee settle follow the same rules as
// org/booker transitions.
func (s *Service) AdminSetBookingStatus(ctx context.Context, bookingID pgtype.UUID, input AdminBookingActionInput) (BookingDTO, error) {
	if !IsValidBookingStatus(input.Status) {
		return BookingDTO{}, ErrInvalidBookingTransition
	}

	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: admin set status: get booking: %w", err)
	}

	var note *string
	if input.ResolutionNote != "" {
		note = &input.ResolutionNote
	}

	// Transitioning into 'confirmed' acquires departure seats — seats are
	// counted iff status='confirmed' (released on the way out, above and
	// in the org/booker paths). A bare status write would overbook, so
	// route the transition through the capacity-guarded increment.
	if input.Status == BookingStatusConfirmed &&
		row.Status != BookingStatusConfirmed && row.DepartureID.Valid {

		var cerr error
		if row.Status == BookingStatusPending {
			// pending → confirmed: the standard atomic confirm path.
			_, cerr = s.store.ConfirmBooking(ctx, bookingID, row.DepartureID, row.PartySize, note)
		} else {
			// e.g. disputed → confirmed: seats were released on the way
			// into 'disputed', so re-acquire in one tx with the status.
			_, cerr = s.store.AdminConfirmBooking(ctx, bookingID, row.DepartureID, row.PartySize, note)
		}

		if cerr != nil {
			if errors.Is(cerr, pgx.ErrNoRows) {
				if s.departureFull(ctx, row) {
					return BookingDTO{}, ErrDepartureFull
				}

				return BookingDTO{}, ErrInvalidBookingTransition
			}

			return BookingDTO{}, fmt.Errorf("servicedir: admin confirm: %w", cerr)
		}

		s.invalidatePackageCache(row.ListingContentID)
		s.enqueueStatusChanged(row.ID, input.Status)

		return s.GetBooking(ctx, bookingID, pgtype.UUID{}, true)
	}

	// Leaving 'confirmed' frees departure seats in the same transaction.
	releaseDep := pgtype.UUID{}
	if row.Status == BookingStatusConfirmed && input.Status != BookingStatusConfirmed {
		releaseDep = row.DepartureID
	}

	if _, err := s.store.AdminSetStatusAndRelease(ctx, bookingID, input.Status, note, releaseDep, row.PartySize); err != nil {
		return BookingDTO{}, fmt.Errorf("servicedir: admin set status: %w", err)
	}

	if releaseDep.Valid {
		s.invalidatePackageCache(row.ListingContentID)
	}

	s.enqueueStatusChanged(row.ID, input.Status)

	switch input.Status {
	case BookingStatusDeclined, BookingStatusRefunded:
		s.enqueueFeeSettle(row.ID, "refund")
	case BookingStatusCancelled:
		s.enqueueFeeSettle(row.ID, s.cancelFeeAction(ctx, row))
	case BookingStatusFulfilled:
		s.enqueueFeeSettle(row.ID, "recognize")
	}

	return s.GetBooking(ctx, bookingID, pgtype.UUID{}, true)
}

// paymentSettled reports whether the booking's fee payment has moved
// money — succeeded (held) or refunded (was captured, then returned).
// Both states justify org-side visibility as an audit record.
func paymentSettled(status *string) bool {
	return status != nil && (*status == "succeeded" || *status == "refunded")
}

// departureFull reports whether the booking's departure rejected the
// seat increment on availability grounds — not open, or capacity
// exceeded. §111: a NULL departure capacity inherits the package-level
// capacity (PackageCapacity on the detail row).
func (s *Service) departureFull(ctx context.Context, row BookingRow) bool {
	if !row.DepartureID.Valid {
		return false
	}

	dep, err := s.store.GetDeparture(ctx, row.DepartureID)
	if err != nil {
		// The increment already rejected; if the departure can't be
		// loaded at all, the booking can't be confirmed anyway.
		return true
	}

	capacity := dep.Capacity
	if capacity == nil {
		capacity = row.PackageCapacity
	}

	return dep.Status != DepartureStatusOpen ||
		(capacity != nil && dep.BookedCount+row.PartySize > *capacity)
}

// ---------------------------------------------------------------------------
// Side effects — enqueued after the booking row commits.
// ---------------------------------------------------------------------------

// afterBookingTransition runs the shared post-transition side effects for
// org-driven status changes.
func (s *Service) afterBookingTransition(ctx context.Context, prev BookingRow, newStatus string) {
	s.enqueueStatusChanged(prev.ID, newStatus)

	switch newStatus {
	case BookingStatusDeclined:
		s.enqueueFeeSettle(prev.ID, "refund")
	case BookingStatusFulfilled:
		s.enqueueFeeSettle(prev.ID, "recognize")
	case BookingStatusCancelled:
		// Org-initiated cancel always refunds (§111 refund policy). Seat
		// release already ran inside OrgCancelBookingAndRelease's tx.
		s.enqueueFeeSettle(prev.ID, "refund")
	}
}

// cancelFeeAction computes the fee outcome for a booker-initiated cancel:
// refund inside service_packages.refund_window_days of confirmation
// (or while still pending — the fee was taken to reserve, so a pending
// cancel always refunds), otherwise retain (the commitment mechanism).
func (s *Service) cancelFeeAction(ctx context.Context, row BookingRow) string {
	if row.Intent != BookingIntentBooking || !row.PaymentID.Valid {
		return "none"
	}

	if row.Status == BookingStatusPending {
		return "refund"
	}

	window := s.settings.GetIntOr(ctx, "service_packages.refund_window_days", 7)
	if row.ConfirmedAt.Valid && time.Since(row.ConfirmedAt.Time) <= time.Duration(window)*24*time.Hour {
		return "refund"
	}

	return "retain"
}

func (s *Service) enqueueBookingCreated(bookingID pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{"booking_id": bookingID.String()})
	if err := s.enqueuer.Enqueue(worker.TypeServiceBookingCreated, payload); err != nil {
		slog.Error("servicedir: enqueue booking_created failed", "booking_id", bookingID, "error", err)
	}
}

func (s *Service) enqueueStatusChanged(bookingID pgtype.UUID, status string) {
	if s.enqueuer == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"booking_id": bookingID.String(),
		"status":     status,
	})
	if err := s.enqueuer.Enqueue(worker.TypeServiceBookingStatusChanged, payload); err != nil {
		slog.Error("servicedir: enqueue status_changed failed", "booking_id", bookingID, "error", err)
	}
}

// enqueueFeeSettle enqueues the critical-tier settle task. action is one
// of 'recognize' | 'refund' | 'retain' | 'none' — the worker validates it
// against the booking's actual state (idempotent by booking_id).
func (s *Service) enqueueFeeSettle(bookingID pgtype.UUID, action string) {
	if s.enqueuer == nil || action == "none" {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"booking_id": bookingID.String(),
		"action":     action,
	})
	if err := s.enqueuer.Enqueue(worker.TypeServiceBookingFeeSettle, payload); err != nil {
		slog.Error("servicedir: enqueue fee_settle failed", "booking_id", bookingID, "error", err)
	}
}

// invalidatePackageCache drops the jc:svc_package:{content_id} detail
// cache after package/departure mutations.
func (s *Service) invalidatePackageCache(contentID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.vk.Del(ctx, "jc:svc_package:"+contentID.String()).Err(); err != nil {
		slog.Warn("servicedir: package cache invalidate failed", "content_id", contentID, "error", err)
	}
}

func strPtr(s string) *string { return &s }
