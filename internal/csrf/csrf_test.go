package csrf

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testPath = "/control/settings"

// okHandler records whether the wrapped handler ran and returns 200.
func okHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*called = true

		w.WriteHeader(http.StatusOK)
	})
}

func findCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()

	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return c
		}
	}

	return nil
}

func TestGenerateToken(t *testing.T) {
	t.Parallel()

	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	if len(token) != 64 {
		t.Errorf("token length = %d, want 64", len(token))
	}

	if _, err := hex.DecodeString(token); err != nil {
		t.Errorf("token %q is not hex: %v", token, err)
	}

	other, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	if token == other {
		t.Error("two generated tokens are identical")
	}
}

func TestSetCookieAttributes(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	SetCookie(rec, "example.com", true)

	cookie := findCookie(t, rec)
	if cookie == nil {
		t.Fatal("csrf cookie not set")
	}

	if cookie.Value == "" {
		t.Error("cookie value is empty")
	}

	if cookie.HttpOnly {
		t.Error("cookie must not be HttpOnly so the SPA can read it")
	}

	if !cookie.Secure {
		t.Error("cookie Secure = false, want true")
	}

	if cookie.Path != "/" {
		t.Errorf("cookie Path = %q, want /", cookie.Path)
	}

	if cookie.Domain != "example.com" {
		t.Errorf("cookie Domain = %q, want example.com", cookie.Domain)
	}

	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want Lax", cookie.SameSite)
	}

	if cookie.MaxAge != 0 {
		t.Errorf("cookie MaxAge = %d, want 0 (session-scoped)", cookie.MaxAge)
	}
}

func TestEnsureCookieSetsWhenMissing(t *testing.T) {
	t.Parallel()

	called := false
	handler := EnsureCookie("", false)(okHandler(&called))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))

	if !called {
		t.Error("next handler was not called")
	}

	if findCookie(t, rec) == nil {
		t.Error("csrf cookie not issued for request without one")
	}
}

func TestEnsureCookieKeepsExisting(t *testing.T) {
	t.Parallel()

	called := false
	handler := EnsureCookie("", false)(okHandler(&called))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "existing"})

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler was not called")
	}

	if c := findCookie(t, rec); c != nil {
		t.Errorf("cookie re-issued as %q, want existing cookie left untouched", c.Value)
	}
}

func TestMiddlewarePassesThrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "safe method on protected path", method: http.MethodGet, path: testPath},
		{name: "head on protected path", method: http.MethodHead, path: testPath},
		{name: "mutating method outside protected prefix", method: http.MethodPost, path: "/api/v1/content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			called := false
			handler := Middleware([]string{"/control/"}, nil)(okHandler(&called))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if !called {
				t.Error("next handler was not called")
			}

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		})
	}
}

func TestMiddlewareRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cookie string
		header string
	}{
		{name: "no cookie and no header"},
		{name: "cookie without header", cookie: "abc123"},
		{name: "header without cookie", header: "abc123"},
		{name: "mismatched token", cookie: "abc123", header: "def456"},
		{name: "empty cookie value", cookie: "", header: "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			called := false
			handler := Middleware([]string{"/control/"}, nil)(okHandler(&called))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, testPath, nil)

			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: tt.cookie})
			}

			if tt.header != "" {
				req.Header.Set(HeaderName, tt.header)
			}

			handler.ServeHTTP(rec, req)

			if called {
				t.Error("next handler ran despite invalid CSRF token")
			}

			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}

			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}

			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			if body.Error.Code != "csrf_token_invalid" {
				t.Errorf("error code = %q, want csrf_token_invalid", body.Error.Code)
			}
		})
	}
}

func TestMiddlewareAcceptsMatchingToken(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			called := false
			handler := Middleware([]string{"/control/"}, nil)(okHandler(&called))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, testPath, nil)
			req.AddCookie(&http.Cookie{Name: CookieName, Value: "matching-token"})
			req.Header.Set(HeaderName, "matching-token")

			handler.ServeHTTP(rec, req)

			if !called {
				t.Errorf("next handler was not called for %s with matching token", method)
			}

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		})
	}
}

func TestMiddlewareExemptFunc(t *testing.T) {
	t.Parallel()

	called := false
	handler := Middleware([]string{"/control/"}, IsNonCookieAuthenticated)(okHandler(&called))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, testPath, nil)
	req.Header.Set("Authorization", "Bearer token")

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("exempt request was rejected")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestIsNonCookieAuthenticated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		value  string
		want   bool
	}{
		{name: "no credential header", want: false},
		{name: "bearer token", header: "Authorization", value: "Bearer token", want: true},
		{name: "api key", header: "X-API-Key", value: "key", want: true},
		{name: "cookie header only", header: "Cookie", value: CookieName + "=abc", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, testPath, nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}

			if got := IsNonCookieAuthenticated(req); got != tt.want {
				t.Errorf("IsNonCookieAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenHandlerEchoesCookieToken(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	TokenHandler("example.com", true)(rec, httptest.NewRequest(http.MethodGet, "/csrf-token", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	cookie := findCookie(t, rec)
	if cookie == nil {
		t.Fatal("csrf cookie not set")
	}

	if body.Data.CSRFToken == "" {
		t.Fatal("response token is empty")
	}

	if body.Data.CSRFToken != cookie.Value {
		t.Errorf("body token %q != cookie token %q", body.Data.CSRFToken, cookie.Value)
	}
}

func TestTokenHandlerThenMiddlewareAccepts(t *testing.T) {
	t.Parallel()

	issue := httptest.NewRecorder()
	TokenHandler("", false)(issue, httptest.NewRequest(http.MethodGet, "/csrf-token", nil))

	cookie := findCookie(t, issue)
	if cookie == nil {
		t.Fatal("csrf cookie not set")
	}

	called := false
	handler := Middleware([]string{"/control/"}, nil)(okHandler(&called))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, testPath, nil)
	req.AddCookie(cookie)
	req.Header.Set(HeaderName, cookie.Value)

	handler.ServeHTTP(rec, req)

	if !called {
		t.Errorf("issued token was rejected by middleware (status %d)", rec.Code)
	}
}
