package fx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jedafe/japa/internal/remittance"
)

func TestParseOXRResponse(t *testing.T) {
	body := `{"disclaimer":"x","license":"x","timestamp":1,"base":"USD","rates":{"NGN":1600.5,"GBP":0.79,"EUR":0.91}}`

	base, rates, err := parseOXRResponse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}

	if base != "USD" {
		t.Fatalf("unexpected base: %q", base)
	}

	if rates["NGN"] != 1600.5 || rates["GBP"] != 0.79 {
		t.Fatalf("unexpected rates: %v", rates)
	}
}

func TestCorridorUnionAndEmission(t *testing.T) {
	m := &Matrix{Base: "USD", Rates: map[string]float64{"USD": 1, "NGN": 1600, "GBP": 0.8, "GHS": 15}}

	configCorridors := []string{"GBP->NGN", "USD->NGN", "EUR->NGN"} // EUR absent → skipped
	requested := []remittance.Corridor{
		{SendCurrency: "USD", ReceiveCurrency: "GHS"},
		{SendCurrency: "USD", ReceiveCurrency: "NGN"}, // dup of config pair → dedup
	}

	snaps := emitSnapshots(m, configCorridors, requested, "https://openexchangerates.org")
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots (GBP->NGN, USD->NGN, USD->GHS), got %d: %+v", len(snaps), snaps)
	}

	for _, s := range snaps {
		if s.SourceURL == "" {
			t.Fatal("snapshot missing SourceURL attribution")
		}
	}
	// GBP->NGN cross-rate: 1600/0.8 = 2000
	for _, s := range snaps {
		if s.SendCurrency == "GBP" && s.ReceiveCurrency == "NGN" && s.Rate != 2000 {
			t.Fatalf("GBP->NGN rate: got %v want 2000", s.Rate)
		}
	}
}

func TestThrottle(t *testing.T) {
	// min_interval_minutes=60, last fetch 30 min ago → throttled.
	if !throttled(time.Now().Add(-30*time.Minute), 60) {
		t.Fatal("expected throttled within interval")
	}

	if throttled(time.Now().Add(-90*time.Minute), 60) {
		t.Fatal("expected not throttled past interval")
	}

	if throttled(time.Time{}, 60) {
		t.Fatal("zero last-fetch must never throttle")
	}
}

func TestFetchRates_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("app_id") != "testkey" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"base":  "USD",
			"rates": map[string]float64{"NGN": 1600, "GBP": 0.8},
		})
	}))
	defer srv.Close()

	a := &Adapter{svc: nil, http: srv.Client(), allowPrivate: true} // svc nil → pure fetch path test via fetchMatrix
	cfg := providerConfig{AppID: "testkey", Endpoint: srv.URL, BaseCurrency: "USD", Engine: "oxr"}

	rates, err := a.fetchMatrix(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if rates["USD"] != 1 || rates["NGN"] != 1600 {
		t.Fatalf("matrix missing entries: %v", rates)
	}
}

// A response base that disagrees with api_config.base_currency must fail —
// persisting a mislabeled matrix silently corrupts every downstream rate.
func TestFetchMatrix_BaseMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"base":  "EUR",
			"rates": map[string]float64{"USD": 1.1},
		})
	}))
	defer srv.Close()

	a := &Adapter{svc: nil, http: srv.Client(), allowPrivate: true}
	cfg := providerConfig{AppID: "k", Endpoint: srv.URL, BaseCurrency: "USD", Engine: "oxr"}

	if _, err := a.fetchMatrix(t.Context(), cfg); err == nil {
		t.Fatal("expected base-mismatch error")
	}
}
