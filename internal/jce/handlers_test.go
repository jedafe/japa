package jce

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func testPlatformAndUser(t *testing.T) (pgtype.UUID, pgtype.UUID) {
	t.Helper()

	var userID pgtype.UUID
	if err := userID.Scan(uuid.New().String()); err != nil {
		t.Fatalf("userID: %v", err)
	}

	return pgtype.UUID{}, userID
}

func newRequest(t *testing.T, method, path, body string, userID pgtype.UUID) *http.Request {
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
	}{ID: pgtype.UUID{}})
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

	_, userID := testPlatformAndUser(t)
	r := newRequest(t, method, path, body, userID)
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

// ---------------------------------------------------------------------------
// Route happy-path and validation tests
// ---------------------------------------------------------------------------

// fakeService is a service stub that lets us exercise handlers through the full
// Chi router without needing a real database. Methods that are not overridden
// are promoted from the embedded *Service and should not be called.
type fakeService struct {
	*Service
	ds          DataSource
	dataSources []DataSource
	freshness   FreshnessSummary
	job         IngestionJob
	jobs        []IngestionJob
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

func newAuthenticatedRequest(t *testing.T, method, path, body string, userID pgtype.UUID) *http.Request {
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
	}{ID: pgtype.UUID{}})
	ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AuthenticatedUser{
		UserID: userID,
	})

	return r.WithContext(ctx)
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

func TestHandlers_ListDataSources_Happy(t *testing.T) {
	_, userID := testPlatformAndUser(t)
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

	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/data-sources", "", userID)
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
	_, userID := testPlatformAndUser(t)
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/data-sources", h.HandleCreateDataSource)

	body := `{"source_name":"Test","source_kind":"invalid","trust_level":3}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/data-sources", body, userID)
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
	_, userID := testPlatformAndUser(t)
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/data-sources", h.HandleCreateDataSource)

	body := `{"source_name":"Test","source_kind":"official","trust_level":3,"entity_id":"not-a-uuid"}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/data-sources", body, userID)
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
	_, userID := testPlatformAndUser(t)
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Get("/jce/data-sources/{id}", h.HandleGetDataSource)
	r.Patch("/jce/data-sources/{id}", h.HandleUpdateDataSource)
	r.Delete("/jce/data-sources/{id}", h.HandleDeleteDataSource)

	dsID := uuid.New().String()

	// Get
	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/data-sources/"+dsID, "", userID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for get, got %d: %s", rec.Code, rec.Body.String())
	}

	// Update
	body := `{"source_name":"Updated","source_kind":"third_party","trust_level":4}`
	req = newAuthenticatedRequest(t, http.MethodPatch, "/jce/data-sources/"+dsID, body, userID)
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
	req = newAuthenticatedRequest(t, http.MethodDelete, "/jce/data-sources/"+dsID, "", userID)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for delete, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlers_GetFreshness_Happy(t *testing.T) {
	_, userID := testPlatformAndUser(t)
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

	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/comparisons/"+contentID+"/freshness", "", userID)
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
	_, userID := testPlatformAndUser(t)
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
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/comparisons/"+contentID+"/refresh", body, userID)
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
	_, userID := testPlatformAndUser(t)
	contentID := uuid.New().String()
	fs := &fakeService{Service: &Service{features: stubFeatureChecker{enabled: true}}}
	h := NewHandlers(fs)

	r := chi.NewRouter()
	r.Post("/jce/comparisons/{id}/refresh", h.HandleRefreshComparison)

	body := `{"source_kind":"bad"}`
	req := newAuthenticatedRequest(t, http.MethodPost, "/jce/comparisons/"+contentID+"/refresh", body, userID)
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
	_, userID := testPlatformAndUser(t)
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

	req := newAuthenticatedRequest(t, http.MethodGet, "/jce/comparisons/"+contentID+"/ingestion-jobs", "", userID)
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
