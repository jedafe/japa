// Package handlers tests cover the JapaConnect intelligence HTTP surface
// (BUG-22 fix). Because the handler dependencies (stores, sources, follows,
// scholarships services) are concrete types backed by a database, the suite
// focuses on:
//   - helper function behavior (writeError, writeJSON, parsePagination,
//     scanUUIDParam, cursor encode/parse, aggregations);
//   - pure projection functions (scholarshipToRadarItem, scholarshipToDetail,
//     sourceToDirectoryEntry, filterScholarshipsPostQuery, buildScholarshipFacets);
//   - nil-dependency graceful degradation: every handler returns 503 with the
//     intel_unavailable error code before touching the (absent) store/service.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// --- shared test helpers ---

// mustUUID scans a UUID string into a pgtype.UUID, failing the test on error.
func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("mustUUID: scan %q: %v", s, err)
	}

	return id
}

// requestWithURLParam builds a request whose chi route context carries the
// given URL parameter, mimicking what the chi router injects during routing.
func requestWithURLParam(method, target, param, value, body string) *http.Request {
	rctx := chi.NewRouteContext()
	if value != "" {
		rctx.URLParams.Add(param, value)
	}

	r := httptest.NewRequest(method, target, strings.NewReader(body))

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// errorEnvelope mirrors the {error:{code,message}} shape written by
// errpkg.WriteError so tests can decode without depending on map typing.
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// assertIntelUnavailable asserts the recorder carries a 503 with the
// intel_unavailable error code — the graceful-degradation contract every
// nil-dependency handler honors.
func assertIntelUnavailable(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (body=%s)", rr.Code, http.StatusServiceUnavailable, rr.Body.String())
	}

	var env errorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v (body=%s)", err, rr.Body.String())
	}

	if env.Error.Code != "intel_unavailable" {
		t.Errorf("error.code = %q, want intel_unavailable", env.Error.Code)
	}

	if env.Error.Message == "" {
		t.Error("error.message is empty, want a human-readable message")
	}
}

// sampleItems returns a fresh slice of radar items for filter/facet tests.
// A fresh copy is returned per call because filterScholarshipsPostQuery
// filters in place (out := items[:0]), so reusing one slice across calls
// would corrupt subsequent assertions.
func sampleItems() []radarItem {
	descHigh := "high confidence award for stem"
	descLow := "low confidence listing"
	uni := "Tokyo University"

	return []radarItem{
		{Title: "MEXT Award", Description: &descHigh, University: &uni, Country: "JP", ConfidenceOverall: 0.95, DegreeLevel: ptrString("Masters")},
		{Title: "Small Grant", Description: &descLow, University: &uni, Country: "US", ConfidenceOverall: 0.30, DegreeLevel: ptrString("Bachelors")},
		{Title: "Other Award", Description: &descLow, University: nil, Country: "GB", ConfidenceOverall: 0.80, DegreeLevel: nil},
	}
}

func ptrString(s string) *string { return &s }

// --- helper function tests ---

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteJSON(rr, http.StatusOK, map[string]interface{}{"ok": true})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if v, ok := body["ok"].(bool); !ok || !v {
		t.Errorf("body[ok] = %v, want true", body["ok"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteError(rr, http.StatusBadRequest, "invalid_uuid", "Invalid item_id")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var env errorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}

	if env.Error.Code != "invalid_uuid" {
		t.Errorf("error.code = %q, want invalid_uuid", env.Error.Code)
	}

	if env.Error.Message != "Invalid item_id" {
		t.Errorf("error.message = %q, want %q", env.Error.Message, "Invalid item_id")
	}
}

func TestParsePagination(t *testing.T) {
	t.Parallel()
	// parsePagination reads page_size + page_offset (spec) with a back-compat
	// fallback to legacy limit + page (1-based) only when page_offset < 0.
	// page_size is clamped to 20 when <= 0 or > 100, but a small valid value
	// (e.g. 5) is preserved — there is no minimum-of-20 floor.
	cases := []struct {
		name      string
		query     string
		wantPage  int
		wantLimit int
	}{
		{"default", "", 1, 20},
		{"page_size_and_offset", "page_size=50&page_offset=100", 3, 50},
		{"legacy_limit_page", "limit=10&page_offset=-1&page=3", 3, 10},
		{"oversize_clamped_to_20", "page_size=200", 1, 20},
		{"zero_clamped_to_20", "page_size=0", 1, 20},
		{"negative_clamped_to_20", "page_size=-5", 1, 20},
		{"small_page_size_preserved", "page_size=5", 1, 5},
		{"legacy_limit_only", "limit=30", 1, 30},
		{"legacy_page_alone_no_offset_trigger", "limit=10&page=3", 1, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)
			h := &Handlers{} // nil settings → hardcoded defaults (20/100)

			page, limit := h.parsePagination(r)
			if page != tc.wantPage {
				t.Errorf("page = %d, want %d", page, tc.wantPage)
			}

			if limit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tc.wantLimit)
			}
		})
	}
}

func TestScanUUIDParam(t *testing.T) {
	t.Parallel()

	uuidStr := "550e8400-e29b-41d4-a716-446655440000"

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		r := requestWithURLParam("GET", "/", "id", uuidStr, "")
		rr := httptest.NewRecorder()
		id, ok := scanUUIDParam(rr, r, "id")

		if !ok {
			t.Fatalf("ok = false, want true (body=%s)", rr.Body.String())
		}

		if !id.Valid {
			t.Fatal("id.Valid = false, want true")
		}

		if id.String() != uuidStr {
			t.Errorf("id = %q, want %q", id.String(), uuidStr)
		}
	})

	for _, tc := range []struct {
		name     string
		paramVal string
		wantCode string
	}{
		{"missing", "", "missing_param"},
		{"invalid", "not-a-uuid", "invalid_uuid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := requestWithURLParam("GET", "/", "id", tc.paramVal, "")
			rr := httptest.NewRecorder()

			if _, ok := scanUUIDParam(rr, r, "id"); ok {
				t.Error("ok = true, want false for bad param")
			}

			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
			}

			var env errorEnvelope
			_ = json.NewDecoder(rr.Body).Decode(&env)

			if env.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %s", env.Error.Code, tc.wantCode)
			}
		})
	}
}

func TestEncodeCursor(t *testing.T) {
	t.Parallel()
	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	ts := pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 12, 30, 0, 123456789, time.UTC), Valid: true}

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		c := encodeCursor(ts, id)
		if c == "" {
			t.Fatal("expected non-empty cursor for valid inputs")
		}
		// Cursor must be opaque base64 (no whitespace, no '|' in encoded form).
		if decoded, err := base64.RawURLEncoding.DecodeString(c); err != nil {
			t.Fatalf("cursor is not valid RawURLEncoding base64: %v", err)
		} else if !contains(string(decoded), "|") {
			t.Errorf("decoded cursor %q has no '|' separator", string(decoded))
		}
	})

	t.Run("invalid_timestamp", func(t *testing.T) {
		t.Parallel()

		if c := encodeCursor(pgtype.Timestamptz{Valid: false}, id); c != "" {
			t.Errorf("expected empty cursor for invalid timestamp, got %q", c)
		}
	})

	t.Run("invalid_id", func(t *testing.T) {
		t.Parallel()

		if c := encodeCursor(ts, pgtype.UUID{Valid: false}); c != "" {
			t.Errorf("expected empty cursor for invalid id, got %q", c)
		}
	})
}

func TestParseCursor(t *testing.T) {
	t.Parallel()
	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	ts := pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 12, 30, 0, 987654321, time.UTC), Valid: true}

	t.Run("round_trip", func(t *testing.T) {
		t.Parallel()

		cursor := encodeCursor(ts, id)
		if cursor == "" {
			t.Fatal("encodeCursor returned empty for valid inputs")
		}

		gotTS, gotID, ok := parseCursor(cursor)
		if !ok {
			t.Fatalf("parseCursor ok = false for valid cursor %q", cursor)
		}

		if !gotTS.Valid {
			t.Fatal("parsed timestamp not valid")
		}

		if gotTS.Time.UnixNano() != ts.Time.UnixNano() {
			t.Errorf("timestamp nano = %d, want %d", gotTS.Time.UnixNano(), ts.Time.UnixNano())
		}

		if !gotID.Valid || gotID.String() != id.String() {
			t.Errorf("parsed id = %v, want %q", gotID, id.String())
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		if _, _, ok := parseCursor(""); ok {
			t.Error("parseCursor('') ok = true, want false")
		}
	})

	t.Run("malformed_not_base64", func(t *testing.T) {
		t.Parallel()

		if _, _, ok := parseCursor("!!!not-base64!!!"); ok {
			t.Error("parseCursor(non-base64) ok = true, want false")
		}
	})

	t.Run("valid_base64_no_separator", func(t *testing.T) {
		t.Parallel()

		c := base64.RawURLEncoding.EncodeToString([]byte("noseparatorhere"))
		if _, _, ok := parseCursor(c); ok {
			t.Error("parseCursor(no separator) ok = true, want false")
		}
	})

	t.Run("valid_base64_bad_uuid", func(t *testing.T) {
		t.Parallel()

		c := base64.RawURLEncoding.EncodeToString([]byte("1700000000|not-a-uuid"))
		if _, _, ok := parseCursor(c); ok {
			t.Error("parseCursor(bad uuid) ok = true, want false")
		}
	})
}

// --- aggregation / projection pure-function tests ---

func TestCountByType(t *testing.T) {
	t.Parallel()

	items := []radarItem{
		{Type: "scholarship", Country: "JP"},
		{Type: "scholarship", Country: "US"},
		{Type: "job", Country: "JP"},
	}

	counts := countByType(items)
	if counts["scholarship"] != 2 {
		t.Errorf("scholarship count = %d, want 2", counts["scholarship"])
	}

	if counts["job"] != 1 {
		t.Errorf("job count = %d, want 1", counts["job"])
	}

	if _, ok := counts["visa"]; ok {
		t.Error("unexpected visa key present")
	}

	empty := countByType(nil)
	if len(empty) != 0 {
		t.Errorf("nil input count len = %d, want 0", len(empty))
	}
}

func TestCountByCountry(t *testing.T) {
	t.Parallel()

	items := []radarItem{
		{Type: "scholarship", Country: "JP"},
		{Type: "scholarship", Country: "JP"},
		{Type: "job", Country: "US"},
	}

	counts := countByCountry(items)
	if counts["JP"] != 2 {
		t.Errorf("JP count = %d, want 2", counts["JP"])
	}

	if counts["US"] != 1 {
		t.Errorf("US count = %d, want 1", counts["US"])
	}

	empty := countByCountry([]radarItem{})
	if len(empty) != 0 {
		t.Errorf("empty input count len = %d, want 0", len(empty))
	}
}

func TestScholarshipToRadarItem(t *testing.T) {
	t.Parallel()
	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	created := pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), Valid: true}
	deadline := pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true}

	s := gen.Scholarship{
		ID:                id,
		Title:             "MEXT Scholarship",
		Description:       ptrString("Merit-based award"),
		Country:           "JP",
		University:        ptrString("Tokyo U"),
		DegreeLevel:       ptrString("Masters"),
		Deadline:          deadline,
		ApplicationUrl:    ptrString("https://apply.example.com"),
		ConfidenceOverall: 0.92,
		TrustLevel:        1,
		FreshnessScore:    85,
		SourceUrl:         "https://source.example.com",
		SourceType:        "official",
		CreatedAt:         created,
	}
	item := scholarshipToRadarItem(s)

	if item.Type != "scholarship" {
		t.Errorf("Type = %q, want scholarship", item.Type)
	}

	if item.Title != s.Title {
		t.Errorf("Title = %q, want %q", item.Title, s.Title)
	}

	if item.Country != s.Country {
		t.Errorf("Country = %q, want %q", item.Country, s.Country)
	}

	if item.SourceURL != s.SourceUrl {
		t.Errorf("SourceURL = %q, want %q", item.SourceURL, s.SourceUrl)
	}

	if item.ConfidenceOverall != s.ConfidenceOverall {
		t.Errorf("ConfidenceOverall = %v, want %v", item.ConfidenceOverall, s.ConfidenceOverall)
	}

	if item.TrustLevel != s.TrustLevel {
		t.Errorf("TrustLevel = %d, want %d", item.TrustLevel, s.TrustLevel)
	}

	if item.FreshnessScore != s.FreshnessScore {
		t.Errorf("FreshnessScore = %d, want %d", item.FreshnessScore, s.FreshnessScore)
	}

	if item.ID != s.ID {
		t.Error("ID not mapped")
	}

	if item.Deadline != s.Deadline {
		t.Error("Deadline not mapped")
	}

	if item.CreatedAt != s.CreatedAt {
		t.Error("CreatedAt not mapped")
	}
	// Pointer fields must be carried through unchanged.
	if item.Description == nil || *item.Description != "Merit-based award" {
		t.Errorf("Description = %v, want non-nil 'Merit-based award'", item.Description)
	}

	if item.University == nil || *item.University != "Tokyo U" {
		t.Errorf("University = %v, want non-nil 'Tokyo U'", item.University)
	}

	if item.DegreeLevel == nil || *item.DegreeLevel != "Masters" {
		t.Errorf("DegreeLevel = %v, want non-nil 'Masters'", item.DegreeLevel)
	}

	if item.ApplicationURL == nil || *item.ApplicationURL != "https://apply.example.com" {
		t.Errorf("ApplicationURL = %v, want non-nil url", item.ApplicationURL)
	}
}

func TestScholarshipToDetail(t *testing.T) {
	t.Parallel()
	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	guideID := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")
	created := pgtype.Timestamptz{Time: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), Valid: true}
	updated := pgtype.Timestamptz{Time: time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC), Valid: true}
	verified := pgtype.Timestamptz{Time: time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC), Valid: true}
	deadline := pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true}

	s := gen.Scholarship{
		ID:                   id,
		GuideID:              guideID,
		Title:                "MEXT",
		Description:          ptrString("Award"),
		Country:              "JP",
		University:           ptrString("Tokyo U"),
		DegreeLevel:          ptrString("Masters"),
		FieldsOfStudy:        []string{"Engineering", "Science"},
		EligibleCountries:    []string{"JP", "US"},
		Funding:              []byte(`{"full":true}`),
		Deadline:             deadline,
		ApplicationUrl:       ptrString("https://apply.example.com"),
		RequiredDocuments:    []string{"transcript", "passport"},
		LanguageRequirements: []byte(`{"toefl":90}`),
		SourceUrl:            "https://source.example.com",
		SourceType:           "official",
		TrustLevel:           1,
		ConfidenceOverall:    0.92,
		VerificationStatus:   "verified",
		FreshnessScore:       85,
		LastVerifiedAt:       verified,
		CreatedAt:            created,
		UpdatedAt:            updated,
	}
	d := scholarshipToDetail(s)

	if d.ID != s.ID {
		t.Error("ID not mapped")
	}

	if d.GuideID != s.GuideID {
		t.Error("GuideID not mapped")
	}

	if d.Title != s.Title {
		t.Errorf("Title = %q, want %q", d.Title, s.Title)
	}

	if d.Country != s.Country {
		t.Errorf("Country = %q, want %q", d.Country, s.Country)
	}

	if d.SourceType != s.SourceType {
		t.Errorf("SourceType = %q, want %q", d.SourceType, s.SourceType)
	}

	if d.SourceURL != s.SourceUrl {
		t.Errorf("SourceURL = %q, want %q", d.SourceURL, s.SourceUrl)
	}

	if d.TrustLevel != s.TrustLevel {
		t.Errorf("TrustLevel = %d, want %d", d.TrustLevel, s.TrustLevel)
	}

	if d.ConfidenceOverall != s.ConfidenceOverall {
		t.Errorf("ConfidenceOverall = %v, want %v", d.ConfidenceOverall, s.ConfidenceOverall)
	}

	if d.VerificationStatus != s.VerificationStatus {
		t.Errorf("VerificationStatus = %q, want %q", d.VerificationStatus, s.VerificationStatus)
	}

	if d.FreshnessScore != s.FreshnessScore {
		t.Errorf("FreshnessScore = %d, want %d", d.FreshnessScore, s.FreshnessScore)
	}

	if d.LastVerifiedAt != s.LastVerifiedAt {
		t.Error("LastVerifiedAt not mapped")
	}

	if d.CreatedAt != s.CreatedAt {
		t.Error("CreatedAt not mapped")
	}

	if d.UpdatedAt != s.UpdatedAt {
		t.Error("UpdatedAt not mapped")
	}

	if d.Deadline != s.Deadline {
		t.Error("Deadline not mapped")
	}
	// Slice fields.
	if len(d.FieldsOfStudy) != 2 || d.FieldsOfStudy[0] != "Engineering" {
		t.Errorf("FieldsOfStudy = %v, want [Engineering Science]", d.FieldsOfStudy)
	}

	if len(d.EligibleCountries) != 2 || d.EligibleCountries[1] != "US" {
		t.Errorf("EligibleCountries = %v, want [JP US]", d.EligibleCountries)
	}

	if len(d.RequiredDocuments) != 2 {
		t.Errorf("RequiredDocuments = %v, want 2 entries", d.RequiredDocuments)
	}

	if string(d.Funding) != `{"full":true}` {
		t.Errorf("Funding = %s, want {\"full\":true}", d.Funding)
	}

	if string(d.LanguageRequirements) != `{"toefl":90}` {
		t.Errorf("LanguageRequirements = %s, want {\"toefl\":90}", d.LanguageRequirements)
	}
	// Pointer fields.
	if d.Description == nil || *d.Description != "Award" {
		t.Errorf("Description = %v, want 'Award'", d.Description)
	}

	if d.University == nil || *d.University != "Tokyo U" {
		t.Errorf("University = %v, want 'Tokyo U'", d.University)
	}

	if d.DegreeLevel == nil || *d.DegreeLevel != "Masters" {
		t.Errorf("DegreeLevel = %v, want 'Masters'", d.DegreeLevel)
	}

	if d.ApplicationURL == nil || *d.ApplicationURL != "https://apply.example.com" {
		t.Errorf("ApplicationURL = %v, want url", d.ApplicationURL)
	}
}

func TestSourceToDirectoryEntry(t *testing.T) {
	t.Parallel()
	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	license := "CC-BY-4.0"
	restriction := "non-commercial"

	src := gen.IntelSource{
		ID:                  id,
		Name:                "MEXT Official",
		Domain:              "mext.go.jp",
		SourceUrl:           "https://mext.go.jp/scholarships",
		Category:            "scholarship",
		SourceType:          "official",
		TrustLevel:          1,
		AttributionRequired: true,
		SourceLicense:       &license,
		UsageRestriction:    &restriction,
		Enabled:             true,
	}
	entry := sourceToDirectoryEntry(src)

	// ID is converted to its string form when valid.
	if entry.ID != id.String() {
		t.Errorf("ID = %q, want %q", entry.ID, id.String())
	}

	if entry.Name != src.Name {
		t.Errorf("Name = %q, want %q", entry.Name, src.Name)
	}

	if entry.Domain != src.Domain {
		t.Errorf("Domain = %q, want %q", entry.Domain, src.Domain)
	}

	if entry.SourceURL != src.SourceUrl {
		t.Errorf("SourceURL = %q, want %q", entry.SourceURL, src.SourceUrl)
	}

	if entry.Category != src.Category {
		t.Errorf("Category = %q, want %q", entry.Category, src.Category)
	}

	if entry.SourceType != src.SourceType {
		t.Errorf("SourceType = %q, want %q", entry.SourceType, src.SourceType)
	}

	if entry.TrustLevel != src.TrustLevel {
		t.Errorf("TrustLevel = %d, want %d", entry.TrustLevel, src.TrustLevel)
	}

	if entry.AttributionRequired != src.AttributionRequired {
		t.Errorf("AttributionRequired = %v, want %v", entry.AttributionRequired, src.AttributionRequired)
	}

	if !entry.Enabled {
		t.Error("Enabled = false, want true")
	}

	if entry.SourceLicense == nil || *entry.SourceLicense != license {
		t.Errorf("SourceLicense = %v, want %q", entry.SourceLicense, license)
	}

	if entry.UsageRestriction == nil || *entry.UsageRestriction != restriction {
		t.Errorf("UsageRestriction = %v, want %q", entry.UsageRestriction, restriction)
	}

	// Invalid ID yields an empty string (not a zero-UUID literal).
	invalid := src
	invalid.ID = pgtype.UUID{Valid: false}

	if got := sourceToDirectoryEntry(invalid).ID; got != "" {
		t.Errorf("invalid ID = %q, want empty string", got)
	}
}

// --- nil-dependency graceful degradation tests (503 intel_unavailable) ---
//
// Every handler checks its required dependency up front and returns 503
// before touching the store/service or requiring tenant/user context, so a
// bare &Handlers{} (all deps nil) is sufficient — no context plumbing is
// needed for these cases.

func TestHandleRadarFeed_NilStores(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/intel/radar", nil)
	h.HandleRadarFeed(rr, r)
	assertIntelUnavailable(t, rr)
}

func TestHandleRadarCounts_NilStores(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/intel/radar/counts", nil)
	h.HandleRadarCounts(rr, r)
	assertIntelUnavailable(t, rr)
}

func TestHandleRadarFollowed_NilStores(t *testing.T) {
	t.Parallel()
	// HandleRadarFollowed requires both intelStores and followsSvc; both are
	// nil in a bare Handlers, so it returns 503 before requireUser.
	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/intel/radar/followed", nil)
	h.HandleRadarFollowed(rr, r)
	assertIntelUnavailable(t, rr)
}

func TestHandleRadarFollow_NilFollowsSvc(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/intel/radar/follow", nil)
	h.HandleRadarFollow(rr, r)
	assertIntelUnavailable(t, rr)
}

func TestHandleRadarSave_NilScholarSvc(t *testing.T) {
	t.Parallel()
	// HandleRadarSave requires scholarSvc AND intelStores; both nil → 503.
	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/intel/radar/save", nil)
	h.HandleRadarSave(rr, r)
	assertIntelUnavailable(t, rr)
}

func TestHandleReviewApprove_NilStores(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/intel/review/550e8400-e29b-41d4-a716-446655440000/approve", nil)
	h.HandleReviewApprove(rr, r)
	assertIntelUnavailable(t, rr)
}

func TestHandleReviewReject_NilStores(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/intel/review/550e8400-e29b-41d4-a716-446655440000/reject", nil)
	h.HandleReviewReject(rr, r)
	assertIntelUnavailable(t, rr)
}

func TestHandleSourcesDirectory_NilSources(t *testing.T) {
	t.Parallel()
	// HandleSourcesDirectory does not require a tenant (public directory),
	// so only the nil intelSources check matters here.
	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/intel/sources", nil)
	h.HandleSourcesDirectory(rr, r)
	assertIntelUnavailable(t, rr)
}

// --- search post-query filter & facet tests ---

func TestFilterScholarshipsPostQuery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		q         url.Values
		wantLen   int
		wantTitle string // first surviving item title; "" skips the check
	}{
		{"min_confidence", url.Values{"min_confidence": {"0.5"}}, 2, ""},
		{"min_confidence_keeps_all", url.Values{"min_confidence": {"0.0"}}, 3, ""},
		{"q_text_title", url.Values{"q": {"mext"}}, 1, "MEXT Award"},
		{"q_text_university", url.Values{"q": {"tokyo"}}, 2, ""},
		{"q_text_description", url.Values{"q": {"stem"}}, 1, "MEXT Award"},
		{"combined_confidence_and_text", url.Values{"min_confidence": {"0.5"}, "q": {"tokyo"}}, 1, "MEXT Award"},
		{"no_filters", url.Values{}, 3, ""},
		{"q_no_match", url.Values{"q": {"zzznomatch"}}, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Fresh copy per case: filterScholarshipsPostQuery filters in
			// place over the input's backing array.
			got := filterScholarshipsPostQuery(sampleItems(), tc.q, "")
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d (items=%+v)", len(got), tc.wantLen, titles(got))
			}

			if tc.wantTitle != "" {
				if len(got) == 0 || got[0].Title != tc.wantTitle {
					t.Errorf("first title = %q, want %q", titles(got), tc.wantTitle)
				}
			}
		})
	}
}

func TestBuildScholarshipFacets(t *testing.T) {
	t.Parallel()

	deg := "Masters"
	bach := "Bachelors"
	items := []radarItem{
		{Country: "JP", DegreeLevel: &deg},
		{Country: "JP", DegreeLevel: &deg},
		{Country: "US", DegreeLevel: &bach},
		{Country: "GB", DegreeLevel: nil},
	}
	facets := buildScholarshipFacets(items)

	countries, ok := facets["country"].(map[string]int)
	if !ok {
		t.Fatalf("facets[country] type = %T, want map[string]int", facets["country"])
	}

	if countries["JP"] != 2 {
		t.Errorf("country JP = %d, want 2", countries["JP"])
	}

	if countries["US"] != 1 {
		t.Errorf("country US = %d, want 1", countries["US"])
	}

	if countries["GB"] != 1 {
		t.Errorf("country GB = %d, want 1", countries["GB"])
	}

	degrees, ok := facets["degree_level"].(map[string]int)
	if !ok {
		t.Fatalf("facets[degree_level] type = %T, want map[string]int", facets["degree_level"])
	}

	if degrees["Masters"] != 2 {
		t.Errorf("degree Masters = %d, want 2", degrees["Masters"])
	}

	if degrees["Bachelors"] != 1 {
		t.Errorf("degree Bachelors = %d, want 1", degrees["Bachelors"])
	}
	// A nil DegreeLevel must not contribute an empty-string key.
	if _, exists := degrees[""]; exists {
		t.Error("unexpected empty-string degree_level facet key")
	}

	// Empty input yields empty facet maps (not nil) so the JSON envelope is {}.
	emptyFacets := buildScholarshipFacets(nil)
	if emptyCountries, ok := emptyFacets["country"].(map[string]int); !ok || len(emptyCountries) != 0 {
		t.Errorf("empty country facets = %v, want empty non-nil map", emptyFacets["country"])
	}
}

// --- small internal helpers ---

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}

func titles(items []radarItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Title)
	}

	return out
}
