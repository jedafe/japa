// Package calendar — handlers_test.go tests the calendar HTTP handlers
// using httptest. Includes auth denial tests for the authenticated routes
// and table-driven tests for parsePageParams.
//
// Ref: japaconnect-engineering/sections/86-community-calendar.md
//      japaconnect-engineering/adr/ADR-094-community-calendar.md
package calendar

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
)

// ---------------------------------------------------------------------------
// Auth denial tests — authenticated routes must 401 without an auth context.
// The RBAC middleware is applied at the route registration site, so these
// tests verify the handler's own requireUser check.
// ---------------------------------------------------------------------------

// TestHandleSave_Unauthorized verifies the save endpoint requires auth.
func TestHandleSave_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/calendar/00000000-0000-0000-0000-000000000000/save", nil)
	rr := httptest.NewRecorder()
	h.HandleSave(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleSave without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleUnsave_Unauthorized verifies the unsave endpoint requires auth.
func TestHandleUnsave_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/calendar/00000000-0000-0000-0000-000000000000/save", nil)
	rr := httptest.NewRecorder()
	h.HandleUnsave(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleUnsave without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleSubscribe_Unauthorized verifies the subscribe endpoint requires auth.
func TestHandleSubscribe_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/calendar/subscribe", nil)
	rr := httptest.NewRecorder()
	h.HandleSubscribe(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleSubscribe without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleUnsubscribe_Unauthorized verifies the unsubscribe endpoint requires auth.
func TestHandleUnsubscribe_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/calendar/subscribe", nil)
	rr := httptest.NewRecorder()
	h.HandleUnsubscribe(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleUnsubscribe without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleSubmit_Unauthorized verifies the submit endpoint requires auth.
func TestHandleSubmit_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/calendar", nil)
	rr := httptest.NewRecorder()
	h.HandleSubmit(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleSubmit without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminList_Unauthorized verifies the admin list endpoint requires auth.
func TestHandleAdminList_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/calendar", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminList(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminList without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminUpdate_Unauthorized verifies the admin update endpoint requires auth.
func TestHandleAdminUpdate_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/admin/calendar/00000000-0000-0000-0000-000000000000", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminUpdate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminUpdate without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleAdminDelete_Unauthorized verifies the admin delete endpoint requires auth.
// BUG-008: soft-delete admin endpoint gated by calendar.manage.
func TestHandleAdminDelete_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/admin/calendar/00000000-0000-0000-0000-000000000000", nil)
	rr := httptest.NewRecorder()
	h.HandleAdminDelete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleAdminDelete without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleListSaved_Unauthorized verifies the saved-list endpoint requires auth.
// BUG-003: /me/calendar/saved endpoint for user dashboard.
func TestHandleListSaved_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/me/calendar/saved", nil)
	rr := httptest.NewRecorder()
	h.HandleListSaved(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleListSaved without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// parsePageParams tests — verifies page_size/page_offset and legacy aliases.
// ---------------------------------------------------------------------------

func TestParsePageParams_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	page, limit := parsePageParams(req)
	if page != 1 {
		t.Errorf("page = %d, want 1", page)
	}
	if limit != 20 {
		t.Errorf("limit = %d, want 20", limit)
	}
}

func TestParsePageParams_PageSize(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/calendar?page_size=50&page=3", nil)
	page, limit := parsePageParams(req)
	if page != 3 {
		t.Errorf("page = %d, want 3", page)
	}
	if limit != 50 {
		t.Errorf("limit = %d, want 50", limit)
	}
}

func TestParsePageParams_LegacyLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/calendar?limit=10&page=2", nil)
	page, limit := parsePageParams(req)
	if page != 2 {
		t.Errorf("page = %d, want 2", page)
	}
	if limit != 10 {
		t.Errorf("limit = %d, want 10", limit)
	}
}

func TestParsePageParams_PageOffset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/calendar?page_offset=5&page_size=15", nil)
	page, limit := parsePageParams(req)
	if page != 5 {
		t.Errorf("page = %d, want 5", page)
	}
	if limit != 15 {
		t.Errorf("limit = %d, want 15", limit)
	}
}

func TestParsePageParams_InvalidValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/calendar?page=-1&page_size=abc", nil)
	page, limit := parsePageParams(req)
	if page != 1 {
		t.Errorf("page = %d, want 1 (default for invalid)", page)
	}
	if limit != 20 {
		t.Errorf("limit = %d, want 20 (default for invalid)", limit)
	}
}

// ---------------------------------------------------------------------------
// Service validation tests — Submit rejects empty source_url (BUG-003).
// ---------------------------------------------------------------------------

// TestSubmit_RejectsEmptySourceURL verifies the service rejects submissions
// without a source_url (ADR-094 requirement).
func TestSubmit_RejectsEmptySourceURL(t *testing.T) {
	svc := NewService(nil, &config.Settings{})
	_, err := svc.Submit(nil, pgtype.UUID{}, pgtype.UUID{}, SubmitEntryInput{
		Title:       "Test Event",
		Category:    "visa_window",
		SourceURL:   "",
	})
	if err == nil {
		t.Error("Submit with empty source_url should return error")
	}
}

// TestSubmit_RejectsEmptyTitle verifies the service rejects submissions
// without a title.
func TestSubmit_RejectsEmptyTitle(t *testing.T) {
	svc := NewService(nil, &config.Settings{})
	_, err := svc.Submit(nil, pgtype.UUID{}, pgtype.UUID{}, SubmitEntryInput{
		Title:    "",
		Category: "visa_window",
		SourceURL: "https://example.com",
	})
	if err == nil {
		t.Error("Submit with empty title should return error")
	}
}

// TestSubmit_RejectsEmptyCategory verifies the service rejects submissions
// without a category.
func TestSubmit_RejectsEmptyCategory(t *testing.T) {
	svc := NewService(nil, &config.Settings{})
	_, err := svc.Submit(nil, pgtype.UUID{}, pgtype.UUID{}, SubmitEntryInput{
		Title:    "Test Event",
		Category: "",
		SourceURL: "https://example.com",
	})
	if err == nil {
		t.Error("Submit with empty category should return error")
	}
}

// TestSubmit_RejectsJavaScriptScheme verifies the service rejects
// source_url with a javascript: scheme (BUG-011 XSS prevention).
func TestSubmit_RejectsJavaScriptScheme(t *testing.T) {
	svc := NewService(nil, &config.Settings{})
	_, err := svc.Submit(nil, pgtype.UUID{}, pgtype.UUID{}, SubmitEntryInput{
		Title:     "Test Event",
		Category:  "visa_window",
		SourceURL: "javascript:alert(1)",
	})
	if err == nil {
		t.Error("Submit with javascript: source_url should return error")
	}
}

// TestSubmit_RejectsDataScheme verifies the service rejects source_url
// with a data: scheme (BUG-011 XSS prevention).
func TestSubmit_RejectsDataScheme(t *testing.T) {
	svc := NewService(nil, &config.Settings{})
	_, err := svc.Submit(nil, pgtype.UUID{}, pgtype.UUID{}, SubmitEntryInput{
		Title:     "Test Event",
		Category:  "visa_window",
		SourceURL: "data:text/html,<script>alert(1)</script>",
	})
	if err == nil {
		t.Error("Submit with data: source_url should return error")
	}
}

// TestSubmit_AcceptsHTTPSScheme verifies the service accepts a valid
// https source_url (BUG-011 positive case).
func TestSubmit_AcceptsHTTPSScheme(t *testing.T) {
	svc := NewService(nil, &config.Settings{})
	// Submit will fail at the store layer (nil store), but it should pass
	// the source_url validation and fail later — we check the error is NOT
	// the source_url validation error.
	_, err := svc.Submit(nil, pgtype.UUID{}, pgtype.UUID{}, SubmitEntryInput{
		Title:     "Test Event",
		Category:  "visa_window",
		SourceURL: "https://example.com/official-source",
	})
	if err != nil && err.Error() == "source_url must be a valid http or https URL" {
		t.Error("Submit with valid https source_url should not fail scheme validation")
	}
}
