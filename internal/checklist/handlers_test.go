// Package checklist — handlers_test.go tests the checklist HTTP handlers
// using httptest. Includes auth denial tests for the authenticated routes
// and table-driven tests for the slugifyItemKey helper.
//
// Ref: japaconnect-engineering/sections/70-visa-checklist.md
//      japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md
package checklist

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Auth denial tests — authenticated routes must 401 without an auth context.
// The RBAC middleware is applied at the route registration site, so these
// tests verify the handler's own requireUser check.
// ---------------------------------------------------------------------------

// TestHandleGetProgress_Unauthorized verifies the progress endpoint requires
// authentication.
func TestHandleGetProgress_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/checklist/00000000-0000-0000-0000-000000000000/progress", nil)
	rr := httptest.NewRecorder()
	h.HandleGetProgress(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleGetProgress without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleUpdateProgress_Unauthorized verifies the update-progress endpoint
// requires authentication.
func TestHandleUpdateProgress_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPatch,
		"/checklist/00000000-0000-0000-0000-000000000000/progress/bank-statement", nil)
	rr := httptest.NewRecorder()
	h.HandleUpdateProgress(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleUpdateProgress without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleResetProgress_Unauthorized verifies the reset-progress endpoint
// requires authentication.
func TestHandleResetProgress_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodDelete,
		"/checklist/00000000-0000-0000-0000-000000000000/progress", nil)
	rr := httptest.NewRecorder()
	h.HandleResetProgress(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleResetProgress without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminListProgress_Unauthorized verifies the admin list-progress
// endpoint requires authentication.
func TestHandleAdminListProgress_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/checklist/progress", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminListProgress(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminListProgress without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminCompletionStats_Unauthorized verifies the admin
// completion-stats endpoint requires authentication.
func TestHandleAdminCompletionStats_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/checklist/completion-stats", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminCompletionStats(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminCompletionStats without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// Public route tests — selector and get-items require a tenant (503 without
// one) but NOT authentication.
// ---------------------------------------------------------------------------

// TestHandleSelector_NoTenant verifies the public selector endpoint requires
// a tenant context (returns 503, not 401 — it is anonymous-accessible).
func TestHandleSelector_NoTenant(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/checklist/selector", nil)
	rr := httptest.NewRecorder()
	h.HandleSelector(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleSelector without tenant = %d, want %d (tenant_unavailable)", rr.Code, http.StatusServiceUnavailable)
	}
}

// TestHandleGetItems_NoTenant verifies the public get-items endpoint requires
// a tenant context.
func TestHandleGetItems_NoTenant(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet,
		"/checklist/00000000-0000-0000-0000-000000000000", nil)
	rr := httptest.NewRecorder()
	h.HandleGetItems(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleGetItems without tenant = %d, want %d (tenant_unavailable)", rr.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// slugifyItemKey table-driven tests.
// ---------------------------------------------------------------------------

// TestSlugifyItemKey verifies the stable-slug derivation from document labels.
func TestSlugifyItemKey(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{"Bank Statement", "bank-statement"},
		{"Passport Copy", "passport-copy"},
		{"Passport Photo (2x)", "passport-photo-2x"},
		{"  Birth  Certificate  ", "birth-certificate"},
		{"Medical Exam Report", "medical-exam-report"},
		{"Proof of Funds / Bank Letter", "proof-of-funds-bank-letter"},
		{"I-20 Form", "i-20-form"},
		{"CAS (Confirmation of Acceptance)", "cas-confirmation-of-acceptance"},
		{"Biometrics Appointment", "biometrics-appointment"},
		{"!!!", "item"}, // no alphanumeric → fallback
		{"", "item"},    // empty → fallback
		{"Visa Fee Receipt", "visa-fee-receipt"},
		{"Academic Transcript", "academic-transcript"},
		{"English Test Score (IELTS/TOEFL)", "english-test-score-ielts-toefl"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := slugifyItemKey(tt.label)
			if got != tt.want {
				t.Errorf("slugifyItemKey(%q) = %q, want %q", tt.label, got, tt.want)
			}
		})
	}
}

// TestSlugifyItemKey_Stability verifies that the same label always produces
// the same key (idempotent — re-running must not change saved progress keys).
func TestSlugifyItemKey_Stability(t *testing.T) {
	label := "Bank Statement"
	first := slugifyItemKey(label)
	for i := 0; i < 10; i++ {
		if got := slugifyItemKey(label); got != first {
			t.Fatalf("slugifyItemKey not stable: iteration %d got %q, want %q", i, got, first)
		}
	}
}

// ---------------------------------------------------------------------------
// extractDocumentLabels tests.
// ---------------------------------------------------------------------------

// TestExtractDocumentItems_JSONArray verifies parsing a JSON array of strings.
func TestExtractDocumentLabels_JSONArray(t *testing.T) {
	field := VisaGuideField{
		FieldKey:  FieldRequiredDocuments,
		ValueJSON: []byte(`["Bank Statement","Passport Copy","Medical Exam Report"]`),
	}
	got := extractDocumentItems(field)
	if len(got) != 3 {
		t.Fatalf("extractDocumentItems JSON array: got %d labels, want 3", len(got))
	}
	if got[0].Label != "Bank Statement" {
		t.Errorf("extractDocumentItems[0].Label = %q, want %q", got[0].Label, "Bank Statement")
	}
}

// TestExtractDocumentItems_JSONObjects verifies parsing a JSON array of
// objects with label/required/category/description metadata.
func TestExtractDocumentItems_JSONObjects(t *testing.T) {
	field := VisaGuideField{
		FieldKey: FieldRequiredDocuments,
		ValueJSON: []byte(`[
			{"label":"Passport Copy","required":true,"category":"Identity","description":"Valid for 6+ months"},
			{"label":"Bank Statement","required":true,"category":"Financial"}
		]`),
	}
	got := extractDocumentItems(field)
	if len(got) != 2 {
		t.Fatalf("extractDocumentItems JSON objects: got %d items, want 2", len(got))
	}
	if got[0].Label != "Passport Copy" || !got[0].Required || got[0].Category != "Identity" {
		t.Errorf("extractDocumentItems[0] = %+v, want label=Passport Copy required=true category=Identity", got[0])
	}
	if got[0].Description != "Valid for 6+ months" {
		t.Errorf("extractDocumentItems[0].Description = %q, want %q", got[0].Description, "Valid for 6+ months")
	}
}

// TestExtractDocumentItems_Text verifies parsing a comma-separated text value.
func TestExtractDocumentLabels_Text(t *testing.T) {
	field := VisaGuideField{
		FieldKey:  FieldRequiredDocuments,
		ValueText: "Bank Statement, Passport Copy, Medical Exam Report",
	}
	got := extractDocumentItems(field)
	if len(got) != 3 {
		t.Fatalf("extractDocumentItems text: got %d labels, want 3", len(got))
	}
}

// TestExtractDocumentItems_Empty verifies an empty field returns no items.
func TestExtractDocumentLabels_Empty(t *testing.T) {
	field := VisaGuideField{FieldKey: FieldRequiredDocuments}
	if got := extractDocumentItems(field); len(got) != 0 {
		t.Errorf("extractDocumentItems empty = %v, want nil", got)
	}
}
