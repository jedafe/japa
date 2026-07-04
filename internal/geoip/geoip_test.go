package geoip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromContext_NoGeoData(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	gc := FromContext(ctx)
	if gc.IsValid {
		t.Error("expected IsValid=false for empty context")
	}
}

func TestFromContext_WithGeoData(t *testing.T) {
	t.Parallel()

	gc := GeoContext{
		CountryCode: "NG",
		CountryName: "Nigeria",
		IsValid:     true,
	}
	ctx := context.WithValue(t.Context(), GeoContextKey, gc)

	result := FromContext(ctx)
	if !result.IsValid {
		t.Error("expected IsValid=true")
	}

	if result.CountryCode != "NG" {
		t.Errorf("CountryCode = %s, want NG", result.CountryCode)
	}

	if result.CountryName != "Nigeria" {
		t.Errorf("CountryName = %s, want Nigeria", result.CountryName)
	}
}

func TestReader_IsLoaded_NoDatabase(t *testing.T) {
	t.Parallel()
	// Create a reader with no database loaded
	r := &Reader{}
	if r.IsLoaded() {
		t.Error("expected IsLoaded=false when no database is loaded")
	}
}

func TestReader_Lookup_NoDatabase(t *testing.T) {
	t.Parallel()

	r := &Reader{}

	gc := r.Lookup("8.8.8.8")
	if gc.IsValid {
		t.Error("expected IsValid=false when no database is loaded")
	}
}

func TestReader_Lookup_InvalidIP(t *testing.T) {
	t.Parallel()

	r := &Reader{}

	gc := r.Lookup("not-an-ip")
	if gc.IsValid {
		t.Error("expected IsValid=false for invalid IP")
	}
}

func TestMiddleware_AddsGeoContext(t *testing.T) {
	t.Parallel()
	// Test that middleware adds geo context (even if invalid when no DB)
	r := &Reader{}
	mw := Middleware(r)

	called := false
	handler := mw(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		called = true
		gc := FromContext(req.Context())
		// Without a database, IsValid should be false
		if gc.IsValid {
			t.Error("expected IsValid=false without database")
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called")
	}
}

func TestRealIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		value  string
		remote string
		want   string
	}{
		{"X-Forwarded-For single", "X-Forwarded-For", "1.2.3.4", "", "1.2.3.4"},
		{"X-Forwarded-For multiple", "X-Forwarded-For", "1.2.3.4, 5.6.7.8", "", "1.2.3.4"},
		{"X-Real-IP", "X-Real-IP", "9.8.7.6", "", "9.8.7.6"},
		{"RemoteAddr fallback", "", "", "192.168.1.1:12345", "192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}

			if tt.remote != "" {
				req.RemoteAddr = tt.remote
			}

			got := realIP(req)
			if got != tt.want {
				t.Errorf("realIP() = %s, want %s", got, tt.want)
			}
		})
	}
}
