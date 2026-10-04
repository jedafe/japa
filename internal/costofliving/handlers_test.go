// Package costofliving — handlers_test.go tests the cost-of-living HTTP
// handlers and service helpers. Includes auth denial, staleness, and
// max_cities enforcement cases.
//
// Ref: japaconnect-engineering/sections/72-cost-of-living.md
//
//	japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md
package costofliving

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestHandleAdminStats_Unauthorized verifies that the admin stats endpoint
// requires authentication. The RBAC middleware is applied at the route
// registration site, so this test verifies the handler's own auth check.
func TestHandleAdminStats_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/calculator/cost-of-living/stats", nil)
	// No auth context set → requireUser should 401.
	rr := httptest.NewRecorder()
	h.HandleAdminStats(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminStats without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestIsStale verifies the staleness check against the col_calculator.stale_days
// threshold (default 180 days per Section 72).
func TestIsStale(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		lastUpdated time.Time
		staleDays   int
		want        bool
	}{
		{
			name:        "200 days old with stale_days=180 is stale",
			lastUpdated: now.AddDate(0, 0, -200),
			staleDays:   180,
			want:        true,
		},
		{
			name:        "100 days old with stale_days=180 is fresh",
			lastUpdated: now.AddDate(0, 0, -100),
			staleDays:   180,
			want:        false,
		},
		{
			name:        "exactly at threshold (180 days) is not stale (Before is exclusive)",
			lastUpdated: now.AddDate(0, 0, -180),
			staleDays:   180,
			want:        false,
		},
		{
			name:        "181 days old with stale_days=180 is stale",
			lastUpdated: now.AddDate(0, 0, -181),
			staleDays:   180,
			want:        true,
		},
		{
			name:        "zero timestamp with stale_days=180 is stale",
			lastUpdated: time.Time{},
			staleDays:   180,
			want:        true,
		},
		{
			name:        "future timestamp is fresh",
			lastUpdated: now.AddDate(0, 0, 10),
			staleDays:   180,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsStaleAt(tt.lastUpdated, now, tt.staleDays)
			if got != tt.want {
				t.Errorf("IsStaleAt(%s, %s, %d) = %v, want %v",
					tt.lastUpdated.Format(time.RFC3339), now.Format(time.RFC3339), tt.staleDays, got, tt.want)
			}
		})
	}
}

// toPgUUID converts a uuid.UUID to a pgtype.UUID.
func toPgUUID(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: true}
}

// TestMaxCitiesEnforcement verifies that CompareCities rejects more than the
// configured col_calculator.max_cities limit. The service is constructed with
// a nil store (so the DB is never hit) — the max_cities check runs before any
// store call. A settings instance is not required because the service falls
// back to the default max_cities (3) when settings is nil.
func TestMaxCitiesEnforcement(t *testing.T) {
	svc := &Service{store: nil, settings: nil}

	// Build 5 city IDs — exceeds the default max_cities of 3.
	ids := make([]pgtype.UUID, 5)
	for i := range ids {
		ids[i] = toPgUUID(uuid.New())
	}

	_, err := svc.CompareCities(t.Context(), ids, pgtype.UUID{})
	if err == nil {
		t.Fatal("CompareCities with 5 cities (> max 3) did not return an error")
	}

	if !containsStr(err.Error(), "too many cities") {
		t.Errorf("CompareCities error = %q, want it to contain \"too many cities\"", err.Error())
	}
}

// TestCompareCities_AtMaxAccepts verifies that CompareCities does not reject
// when the count equals max_cities. With a nil store the call proceeds past
// the max_cities check and panics on the nil store dereference — we recover
// and assert the panic is NOT the max_cities rejection (i.e. the count check
// passed). This confirms the boundary: count == max is accepted.
func TestCompareCities_AtMaxAccepts(t *testing.T) {
	svc := &Service{store: nil, settings: nil}

	ids := make([]pgtype.UUID, 3)
	for i := range ids {
		ids[i] = toPgUUID(uuid.New())
	}

	var err error

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Nil store dereference — expected since we only want to
				// prove the max_cities check passed. Synthesize a non-
				// max_cities error so the assertion below holds.
				err = errNotNilStore
			}
		}()

		_, err = svc.CompareCities(t.Context(), ids, pgtype.UUID{})
	}()

	if err != nil && containsStr(err.Error(), "too many cities") {
		t.Errorf("CompareCities at max should not be rejected as too many cities, got: %v", err)
	}
}

// errNotNilStore is a sentinel used by TestCompareCities_AtMaxAccepts to
// represent the expected nil-store failure path (the max_cities check passed).
var errNotNilStore = errors.New("nil store reached")

// TestParseWeights verifies the config JSONB weight parsing.
func TestParseWeights(t *testing.T) {
	t.Run("nil config returns empty map", func(t *testing.T) {
		w := parseWeights(nil)
		if len(w) != 0 {
			t.Errorf("parseWeights(nil) = %v, want empty map", w)
		}
	})

	t.Run("valid config returns weights", func(t *testing.T) {
		cfg := []byte(`{"weights": {"dim-1": 0.25, "dim-2": 0.75}}`)

		w := parseWeights(cfg)
		if w["dim-1"] != 0.25 {
			t.Errorf("parseWeights dim-1 = %v, want 0.25", w["dim-1"])
		}

		if w["dim-2"] != 0.75 {
			t.Errorf("parseWeights dim-2 = %v, want 0.75", w["dim-2"])
		}
	})

	t.Run("malformed config returns empty map", func(t *testing.T) {
		w := parseWeights([]byte("{not json"))
		if len(w) != 0 {
			t.Errorf("parseWeights(malformed) = %v, want empty map", w)
		}
	})

	t.Run("config without weights key returns empty map", func(t *testing.T) {
		w := parseWeights([]byte(`{"other": 1}`))
		if len(w) != 0 {
			t.Errorf("parseWeights(no weights key) = %v, want empty map", w)
		}
	})
}

// TestParseFloatValue verifies numeric cost value parsing from text/numeric.
func TestParseFloatValue(t *testing.T) {
	t.Run("text value parses", func(t *testing.T) {
		s := "1200.50"

		f, ok := parseFloatValue(&s, pgtype.Numeric{})
		if !ok {
			t.Fatal("parseFloatValue(&\"1200.50\", ...) ok = false, want true")
		}

		if f != 1200.50 {
			t.Errorf("parseFloatValue = %v, want 1200.50", f)
		}
	})

	t.Run("text value with currency prefix parses", func(t *testing.T) {
		s := "$800"

		f, ok := parseFloatValue(&s, pgtype.Numeric{})
		if !ok {
			t.Fatal("parseFloatValue(&\"$800\", ...) ok = false, want true")
		}

		if f != 800 {
			t.Errorf("parseFloatValue = %v, want 800", f)
		}
	})

	t.Run("empty text value returns false", func(t *testing.T) {
		s := ""

		_, ok := parseFloatValue(&s, pgtype.Numeric{})
		if ok {
			t.Error("parseFloatValue(&\"\", ...) ok = true, want false")
		}
	})

	t.Run("nil text value returns false", func(t *testing.T) {
		_, ok := parseFloatValue(nil, pgtype.Numeric{})
		if ok {
			t.Error("parseFloatValue(nil, ...) ok = true, want false")
		}
	})

	t.Run("non-numeric text returns false", func(t *testing.T) {
		s := "n/a"

		_, ok := parseFloatValue(&s, pgtype.Numeric{})
		if ok {
			t.Error("parseFloatValue(&\"n/a\", ...) ok = true, want false")
		}
	})
}

// containsStr is a small helper for substring checks in tests.
func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

// TestParsePageLimit verifies the page/limit parsing including the max cap.
func TestParsePageLimit(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantPage  int
		wantLimit int
	}{
		{"defaults", "", 1, 20},
		{"custom page and limit", "?page=3&limit=50", 3, 50},
		{"limit exceeds cap", "?limit=999999", 1, 200},
		{"invalid page falls back", "?page=-1&limit=10", 1, 10},
		{"invalid limit falls back", "?page=2&limit=abc", 2, 20},
		{"zero page falls back", "?page=0", 1, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test"+tt.query, nil)

			page, limit := parsePageLimit(req)
			if page != tt.wantPage {
				t.Errorf("page = %d, want %d", page, tt.wantPage)
			}

			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
		})
	}
}

// TestParseCityIDs verifies the city_ids query parameter parsing.
func TestParseCityIDs(t *testing.T) {
	t.Run("missing city_ids returns error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/compare", nil)
		rr := httptest.NewRecorder()

		_, ok := parseCityIDs(rr, req)
		if ok {
			t.Error("parseCityIDs with no param should return ok=false")
		}

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid UUID returns error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/compare?city_ids=not-a-uuid", nil)
		rr := httptest.NewRecorder()

		_, ok := parseCityIDs(rr, req)
		if ok {
			t.Error("parseCityIDs with invalid UUID should return ok=false")
		}
	})

	t.Run("valid UUIDs parse correctly", func(t *testing.T) {
		u1 := uuid.New()
		u2 := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/compare?city_ids="+u1.String()+","+u2.String(), nil)
		rr := httptest.NewRecorder()
		ids, ok := parseCityIDs(rr, req)

		if !ok {
			t.Fatalf("parseCityIDs with valid UUIDs should return ok=true, got error: %s", rr.Body.String())
		}

		if len(ids) != 2 {
			t.Errorf("len(ids) = %d, want 2", len(ids))
		}
	})

	t.Run("empty parts are skipped", func(t *testing.T) {
		u1 := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/compare?city_ids="+u1.String()+",,", nil)
		rr := httptest.NewRecorder()
		ids, ok := parseCityIDs(rr, req)

		if !ok {
			t.Fatalf("parseCityIDs should succeed with trailing commas, got: %s", rr.Body.String())
		}

		if len(ids) != 1 {
			t.Errorf("len(ids) = %d, want 1 (empty parts skipped)", len(ids))
		}
	})
}

// TestCompareCities_Dedup verifies that duplicate city IDs are deduplicated
// before the max_cities check, so that e.g. 3 copies of the same ID + 1 other
// doesn't trigger "too many cities" (only 2 unique).
func TestCompareCities_Dedup(t *testing.T) {
	svc := &Service{store: nil, settings: nil}

	// 4 IDs but only 2 unique — should pass the max_cities check (max=3).
	id := toPgUUID(uuid.New())
	other := toPgUUID(uuid.New())
	ids := []pgtype.UUID{id, id, id, other}

	var err error

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Nil store dereference — expected; the dedup + max check passed.
				err = errNotNilStore
			}
		}()

		_, err = svc.CompareCities(t.Context(), ids, pgtype.UUID{})
	}()

	if err != nil && containsStr(err.Error(), "too many cities") {
		t.Errorf("CompareCities with 2 unique IDs (4 raw with dups) should not be rejected as too many cities, got: %v", err)
	}
}

// TestCompareCities_TooManyUnique verifies that unique IDs exceeding max_cities
// are still rejected after dedup.
func TestCompareCities_TooManyUnique(t *testing.T) {
	svc := &Service{store: nil, settings: nil}

	// 4 unique IDs — exceeds max_cities of 3.
	ids := make([]pgtype.UUID, 4)
	for i := range ids {
		ids[i] = toPgUUID(uuid.New())
	}

	_, err := svc.CompareCities(t.Context(), ids, pgtype.UUID{})
	if err == nil {
		t.Fatal("CompareCities with 4 unique cities (> max 3) did not return an error")
	}

	if !containsStr(err.Error(), "too many cities") {
		t.Errorf("error = %q, want it to contain \"too many cities\"", err.Error())
	}
}

// TestCompareCities_BaselineExemptFromMax verifies that the requested
// baseline_id does not consume a destination slot: 3 destinations + the
// baseline inside city_ids (4 unique total) must pass the max=3 check.
// This is the shape the JCE export form posts.
func TestCompareCities_BaselineExemptFromMax(t *testing.T) {
	svc := &Service{store: nil, settings: nil}

	baseline := toPgUUID(uuid.New())
	ids := []pgtype.UUID{baseline, toPgUUID(uuid.New()), toPgUUID(uuid.New()), toPgUUID(uuid.New())}

	var err error

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Nil store dereference — expected; the max check passed.
				err = errNotNilStore
			}
		}()

		_, err = svc.CompareCities(t.Context(), ids, baseline)
	}()

	if err != nil && containsStr(err.Error(), "too many cities") {
		t.Errorf("CompareCities with baseline + 3 destinations should not be rejected as too many cities, got: %v", err)
	}
}
