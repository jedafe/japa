package publicapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type stubTierResolver struct{ tier string }

func (s stubTierResolver) ResolveUserTier(_ context.Context, _ pgtype.UUID) (string, error) {
	return s.tier, nil
}

func requireTierReq(tier string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/public/x", nil)
	return r.WithContext(context.WithValue(r.Context(), apiKeyContextKey, &APIKeyContext{UserID: pgtype.UUID{Valid: true}}))
}

func TestRequireTier(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	cases := []struct {
		name     string
		userTier string
		minTier  string
		want     int
	}{
		{"regular denied pro endpoint", "regular", "pro", http.StatusForbidden},
		{"pro allowed pro endpoint", "pro", "pro", http.StatusNoContent},
		{"business allowed pro endpoint", "business", "pro", http.StatusNoContent},
		{"unknown tier denied", "gold", "pro", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			RequireTier(stubTierResolver{tier: c.userTier}, c.minTier)(ok).ServeHTTP(rec, requireTierReq(c.userTier))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}

	t.Run("no api key -> 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/public/x", nil)
		RequireTier(stubTierResolver{tier: "pro"}, "pro")(ok).ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}
