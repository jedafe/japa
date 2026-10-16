package worker

import (
	"testing"
)

// The helpers depend on dbPool which is nil in unit tests — the fallback
// path is the testable unit here.
func TestOnboardingReminderIntSetting_NilPoolFallback(t *testing.T) {
	if got := onboardingReminderIntSetting(t.Context(), "onboarding.reminder_max_count", 5); got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

func TestOnboardingReminderEnabled_NilPoolFallback(t *testing.T) {
	if !onboardingReminderEnabled(t.Context()) {
		t.Fatal("expected enabled fallback with nil pool")
	}
}
