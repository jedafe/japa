// Package travel — handlers_test.go covers auth denial for admin routes and
// the pure-function affiliate injection and hash helpers.
//
// Ref: japaconnect-engineering/sections/67-travel-search.md
//
//	japaconnect-engineering/adr/ADR-085-travel-search.md
package travel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Admin route auth denial — no authenticated user in context → 401.
// ---------------------------------------------------------------------------

// newHandlersWithSettings builds a Handlers with nil service/settings so the
// requireUser guard is the first check (it must fire before any service
// access). This mirrors how the production router mounts the handlers behind
// RBAC middleware; here we exercise the in-handler guard directly.
func newHandlersWithSettings() *Handlers {
	return &Handlers{service: nil, settings: nil}
}

func TestAdminListProviders_NoAuth_401(t *testing.T) {
	h := newHandlersWithSettings()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/travel/providers", nil)
	rec := httptest.NewRecorder()
	h.HandleAdminListProviders(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "unauthorized") {
		t.Fatalf("expected unauthorized code in body, got %s", rec.Body.String())
	}
}

func TestAdminCreateProvider_NoAuth_401(t *testing.T) {
	h := newHandlersWithSettings()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/travel/providers", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.HandleAdminCreateProvider(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAdminUpdateProvider_NoAuth_401(t *testing.T) {
	h := newHandlersWithSettings()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/travel/providers/00000000-0000-0000-0000-000000000000", strings.NewReader(`{}`))
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")

	rec := httptest.NewRecorder()
	h.HandleAdminUpdateProvider(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// InjectAffiliateParams — ADR-009 render-time affiliate processing.
// ---------------------------------------------------------------------------

func TestInjectAffiliateParams(t *testing.T) {
	tests := []struct {
		name        string
		bookingURL  string
		affiliate   json.RawMessage
		wantContain []string
	}{
		{
			name:       "appends params to clean URL",
			bookingURL: "https://www.kiwi.com/booking/abc",
			affiliate:  json.RawMessage(`{"aid":"12345","utm_source":"japaconnect"}`),
			wantContain: []string{
				"aid=12345",
				"utm_source=japaconnect",
			},
		},
		{
			name:       "preserves existing query params",
			bookingURL: "https://www.booking.com/hotel/123?lang=en",
			affiliate:  json.RawMessage(`{"aid":"999"}`),
			wantContain: []string{
				"lang=en",
				"aid=999",
			},
		},
		{
			name:       "empty affiliate config returns URL unchanged",
			bookingURL: "https://example.com/booking",
			affiliate:  json.RawMessage(`{}`),
			wantContain: []string{
				"https://example.com/booking",
			},
		},
		{
			name:        "empty booking URL returns empty",
			bookingURL:  "",
			affiliate:   json.RawMessage(`{"aid":"12345"}`),
			wantContain: []string{""},
		},
		{
			name:       "invalid affiliate JSON returns URL unchanged",
			bookingURL: "https://example.com/booking?x=1",
			affiliate:  json.RawMessage(`{invalid`),
			wantContain: []string{
				"https://example.com/booking?x=1",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := InjectAffiliateParams(tc.bookingURL, tc.affiliate)
			for _, want := range tc.wantContain {
				if !strings.Contains(got, want) {
					t.Fatalf("expected result to contain %q, got %q", want, got)
				}
			}
		})
	}
}

func TestInjectAffiliateParams_OverridesExistingParam(t *testing.T) {
	// If the booking URL already has "aid" and the affiliate config also
	// sets "aid", the affiliate value wins (q.Set overwrites).
	got := InjectAffiliateParams(
		"https://example.com/booking?aid=old",
		json.RawMessage(`{"aid":"new"}`),
	)
	if !strings.Contains(got, "aid=new") {
		t.Fatalf("expected aid=new to override, got %q", got)
	}

	if strings.Contains(got, "aid=old") {
		t.Fatalf("did not expect aid=old to remain, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// HashSearchQuery — identical params produce identical hashes.
// ---------------------------------------------------------------------------

func TestHashSearchQuery(t *testing.T) {
	paramsA := FlightSearchParams{
		Origin:      "LOS",
		Destination: "LHR",
		Date:        "2026-10-01",
		Adults:      1,
		Children:    0,
		ReturnDate:  "",
		CabinClass:  "economy",
	}
	paramsB := FlightSearchParams{
		Origin:      "LOS",
		Destination: "LHR",
		Date:        "2026-10-01",
		Adults:      1,
		Children:    0,
		ReturnDate:  "",
		CabinClass:  "economy",
	}

	hashA := HashSearchQuery(paramsA)
	hashB := HashSearchQuery(paramsB)

	if hashA == "" {
		t.Fatal("expected non-empty hash")
	}

	if hashA != hashB {
		t.Fatalf("identical params produced different hashes: %q vs %q", hashA, hashB)
	}
}

func TestHashSearchQuery_DifferentParams_DifferentHash(t *testing.T) {
	paramsA := FlightSearchParams{Origin: "LOS", Destination: "LHR", Date: "2026-10-01", Adults: 1}
	paramsB := FlightSearchParams{Origin: "LOS", Destination: "LHR", Date: "2026-10-01", Adults: 2}

	hashA := HashSearchQuery(paramsA)
	hashB := HashSearchQuery(paramsB)

	if hashA == hashB {
		t.Fatalf("different params produced the same hash: %q", hashA)
	}
}

func TestHashSearchQuery_HotelParams(t *testing.T) {
	paramsA := HotelSearchParams{Destination: "London", CheckIn: "2026-10-01", CheckOut: "2026-10-07", Guests: 2, Rooms: 1}
	paramsB := HotelSearchParams{Destination: "London", CheckIn: "2026-10-01", CheckOut: "2026-10-07", Guests: 2, Rooms: 1}

	if HashSearchQuery(paramsA) != HashSearchQuery(paramsB) {
		t.Fatal("identical hotel params produced different hashes")
	}
}

// ---------------------------------------------------------------------------
// T6 — coverage-hash normalization: case/whitespace variants of the same
// query share one coverage key, distinct queries do not.
// ---------------------------------------------------------------------------

func TestFlightCoverageHash_NormalizesCaseAndWhitespace(t *testing.T) {
	base := FlightSearchParams{Origin: "los", Destination: "lhr", Date: "2026-10-01", Adults: 1, CabinClass: "Economy"}
	variant := FlightSearchParams{Origin: " LOS ", Destination: " LHR", Date: "2026-10-01", Adults: 1, CabinClass: " economy "}

	if flightCoverageHash(base) != flightCoverageHash(variant) {
		t.Fatal("case/whitespace variants produced different flight coverage hashes")
	}
}

func TestFlightCoverageHash_DistinctQueries(t *testing.T) {
	base := FlightSearchParams{Origin: "LOS", Destination: "LHR", Date: "2026-10-01", Adults: 1}
	other := FlightSearchParams{Origin: "LOS", Destination: "CDG", Date: "2026-10-01", Adults: 1}

	if flightCoverageHash(base) == flightCoverageHash(other) {
		t.Fatal("different flight queries produced the same coverage hash")
	}
}

func TestHotelCoverageHash_NormalizesCaseAndWhitespace(t *testing.T) {
	base := HotelSearchParams{Destination: "London", CheckIn: "2026-10-01", CheckOut: "2026-10-07", Guests: 2, Rooms: 1}
	variant := HotelSearchParams{Destination: " LONDON ", CheckIn: "2026-10-01", CheckOut: "2026-10-07", Guests: 2, Rooms: 1}

	if hotelCoverageHash(base) != hotelCoverageHash(variant) {
		t.Fatal("case/whitespace variants produced different hotel coverage hashes")
	}
}

func TestHotelCoverageHash_DistinctQueries(t *testing.T) {
	base := HotelSearchParams{Destination: "London", CheckIn: "2026-10-01", CheckOut: "2026-10-07", Guests: 2, Rooms: 1}
	other := HotelSearchParams{Destination: "Paris", CheckIn: "2026-10-01", CheckOut: "2026-10-07", Guests: 2, Rooms: 1}

	if hotelCoverageHash(base) == hotelCoverageHash(other) {
		t.Fatal("different hotel queries produced the same coverage hash")
	}
}

func TestCoverageKeys_MatchHashNormalization(t *testing.T) {
	if flightRouteKey(FlightSearchParams{Origin: " los", Destination: "LHR "}) != "LOS-LHR" {
		t.Fatalf("unexpected flight route key: %q", flightRouteKey(FlightSearchParams{Origin: " los", Destination: "LHR "}))
	}

	if hotelCityKey(HotelSearchParams{Destination: " London "}) != "london" {
		t.Fatalf("unexpected hotel city key: %q", hotelCityKey(HotelSearchParams{Destination: " London "}))
	}
}
