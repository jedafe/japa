// Package servicedir — pricing.go implements the Services Marketplace
// checkout quote path: package-tier base resolution, listing-coupon
// redemption-rule enforcement, discount math, and the §111 platform fee
// on the discounted total. Consumed by CreateBooking (intent='booking')
// and by GET /services/:id/price-preview.
//
// The discount is agent-funded (spec §2.3): the platform fee is computed
// on the discounted total and agent_balance_due = total - fee.
//
// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§2.3, §2.4)
package servicedir

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
)

// Quote is the resolved pricing snapshot for a booking — base price
// (listing price or the selected tier's), the applied coupon discount,
// the platform fee on the discounted total, and the agent-collected
// balance. Tier*/Coupon* carry the ids/codes so the caller can stamp the
// booking snapshot columns without re-resolving.
type Quote struct {
	Base       int64
	Discount   int64
	Total      int64
	Fee        int64
	Balance    int64
	Currency   string
	TierID     pgtype.UUID // invalid when no tier was selected
	TierName   string
	CouponID   pgtype.UUID // invalid when no coupon was applied
	CouponCode string      // canonical (DB) code casing
}

// PricePreviewDTO is the GET /services/:id/price-preview payload. On a
// coupon rejection the handler returns 200 with CouponValid=false and
// ErrorCode set — Base/Total/Fee/BalanceDue still carry the undiscounted
// quote so the booking modal can render the pre-coupon totals.
type PricePreviewDTO struct {
	Base        int64  `json:"base"`
	Discount    int64  `json:"discount"`
	Total       int64  `json:"total"`
	Fee         int64  `json:"fee"`
	BalanceDue  int64  `json:"balance_due"`
	Currency    string `json:"currency"`
	CouponValid bool   `json:"coupon_valid"`
	ErrorCode   string `json:"error_code,omitempty"`
}

// applyDiscount returns the post-discount price (minor units), floored at
// zero. 'percent' is a 0-100 rate (>100 floors at free); 'fixed' is a
// minor-unit amount. Non-positive, non-finite, or unknown discount values
// leave the price unchanged — a coupon that doesn't reduce the price is a
// no-op, not an error.
func applyDiscount(base int64, dtype string, dval float64) int64 {
	if base <= 0 || dval <= 0 || math.IsNaN(dval) || math.IsInf(dval, 0) {
		return base
	}

	var out int64
	switch dtype {
	case "percent":
		if dval >= 100 {
			return 0
		}
		out = int64(math.Round(float64(base) * (100 - dval) / 100))
	case "fixed":
		out = base - int64(math.Round(dval))
	default:
		return base
	}

	if out < 0 {
		return 0
	}

	return out
}

// belowMinSpend reports whether the booking base is under the coupon's
// min_spend — a nil limit always passes. Boundary: base == min_spend is
// redeemable.
func belowMinSpend(base int64, minSpend *int64) bool {
	return minSpend != nil && base < *minSpend
}

// couponCurrencyMismatch reports whether a coupon denominated in a
// different currency than the quote carries an amount that is read
// unit-blind — a 'fixed' discount_value or a min_spend threshold. A USD
// fixed coupon on an NGN-priced quote would silently mis-discount (and
// its min_spend would compare against the wrong unit). Percent coupons
// with no min_spend are unit-free and skip the check; an unset coupon
// currency matches anything (org coupons may not record one).
func couponCurrencyMismatch(c ListingCoupon, quoteCurrency string) bool {
	if c.Currency == "" || (c.DiscountType != "fixed" && c.MinSpend == nil) {
		return false
	}

	return !strings.EqualFold(c.Currency, quoteCurrency)
}

// isCouponQuoteError reports whether err is one of the coupon redemption
// sentinels — the price-preview endpoint maps these to a 200
// coupon_valid:false payload rather than an HTTP error.
func isCouponQuoteError(err error) bool {
	return errors.Is(err, ErrInvalidCoupon) ||
		errors.Is(err, ErrCouponNotApplicable) ||
		errors.Is(err, ErrCouponMinSpend) ||
		errors.Is(err, ErrCouponCapReached) ||
		errors.Is(err, ErrCouponUserLimit) ||
		errors.Is(err, ErrCouponFirstTime)
}

// couponQuoteErrorCode maps a coupon sentinel to its snake_case API code —
// the same codes writeServiceError emits, surfaced in the price-preview
// payload's error_code field.
func couponQuoteErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrCouponNotApplicable):
		return "coupon_not_applicable"
	case errors.Is(err, ErrCouponMinSpend):
		return "coupon_min_spend"
	case errors.Is(err, ErrCouponCapReached):
		return "coupon_cap_reached"
	case errors.Is(err, ErrCouponUserLimit):
		return "coupon_user_limit"
	case errors.Is(err, ErrCouponFirstTime):
		return "coupon_first_time_only"
	default:
		return "invalid_coupon"
	}
}

// priceQuote resolves the booking price for (listing, tier, coupon):
//
//	base     = tier price when tierID selects an active tier of this
//	           listing (else ErrInvalidTier), otherwise the listing's
//	           price_amount (ErrMissingPackagePrice when unset). Any
//	           tier_id reference is ErrInvalidTier while the
//	           service_packages.tiers_enabled flag is off — the flag
//	           gates the booking-time reference too, not just the
//	           editor/detail payloads (service_tiers.go).
//	coupon   = attached coupon resolved by code, checked in order:
//	           status+validity window → org ownership → applies_to →
//	           min_spend → max_redemptions → per_user_limit →
//	           first_time_only → discount. Any coupon_code is
//	           ErrInvalidCoupon while the service_coupons.enabled
//	           flag is off — the flag gates redemption too, not just
//	           the attach API and public payloads (coupons.go).
//	total    = base - discount (floored at 0)
//	fee      = computeFee(total); balance = total - fee
//
// Read-only — safe to run for the price-preview endpoint.
func (s *Service) priceQuote(ctx context.Context, listing ServiceRow, bookerID pgtype.UUID, tierIDRaw, couponCode string) (Quote, error) {
	var q Quote

	if tierIDRaw != "" {
		// The tiers flag gates the booking tier_id reference: with tiers
		// dark-launched a tier reference is invalid, not silently ignored.
		// Checked before the store lookup so a flag-off request never
		// reaches the database.
		if !s.TiersEnabled(ctx) {
			return Quote{}, ErrInvalidTier
		}

		var tierID pgtype.UUID
		if err := tierID.Scan(tierIDRaw); err != nil {
			return Quote{}, ErrInvalidTier
		}

		tier, err := s.store.getPackageTier(ctx, tierID, listing.MetaID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Quote{}, ErrInvalidTier
			}

			return Quote{}, fmt.Errorf("servicedir: pricing: get tier: %w", err)
		}

		if !tier.IsActive {
			return Quote{}, ErrInvalidTier
		}

		q.Base = tier.PriceAmount
		q.TierID = tier.ID
		q.TierName = tier.Name
		q.Currency = strOrEmpty(tier.PriceCurrency)
	} else {
		if listing.PriceAmount == nil {
			return Quote{}, ErrMissingPackagePrice
		}

		q.Base = *listing.PriceAmount
	}

	if q.Currency == "" && listing.PriceCurrency != nil {
		q.Currency = *listing.PriceCurrency
	}

	couponCode = strings.TrimSpace(couponCode)
	if couponCode != "" {
		// The coupons flag gates redemption, not just the attach API and
		// public payloads: with coupons dark-launched a coupon_code is
		// invalid, not silently ignored. Checked before the store lookup
		// so a flag-off request never reaches the database.
		if !s.CouponsEnabled(ctx) {
			return Quote{}, ErrInvalidCoupon
		}

		lc, err := s.store.getListingCouponByCode(ctx, listing.MetaID, couponCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Quote{}, ErrInvalidCoupon
			}

			return Quote{}, fmt.Errorf("servicedir: pricing: get coupon: %w", err)
		}

		// Status + validity window — an inactive, not-yet-valid, or
		// expired coupon is simply invalid (same code as unknown).
		now := time.Now()
		if lc.Status != "active" ||
			(lc.ValidFrom.Valid && lc.ValidFrom.Time.After(now)) ||
			(lc.ExpiresAt.Valid && !lc.ExpiresAt.Time.After(now)) {
			return Quote{}, ErrInvalidCoupon
		}

		// Only the listing's own org coupons redeem on it — a platform or
		// foreign-org code attached in error reads as invalid.
		if lc.OrganizationID != listing.OrgID {
			return Quote{}, ErrInvalidCoupon
		}

		// The DTO applies the service_listing_coupons.overrides jsonb —
		// discount_value/min_spend may be re-scoped per listing.
		c := mapListingCoupon(lc)

		if c.AppliesTo != "both" && c.AppliesTo != listing.ListingKind {
			return Quote{}, ErrCouponNotApplicable
		}

		// Currency guard — fixed amounts and min_spend thresholds are
		// denominated in the coupon's currency; a unit mismatch reads as
		// not applicable rather than mis-discounting the quote.
		if couponCurrencyMismatch(c, q.Currency) {
			return Quote{}, ErrCouponNotApplicable
		}

		if belowMinSpend(q.Base, c.MinSpend) {
			return Quote{}, ErrCouponMinSpend
		}

		if c.MaxRedemptions != nil {
			n, err := s.store.countCouponRedemptions(ctx, lc.ID)
			if err != nil {
				return Quote{}, err
			}

			if n >= *c.MaxRedemptions {
				return Quote{}, ErrCouponCapReached
			}
		}

		if c.PerUserLimit != nil {
			n, err := s.store.countCouponRedemptionsByUser(ctx, lc.ID, bookerID)
			if err != nil {
				return Quote{}, err
			}

			if n >= *c.PerUserLimit {
				return Quote{}, ErrCouponUserLimit
			}
		}

		if c.FirstTimeOnly {
			n, err := s.store.countBookingsByUserOrg(ctx, bookerID, listing.OrgID)
			if err != nil {
				return Quote{}, err
			}

			if n > 0 {
				return Quote{}, ErrCouponFirstTime
			}
		}

		q.Discount = q.Base - applyDiscount(q.Base, c.DiscountType, c.DiscountValue)

		// A coupon that reduces nothing (unknown discount type, zero
		// value) must not burn a redemption row or stamp the booking —
		// reject it so the booker sees invalid_coupon instead of a
		// phantom "applied" discount of zero.
		if q.Discount <= 0 {
			return Quote{}, ErrInvalidCoupon
		}

		q.CouponID = lc.ID
		q.CouponCode = c.Code
	}

	q.Total = q.Base - q.Discount
	q.Fee = s.computeFee(ctx, q.Total)
	q.Balance = q.Total - q.Fee

	return q, nil
}

// PricePreview computes the live booking quote for the price-preview
// endpoint (GET /services/:id/price-preview?tier_id=&coupon_code=). Gated
// on service_packages.enabled + the listing being bookable, same as
// CreateBooking — a preview on a non-bookable listing would quote a price
// the booker can't actually pay.
func (s *Service) PricePreview(ctx context.Context, bookerID, listingContentID pgtype.UUID, tierID, couponCode string) (Quote, error) {
	if !s.PackagesEnabled(ctx) {
		return Quote{}, ErrPackagesDisabled
	}

	listing, err := s.store.GetByContentID(ctx, listingContentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Quote{}, pgx.ErrNoRows
		}

		return Quote{}, fmt.Errorf("servicedir: price preview: get listing: %w", err)
	}

	if listing.Status != content.StatusPublished {
		return Quote{}, ErrBookingUnavailable
	}

	// Only paid listings produce a real quote — a preview on an inquiry or
	// external listing would quote a price the booker cannot pay. Mirrors
	// CreateBooking's purchase_mode gate (service_bookings.go).
	switch listing.PurchaseMode {
	case PurchaseModeExternal:
		return Quote{}, ErrBookingUnavailable
	case PurchaseModePaid:
	default: // inquiry
		return Quote{}, ErrInvalidIntent
	}

	return s.priceQuote(ctx, listing, bookerID, tierID, couponCode)
}
