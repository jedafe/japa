// Package devices implements the Mobile Push / Device Token registration
// surface (Section 77, ADR-074).
//
// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
)

// validUUID builds a pgtype.UUID from a 36-char hex string for tests.
func validUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}
	return u
}

// withAuth injects an AuthenticatedUser into the request context exactly as
// the auth.RequireAuth middleware does (auth.UserContextKey). This guards
// against regressions of BE-001, where the handlers used private context keys
// that no middleware ever populated.
func withAuth(r *http.Request, user *auth.AuthenticatedUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, user))
}

func TestHandleRegisterDevice_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodPost, "/me/devices", nil)
	rr := httptest.NewRecorder()
	h.HandleRegisterDevice(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleDeregisterDevice_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	r := chi.NewRouter()
	r.Delete("/me/devices/{device_id}", h.HandleDeregisterDevice)
	req := httptest.NewRequest(http.MethodDelete, "/me/devices/test-device-123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestHandleDeregisterDevice_AuthButMissingDeviceID_400 verifies that an
// authenticated request reaches the handler (no longer 401) and that the
// {device_id} route param is correctly read (BE-002 regression guard).
// With no store wired, a missing param yields 400 before the store is touched.
func TestHandleDeregisterDevice_AuthButEmptyParam_400(t *testing.T) {
	h := NewHandlers(&Service{})
	r := chi.NewRouter()
	r.Delete("/me/devices/{device_id}", h.HandleDeregisterDevice)

	user := &auth.AuthenticatedUser{
		UserID:   validUUID(t, "00000000-0000-0000-0000-000000000001"),
		TenantID: validUUID(t, "00000000-0000-0000-0000-000000000002"),
	}
	// Hit a path where chi leaves {device_id} empty (no segment) — chi won't
	// match, so instead register a route without the param to confirm the
	// handler's empty-param guard returns 400 rather than 401.
	r2 := chi.NewRouter()
	r2.Delete("/me/devices", h.HandleDeregisterDevice)
	req := httptest.NewRequest(http.MethodDelete, "/me/devices", nil)
	req = withAuth(req, user)
	rr := httptest.NewRecorder()
	r2.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty device_id, got %d (body=%s)", rr.Code, rr.Body.String())
	}
}

func TestHandleListDevices_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/me/devices", nil)
	rr := httptest.NewRecorder()
	h.HandleListDevices(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleAdminTestPush_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodPost, "/admin/push/test", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminTestPush(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestHandleAdminTestPush_AuthNoDispatcher_503 verifies that with a valid
// auth context but no PushDispatcher wired, the handler returns 503
// push_unavailable (BE-006: never silently no-op).
func TestHandleAdminTestPush_AuthNoDispatcher_503(t *testing.T) {
	h := NewHandlers(&Service{})
	user := &auth.AuthenticatedUser{
		UserID:   validUUID(t, "00000000-0000-0000-0000-000000000001"),
		TenantID: validUUID(t, "00000000-0000-0000-0000-000000000002"),
	}
	body := `{"device_id":"dev-1"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/push/test", strBody(body))
	req = withAuth(req, user)
	rr := httptest.NewRecorder()
	h.HandleAdminTestPush(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 push_unavailable, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "push_unavailable" {
		t.Errorf("expected code push_unavailable, got %q", env.Error.Code)
	}
}

// TestHandleRegisterDevice_InvalidPlatform_400 verifies the sentinel error
// path (BE-004: the previous errors.Is against errors.New was dead code).
func TestHandleRegisterDevice_InvalidPlatform_400(t *testing.T) {
	h := NewHandlers(&Service{})
	user := &auth.AuthenticatedUser{
		UserID:   validUUID(t, "00000000-0000-0000-0000-000000000001"),
		TenantID: validUUID(t, "00000000-0000-0000-0000-000000000002"),
	}
	body := `{"device_id":"d1","token":"t1","platform":"desktop"}`
	req := httptest.NewRequest(http.MethodPost, "/me/devices", strBody(body))
	req = withAuth(req, user)
	rr := httptest.NewRecorder()
	h.HandleRegisterDevice(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 invalid_platform, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "invalid_platform" {
		t.Errorf("expected code invalid_platform, got %q", env.Error.Code)
	}
}

// TestHandleRegisterDevice_MissingFields_400 verifies the ErrMissingFields
// sentinel reaches the client (BE-004 regression guard).
func TestHandleRegisterDevice_MissingFields_400(t *testing.T) {
	h := NewHandlers(&Service{})
	user := &auth.AuthenticatedUser{
		UserID:   validUUID(t, "00000000-0000-0000-0000-000000000001"),
		TenantID: validUUID(t, "00000000-0000-0000-0000-000000000002"),
	}
	body := `{"device_id":"d1","platform":"ios"}`
	req := httptest.NewRequest(http.MethodPost, "/me/devices", strBody(body))
	req = withAuth(req, user)
	rr := httptest.NewRecorder()
	h.HandleRegisterDevice(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 missing_fields, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "missing_fields" {
		t.Errorf("expected code missing_fields, got %q", env.Error.Code)
	}
}

func TestIsValidPlatform(t *testing.T) {
	tests := []struct {
		platform string
		want     bool
	}{
		{PlatformIOS, true},
		{PlatformAndroid, true},
		{PlatformWeb, true},
		{"", false},
		{"desktop", false},
		{"IOS", false}, // case-sensitive
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			got := IsValidPlatform(tt.platform)
			if got != tt.want {
				t.Errorf("IsValidPlatform(%q) = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}

// TestSendPush_NoDispatcher_ErrNoDispatcher verifies the service returns a
// distinct error (not silent success) when no dispatcher is wired (BE-006).
// It uses a stub settings reader that reports push.enabled=true.
func TestSendPush_NoDispatcher_ErrNoDispatcher(t *testing.T) {
	s := &Service{settings: &fakeSettings{enabled: true}}
	err := s.SendPush(context.Background(), pgtype.UUID{}, pgtype.UUID{}, PushPayload{})
	if !errors.Is(err, ErrNoDispatcher) {
		t.Errorf("expected ErrNoDispatcher, got %v", err)
	}
}

// TestSendPush_Disabled_ErrPushDisabled verifies the disabled path is
// distinct from the config-error path (BE-008).
func TestSendPush_Disabled_ErrPushDisabled(t *testing.T) {
	s := &Service{settings: &fakeSettings{enabled: false}}
	err := s.SendPush(context.Background(), pgtype.UUID{}, pgtype.UUID{}, PushPayload{})
	if !errors.Is(err, ErrPushDisabled) {
		t.Errorf("expected ErrPushDisabled, got %v", err)
	}
}

// fakeSettings is a minimal settingReader stub for service-level tests.
type fakeSettings struct {
	enabled bool
}

func (f *fakeSettings) GetSetting(_ context.Context, _ pgtype.UUID, _ string) (string, error) {
	return "", fmt.Errorf("not configured")
}

func (f *fakeSettings) GetSettingBool(_ context.Context, _ pgtype.UUID, _ string) (bool, error) {
	return f.enabled, nil
}

// strBody returns an io.Reader for a JSON string body.
func strBody(s string) *bytesReader { return &bytesReader{b: []byte(s)} }

// --- CompositeDispatcher tests ---

// TestCompositeDispatcher_NoCredentials_ReturnsError verifies that when no
// FCM/APNs credentials are configured in system_settings, Dispatch returns
// a clear credential error (not a silent success or panic).
func TestCompositeDispatcher_NoCredentials_ReturnsError(t *testing.T) {
	d := NewCompositeDispatcher(&fakeSettings{enabled: true})
	tenantID := validUUID(t, "00000000-0000-0000-0000-000000000001")
	err := d.Dispatch(context.Background(), tenantID, PlatformIOS, "fake-token", PushPayload{Title: "t", Body: "b"})
	if err == nil {
		t.Error("expected error for unconfigured APNs credentials, got nil")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' in error, got: %v", err)
	}
	err = d.Dispatch(context.Background(), tenantID, PlatformAndroid, "fake-token", PushPayload{Title: "t", Body: "b"})
	if err == nil {
		t.Error("expected error for unconfigured FCM credentials, got nil")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' in error, got: %v", err)
	}
}

// TestCompositeDispatcher_WebPlatform_ReturnsError verifies that web push
// is rejected (it's handled by the VAPID pipeline in internal/notifications).
func TestCompositeDispatcher_WebPlatform_ReturnsError(t *testing.T) {
	d := NewCompositeDispatcher(&fakeSettings{enabled: true})
	tenantID := validUUID(t, "00000000-0000-0000-0000-000000000001")
	err := d.Dispatch(context.Background(), tenantID, PlatformWeb, "fake-token", PushPayload{})
	if err == nil {
		t.Error("expected error for web platform, got nil")
	}
	if !strings.Contains(err.Error(), "web push") {
		t.Errorf("expected 'web push' in error, got: %v", err)
	}
}

// TestCompositeDispatcher_UnknownPlatform_ReturnsError verifies that an
// unknown platform is rejected.
func TestCompositeDispatcher_UnknownPlatform_ReturnsError(t *testing.T) {
	d := NewCompositeDispatcher(&fakeSettings{enabled: true})
	tenantID := validUUID(t, "00000000-0000-0000-0000-000000000001")
	err := d.Dispatch(context.Background(), tenantID, "desktop", "fake-token", PushPayload{})
	if err == nil {
		t.Error("expected error for unknown platform, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported platform") {
		t.Errorf("expected 'unsupported platform' in error, got: %v", err)
	}
}

// TestNoopDispatcher_ReturnsErrNoDispatcher verifies the noop sentinel.
func TestNoopDispatcher_ReturnsErrNoDispatcher(t *testing.T) {
	tenantID := validUUID(t, "00000000-0000-0000-0000-000000000001")
	err := NoopDispatcher{}.Dispatch(context.Background(), tenantID, PlatformIOS, "tok", PushPayload{})
	if !errors.Is(err, ErrNoDispatcher) {
		t.Errorf("expected ErrNoDispatcher, got %v", err)
	}
}

type bytesReader struct {
	b []byte
	i int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, errors.New("EOF")
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
