package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRequireAuth_NoToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	handler := RequireAuth(kr, nil)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called without token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_InvalidScheme(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	handler := RequireAuth(kr, nil)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic abc123")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_ValidToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	tenantID := pgtype.UUID{Valid: true}

	token, _ := kr.MintAccessToken(userID, tenantID)

	called := false
	handler := RequireAuth(kr, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true

		user := FromContext(r.Context())
		if user == nil {
			t.Error("expected user in context")
			return
		}

		if !user.UserID.Valid {
			t.Error("expected valid user ID")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	handler := RequireAuth(kr, nil)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOptionalAuth_NoToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	called := false
	handler := OptionalAuth(kr, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true

		if user := FromContext(r.Context()); user != nil {
			t.Error("expected no user without token")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called")
	}
}

func TestOptionalAuth_ValidToken(t *testing.T) {
	t.Parallel()

	kr := NewKeyRotation("key-test", []byte("test-secret-key-32-bytes-long!!!"))
	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	tenantID := pgtype.UUID{Valid: true}
	token, _ := kr.MintAccessToken(userID, tenantID)

	handler := OptionalAuth(kr, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		user := FromContext(r.Context())
		if user == nil {
			t.Error("expected user in context with valid token")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}
