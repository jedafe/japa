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

// ---------------------------------------------------------------------------
// Route happy-path and validation tests
// ---------------------------------------------------------------------------

// fakeService is a service stub that lets us exercise handlers through the full
// Chi router without needing a real database. Methods that are not overridden
// are promoted from the embedded *Service and should not be called.
type fakeService struct {
	*Service
	profiles []ComparisonProfile
	score    ScoreResult
	recs     []ComparisonRecommendation
}

func (f *fakeService) ListComparisonProfiles(ctx context.Context, tenantID, userID pgtype.UUID, includeTemplates, all bool) ([]ComparisonProfile, error) {
	return append([]ComparisonProfile(nil), f.profiles...), nil
}

func (f *fakeService) GetComparisonScore(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID) (ScoreResult, error) {
	return f.score, nil
}

func (f *fakeService) ListRecommendations(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID, limit int) ([]ComparisonRecommendation, error) {
	return append([]ComparisonRecommendation(nil), f.recs...), nil
}

func newAuthenticatedRequest(t *testing.T, method, path, body string, tenantID, userID pgtype.UUID) *http.Request {
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

	return r.WithContext(ctx)
}

func TestHandlers_ListProfiles_Happy(t *testing.T) {
	profileID := uuid.New().String()
	fs := &fakeService{
		Service: &Service{features: stubFeatureChecker{enabled: true}},
		profiles: []ComparisonProfile{
			{
				ID:     profileID,
				UserID: &profileID,
				Title:  "Test Profile",
			},
		},
	}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Get("/jce/profiles", h.ListProfiles)

	tenantID, userID := testTenantAndUser(t)
	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/profiles", "", tenantID, userID)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []ComparisonProfile `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Title != "Test Profile" {
		t.Fatalf("unexpected response: %+v", resp.Data)
	}
}

func TestHandlers_GetComparisonScore_InvalidProfileID(t *testing.T) {
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Get("/jce/comparisons/{id}/score", h.GetComparisonScore)

	tenantID, userID := testTenantAndUser(t)
	path := fmt.Sprintf("/jce/comparisons/%s/score?profile_id=not-a-uuid", uuid.New().String())
	req := newAuthenticatedRequest(t, http.MethodGet, path, "", tenantID, userID)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Error.Code != "invalid_profile_id" {
		t.Fatalf("expected error code invalid_profile_id, got %q", resp.Error.Code)
	}
}

func TestHandlers_ListRecommendations_InvalidProfileID(t *testing.T) {
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Get("/jce/comparisons/{id}/recommendations", h.ListRecommendations)

	tenantID, userID := testTenantAndUser(t)
	path := fmt.Sprintf("/jce/comparisons/%s/recommendations?profile_id=malformed-uuid", uuid.New().String())
	req := newAuthenticatedRequest(t, http.MethodGet, path, "", tenantID, userID)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Error.Code != "invalid_profile_id" {
		t.Fatalf("expected error code invalid_profile_id, got %q", resp.Error.Code)
	}
}

func TestHandlers_ScoreComparison_InvalidProfileID(t *testing.T) {
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/comparisons/{id}/score", h.ScoreComparison)

	tenantID, userID := testTenantAndUser(t)
	path := fmt.Sprintf("/jce/comparisons/%s/score", uuid.New().String())
	body := `{"profile_id":"not-a-uuid","force":true}`
	req := newAuthenticatedRequest(t, http.MethodPost, path, body, tenantID, userID)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Error.Code != "invalid_profile_id" {
		t.Fatalf("expected error code invalid_profile_id, got %q", resp.Error.Code)
	}
}
