// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073).
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
package scholarships

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestHandleSaveScholarship_NoAuth_401 verifies that saving requires auth.
func TestHandleSaveScholarship_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	r := chi.NewRouter()
	r.Post("/me/scholarships/{id}/save", h.HandleSaveScholarship)

	req := httptest.NewRequest(http.MethodPost, "/me/scholarships/00000000-0000-0000-0000-000000000001/save", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestHandleUpdateSavedScholarship_NoAuth_401 verifies that updating requires auth.
func TestHandleUpdateSavedScholarship_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	r := chi.NewRouter()
	r.Patch("/me/scholarships/{id}", h.HandleUpdateSavedScholarship)

	req := httptest.NewRequest(http.MethodPatch, "/me/scholarships/00000000-0000-0000-0000-000000000001", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestHandleDeleteSavedScholarship_NoAuth_401 verifies that deleting requires auth.
func TestHandleDeleteSavedScholarship_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	r := chi.NewRouter()
	r.Delete("/me/scholarships/{id}", h.HandleDeleteSavedScholarship)

	req := httptest.NewRequest(http.MethodDelete, "/me/scholarships/00000000-0000-0000-0000-000000000001", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestHandleListSavedScholarships_NoAuth_401 verifies that listing requires auth.
func TestHandleListSavedScholarships_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/me/scholarships", nil)
	rr := httptest.NewRecorder()
	h.HandleListSavedScholarships(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestHandleAdminStatusDistribution_NoAuth_401 verifies admin routes require auth.
func TestHandleAdminStatusDistribution_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/admin/scholarships/stats", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminStatusDistribution(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestIsValidStatus verifies the status validation logic.
func TestIsValidStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{StatusSaved, true},
		{StatusResearching, true},
		{StatusApplied, true},
		{StatusAccepted, true},
		{StatusRejected, true},
		{StatusWithdrawn, true},
		{"", false},
		{"invalid", false},
		{"SAVED", false}, // case-sensitive
		{"pending", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := IsValidStatus(tt.status)
			if got != tt.want {
				t.Errorf("IsValidStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
