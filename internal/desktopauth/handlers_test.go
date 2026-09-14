package desktopauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/publicapi"
)

type fakeAPIKeyValidator struct {
	keyCtx      *publicapi.APIKeyContext
	validateErr error
	allowed     bool
	ipErr       error
}

func (f *fakeAPIKeyValidator) ValidateAPIKey(ctx context.Context, key string) (*publicapi.APIKeyContext, error) {
	if f.validateErr != nil {
		return nil, f.validateErr
	}

	return f.keyCtx, nil
}

func (f *fakeAPIKeyValidator) HasIPAccess(ctx context.Context, keyID pgtype.UUID, clientIP string) (bool, error) {
	if f.ipErr != nil {
		return false, f.ipErr
	}

	return f.allowed, nil
}

type fakeRefreshCreator struct {
	calls []gen.CreateRefreshTokenParams
	err   error
}

func (f *fakeRefreshCreator) CreateRefreshToken(ctx context.Context, arg gen.CreateRefreshTokenParams) error {
	f.calls = append(f.calls, arg)
	return f.err
}

type fakeSettingsReader struct{}

func (f *fakeSettingsReader) GetSettingInt(ctx context.Context, key string) (int, error) {
	return 0, nil
}

func (f *fakeSettingsReader) GetSettingBool(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (f *fakeSettingsReader) GetIntOr(ctx context.Context, key string, fallback int) int {
	return fallback
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	u, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}

	return pgtype.UUID{Bytes: u, Valid: true}
}

func newTestKeyRotation(t *testing.T) *auth.KeyRotation {
	kr := auth.NewKeyRotation("test", []byte("super-secret-key-for-tests-only-32b"))
	kr.Settings = &fakeSettingsReader{}

	return kr
}

func TestExchangeKey_MissingKey(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodPost, "/auth/exchange-key", nil)
	rr := httptest.NewRecorder()
	h.ExchangeKey(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	assertErrorCode(t, rr.Body.String(), "missing_api_key")
}

func TestExchangeKey_InvalidKey(t *testing.T) {
	v := &fakeAPIKeyValidator{validateErr: errors.New("not found")}
	h := &Handlers{APIKeys: v}
	req := httptest.NewRequest(http.MethodPost, "/auth/exchange-key", nil)
	req.Header.Set("X-API-Key", "invalid-key")

	rr := httptest.NewRecorder()
	h.ExchangeKey(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	assertErrorCode(t, rr.Body.String(), "invalid_api_key")
}

func TestExchangeKey_InsufficientScope(t *testing.T) {
	uid := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	ctx := &publicapi.APIKeyContext{
		KeyID:    uid,
		UserID:   uid,

		Scopes: []string{"api.read.content"},
	}
	h := &Handlers{APIKeys: &fakeAPIKeyValidator{keyCtx: ctx, allowed: true}}
	req := httptest.NewRequest(http.MethodPost, "/auth/exchange-key", nil)
	req.Header.Set("X-API-Key", "valid-key")

	rr := httptest.NewRecorder()
	h.ExchangeKey(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	assertErrorCode(t, rr.Body.String(), "insufficient_scope")
}

func TestExchangeKey_IPNotAllowed(t *testing.T) {
	uid := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	ctx := &publicapi.APIKeyContext{
		KeyID:    uid,
		UserID:   uid,

		Scopes: []string{"api.write.content"},
	}
	v := &fakeAPIKeyValidator{keyCtx: ctx, allowed: false}
	h := &Handlers{APIKeys: v}
	req := httptest.NewRequest(http.MethodPost, "/auth/exchange-key", nil)
	req.Header.Set("X-API-Key", "valid-key")
	req.RemoteAddr = "203.0.113.1:1234"
	rr := httptest.NewRecorder()
	h.ExchangeKey(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	assertErrorCode(t, rr.Body.String(), "ip_not_allowed")
}

func TestExchangeKey_Success(t *testing.T) {
	uid := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	ctx := &publicapi.APIKeyContext{
		KeyID:    uid,
		UserID:   uid,

		Scopes: []string{"api.write.content"},
	}
	q := &fakeRefreshCreator{}
	h := &Handlers{
		APIKeys:     &fakeAPIKeyValidator{keyCtx: ctx, allowed: true},
		KeyRotation: newTestKeyRotation(t),
		Queries:     q,
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/exchange-key", nil)
	req.Header.Set("X-API-Key", "valid-key")
	req.Header.Set("X-Device-Id", "my-device")
	req.RemoteAddr = "203.0.113.2:1234"
	rr := httptest.NewRecorder()
	h.ExchangeKey(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ExchangeKeyResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.TokenType != "bearer" {
		t.Fatalf("expected bearer token type, got %q", resp.TokenType)
	}

	if resp.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}

	if resp.RefreshToken == "" {
		t.Fatal("expected non-empty refresh token")
	}

	if resp.ExpiresIn == 0 {
		t.Fatal("expected non-zero expires_in")
	}

	if len(q.calls) != 1 {
		t.Fatalf("expected 1 refresh token created, got %d", len(q.calls))
	}

	created := q.calls[0]
	if created.UserID != uid {
		t.Fatal("refresh token user ID mismatch")
	}

	if created.DeviceName == nil || *created.DeviceName != "my-device" {
		t.Fatalf("expected device name my-device, got %v", created.DeviceName)
	}

	if created.IpAddress == nil || !strings.HasPrefix(*created.IpAddress, "203.0.113") {
		t.Fatalf("expected IP address 203.0.113.*, got %v", created.IpAddress)
	}

	if created.TokenHash == "" {
		t.Fatal("expected non-empty token hash")
	}
}

func TestExchangeKey_WildcardScope(t *testing.T) {
	uid := mustUUID(t, "11111111-1111-1111-1111-111111111111")

	for _, scopes := range [][]string{
		{"*"},
		{"api.write.*"},
		{"api.read.*", "api.write.*"},
	} {
		ctx := &publicapi.APIKeyContext{
			KeyID:    uid,
			UserID:   uid,
	
			Scopes: scopes,
		}
		q := &fakeRefreshCreator{}
		h := &Handlers{
			APIKeys:     &fakeAPIKeyValidator{keyCtx: ctx, allowed: true},
			KeyRotation: newTestKeyRotation(t),
			Queries:     q,
		}
		req := httptest.NewRequest(http.MethodPost, "/auth/exchange-key", nil)
		req.Header.Set("X-API-Key", "valid-key")

		rr := httptest.NewRecorder()
		h.ExchangeKey(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for scopes %v, got %d: %s", scopes, rr.Code, rr.Body.String())
		}
	}
}

func TestExchangeKey_BodyFallback(t *testing.T) {
	uid := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	ctx := &publicapi.APIKeyContext{
		KeyID:    uid,
		UserID:   uid,

		Scopes: []string{"api.write.content"},
	}
	h := &Handlers{
		APIKeys:     &fakeAPIKeyValidator{keyCtx: ctx, allowed: true},
		KeyRotation: newTestKeyRotation(t),
		Queries:     &fakeRefreshCreator{},
	}
	body := strings.NewReader(`{"api_key":"fallback-key"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/exchange-key", body)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ExchangeKey(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func assertErrorCode(t *testing.T, body, code string) {
	t.Helper()

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal([]byte(body), &errResp); err != nil {
		t.Fatalf("decode error body: %v", err)
	}

	if errResp.Error.Code != code {
		t.Fatalf("expected error code %q, got %q", code, errResp.Error.Code)
	}
}
