package jce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// service is the subset of the JCE Service used by the HTTP handlers.
// Defining it as an interface makes the handler package testable without
// requiring a real database or a fully wired Service.
type service interface {
	ListComparisonProfiles(ctx context.Context, tenantID, userID pgtype.UUID, includeTemplates bool) ([]ComparisonProfile, error)
	CreateComparisonProfile(ctx context.Context, tenantID, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error)
	GetComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) (ComparisonProfile, error)
	UpdateComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error)
	DeleteComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) error
	SetDefaultComparisonProfile(ctx context.Context, id, tenantID, userID pgtype.UUID) error
	GetProfileStats(ctx context.Context, tenantID, userID pgtype.UUID) ([]ProfileStat, error)

	ListScoreFactors(ctx context.Context, tenantID, comparisonTypeID pgtype.UUID, includeGlobal bool) ([]ScoreFactor, error)
	CreateScoreFactor(ctx context.Context, tenantID, userID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error)
	GetScoreFactor(ctx context.Context, id, tenantID pgtype.UUID) (ScoreFactor, error)
	UpdateScoreFactor(ctx context.Context, id, tenantID, userID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error)
	DeleteScoreFactor(ctx context.Context, id, tenantID, userID pgtype.UUID) error

	ScoreComparison(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID, force bool) (ScoreResult, error)
	GetComparisonScore(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID) (ScoreResult, error)
	ListRecommendations(ctx context.Context, contentID, tenantID, userID pgtype.UUID, profileID *pgtype.UUID, limit int) ([]ComparisonRecommendation, error)
	PreviewScoreForItems(ctx context.Context, tenantID, userID pgtype.UUID, in ScorePreviewInput) (ScoreResult, error)
}

// Handlers exposes the JCE Sub-spec 2/3 HTTP handlers.
type Handlers struct {
	service service
}

// NewHandlers creates Handlers backed by the given JCE service.
func NewHandlers(service service) *Handlers { return &Handlers{service: service} }

// requireTenant resolves the tenant from the request context.
func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	profiles, err := h.service.ListComparisonProfiles(r.Context(), tenantID, user.UserID, true)
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

	tenantID, ok := requireTenant(w, r)
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

	profile, err := h.service.CreateComparisonProfile(r.Context(), tenantID, user.UserID, input)
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	profile, err := h.service.GetComparisonProfile(r.Context(), id, tenantID, user.UserID)
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

	tenantID, ok := requireTenant(w, r)
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

	profile, err := h.service.UpdateComparisonProfile(r.Context(), id, tenantID, user.UserID, input)
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	if err := h.service.DeleteComparisonProfile(r.Context(), id, tenantID, user.UserID); err != nil {
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	if err := h.service.SetDefaultComparisonProfile(r.Context(), id, tenantID, user.UserID); err != nil {
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	stats, err := h.service.GetProfileStats(r.Context(), tenantID, user.UserID)
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	factors, err := h.service.ListScoreFactors(r.Context(), tenantID, id, true)
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

	tenantID, ok := requireTenant(w, r)
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

	factor, err := h.service.CreateScoreFactor(r.Context(), tenantID, user.UserID, input)
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	factor, err := h.service.GetScoreFactor(r.Context(), id, tenantID)
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

	tenantID, ok := requireTenant(w, r)
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

	factor, err := h.service.UpdateScoreFactor(r.Context(), id, tenantID, user.UserID, input)
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

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	if err := h.service.DeleteScoreFactor(r.Context(), id, tenantID, user.UserID); err != nil {
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

	tenantID, ok := requireTenant(w, r)
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

	// The force flag is not part of the public request schema until
	// cache-bypass logic is implemented; always pass false for now.
	result, err := h.service.ScoreComparison(r.Context(), id, tenantID, user.UserID, profileID, false)
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

	tenantID, ok := requireTenant(w, r)
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

	result, err := h.service.GetComparisonScore(r.Context(), id, tenantID, user.UserID, profileID)
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

	tenantID, ok := requireTenant(w, r)
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

	recs, err := h.service.ListRecommendations(r.Context(), id, tenantID, user.UserID, profileID, limit)
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

	tenantID, ok := requireTenant(w, r)
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

	result, err := h.service.PreviewScoreForItems(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

