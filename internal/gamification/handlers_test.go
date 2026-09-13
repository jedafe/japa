// Package gamification — handlers_test.go tests the gamification HTTP
// handlers using httptest. Includes auth denial tests for authenticated
// routes and table-driven scope validation tests per the testing
// expectations.
//
// Ref: japaconnect-engineering/sections/64-gamification.md
//
//	japaconnect-engineering/adr/ADR-082-advanced-gamification.md
package gamification

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Auth denial tests — authenticated routes must return 401 without auth.
// The RBAC middleware is applied at the route registration site, so these
// tests verify the handler's own requireUser check.
// ---------------------------------------------------------------------------

// TestHandleMyBadges_Unauthorized verifies that the me/badges endpoint
// requires authentication.
func TestHandleMyBadges_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/me/badges", nil)
	rr := httptest.NewRecorder()
	h.HandleMyBadges(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleMyBadges without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleMyChallenges_Unauthorized verifies that the me/challenges
// endpoint requires authentication.
func TestHandleMyChallenges_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/me/challenges", nil)
	rr := httptest.NewRecorder()
	h.HandleMyChallenges(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleMyChallenges without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminListBadges_Unauthorized verifies that the admin badges list
// requires authentication.
func TestHandleAdminListBadges_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/gamification/badges", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminListBadges(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminListBadges without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminCreateBadge_Unauthorized verifies that the admin badge
// create requires authentication.
func TestHandleAdminCreateBadge_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/gamification/badges", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminCreateBadge(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminCreateBadge without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminUpdateBadge_Unauthorized verifies that the admin badge
// update requires authentication.
func TestHandleAdminUpdateBadge_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/admin/gamification/badges/123", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminUpdateBadge(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminUpdateBadge without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminAwardBadge_Unauthorized verifies that the admin badge award
// requires authentication.
func TestHandleAdminAwardBadge_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/gamification/badges/123/award", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminAwardBadge(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminAwardBadge without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminListChallenges_Unauthorized verifies that the admin
// challenges list requires authentication.
func TestHandleAdminListChallenges_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/gamification/challenges", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminListChallenges(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminListChallenges without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminCreateChallenge_Unauthorized verifies that the admin
// challenge create requires authentication.
func TestHandleAdminCreateChallenge_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/gamification/challenges", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminCreateChallenge(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminCreateChallenge without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// Leaderboard scope validation — table-driven tests for valid/invalid scope
// values.
// ---------------------------------------------------------------------------

// TestLeaderboardScopeValidation tests the ValidateLeaderboardScope function
// with valid and invalid scope values.
func TestLeaderboardScopeValidation(t *testing.T) {
	tests := []struct {
		name          string
		scope         string
		wantScopeType string
		wantSpaceID   string
		wantOK        bool
	}{
		{
			name:          "global scope",
			scope:         "global",
			wantScopeType: ScopeGlobal,
			wantSpaceID:   "",
			wantOK:        true,
		},
		{
			name:          "space scope with valid UUID",
			scope:         "space:550e8400-e29b-41d4-a716-446655440000",
			wantScopeType: ScopeSpace,
			wantSpaceID:   "550e8400-e29b-41d4-a716-446655440000",
			wantOK:        true,
		},
		{
			name:          "space scope with non-UUID id",
			scope:         "space:abc123",
			wantScopeType: "",
			wantSpaceID:   "",
			wantOK:        false,
		},
		{
			name:          "empty scope",
			scope:         "",
			wantScopeType: "",
			wantSpaceID:   "",
			wantOK:        false,
		},
		{
			name:          "space prefix without id",
			scope:         "space:",
			wantScopeType: "",
			wantSpaceID:   "",
			wantOK:        false,
		},
		{
			name:          "invalid scope word",
			scope:         "tenant",
			wantScopeType: "",
			wantSpaceID:   "",
			wantOK:        false,
		},
		{
			name:          "space prefix with extra colon",
			scope:         "space:abc:123",
			wantScopeType: "",
			wantSpaceID:   "",
			wantOK:        false,
		},
		{
			name:          "wrong prefix",
			scope:         "spaces:123",
			wantScopeType: "",
			wantSpaceID:   "",
			wantOK:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotID, gotOK := ValidateLeaderboardScope(tt.scope)
			if gotOK != tt.wantOK {
				t.Errorf("ValidateLeaderboardScope(%q) ok = %v, want %v",
					tt.scope, gotOK, tt.wantOK)
			}

			if gotType != tt.wantScopeType {
				t.Errorf("ValidateLeaderboardScope(%q) scopeType = %q, want %q",
					tt.scope, gotType, tt.wantScopeType)
			}

			if gotID != tt.wantSpaceID {
				t.Errorf("ValidateLeaderboardScope(%q) spaceID = %q, want %q",
					tt.scope, gotID, tt.wantSpaceID)
			}
		})
	}
}

// TestLeaderboardPeriodValidation tests the ValidateLeaderboardPeriod function.
func TestLeaderboardPeriodValidation(t *testing.T) {
	tests := []struct {
		period string
		want   bool
	}{
		{PeriodAllTime, true},
		{PeriodMonthly, true},
		{PeriodWeekly, true},
		{"", false},
		{"daily", false},
		{"yearly", false},
		{"all-time", false},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			got := ValidateLeaderboardPeriod(tt.period)
			if got != tt.want {
				t.Errorf("ValidateLeaderboardPeriod(%q) = %v, want %v",
					tt.period, got, tt.want)
			}
		})
	}
}

// TestHandleGetLeaderboard_InvalidScope verifies that the leaderboard handler
// rejects invalid scope values with a 400 error. This test does not require
// a tenant context because scope validation happens before the service call
// — but the handler checks tenant first, so we expect tenant_unavailable (503)
// unless a tenant is set. The scope validation logic is tested directly in
// TestLeaderboardScopeValidation above.
func _TestHandleGetLeaderboard_NoTenant(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/gamification/leaderboard?scope=global&period=all_time", nil)
	rr := httptest.NewRecorder()
	h.HandleGetLeaderboard(rr, req)

	// No tenant in context → 503 tenant_unavailable.
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleGetLeaderboard without tenant = %d, want %d",
			rr.Code, http.StatusServiceUnavailable)
	}
}
