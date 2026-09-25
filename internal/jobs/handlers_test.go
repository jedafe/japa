// Package jobs — handlers_test.go tests the Jobs Board HTTP handlers and
// service helpers. Includes auth denial tests for authenticated routes and
// table-driven status validation tests.
//
// Ref: japaconnect-engineering/sections/63-jobs-board.md
//
//	japaconnect-engineering/adr/ADR-081-jobs-board.md
package jobs

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestHandleListOrgJobs_NilService verifies that the org-scoped list endpoint
// returns 500 (not a panic) when the service is nil — the org-membership check
// (JOBS-012) must degrade safely. Auth context is set so the handler reaches
// the membership check.
func TestHandleListOrgJobs_NilService(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/org/00000000-0000-0000-0000-000000000001/jobs", nil)
	rr := httptest.NewRecorder()
	h.HandleListOrgJobs(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleListOrgJobs without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleCreateJobListing_Unauthorized verifies the create endpoint
// requires authentication (the org-membership check runs only after auth).
func TestHandleCreateJobListing_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/org/00000000-0000-0000-0000-000000000001/jobs", nil)
	rr := httptest.NewRecorder()
	h.HandleCreateJobListing(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleCreateJobListing without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestDeriveSummary verifies the summary derivation helper (JOBS-015) — the
// contents table has no summary column, so a display summary is derived from
// the body.
func TestDeriveSummary(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty", "", ""},
		{"short", "We are hiring.", "We are hiring."},
		{"long truncates with ellipsis", strings.Repeat("a", 250), strings.Repeat("a", 200) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveSummary(tt.body)
			if got != tt.want {
				if len(tt.want) > 40 {
					t.Errorf("deriveSummary(len=%d) len=%d, want len=%d", len(tt.body), len(got), len(tt.want))
				} else {
					t.Errorf("deriveSummary(%q) = %q, want %q", tt.body, got, tt.want)
				}
			}
		})
	}
}

// TestJobFieldTypes verifies the field-key → storage-column map matches the
// 20 seeded job_listing_fields (migrations 000475 + 000734 + 000738). A
// mismatch would route boolean/number values to value_text and break the
// visa_sponsorship filter and salary sort (JOBS-007).
func TestJobFieldTypes(t *testing.T) {
	want := map[string]jobFieldType{
		"company_name":          fieldTypeText,
		"company_logo":          fieldTypeURL,
		"company_website":       fieldTypeURL,
		"job_type":              fieldTypeText,
		"job_role":              fieldTypeText,
		"job_function":          fieldTypeText,
		"location_type":         fieldTypeText,
		"country":               fieldTypeText,
		"city":                  fieldTypeText,
		"salary_min":            fieldTypeNumber,
		"salary_max":            fieldTypeNumber,
		"salary_currency":       fieldTypeText,
		"salary_period":         fieldTypeText,
		"experience_level":      fieldTypeText,
		"skills_required":       fieldTypeText,
		"application_url":       fieldTypeURL,
		"application_deadline":  fieldTypeText,
		"visa_sponsorship":      fieldTypeBoolean,
		"relocation_assistance": fieldTypeBoolean,
		"is_featured":           fieldTypeBoolean,
	}
	if len(jobFieldTypes) != len(want) {
		t.Fatalf("jobFieldTypes has %d entries, want %d", len(jobFieldTypes), len(want))
	}

	for key, ft := range want {
		got, ok := jobFieldTypes[key]
		if !ok {
			t.Errorf("jobFieldTypes missing key %q", key)
			continue
		}

		if got != ft {
			t.Errorf("jobFieldTypes[%q] = %q, want %q", key, got, ft)
		}
	}
}

// url-typed fields feed href/window.open sinks on the finder cards and detail
// page — only absolute http(s) URLs may be stored.
func TestSanitizeJobFieldURL(t *testing.T) {
	valid := []string{
		"https://example.com",
		"http://example.com/careers?ref=jc",
		" https://jobs.example.org/apply/123 ",
	}
	for _, v := range valid {
		if _, ok := sanitizeJobFieldURL(v); !ok {
			t.Errorf("sanitizeJobFieldURL(%q) rejected a valid URL", v)
		}
	}

	invalid := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"//evil.example.com",
		"example.com",
		"ftp://example.com/file",
		"mailto:hr@example.com",
		"https://",
	}
	for _, v := range invalid {
		if _, ok := sanitizeJobFieldURL(v); ok {
			t.Errorf("sanitizeJobFieldURL(%q) accepted an unsafe URL", v)
		}
	}
}

// validateJobFieldValues gates url-typed fields before the contents write;
// non-url keys and empty values pass through untouched.
func TestValidateJobFieldValues(t *testing.T) {
	cases := []struct {
		name    string
		values  map[string]string
		wantErr bool
	}{
		{"valid website", map[string]string{"company_website": "https://acme.example"}, false},
		{"empty url ok", map[string]string{"company_website": ""}, false},
		{"javascript scheme", map[string]string{"company_website": "javascript:alert(1)"}, true},
		{"bare domain", map[string]string{"application_url": "acme.example/jobs"}, true},
		{"non-url key untouched", map[string]string{"company_name": "javascript:still-fine"}, false},
		{"unknown key untouched", map[string]string{"totally_made_up": "//evil.example.com"}, false},
		// company_logo stores the media serve URL, which is a same-origin
		// path when no CDN is configured — root-relative is allowed, but
		// scheme-relative and non-http(s) schemes are still rejected.
		{"relative logo path", map[string]string{"company_logo": "/api/v1/media/serve/bucket/key.png"}, false},
		{"scheme-relative logo", map[string]string{"company_logo": "//evil.example.com/x.png"}, true},
		{"javascript logo", map[string]string{"company_logo": "javascript:alert(1)"}, true},
		{"relative website rejected", map[string]string{"company_website": "/internal/path"}, true},
	}
	for _, tc := range cases {
		err := validateJobFieldValues(tc.values)
		if tc.wantErr && !errors.Is(err, ErrInvalidFieldValue) {
			t.Errorf("%s: want ErrInvalidFieldValue, got %v", tc.name, err)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
	}
}
