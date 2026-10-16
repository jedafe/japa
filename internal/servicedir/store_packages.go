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
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
// Package pricing tiers (replace-on-save)
// ---------------------------------------------------------------------------

// ReplacePackageTiers swaps the full tier set for a package inside one
// transaction — tiers[] is replace-on-save on create/edit, same pattern as
// ReplacePackageItems. position follows slice order (already stamped by the
// service layer's validatePackageTiers).
func (s *Store) ReplacePackageTiers(ctx context.Context, packageMetaID pgtype.UUID, tiers []gen.CreateServicePackageTierParams) error {
	return db.WithTx(ctx, s.pool.Writer, func(q *gen.Queries) error {
		if err := q.DeleteServicePackageTiers(ctx, packageMetaID); err != nil {
			return fmt.Errorf("servicedir: delete package tiers: %w", err)
		}

		for i := range tiers {
			tiers[i].ListingMetaID = packageMetaID
			if _, err := q.CreateServicePackageTier(ctx, tiers[i]); err != nil {
				return fmt.Errorf("servicedir: create package tier %d: %w", i, err)
			}
		}

		return nil
	})
}

// listPackageTiers returns the package's ACTIVE pricing tiers in position
// order (the public detail set). Inactive rows are editor-only and are
// fetched via ListAllServicePackageTiers when needed.
func (s *Store) listPackageTiers(ctx context.Context, packageMetaID pgtype.UUID) ([]gen.ServicePackageTier, error) {
	rows, err := s.read.ListServicePackageTiers(ctx, packageMetaID)
	if err != nil {
		return nil, fmt.Errorf("servicedir: list package tiers: %w", err)
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

// UpdatePackageDeparture edits one departure in place — keeps the row id
// so service_bookings.departure_id references stay attached (§111).
func (s *Store) UpdatePackageDeparture(ctx context.Context, arg gen.UpdateServicePackageDepartureParams) (gen.ServicePackageDeparture, error) {
	row, err := s.write.UpdateServicePackageDeparture(ctx, arg)
	if err != nil {
		return gen.ServicePackageDeparture{}, fmt.Errorf("servicedir: update package departure: %w", err)
	}

	return row, nil
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
//
// 000824: provider_user_id/destination_space_id/meta are the mentorship-
// unification columns — user-provider (mentorship) bookings carry
// provider_user_id instead of org_id (XOR-or-anonymized CHECK).
type BookingRow struct {
	ID                 pgtype.UUID
	ListingMetaID      pgtype.UUID
	DepartureID        pgtype.UUID
	BookerUserID       pgtype.UUID
	OrgID              pgtype.UUID
	ProviderUserID     pgtype.UUID
	DestinationSpaceID pgtype.UUID
	Meta               []byte
	Intent             string
	PartySize          int32
	PreferredDates     []byte
	Note               *string
	Status             string
	PurchaseMode       string
	PaymentID          pgtype.UUID
	TotalPrice         *int64
	FeeAmount          *int64
	FeeCurrency        *string
	AgentBalanceDue    *int64
	OrgResponseNote    *string
	DisputeReason      *string
	ExpiresAt          pgtype.Timestamptz
	ConfirmedAt        pgtype.Timestamptz
	FulfilledAt        pgtype.Timestamptz
	CancelledAt        pgtype.Timestamptz
	CancelReason       *string
	AnonymizedAt       pgtype.Timestamptz
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
	ListingTitle       string
	ListingSlug        string
	ListingKind        string
	// ListingContentID is the contents.id behind the listing — the
	// jc:svc_package cache key (only populated by the detail query).
	ListingContentID pgtype.UUID
	// PackageCapacity is the package-level capacity a NULL departure
	// capacity inherits (only populated by the detail query).
	PackageCapacity   *int32
	PaymentStatus     *string // joined payment row — nil when payment_id is unset
	PaymentProvider   *string // joined payment row — nil when payment_id is unset
	OrgName           *string
	OrgSlug           *string
	DepartureStartsOn pgtype.Date
	DepartureEndsOn   pgtype.Date
	BookerUsername    *string
	BookerDisplayName *string
	BookerAvatar      *string
	// Provider display joins (000824) — populated by the detail, by-org,
	// and by-provider list queries; nil on org-owned rows.
	ProviderUsername     *string
	ProviderDisplayName  *string
	ProviderAvatar       *string
	DestinationSpaceName *string

	// Marketplace booking snapshots (spec §2.3/§2.4). TierID/TierName/
	// TierPriceAmount freeze the selected pricing tier — TierName carries
	// the COALESCE(sb.tier_name, t.name) effective name on joined queries
	// so the snapshot wins after a tiers[] replace-set.
	TierID          pgtype.UUID
	TierName        *string
	TierPriceAmount *int64
	CouponID        pgtype.UUID
	CouponCode      *string
	DiscountAmount  *int64
}

// bookingFromGen projects a gen.ServiceBooking into the flat row shape.
func bookingFromGen(b gen.ServiceBooking) BookingRow {
	return BookingRow{
		ID: b.ID, ListingMetaID: b.ListingMetaID, DepartureID: b.DepartureID,
		BookerUserID: b.BookerUserID, OrgID: b.OrgID,
		ProviderUserID:     b.ProviderUserID,
		DestinationSpaceID: b.DestinationSpaceID, Meta: b.Meta,
		Intent:    b.Intent,
		PartySize: b.PartySize, PreferredDates: b.PreferredDates, Note: b.Note,
		Status: b.Status, PurchaseMode: b.PurchaseMode, PaymentID: b.PaymentID,
		TotalPrice: b.TotalPrice, FeeAmount: b.FeeAmount, FeeCurrency: b.FeeCurrency,
		AgentBalanceDue: b.AgentBalanceDue, OrgResponseNote: b.OrgResponseNote,
		DisputeReason: b.DisputeReason,
		ExpiresAt:     b.ExpiresAt, ConfirmedAt: b.ConfirmedAt, FulfilledAt: b.FulfilledAt,
		CancelledAt: b.CancelledAt, CancelReason: b.CancelReason,
		AnonymizedAt: b.AnonymizedAt, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		TierID: b.TierID, TierName: b.TierName, TierPriceAmount: b.TierPriceAmount,
		CouponID: b.CouponID, CouponCode: b.CouponCode, DiscountAmount: b.DiscountAmount,
	}
}

func bookingRowFromDetail(r gen.GetServiceBookingDetailRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID,
		ProviderUserID: r.ProviderUserID, DestinationSpaceID: r.DestinationSpaceID,
		Meta: r.Meta, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason: r.DisputeReason,
		ExpiresAt:     r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		TierID: r.TierID, TierName: r.TierName, TierPriceAmount: r.TierPriceAmount,
		CouponID: r.CouponID, CouponCode: r.CouponCode, DiscountAmount: r.DiscountAmount,
	})
	if r.EffectiveTierName != "" {
		row.TierName = strPtr(r.EffectiveTierName) // snapshot wins, live tier name is the fallback
	}

	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.PackageCapacity = r.PackageCapacity
	row.PaymentStatus = r.PaymentStatus
	row.PaymentProvider = r.PaymentProvider
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn
	row.BookerUsername = r.BookerUsername
	row.BookerDisplayName = r.BookerDisplayName
	row.BookerAvatar = r.BookerAvatar
	row.ProviderUsername = r.ProviderUsername
	row.ProviderDisplayName = r.ProviderDisplayName
	row.ProviderAvatar = r.ProviderAvatar
	row.DestinationSpaceName = r.DestinationSpaceName

	return row
}

func bookingRowFromUser(r gen.ListServiceBookingsByUserRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID,
		ProviderUserID: r.ProviderUserID, DestinationSpaceID: r.DestinationSpaceID,
		Meta: r.Meta, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason: r.DisputeReason,
		ExpiresAt:     r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		TierID: r.TierID, TierName: r.TierName, TierPriceAmount: r.TierPriceAmount,
		CouponID: r.CouponID, CouponCode: r.CouponCode, DiscountAmount: r.DiscountAmount,
	})
	if r.EffectiveTierName != "" {
		row.TierName = strPtr(r.EffectiveTierName)
	}

	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.PaymentStatus = r.PaymentStatus
	row.PaymentProvider = r.PaymentProvider
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn
	row.ProviderUsername = r.ProviderUsername
	row.ProviderDisplayName = r.ProviderDisplayName
	row.ProviderAvatar = r.ProviderAvatar
	row.DestinationSpaceName = r.DestinationSpaceName

	return row
}

func bookingRowFromOrg(r gen.ListServiceBookingsByOrgRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID,
		ProviderUserID: r.ProviderUserID, DestinationSpaceID: r.DestinationSpaceID,
		Meta: r.Meta, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason: r.DisputeReason,
		ExpiresAt:     r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		TierID: r.TierID, TierName: r.TierName, TierPriceAmount: r.TierPriceAmount,
		CouponID: r.CouponID, CouponCode: r.CouponCode, DiscountAmount: r.DiscountAmount,
	})
	if r.EffectiveTierName != "" {
		row.TierName = strPtr(r.EffectiveTierName)
	}

	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn
	row.BookerUsername = r.BookerUsername
	row.BookerDisplayName = r.BookerDisplayName
	row.BookerAvatar = r.BookerAvatar
	row.ProviderUsername = r.ProviderUsername
	row.ProviderDisplayName = r.ProviderDisplayName
	row.ProviderAvatar = r.ProviderAvatar
	row.DestinationSpaceName = r.DestinationSpaceName

	return row
}

// bookingRowFromProvider maps the provider-inbox query row (000824):
// ListServiceBookingsByProviderUser carries every join column the detail
// query has — the provider inbox shows the booker on each row.
func bookingRowFromProvider(r gen.ListServiceBookingsByProviderUserRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID,
		ProviderUserID: r.ProviderUserID, DestinationSpaceID: r.DestinationSpaceID,
		Meta: r.Meta, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason: r.DisputeReason,
		ExpiresAt:     r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		TierID: r.TierID, TierName: r.TierName, TierPriceAmount: r.TierPriceAmount,
		CouponID: r.CouponID, CouponCode: r.CouponCode, DiscountAmount: r.DiscountAmount,
	})
	if r.EffectiveTierName != "" {
		row.TierName = strPtr(r.EffectiveTierName)
	}

	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.PaymentStatus = r.PaymentStatus
	row.PaymentProvider = r.PaymentProvider
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn
	row.BookerUsername = r.BookerUsername
	row.BookerDisplayName = r.BookerDisplayName
	row.BookerAvatar = r.BookerAvatar
	row.ProviderUsername = r.ProviderUsername
	row.ProviderDisplayName = r.ProviderDisplayName
	row.ProviderAvatar = r.ProviderAvatar
	row.DestinationSpaceName = r.DestinationSpaceName

	return row
}

func bookingRowFromAdmin(r gen.AdminListServiceBookingsRow) BookingRow {
	row := bookingFromGen(gen.ServiceBooking{
		ID: r.ID, ListingMetaID: r.ListingMetaID, DepartureID: r.DepartureID,
		BookerUserID: r.BookerUserID, OrgID: r.OrgID,
		ProviderUserID: r.ProviderUserID, DestinationSpaceID: r.DestinationSpaceID,
		Meta: r.Meta, Intent: r.Intent,
		PartySize: r.PartySize, PreferredDates: r.PreferredDates, Note: r.Note,
		Status: r.Status, PurchaseMode: r.PurchaseMode, PaymentID: r.PaymentID,
		TotalPrice: r.TotalPrice, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency,
		AgentBalanceDue: r.AgentBalanceDue, OrgResponseNote: r.OrgResponseNote,
		DisputeReason: r.DisputeReason,
		ExpiresAt:     r.ExpiresAt, ConfirmedAt: r.ConfirmedAt, FulfilledAt: r.FulfilledAt,
		CancelledAt: r.CancelledAt, CancelReason: r.CancelReason,
		AnonymizedAt: r.AnonymizedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		TierID: r.TierID, TierName: r.TierName, TierPriceAmount: r.TierPriceAmount,
		CouponID: r.CouponID, CouponCode: r.CouponCode, DiscountAmount: r.DiscountAmount,
	})
	if r.EffectiveTierName != "" {
		row.TierName = strPtr(r.EffectiveTierName) // snapshot wins, live tier name is the fallback
	}

	row.ListingTitle = r.ListingTitle
	row.ListingSlug = r.ListingSlug
	row.ListingKind = r.ListingKind
	row.ListingContentID = r.ListingContentID
	row.PaymentStatus = r.PaymentStatus
	row.PaymentProvider = r.PaymentProvider
	row.OrgName = r.OrgName
	row.OrgSlug = r.OrgSlug
	row.DepartureStartsOn = r.DepartureStartsOn
	row.DepartureEndsOn = r.DepartureEndsOn
	row.BookerUsername = r.BookerUsername
	row.BookerDisplayName = r.BookerDisplayName
	row.BookerAvatar = r.BookerAvatar
	row.ProviderUsername = r.ProviderUsername
	row.ProviderDisplayName = r.ProviderDisplayName
	row.ProviderAvatar = r.ProviderAvatar
	row.DestinationSpaceName = r.DestinationSpaceName

	return row
}

// ---------------------------------------------------------------------------
// Bookings
// ---------------------------------------------------------------------------

// createBookingWithRedemption inserts the booking and — when redemption is
// non-nil — its coupon_redemptions row in one transaction (spec §2.3: the
// redemption is the authoritative record, so a failed insert must roll the
// booking back rather than silently lose the recorded discount).
// redemption.ServiceBookingID is stamped from the new booking row inside
// the tx.
//
// When guard is non-nil the coupon row is locked first and the cap/window
// rule set re-checked inside the same transaction: the priceQuote counts
// ran outside any lock, so without this two concurrent checkouts could
// both pass max_redemptions and both commit.
func (s *Store) createBookingWithRedemption(ctx context.Context, p gen.CreateServiceBookingParams, redemption *gen.CreateCouponRedemptionParams, guard *couponCommitGuard) (gen.ServiceBooking, error) {
	return db.WithTxResult(ctx, s.pool.Writer, func(q *gen.Queries) (gen.ServiceBooking, error) {
		if guard != nil {
			if err := checkCouponCapsTx(ctx, q, *guard); err != nil {
				return gen.ServiceBooking{}, err
			}
		}

		booking, err := q.CreateServiceBooking(ctx, p)
		if err != nil {
			return gen.ServiceBooking{}, fmt.Errorf("servicedir: create booking: %w", err)
		}

		if redemption != nil {
			redemption.ServiceBookingID = booking.ID
			if _, err := q.CreateCouponRedemption(ctx, *redemption); err != nil {
				return gen.ServiceBooking{}, fmt.Errorf("servicedir: create coupon redemption: %w", err)
			}
		}

		return booking, nil
	})
}

// couponCommitGuard carries the priceQuote resolution into the booking
// transaction so cap/window enforcement re-runs under the coupon row
// lock: concurrent checkouts against the same coupon serialize on the
// FOR UPDATE lock instead of oversubscribing max_redemptions /
// per_user_limit / first_time_only on a read-committed snapshot.
type couponCommitGuard struct {
	Code     string // original coupon code — the lock lookup is case-insensitive
	MetaID   pgtype.UUID
	BookerID pgtype.UUID
	OrgID    pgtype.UUID // listing's org, for the ownership + first-time checks
	Base     int64       // quoted base price — min_spend/discount recompute identically
	Kind     string      // listing kind, for the applies_to check
	Currency string      // quoted currency, for the unit-mismatch check
	Discount int64       // quoted discount — a changed coupon value aborts the commit
}

// checkCouponCapsTx re-validates the coupon rule set under the coupon row
// lock, using the tx-scoped queries so the counts observe every committed
// redemption up to the lock. Stateless checks (status/window, org
// ownership, applies_to, currency, min_spend, non-zero discount) mirror
// priceQuote so a coupon muted or re-scoped after the quote can't commit
// a stale discount; the live counters then enforce the caps.
func checkCouponCapsTx(ctx context.Context, q *gen.Queries, g couponCommitGuard) error {
	row, err := q.LockCouponForRedemption(ctx, gen.LockCouponForRedemptionParams{
		ListingMetaID: g.MetaID, Code: g.Code,
	})
	if err != nil {
		return fmt.Errorf("servicedir: lock coupon: %w", err)
	}

	c := mapListingCoupon(couponLinkRow{
		ID: row.ID, Title: row.Title, Code: row.Code, DiscountType: row.DiscountType,
		DiscountValue: row.DiscountValue, Currency: row.Currency,
		TermsURL: row.TermsUrl, ReferralURL: row.ReferralUrl, Status: row.Status,
		ValidFrom: row.ValidFrom, ExpiresAt: row.ExpiresAt, AppliesTo: row.AppliesTo,
		MaxRedemptions: row.MaxRedemptions, PerUserLimit: row.PerUserLimit,
		MinSpend: row.MinSpend, FirstTimeOnly: row.FirstTimeOnly,
		OrganizationID: row.OrganizationID, Position: row.Position,
		Overrides: row.Overrides,
	})

	// Status + validity window — same rule as priceQuote (an inactive,
	// not-yet-valid, or expired coupon is invalid).
	now := time.Now()
	if c.Status != "active" ||
		(row.ValidFrom.Valid && row.ValidFrom.Time.After(now)) ||
		(row.ExpiresAt.Valid && !row.ExpiresAt.Time.After(now)) {
		return ErrInvalidCoupon
	}

	if c.OrgID == "" || c.OrgID != g.OrgID.String() {
		return ErrInvalidCoupon
	}

	if c.AppliesTo != "both" && c.AppliesTo != g.Kind {
		return ErrCouponNotApplicable
	}

	if couponCurrencyMismatch(c, g.Currency) {
		return ErrCouponNotApplicable
	}

	if belowMinSpend(g.Base, c.MinSpend) {
		return ErrCouponMinSpend
	}

	discount := g.Base - applyDiscount(g.Base, c.DiscountType, c.DiscountValue)
	if discount <= 0 || discount != g.Discount {
		return ErrInvalidCoupon
	}

	if c.MaxRedemptions != nil {
		n, err := q.CountCouponRedemptions(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("servicedir: count coupon redemptions: %w", err)
		}

		if n >= *c.MaxRedemptions {
			return ErrCouponCapReached
		}
	}

	if c.PerUserLimit != nil {
		n, err := q.CountCouponRedemptionsByUser(ctx, gen.CountCouponRedemptionsByUserParams{
			CouponID: row.ID, UserID: g.BookerID,
		})
		if err != nil {
			return fmt.Errorf("servicedir: count coupon redemptions by user: %w", err)
		}

		if n >= *c.PerUserLimit {
			return ErrCouponUserLimit
		}
	}

	if c.FirstTimeOnly {
		n, err := q.CountServiceBookingsByUserOrg(ctx, gen.CountServiceBookingsByUserOrgParams{
			UserID: g.BookerID, OrgID: g.OrgID,
		})
		if err != nil {
			return fmt.Errorf("servicedir: count bookings by user+org: %w", err)
		}

		if n > 0 {
			return ErrCouponFirstTime
		}
	}

	return nil
}

// getPackageTier returns a service_package_tiers row scoped to the
// listing's meta id — the scoping IS the foreign-tier check (a tier id
// belonging to another listing resolves to pgx.ErrNoRows). The caller
// checks IsActive.
func (s *Store) getPackageTier(ctx context.Context, tierID, metaID pgtype.UUID) (gen.ServicePackageTier, error) {
	row, err := s.read.GetServicePackageTier(ctx, gen.GetServicePackageTierParams{
		ID: tierID, ListingMetaID: metaID,
	})
	if err != nil {
		return gen.ServicePackageTier{}, fmt.Errorf("servicedir: get package tier: %w", err)
	}

	return row, nil
}

// countBookingsByUserOrg returns the user's live bookings with the org
// (cancelled/declined/refunded excluded by the query) — the coupon
// first_time_only check.
func (s *Store) countBookingsByUserOrg(ctx context.Context, userID, orgID pgtype.UUID) (int64, error) {
	n, err := s.read.CountServiceBookingsByUserOrg(ctx, gen.CountServiceBookingsByUserOrgParams{
		UserID: userID, OrgID: orgID,
	})
	if err != nil {
		return 0, fmt.Errorf("servicedir: count bookings by user+org: %w", err)
	}

	return n, nil
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
	Kind   string // optional ?kind= listing_kind filter (000824)
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

	if f.Kind != "" {
		params.Kind = &f.Kind
	}

	rows, err := s.read.ListServiceBookingsByUser(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: list bookings by user: %w", err)
	}

	total, err := s.read.CountServiceBookingsByUser(ctx, gen.CountServiceBookingsByUserParams{
		BookerUserID: userID, Status: params.Status, Intent: params.Intent, Kind: params.Kind,
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

// BookingsByProviderFilter holds filters for the user-provider inbox
// (000824: GET /me/service-bookings?role=provider).
type BookingsByProviderFilter struct {
	Status string
	Intent string
	Kind   string // optional ?kind= listing_kind filter
	Limit  int32
	Offset int32
}

// ListBookingsByProviderUser returns the caller's provider-side inbox —
// bookings where provider_user_id = the caller (mentorship requests).
// Same money-moved visibility gate as the org inbox (enforced in SQL).
func (s *Store) ListBookingsByProviderUser(ctx context.Context, providerID pgtype.UUID, f BookingsByProviderFilter) ([]BookingRow, int64, error) {
	params := gen.ListServiceBookingsByProviderUserParams{
		ProviderUserID: providerID, PageLimit: f.Limit, PageOffset: f.Offset,
	}
	if f.Status != "" {
		params.Status = &f.Status
	}

	if f.Intent != "" {
		params.Intent = &f.Intent
	}

	if f.Kind != "" {
		params.Kind = &f.Kind
	}

	rows, err := s.read.ListServiceBookingsByProviderUser(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: list bookings by provider: %w", err)
	}

	total, err := s.read.CountServiceBookingsByProviderUser(ctx, gen.CountServiceBookingsByProviderUserParams{
		ProviderUserID: providerID, Status: params.Status, Intent: params.Intent, Kind: params.Kind,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: count bookings by provider: %w", err)
	}

	out := make([]BookingRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, bookingRowFromProvider(r))
	}

	return out, total, nil
}

// HasOpenMentorshipBooking reports whether the booker already holds an
// open request (pending or confirmed) against the MENTOR — the dedupe is
// keyed on (booker, provider_user_id), the same pair the
// idx_svc_bookings_mentorship_open_uniq index enforces (000828), so a
// re-minted shadow meta can't re-open the gate. Replaces the dropped
// GetActiveOrPendingConnection.
func (s *Store) HasOpenMentorshipBooking(ctx context.Context, bookerID, providerID pgtype.UUID) (bool, error) {
	_, err := s.read.GetOpenMentorshipBooking(ctx, gen.GetOpenMentorshipBookingParams{
		BookerUserID: bookerID, ProviderUserID: providerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("servicedir: open mentorship booking probe: %w", err)
	}

	return true, nil
}

// MentorCapacity is the provider-side capacity pair from user_profiles —
// active_mentee_count is kept in sync with confirmed mentorship bookings
// by the trg_service_booking_mentee_count trigger (000824). IsMentor lets
// the create path re-verify the mentor flag so a stale/orphaned shadow
// can't keep minting requests.
type MentorCapacity struct {
	Capacity *int16
	Active   *int16
	IsMentor *bool
}

// GetMentorCapacity loads the provider's mentor_capacity vs
// active_mentee_count for the create-booking capacity check, plus the
// is_mentor flag for the orphan-shadow re-verify.
func (s *Store) GetMentorCapacity(ctx context.Context, userID pgtype.UUID) (MentorCapacity, error) {
	row, err := s.read.GetMentorCapacity(ctx, userID)
	if err != nil {
		return MentorCapacity{}, fmt.Errorf("servicedir: get mentor capacity: %w", err)
	}

	return MentorCapacity{Capacity: row.MentorCapacity, Active: row.ActiveMenteeCount, IsMentor: row.IsMentor}, nil
}

// SpaceExists reports whether the destination space id resolves AND is
// live — the mentorship booking's destination_space_id input validation
// (000824) uses SpaceExistsLive so a soft-deleted space can't anchor a
// new booking (the all-rows SpaceExists stays reserved for the spaces
// hard-delete probe).
func (s *Store) SpaceExists(ctx context.Context, spaceID pgtype.UUID) (bool, error) {
	exists, err := s.read.SpaceExistsLive(ctx, spaceID)
	if err != nil {
		return false, fmt.Errorf("servicedir: space exists probe: %w", err)
	}

	return exists, nil
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
// Kind/ProviderUserID/BookerUserID are the mentorship-compat filters
// (000824): the legacy admin connections list filters kind='mentorship'
// by mentor (provider) and mentee (booker).
type BookingsAdminFilter struct {
	Status         string
	Intent         string
	OrgID          pgtype.UUID
	Kind           string
	ProviderUserID pgtype.UUID
	BookerUserID   pgtype.UUID
	Limit          int32
	Offset         int32
}

// AdminListBookings returns all bookings (any payment state) for admins.
func (s *Store) AdminListBookings(ctx context.Context, f BookingsAdminFilter) ([]BookingRow, int64, error) {
	params := gen.AdminListServiceBookingsParams{
		OrgID:          f.OrgID,
		ProviderUserID: f.ProviderUserID, BookerUserID: f.BookerUserID,
		PageLimit: f.Limit, PageOffset: f.Offset,
	}
	if f.Status != "" {
		params.Status = &f.Status
	}

	if f.Intent != "" {
		params.Intent = &f.Intent
	}

	if f.Kind != "" {
		params.Kind = &f.Kind
	}

	rows, err := s.read.AdminListServiceBookings(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("servicedir: admin list bookings: %w", err)
	}

	total, err := s.read.AdminCountServiceBookings(ctx, gen.AdminCountServiceBookingsParams{
		Status: params.Status, Intent: params.Intent, OrgID: f.OrgID,
		Kind: params.Kind, ProviderUserID: f.ProviderUserID, BookerUserID: f.BookerUserID,
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
