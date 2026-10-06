package creator

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestScanOptionalUUIDParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantOK   bool
		wantZero bool
	}{
		{name: "absent", query: "", wantOK: false, wantZero: true},
		{name: "empty", query: "?instructor_id=", wantOK: false, wantZero: true},
		{name: "valid", query: "?instructor_id=00000000-0000-0000-0000-000000000001", wantOK: true, wantZero: false},
		{name: "malformed", query: "?instructor_id=not-a-uuid", wantOK: false, wantZero: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/admin/academy/revenue-splits"+tt.query, nil)
			got, ok := scanOptionalUUIDParam(req, "instructor_id")

			// A "zero" pgtype.UUID has Valid=false and zero bytes.
			isZero := !got.Valid
			if isZero != tt.wantZero {
				t.Errorf("zero=%v, want %v (got %+v)", isZero, tt.wantZero, got)
			}

			if ok != tt.wantOK {
				t.Errorf("ok=%v, want %v", ok, tt.wantOK)
			}
			// A valid UUID must return ok=true and a non-zero ID.
			if tt.wantOK && isZero {
				t.Errorf("expected a non-zero UUID, got zero")
			}
		})
	}
}

func TestParseOptionalTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		query  string
		wantOK bool
	}{
		{name: "absent", query: "", wantOK: false},
		{name: "empty", query: "?from=", wantOK: false},
		{name: "valid rfc3339", query: "?from=2026-07-28T00:00:00Z", wantOK: true},
		{name: "malformed", query: "?from=2026/07/28", wantOK: false},
		{name: "partial date", query: "?from=2026-07-28", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/admin/academy/revenue-splits"+tt.query, nil)

			got, ok := parseOptionalTime(req, "from")
			if ok != tt.wantOK {
				t.Errorf("ok=%v, want %v", ok, tt.wantOK)
			}

			if tt.wantOK && !got.Valid {
				t.Errorf("expected Valid=true, got Valid=false")
			}

			if !tt.wantOK && got.Valid {
				t.Errorf("expected Valid=false for invalid input, got Valid=true (Time=%v)", got.Time)
			}
		})
	}
}

func TestParseOptionalTimeRoundTrip(t *testing.T) {
	t.Parallel()

	// Verify the parsed timestamp preserves the exact instant (half-open
	// range filters depend on this — a drift would silently include/exclude
	// boundary splits).
	want := time.Date(2026, 7, 28, 12, 30, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/admin/academy/revenue-splits?from="+want.Format(time.RFC3339), nil)

	got, ok := parseOptionalTime(req, "from")
	if !ok {
		t.Fatal("ok=false, want true")
	}

	if !got.Valid {
		t.Fatal("Valid=false, want true")
	}

	if !got.Time.Equal(want) {
		t.Errorf("Time=%v, want %v", got.Time, want)
	}
}

func TestParsePaginationRevenueSplits(t *testing.T) {
	t.Parallel()

	// The audit viewer uses the shared parsePagination helper; verify the
	// page/limit interaction the handler relies on for the meta.page calc.
	tests := []struct {
		name       string
		query      string
		wantLimit  int32
		wantOffset int32
	}{
		{name: "defaults", query: "", wantLimit: 20, wantOffset: 0},
		{name: "page 1", query: "?page=1&limit=10", wantLimit: 10, wantOffset: 0},
		{name: "page 3 limit 10", query: "?page=3&limit=10", wantLimit: 10, wantOffset: 20},
		{name: "limit over 100 falls back to default", query: "?limit=500", wantLimit: 20, wantOffset: 0},
		{name: "limit at max 100", query: "?limit=100", wantLimit: 100, wantOffset: 0},
		{name: "limit 0 falls back", query: "?limit=0", wantLimit: 20, wantOffset: 0},
		{name: "negative limit falls back", query: "?limit=-5", wantLimit: 20, wantOffset: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/admin/academy/revenue-splits"+tt.query, nil)

			limit, offset := parsePagination(req)
			if limit != tt.wantLimit {
				t.Errorf("limit=%d, want %d", limit, tt.wantLimit)
			}

			if offset != tt.wantOffset {
				t.Errorf("offset=%d, want %d", offset, tt.wantOffset)
			}
		})
	}
}

func TestRevenueSplitsMetaPageCalc(t *testing.T) {
	t.Parallel()

	// The handler computes page = offset/limit + 1. Verify the boundary
	// math so the meta envelope reports 1-based pages correctly.
	tests := []struct {
		limit, offset, wantPage int32
	}{
		{limit: 20, offset: 0, wantPage: 1},
		{limit: 10, offset: 20, wantPage: 3},
		{limit: 10, offset: 9, wantPage: 1}, // offset < limit → page 1
		{limit: 0, offset: 0, wantPage: 0},  // guard against div-by-zero
	}
	for _, tt := range tests {
		page := int32(0)
		if tt.limit > 0 {
			page = tt.offset/tt.limit + 1
		}

		if page != tt.wantPage {
			t.Errorf("limit=%d offset=%d: page=%d, want %d", tt.limit, tt.offset, page, tt.wantPage)
		}
	}
}

func TestListRevenueSplitsFilterDefaults(t *testing.T) {
	t.Parallel()

	// A bare request (no filters) must produce a filter that lets every
	// platform row through — the SQL uses `@x IS NULL OR col = @x`, so zero
	// pgtype values mean "no filter".
	req := httptest.NewRequest(http.MethodGet, "/admin/academy/revenue-splits", nil)
	f := ListRevenueSplitsFilter{
		Source: req.URL.Query().Get("source"),
	}

	if id, ok := scanOptionalUUIDParam(req, "instructor_id"); ok {
		f.InstructorID = id
	}

	if id, ok := scanOptionalUUIDParam(req, "track_id"); ok {
		f.TrackID = id
	}

	if id, ok := scanOptionalUUIDParam(req, "payout_id"); ok {
		f.PayoutID = id
	}

	if ts, ok := parseOptionalTime(req, "from"); ok {
		f.FromTs = ts
	}

	if ts, ok := parseOptionalTime(req, "to"); ok {
		f.ToTs = ts
	}

	if f.InstructorID.Valid {
		t.Errorf("InstructorID should be zero/invalid, got %+v", f.InstructorID)
	}

	if f.TrackID.Valid {
		t.Errorf("TrackID should be zero/invalid, got %+v", f.TrackID)
	}

	if f.PayoutID.Valid {
		t.Errorf("PayoutID should be zero/invalid, got %+v", f.PayoutID)
	}

	if f.SpaceID.Valid {
		t.Errorf("SpaceID should be zero/invalid, got %+v", f.SpaceID)
	}

	if f.FromTs.Valid {
		t.Errorf("FromTs should be zero/invalid, got %+v", f.FromTs)
	}

	if f.ToTs.Valid {
		t.Errorf("ToTs should be zero/invalid, got %+v", f.ToTs)
	}

	if f.Source != "" {
		t.Errorf("Source should be empty, got %q", f.Source)
	}
}

func TestListRevenueSplitsFilterAllSet(t *testing.T) {
	t.Parallel()

	// Every filter set at once — verify each is parsed into the filter struct.
	q := "?instructor_id=00000000-0000-0000-0000-000000000001" +
		"&track_id=00000000-0000-0000-0000-000000000002" +
		"&payout_id=00000000-0000-0000-0000-000000000003" +
		"&space_id=00000000-0000-0000-0000-000000000004" +
		"&source=pro_stream" +
		"&from=2026-01-01T00:00:00Z" +
		"&to=2026-12-31T23:59:59Z"
	req := httptest.NewRequest(http.MethodGet, "/admin/academy/revenue-splits"+q, nil)

	f := ListRevenueSplitsFilter{Source: req.URL.Query().Get("source")}

	if id, ok := scanOptionalUUIDParam(req, "instructor_id"); !ok || !id.Valid {
		t.Error("instructor_id not parsed")
	} else {
		f.InstructorID = id
	}

	if id, ok := scanOptionalUUIDParam(req, "track_id"); !ok || !id.Valid {
		t.Error("track_id not parsed")
	} else {
		f.TrackID = id
	}

	if id, ok := scanOptionalUUIDParam(req, "payout_id"); !ok || !id.Valid {
		t.Error("payout_id not parsed")
	} else {
		f.PayoutID = id
	}

	if id, ok := scanOptionalUUIDParam(req, "space_id"); !ok || !id.Valid {
		t.Error("space_id not parsed")
	} else {
		f.SpaceID = id
	}

	if ts, ok := parseOptionalTime(req, "from"); !ok || !ts.Valid {
		t.Error("from not parsed")
	} else {
		f.FromTs = ts
	}

	if ts, ok := parseOptionalTime(req, "to"); !ok || !ts.Valid {
		t.Error("to not parsed")
	} else {
		f.ToTs = ts
	}

	if f.Source != "pro_stream" {
		t.Errorf("Source=%q, want pro_stream", f.Source)
	}
	// Sanity: the three UUIDs should be distinct (guards against a copy-paste
	// bug where all three resolve to the same param).
	if f.InstructorID == f.TrackID {
		t.Error("instructor_id and track_id resolved to the same UUID")
	}
}

// Compile-time guard: ensure pgtype.Timestamptz is used so the filter struct
// stays compatible with the sqlc-generated params.
var _ pgtype.Timestamptz = ListRevenueSplitsFilter{}.FromTs
