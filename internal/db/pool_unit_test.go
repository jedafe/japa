package db

import (
	"strings"
	"testing"
	"time"
)

// TestDefaultPoolConfig verifies the default pool tuning matches the
// values documented in Section 27 (DevOps, Testing & QA).
func TestDefaultPoolConfig(t *testing.T) {
	t.Parallel()

	cfg := defaultPoolConfig()

	if cfg.MaxConns != 25 {
		t.Errorf("MaxConns = %d, want 25", cfg.MaxConns)
	}

	if cfg.MinConns != 5 {
		t.Errorf("MinConns = %d, want 5", cfg.MinConns)
	}

	if cfg.MaxConnLifetime != 30*time.Minute {
		t.Errorf("MaxConnLifetime = %v, want 30m", cfg.MaxConnLifetime)
	}

	if cfg.MaxConnIdleTime != 5*time.Minute {
		t.Errorf("MaxConnIdleTime = %v, want 5m", cfg.MaxConnIdleTime)
	}
}

// TestResolvePoolConfig_ZeroValues verifies that a zero-valued
// PoolConfig falls back to defaults for every field.
func TestResolvePoolConfig_ZeroValues(t *testing.T) {
	t.Parallel()

	resolved := resolvePoolConfig(PoolConfig{})
	def := defaultPoolConfig()

	if resolved != def {
		t.Errorf("resolvePoolConfig(zero) = %+v, want %+v", resolved, def)
	}
}

// TestResolvePoolConfig_PartialOverride verifies that non-zero fields
// are preserved while zero fields fall back to defaults.
func TestResolvePoolConfig_PartialOverride(t *testing.T) {
	t.Parallel()

	custom := PoolConfig{
		MaxConns: 50,
		// MinConns, MaxConnLifetime, MaxConnIdleTime left zero
	}
	resolved := resolvePoolConfig(custom)

	if resolved.MaxConns != 50 {
		t.Errorf("MaxConns = %d, want 50 (override)", resolved.MaxConns)
	}

	if resolved.MinConns != 5 {
		t.Errorf("MinConns = %d, want 5 (default)", resolved.MinConns)
	}

	if resolved.MaxConnLifetime != 30*time.Minute {
		t.Errorf("MaxConnLifetime = %v, want 30m (default)", resolved.MaxConnLifetime)
	}

	if resolved.MaxConnIdleTime != 5*time.Minute {
		t.Errorf("MaxConnIdleTime = %v, want 5m (default)", resolved.MaxConnIdleTime)
	}
}

// TestResolvePoolConfig_AllOverride verifies that all non-zero fields
// are preserved.
func TestResolvePoolConfig_AllOverride(t *testing.T) {
	t.Parallel()

	custom := PoolConfig{
		MaxConns:        100,
		MinConns:        10,
		MaxConnLifetime: 1 * time.Hour,
		MaxConnIdleTime: 15 * time.Minute,
	}
	resolved := resolvePoolConfig(custom)

	if resolved != custom {
		t.Errorf("resolvePoolConfig(all-set) = %+v, want %+v", resolved, custom)
	}
}

// TestVacuum_AllowListRejection verifies that a table name not in the
// allow-list is rejected with an error mentioning the table name.
func TestVacuum_AllowListRejection(t *testing.T) {
	t.Parallel()

	// We can't create a real DB pool in a unit test, but Vacuum validates
	// the table name against the allow-list BEFORE attempting to exec.
	// A nil DB will panic only if the allow-list check passes, so we
	// expect the allow-list error, not a nil-deref.
	db := &DB{Writer: nil, Reader: nil}

	err := db.Vacuum(t.Context(), "nonexistent_table", false)
	if err == nil {
		t.Fatal("Vacuum with non-allowlisted table returned nil error")
	}

	if !strings.Contains(err.Error(), "nonexistent_table") {
		t.Errorf("error %q does not mention the rejected table name", err)
	}

	if !strings.Contains(err.Error(), "allow-list") {
		t.Errorf("error %q does not mention the allow-list", err)
	}
}

// TestVacuum_AllowListAcceptance verifies that allow-listed table names
// pass the validation step. We cannot call Vacuum with a real pool in a
// unit test, so we only verify the table is present in the allow-list
// (which is the gate Vacuum checks before exec). The actual exec path
// is covered by integration tests.
func TestVacuum_AllowListAcceptance(t *testing.T) {
	t.Parallel()

	allowListed := []string{"contents", "users", "spaces", "comments", "media"}
	for _, table := range allowListed {
		if _, ok := allowedVacuumTables[table]; !ok {
			t.Errorf("table %q is not in allowedVacuumTables — test is stale", table)
		}
	}
}

// TestAllowedVacuumTables_CoversV2CommunityTables verifies that key
// v2 community/tool tables (sections 79–102) are in the allow-list.
// This is a regression test for BUG-001.
func TestAllowedVacuumTables_CoversV2CommunityTables(t *testing.T) {
	t.Parallel()

	v2Tables := []string{
		// §79 community story wall
		"story_meta",
		// §80 destination wiki
		"wiki_pages", "wiki_revisions",
		// §81 scam alert board
		"scam_reports", "scam_alert_subscriptions",
		// §82 planning groups
		"planning_groups", "planning_group_members",
		"planning_group_milestones", "planning_group_checkins",
		// §83 visa timeline
		"visa_timelines", "visa_timeline_milestones", "visa_processing_stats",
		// §84 digest widget
		"digest_editions",
		// §85 mutual aid
		"mutual_aid_posts",
		// §86 community calendar
		"calendar_entries", "calendar_subscriptions", "calendar_saves",
		// §89 achievement feed
		"achievement_meta",
		// §90 visa routes
		"visa_routes",
		// §91 embassy alerter
		"appointment_alerts", "monitored_embassies",
		// §92 arrival checklist
		"arrival_checklist_guides", "arrival_checklist_items",
		// §94 credential checker
		"credential_professions", "credential_pathways", "credential_community_notes",
		// §95 CV formatter
		"cv_standards", "cv_format_sessions",
		// §96 housing guide
		"housing_guides",
		// §97 attestation guide
		"attestation_pathways", "attestation_community_updates",
		// §98 service directory
		"service_listing_meta",
		// §99 language test hub
		"language_tests", "language_test_requirements",
		"language_test_resources", "language_score_stories",
		// §100 tax calculator
		"tax_treaty_pairs", "tax_obligation_guides",
		// §101 community locator
		"community_organisations",
		// §102 mentorship
		"mentorship_connections",
	}

	for _, table := range v2Tables {
		if _, ok := allowedVacuumTables[table]; !ok {
			t.Errorf("v2 table %q is missing from allowedVacuumTables (BUG-001 regression)", table)
		}
	}
}

// TestAllowedVacuumTables_CoversV1CoreTables verifies that core v1
// tables remain in the allow-list.
func TestAllowedVacuumTables_CoversV1CoreTables(t *testing.T) {
	t.Parallel()

	v1Tables := []string{
		"contents", "feeds", "notifications", "users", "user_profiles",
		"spaces", "space_members", "comments", "reactions", "bookmarks",
		"follows", "tickets", "payments", "media", "audit_logs",
	}

	for _, table := range v1Tables {
		if _, ok := allowedVacuumTables[table]; !ok {
			t.Errorf("v1 table %q is missing from allowedVacuumTables", table)
		}
	}
}
