// Package costofliving — handlers_test.go tests the cost-of-living HTTP
// handlers and service helpers. Includes auth denial, staleness, and
// max_cities enforcement cases.
//
// Ref: japaconnect-engineering/sections/72-cost-of-living.md
//      japaconnect-engineering/adr/ADR-072-cost-of-living-calculator.md
package costofliving

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/google/uuid"
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

	_, err := svc.CompareCities(context.Background(), toPgUUID(uuid.New()), ids)
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
		_, err = svc.CompareCities(context.Background(), toPgUUID(uuid.New()), ids)
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
