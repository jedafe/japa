// Tests for the ADR-136 journey lifecycle graph and travel-window
// validation — pure logic, no database.
package japastage

import (
	"errors"
	"testing"
)

func TestValidJourneyStatus(t *testing.T) {
	for _, s := range []string{"planned", "active", "completed", "cancelled"} {
		if !ValidJourneyStatus(s) {
			t.Errorf("expected %q to be a valid status", s)
		}
	}

	for _, s := range []string{"", "done", "archived", "PLANNED"} {
		if ValidJourneyStatus(s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}

func TestValidJourneyTransition(t *testing.T) {
	valid := [][2]string{
		{"planned", "active"},
		{"planned", "cancelled"},
		{"active", "completed"},
		{"active", "cancelled"},
		{"active", "planned"},    // postponed trip
		{"cancelled", "planned"}, // reopen
		{"planned", "planned"},   // no-op resubmit
	}

	for _, pair := range valid {
		if !ValidJourneyTransition(pair[0], pair[1]) {
			t.Errorf("expected %s → %s to be legal", pair[0], pair[1])
		}
	}

	invalid := [][2]string{
		{"planned", "completed"}, // must pass through active
		{"completed", "active"},  // terminal
		{"completed", "planned"}, // terminal
		{"completed", "cancelled"},
		{"cancelled", "active"}, // reopen goes through planned
		{"cancelled", "completed"},
	}

	for _, pair := range invalid {
		if ValidJourneyTransition(pair[0], pair[1]) {
			t.Errorf("expected %s → %s to be rejected", pair[0], pair[1])
		}
	}
}

func TestValidateWindow(t *testing.T) {
	cases := []struct {
		start, end string
		wantErr    bool
	}{
		{"", "", false},
		{"2026-03-01", "", false},
		{"", "2026-03-10", false},
		{"2026-03-01", "2026-03-10", false},
		{"2026-03-10", "2026-03-10", false}, // same-day window ok
		{"2026-03-10", "2026-03-01", true},  // end before start
		{"not-a-date", "", true},
		{"2026-13-01", "", true}, // month 13
		{"", "2026/03/01", true}, // wrong separator
	}

	for _, c := range cases {
		err := validateWindow(c.start, c.end)
		if c.wantErr && !errors.Is(err, ErrInvalidJourneyDate) {
			t.Errorf("start=%q end=%q: expected ErrInvalidJourneyDate, got %v", c.start, c.end, err)
		}

		if !c.wantErr && err != nil {
			t.Errorf("start=%q end=%q: unexpected error %v", c.start, c.end, err)
		}
	}
}

func TestJourneyClassStageRule(t *testing.T) {
	// A visit journey can reach traveled/returned but never relocated/settled.
	if !ValidStageForIntent("traveled", "visit") {
		t.Error("traveled should be valid for visit")
	}

	if ValidStageForIntent("settled", "business") {
		t.Error("settled must not be valid for business")
	}

	if !ValidStageForIntent("settled", "relocation") {
		t.Error("settled should be valid for relocation")
	}
}
