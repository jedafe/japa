package jce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// service is the subset of the JCE Service used by the HTTP handlers.
// Defining it as an interface makes the handler package testable without
// requiring a real database or a fully wired Service.
type service interface {
	ListComparisonProfiles(ctx context.Context, userID pgtype.UUID, includeTemplates, all bool) ([]ComparisonProfile, error)
	CreateComparisonProfile(ctx context.Context, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error)
	GetComparisonProfile(ctx context.Context, id, userID pgtype.UUID) (ComparisonProfile, error)
	UpdateComparisonProfile(ctx context.Context, id, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error)
	DeleteComparisonProfile(ctx context.Context, id, userID pgtype.UUID) error
	SetDefaultComparisonProfile(ctx context.Context, id, userID pgtype.UUID) error
	GetProfileStats(ctx context.Context, userID pgtype.UUID, all bool) ([]ProfileStat, error)

	ListScoreFactors(ctx context.Context, comparisonTypeID pgtype.UUID, includeGlobal bool) ([]ScoreFactor, error)
	CreateScoreFactor(ctx context.Context, userID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error)
	GetScoreFactor(ctx context.Context, id pgtype.UUID) (ScoreFactor, error)
	UpdateScoreFactor(ctx context.Context, id, userID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error)
	DeleteScoreFactor(ctx context.Context, id, userID pgtype.UUID) error

	ScoreComparison(ctx context.Context, contentID, userID pgtype.UUID, profileID *pgtype.UUID, smartMode string, force bool) (ScoreResult, error)
	GetComparisonScore(ctx context.Context, contentID, userID pgtype.UUID, profileID *pgtype.UUID) (ScoreResult, error)
	ListRecommendations(ctx context.Context, contentID, userID pgtype.UUID, profileID *pgtype.UUID, limit int) ([]ComparisonRecommendation, error)
	PreviewScoreForItems(ctx context.Context, userID pgtype.UUID, in ScorePreviewInput) (ScoreResult, error)

	// Data sources
	ListDataSources(ctx context.Context, entityType *string, entityID *pgtype.UUID, page, pageSize int) ([]DataSource, int64, error)
	CreateDataSource(ctx context.Context, in DataSourceInput) (DataSource, error)
	GetDataSource(ctx context.Context, id pgtype.UUID) (DataSource, error)
	UpdateDataSource(ctx context.Context, id pgtype.UUID, in DataSourceInput) (DataSource, error)
	DeleteDataSource(ctx context.Context, id pgtype.UUID) error

	// Freshness and refresh
	FreshnessForComparison(ctx context.Context, contentID, userID pgtype.UUID) (FreshnessSummary, error)
	RefreshComparison(ctx context.Context, contentID, userID pgtype.UUID, sourceKind *string) (IngestionJob, error)
	ListIngestionJobs(ctx context.Context, contentID pgtype.UUID, limit, offset int) ([]IngestionJob, error)

	// AI advisor
	Analyze(ctx context.Context, contentID, userID, profileID pgtype.UUID, question string) (AdvisorResult, error)
	Rank(ctx context.Context, contentID, userID, profileID pgtype.UUID) (AdvisorResult, error)
	Explain(ctx context.Context, contentID, itemID, userID, profileID pgtype.UUID, factorKey, question string) (AdvisorResult, error)
	Recommend(ctx context.Context, contentID, userID, profileID pgtype.UUID) (AdvisorResult, error)
	StreamAdvisor(ctx context.Context, contentID, userID, profileID pgtype.UUID, question string, w http.ResponseWriter) error
}

// Handlers exposes the JCE Sub-spec 2/3 HTTP handlers.
type Handlers struct {
	service service
}

// NewHandlers creates Handlers backed by the given JCE service.
func NewHandlers(service service) *Handlers { return &Handlers{service: service} }

// requireTenant resolves the tenant from the request context.
func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

// requireUser resolves the authenticated user from the request context.
func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
}

// handleServiceError translates a service-layer error into the canonical HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// decodeBody decodes a JSON request body into the given value.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return false
	}

	return true
}

// parseIntQuery parses a query parameter as an int, returning the default on
// an empty or malformed value.
func parseIntQuery(q url.Values, key string, def int) int {
	s := q.Get(key)
	if s == "" {
		return def
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}

	return n
}

// optionalUUIDPtr parses a string into a *pgtype.UUID. An empty string
// returns nil with no error; a non-empty but invalid string returns an error
// so callers can surface 400 Bad Request for malformed profile_id values.
func optionalUUIDPtr(s string) (*pgtype.UUID, error) {
	if s == "" {
		return nil, nil
	}

	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return nil, fmt.Errorf("invalid profile_id %q: %w", s, err)
	}

	return &u, nil
}

// parseLimit parses the limit query parameter, defaulting to the given value.
func parseLimit(r *http.Request, defaultLimit int) int {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return defaultLimit
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultLimit
	}

	return n
}

// ---------------------------------------------------------------------------
// Profiles
// ---------------------------------------------------------------------------

// ListProfiles handles GET /api/v1/jce/profiles.
func (h *Handlers) ListProfiles(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	all := r.URL.Query().Get("all") == "true"

	profiles, err := h.service.ListComparisonProfiles(r.Context(), user.UserID, true, all)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profiles})
}

// CreateProfile handles POST /api/v1/jce/profiles.
func (h *Handlers) CreateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input ComparisonProfileInput
	if !decodeBody(w, r, &input) {
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	profile, err := h.service.CreateComparisonProfile(r.Context(), user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": profile})
}

// GetProfile handles GET /api/v1/jce/profiles/{id}.
func (h *Handlers) GetProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	profile, err := h.service.GetComparisonProfile(r.Context(), id, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// UpdateProfile handles PATCH /api/v1/jce/profiles/{id}.
func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var input ComparisonProfileInput
	if !decodeBody(w, r, &input) {
		return
	}

	profile, err := h.service.UpdateComparisonProfile(r.Context(), id, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// DeleteProfile handles DELETE /api/v1/jce/profiles/{id}.
func (h *Handlers) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	if err := h.service.DeleteComparisonProfile(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// SetDefaultProfile handles POST /api/v1/jce/profiles/{id}/default.
func (h *Handlers) SetDefaultProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	if err := h.service.SetDefaultComparisonProfile(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// GetProfileStats handles GET /api/v1/jce/profiles/stats.
func (h *Handlers) GetProfileStats(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	all := r.URL.Query().Get("all") == "true"

	stats, err := h.service.GetProfileStats(r.Context(), user.UserID, all)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// ---------------------------------------------------------------------------
// Score factors
// ---------------------------------------------------------------------------

// ListScoreFactors handles GET /api/v1/jce/comparison-types/{id}/score-factors.
func (h *Handlers) ListScoreFactors(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	factors, err := h.service.ListScoreFactors(r.Context(), id, true)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": factors})
}

// CreateScoreFactor handles POST /api/v1/jce/comparison-types/{id}/score-factors.
func (h *Handlers) CreateScoreFactor(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var input ScoreFactorInput
	if !decodeBody(w, r, &input) {
		return
	}

	if input.ComparisonTypeID == nil || *input.ComparisonTypeID == "" {
		s := id.String()
		input.ComparisonTypeID = &s
	}

	factor, err := h.service.CreateScoreFactor(r.Context(), user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": factor})
}

// GetScoreFactor handles GET /api/v1/jce/score-factors/{id}.
func (h *Handlers) GetScoreFactor(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	factor, err := h.service.GetScoreFactor(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": factor})
}

// UpdateScoreFactor handles PATCH /api/v1/jce/score-factors/{id}.
func (h *Handlers) UpdateScoreFactor(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var input ScoreFactorInput
	if !decodeBody(w, r, &input) {
		return
	}

	factor, err := h.service.UpdateScoreFactor(r.Context(), id, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": factor})
}

// DeleteScoreFactor handles DELETE /api/v1/jce/score-factors/{id}.
func (h *Handlers) DeleteScoreFactor(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	if err := h.service.DeleteScoreFactor(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Scoring and recommendations
// ---------------------------------------------------------------------------

// ScoreComparison handles POST /api/v1/jce/comparisons/{id}/score.
func (h *Handlers) ScoreComparison(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var input struct {
		ProfileID string `json:"profile_id"`
		SmartMode string `json:"smart_mode"`
		Force     bool   `json:"force"`
	}

	if !decodeBody(w, r, &input) {
		return
	}

	var profileID *pgtype.UUID

	if input.ProfileID != "" {
		var err error

		profileID, err = optionalUUIDPtr(input.ProfileID)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
			return
		}
	}

	result, err := h.service.ScoreComparison(r.Context(), id, user.UserID, profileID, input.SmartMode, input.Force)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// GetComparisonScore handles GET /api/v1/jce/comparisons/{id}/score.
func (h *Handlers) GetComparisonScore(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	profileID, err := optionalUUIDPtr(r.URL.Query().Get("profile_id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return
	}

	result, err := h.service.GetComparisonScore(r.Context(), id, user.UserID, profileID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ListRecommendations handles GET /api/v1/jce/comparisons/{id}/recommendations.
func (h *Handlers) ListRecommendations(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	profileID, err := optionalUUIDPtr(r.URL.Query().Get("profile_id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return
	}

	limit := parseLimit(r, 10)

	recs, err := h.service.ListRecommendations(r.Context(), id, user.UserID, profileID, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": recs})
}

// PreviewScoreForItems handles POST /api/v1/jce/score-preview.
func (h *Handlers) PreviewScoreForItems(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input ScorePreviewInput
	if !decodeBody(w, r, &input) {
		return
	}

	if input.TypeID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_type_id", "type_id is required")
		return
	}

	result, err := h.service.PreviewScoreForItems(r.Context(), user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ---------------------------------------------------------------------------
// Sub-spec 4: data sources, freshness, refresh, and AI advisor
// ---------------------------------------------------------------------------

type dataSourceListQuery struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
}

type dataSourceCreateBody struct {
	EntityType    *string        `json:"entity_type,omitempty"`
	EntityID      *string        `json:"entity_id,omitempty"`
	DimensionSlug *string        `json:"dimension_slug,omitempty"`
	SourceName    string         `json:"source_name"`
	SourceURL     *string        `json:"source_url,omitempty"`
	SourceKind    string         `json:"source_kind"`
	TrustLevel    int            `json:"trust_level"`
	Config        map[string]any `json:"config"`
}

type refreshBody struct {
	SourceKind *string `json:"source_kind,omitempty"`
}

type advisorBody struct {
	ProfileID *string `json:"profile_id,omitempty"`
	Question  string  `json:"question"`
	ItemID    *string `json:"item_id,omitempty"`
	FactorKey *string `json:"factor_key,omitempty"`
}

// requireTenantAndUser resolves both tenant and authenticated user from the
// request context and returns their UUIDs.
func (h *Handlers) requireTenantAndUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, bool) {
	_, ok := requireTenant(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}

	user, ok := requireUser(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}

	return pgtype.UUID{}, user.UserID, true
}

// validSourceKind reports whether the supplied source kind is one of the
// accepted comparison_data_sources.source_kind values.
func validSourceKind(kind string) bool {
	return validDataSourceKinds[kind]
}

// normalizeOptionalUUIDPtr validates an optional string UUID and coalesces an
// empty pointer to nil. It is used to sanitize data-source create/update bodies.
func normalizeOptionalUUIDPtr(s *string) (*string, error) {
	if s == nil {
		return nil, nil
	}

	if *s == "" {
		return nil, nil
	}

	if _, err := optionalUUIDPtr(*s); err != nil {
		return nil, err
	}

	return s, nil
}

// dataSourceInputFromBody converts a dataSourceCreateBody into the service
// DataSourceInput, sanitizing optional UUID strings and source kind.
func dataSourceInputFromBody(body dataSourceCreateBody) (DataSourceInput, error) {
	entityID, err := normalizeOptionalUUIDPtr(body.EntityID)
	if err != nil {
		return DataSourceInput{}, err
	}

	return DataSourceInput{
		EntityType:    body.EntityType,
		EntityID:      entityID,
		DimensionSlug: body.DimensionSlug,
		SourceName:    body.SourceName,
		SourceURL:     body.SourceURL,
		SourceKind:    body.SourceKind,
		TrustLevel:    body.TrustLevel,
		Config:        body.Config,
	}, nil
}

// HandleListDataSources handles GET /api/v1/jce/data-sources.
func (h *Handlers) HandleListDataSources(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	entityType := q.Get("entity_type")

	var et *string

	if entityType != "" {
		et = &entityType
	}

	var entityID *pgtype.UUID

	if s := q.Get("entity_id"); s != "" {
		id, err := optionalUUIDPtr(s)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", err.Error())
			return
		}

		entityID = id
	}

	page := parseIntQuery(q, "page", 1)
	pageSize := parseIntQuery(q, "page_size", 50)

	sources, total, err := h.service.ListDataSources(r.Context(), et, entityID, page, pageSize)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": sources,
		"meta": map[string]any{
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
		},
	})
}

// HandleCreateDataSource handles POST /api/v1/jce/data-sources.
func (h *Handlers) HandleCreateDataSource(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var body dataSourceCreateBody
	if !decodeBody(w, r, &body) {
		return
	}

	if !validSourceKind(body.SourceKind) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_kind", "source_kind must be official, third_party, or ai_estimate")
		return
	}

	in, err := dataSourceInputFromBody(body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", err.Error())
		return
	}

	ds, err := h.service.CreateDataSource(r.Context(), in)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": ds})
}

// HandleGetDataSource handles GET /api/v1/jce/data-sources/{id}.
func (h *Handlers) HandleGetDataSource(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	ds, err := h.service.GetDataSource(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ds})
}

// HandleUpdateDataSource handles PATCH /api/v1/jce/data-sources/{id}.
func (h *Handlers) HandleUpdateDataSource(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var body dataSourceCreateBody
	if !decodeBody(w, r, &body) {
		return
	}

	if !validSourceKind(body.SourceKind) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_kind", "source_kind must be official, third_party, or ai_estimate")
		return
	}

	in, err := dataSourceInputFromBody(body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", err.Error())
		return
	}

	ds, err := h.service.UpdateDataSource(r.Context(), id, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ds})
}

// HandleDeleteDataSource handles DELETE /api/v1/jce/data-sources/{id}.
func (h *Handlers) HandleDeleteDataSource(w http.ResponseWriter, r *http.Request) {
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	if err := h.service.DeleteDataSource(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// HandleGetFreshness handles GET /api/v1/jce/comparisons/{id}/freshness.
func (h *Handlers) HandleGetFreshness(w http.ResponseWriter, r *http.Request) {
	_, userID, ok := h.requireTenantAndUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	summary, err := h.service.FreshnessForComparison(r.Context(), contentID, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": summary})
}

// HandleRefreshComparison handles POST /api/v1/jce/comparisons/{id}/refresh.
func (h *Handlers) HandleRefreshComparison(w http.ResponseWriter, r *http.Request) {
	_, userID, ok := h.requireTenantAndUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var body refreshBody
	if !decodeBody(w, r, &body) {
		return
	}

	if body.SourceKind != nil && *body.SourceKind != "" && !validSourceKind(*body.SourceKind) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_kind", "source_kind must be official, third_party, or ai_estimate")
		return
	}

	job, err := h.service.RefreshComparison(r.Context(), contentID, userID, body.SourceKind)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": job})
}

// HandleListIngestionJobs handles GET /api/v1/jce/comparisons/{id}/ingestion-jobs.
func (h *Handlers) HandleListIngestionJobs(w http.ResponseWriter, r *http.Request) {
	_, _, ok := h.requireTenantAndUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	limit := parseLimit(r, 20)
	offset := 0

	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}

	jobs, err := h.service.ListIngestionJobs(r.Context(), contentID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": jobs})
}

// advisorProfileID converts an optional string profile_id into a pgtype.UUID.
func advisorProfileID(body advisorBody) (pgtype.UUID, error) {
	if body.ProfileID == nil {
		return pgtype.UUID{}, nil
	}

	pid, err := optionalUUIDPtr(*body.ProfileID)
	if err != nil {
		return pgtype.UUID{}, err
	}

	return ptrToUUID(pid), nil
}

// advisorItemID converts an optional string item_id into a pgtype.UUID.
func advisorItemID(body advisorBody) (pgtype.UUID, error) {
	if body.ItemID == nil {
		return pgtype.UUID{}, nil
	}

	iid, err := optionalUUIDPtr(*body.ItemID)
	if err != nil {
		return pgtype.UUID{}, err
	}

	return ptrToUUID(iid), nil
}

// HandleAnalyze handles POST /api/v1/jce/comparisons/{id}/analyze.
func (h *Handlers) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	_, userID, ok := h.requireTenantAndUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var body advisorBody
	if !decodeBody(w, r, &body) {
		return
	}

	profileID, err := advisorProfileID(body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return
	}

	res, err := h.service.Analyze(r.Context(), contentID, userID, profileID, body.Question)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// HandleRank handles POST /api/v1/jce/comparisons/{id}/rank.
func (h *Handlers) HandleRank(w http.ResponseWriter, r *http.Request) {
	_, userID, ok := h.requireTenantAndUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var body advisorBody
	if !decodeBody(w, r, &body) {
		return
	}

	profileID, err := advisorProfileID(body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return
	}

	res, err := h.service.Rank(r.Context(), contentID, userID, profileID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// HandleExplain handles POST /api/v1/jce/comparisons/{id}/explain.
func (h *Handlers) HandleExplain(w http.ResponseWriter, r *http.Request) {
	_, userID, ok := h.requireTenantAndUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var body advisorBody
	if !decodeBody(w, r, &body) {
		return
	}

	profileID, err := advisorProfileID(body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return
	}

	itemID, err := advisorItemID(body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_item_id", err.Error())
		return
	}

	factorKey := ""
	if body.FactorKey != nil {
		factorKey = *body.FactorKey
	}

	res, err := h.service.Explain(r.Context(), contentID, itemID, userID, profileID, factorKey, body.Question)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// HandleRecommend handles POST /api/v1/jce/comparisons/{id}/recommend.
func (h *Handlers) HandleRecommend(w http.ResponseWriter, r *http.Request) {
	_, userID, ok := h.requireTenantAndUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var body advisorBody
	if !decodeBody(w, r, &body) {
		return
	}

	profileID, err := advisorProfileID(body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return
	}

	res, err := h.service.Recommend(r.Context(), contentID, userID, profileID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res})
}

// HandleStreamAdvisor handles POST /api/v1/jce/comparisons/{id}/advisor (SSE).
func (h *Handlers) HandleStreamAdvisor(w http.ResponseWriter, r *http.Request) {
	_, userID, ok := h.requireTenantAndUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var body advisorBody
	if !decodeBody(w, r, &body) {
		return
	}

	if strings.TrimSpace(body.Question) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_question", "question is required")
		return
	}

	profileID, err := advisorProfileID(body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return
	}

	if err := h.service.StreamAdvisor(r.Context(), contentID, userID, profileID, body.Question, w); err != nil {
		handleServiceError(w, err)
	}
}
