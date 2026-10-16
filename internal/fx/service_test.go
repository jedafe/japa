package fx

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

func testMatrix() *Matrix {
	return &Matrix{
		Base:      "USD",
		Rates:     map[string]float64{"USD": 1, "NGN": 1600, "GBP": 0.8, "EUR": 0.9, "ZZZ": 0},
		FetchedAt: time.Now(),
	}
}

func TestComputeConvert_CrossRate(t *testing.T) {
	m := testMatrix()

	tests := []struct {
		name     string
		amount   float64
		from, to string
		wantRate float64
		wantConv float64
		wantErr  bool
	}{
		{"identity", 100, "USD", "USD", 1, 100, false},
		{"base to quote", 100, "USD", "NGN", 1600, 160000, false},
		{"quote to base", 1600, "NGN", "USD", 1.0 / 1600.0, 1, false},
		{"cross pair", 100, "GBP", "NGN", 2000, 200000, false},
		{"cross pair inverse", 2000, "NGN", "GBP", 0.0005, 1, false},
		{"unsupported from", 1, "XXX", "NGN", 0, 0, true},
		{"unsupported to", 1, "USD", "XXX", 0, 0, true},
		{"zero amount", 0, "USD", "NGN", 0, 0, true},
		{"negative amount", -5, "USD", "NGN", 0, 0, true},
		{"zero from rate", 1, "ZZZ", "NGN", 0, 0, true},
		{"zero to rate", 1, "USD", "ZZZ", 0, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rate, converted, err := computeConvert(m, tc.amount, tc.from, tc.to)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got rate=%v converted=%v", rate, converted)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := rate - tc.wantRate; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("rate: got %v want %v", rate, tc.wantRate)
			}

			if diff := converted - tc.wantConv; diff > 1e-6 || diff < -1e-6 {
				t.Fatalf("converted: got %v want %v", converted, tc.wantConv)
			}
		})
	}
}

func TestComputeConvert_UnsupportedCurrency(t *testing.T) {
	m := testMatrix()

	_, _, err := computeConvert(m, 100, "XXX", "NGN")
	if !errors.Is(err, ErrUnsupportedCurrency) {
		t.Fatalf("expected ErrUnsupportedCurrency, got %v", err)
	}

	_, _, err = computeConvert(m, 100, "USD", "ZZZ")
	if !errors.Is(err, ErrUnsupportedCurrency) {
		t.Fatalf("expected ErrUnsupportedCurrency, got %v", err)
	}
}

func TestMatrixStale_Boundary(t *testing.T) {
	now := time.Now()

	const staleHours = 48

	tests := []struct {
		name      string
		fetchedAt time.Time
		want      bool
	}{
		{"fresh", now.Add(-1 * time.Hour), false},
		{"just under threshold", now.Add(-staleHours*time.Hour + time.Minute), false},
		{"exactly at threshold", now.Add(-staleHours * time.Hour), false},
		{"just over threshold", now.Add(-staleHours*time.Hour - time.Minute), true},
		{"very old", now.Add(-30 * 24 * time.Hour), true},
		{"zero time", time.Time{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := matrixStale(tc.fetchedAt, staleHours, now); got != tc.want {
				t.Fatalf("matrixStale(%v, %d, %v) = %v, want %v", tc.fetchedAt, staleHours, now, got, tc.want)
			}
		})
	}
}

func TestMatrixFromRows_Empty(t *testing.T) {
	_, err := matrixFromRows("USD", nil)
	if !errors.Is(err, ErrEmptyMatrix) {
		t.Fatalf("expected ErrEmptyMatrix, got %v", err)
	}

	_, err = matrixFromRows("USD", []gen.GetLatestFxMatrixRow{})
	if !errors.Is(err, ErrEmptyMatrix) {
		t.Fatalf("expected ErrEmptyMatrix, got %v", err)
	}
}

func TestMatrixFromRows(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-30 * time.Minute)

	rows := []gen.GetLatestFxMatrixRow{
		{QuoteCurrency: "NGN", Rate: numericFromFloat(1600), FetchedAt: pgtype.Timestamptz{Time: old, Valid: true}},
		{QuoteCurrency: "GBP", Rate: numericFromFloat(0.8), FetchedAt: pgtype.Timestamptz{Time: newer, Valid: true}},
	}

	m, err := matrixFromRows("USD", rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.Base != "USD" {
		t.Fatalf("base: got %q want USD", m.Base)
	}

	if m.Rates["USD"] != 1 {
		t.Fatalf("base→base rate: got %v want 1", m.Rates["USD"])
	}

	if m.Rates["NGN"] != 1600 {
		t.Fatalf("NGN rate: got %v want 1600", m.Rates["NGN"])
	}

	if m.Rates["GBP"] != 0.8 {
		t.Fatalf("GBP rate: got %v want 0.8", m.Rates["GBP"])
	}

	if !m.FetchedAt.Equal(newer) {
		t.Fatalf("fetched_at: got %v want %v (newest row)", m.FetchedAt, newer)
	}
}

func TestBaseRateFromMatrix(t *testing.T) {
	m := testMatrix()

	rate, err := baseRateFromMatrix(m, "NGN")
	if err != nil || rate != 1600 {
		t.Fatalf("NGN: got rate=%v err=%v, want 1600", rate, err)
	}

	rate, err = baseRateFromMatrix(m, "USD")
	if err != nil || rate != 1 {
		t.Fatalf("USD base: got rate=%v err=%v, want 1", rate, err)
	}

	if _, err = baseRateFromMatrix(m, "XXX"); !errors.Is(err, ErrUnsupportedCurrency) {
		t.Fatalf("XXX: expected ErrUnsupportedCurrency, got %v", err)
	}

	if _, err = baseRateFromMatrix(m, "ZZZ"); !errors.Is(err, ErrUnsupportedCurrency) {
		t.Fatalf("zero rate: expected ErrUnsupportedCurrency, got %v", err)
	}
}
