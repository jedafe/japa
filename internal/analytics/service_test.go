package analytics

import (
	"testing"
	"time"
)

func TestValidEventTypes(t *testing.T) {
	t.Parallel()

	events := ValidEventTypes()
	if len(events) == 0 {
		t.Fatal("expected at least one event type")
	}

	seen := make(map[EventType]bool)
	for _, e := range events {
		if seen[e] {
			t.Errorf("duplicate event type: %s", e)
		}

		seen[e] = true
	}
}

func TestEventFilterDefaults(t *testing.T) {
	t.Parallel()

	f := EventFilter{}
	if f.Limit != 0 {
		t.Errorf("expected zero limit for empty filter, got %d", f.Limit)
	}
}

func TestSecurityScanFilterDefaults(t *testing.T) {
	t.Parallel()

	f := SecurityScanFilter{}
	if f.Limit != 0 {
		t.Errorf("expected zero limit for empty filter, got %d", f.Limit)
	}
}

func TestScanSeverityConstants(t *testing.T) {
	t.Parallel()

	if SeverityInfo != "info" {
		t.Errorf("expected info, got %s", SeverityInfo)
	}

	if SeverityWarning != "warning" {
		t.Errorf("expected warning, got %s", SeverityWarning)
	}

	if SeverityCritical != "critical" {
		t.Errorf("expected critical, got %s", SeverityCritical)
	}
}

func TestScanTypeConstants(t *testing.T) {
	t.Parallel()

	types := []ScanType{
		ScanDependencyAudit,
		ScanFailedLoginSpike,
		ScanLargeErrorRate,
		ScanSLABreachCount,
		ScanSuspiciousIPs,
		ScanOrphanedMedia,
		ScanStaleTokens,
	}
	for _, st := range types {
		if st == "" {
			t.Error("empty scan type constant")
		}
	}
}

func TestUserDashboardZeroValue(t *testing.T) {
	t.Parallel()

	var d UserDashboard
	if d.TopContent != nil {
		t.Error("expected nil TopContent in zero-value dashboard")
	}
}

func TestAdminDashboardZeroValue(t *testing.T) {
	t.Parallel()

	var d AdminDashboard
	if d.DailyActiveUsers != nil {
		t.Error("expected nil DailyActiveUsers in zero-value dashboard")
	}
}

func TestTimeParseValid(t *testing.T) {
	t.Parallel()

	_, err := time.Parse("2006-01-02", "2026-01-15")
	if err != nil {
		t.Errorf("expected valid date parse, got %v", err)
	}
}

func TestTimeParseInvalid(t *testing.T) {
	t.Parallel()

	_, err := time.Parse("2006-01-02", "not-a-date")
	if err == nil {
		t.Error("expected error for invalid date")
	}
}
