// Package devices implements the Mobile Push / Device Token registration
// surface (Section 77, ADR-074).
//
// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
package devices

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

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
