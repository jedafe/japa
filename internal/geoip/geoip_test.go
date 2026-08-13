package geoip

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jedafe/japa/internal/clientip"
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

	// Since the middleware now uses clientip.FromRequest, forwarding headers
	// are only honoured when the immediate peer is a trusted proxy
	// (loopback / RFC1918 by default). Tests use 127.0.0.1 as the peer so
	// the headers are respected; a non-trusted peer would ignore them.
	tests := []struct {
		name   string
		header string
		value  string
		remote string
		want   string
	}{
		{"X-Forwarded-For single", "X-Forwarded-For", "1.2.3.4", "127.0.0.1:12345", "1.2.3.4"},
		// clientip walks XFF right-to-left, returning the first untrusted IP.
		// For "1.2.3.4, 5.6.7.8" the right-most (5.6.7.8) is the actual client.
		{"X-Forwarded-For multiple", "X-Forwarded-For", "1.2.3.4, 5.6.7.8", "127.0.0.1:12345", "5.6.7.8"},
		{"X-Real-IP", "X-Real-IP", "9.8.7.6", "127.0.0.1:12345", "9.8.7.6"},
		{"RemoteAddr fallback", "", "", "192.168.1.1:12345", "192.168.1.1"},
		{"Untrusted peer ignores X-Forwarded-For", "X-Forwarded-For", "1.2.3.4", "8.8.8.8:12345", "8.8.8.8"},
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

			got := clientip.FromRequest(req)
			if got != tt.want {
				t.Errorf("clientip.FromRequest() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestReader_Close_NilsPointer verifies that after Close(), IsLoaded()
// returns false and Lookup degrades gracefully. This catches the
// post-close state corruption bug where the atomic pointer was not
// nilled out, leaving IsLoaded() returning true on a closed reader.
func TestReader_Close_NilsPointer(t *testing.T) {
	//nolint:paralleltest // uses a shared temp file
	tmp := t.TempDir() + "/test.mmdb"

	// NewReader on a non-existent path returns a Reader with nil
	// internal pointer (graceful degradation). Close on such a reader
	// should be a no-op and leave IsLoaded=false.
	r, err := NewReader(tmp)
	if err != nil {
		t.Fatalf("NewReader returned error: %v", err)
	}

	if r.IsLoaded() {
		t.Error("expected IsLoaded=false for non-existent DB path")
	}

	// Close should be safe on an unloaded reader.
	r.Close()

	if r.IsLoaded() {
		t.Error("expected IsLoaded=false after Close on unloaded reader")
	}

	// Lookup should still degrade gracefully.
	gc := r.Lookup("8.8.8.8")
	if gc.IsValid {
		t.Error("expected IsValid=false after Close")
	}
}

// TestReader_Close_Idempotent verifies that calling Close multiple
// times is safe (no panic, no error). The atomic Swap(nil) ensures
// only the first Close takes ownership of the reader; subsequent
// calls get nil and are no-ops.
func TestReader_Close_Idempotent(t *testing.T) {
	//nolint:paralleltest // shared Reader state
	r := &Reader{}

	// Multiple closes on an unloaded reader should all be safe.
	r.Close()
	r.Close()
	r.Close()

	if r.IsLoaded() {
		t.Error("expected IsLoaded=false after multiple Close calls")
	}
}

// TestExtractMMDB_ValidArchive verifies that extractMMDB correctly
// extracts a .mmdb file from a .tar.gz archive.
func TestExtractMMDB_ValidArchive(t *testing.T) {
	t.Parallel()

	// Build a .tar.gz archive containing a fake .mmdb file.
	mmdbContent := []byte("fake-mmdb-data")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: "GeoLite2-City_20260101/GeoLite2-City.mmdb",
		Mode: 0o644,
		Size: int64(len(mmdbContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := tw.Write(mmdbContent); err != nil {
		t.Fatalf("writing mmdb content: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("closing gzip: %v", err)
	}

	outPath := t.TempDir() + "/extracted.mmdb"
	if err := extractMMDB(&buf, outPath); err != nil {
		t.Fatalf("extractMMDB returned error: %v", err)
	}

	// Verify the extracted file content.
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if !bytes.Equal(got, mmdbContent) {
		t.Errorf("extracted content mismatch: got %q, want %q", got, mmdbContent)
	}
}

// TestExtractMMDB_NoMMDBInArchive verifies that extractMMDB returns an
// error when the archive contains no .mmdb file.
func TestExtractMMDB_NoMMDBInArchive(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: "readme.txt",
		Mode: 0o644,
		Size: 5,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatalf("writing content: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("closing gzip: %v", err)
	}

	outPath := t.TempDir() + "/should-not-exist.mmdb"
	err := extractMMDB(&buf, outPath)
	if err == nil {
		t.Fatal("expected error for archive with no .mmdb file")
	}
}
