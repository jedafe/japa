package jce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// service is the subset of the JCE Service used by the HTTP handlers.
// Defining it as an interface makes the handler package testable without
// requiring a real database or a fully wired Service.
type service interface {
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
}

// Handlers exposes the JCE HTTP handlers (Sub-spec 4: data sources,
// freshness, and refresh).
type Handlers struct {
	service service
}

// NewHandlers creates Handlers backed by the given JCE service.
func NewHandlers(service service) *Handlers { return &Handlers{service: service} }

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
// so callers can surface 400 Bad Request for malformed UUID values.
func optionalUUIDPtr(s string) (*pgtype.UUID, error) {
	if s == "" {
		return nil, nil
	}

	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return nil, fmt.Errorf("invalid uuid %q: %w", s, err)
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
// Sub-spec 4: data sources, freshness, and refresh
// ---------------------------------------------------------------------------

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
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	userID := user.UserID

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
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	userID := user.UserID

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
	_, ok := requireUser(w, r)
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
