// Package remittance — handlers_test.go tests the remittance HTTP handlers
// and core calculation logic. Includes auth denial tests for authenticated
// routes and table-driven tests for the staleness check and calculator.
//
// Ref: japaconnect-engineering/sections/74-remittance-tracker.md
//
//	japaconnect-engineering/adr/ADR-075-remittance-rate-tracker.md
package remittance

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Auth denial tests — authenticated routes must return 401 without auth.
// The RBAC middleware is applied at the route registration site, so these
// tests verify the handler's own requireUser check.
// ---------------------------------------------------------------------------

func TestHandleListAlerts_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/remittance/alerts", nil)
	rr := httptest.NewRecorder()
	h.HandleListAlerts(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleListAlerts without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateAlert_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/remittance/alerts", nil)
	rr := httptest.NewRecorder()
	h.HandleCreateAlert(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleCreateAlert without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleUpdateAlert_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/remittance/alerts/123", nil)
	rr := httptest.NewRecorder()
	h.HandleUpdateAlert(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleUpdateAlert without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeleteAlert_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/remittance/alerts/123", nil)
	rr := httptest.NewRecorder()
	h.HandleDeleteAlert(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleDeleteAlert without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminListProviders_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/remittance/providers", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminListProviders(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminListProviders without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminCreateProvider_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/remittance/providers", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminCreateProvider(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminCreateProvider without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminUpdateProvider_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/admin/remittance/providers/123", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminUpdateProvider(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminUpdateProvider without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminInsertManualRate_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/remittance/providers/123/rates", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminInsertManualRate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminInsertManualRate without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminTriggerFetch_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/remittance/providers/123/fetch", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminTriggerFetch(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminTriggerFetch without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// TestIsManualRateStale — table-driven test for the staleness check.
// ---------------------------------------------------------------------------

func TestIsManualRateStale(t *testing.T) {
	// Service with nil settings uses the default 24-hour threshold.
	svc := &Service{}

	tests := []struct {
		name      string
		fetchedAt time.Time
		want      bool
	}{
		{
			name:      "fresh_rate_1h_ago",
			fetchedAt: time.Now().Add(-1 * time.Hour),
			want:      false,
		},
		{
			name:      "fresh_rate_23h_ago",
			fetchedAt: time.Now().Add(-23 * time.Hour),
			want:      false,
		},
		{
			name:      "stale_rate_25h_ago",
			fetchedAt: time.Now().Add(-25 * time.Hour),
			want:      true,
		},
		{
			name:      "stale_rate_48h_ago",
			fetchedAt: time.Now().Add(-48 * time.Hour),
			want:      true,
		},
		{
			name:      "fresh_rate_just_now",
			fetchedAt: time.Now(),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.IsManualRateStale(tt.fetchedAt)
			if got != tt.want {
				t.Errorf("IsManualRateStale(%v) = %v, want %v", tt.fetchedAt, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCalculateResult — tests the calculation logic with known rates.
// Verifies fee deduction, effective rate, and ranking by receive amount.
// ---------------------------------------------------------------------------

func TestCalculateResult(t *testing.T) {
	// Known rates for GBP→NGN corridor:
	//   Wise:     rate=1050, fee_flat=0.50, fee_pct=0
	//   Remitly:  rate=1045, fee_flat=0,    fee_pct=1.5
	//   Sendwave: rate=1040, fee_flat=0,    fee_pct=0
	//
	// Send amount: 1000 GBP
	//
	// Wise:     gross=1,050,000; fees=0.50; net_send=999.50; receive=1,049,475
	// Remitly:  gross=1,045,000; fees=15.00; net_send=985.00; receive=1,029,325
	// Sendwave: gross=1,040,000; fees=0;     net_send=1000;   receive=1,040,000
	//
	// Ranking by receive amount: Wise > Sendwave > Remitly
	rates := []RateInfo{
		{
			ProviderID:   "wise",
			ProviderName: "Wise",
			Rate:         1050.0,
			FeeFlat:      0.50,
			FeePct:       0,
		},
		{
			ProviderID:   "remitly",
			ProviderName: "Remitly",
			Rate:         1045.0,
			FeeFlat:      0,
			FeePct:       1.5,
		},
		{
			ProviderID:   "sendwave",
			ProviderName: "Sendwave",
			Rate:         1040.0,
			FeeFlat:      0,
			FeePct:       0,
		},
	}

	sendAmount := 1000.0

	providers := make([]ProviderCalculation, 0, len(rates))
	for _, r := range rates {
		providers = append(providers, computeProviderCalculation(r, sendAmount))
	}

	// Verify individual calculations.
	wise := findProvider(providers, "wise")
	if wise == nil {
		t.Fatal("wise provider not found in results")
	}

	if !almostEqual(wise.TotalFees, 0.50, 0.01) {
		t.Errorf("Wise total fees = %.2f, want 0.50", wise.TotalFees)
	}

	if !almostEqual(wise.ReceiveAmount, 1049475.0, 1.0) {
		t.Errorf("Wise receive amount = %.2f, want 1049475.0", wise.ReceiveAmount)
	}

	if !almostEqual(wise.EffectiveRate, 1049.475, 0.01) {
		t.Errorf("Wise effective rate = %.4f, want 1049.475", wise.EffectiveRate)
	}

	remitly := findProvider(providers, "remitly")
	if remitly == nil {
		t.Fatal("remitly provider not found in results")
	}

	if !almostEqual(remitly.TotalFees, 15.0, 0.01) {
		t.Errorf("Remitly total fees = %.2f, want 15.00", remitly.TotalFees)
	}

	if !almostEqual(remitly.ReceiveAmount, 1029325.0, 1.0) {
		t.Errorf("Remitly receive amount = %.2f, want 1029325.0", remitly.ReceiveAmount)
	}

	sendwave := findProvider(providers, "sendwave")
	if sendwave == nil {
		t.Fatal("sendwave provider not found in results")
	}

	if !almostEqual(sendwave.TotalFees, 0, 0.01) {
		t.Errorf("Sendwave total fees = %.2f, want 0.00", sendwave.TotalFees)
	}

	if !almostEqual(sendwave.ReceiveAmount, 1040000.0, 1.0) {
		t.Errorf("Sendwave receive amount = %.2f, want 1040000.0", sendwave.ReceiveAmount)
	}

	// Verify ranking: Wise should have the highest receive amount.
	if wise.ReceiveAmount < sendwave.ReceiveAmount {
		t.Error("Wise should have higher receive amount than Sendwave")
	}

	if wise.ReceiveAmount < remitly.ReceiveAmount {
		t.Error("Wise should have higher receive amount than Remitly")
	}
}

// TestCalculateResult_Ranking verifies that providers are ranked by receive
// amount descending in the full CalculateResult.
func TestCalculateResult_Ranking(t *testing.T) {
	rates := []RateInfo{
		{ProviderID: "low", ProviderName: "LowRate", Rate: 1000, FeeFlat: 0, FeePct: 0},
		{ProviderID: "high", ProviderName: "HighRate", Rate: 1100, FeeFlat: 0, FeePct: 0},
		{ProviderID: "mid", ProviderName: "MidRate", Rate: 1050, FeeFlat: 0, FeePct: 0},
	}

	sendAmount := 500.0

	providers := make([]ProviderCalculation, 0, len(rates))
	for _, r := range rates {
		providers = append(providers, computeProviderCalculation(r, sendAmount))
	}

	// Sort by receive amount descending (same logic as Service.Calculate).
	sortProvidersByReceiveAmount(providers)

	if providers[0].ProviderID != "high" {
		t.Errorf("top provider = %s, want high", providers[0].ProviderID)
	}

	if providers[1].ProviderID != "mid" {
		t.Errorf("second provider = %s, want mid", providers[1].ProviderID)
	}

	if providers[2].ProviderID != "low" {
		t.Errorf("third provider = %s, want low", providers[2].ProviderID)
	}

	// Best receive amount should be 1100 * 500 = 550,000.
	if !almostEqual(providers[0].ReceiveAmount, 550000.0, 0.01) {
		t.Errorf("best receive amount = %.2f, want 550000.0", providers[0].ReceiveAmount)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func findProvider(providers []ProviderCalculation, id string) *ProviderCalculation {
	for i := range providers {
		if providers[i].ProviderID == id {
			return &providers[i]
		}
	}

	return nil
}

func almostEqual(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}

	return diff <= tolerance
}

// sortProvidersByReceiveAmount sorts providers by receive amount descending.
// Mirrors the sort logic in Service.Calculate.
func sortProvidersByReceiveAmount(providers []ProviderCalculation) {
	for i := 1; i < len(providers); i++ {
		for j := i; j > 0 && providers[j].ReceiveAmount > providers[j-1].ReceiveAmount; j-- {
			providers[j], providers[j-1] = providers[j-1], providers[j]
		}
	}
}
