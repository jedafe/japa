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
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	mw "github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/worker"
)

// BookingDTO is the API response shape for one service_bookings row.
type BookingDTO struct {
	ID                string `json:"id"`
	ListingMetaID     string `json:"listing_meta_id"`
	ListingID         string `json:"listing_id,omitempty"` // the listing's public content id — the ?listing_id= filter key
	ListingTitle      string `json:"listing_title"`
	ListingSlug       string `json:"listing_slug"`
	ListingKind       string `json:"listing_kind"`
	DepartureID       string `json:"departure_id,omitempty"`
	DepartureStart    string `json:"departure_starts_on,omitempty"`
	DepartureEnd      string `json:"departure_ends_on,omitempty"`
	BookerUserID      string `json:"booker_user_id,omitempty"`
	BookerUsername    string `json:"booker_username,omitempty"`
	BookerDisplayName string `json:"booker_display_name,omitempty"`
	BookerAvatar      string `json:"booker_avatar,omitempty"`
	// Provider dimension (000824) — populated on user-provider
	// (mentorship) bookings; org fields are empty on those rows.
	ProviderUserID       string `json:"provider_user_id,omitempty"`
	ProviderUsername     string `json:"provider_username,omitempty"`
	ProviderDisplayName  string `json:"provider_display_name,omitempty"`
	ProviderAvatar       string `json:"provider_avatar,omitempty"`
	DestinationSpaceID   string `json:"destination_space_id,omitempty"`
	DestinationSpaceName string `json:"destination_space_name,omitempty"`
	// Goals carries the mentorship request's goals verbatim — read from
	// meta->>'goals' with the note column as the fallback (migrated
	// connection rows predate the meta key).
	Goals string `json:"goals,omitempty"`
	// ConnectionStatus is the legacy mentorship_connections vocabulary
	// the mentorship frontend still reads — populated only on
	// mentorship-kind rows: confirmed→active, fulfilled→completed,
	// everything else maps verbatim.
	ConnectionStatus string               `json:"connection_status,omitempty"`
	OrgID            string               `json:"org_id"`
	OrgName          string               `json:"org_name,omitempty"`
	OrgSlug          string               `json:"org_slug,omitempty"`
	Intent           string               `json:"intent"`
	PartySize        int32                `json:"party_size"`
	PreferredDates   *PreferredDatesInput `json:"preferred_dates,omitempty"`
	Note             string               `json:"note,omitempty"`
	Status           string               `json:"status"`
	PurchaseMode     string               `json:"purchase_mode"`
	PaymentID        string               `json:"payment_id,omitempty"`
	// Joined payment row — lets the booker UI offer checkout resume on a
	// pending fee. Empty when payment_id is unset (inquiries, zero fee).
	PaymentStatus   string `json:"payment_status,omitempty"`
	PaymentProvider string `json:"payment_provider,omitempty"`
	TotalPrice      *int64 `json:"total_price,omitempty"`
	FeeAmount       *int64 `json:"fee_amount,omitempty"`
	FeeCurrency     string `json:"fee_currency,omitempty"`
	AgentBalanceDue *int64 `json:"agent_balance_due,omitempty"`
	// Marketplace snapshots (spec §2.3/§2.4) — the tier/coupon the booking
	// was priced with, frozen at create time.
	TierID          string `json:"tier_id,omitempty"`
	TierName        string `json:"tier_name,omitempty"`
	TierPriceAmount *int64 `json:"tier_price_amount,omitempty"`
	CouponCode      string `json:"coupon_code,omitempty"`
	DiscountAmount  *int64 `json:"discount_amount,omitempty"`
	OrgResponseNote string `json:"org_response_note,omitempty"`
	DisputeReason   string `json:"dispute_reason,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	ConfirmedAt     string `json:"confirmed_at,omitempty"`
	FulfilledAt     string `json:"fulfilled_at,omitempty"`
	CancelledAt     string `json:"cancelled_at,omitempty"`
	CancelReason    string `json:"cancel_reason,omitempty"`
	CreatedAt       string `json:"created_at"`
	// §48/ADR-110 DM auto-open — populated only on the respond response
	// when a mentorship accept materialized the provider↔booker thread;
	// lets the client deep-link straight into /messages?c={id}.
	DMConversationID string `json:"dm_conversation_id,omitempty"`
}

// maxBookingTextLen caps free-text booking inputs (note, goals,
// cancel_reason, dispute_reason, respond note) — §111.
const maxBookingTextLen = 4000

// maxBookingPartySize is the absolute upper bound for a booking's
// party_size — a backstop against absurd inputs; departure/package
// capacity applies the tighter real limit below.
const maxBookingPartySize = 500

// ErrInvalidPartySize — party_size exceeds the absolute cap or the
// listing's capacity (400 invalid_party_size).
var ErrInvalidPartySize = errors.New("invalid party size")

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
// purchase_mode='paid' listing (the fee checkout fields). tier_id selects
// one of the listing's active service_package_tiers rows (marketplace spec
// §2.4) — parsed and priced by the redemption path. coupon_code applies a
// listing-attached coupon (spec §2.3) — both are rejected on inquiries.
type CreateBookingInput struct {
	Intent         string               `json:"intent"` // inquiry | booking (default derived from purchase_mode)
	DepartureID    string               `json:"departure_id"`
	TierID         string               `json:"tier_id"`
	CouponCode     string               `json:"coupon_code"`
	PartySize      int32                `json:"party_size"`
	PreferredDates *PreferredDatesInput `json:"preferred_dates"`
	Note           string               `json:"note"`
	// Mentorship-kind fields (000824): goals is stored in the note column
	// AND meta->>'goals'; destination_space_id is the mentee's target
	// space (validated against spaces). Both are ignored on non-mentorship
	// listings.
	Goals              string `json:"goals"`
	DestinationSpaceID string `json:"destination_space_id"`
	Provider           string `json:"provider"`
	Email              string `json:"email"`
	ReturnURL          string `json:"return_url"`
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

	if r.BookerDisplayName != nil {
		dto.BookerDisplayName = *r.BookerDisplayName
	}

	if r.BookerAvatar != nil {
		dto.BookerAvatar = *r.BookerAvatar
	}

	if r.ProviderUserID.Valid {
		dto.ProviderUserID = r.ProviderUserID.String()
	}

	if r.ProviderUsername != nil {
		dto.ProviderUsername = *r.ProviderUsername
	}

	if r.ProviderDisplayName != nil {
		dto.ProviderDisplayName = *r.ProviderDisplayName
	}

	if r.ProviderAvatar != nil {
		dto.ProviderAvatar = *r.ProviderAvatar
	}

	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}

	if r.DestinationSpaceName != nil {
		dto.DestinationSpaceName = *r.DestinationSpaceName
	}

	// Mentorship goals — meta->>'goals' wins, note is the fallback for
	// migrated rows that only carry goals in the note column (000824).
	if len(r.Meta) > 0 {
		var meta struct {
			Goals string `json:"goals"`
		}

		if err := json.Unmarshal(r.Meta, &meta); err == nil && meta.Goals != "" {
			dto.Goals = meta.Goals
		}
	}

	if dto.Goals == "" && r.ListingKind == ListingKindMentorship && r.Note != nil {
		dto.Goals = *r.Note
	}

	if r.ListingKind == ListingKindMentorship {
		dto.ConnectionStatus = mentorshipConnectionStatus(r.Status)
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

	if r.PaymentStatus != nil {
		dto.PaymentStatus = *r.PaymentStatus
	}

	if r.PaymentProvider != nil {
		dto.PaymentProvider = *r.PaymentProvider
	}

	if r.FeeCurrency != nil {
		dto.FeeCurrency = *r.FeeCurrency
	}

	if r.TierID.Valid {
		dto.TierID = r.TierID.String()
	}

	if r.TierName != nil {
		dto.TierName = *r.TierName
	}

	dto.TierPriceAmount = r.TierPriceAmount

	if r.CouponCode != nil {
		dto.CouponCode = *r.CouponCode
	}

	dto.DiscountAmount = r.DiscountAmount

	if r.OrgResponseNote != nil {
		dto.OrgResponseNote = *r.OrgResponseNote
	}

	if r.DisputeReason != nil {
		dto.DisputeReason = *r.DisputeReason
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

// mentorshipConnectionStatus maps the service_bookings state machine onto
// the legacy mentorship_connections vocabulary the mentorship frontend
// still reads (000824): confirmed → active, fulfilled → completed, every
// other status carries through unchanged.
func mentorshipConnectionStatus(status string) string {
	switch status {
	case BookingStatusConfirmed:
		return "active"
	case BookingStatusFulfilled:
		return "completed"
	default:
		return status
	}
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

// ErrInvalidReturnURL — return_url failed same-origin validation (M5).
var ErrInvalidReturnURL = errors.New("invalid return_url")

// validReturnURL reports whether raw is an acceptable fee-checkout
// return target (M5): empty (no override), a site-relative path ("/...",
// never protocol-relative "//" or backslash tricks), or an absolute URL
// on the request's own host (the client allowlist mirrors this — only
// same-origin hrefs are ever sent). Anything else is an open-redirect
// vector through the payment provider's success_url.
func validReturnURL(raw, host string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}

	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return !strings.Contains(raw, "\\") && !strings.Contains(raw, "://")
	}

	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return false
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}

	want := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		want = h
	}

	return strings.EqualFold(u.Hostname(), want)
}

// validBookingProviders mirrors the §17 provider enum
// (payments.IsValidProvider) so CreateBooking can reject an unknown
// provider with a structured 400 before any row is written — servicedir
// reaches payments only through the PaymentCreator seam, so the enum is
// duplicated at the boundary. Keep in sync with validProviders in
// internal/payments/payments.go.
var validBookingProviders = map[string]bool{
	"paystack":        true,
	"stripe":          true,
	"nowpayments":     true,
	"manual_transfer": true,
	"credit":          true,
}

// computeFee calculates the §111 platform fee for a package price (both
// minor units). fee_type='percent' → price × fee_pct / 100; 'fixed' →
// fee_fixed. The fee never exceeds the package price. M2: a nil settings
// reader (unwired tests, degraded deploys) falls back to the 10%-of-price
// default instead of dereferencing nil.
func (s *Service) computeFee(ctx context.Context, priceAmount int64) int64 {
	if s.settings == nil {
		fee := priceAmount * 10 / 100
		if fee > priceAmount {
			return priceAmount
		}

		return fee
	}

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

// bookingExpiresAt computes the pending SLA deadline. A nil settings
// reader falls back to the 72h default (same nil-guard as computeFee).
func (s *Service) bookingExpiresAt(ctx context.Context) pgtype.Timestamptz {
	sla := 72
	if s.settings != nil {
		sla = s.settings.GetIntOr(ctx, "service_packages.org_response_sla_hours", 72)
	}

	return pgtype.Timestamptz{Time: time.Now().Add(time.Duration(sla) * time.Hour), Valid: true}
}

// CreateBooking creates a service inquiry or paid booking against a
// listing (POST /services/:id/bookings). For intent='booking' on a
// purchase_mode='paid' package it snapshots total/fee/balance and opens
// the §17 fee checkout; the org never sees paid bookings until the fee
// payment succeeds (query-level filter + notification deferred to the
// payment-success path).
func (s *Service) CreateBooking(ctx context.Context, bookerID, listingContentID pgtype.UUID, input CreateBookingInput) (BookingResult, error) {
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

	// Mentorship-kind listings (000824) take an entirely different arm:
	// user-provider bookings carry provider_user_id instead of org_id,
	// are gated on mentorship.enabled (not service_packages.enabled), and
	// skip every paid/package surface.
	if listing.ListingKind == ListingKindMentorship {
		return s.createMentorshipBooking(ctx, bookerID, listingContentID, listing, input)
	}

	if !s.PackagesEnabled(ctx) {
		return BookingResult{}, ErrPackagesDisabled
	}

	// §111 free-text cap — note is unbounded user input landing in a text
	// column; reject oversized payloads before any row is written.
	if utf8.RuneCountInString(input.Note) > maxBookingTextLen {
		return BookingResult{}, ErrInputTooLong
	}

	// §111 availability window — a package that hasn't opened yet or is
	// past available_until doesn't accept bookings. Date columns compare
	// against the UTC civil date (pgtype.Date stores UTC midnight).
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if (listing.AvailableFrom.Valid && listing.AvailableFrom.Time.After(today)) ||
		(listing.AvailableUntil.Valid && listing.AvailableUntil.Time.Before(today)) {
		return BookingResult{}, ErrBookingUnavailable
	}

	// Org-less listings (personal listings created without an org) have no
	// org inbox — deliver the lead to the listing author through the same
	// user-provider arm mentorship requests use (000824): provider_user_id
	// = author, inquiry intent only. The DB provider check requires
	// exactly one of org_id/provider_user_id, so the author-provider row
	// inserts legally.
	providerUserID := pgtype.UUID{}
	if !listing.OrgID.Valid {
		if !listing.AuthorID.Valid {
			return BookingResult{}, ErrBookingUnavailable
		}

		if listing.AuthorID == bookerID {
			return BookingResult{}, ErrSelfBooking
		}

		providerUserID = listing.AuthorID
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
		// Both intents valid on paid listings — an inquiry is a pre-sales
		// lead landing in the provider's inbox with no payment surface
		// ("Message provider" on org listings routes here).
	default: // inquiry
		if intent != BookingIntentInquiry {
			return BookingResult{}, ErrInvalidIntent
		}
	}

	// The author-provider arm is inquiry-only: paid bookings are org-bound
	// (fee settlement, org inbox, refund policy are all org concepts).
	if providerUserID.Valid && intent != BookingIntentInquiry {
		return BookingResult{}, ErrBookingUnavailable
	}

	// Coupons and tiers only apply to paid bookings — an inquiry naming
	// either is a malformed request, not a silent ignore (parity with the
	// strict intent checks above).
	if intent == BookingIntentInquiry && (input.TierID != "" || input.CouponCode != "") {
		return BookingResult{}, ErrInvalidCoupon
	}

	// Rule 9 — a booking intent is a financial transaction attempt:
	// Idempotency-Key is required before pricing runs so a rejected
	// request writes nothing (and consumes no coupon rule checks).
	if intent == BookingIntentBooking && mw.IdempotencyKeyFromContext(ctx) == "" {
		return BookingResult{}, ErrIdempotencyRequired
	}

	// §111 monetary snapshot for paid bookings — tier + coupon resolution
	// live in priceQuote (spec §2.3/§2.4). Computed up front so the
	// checkout-field checks below can skip a zero configured fee
	// (fee_pct=0 / fee_fixed=0 means no charge and no checkout).
	var quote Quote

	if intent == BookingIntentBooking {
		var qerr error

		quote, qerr = s.priceQuote(ctx, listing, bookerID, input.TierID, input.CouponCode)
		if qerr != nil {
			return BookingResult{}, qerr
		}

		if quote.Currency == "" {
			return BookingResult{}, ErrMissingPackagePrice
		}
	}

	// A fee of zero means no checkout — the booking lands directly in the
	// org inbox (mirrors the inbox query's `sb.fee_amount = 0` arm).
	paidCheckout := intent == BookingIntentBooking && quote.Fee > 0

	partySize := input.PartySize
	if partySize <= 0 {
		partySize = 1
	}

	// Fail fast on absurd party sizes — previously an unbounded party
	// spammed the inbox and failed only at confirm (departure_full).
	// The departure/package capacity gates apply the tighter real limit
	// when a departure is selected below.
	if partySize > maxBookingPartySize {
		return BookingResult{}, ErrInvalidPartySize
	}

	// departure_id and preferred_dates are mutually exclusive — both
	// supplied is a malformed request, not a silent coin-flip on which
	// date surface wins.
	if input.DepartureID != "" && input.PreferredDates != nil {
		return BookingResult{}, ErrInvalidDeparture
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

		// A departure that already started can't take new bookings.
		if dep.StartsOn.Valid && dep.StartsOn.Time.Before(today) {
			return BookingResult{}, ErrInvalidDeparture
		}

		// Capacity is knowable now — check it at create (400) rather
		// than failing the org's confirm with departure_full later. A
		// NULL departure capacity inherits the package-level capacity
		// (same rule as departureFull); both NULL means unlimited.
		capacity := dep.Capacity
		if capacity == nil {
			capacity = listing.Capacity
		}

		if capacity != nil && dep.BookedCount+partySize > *capacity {
			return BookingResult{}, ErrDepartureFull
		}
	} else if listing.Capacity != nil && partySize > *listing.Capacity {
		// Non-departure booking on a capacity-capped package — the org
		// confirm path has no seat counter for these, so enforce here.
		return BookingResult{}, ErrInvalidPartySize
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
	if paidCheckout {
		if input.Provider == "" || input.Email == "" {
			return BookingResult{}, ErrCheckoutFieldsRequired
		}

		// The provider enum lives in §17 (internal/payments); mirrored here
		// because servicedir is decoupled through the PaymentCreator seam —
		// a bad provider would otherwise surface as a 500 after the row
		// committed (and the compensating cancel).
		if !validBookingProviders[input.Provider] {
			return BookingResult{}, ErrInvalidProvider
		}
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

	if providerUserID.Valid {
		// Author-provider rows carry no SLA expiry (same posture as
		// mentorship rows — a personal inbox is not SLA-bound).
		params.ProviderUserID = providerUserID
		params.ExpiresAt = pgtype.Timestamptz{}
	}

	if input.Note != "" {
		params.Note = &input.Note
	}

	// §111 monetary snapshot for paid bookings — values frozen at booking
	// time so later price/fee-setting changes don't mutate open bookings.
	// Tier name/price snapshot likewise freezes the selected tier (§2.4):
	// a later tiers[] replace-set must not rewrite what the booker paid.
	if intent == BookingIntentBooking {
		params.TotalPrice = &quote.Total
		params.FeeAmount = &quote.Fee
		params.FeeCurrency = &quote.Currency
		params.AgentBalanceDue = &quote.Balance

		if quote.TierID.Valid {
			params.TierID = quote.TierID
			params.TierName = &quote.TierName
			params.TierPriceAmount = &quote.Base
		}

		if quote.CouponID.Valid {
			params.CouponID = quote.CouponID
			params.CouponCode = &quote.CouponCode
			params.DiscountAmount = &quote.Discount
		}
	}

	// The coupon_redemptions row commits in the same transaction as the
	// booking — a failed redemption insert rolls the booking back rather
	// than silently losing the recorded discount (spec §2.3).
	var redemption *gen.CreateCouponRedemptionParams

	var guard *couponCommitGuard

	if quote.CouponID.Valid {
		redemption = &gen.CreateCouponRedemptionParams{
			CouponID:      quote.CouponID,
			UserID:        bookerID,
			ListingMetaID: listing.MetaID,
			AmountSaved:   quote.Discount,
			Currency:      params.FeeCurrency,
		}
		// Cap re-validation under the coupon row lock — the priceQuote
		// counts ran unlocked, so the commit path re-checks them inside
		// the booking transaction. Trimmed like priceQuote so a padded
		// code locks the same row it quoted.
		guard = &couponCommitGuard{
			Code:     strings.TrimSpace(input.CouponCode),
			MetaID:   listing.MetaID,
			BookerID: bookerID,
			OrgID:    listing.OrgID,
			Base:     quote.Base,
			Kind:     listing.ListingKind,
			Currency: quote.Currency,
			Discount: quote.Discount,
		}
	}

	booking, err := s.store.createBookingWithRedemption(ctx, params, redemption, guard)
	if err != nil {
		// The dedupe index covers every org_id IS NULL row — the
		// author-provider arm included — so a second open inquiry to the
		// same listing author surfaces here rather than as a raw 500.
		if isDuplicateMentorshipBookingErr(err) {
			return BookingResult{}, ErrDuplicateInquiry
		}

		return BookingResult{}, err
	}

	result := BookingResult{Booking: bookingToDTO(bookingFromGen(booking))}

	// bookingToDTO reads joined listing columns that only exist on the
	// list/detail queries — the create path must stamp them itself.
	result.Booking.ListingID = listingContentID.String()
	result.Booking.ListingTitle = listing.Title
	result.Booking.ListingSlug = listing.Slug
	result.Booking.ListingKind = listing.ListingKind

	if paidCheckout {
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

// createMentorshipBooking is the listing_kind='mentorship' arm of
// CreateBooking (000824 unification). A mentorship request is always an
// inquiry: no paid surface (tiers/coupons/booking intent rejected), no
// SLA expiry (expires_at stays NULL so the pending-expiry sweep never
// touches it), party_size 1, provider_user_id = the shadow listing's
// author. Capacity and dedupe mirror the dropped mentorship_connections
// rules: one open request per (booker, mentor), and the provider's
// active_mentee_count must be below mentor_capacity.
func (s *Service) createMentorshipBooking(ctx context.Context, bookerID, listingContentID pgtype.UUID, listing ServiceRow, input CreateBookingInput) (BookingResult, error) {
	if !s.MentorshipEnabled(ctx) {
		return BookingResult{}, ErrMentorshipDisabled
	}

	// The listing lookup (GetServiceListingByContentID) is deliberately
	// status-agnostic so non-published rows can produce kind-correct 404s —
	// but an in_review/draft/unpublished shadow must not mint bookings.
	// Booking only becomes possible once the admin review gate publishes
	// the shadow (000824 fix-pass).
	if listing.Status != content.StatusPublished {
		return BookingResult{}, pgx.ErrNoRows
	}

	// §111 free-text caps — goals/note are unbounded user input landing in
	// a text column + meta JSON; reject oversized payloads early.
	if utf8.RuneCountInString(input.Note) > maxBookingTextLen || utf8.RuneCountInString(input.Goals) > maxBookingTextLen {
		return BookingResult{}, ErrInputTooLong
	}

	providerID := listing.AuthorID
	if bookerID == providerID {
		return BookingResult{}, ErrSelfMentorship
	}

	// Paid surfaces don't exist on mentorship requests — an explicit
	// non-inquiry intent or a tier/coupon reference is a malformed request,
	// same strictness as the inquiry/package arms above.
	if input.Intent != "" && input.Intent != BookingIntentInquiry {
		return BookingResult{}, ErrInvalidIntent
	}

	if input.TierID != "" || input.CouponCode != "" {
		return BookingResult{}, ErrInvalidCoupon
	}

	// Mentorship requests are point-in-time inquiries — no departure or
	// date surface exists. preferred_dates is rejected outright rather
	// than silently dropped so the caller learns the field does nothing
	// on this kind (§111 hardening amendment).
	if input.PreferredDates != nil {
		return BookingResult{}, ErrInvalidInput
	}

	// Dedupe keys on (booker, provider) — the same pair the
	// idx_svc_bookings_mentorship_open_uniq partial index (000828) guards
	// at the DB level. Keying on the mentor user keeps the gate closed
	// when the shadow meta row is re-minted.
	open, err := s.store.HasOpenMentorshipBooking(ctx, bookerID, providerID)
	if err != nil {
		return BookingResult{}, err
	}

	if open {
		return BookingResult{}, ErrDuplicateRequest
	}

	cap, err := s.store.GetMentorCapacity(ctx, providerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A shadow listing without a user_profiles row can't enforce
			// capacity — treat as unavailable rather than unbounded.
			return BookingResult{}, ErrBookingUnavailable
		}

		return BookingResult{}, fmt.Errorf("servicedir: mentorship booking: capacity probe: %w", err)
	}

	// Orphan-shadow re-verify: a live listing_kind='mentorship' row whose
	// author no longer has is_mentor (opt-out raced the listing fetch, or
	// a stale shadow survived) must not mint bookings. The compat layer
	// maps ErrBookingUnavailable → mentor_not_found.
	if cap.IsMentor == nil || !*cap.IsMentor {
		return BookingResult{}, ErrBookingUnavailable
	}

	if cap.Capacity != nil {
		var active int16
		if cap.Active != nil {
			active = *cap.Active
		}

		if active >= *cap.Capacity {
			return BookingResult{}, ErrMentorAtCapacity
		}
	}

	var destSpaceID pgtype.UUID
	if input.DestinationSpaceID != "" {
		if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
			return BookingResult{}, ErrInvalidID
		}

		exists, err := s.store.SpaceExists(ctx, destSpaceID)
		if err != nil {
			return BookingResult{}, fmt.Errorf("servicedir: mentorship booking: space probe: %w", err)
		}

		if !exists {
			return BookingResult{}, ErrInvalidID
		}
	}

	party := int32(1)
	params := gen.CreateServiceBookingParams{
		ListingMetaID:      listing.MetaID,
		BookerUserID:       bookerID,
		ProviderUserID:     providerID,
		DestinationSpaceID: destSpaceID,
		Intent:             BookingIntentInquiry,
		PartySize:          &party,
		Status:             strPtr(BookingStatusPending),
		PurchaseMode:       listing.PurchaseMode,
		// ExpiresAt deliberately unset: mentorship requests are not
		// SLA-bound — NULL keeps them out of the pending-expiry sweep.
	}

	// goals rides both surfaces: the note column (display/migrated-row
	// parity) and meta->>'goals' (the structured read the DTO prefers).
	// A bare note still lands in note for API forgiveness.
	note := input.Goals
	if note == "" {
		note = input.Note
	}

	if note != "" {
		params.Note = &note
	}

	if input.Goals != "" {
		meta, err := json.Marshal(map[string]string{"goals": input.Goals})
		if err != nil {
			return BookingResult{}, fmt.Errorf("servicedir: mentorship booking: meta encode: %w", err)
		}

		params.Meta = meta
	}

	booking, err := s.store.createBookingWithRedemption(ctx, params, nil, nil)
	if err != nil {
		// 000825: the partial unique index
		// idx_svc_bookings_mentorship_open_uniq is the DB-level guard for
		// the SELECT-then-INSERT race above — two concurrent requests can
		// both pass HasOpenMentorshipBooking; the loser surfaces here.
		if isDuplicateMentorshipBookingErr(err) {
			return BookingResult{}, ErrDuplicateRequest
		}

		return BookingResult{}, err
	}

	result := BookingResult{Booking: bookingToDTO(bookingFromGen(booking))}

	// bookingToDTO reads joined listing columns that only exist on the
	// list/detail queries — the create path must stamp them itself.
	result.Booking.ListingID = listingContentID.String()
	result.Booking.ListingTitle = listing.Title
	result.Booking.ListingSlug = listing.Slug
	result.Booking.ListingKind = listing.ListingKind
	result.Booking.ProviderUserID = providerID.String()

	// §102 analytics: mentorship_request_sent — the mentee is the actor;
	// emitted here (not only in the compat route) so the unified UI flow
	// the frontend actually calls produces the event.
	s.trackEvent(ctx, bookerID, "mentorship_request_sent", "mentorship_connection", booking.ID,
		map[string]any{"mentor_id": providerID.String()})

	// §102: refresh the mentee's goals embedding — the latest request's
	// goals text feeds mentee_goal_embedding for sort=match scoring.
	if s.enqueuer != nil {
		if payload, merr := json.Marshal(map[string]string{"user_id": bookerID.String()}); merr == nil {
			if eerr := s.enqueuer.Enqueue("mentor:generate_embedding", payload); eerr != nil {
				slog.Warn("servicedir: enqueue mentee goals embedding", "error", eerr)
			}
		}
	}
	result.Booking.ConnectionStatus = mentorshipConnectionStatus(result.Booking.Status)

	// Inquiry path — notify immediately (audience resolution for
	// provider-owned rows is the worker's job).
	s.enqueueBookingCreated(booking.ID)

	return result, nil
}

// GetBooking returns the booking detail for the booker, the provider user
// (mentorship), a member of the owning org, or an admin (checked at route
// level for admin).
func (s *Service) GetBooking(ctx context.Context, bookingID, viewerID pgtype.UUID, isAdmin bool) (BookingDTO, error) {
	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: get booking: %w", err)
	}

	// Read access: the booker, the provider user (000824 user-provider
	// bookings), or a member of the owning org. org_id is nullable now —
	// the org-membership probe only runs when the booking is org-owned.
	if !isAdmin && row.BookerUserID != viewerID && row.ProviderUserID != viewerID {
		if !row.OrgID.Valid {
			return BookingDTO{}, ErrNotOwner
		}

		isMember, err := s.store.IsOrgMember(ctx, row.OrgID, viewerID)
		if err != nil {
			return BookingDTO{}, fmt.Errorf("servicedir: booking: org membership check: %w", err)
		}

		if !isMember {
			return BookingDTO{}, ErrNotOwner
		}

		// Org-side visibility gates paid bookings on money moved — same
		// rule as the inbox query (succeeded|refunded, or a zero fee).
		// A pending/failed fee payment means the booking never landed
		// in the org's queue.
		if row.Intent == BookingIntentBooking && !feeSettled(row) {
			return BookingDTO{}, pgx.ErrNoRows
		}
	}

	dto := bookingToDTO(row)

	// §48/ADR-110 — on confirmed/fulfilled mentorship rows the provider↔
	// booker thread already exists (accept materialized it), so surface
	// its id on the detail read: the booker's "Open messages" deep-links
	// straight into /messages?c={id} instead of the bare inbox.
	if row.ListingKind == ListingKindMentorship &&
		(row.Status == "confirmed" || row.Status == "fulfilled") &&
		row.ProviderUserID.Valid && row.BookerUserID.Valid && s.dmCreator != nil {
		if conv, err := s.dmCreator.GetConversationByParticipants(ctx, row.ProviderUserID, row.BookerUserID); err == nil && conv.ID.Valid {
			dto.DMConversationID = conv.ID.String()
		}
	}

	return dto, nil
}

// BookingPayInput is the POST /me/service-bookings/:id/pay body —
// the payer email providers require for re-initiation, plus an optional
// return URL override (same fields the create-checkout path accepts).
type BookingPayInput struct {
	Email     string `json:"email"`
	ReturnURL string `json:"return_url,omitempty"`
}

// ResumeBookingPayment re-initiates the hosted fee checkout for a pending
// paid booking whose session was abandoned before capture (§111). The
// payment row is reused — see payments.ResumePayment. Booker-only: org
// members and admins cannot pay someone else's fee.
func (s *Service) ResumeBookingPayment(ctx context.Context, bookingID, userID pgtype.UUID, input BookingPayInput) (*BookingPayment, error) {
	// Rule 9 — resume mints a new provider session, so it carries the same
	// Idempotency-Key requirement as booking creation.
	if mw.IdempotencyKeyFromContext(ctx) == "" {
		return nil, ErrIdempotencyRequired
	}

	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		return nil, err // pgx.ErrNoRows → not_found at the handler
	}

	if row.BookerUserID != userID {
		return nil, ErrNotOwner
	}

	// Only a pending paid booking with an unsettled fee has a checkout to
	// resume. payment_status is joined from the payments row — pending is
	// the sole resumable state (processing/succeeded/failed/refunded are
	// all closed for this booking).
	if row.Intent != BookingIntentBooking || row.Status != BookingStatusPending ||
		!row.PaymentID.Valid || row.PaymentStatus == nil ||
		*row.PaymentStatus != "pending" {
		return nil, ErrInvalidBookingTransition
	}

	if s.payments == nil {
		return nil, errors.New("payments service unavailable")
	}

	res, err := s.payments.ResumePayment(ctx, row.PaymentID, userID, input.Email, input.ReturnURL)
	if err != nil {
		return nil, fmt.Errorf("servicedir: resume booking payment: %w", err)
	}

	return &BookingPayment{ID: res.PaymentID, CheckoutURL: res.CheckoutURL}, nil
}

// ListMyBookings returns the caller's bookings + inquiries (role=booker).
func (s *Service) ListMyBookings(ctx context.Context, userID pgtype.UUID, status, intent, kind string, page, limit int32) ([]BookingDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	rows, total, err := s.store.ListBookingsByUser(ctx, userID, BookingsByUserFilter{
		Status: status, Intent: intent, Kind: kind, Limit: limit, Offset: (page - 1) * limit,
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

// ListProviderBookings returns the caller's provider-side inbox (000824 —
// role=provider on GET /me/service-bookings): bookings where
// provider_user_id = the caller. Mentorship is the only user-provider
// surface today, so the inbox is gated on mentorship.enabled.
func (s *Service) ListProviderBookings(ctx context.Context, providerID pgtype.UUID, status, intent, kind string, page, limit int32) ([]BookingDTO, int64, error) {
	if !s.MentorshipEnabled(ctx) {
		return nil, 0, ErrMentorshipDisabled
	}

	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	rows, total, err := s.store.ListBookingsByProviderUser(ctx, providerID, BookingsByProviderFilter{
		Status: status, Intent: intent, Kind: kind, Limit: limit, Offset: (page - 1) * limit,
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

// BookingRespondInput is the POST /me/service-bookings/:id/respond body —
// the provider's action on a user-owned (mentorship) booking.
type BookingRespondInput struct {
	Action string `json:"action"` // accept | decline | complete | cancel
	Note   string `json:"note"`
}

// RespondToBooking applies the provider-side transition on a
// user-provider booking (000824): accept (pending→confirmed), decline
// (pending→declined, note → org_response_note), complete
// (confirmed→fulfilled), cancel (pending|confirmed→cancelled, note →
// cancel_reason — the mentor arm of "either party cancels", delegated to
// CancelProviderBooking). Only the booking's provider_user_id may respond
// — org-owned bookings keep the PATCH /org/:org_id/service-bookings/:id
// route; this method refuses them outright.
func (s *Service) RespondToBooking(ctx context.Context, providerID, bookingID pgtype.UUID, action, note string) (BookingDTO, error) {
	if !s.MentorshipEnabled(ctx) {
		return BookingDTO{}, ErrMentorshipDisabled
	}

	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: respond: get booking: %w", err)
	}

	if !row.ProviderUserID.Valid || row.ProviderUserID != providerID || row.OrgID.Valid {
		return BookingDTO{}, ErrNotOwner
	}

	if utf8.RuneCountInString(note) > maxBookingTextLen {
		return BookingDTO{}, ErrInputTooLong
	}

	var notePtr *string
	if note != "" {
		notePtr = &note
	}

	var newStatus string

	switch action {
	case "accept":
		newStatus = BookingStatusConfirmed
		_, err = s.store.ConfirmBooking(ctx, bookingID, pgtype.UUID{}, row.PartySize, notePtr)
	case "decline":
		newStatus = BookingStatusDeclined
		_, err = s.store.DeclineBooking(ctx, bookingID, notePtr)
	case "complete":
		newStatus = BookingStatusFulfilled
		_, err = s.store.FulfillBooking(ctx, bookingID)
	case "cancel":
		// pending|confirmed → cancelled with cancel_reason — the
		// dedicated method owns the transition, the booker notification,
		// and the mentee-count re-index.
		return s.CancelProviderBooking(ctx, providerID, bookingID, note)
	default:
		return BookingDTO{}, ErrInvalidBookingTransition
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, ErrInvalidBookingTransition
		}

		return BookingDTO{}, fmt.Errorf("servicedir: respond %s: %w", action, err)
	}

	// Booker notification — the default status_changed audience is the
	// booker (the mentee learns their request was answered).
	s.enqueueStatusChanged(row.ID, newStatus)

	var dmConvID string

	if action == "accept" && row.ListingKind == ListingKindMentorship {
		// §48/ADR-110: auto-open the mentor↔mentee DM thread. Non-fatal —
		// the booking is already confirmed. The conversation id rides the
		// response so the UI can deep-link into the thread (the auto-open).
		if s.dmCreator != nil {
			if conv, dmErr := s.dmCreator.GetOrCreateConversation(ctx, row.ProviderUserID, row.BookerUserID); dmErr != nil {
				slog.Warn("servicedir: mentorship accept DM auto-open failed",
					"booking_id", bookingID, "error", dmErr)
			} else if conv.ID.Valid {
				dmConvID = conv.ID.String()
			}
		}

		// §102 analytics: mentorship_accepted — the mentor is the actor.
		s.trackEvent(ctx, providerID, "mentorship_accepted", "mentorship_connection", bookingID,
			map[string]any{"mentee_id": row.BookerUserID.String()})
	}

	// §102 analytics: mentorship_completed — fires on the provider's
	// 'complete' arm; the mentee arm lives in CompleteMyBooking.
	if action == "complete" && row.ListingKind == ListingKindMentorship {
		s.trackEvent(ctx, providerID, "mentorship_completed", "mentorship_connection", bookingID, nil)
	}

	// Any mentorship-kind transition out of 'confirmed' (complete) or into
	// it (accept) moved active_mentee_count via the booking trigger —
	// re-index the mentor's search document (§102).
	if row.ListingKind == ListingKindMentorship &&
		(action == "accept" || action == "complete") {
		s.enqueueMentorMeilisearch(row.ProviderUserID)
	}

	dto, err := s.GetBooking(ctx, bookingID, providerID, false)
	if err != nil {
		return dto, err
	}

	dto.DMConversationID = dmConvID

	return dto, nil
}

// isDuplicateMentorshipBookingErr reports whether err is the SQLSTATE
// 23505 raised by idx_svc_bookings_mentorship_open_uniq (000825) — the
// DB-level guard backing the SELECT-then-INSERT dedupe race in
// createMentorshipBooking.
func isDuplicateMentorshipBookingErr(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		pgErr.ConstraintName == "idx_svc_bookings_mentorship_open_uniq"
}

// enqueueMentorMeilisearch fires the mentor:update_meilisearch task — the
// mentor's Meilisearch document is re-indexed whenever their mentee count
// may have moved (payload mirrors the §102 producer: {user_id}).
func (s *Service) enqueueMentorMeilisearch(mentorID pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{"user_id": mentorID.String()})
	if err := s.enqueuer.Enqueue(worker.TypeMentorUpdateMeilisearch, payload); err != nil {
		slog.Warn("servicedir: enqueue mentor:update_meilisearch failed", "mentor_id", mentorID, "error", err)
	}
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

// OrgTransition applies the org-facing status transition
// (confirmed|declined|fulfilled|cancelled) with org-membership and
// state-machine guards. Side effects (notifications, fee settlement,
// seat release) are enqueued after the row commits.
func (s *Service) OrgTransition(ctx context.Context, orgID, bookingID, userID pgtype.UUID, input OrgBookingActionInput) (BookingDTO, error) {
	if !s.PackagesEnabled(ctx) {
		return BookingDTO{}, ErrPackagesDisabled
	}

	// §111 hardening — the privileged free-text fields carry the same
	// 4000-char cap as the booker-facing inputs (input_too_long).
	if utf8.RuneCountInString(input.OrgResponseNote) > maxBookingTextLen ||
		utf8.RuneCountInString(input.CancelReason) > maxBookingTextLen {
		return BookingDTO{}, ErrInputTooLong
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
	// org confirm a booking whose fee was never captured. 403, not 404:
	// the row is real, just not actionable yet.
	if row.Intent == BookingIntentBooking && !feeSettled(row) {
		return BookingDTO{}, ErrPaymentPending
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

	if utf8.RuneCountInString(reason) > maxBookingTextLen {
		return BookingDTO{}, ErrInputTooLong
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

	// Booker-initiated — the alert goes to the PROVIDER side (mentee
	// cancelling never self-notifies "provider cancelled"): mentorship
	// rows → provider_user_id, org rows → org members.
	s.enqueueStatusChangedAudience(row.ID, BookingStatusCancelled, "provider")
	s.enqueueFeeSettle(row.ID, s.cancelFeeAction(ctx, row))

	// Mentee-initiated cancel of an active mentorship connection also
	// moves active_mentee_count — same §102 reindex as the provider arm.
	if row.ListingKind == ListingKindMentorship && row.Status == BookingStatusConfirmed {
		s.enqueueMentorMeilisearch(row.ProviderUserID)
	}

	return s.GetBooking(ctx, bookingID, userID, false)
}

// DisputeMyBooking lets the booker report a problem on a confirmed or
// fulfilled booking (POST /me/service-bookings/:id/dispute). The reason
// lands in dispute_reason and the row moves to 'disputed' — the admin
// dispute queue owns resolution. Any held departure seat releases in the
// same transaction; no fee action fires here (admin resolution decides
// refund vs. uphold).
func (s *Service) DisputeMyBooking(ctx context.Context, bookingID, userID pgtype.UUID, reason string) (BookingDTO, error) {
	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: dispute: get booking: %w", err)
	}

	if row.BookerUserID != userID {
		return BookingDTO{}, ErrNotOwner
	}

	// Mentorship-kind rows are inquiry-only and non-disputable (§111):
	// disputed/refunded are unreachable on mentor requests — there is no
	// fee to contest. The query's org_id IS NOT NULL predicate is the
	// defence-in-depth backstop.
	if row.ListingKind == ListingKindMentorship {
		return BookingDTO{}, ErrInvalidBookingTransition
	}

	if utf8.RuneCountInString(reason) > maxBookingTextLen {
		return BookingDTO{}, ErrInputTooLong
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	if _, err := s.store.DisputeBookingAndRelease(ctx, bookingID, row.DepartureID, row.PartySize, reasonPtr); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, ErrInvalidBookingTransition
		}

		return BookingDTO{}, fmt.Errorf("servicedir: dispute booking: %w", err)
	}

	if row.DepartureID.Valid {
		s.invalidatePackageCache(row.ListingContentID)
	}

	// Notify the org — the booker raised the dispute, so the alert is
	// org-facing (audience=org → service_booking_disputed).
	s.enqueueStatusChangedAudience(row.ID, BookingStatusDisputed, "org")

	return s.GetBooking(ctx, bookingID, userID, false)
}

// AdminListBookings returns all bookings for the admin view.
func (s *Service) AdminListBookings(ctx context.Context, status, intent, orgIDStr, kind, providerIDStr, bookerIDStr string, page, limit int32) ([]BookingDTO, int64, error) {
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

	// Mentorship-compat filters (000824): kind/provider/booker narrow the
	// dispute queue to mentor rows. Malformed UUIDs are 400, not silent
	// ignore — a typo'd filter must not masquerade as "no results".
	var providerID, bookerID pgtype.UUID
	if providerIDStr != "" {
		if err := providerID.Scan(providerIDStr); err != nil {
			return nil, 0, ErrInvalidID
		}
	}

	if bookerIDStr != "" {
		if err := bookerID.Scan(bookerIDStr); err != nil {
			return nil, 0, ErrInvalidID
		}
	}

	if kind != "" && kind != ListingKindService && kind != ListingKindPackage && kind != ListingKindMentorship {
		return nil, 0, ErrInvalidListingKind
	}

	rows, total, err := s.store.AdminListBookings(ctx, BookingsAdminFilter{
		Status: status, Intent: intent, OrgID: orgID,
		Kind: kind, ProviderUserID: providerID, BookerUserID: bookerID,
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

// adminAllowedTransitions is the §111 admin dispute-resolution matrix —
// which target statuses are reachable from each source. Terminal rows
// (cancelled/declined/refunded) stay terminal: reopening them would
// resurrect dead money flows. 'refunded' is reachable only via the fee
// worker, never by direct admin jump from a non-disputed source — the
// refund itself is the audit trail.
var adminAllowedTransitions = map[string]map[string]bool{
	BookingStatusPending: {
		BookingStatusConfirmed: true,
		BookingStatusDeclined:  true,
		BookingStatusCancelled: true,
	},
	BookingStatusConfirmed: {
		BookingStatusFulfilled: true,
		BookingStatusCancelled: true,
		BookingStatusDisputed:  true,
	},
	BookingStatusFulfilled: {
		BookingStatusDisputed: true,
	},
	BookingStatusDisputed: {
		BookingStatusRefunded:  true,
		BookingStatusConfirmed: true,
		BookingStatusFulfilled: true,
		BookingStatusCancelled: true,
	},
}

// AdminSetBookingStatus is the admin dispute-resolution transition. Any
// target status is allowed; the resolution note lands in
// org_response_note. Seat release + fee settle follow the same rules as
// org/booker transitions.
func (s *Service) AdminSetBookingStatus(ctx context.Context, bookingID pgtype.UUID, input AdminBookingActionInput) (BookingDTO, error) {
	if !IsValidBookingStatus(input.Status) {
		return BookingDTO{}, ErrInvalidBookingTransition
	}

	// §111 hardening — the admin resolution note is privileged input but
	// lands in the same text column as org_response_note; same cap.
	if utf8.RuneCountInString(input.ResolutionNote) > maxBookingTextLen {
		return BookingDTO{}, ErrInputTooLong
	}

	row, err := s.store.GetBookingDetail(ctx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookingDTO{}, pgx.ErrNoRows
		}

		return BookingDTO{}, fmt.Errorf("servicedir: admin set status: get booking: %w", err)
	}

	// No-op transitions are a no-op — same-status writes would bump
	// updated_at and re-fire notifications for nothing.
	if input.Status == row.Status {
		return bookingToDTO(row), nil
	}

	// Enforce the transition matrix — previously any status jumped to any
	// other (e.g. refunded→pending resurrecting dead money flows).
	if !adminAllowedTransitions[row.Status][input.Status] {
		return BookingDTO{}, ErrInvalidBookingTransition
	}

	var note *string
	if input.ResolutionNote != "" {
		note = &input.ResolutionNote
	}

	// §111 seat model: seats are held iff status ∈ {confirmed, fulfilled}
	// (a fulfilled booking consumed its seat). Acquire on entering a
	// seat-holding status, release on leaving one — always inside the
	// same transaction as the status write.
	holdsSeat := func(st string) bool {
		return st == BookingStatusConfirmed || st == BookingStatusFulfilled
	}
	acquire := holdsSeat(input.Status) && !holdsSeat(row.Status)
	release := !holdsSeat(input.Status) && holdsSeat(row.Status)

	// Mentorship rows: an admin transition crossing 'confirmed' in either
	// direction moved active_mentee_count via the booking trigger —
	// re-index the mentor's §102 document (same as the respond/cancel
	// paths; unrelated mentorship transitions don't touch the count).
	crossedConfirmed := row.ListingKind == ListingKindMentorship &&
		(row.Status == BookingStatusConfirmed) != (input.Status == BookingStatusConfirmed)

	switch {
	case acquire && row.DepartureID.Valid:
		var cerr error
		if input.Status == BookingStatusConfirmed && row.Status == BookingStatusPending {
			// pending → confirmed: the standard atomic confirm path.
			_, cerr = s.store.ConfirmBooking(ctx, bookingID, row.DepartureID, row.PartySize, note)
		} else {
			// e.g. disputed → confirmed/fulfilled or pending → fulfilled:
			// re-acquire seats in one tx with the status write.
			_, cerr = s.store.AdminSetStatusAcquire(ctx, bookingID, row.DepartureID, row.PartySize, input.Status, note)
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

		if crossedConfirmed {
			s.enqueueMentorMeilisearch(row.ProviderUserID)
		}

		return s.GetBooking(ctx, bookingID, pgtype.UUID{}, true)
	default:
		releaseDep := pgtype.UUID{}
		if release {
			releaseDep = row.DepartureID
		}

		if _, err := s.store.AdminSetStatusAndRelease(ctx, bookingID, input.Status, note, releaseDep, row.PartySize); err != nil {
			return BookingDTO{}, fmt.Errorf("servicedir: admin set status: %w", err)
		}

		if releaseDep.Valid {
			s.invalidatePackageCache(row.ListingContentID)
		}
	}

	s.enqueueStatusChanged(row.ID, input.Status)

	if crossedConfirmed {
		s.enqueueMentorMeilisearch(row.ProviderUserID)
	}

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

// feeSettled is the org-visibility gate for paid bookings: money must
// have moved (paymentSettled) — or the fee was configured at zero, in
// which case no checkout exists and the booking lands in the inbox
// directly (mirrors the inbox query's `sb.fee_amount = 0` arm).
func feeSettled(row BookingRow) bool {
	if row.FeeAmount != nil && *row.FeeAmount == 0 {
		return true
	}

	return paymentSettled(row.PaymentStatus)
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

	window := 7
	if s.settings != nil {
		window = s.settings.GetIntOr(ctx, "service_packages.refund_window_days", 7)
	}

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
	s.enqueueStatusChangedAudience(bookingID, status, "")
}

// enqueueStatusChangedAudience enqueues the status_changed task with an
// audience hint — "org" notifies org members (booker-raised disputes);
// "provider" notifies the provider (booker-initiated cancel/complete);
// empty notifies the booker (the default).
func (s *Service) enqueueStatusChangedAudience(bookingID pgtype.UUID, status, audience string) {
	if s.enqueuer == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"booking_id": bookingID.String(),
		"status":     status,
		"audience":   audience,
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
