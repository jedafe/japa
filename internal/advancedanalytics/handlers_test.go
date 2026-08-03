// Package advancedanalytics implements the Advanced Analytics Dashboard
// (Section 66, ADR-084).
//
// Ref: japaconnect-engineering/sections/66-advanced-analytics.md
package advancedanalytics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleTopContent_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/top-content", nil)
	rr := httptest.NewRecorder()
	h.HandleTopContent(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleRevenueStreams_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/revenue-streams", nil)
	rr := httptest.NewRecorder()
	h.HandleRevenueStreams(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleAdPerformance_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/ads", nil)
	rr := httptest.NewRecorder()
	h.HandleAdPerformance(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleNewsletterEngagement_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/newsletter", nil)
	rr := httptest.NewRecorder()
	h.HandleNewsletterEngagement(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleRetention_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/retention", nil)
	rr := httptest.NewRecorder()
	h.HandleRetention(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleExport_NoAuth_401(t *testing.T) {
	h := NewHandlers(&Service{})
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/export?metric=top_content", nil)
	rr := httptest.NewRecorder()
	h.HandleExport(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
