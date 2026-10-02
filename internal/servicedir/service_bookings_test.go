// service_bookings_test.go — unit coverage for the §111 pure/validator
// paths that don't need a database: enum validators, departure input
// validation, platform-fee computation, and the booker-cancel refund
// policy. Store-backed paths are covered by integration tests.
//
// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
package servicedir

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// fakeSettings is an in-memory SettingsReader for unit tests.
type fakeSettings map[string]string

func (f fakeSettings) GetBoolOr(_ context.Context, key string, fallback bool) bool {
	if v, ok := f[key]; ok {
		return v == "true"
	}

	return fallback
}

func (f fakeSettings) GetIntOr(_ context.Context, key string, fallback int) int {
	if v, ok := f[key]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}

	return fallback
}

func (f fakeSettings) GetStringOr(_ context.Context, key, fallback string) string {
	if v, ok := f[key]; ok {
		return v
	}

	return fallback
}

func TestIsValidListingKind(t *testing.T) {
	valid := []string{"service", "package"}
	for _, v := range valid {
		if !IsValidListingKind(v) {
			t.Errorf("expected %q to be a valid listing_kind", v)
		}
	}

	for _, v := range []string{"", "bundle", "SERVICE", "pkg"} {
		if IsValidListingKind(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestIsValidPurchaseMode(t *testing.T) {
	for _, v := range []string{"inquiry", "external", "paid"} {
		if !IsValidPurchaseMode(v) {
			t.Errorf("expected %q to be a valid purchase_mode", v)
		}
	}

	// 'full' is deliberately rejected — deferred per §111 Non-Goals.
	for _, v := range []string{"", "full", "deposit", "PAID"} {
		if IsValidPurchaseMode(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestIsValidBookingStatus(t *testing.T) {
	for _, v := range []string{"pending", "confirmed", "fulfilled", "cancelled", "declined", "disputed", "refunded"} {
		if !IsValidBookingStatus(v) {
			t.Errorf("expected %q to be a valid booking status", v)
		}
	}

	for _, v := range []string{"", "expired", "active", "PENDING"} {
		if IsValidBookingStatus(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestIsValidPricePer(t *testing.T) {
	for _, v := range []string{"person", "package", "group"} {
		if !IsValidPricePer(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}

	for _, v := range []string{"", "seat", "unit"} {
		if IsValidPricePer(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestValidatePackageDepartures(t *testing.T) {
	t.Run("empty set is valid", func(t *testing.T) {
		params, err := validatePackageDepartures(nil)
		if err != nil || params != nil {
			t.Fatalf("expected nil params, no error; got %v, %v", params, err)
		}
	})

	t.Run("requires starts_on", func(t *testing.T) {
		_, err := validatePackageDepartures([]PackageDepartureInput{{EndsOn: "2026-01-01"}})
		if err == nil {
			t.Fatal("expected error for missing starts_on")
		}
	})

	t.Run("rejects ends_on before starts_on", func(t *testing.T) {
		_, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			EndsOn:   "2026-05-01",
		}})
		if err == nil {
			t.Fatal("expected error for ends_on preceding starts_on")
		}
	})

	t.Run("rejects non-positive capacity", func(t *testing.T) {
		zero := int32(0)
		_, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			Capacity: &zero,
		}})
		if err == nil {
			t.Fatal("expected error for zero capacity")
		}
	})

	// 'full' is system-managed but accepted so a loaded row round-trips
	// unchanged through the editor — re-saving a full departure must not
	// 400. Unknown statuses still reject.
	t.Run("accepts 'full' status for round-trip", func(t *testing.T) {
		params, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			Status:   DepartureStatusFull,
		}})
		if err != nil {
			t.Fatalf("unexpected error for 'full' status: %v", err)
		}
		if *params[0].Status != DepartureStatusFull {
			t.Fatalf("status not preserved: %s", *params[0].Status)
		}
	})

	t.Run("rejects unknown status", func(t *testing.T) {
		_, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			Status:   "bogus",
		}})
		if err == nil {
			t.Fatal("expected error for unknown status")
		}
	})

	t.Run("accepts a well-formed open departure", func(t *testing.T) {
		cap := int32(12)
		params, err := validatePackageDepartures([]PackageDepartureInput{{
			StartsOn: "2026-06-01",
			EndsOn:   "2026-06-10",
			Capacity: &cap,
			Status:   DepartureStatusOpen,
		}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(params) != 1 || params[0].Capacity == nil || *params[0].Capacity != 12 {
			t.Fatalf("bad params: %+v", params)
		}
	})
}

func TestComputeFee(t *testing.T) {
	ctx := context.Background()

	t.Run("percent fee", func(t *testing.T) {
		s := &Service{settings: fakeSettings{
			"service_packages.fee_type": "percent",
			"service_packages.fee_pct":  "10",
		}}
		if fee := s.computeFee(ctx, 500_000); fee != 50_000 {
			t.Fatalf("expected 50000, got %d", fee)
		}
	})

	t.Run("fixed fee", func(t *testing.T) {
		s := &Service{settings: fakeSettings{
			"service_packages.fee_type":  "fixed",
			"service_packages.fee_fixed": "25000",
		}}
		if fee := s.computeFee(ctx, 500_000); fee != 25_000 {
			t.Fatalf("expected 25000, got %d", fee)
		}
	})

	t.Run("fee is capped at the package price", func(t *testing.T) {
		s := &Service{settings: fakeSettings{
			"service_packages.fee_type":  "fixed",
			"service_packages.fee_fixed": "999999",
		}}
		if fee := s.computeFee(ctx, 500_000); fee != 500_000 {
			t.Fatalf("expected fee capped at price 500000, got %d", fee)
		}
	})

	t.Run("defaults to 10 percent when unset", func(t *testing.T) {
		s := &Service{settings: fakeSettings{}}
		if fee := s.computeFee(ctx, 200_000); fee != 20_000 {
			t.Fatalf("expected 20000, got %d", fee)
		}
	})
}

func TestCancelFeeAction(t *testing.T) {
	ctx := context.Background()
	s := &Service{settings: fakeSettings{"service_packages.refund_window_days": "7"}}

	t.Run("inquiry bookings carry no fee", func(t *testing.T) {
		row := BookingRow{Intent: BookingIntentInquiry}
		if got := s.cancelFeeAction(ctx, row); got != "none" {
			t.Fatalf("expected none, got %q", got)
		}
	})

	t.Run("booking without payment carries no fee", func(t *testing.T) {
		row := BookingRow{Intent: BookingIntentBooking}
		if got := s.cancelFeeAction(ctx, row); got != "none" {
			t.Fatalf("expected none, got %q", got)
		}
	})

	paymentID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	t.Run("pending booking cancels always refund", func(t *testing.T) {
		row := BookingRow{Intent: BookingIntentBooking, PaymentID: paymentID, Status: BookingStatusPending}
		if got := s.cancelFeeAction(ctx, row); got != "refund" {
			t.Fatalf("expected refund, got %q", got)
		}
	})

	t.Run("confirmed inside the window refunds", func(t *testing.T) {
		row := BookingRow{
			Intent: BookingIntentBooking, PaymentID: paymentID, Status: BookingStatusConfirmed,
			ConfirmedAt: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
		}
		if got := s.cancelFeeAction(ctx, row); got != "refund" {
			t.Fatalf("expected refund, got %q", got)
		}
	})

	t.Run("confirmed outside the window forfeits the fee", func(t *testing.T) {
		row := BookingRow{
			Intent: BookingIntentBooking, PaymentID: paymentID, Status: BookingStatusConfirmed,
			ConfirmedAt: pgtype.Timestamptz{Time: time.Now().Add(-30 * 24 * time.Hour), Valid: true},
		}
		if got := s.cancelFeeAction(ctx, row); got != "retain" {
			t.Fatalf("expected retain, got %q", got)
		}
	})
}
