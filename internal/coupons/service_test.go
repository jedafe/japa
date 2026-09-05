package coupons

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestPaginate(t *testing.T) {
	tests := []struct {
		name     string
		page     int32
		pageSize int32
		wantLim  int32
		wantOff  int32
	}{
		{"defaults", 0, 0, DefaultPageSize, 0},
		{"page 1", 1, 10, 10, 0},
		{"page 2", 2, 10, 10, 10},
		{"page 3 size 20", 3, 20, 20, 40},
		{"max page size", 1, MaxPageSize + 50, DefaultPageSize, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLim, gotOff := paginate(tt.page, tt.pageSize)
			if gotLim != tt.wantLim || gotOff != tt.wantOff {
				t.Errorf("paginate(%d, %d) = (%d, %d), want (%d, %d)",
					tt.page, tt.pageSize, gotLim, gotOff, tt.wantLim, tt.wantOff)
			}
		})
	}
}

func TestDerefStr(t *testing.T) {
	if derefStr(nil) != "" {
		t.Error("derefStr(nil) should return empty string")
	}

	s := "hello"
	if derefStr(&s) != "hello" {
		t.Errorf("derefStr(&%q) = %q, want %q", s, derefStr(&s), s)
	}
}

func TestToNullableUUID(t *testing.T) {
	u := toNullableUUID(nil)
	if u.Valid {
		t.Error("toNullableUUID(nil) should return invalid UUID")
	}

	id := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true}
	u2 := toNullableUUID(&id)

	if !u2.Valid || u2 != id {
		t.Error("toNullableUUID should copy UUID value")
	}
}

func TestToNullableNumeric(t *testing.T) {
	n := toNullableNumeric(nil)
	if n.Valid {
		t.Error("toNullableNumeric(nil) should return invalid numeric")
	}
}

func TestToTimestamptz(t *testing.T) {
	ts := toTimestamptz(nil)
	if ts.Valid {
		t.Error("toTimestamptz(nil) should return invalid timestamp")
	}

	now := time.Now()
	tp := pgtype.Timestamptz{Time: now, Valid: true}

	ts2 := toTimestamptz(&tp)
	if !ts2.Valid || !ts2.Time.Equal(now) {
		t.Error("toTimestamptz should copy timestamp value")
	}
}

func TestPtrNumeric(t *testing.T) {
	n := pgtype.Numeric{Valid: false}
	if ptrNumeric(n) != nil {
		t.Error("ptrNumeric with invalid numeric should return nil")
	}
}

func TestPtrUUID(t *testing.T) {
	u := pgtype.UUID{Valid: false}
	if ptrUUID(u) != nil {
		t.Error("ptrUUID with invalid UUID should return nil")
	}
}

func TestPtrTimestamptz(t *testing.T) {
	ts := pgtype.Timestamptz{Valid: false}
	if ptrTimestamptz(ts) != nil {
		t.Error("ptrTimestamptz with invalid timestamp should return nil")
	}
}

func TestErrors(t *testing.T) {
	if ErrCouponNotFound.Error() != "coupon not found" {
		t.Error("wrong error message for ErrCouponNotFound")
	}

	if ErrProviderNotFound.Error() != "affiliate provider not found" {
		t.Error("wrong error message for ErrProviderNotFound")
	}

	if ErrProviderHasCoupons.Error() != "cannot delete provider with existing coupons" {
		t.Error("wrong error message for ErrProviderHasCoupons")
	}
}

func TestConstants(t *testing.T) {
	if CouponStatusActive != "active" {
		t.Error("wrong CouponStatusActive value")
	}

	if CouponStatusExpired != "expired" {
		t.Error("wrong CouponStatusExpired value")
	}

	if CouponStatusSuspended != "suspended" {
		t.Error("wrong CouponStatusSuspended value")
	}

	if CouponTypeExternal != "external" {
		t.Error("wrong CouponTypeExternal value")
	}

	if CouponTypeInternal != "internal" {
		t.Error("wrong CouponTypeInternal value")
	}
}
