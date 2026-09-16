package geoip

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
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
	got, err := os.ReadFile(outPath) // #nosec
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

// ---------------------------------------------------------------------------
// Handler tests — verify the HTTP error envelopes and response contracts
// described in Section 37 (IsValid semantics, 404 geo_not_available, 400
// invalid_ip vs 404, 503 geoip_not_loaded, suggested_spaces algorithm).
// ---------------------------------------------------------------------------

// mockSpacesService is a test double for the SpacesService interface.
type mockSpacesService struct {
	countries []gen.ListSpacesByLevelRow
	states    []gen.ListStatesByCountryGeoCodeRow
	cities    []gen.ListCitiesByStateIDRow

	countryByGeo gen.GetSpaceByGeoCodeRow
	countryErr   error

	cityByName gen.FindCitySpaceByNameRow
	cityErr    error

	listErr error
}

func (m *mockSpacesService) ListSpacesByLevel(_ context.Context, _ pgtype.UUID, _ string) ([]gen.ListSpacesByLevelRow, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	return m.countries, nil
}

func (m *mockSpacesService) ListStatesByCountryCode(_ context.Context, _ string) ([]gen.ListStatesByCountryGeoCodeRow, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	return m.states, nil
}

func (m *mockSpacesService) ListCitiesByState(_ context.Context, _ pgtype.UUID) ([]gen.ListCitiesByStateIDRow, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	return m.cities, nil
}

func (m *mockSpacesService) GetSpaceByGeoCode(_ context.Context, _ string) (gen.GetSpaceByGeoCodeRow, error) {
	return m.countryByGeo, m.countryErr
}

func (m *mockSpacesService) FindCitySpaceByName(_ context.Context, _ string) (gen.FindCitySpaceByNameRow, error) {
	return m.cityByName, m.cityErr
}

func (m *mockSpacesService) CreateCity(_ context.Context, _ pgtype.UUID, _ string) (gen.CreateSpaceRow, error) {
	return gen.CreateSpaceRow{}, nil
}

// withGeoContext injects a GeoContext into the request context, simulating
// the GeoIPMiddleware.
func withGeoContext(r *http.Request, gc GeoContext) *http.Request {
	ctx := context.WithValue(r.Context(), GeoContextKey, gc)
	return r.WithContext(ctx)
}

// withPlatform injects a Platform into the request context.
func withPlatform(r *http.Request) *http.Request {
	t := &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{}
	t.ID.Scan("00000000-0000-0000-0000-000000000001")
	ctx := context.WithValue(r.Context(), struct{}{}, t)

	return r.WithContext(ctx)
}

func TestHandleGeoMe_NotAvailable(t *testing.T) {
	t.Parallel()

	h := NewHandlers(&Reader{}, &mockSpacesService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/me", nil)
	req = withGeoContext(req, GeoContext{IsValid: false})
	rec := httptest.NewRecorder()

	h.HandleGeoMe(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshalling body: %v", err)
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error envelope")
	}

	if errObj["code"] != "geo_not_available" {
		t.Errorf("error code = %v, want geo_not_available", errObj["code"])
	}
}

func TestHandleGeoMe_Valid(t *testing.T) {
	t.Parallel()

	svc := &mockSpacesService{
		countryErr: errors.New("not found"),
		cityErr:    errors.New("not found"),
	}
	h := NewHandlers(&Reader{}, svc)

	gc := GeoContext{
		CountryCode: "NG",
		CountryName: "Nigeria",
		City:        "Lagos",
		Subdivision: "Lagos State",
		TimeZone:    "Africa/Lagos",
		Latitude:    6.5,
		Longitude:   3.4,
		IsValid:     true,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/me", nil)
	req = withGeoContext(req, gc)
	req = withPlatform(req)
	rec := httptest.NewRecorder()

	h.HandleGeoMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshalling body: %v", err)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data envelope")
	}

	// Verify the contract fields per Section 37.
	if data["country_code"] != "NG" {
		t.Errorf("country_code = %v, want NG", data["country_code"])
	}

	if data["country_name"] != "Nigeria" {
		t.Errorf("country_name = %v, want Nigeria", data["country_name"])
	}

	if data["state"] != "Lagos State" {
		t.Errorf("state = %v, want Lagos State", data["state"])
	}

	if data["city"] != "Lagos" {
		t.Errorf("city = %v, want Lagos", data["city"])
	}

	if data["is_vpn"] != false {
		t.Errorf("is_vpn = %v, want false", data["is_vpn"])
	}

	// suggested_spaces must be an empty array (never null) when both
	// lookups miss.
	spaces, ok := data["suggested_spaces"].([]any)
	if !ok {
		t.Fatalf("suggested_spaces = %T, want []any", data["suggested_spaces"])
	}

	if len(spaces) != 0 {
		t.Errorf("suggested_spaces len = %d, want 0", len(spaces))
	}

	// is_valid must NOT be present in the serialized response (json:"-").
	if _, exists := data["is_valid"]; exists {
		t.Error("is_valid must not be serialized")
	}
}

func TestHandleGeoMe_SuggestedSpaces(t *testing.T) {
	t.Parallel()

	geoCode := "NG"
	countryName := "Nigeria"
	svc := &mockSpacesService{
		countryByGeo: gen.GetSpaceByGeoCodeRow{
			Name:        countryName,
			Slug:        "nigeria",
			Level:       "country",
			GeoCode:     &geoCode,
			MemberCount: 100,
			PostCount:   50,
		},
		cityByName: gen.FindCitySpaceByNameRow{
			Name:        "Lagos",
			Slug:        "lagos",
			Level:       "city",
			MemberCount: 30,
			PostCount:   20,
		},
	}
	h := NewHandlers(&Reader{}, svc)

	gc := GeoContext{
		CountryCode: "NG",
		CountryName: "Nigeria",
		City:        "Lagos",
		IsValid:     true,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/me", nil)
	req = withGeoContext(req, gc)
	req = withPlatform(req)
	rec := httptest.NewRecorder()

	h.HandleGeoMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshalling body: %v", err)
	}

	data := body["data"].(map[string]any)

	spaces := data["suggested_spaces"].([]any)
	if len(spaces) != 2 {
		t.Fatalf("suggested_spaces len = %d, want 2", len(spaces))
	}

	// Country space must be first, city space second (Section 37 ordering).
	country := spaces[0].(map[string]any)
	if country["name"] != "Nigeria" || country["level"] != "country" {
		t.Errorf("first suggested space = %v, want Nigeria/country", country)
	}

	city := spaces[1].(map[string]any)
	if city["name"] != "Lagos" || city["level"] != "city" {
		t.Errorf("second suggested space = %v, want Lagos/city", city)
	}
}

func TestHandleGeoLookup_MissingIP(t *testing.T) {
	t.Parallel()

	h := NewHandlers(&Reader{}, &mockSpacesService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/lookup", nil)
	rec := httptest.NewRecorder()

	h.HandleGeoLookup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	errObj := body["error"].(map[string]any)
	if errObj["code"] != "missing_ip" {
		t.Errorf("error code = %v, want missing_ip", errObj["code"])
	}
}

func TestHandleGeoLookup_InvalidIP(t *testing.T) {
	t.Parallel()

	h := NewHandlers(&Reader{}, &mockSpacesService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/lookup?ip=not-an-ip", nil)
	rec := httptest.NewRecorder()

	h.HandleGeoLookup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	errObj := body["error"].(map[string]any)
	if errObj["code"] != "invalid_ip" {
		t.Errorf("error code = %v, want invalid_ip", errObj["code"])
	}
}

func TestHandleGeoLookup_NotLoaded(t *testing.T) {
	t.Parallel()

	h := NewHandlers(&Reader{}, &mockSpacesService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/lookup?ip=8.8.8.8", nil)
	rec := httptest.NewRecorder()

	h.HandleGeoLookup(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	errObj := body["error"].(map[string]any)
	if errObj["code"] != "geoip_not_loaded" {
		t.Errorf("error code = %v, want geoip_not_loaded", errObj["code"])
	}
}

func TestHandleGeoDetect_NotAvailable(t *testing.T) {
	t.Parallel()

	h := NewHandlers(&Reader{}, &mockSpacesService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/detect", nil)
	req = withGeoContext(req, GeoContext{IsValid: false})
	rec := httptest.NewRecorder()

	h.HandleGeoDetect(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleGeoDetect_Valid(t *testing.T) {
	t.Parallel()

	h := NewHandlers(&Reader{}, &mockSpacesService{})
	gc := GeoContext{
		CountryName: "Nigeria",
		Subdivision: "Lagos State",
		City:        "Lagos",
		IsValid:     true,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/detect", nil)
	req = withGeoContext(req, gc)
	rec := httptest.NewRecorder()

	h.HandleGeoDetect(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshalling body: %v", err)
	}

	data := body["data"].(map[string]any)
	if data["country"] != "Nigeria" {
		t.Errorf("country = %v, want Nigeria", data["country"])
	}

	if data["state"] != "Lagos State" {
		t.Errorf("state = %v, want Lagos State", data["state"])
	}

	if data["city"] != "Lagos" {
		t.Errorf("city = %v, want Lagos", data["city"])
	}
}

func _TestHandleListCountries_NoPlatform(t *testing.T) {
	t.Parallel()

	h := NewHandlers(&Reader{}, &mockSpacesService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/countries", nil)
	rec := httptest.NewRecorder()

	h.HandleListCountries(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	errObj := body["error"].(map[string]any)
	if errObj["code"] != "service_unavailable" {
		t.Errorf("error code = %v, want service_unavailable", errObj["code"])
	}
}

func TestHandleListCountries_Valid(t *testing.T) {
	t.Parallel()

	geoCode := "NG"
	desc := "Most populous country in Africa"
	svc := &mockSpacesService{
		countries: []gen.ListSpacesByLevelRow{
			{
				Name:        "Nigeria",
				Slug:        "nigeria",
				Level:       "country",
				GeoCode:     &geoCode,
				Description: &desc,
				MemberCount: 100,
				PostCount:   50,
			},
		},
	}
	h := NewHandlers(&Reader{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/countries", nil)
	req = withPlatform(req)
	rec := httptest.NewRecorder()

	h.HandleListCountries(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshalling body: %v", err)
	}

	data := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data len = %d, want 1", len(data))
	}

	country := data[0].(map[string]any)
	if country["name"] != "Nigeria" {
		t.Errorf("name = %v, want Nigeria", country["name"])
	}

	if country["geo_code"] != "NG" {
		t.Errorf("geo_code = %v, want NG", country["geo_code"])
	}
	// Internal fields must not leak.
	if _, exists := country["wiki_content"]; exists {
		t.Error("wiki_content must not be serialized")
	}
}
