// Package jobs — handlers_test.go tests the Jobs Board HTTP handlers and
// service helpers. Includes auth denial tests for authenticated routes and
// table-driven status validation tests.
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md
//
//	japaconnect-engineering/adr/ADR-081-jobs-board.md
package jobs

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleApply_Unauthorized verifies that the apply endpoint requires
// authentication. No auth context is set, so requireUser should return 401.
func TestHandleApply_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/jobs/00000000-0000-0000-0000-000000000001/apply", nil)
	rr := httptest.NewRecorder()
	h.HandleApply(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleApply without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleMyApplications_Unauthorized verifies that the my applications
// endpoint requires authentication.
func TestHandleMyApplications_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/me/jobs/applications", nil)
	rr := httptest.NewRecorder()
	h.HandleMyApplications(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleMyApplications without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleListJobApplications_Unauthorized verifies that the employer
// applications list endpoint requires authentication.
func TestHandleListJobApplications_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/org/00000000-0000-0000-0000-000000000001/jobs/00000000-0000-0000-0000-000000000002/applications", nil)
	rr := httptest.NewRecorder()
	h.HandleListJobApplications(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleListJobApplications without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleUpdateApplicationStatus_Unauthorized verifies that the update
// application status endpoint requires authentication.
func TestHandleUpdateApplicationStatus_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/org/00000000-0000-0000-0000-000000000001/jobs/00000000-0000-0000-0000-000000000002/applications/00000000-0000-0000-0000-000000000003", nil)
	rr := httptest.NewRecorder()
	h.HandleUpdateApplicationStatus(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleUpdateApplicationStatus without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminListJobs_Unauthorized verifies that the admin listings
// endpoint requires authentication.
func TestHandleAdminListJobs_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminListJobs(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminListJobs without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminToggleFeature_Unauthorized verifies that the admin feature
// toggle endpoint requires authentication.
func TestHandleAdminToggleFeature_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/admin/jobs/00000000-0000-0000-0000-000000000001/feature", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminToggleFeature(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminToggleFeature without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestJobStatusValidation runs table-driven tests for valid and invalid
// status values against the IsValidStatus helper. The valid set must match
// the CHECK constraint on job_applications.status.
func TestJobStatusValidation(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		// Valid statuses (match the CHECK constraint).
		{"applied is valid", StatusApplied, true},
		{"reviewing is valid", StatusReviewing, true},
		{"shortlisted is valid", StatusShortlisted, true},
		{"rejected is valid", StatusRejected, true},
		{"offered is valid", StatusOffered, true},
		{"withdrawn is valid", StatusWithdrawn, true},

		// Invalid statuses.
		{"empty string is invalid", "", false},
		{"pending is invalid", "pending", false},
		{"accepted is invalid", "accepted", false},
		{"declined is invalid", "declined", false},
		{"hired is invalid", "hired", false},
		{"APPLIED uppercase is invalid", "APPLIED", false},
		{"Applied mixed case is invalid", "Applied", false},
		{"random string is invalid", "xyz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidStatus(tt.status)
			if got != tt.want {
				t.Errorf("IsValidStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

// TestStatusConstants verifies that the status constants match the expected
// string values defined in the schema CHECK constraint.
func TestStatusConstants(t *testing.T) {
	expected := map[string]string{
		"StatusApplied":     StatusApplied,
		"StatusReviewing":   StatusReviewing,
		"StatusShortlisted": StatusShortlisted,
		"StatusRejected":    StatusRejected,
		"StatusOffered":     StatusOffered,
		"StatusWithdrawn":   StatusWithdrawn,
	}

	want := map[string]string{
		"StatusApplied":     "applied",
		"StatusReviewing":   "reviewing",
		"StatusShortlisted": "shortlisted",
		"StatusRejected":    "rejected",
		"StatusOffered":     "offered",
		"StatusWithdrawn":   "withdrawn",
	}

	for name, val := range expected {
		if val != want[name] {
			t.Errorf("%s = %q, want %q", name, val, want[name])
		}
	}
}
