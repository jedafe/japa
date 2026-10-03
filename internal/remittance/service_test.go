// Package remittance — service_test.go tests the pure rate-selection logic
// that does not require a store: benchmark partitioning and its use in the
// best-rate pick and calculator ranking (§112, ADR-125).
//
// Ref: japaconnect-engineering/sections/112-fx-converter.md
//
//	japaconnect-engineering/sections/74-remittance-tracker.md (benchmark amendment)
package remittance

import "testing"

// TestSplitBenchmark verifies that benchmark rows are separated from sellable
// provider rows: the mid-market reference never leaks into the provider list.
func TestSplitBenchmark(t *testing.T) {
	rates := []RateInfo{
		{ProviderID: "a", ProviderName: "Provider A", Rate: 1500},
		{ProviderID: "mm", ProviderName: "Mid-market", Rate: 1540, IsBenchmark: true},
		{ProviderID: "b", ProviderName: "Provider B", Rate: 1480},
	}

	providers, bench := splitBenchmark(rates)
	if len(providers) != 2 {
		t.Fatalf("expected 2 provider rows, got %d", len(providers))
	}

	if bench == nil || bench.ProviderID != "mm" {
		t.Fatalf("expected benchmark row 'mm', got %+v", bench)
	}

	for _, p := range providers {
		if p.IsBenchmark {
			t.Fatal("benchmark row leaked into provider list")
		}
	}
}

// TestSplitBenchmark_None verifies that input without a benchmark row passes
// through unchanged.
func TestSplitBenchmark_None(t *testing.T) {
	providers, bench := splitBenchmark([]RateInfo{{ProviderID: "a", Rate: 1}})
	if bench != nil || len(providers) != 1 {
		t.Fatal("no-benchmark input must pass through unchanged")
	}
}

// TestSplitBenchmark_Multiple verifies that when more than one benchmark row
// exists for a corridor, the highest rate is kept as the reference.
func TestSplitBenchmark_Multiple(t *testing.T) {
	rates := []RateInfo{
		{ProviderID: "mm1", Rate: 1540, IsBenchmark: true},
		{ProviderID: "mm2", Rate: 1542, IsBenchmark: true},
		{ProviderID: "a", Rate: 1500},
	}

	providers, bench := splitBenchmark(rates)
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider row, got %d", len(providers))
	}

	if bench == nil || bench.ProviderID != "mm2" {
		t.Fatalf("expected highest benchmark row 'mm2', got %+v", bench)
	}
}

// TestBestProviderExcludesBenchmark verifies the best-rate selection logic
// used by GetBestRate: a benchmark row with the highest raw rate must not be
// picked as the best provider.
func TestBestProviderExcludesBenchmark(t *testing.T) {
	rates := []RateInfo{
		{ProviderID: "mm", ProviderName: "Mid-market", Rate: 1540, IsBenchmark: true},
		{ProviderID: "a", ProviderName: "Provider A", Rate: 1500},
		{ProviderID: "b", ProviderName: "Provider B", Rate: 1480},
	}

	providers, _ := splitBenchmark(rates)
	if len(providers) == 0 {
		t.Fatal("expected at least one provider")
	}

	best := providers[0]
	for _, r := range providers[1:] {
		if r.Rate > best.Rate {
			best = r
		}
	}

	if best.IsBenchmark || best.ProviderID == "mm" {
		t.Fatalf("benchmark row picked as best provider: %+v", best)
	}

	if best.ProviderID != "a" {
		t.Fatalf("best provider = %s, want a", best.ProviderID)
	}
}

// TestBestProvider_OnlyBenchmark verifies that a corridor containing only a
// benchmark row yields no sellable providers (GetBestRate returns nil).
func TestBestProvider_OnlyBenchmark(t *testing.T) {
	providers, bench := splitBenchmark([]RateInfo{
		{ProviderID: "mm", Rate: 1540, IsBenchmark: true},
	})

	if len(providers) != 0 {
		t.Fatalf("expected 0 providers, got %d", len(providers))
	}

	if bench == nil {
		t.Fatal("expected benchmark row to be preserved")
	}
}

// TestBenchmarkResultComputation verifies that the benchmark receive amount
// is computed as rate × send amount with no fee deduction.
func TestBenchmarkResultComputation(t *testing.T) {
	rates := []RateInfo{
		{ProviderID: "a", ProviderName: "Provider A", Rate: 1500, FeePct: 1.0},
		{ProviderID: "mm", ProviderName: "Mid-market", Rate: 1540, IsBenchmark: true},
	}

	providers, bench := splitBenchmark(rates)
	if bench == nil {
		t.Fatal("expected benchmark row")
	}

	sendAmount := 1000.0

	bc := computeProviderCalculation(*bench, sendAmount)
	if !almostEqual(bc.TotalFees, 0, 0.01) {
		t.Errorf("benchmark total fees = %.2f, want 0", bc.TotalFees)
	}

	if !almostEqual(bc.ReceiveAmount, 1540000.0, 0.01) {
		t.Errorf("benchmark receive amount = %.2f, want 1540000.0", bc.ReceiveAmount)
	}

	if len(providers) != 1 || providers[0].ProviderID != "a" {
		t.Fatalf("benchmark leaked into ranked results: %+v", providers)
	}
}
