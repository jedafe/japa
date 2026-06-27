package tickets

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Constants and configuration
// ---------------------------------------------------------------------------

func TestTicketStatusConstants(t *testing.T) {
	expected := map[TicketStatus]string{
		StatusOpen:        "open",
		StatusInProgress:  "in_progress",
		StatusWaitingUser: "waiting_on_user",
		StatusResolved:    "resolved",
		StatusClosed:      "closed",
		StatusSpam:        "spam",
	}
	for got, want := range expected {
		if string(got) != want {
			t.Errorf("status constant = %q, want %q", got, want)
		}
	}
}

func TestTicketPriorityConstants(t *testing.T) {
	expected := map[TicketPriority]string{
		PriorityLow:    "low",
		PriorityNormal: "normal",
		PriorityHigh:   "high",
		PriorityUrgent: "urgent",
	}
	for got, want := range expected {
		if string(got) != want {
			t.Errorf("priority constant = %q, want %q", got, want)
		}
	}
}

func TestEventTypeConstants(t *testing.T) {
	expected := map[EventType]string{
		EventStatusChanged:   "status_changed",
		EventPriorityChanged: "priority_changed",
		EventAssigned:        "assigned",
		EventCommentAdded:    "comment_added",
		EventSLABreached:     "sla_breached",
	}
	for got, want := range expected {
		if string(got) != want {
			t.Errorf("event type constant = %q, want %q", got, want)
		}
	}
}

func TestPaginationDefaults(t *testing.T) {
	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}
	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

// ---------------------------------------------------------------------------
// Status state machine
// ---------------------------------------------------------------------------

func TestIsValidStatusTransition(t *testing.T) {
	validCases := []struct {
		from TicketStatus
		to   TicketStatus
	}{
		{StatusOpen, StatusInProgress},
		{StatusOpen, StatusSpam},
		{StatusOpen, StatusClosed},
		{StatusInProgress, StatusWaitingUser},
		{StatusInProgress, StatusResolved},
		{StatusInProgress, StatusSpam},
		{StatusInProgress, StatusClosed},
		{StatusWaitingUser, StatusInProgress},
		{StatusWaitingUser, StatusResolved},
		{StatusWaitingUser, StatusSpam},
		{StatusWaitingUser, StatusClosed},
		{StatusResolved, StatusClosed},
		{StatusResolved, StatusSpam},
		{StatusClosed, StatusSpam},
	}

	for _, tc := range validCases {
		if !IsValidStatusTransition(tc.from, tc.to) {
			t.Errorf("expected transition %s → %s to be valid", tc.from, tc.to)
		}
	}

	invalidCases := []struct {
		from TicketStatus
		to   TicketStatus
	}{
		{StatusOpen, StatusResolved},
		{StatusOpen, StatusWaitingUser},
		{StatusInProgress, StatusOpen},
		{StatusWaitingUser, StatusOpen},
		{StatusResolved, StatusOpen},
		{StatusClosed, StatusInProgress},
		{StatusClosed, StatusOpen},
		{StatusSpam, StatusInProgress},
		{StatusSpam, StatusOpen},
	}

	for _, tc := range invalidCases {
		if IsValidStatusTransition(tc.from, tc.to) {
			t.Errorf("expected transition %s → %s to be invalid", tc.from, tc.to)
		}
	}
}

func TestAllStatuses(t *testing.T) {
	statuses := AllStatuses()
	if len(statuses) != 6 {
		t.Errorf("AllStatuses() returned %d items, want 6", len(statuses))
	}
}

func TestAllPriorities(t *testing.T) {
	priorities := AllPriorities()
	if len(priorities) != 4 {
		t.Errorf("AllPriorities() returned %d items, want 4", len(priorities))
	}
}
