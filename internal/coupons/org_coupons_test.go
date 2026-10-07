package coupons

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

func orgCouponNum(v string) *pgtype.Numeric {
	n := &pgtype.Numeric{}
	if err := n.Scan(v); err != nil {
		panic(err)
	}
	return n
}

func TestValidateOrgCouponInput(t *testing.T) {
	strp := func(s string) *string { return &s }
	i32 := func(v int32) *int32 { return &v }
	i64 := func(v int64) *int64 { return &v }
	validFrom := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	expiresAfter := pgtype.Timestamptz{Time: validFrom.Time.Add(24 * time.Hour), Valid: true}
	expiresBefore := pgtype.Timestamptz{Time: validFrom.Time.Add(-time.Hour), Valid: true}

	base := func() OrgCouponInput {
		return OrgCouponInput{
			Title:         "10% off services",
			Code:          "SAVE10",
			DiscountType:  string(DiscountTypePercent),
			DiscountValue: orgCouponNum("10"),
		}
	}

	cases := []struct {
		name    string
		mutate  func(*OrgCouponInput)
		wantErr error // nil → expect success
	}{
		{"valid percent", nil, nil},
		{"valid percent boundary 100", func(i *OrgCouponInput) { i.DiscountValue = orgCouponNum("100") }, nil},
		{"valid fixed", func(i *OrgCouponInput) {
			i.DiscountType = string(DiscountTypeFixed)
			i.DiscountValue = orgCouponNum("500")
		}, nil},
		{"valid no discount", func(i *OrgCouponInput) {
			i.DiscountType = ""
			i.DiscountValue = nil
		}, nil},
		{"lowercase code normalized", func(i *OrgCouponInput) { i.Code = "save10" }, nil},
		{"mixed separators ok", func(i *OrgCouponInput) { i.Code = "SAVE-10_NOW" }, nil},
		{"valid applies_to each", func(i *OrgCouponInput) { i.AppliesTo = strp("package") }, nil},
		{"valid window", func(i *OrgCouponInput) {
			i.ValidFrom = &validFrom
			i.ExpiresAt = &expiresAfter
		}, nil},

		{"missing title", func(i *OrgCouponInput) { i.Title = "  " }, ErrInvalidCouponInput},
		{"missing code", func(i *OrgCouponInput) { i.Code = "" }, ErrInvalidCouponInput},
		{"code too short", func(i *OrgCouponInput) { i.Code = "A1" }, ErrInvalidCouponInput},
		{"code too long", func(i *OrgCouponInput) {
			i.Code = "ABCDEFGHIJKLMNOPQRSTUVWXYZ01234567" // 33 chars
		}, ErrInvalidCouponInput},
		{"code bad chars", func(i *OrgCouponInput) { i.Code = "SAVE!10" }, ErrInvalidCouponInput},
		{"code spaces", func(i *OrgCouponInput) { i.Code = "SAVE 10" }, ErrInvalidCouponInput},

		{"bad discount_type", func(i *OrgCouponInput) { i.DiscountType = "bogus" }, ErrInvalidCouponInput},
		{"free_shipping rejected", func(i *OrgCouponInput) {
			i.DiscountType = string(DiscountTypeFreeShipping)
		}, ErrUnsupportedDiscount},
		{"percent >100 rejected", func(i *OrgCouponInput) { i.DiscountValue = orgCouponNum("100.01") }, ErrInvalidCouponInput},
		{"percent 0 rejected", func(i *OrgCouponInput) { i.DiscountValue = orgCouponNum("0") }, ErrInvalidCouponInput},
		{"percent negative rejected", func(i *OrgCouponInput) { i.DiscountValue = orgCouponNum("-5") }, ErrInvalidCouponInput},
		{"percent missing value", func(i *OrgCouponInput) { i.DiscountValue = nil }, ErrInvalidCouponInput},
		// pgtype.Numeric accepts NaN/±Infinity — non-finite values must not
		// slip through the range checks (every comparison against them is false).
		{"percent NaN rejected", func(i *OrgCouponInput) { i.DiscountValue = orgCouponNum("NaN") }, ErrInvalidCouponInput},
		{"percent -Infinity rejected", func(i *OrgCouponInput) { i.DiscountValue = orgCouponNum("-Infinity") }, ErrInvalidCouponInput},
		{"fixed 0 rejected", func(i *OrgCouponInput) {
			i.DiscountType = string(DiscountTypeFixed)
			i.DiscountValue = orgCouponNum("0")
		}, ErrInvalidCouponInput},
		{"fixed negative rejected", func(i *OrgCouponInput) {
			i.DiscountType = string(DiscountTypeFixed)
			i.DiscountValue = orgCouponNum("-1")
		}, ErrInvalidCouponInput},
		{"fixed Infinity rejected", func(i *OrgCouponInput) {
			i.DiscountType = string(DiscountTypeFixed)
			i.DiscountValue = orgCouponNum("Infinity")
		}, ErrInvalidCouponInput},
		{"value without type rejected", func(i *OrgCouponInput) {
			i.DiscountType = ""
		}, ErrInvalidCouponInput},

		{"expires before valid_from", func(i *OrgCouponInput) {
			i.ValidFrom = &validFrom
			i.ExpiresAt = &expiresBefore
		}, ErrInvalidCouponInput},

		{"negative max_redemptions", func(i *OrgCouponInput) { i.MaxRedemptions = i32(-1) }, ErrInvalidCouponInput},
		{"negative per_user_limit", func(i *OrgCouponInput) { i.PerUserLimit = i32(-2) }, ErrInvalidCouponInput},
		{"negative min_spend", func(i *OrgCouponInput) { i.MinSpend = i64(-100) }, ErrInvalidCouponInput},

		{"bad applies_to", func(i *OrgCouponInput) { i.AppliesTo = strp("weekly") }, ErrInvalidCouponInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base()
			if tc.mutate != nil {
				tc.mutate(&input)
			}
			err := validateOrgCouponInput(input)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %v, got nil", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error matching %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestResolveOrgCouponStatus(t *testing.T) {
	strp := func(s string) *string { return &s }
	ts := func(d time.Duration) *pgtype.Timestamptz {
		return &pgtype.Timestamptz{Time: time.Now().Add(d), Valid: true}
	}

	activeCoupon := gen.Coupon{Status: string(CouponStatusActive)}
	suspendedCoupon := gen.Coupon{Status: string(CouponStatusSuspended)}
	expiredCoupon := gen.Coupon{Status: string(CouponStatusExpired)}
	lapsedCoupon := gen.Coupon{
		Status:    string(CouponStatusActive),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	}

	cases := []struct {
		name          string
		requested     *string
		current       gen.Coupon
		mergedExpires *pgtype.Timestamptz
		want          string
		wantErr       error
	}{
		{name: "absent keeps active", requested: nil, current: activeCoupon, want: "active"},
		{name: "absent keeps suspended", requested: nil, current: suspendedCoupon, want: "suspended"},
		{name: "absent keeps expired", requested: nil, current: expiredCoupon, want: "expired"},

		{name: "suspend active", requested: strp("suspended"), current: activeCoupon, want: "suspended"},
		{name: "reactivate suspended", requested: strp("active"), current: suspendedCoupon, want: "active"},
		{name: "case and whitespace tolerant", requested: strp("  Suspended "), current: activeCoupon, want: "suspended"},
		{name: "no-op active on active", requested: strp("active"), current: activeCoupon, want: "active"},

		{name: "expired target rejected", requested: strp("expired"), current: activeCoupon, wantErr: ErrInvalidCouponInput},
		{name: "pending target rejected", requested: strp("pending"), current: activeCoupon, wantErr: ErrInvalidCouponInput},
		{name: "deleted target rejected", requested: strp("deleted"), current: activeCoupon, wantErr: ErrInvalidCouponInput},
		{name: "bogus target rejected", requested: strp("bogus"), current: activeCoupon, wantErr: ErrInvalidCouponInput},
		{name: "blank target rejected", requested: strp("   "), current: activeCoupon, wantErr: ErrInvalidCouponInput},

		{name: "reactivate expired status rejected", requested: strp("active"), current: expiredCoupon, wantErr: ErrInvalidCouponInput},
		{name: "reactivate lapsed expiry rejected", requested: strp("active"), current: lapsedCoupon, wantErr: ErrInvalidCouponInput},
		{name: "reactivate with past merged expiry rejected", requested: strp("active"), current: suspendedCoupon, mergedExpires: ts(-time.Hour), wantErr: ErrInvalidCouponInput},
		{name: "suspend lapsed coupon allowed", requested: strp("suspended"), current: lapsedCoupon, want: "suspended"},
		{name: "suspend expired status allowed", requested: strp("suspended"), current: expiredCoupon, want: "suspended"},
		{name: "activate with future merged expiry ok", requested: strp("active"), current: suspendedCoupon, mergedExpires: ts(time.Hour), want: "active"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveOrgCouponStatus(tc.requested, tc.current, tc.mergedExpires)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.want {
					t.Fatalf("resolveOrgCouponStatus = %q, want %q", got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %v, got nil (status %q)", tc.wantErr, got)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error matching %v, got %v", tc.wantErr, err)
			}
		})
	}
}
