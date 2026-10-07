// pricing_test.go — unit coverage for the Services Marketplace quote math:
// the applyDiscount table (percent/fixed, floor-at-zero), the min_spend
// boundary, and the coupon sentinel → error_code mapping. Store-backed
// priceQuote paths are covered by integration tests.
//
// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§2.3)
package servicedir

import (
	"errors"
	"math"
	"testing"
)

func TestApplyDiscount(t *testing.T) {
	cases := []struct {
		name  string
		dtype string
		val   float64
		price int64
		want  int64
	}{
		{"percent 10%", "percent", 10, 100_000, 90_000},
		{"percent 100%", "percent", 100, 100_000, 0},
		{"percent floor", "percent", 150, 100_000, 0},
		{"fixed", "fixed", 25_000, 100_000, 75_000},
		{"fixed over price", "fixed", 150_000, 100_000, 0},

		// Boundary + degenerate cases.
		{"fixed exact price", "fixed", 100_000, 100_000, 0},
		{"percent zero", "percent", 0, 100_000, 100_000},
		{"fixed zero", "fixed", 0, 100_000, 100_000},
		{"negative percent is a no-op", "percent", -10, 100_000, 100_000},
		{"negative fixed is a no-op", "fixed", -5_000, 100_000, 100_000},
		{"fractional percent rounds", "percent", 33.3, 100, 67},
		{"zero base stays zero", "percent", 10, 0, 0},
		{"unknown type is a no-op", "free_shipping", 10, 100_000, 100_000},
		{"empty type is a no-op", "", 10, 100_000, 100_000},
		{"NaN value is a no-op", "percent", math.NaN(), 100_000, 100_000},
		{"+Inf value is a no-op", "percent", math.Inf(1), 100_000, 100_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyDiscount(tc.price, tc.dtype, tc.val); got != tc.want {
				t.Fatalf("applyDiscount(%d, %q, %v) = %d, want %d",
					tc.price, tc.dtype, tc.val, got, tc.want)
			}
		})
	}
}

func TestBelowMinSpend(t *testing.T) {
	min := int64(50_000)

	t.Run("nil min_spend always passes", func(t *testing.T) {
		if belowMinSpend(0, nil) {
			t.Fatal("expected nil min_spend to never block")
		}
	})

	t.Run("base below min_spend blocks", func(t *testing.T) {
		if !belowMinSpend(49_999, &min) {
			t.Fatal("expected base < min_spend to block")
		}
	})

	t.Run("base equal to min_spend is redeemable", func(t *testing.T) {
		if belowMinSpend(50_000, &min) {
			t.Fatal("expected base == min_spend to pass")
		}
	})

	t.Run("base above min_spend is redeemable", func(t *testing.T) {
		if belowMinSpend(50_001, &min) {
			t.Fatal("expected base > min_spend to pass")
		}
	})
}

func TestCouponQuoteErrorCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"invalid", ErrInvalidCoupon, "invalid_coupon"},
		{"not applicable", ErrCouponNotApplicable, "coupon_not_applicable"},
		{"min spend", ErrCouponMinSpend, "coupon_min_spend"},
		{"cap reached", ErrCouponCapReached, "coupon_cap_reached"},
		{"user limit", ErrCouponUserLimit, "coupon_user_limit"},
		{"first time", ErrCouponFirstTime, "coupon_first_time_only"},
		{"wrapped sentinel", errors.Join(errors.New("x"), ErrCouponMinSpend), "coupon_min_spend"},
		{"non-coupon falls back", errors.New("boom"), "invalid_coupon"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := couponQuoteErrorCode(tc.err); got != tc.want {
				t.Fatalf("couponQuoteErrorCode(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsCouponQuoteError(t *testing.T) {
	for _, err := range []error{
		ErrInvalidCoupon, ErrCouponNotApplicable, ErrCouponMinSpend,
		ErrCouponCapReached, ErrCouponUserLimit, ErrCouponFirstTime,
	} {
		if !isCouponQuoteError(err) {
			t.Errorf("expected %v to be a coupon quote error", err)
		}
	}

	// Tier and non-coupon failures are hard errors on the preview endpoint.
	for _, err := range []error{ErrInvalidTier, ErrPackagesDisabled, errors.New("boom")} {
		if isCouponQuoteError(err) {
			t.Errorf("expected %v to NOT be a coupon quote error", err)
		}
	}
}
