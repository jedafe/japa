package content

import (
	"testing"
	"time"
)

// ParsePublishAtParam must accept the RFC 3339 timestamps JSON clients send
// (pgtype.Timestamptz.Scan only understands Postgres text format) while
// keeping the Postgres forms working for backward compatibility.
func TestParsePublishAtParam(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  time.Time
		ok    bool
	}{
		{"rfc3339 utc", "2026-10-15T10:00:00Z", time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC), true},
		{"rfc3339 millis", "2026-10-15T10:00:00.000Z", time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC), true},
		{"rfc3339 offset", "2026-10-15T10:00:00+01:00", time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC), true},
		{"pg text", "2026-10-15 10:00:00+00", time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC), true},
		{"garbage", "not-a-date", time.Time{}, false},
		{"date only", "2026-10-15", time.Time{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, err := ParsePublishAtParam(tc.input)
			if !tc.ok {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}

			if !ts.Valid || !ts.Time.Equal(tc.want) {
				t.Fatalf("got %v (valid=%v), want %v", ts.Time, ts.Valid, tc.want)
			}
		})
	}
}
