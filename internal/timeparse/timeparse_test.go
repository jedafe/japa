package timeparse

import (
	"testing"
	"time"
)

func TestParseTimestamptz(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, ts time.Time)
	}{
		{
			name:  "rfc3339 utc",
			input: "2026-10-15T10:00:00Z",
			check: func(t *testing.T, ts time.Time) {
				if ts.UTC().Format(time.RFC3339) != "2026-10-15T10:00:00Z" {
					t.Fatalf("got %s", ts.UTC().Format(time.RFC3339))
				}
			},
		},
		{
			name:  "rfc3339 with millis",
			input: "2026-10-15T10:00:00.123Z",
			check: func(t *testing.T, ts time.Time) {
				if ts.UTC().Format("2006-01-02T15:04:05.000") != "2026-10-15T10:00:00.123" {
					t.Fatalf("got %s", ts.UTC().Format(time.RFC3339))
				}
			},
		},
		{
			name:  "rfc3339 with offset keeps instant",
			input: "2026-10-15T10:00:00+01:00",
			check: func(t *testing.T, ts time.Time) {
				if ts.UTC().Format(time.RFC3339) != "2026-10-15T09:00:00Z" {
					t.Fatalf("got %s", ts.UTC().Format(time.RFC3339))
				}
			},
		},
		{
			name:  "postgres text format",
			input: "2026-10-15 10:00:00+00",
			check: func(t *testing.T, ts time.Time) {
				if ts.UTC().Format(time.RFC3339) != "2026-10-15T10:00:00Z" {
					t.Fatalf("got %s", ts.UTC().Format(time.RFC3339))
				}
			},
		},
		{name: "garbage", input: "not-a-time", wantErr: true},
		{name: "date only", input: "2026-10-15", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, err := ParseTimestamptz(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !ts.Valid {
				t.Fatal("expected Valid timestamptz")
			}

			if tc.check != nil {
				tc.check(t, ts.Time)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
		want    string // "2006-01-02"
	}{
		{name: "iso date", input: "2026-10-15", want: "2026-10-15"},
		{name: "postgres date", input: "2026-10-15", want: "2026-10-15"},
		{
			// The sender's calendar date is preserved even when the instant
			// falls on the previous day in UTC.
			name:  "rfc3339 keeps sender's date",
			input: "2026-10-15T00:30:00+05:00",
			want:  "2026-10-15",
		},
		{name: "rfc3339 utc", input: "2026-10-15T12:00:00Z", want: "2026-10-15"},
		{name: "garbage", input: "not-a-date", wantErr: true},
		{name: "datetime-local without zone", input: "2026-10-15T10:00", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := ParseDate(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !d.Valid {
				t.Fatal("expected Valid date")
			}

			if got := d.Time.Format("2006-01-02"); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
