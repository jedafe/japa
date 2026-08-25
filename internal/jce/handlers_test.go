package jce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/tenant"
)

// stubFeatureChecker is a feature checker that always returns a fixed value.
type stubFeatureChecker struct {
	enabled bool
}

func (s stubFeatureChecker) IsEnabled(ctx context.Context, tenantID pgtype.UUID, key string) bool {
	return s.enabled
}

var _ features.FeatureChecker = stubFeatureChecker{}

func testTenantAndUser(t *testing.T) (pgtype.UUID, pgtype.UUID) {
	t.Helper()
	var tenantID, userID pgtype.UUID
	if err := tenantID.Scan(uuid.New().String()); err != nil {
		t.Fatalf("tenantID: %v", err)
	}
	if err := userID.Scan(uuid.New().String()); err != nil {
		t.Fatalf("userID: %v", err)
	}
	return tenantID, userID
}

func newRequest(t *testing.T, method, path, body string, tenantID, userID pgtype.UUID) *http.Request {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("Content-Type", "application/json")

	ctx := r.Context()
	ctx = context.WithValue(ctx, tenant.TenantContextKey, &tenant.Tenant{ID: tenantID})
	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID:   userID,
		TenantID: tenantID,
	})

	// Set a chi route context so chi.URLParam works inside handlers.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	return r.WithContext(ctx)
}

func newHandlersWithFeatureEnabled(enabled bool) *Handlers {
	svc := &Service{features: stubFeatureChecker{enabled: enabled}}
	return NewHandlers(svc)
}

// assertRoute404 tests that a handler returns a 404 with the standard error
// envelope when the feature flag is disabled.
func assertRoute404(t *testing.T, handler http.HandlerFunc, method, path, body string) {
	t.Helper()
	if path == "" {
		path = fmt.Sprintf("/jce/%s", uuid.New().String())
	}
	if method == http.MethodPost && body == "" {
		body = "{}"
	}

	tenantID, userID := testTenantAndUser(t)
	r := newRequest(t, method, path, body, tenantID, userID)
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("%s %s: expected 404, got %d: %s", method, path, w.Code, w.Body.String())
	}

	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s %s: invalid error JSON: %v", method, path, err)
	}
	if resp.Error.Code == "" {
		t.Fatalf("%s %s: expected error code in response body", method, path)
	}
}

func TestHandlers_FeatureDisabled_Profiles(t *testing.T) {
	h := newHandlersWithFeatureEnabled(false)

	assertRoute404(t, h.ListProfiles, http.MethodGet, "/jce/profiles", "")
	assertRoute404(t, h.CreateProfile, http.MethodPost, "/jce/profiles", `{"title":"Test Profile"}`)
	assertRoute404(t, h.GetProfileStats, http.MethodGet, "/jce/profiles/stats", "")
	assertRoute404(t, h.GetProfile, http.MethodGet, "/jce/profiles/{id}", "")
	assertRoute404(t, h.UpdateProfile, http.MethodPatch, "/jce/profiles/{id}", `{"title":"Updated"}`)
	assertRoute404(t, h.DeleteProfile, http.MethodDelete, "/jce/profiles/{id}", "")
	assertRoute404(t, h.SetDefaultProfile, http.MethodPost, "/jce/profiles/{id}/default", "")
}

func TestHandlers_FeatureDisabled_ScoreFactors(t *testing.T) {
	h := newHandlersWithFeatureEnabled(false)

	assertRoute404(t, h.ListScoreFactors, http.MethodGet, "/jce/comparison-types/{id}/score-factors", "")
	assertRoute404(t, h.CreateScoreFactor, http.MethodPost, "/jce/comparison-types/{id}/score-factors", `{"factor_key":"cost","label":"Cost","data_source":"resolver_metadata"}`)
	assertRoute404(t, h.GetScoreFactor, http.MethodGet, "/jce/score-factors/{id}", "")
	assertRoute404(t, h.UpdateScoreFactor, http.MethodPatch, "/jce/score-factors/{id}", `{"factor_key":"cost","label":"Cost","data_source":"resolver_metadata"}`)
	assertRoute404(t, h.DeleteScoreFactor, http.MethodDelete, "/jce/score-factors/{id}", "")
}

func TestHandlers_FeatureDisabled_ScoringAndRecommendations(t *testing.T) {
	h := newHandlersWithFeatureEnabled(false)

	assertRoute404(t, h.ScoreComparison, http.MethodPost, "/jce/comparisons/{id}/score", `{"profile_id":""}`)
	assertRoute404(t, h.GetComparisonScore, http.MethodGet, "/jce/comparisons/{id}/score", "")
	assertRoute404(t, h.ListRecommendations, http.MethodGet, "/jce/comparisons/{id}/recommendations", "")
}

func TestHandlers_FeatureDisabled_ScorePreview(t *testing.T) {
	h := newHandlersWithFeatureEnabled(false)

	assertRoute404(t, h.PreviewScoreForItems, http.MethodPost, "/jce/score-preview", `{"type_id":"`+uuid.New().String()+`"}`)
}
