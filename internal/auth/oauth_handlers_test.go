package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (OAuth social login)
// These tests cover the callback redirect behavior and pure helper functions.
// The full account-creation + token-issuance path requires a real DB and
// KeyRotation, and is covered by integration tests.

// --- Pure function tests ---

func TestSanitizeUsernameBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email string
		nm    string
		want  string
	}{
		{"email local part", "john.doe@gmail.com", "", "john_doe"},
		{"name fallback when no email", "", "John Doe", "john_doe"},
		{"empty both", "", "", "user"},
		{"special chars stripped", "user+test@example.com", "", "user_test"},
		{"truncated to 20", "verylongemailaddress@example.com", "", "verylongemailaddress"},
		{"hyphens to underscores", "my-name@example.com", "", "my_name"},
		{"uppercase lowercased", "John.Doe@Example.COM", "", "john_doe"},
		{"leading trailing underscores trimmed", "...test@example.com", "", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeUsernameBase(tt.email, tt.nm)
			if got != tt.want {
				t.Errorf("sanitizeUsernameBase(%q, %q) = %q, want %q", tt.email, tt.nm, got, tt.want)
			}
		})
	}
}

func TestRandomSuffix(t *testing.T) {
	t.Parallel()

	s1, err := randomSuffix(6)
	if err != nil {
		t.Fatalf("randomSuffix(6) error: %v", err)
	}
	if len(s1) != 12 { // 6 bytes → 12 hex chars
		t.Errorf("randomSuffix(6) len = %d, want 12", len(s1))
	}

	s2, _ := randomSuffix(6)
	if s1 == s2 {
		t.Error("two consecutive randomSuffix calls produced the same value")
	}
}

// --- HandleCallback redirect/error path tests ---
// These exercise the paths that redirect to /login?oauth_error= without
// reaching the DB or KeyRotation.

func newOAuthTestHandlers(frontendBase string) *OAuthHandlers {
	return &OAuthHandlers{
		Config: OAuthConfig{
			GoogleClientID:     "test-google-id",
			GoogleClientSecret: "test-google-secret",
			RedirectBase:       "http://localhost:8080",
		},
		FrontendBaseURL: frontendBase,
		SecureCookies:   false,
	}
}

func TestHandleCallback_UnsupportedProvider_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	r := chi.NewRouter()
	r.Get("/oauth/{provider}/callback", h.HandleCallback)

	req := httptest.NewRequest(http.MethodGet, "/oauth/notreal/callback?code=x&state=y", nil)
	req.AddCookie(&http.Cookie{Name: "jc_oauth_state", Value: "y"})
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if loc != "http://localhost:3000/login?oauth_error=unsupported_provider" {
		t.Errorf("Location = %q, want redirect with unsupported_provider", loc)
	}
}

func TestHandleCallback_MissingStateCookie_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	r := chi.NewRouter()
	r.Get("/oauth/{provider}/callback", h.HandleCallback)

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=x&state=y", nil)
	// No state cookie
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if loc != "http://localhost:3000/login?oauth_error=missing_state" {
		t.Errorf("Location = %q, want missing_state redirect", loc)
	}
}

func TestHandleCallback_InvalidState_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	r := chi.NewRouter()
	r.Get("/oauth/{provider}/callback", h.HandleCallback)

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=x&state=wrong", nil)
	req.AddCookie(&http.Cookie{Name: "jc_oauth_state", Value: "correct"})
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if loc != "http://localhost:3000/login?oauth_error=invalid_state" {
		t.Errorf("Location = %q, want invalid_state redirect", loc)
	}
}

func TestHandleCallback_ProviderErrorParam_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	r := chi.NewRouter()
	r.Get("/oauth/{provider}/callback", h.HandleCallback)

	// Valid state cookie, but provider returns error param
	state := "test-state"
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?error=access_denied&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "jc_oauth_state", Value: state})
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if loc != "http://localhost:3000/login?oauth_error=oauth_error" {
		t.Errorf("Location = %q, want oauth_error redirect", loc)
	}
}

func TestHandleCallback_MissingCode_RedirectsToLogin(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	r := chi.NewRouter()
	r.Get("/oauth/{provider}/callback", h.HandleCallback)

	state := "test-state"
	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "jc_oauth_state", Value: state})
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if loc != "http://localhost:3000/login?oauth_error=missing_code" {
		t.Errorf("Location = %q, want missing_code redirect", loc)
	}
}

// --- HandleInitiate tests ---

func TestHandleInitiate_SetsStateAndRedirectCookies(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	r := chi.NewRouter()
	r.Post("/oauth/{provider}/initiate", h.HandleInitiate)

	req := httptest.NewRequest(http.MethodPost, "/oauth/google/initiate?redirect=/dashboard", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var stateCookie, redirectCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case "jc_oauth_state":
			stateCookie = c
		case "jc_oauth_redirect":
			redirectCookie = c
		}
	}

	if stateCookie == nil {
		t.Error("jc_oauth_state cookie not set")
	} else if stateCookie.Value == "" {
		t.Error("jc_oauth_state cookie value is empty")
	}

	if redirectCookie == nil {
		t.Error("jc_oauth_redirect cookie not set")
	} else if redirectCookie.Value != "/dashboard" {
		t.Errorf("jc_oauth_redirect = %q, want /dashboard", redirectCookie.Value)
	}
}

func TestHandleInitiate_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	r := chi.NewRouter()
	r.Post("/oauth/{provider}/initiate", h.HandleInitiate)

	req := httptest.NewRequest(http.MethodPost, "/oauth/notreal/initiate", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleInitiate_DefaultRedirectToRoot(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	r := chi.NewRouter()
	r.Post("/oauth/{provider}/initiate", h.HandleInitiate)

	req := httptest.NewRequest(http.MethodPost, "/oauth/google/initiate", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var redirectCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "jc_oauth_redirect" {
			redirectCookie = c
		}
	}

	if redirectCookie == nil {
		t.Fatal("jc_oauth_redirect cookie not set")
	}
	if redirectCookie.Value != "/" {
		t.Errorf("jc_oauth_redirect = %q, want /", redirectCookie.Value)
	}
}

// --- Redirect helper tests ---

func TestRedirectToFrontend(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	h.redirectToFrontend(rec, req, "/onboarding")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "http://localhost:3000/onboarding" {
		t.Errorf("Location = %q, want http://localhost:3000/onboarding", loc)
	}
}

func TestRedirectToFrontend_TrimsTrailingSlash(t *testing.T) {
	t.Parallel()

	h := &OAuthHandlers{FrontendBaseURL: "http://localhost:3000/"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	h.redirectToFrontend(rec, req, "/dashboard")

	if loc := rec.Header().Get("Location"); loc != "http://localhost:3000/dashboard" {
		t.Errorf("Location = %q, want http://localhost:3000/dashboard", loc)
	}
}

func TestRedirectOAuthError_EncodesCode(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	h.redirectOAuthError(rec, req, "invalid_state")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "http://localhost:3000/login?oauth_error=invalid_state" {
		t.Errorf("Location = %q, want login with oauth_error", loc)
	}
}

// --- ClearOAuthCookies test ---

func TestClearOAuthCookies(t *testing.T) {
	t.Parallel()

	h := newOAuthTestHandlers("http://localhost:3000")
	rec := httptest.NewRecorder()

	h.clearOAuthCookies(rec)

	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected 2 cleared cookies, got %d", len(cookies))
	}

	for _, c := range cookies {
		if c.Name != "jc_oauth_state" && c.Name != "jc_oauth_redirect" {
			t.Errorf("unexpected cookie %q", c.Name)
		}
		if c.MaxAge != -1 {
			t.Errorf("cookie %q MaxAge = %d, want -1 (expired)", c.Name, c.MaxAge)
		}
	}
}

// --- Mock OAuthStore for future integration tests ---

type mockOAuthStore struct {
	oauthAccounts map[string]*OAuthAccount // key: provider:providerUserID
	usersByEmail  map[string]*User         // key: email
	linkCalls     int
}

func newMockOAuthStore() *mockOAuthStore {
	return &mockOAuthStore{
		oauthAccounts: make(map[string]*OAuthAccount),
		usersByEmail:  make(map[string]*User),
	}
}

func (m *mockOAuthStore) GetOAuthAccount(_ context.Context, _ pgtype.UUID, provider, providerUserID string) (*OAuthAccount, error) {
	if acct, ok := m.oauthAccounts[provider+":"+providerUserID]; ok {
		return acct, nil
	}
	return nil, errAccountNotFound
}

func (m *mockOAuthStore) GetOAuthAccountByUser(_ context.Context, _, _ pgtype.UUID, _ string) (*OAuthAccount, error) {
	return nil, errAccountNotFound
}

func (m *mockOAuthStore) LinkOAuthAccount(_ context.Context, _, _ pgtype.UUID, _, _ string) error {
	m.linkCalls++
	return nil
}

func (m *mockOAuthStore) GetUserByEmail(_ context.Context, _ pgtype.UUID, email string) (*User, error) {
	if u, ok := m.usersByEmail[email]; ok {
		return u, nil
	}
	return nil, errUserNotFound
}

var errAccountNotFound = errors.New("oauth account not found")

// --- safeRedirectPath tests (open-redirect prevention, CWE-601) ---

func TestSafeRedirectPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty defaults to root", "", "/"},
		{"normal relative path", "/dashboard", "/dashboard"},
		{"nested relative path", "/onboarding/step-2", "/onboarding/step-2"},
		{"root path", "/", "/"},
		{"protocol-relative http", "//evil.com", "/"},
		{"protocol-relative https", "//https://evil.com", "/"},
		{"absolute http URL", "http://evil.com", "/"},
		{"absolute https URL", "https://evil.com/path", "/"},
		{"no leading slash", "dashboard", "/"},
		{"backslash not treated as path", "\\evil.com", "/"},
		{"data URI", "data:text/html,<script>", "/"},
		{"javascript URI", "javascript:alert(1)", "/"},
		{"path with query string", "/login?next=/dashboard", "/login?next=/dashboard"},
		{"path with fragment", "/dashboard#section", "/dashboard#section"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := safeRedirectPath(tt.path)
			if got != tt.want {
				t.Fatalf("safeRedirectPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

var errUserNotFound = errors.New("user not found")
