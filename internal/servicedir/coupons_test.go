// coupons_test.go — unit coverage for the listing-coupon pure paths that
// don't need a database: the join-row → DTO mapper (incl. overrides jsonb
// re-scoping), the public JSON contract, and the feature flag gate.
//
// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md
package servicedir

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func uuidByte(b byte) pgtype.UUID {
	var id pgtype.UUID
	id.Bytes[0] = b
	id.Valid = true

	return id
}

func TestMapListingCoupon(t *testing.T) {
	code := "SAVE10"
	dtype := "percent"
	currency := "USD"
	terms := "https://example.com/terms"
	ref := "https://example.com/ref"
	maxR := int32(50)
	perUser := int32(2)
	minSpend := int64(10_000)
	expires := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	num := pgtype.Numeric{}
	if err := num.Scan("25"); err != nil {
		t.Fatalf("scan numeric: %v", err)
	}

	row := couponLinkRow{
		ID:             uuidByte(1),
		Title:          "Launch deal",
		Code:           &code,
		DiscountType:   &dtype,
		DiscountValue:  num,
		Currency:       &currency,
		TermsURL:       &terms,
		ReferralURL:    &ref,
		Status:         "active",
		ExpiresAt:      pgtype.Timestamptz{Time: expires, Valid: true},
		AppliesTo:      "both",
		MaxRedemptions: &maxR,
		PerUserLimit:   &perUser,
		MinSpend:       &minSpend,
		FirstTimeOnly:  true,
		OrganizationID: uuidByte(2),
		Position:       3,
	}

	c := mapListingCoupon(row)

	if c.ID == "" || c.Code != "SAVE10" || c.DiscountType != "percent" || c.DiscountValue != 25 {
		t.Fatalf("bad mapping: %+v", c)
	}

	if c.Currency != "USD" || c.TermsURL != terms || c.ReferralURL != ref || c.AppliesTo != "both" {
		t.Fatalf("bad mapping: %+v", c)
	}

	if c.ExpiresAt != expires.Format(time.RFC3339) {
		t.Fatalf("expires_at = %q", c.ExpiresAt)
	}

	if c.MaxRedemptions == nil || *c.MaxRedemptions != 50 ||
		c.PerUserLimit == nil || *c.PerUserLimit != 2 ||
		c.MinSpend == nil || *c.MinSpend != 10_000 {
		t.Fatalf("rule fields not carried: %+v", c)
	}

	if !c.FirstTimeOnly || c.Position != 3 || c.OrgID == "" {
		t.Fatalf("internal fields not carried: %+v", c)
	}
}

func TestMapListingCouponNilSafe(t *testing.T) {
	// code/discount_value/etc are nullable on coupons — a NULL set must
	// not panic and must produce empty values.
	c := mapListingCoupon(couponLinkRow{
		ID:        uuidByte(1),
		Title:     "bare",
		Status:    "active",
		AppliesTo: "service",
	})

	if c.Code != "" || c.DiscountType != "" || c.DiscountValue != 0 || c.ExpiresAt != "" {
		t.Fatalf("expected empty optional fields: %+v", c)
	}

	if c.MaxRedemptions != nil || c.PerUserLimit != nil || c.MinSpend != nil {
		t.Fatalf("expected nil rule fields: %+v", c)
	}
}

func TestMapListingCouponOverrides(t *testing.T) {
	num := pgtype.Numeric{}
	if err := num.Scan("10"); err != nil {
		t.Fatalf("scan numeric: %v", err)
	}

	minSpend := int64(5_000)

	row := couponLinkRow{
		ID:            uuidByte(1),
		Title:         "scoped",
		DiscountValue: num,
		MinSpend:      &minSpend,
		Status:        "active",
		AppliesTo:     "both",
		Overrides:     []byte(`{"discount_value": 30, "min_spend": 2000, "ignored_key": true}`),
	}

	c := mapListingCoupon(row)

	if c.DiscountValue != 30 {
		t.Fatalf("overrides.discount_value not applied: %v", c.DiscountValue)
	}

	if c.MinSpend == nil || *c.MinSpend != 2000 {
		t.Fatalf("overrides.min_spend not applied: %+v", c.MinSpend)
	}
}

func TestMapListingCouponBadOverrides(t *testing.T) {
	num := pgtype.Numeric{}
	if err := num.Scan("10"); err != nil {
		t.Fatalf("scan numeric: %v", err)
	}

	// A malformed overrides blob must not corrupt the base values.
	c := mapListingCoupon(couponLinkRow{
		ID:            uuidByte(1),
		DiscountValue: num,
		Overrides:     []byte(`{not json`),
	})

	if c.DiscountValue != 10 {
		t.Fatalf("bad overrides clobbered discount_value: %v", c.DiscountValue)
	}
}

func TestListingCouponPublicJSONShape(t *testing.T) {
	// The public coupons[] contract is exactly these keys — internal rule
	// fields must not leak into the payload.
	c := ListingCoupon{
		ID: "id", Title: "t", Code: "C", DiscountType: "percent",
		DiscountValue: 10, Currency: "USD", AppliesTo: "both",
		Status: "active", FirstTimeOnly: true, OrgID: "org", Position: 1,
	}

	var m map[string]any

	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := []string{
		"id", "title", "code", "discount_type", "discount_value",
		"currency", "expires_at", "terms_url", "referral_url", "applies_to",
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in public payload", k)
		}
	}

	for _, k := range []string{
		"status", "max_redemptions", "per_user_limit",
		"min_spend", "first_time_only", "org_id", "position",
	} {
		if _, ok := m[k]; ok {
			t.Errorf("internal key %q leaked into public payload", k)
		}
	}
}

func TestCouponsEnabled(t *testing.T) {
	ctx := t.Context()

	t.Run("flag on via settings", func(t *testing.T) {
		s := &Service{settings: fakeSettings{"service_coupons.enabled": "true"}}
		if !s.CouponsEnabled(ctx) {
			t.Fatal("expected enabled")
		}
	})

	t.Run("flag off via settings", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}
		if s.CouponsEnabled(ctx) {
			t.Fatal("expected disabled")
		}
	})

	t.Run("nil settings is disabled", func(t *testing.T) {
		s := &Service{}
		if s.CouponsEnabled(ctx) {
			t.Fatal("expected disabled")
		}
	})

	t.Run("checker wins over settings", func(t *testing.T) {
		s := &Service{
			settings: fakeSettings{"service_coupons.enabled": "false"},
			checker:  fakeChecker{"service_coupons.enabled": true},
		}
		if !s.CouponsEnabled(ctx) {
			t.Fatal("checker should take precedence")
		}
	})
}

// fakeChecker is an in-memory FeatureChecker for unit tests.
type fakeChecker map[string]bool

func (f fakeChecker) IsEnabled(_ context.Context, key string) bool {
	return f[key]
}
