package nextactions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/auth"
)

// runGuestMiddleware drives GuestSessionMiddleware with the given cookies
// and returns the Set-Cookie headers the middleware emitted.
func runGuestMiddleware(t *testing.T, cookies []*http.Cookie, authenticated bool) []*http.Cookie {
	t.Helper()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	if authenticated {
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.AuthenticatedUser{}))
	}
	rec := httptest.NewRecorder()
	GuestSessionMiddleware(false)(next).ServeHTTP(rec, req)

	return rec.Result().Cookies()
}

func TestGuestSessionMiddleware_MintsOnlyWithAffirmativeConsent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cookies     []*http.Cookie
		wantMint    bool
		wantExpired bool
	}{
		{
			name:        "no cookies — no mint (strict opt-in)",
			wantExpired: false,
		},
		{
			name:    "jc_consent=necessary — no mint",
			cookies: []*http.Cookie{{Name: ConsentCookieName, Value: "necessary"}},
		},
		{
			name:    "jc_consent with unknown value — no mint",
			cookies: []*http.Cookie{{Name: ConsentCookieName, Value: "yes"}},
		},
		{
			name:     "jc_consent=all — mints jc_guest",
			cookies:  []*http.Cookie{{Name: ConsentCookieName, Value: ConsentValueAll}},
			wantMint: true,
		},
		{
			name: "jc_consent=all + existing guest — no re-mint",
			cookies: []*http.Cookie{
				{Name: ConsentCookieName, Value: ConsentValueAll},
				{Name: GuestCookieName, Value: "existing-id"},
			},
		},
		{
			name:        "no consent + existing guest — expired",
			cookies:     []*http.Cookie{{Name: GuestCookieName, Value: "stale-id"}},
			wantExpired: true,
		},
		{
			name: "jc_consent=necessary + existing guest — expired (withdrawal)",
			cookies: []*http.Cookie{
				{Name: ConsentCookieName, Value: "necessary"},
				{Name: GuestCookieName, Value: "stale-id"},
			},
			wantExpired: true,
		},
		{
			name: "jc_consent=all + empty guest value — re-mints",
			cookies: []*http.Cookie{
				{Name: ConsentCookieName, Value: ConsentValueAll},
				{Name: GuestCookieName, Value: ""},
			},
			wantMint: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := runGuestMiddleware(t, tt.cookies, false)

			var guest *http.Cookie
			for _, c := range got {
				if c.Name == GuestCookieName {
					guest = c
				}
			}

			switch {
			case tt.wantMint:
				if guest == nil {
					t.Fatal("expected jc_guest to be minted, got no Set-Cookie")
				}
				if guest.Value == "" || guest.MaxAge <= 0 || !guest.HttpOnly {
					t.Fatalf("minted cookie malformed: %+v", guest)
				}
			case tt.wantExpired:
				if guest == nil {
					t.Fatal("expected jc_guest expiry cookie, got no Set-Cookie")
				}
				if guest.MaxAge >= 0 {
					t.Fatalf("expected negative MaxAge expiry, got %+v", guest)
				}
			default:
				if guest != nil {
					t.Fatalf("expected no jc_guest Set-Cookie, got %+v", guest)
				}
			}
		})
	}
}

func TestGuestSessionMiddleware_SkipsAuthenticatedRequests(t *testing.T) {
	t.Parallel()

	// An authenticated request with no consent and a stale jc_guest must be
	// left untouched — guest cookies are irrelevant once a session exists.
	got := runGuestMiddleware(t, []*http.Cookie{{Name: GuestCookieName, Value: "stale-id"}}, true)
	for _, c := range got {
		if c.Name == GuestCookieName {
			t.Fatalf("expected no jc_guest Set-Cookie for authenticated request, got %+v", c)
		}
	}
}
