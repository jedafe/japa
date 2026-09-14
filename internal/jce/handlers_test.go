package jce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/features"
)

// stubFeatureChecker is a feature checker that always returns a fixed value.
type stubFeatureChecker struct {
	enabled bool
}

func (s stubFeatureChecker) IsEnabled(ctx context.Context, key string) bool {
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
	ctx = context.WithValue(ctx, struct{}{}, &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{ID: tenantID})
	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: userID,
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
		path = "/jce/" + uuid.New().String()
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
	profiles      []ComparisonProfile
	score         ScoreResult
	recs          []ComparisonRecommendation
	ds            DataSource
	dataSources   []DataSource
	freshness     FreshnessSummary
	job           IngestionJob
	jobs          []IngestionJob
	advisorResult AdvisorResult
}

func (f *fakeService) ListComparisonProfiles(ctx context.Context, userID pgtype.UUID, includeTemplates, all bool) ([]ComparisonProfile, error) {
	return append([]ComparisonProfile(nil), f.profiles...), nil
}

func (f *fakeService) GetComparisonScore(ctx context.Context, contentID, userID pgtype.UUID, profileID *pgtype.UUID) (ScoreResult, error) {
	return f.score, nil
}

func (f *fakeService) ListRecommendations(ctx context.Context, contentID, userID pgtype.UUID, profileID *pgtype.UUID, limit int) ([]ComparisonRecommendation, error) {
	return append([]ComparisonRecommendation(nil), f.recs...), nil
}

func (f *fakeService) ListDataSources(ctx context.Context, entityType *string, entityID *pgtype.UUID, page, pageSize int) ([]DataSource, int64, error) {
	return append([]DataSource(nil), f.dataSources...), int64(len(f.dataSources)), nil
}

func (f *fakeService) CreateDataSource(ctx context.Context, in DataSourceInput) (DataSource, error) {
	return DataSource{
		ID: uuid.New().String(),

		EntityType:     in.EntityType,
		EntityID:       in.EntityID,
		DimensionSlug:  in.DimensionSlug,
		SourceName:     in.SourceName,
		SourceURL:      in.SourceURL,
		SourceKind:     in.SourceKind,
		TrustLevel:     in.TrustLevel,
		Config:         in.Config,
		FreshnessScore: 0,
	}, nil
}

func (f *fakeService) GetDataSource(ctx context.Context, id pgtype.UUID) (DataSource, error) {
	return f.ds, nil
}

func (f *fakeService) UpdateDataSource(ctx context.Context, id pgtype.UUID, in DataSourceInput) (DataSource, error) {
	return DataSource{
		ID: id.String(),

		EntityType:     in.EntityType,
		EntityID:       in.EntityID,
		DimensionSlug:  in.DimensionSlug,
		SourceName:     in.SourceName,
		SourceURL:      in.SourceURL,
		SourceKind:     in.SourceKind,
		TrustLevel:     in.TrustLevel,
		Config:         in.Config,
		FreshnessScore: 0,
	}, nil
}

func (f *fakeService) DeleteDataSource(ctx context.Context, id pgtype.UUID) error {
	return nil
}

func (f *fakeService) FreshnessForComparison(ctx context.Context, contentID, userID pgtype.UUID) (FreshnessSummary, error) {
	f.freshness.ContentID = contentID.String()
	return f.freshness, nil
}

func (f *fakeService) RefreshComparison(ctx context.Context, contentID, userID pgtype.UUID, sourceKind *string) (IngestionJob, error) {
	f.job.ContentID = contentID.String()
	f.job.SourceKind = sourceKind

	return f.job, nil
}

func (f *fakeService) ListIngestionJobs(ctx context.Context, contentID pgtype.UUID, limit, offset int) ([]IngestionJob, error) {
	return append([]IngestionJob(nil), f.jobs...), nil
}

func (f *fakeService) Analyze(ctx context.Context, contentID, userID, profileID pgtype.UUID, question string) (AdvisorResult, error) {
	f.advisorResult.ContentID = contentID.String()
	return f.advisorResult, nil
}

func (f *fakeService) Rank(ctx context.Context, contentID, userID, profileID pgtype.UUID) (AdvisorResult, error) {
	f.advisorResult.ContentID = contentID.String()
	return f.advisorResult, nil
}

func (f *fakeService) Explain(ctx context.Context, contentID, itemID, userID, profileID pgtype.UUID, factorKey, question string) (AdvisorResult, error) {
	f.advisorResult.ContentID = contentID.String()
	return f.advisorResult, nil
}

func (f *fakeService) Recommend(ctx context.Context, contentID, userID, profileID pgtype.UUID) (AdvisorResult, error) {
	f.advisorResult.ContentID = contentID.String()
	return f.advisorResult, nil
}

func (f *fakeService) StreamAdvisor(ctx context.Context, contentID, userID, profileID pgtype.UUID, question string, w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	payload, _ := json.Marshal(map[string]any{
		"type":    "message",
		"content": f.advisorResult.Summary,
	})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)

	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}

	return nil
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
	ctx = context.WithValue(ctx, struct{}{}, &struct {
		ID   pgtype.UUID
		Name string
		Slug string
	}{ID: tenantID})
	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: userID,
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

// ---------------------------------------------------------------------------
// Sub-spec 4 handler tests
// ---------------------------------------------------------------------------

func TestHandlers_FeatureDisabled_DataSources(t *testing.T) {
	h := newHandlersWithFeatureEnabled(false)

	assertRoute404(t, h.HandleListDataSources, http.MethodGet, "/jce/data-sources", "")
	assertRoute404(t, h.HandleCreateDataSource, http.MethodPost, "/jce/data-sources", `{"source_name":"Test","source_kind":"official","trust_level":3}`)
	assertRoute404(t, h.HandleGetDataSource, http.MethodGet, "/jce/data-sources/{id}", "")
	assertRoute404(t, h.HandleUpdateDataSource, http.MethodPatch, "/jce/data-sources/{id}", `{"source_name":"Test","source_kind":"official","trust_level":3}`)
	assertRoute404(t, h.HandleDeleteDataSource, http.MethodDelete, "/jce/data-sources/{id}", "")
}

func TestHandlers_FeatureDisabled_Advisor(t *testing.T) {
	h := newHandlersWithFeatureEnabled(false)

	assertRoute404(t, h.HandleAnalyze, http.MethodPost, "/jce/comparisons/{id}/analyze", `{"question":"best?"}`)
	assertRoute404(t, h.HandleRank, http.MethodPost, "/jce/comparisons/{id}/rank", `{}`)
	assertRoute404(t, h.HandleExplain, http.MethodPost, "/jce/comparisons/{id}/explain", `{"item_id":"`+uuid.New().String()+`"}`)
	assertRoute404(t, h.HandleRecommend, http.MethodPost, "/jce/comparisons/{id}/recommend", `{}`)
	assertRoute404(t, h.HandleStreamAdvisor, http.MethodPost, "/jce/comparisons/{id}/advisor", `{"question":"best?"}`)
}

func TestHandlers_ListDataSources_Happy(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	fs := &fakeService{
		Service: &Service{features: stubFeatureChecker{enabled: true}},
		dataSources: []DataSource{
			{
				ID: uuid.New().String(),

				SourceName: "Official Source",
				SourceKind: "official",
				TrustLevel: 5,
			},
		},
	}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Get("/jce/data-sources", h.HandleListDataSources)

	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/data-sources", "", tenantID, userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []DataSource `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Data) != 1 || resp.Data[0].SourceName != "Official Source" {
		t.Fatalf("unexpected response: %+v", resp.Data)
	}
}

func TestHandlers_CreateDataSource_InvalidSourceKind(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/data-sources", h.HandleCreateDataSource)

	body := `{"source_name":"Test","source_kind":"invalid","trust_level":3}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/data-sources", body, tenantID, userID)
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

	if resp.Error.Code != "invalid_source_kind" {
		t.Fatalf("expected error code invalid_source_kind, got %q", resp.Error.Code)
	}
}

func TestHandlers_CreateDataSource_InvalidEntityID(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/data-sources", h.HandleCreateDataSource)

	body := `{"source_name":"Test","source_kind":"official","trust_level":3,"entity_id":"not-a-uuid"}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/data-sources", body, tenantID, userID)
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

	if resp.Error.Code != "invalid_entity_id" {
		t.Fatalf("expected error code invalid_entity_id, got %q", resp.Error.Code)
	}
}

func TestHandlers_DataSourceCRUD_Happy(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Get("/jce/data-sources/{id}", h.HandleGetDataSource)
	r.Patch("/jce/data-sources/{id}", h.HandleUpdateDataSource)
	r.Delete("/jce/data-sources/{id}", h.HandleDeleteDataSource)

	dsID := uuid.New().String()

	// Get
	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/data-sources/"+dsID, "", tenantID, userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for get, got %d: %s", rec.Code, rec.Body.String())
	}

	// Update
	body := `{"source_name":"Updated","source_kind":"third_party","trust_level":4}`
	req = newAuthenticatedRequest(t, http.MethodPatch, "/jce/data-sources/"+dsID, body, tenantID, userID)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for update, got %d: %s", rec.Code, rec.Body.String())
	}

	var updateResp struct {
		Data DataSource `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("decoding update response: %v", err)
	}

	if updateResp.Data.SourceName != "Updated" {
		t.Fatalf("expected updated source name, got %q", updateResp.Data.SourceName)
	}

	// Delete
	req = newAuthenticatedRequest(t, http.MethodDelete, "/jce/data-sources/"+dsID, "", tenantID, userID)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for delete, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlers_GetFreshness_Happy(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	contentID := uuid.New().String()
	fs := &fakeService{
		Service: &Service{features: stubFeatureChecker{enabled: true}},
		freshness: FreshnessSummary{
			OverallFreshnessScore: 85,
			StaleCount:            1,
			TotalCount:            5,
			ByItem: []FreshnessItem{
				{ItemID: uuid.New().String(), ItemTitle: "Item A", FreshnessScore: 85},
			},
		},
	}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Get("/jce/comparisons/{id}/freshness", h.HandleGetFreshness)

	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/comparisons/"+contentID+"/freshness", "", tenantID, userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data FreshnessSummary `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Data.ContentID != contentID {
		t.Fatalf("expected content_id %q, got %q", contentID, resp.Data.ContentID)
	}

	if resp.Data.OverallFreshnessScore != 85 {
		t.Fatalf("expected overall freshness 85, got %d", resp.Data.OverallFreshnessScore)
	}
}

func TestHandlers_RefreshComparison_Happy(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	contentID := uuid.New().String()
	fs := &fakeService{
		Service: &Service{features: stubFeatureChecker{enabled: true}},
		job: IngestionJob{
			ID:        uuid.New().String(),
			ContentID: contentID,
			Status:    "pending",
		},
	}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/comparisons/{id}/refresh", h.HandleRefreshComparison)

	body := `{"source_kind":"official"}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/comparisons/"+contentID+"/refresh", body, tenantID, userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data IngestionJob `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Data.ContentID != contentID {
		t.Fatalf("expected content_id %q, got %q", contentID, resp.Data.ContentID)
	}

	if resp.Data.SourceKind == nil || *resp.Data.SourceKind != "official" {
		t.Fatalf("expected source_kind official, got %v", resp.Data.SourceKind)
	}
}

func TestHandlers_RefreshComparison_InvalidSourceKind(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	contentID := uuid.New().String()
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/comparisons/{id}/refresh", h.HandleRefreshComparison)

	body := `{"source_kind":"bad"}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/comparisons/"+contentID+"/refresh", body, tenantID, userID)
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

	if resp.Error.Code != "invalid_source_kind" {
		t.Fatalf("expected error code invalid_source_kind, got %q", resp.Error.Code)
	}
}

func TestHandlers_ListIngestionJobs_Happy(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	contentID := uuid.New().String()
	fs := &fakeService{
		Service: &Service{features: stubFeatureChecker{enabled: true}},
		jobs: []IngestionJob{
			{ID: uuid.New().String(), ContentID: contentID, Status: "completed"},
		},
	}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Get("/jce/comparisons/{id}/ingestion-jobs", h.HandleListIngestionJobs)

	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/comparisons/"+contentID+"/ingestion-jobs", "", tenantID, userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []IngestionJob `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Data) != 1 || resp.Data[0].ContentID != contentID {
		t.Fatalf("unexpected response: %+v", resp.Data)
	}
}

func TestHandlers_Analyze_Happy(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	contentID := uuid.New().String()
	profileID := uuid.New().String()
	fs := &fakeService{
		Service: &Service{features: stubFeatureChecker{enabled: true}},
		advisorResult: AdvisorResult{
			Summary:        "The best option is A.",
			FreshnessScore: 90,
			Ranking: []AdvisorRankItem{
				{ItemID: uuid.New().String(), ItemTitle: "Option A", Rank: 1, JapaScore: 92},
			},
		},
	}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/comparisons/{id}/analyze", h.HandleAnalyze)

	body := `{"profile_id":"` + profileID + `","question":"Which is best?"}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/comparisons/"+contentID+"/analyze", body, tenantID, userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data AdvisorResult `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Data.Summary != "The best option is A." {
		t.Fatalf("unexpected summary: %q", resp.Data.Summary)
	}

	if len(resp.Data.Ranking) != 1 {
		t.Fatalf("expected 1 ranked item, got %d", len(resp.Data.Ranking))
	}
}

func TestHandlers_Explain_InvalidItemID(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	contentID := uuid.New().String()
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/comparisons/{id}/explain", h.HandleExplain)

	body := `{"item_id":"not-a-uuid","factor_key":"cost"}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/comparisons/"+contentID+"/explain", body, tenantID, userID)
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

	if resp.Error.Code != "invalid_item_id" {
		t.Fatalf("expected error code invalid_item_id, got %q", resp.Error.Code)
	}
}

func TestHandlers_StreamAdvisor_Happy(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	contentID := uuid.New().String()
	fs := &fakeService{
		Service: &Service{features: stubFeatureChecker{enabled: true}},
		advisorResult: AdvisorResult{
			Summary:        "The best option is A.",
			FreshnessScore: 90,
		},
	}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/comparisons/{id}/advisor", h.HandleStreamAdvisor)

	body := `{"question":"Which is best for me?"}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/comparisons/"+contentID+"/advisor", body, tenantID, userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", ct)
	}

	bodyStr := rec.Body.String()
	if !strings.Contains(bodyStr, "data:") {
		t.Fatalf("expected SSE data: line, got %q", bodyStr)
	}

	if !strings.Contains(bodyStr, `"content"`) {
		t.Fatalf("expected JSON content in SSE payload, got %q", bodyStr)
	}
}

func TestHandlers_StreamAdvisor_MissingQuestion(t *testing.T) {
	tenantID, userID := testTenantAndUser(t)
	contentID := uuid.New().String()
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/comparisons/{id}/advisor", h.HandleStreamAdvisor)

	body := `{}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/comparisons/"+contentID+"/advisor", body, tenantID, userID)
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

	if resp.Error.Code != "missing_question" {
		t.Fatalf("expected error code missing_question, got %q", resp.Error.Code)
	}
}
