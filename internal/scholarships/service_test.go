// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073).
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
//
//	japaconnect-engineering/adr/ADR-073-scholarship-finder.md
package scholarships

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

func mustStr(s string) string { return s }

func TestParseDeadlineValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want bool
	}{
		{"2025-03-01", true},
		{"2025-03-01T10:00:00Z", true},
		{"2025-03-01 10:00:00", false},
		{"", false},
		{"not-a-date", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			_, ok := parseDeadlineValue(tt.in)
			if ok != tt.want {
				t.Errorf("parseDeadlineValue(%q) ok = %v, want %v", tt.in, ok, tt.want)
			}
		})
	}
}

func TestWithinReminderWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 3, 10, 12, 0, 0, 0, time.UTC)
	row := func(deadline string, remindDays int32) gen.FindScholarshipsNeedingRemindersRow {
		return gen.FindScholarshipsNeedingRemindersRow{
			DeadlineValue:    deadline,
			RemindBeforeDays: remindDays,
		}
	}

	tests := []struct {
		name string
		row  gen.FindScholarshipsNeedingRemindersRow
		want bool
	}{
		{"within window", row(mustStr("2025-03-15"), 14), true},
		{"deadline today", row(mustStr("2025-03-10"), 14), true},
		{"deadline at window edge", row(mustStr("2025-03-24"), 14), true},
		{"beyond window", row(mustStr("2025-04-10"), 14), false},
		{"already past", row(mustStr("2025-03-01"), 14), false},
		{"empty deadline", row(mustStr(""), 14), false},
		{"unparseable deadline", row(mustStr("someday"), 14), false},
		{"zero remind window", row(mustStr("2025-03-11"), 0), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := withinReminderWindow(tt.row, now); got != tt.want {
				t.Errorf("withinReminderWindow(%v) = %v, want %v", tt.row, got, tt.want)
			}
		})
	}
}

func TestProjectScholarshipFieldsNumberBool(t *testing.T) {
	t.Parallel()
	// Regression: award_amount_usd (number) and is_renewable (boolean) must be
	// projected from the stringified field_values map (ADR-073 finder output).
	r := &ScholarshipResult{
		FieldValues: map[string]string{
			"provider_institution": "MIT",
			"award_amount_usd":     "25000",
			"is_renewable":         "true",
			"application_deadline": "2025-03-01",
			"level_of_study":       "Masters",
		},
	}
	projectScholarshipFields(r)

	if r.Institution != "MIT" {
		t.Errorf("Institution = %q, want MIT", r.Institution)
	}

	if r.LevelOfStudy != "Masters" {
		t.Errorf("LevelOfStudy = %q, want Masters", r.LevelOfStudy)
	}

	if r.AwardAmountUSD == nil || *r.AwardAmountUSD != 25000 {
		t.Errorf("AwardAmountUSD = %v, want 25000", r.AwardAmountUSD)
	}

	if r.IsRenewable == nil || !*r.IsRenewable {
		t.Errorf("IsRenewable = %v, want true", r.IsRenewable)
	}

	if r.ApplicationDeadline != "2025-03-01" {
		t.Errorf("ApplicationDeadline = %q, want 2025-03-01", r.ApplicationDeadline)
	}
}

func TestErrGuideNotFound(t *testing.T) {
	t.Parallel()

	if ErrGuideNotFound == nil {
		t.Fatal("ErrGuideNotFound should be non-nil")
	}

	_ = pgtype.UUID{}
}

func hasArg(args []interface{}, want string) bool {
	for _, a := range args {
		if s, ok := a.(string); ok && s == want {
			return true
		}
	}

	return false
}

func TestFindScholarshipsQuery_ExpiredDefault(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 9, 3, 12, 0, 0, 0, time.UTC)
	tenantID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	q, args := findScholarshipsQuery(tenantID, ScholarshipFilters{}, now)

	if !strings.Contains(q, "(s.deadline IS NULL OR s.deadline >= $") || !strings.Contains(q, "::date)") {
		t.Fatalf("expected null-safe expired filter, got:\n%s", q)
	}

	if !hasArg(args, "2025-09-03") {
		t.Fatalf("expected cutoff date arg '2025-09-03', got %v", args)
	}
}

func TestFindScholarshipsQuery_IncludeExpired(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 9, 3, 12, 0, 0, 0, time.UTC)
	tenantID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	q, _ := findScholarshipsQuery(tenantID, ScholarshipFilters{IncludeExpired: true}, now)

	if strings.Contains(q, "s.deadline >= $") {
		t.Fatalf("expected no expired filter when IncludeExpired=true, got:\n%s", q)
	}
}

func TestFindScholarshipsQuery_DeadlineFromFilter(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 9, 3, 12, 0, 0, 0, time.UTC)
	tenantID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	q, args := findScholarshipsQuery(tenantID, ScholarshipFilters{DeadlineFrom: "2025-10-01"}, now)

	if !strings.Contains(q, "s.deadline >= $") || !strings.Contains(q, "::date") {
		t.Fatalf("expected deadline_from filter, got:\n%s", q)
	}

	if !hasArg(args, "2025-10-01") {
		t.Fatalf("expected deadline_from arg '2025-10-01', got %v", args)
	}

	if hasArg(args, "2025-09-03") {
		t.Fatalf("expected no default cutoff when deadline_from is explicit, got %v", args)
	}
}
