// Package reviews — HTTP handlers for the Universal Ratings & Reviews system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package reviews

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for review HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "already_exists"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Review Handlers
// ---------------------------------------------------------------------------

// CreateReview handles POST /api/v1/reviews.
func (h *Handlers) CreateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// entity_type/entity_id come from the URL path (catalog: POST
	// /reviews/:entity_type/:entity_id); the body carries only the review fields.
	entityType := chi.URLParam(r, "entity_type")
	entityIDStr := chi.URLParam(r, "entity_id")

	var input struct {
		Rating int16  `json:"rating"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if entityType == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_type", "entity_type is required")
		return
	}

	if entityIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing_entity_id", "entity_id is required")
		return
	}

	if input.Rating < RatingMin || input.Rating > RatingMax {
		writeError(w, http.StatusBadRequest, "invalid_rating", "Rating must be between 1 and 5")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
		return
	}

	review, err := h.Service.CreateReview(r.Context(), tenantID, user.UserID, CreateReviewInput{
		EntityType: entityType,
		EntityID:   entityID,
		Rating:     input.Rating,
		Title:      input.Title,
		Body:       input.Body,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": review})
}

// ListReviews handles GET /api/v1/reviews.
// Supports ?entity_type=, ?entity_id=, ?reviewer_id=, ?page_size=, ?page_offset=.
func (h *Handlers) ListReviews(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	// Entity-scoped listing (catalog: GET /reviews/:entity_type/:entity_id)
	// takes URL params; bare GET /reviews falls back to ?reviewer_id= (kept
	// as a documented extra — the catalog only specifies entity-scoped list).
	entityType := chi.URLParam(r, "entity_type")
	entityIDStr := chi.URLParam(r, "entity_id")
	reviewerIDStr := r.URL.Query().Get("reviewer_id")

	// List by entity.
	if entityType != "" && entityIDStr != "" {
		var entityID pgtype.UUID
		if err := entityID.Scan(entityIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
			return
		}

		reviews, err := h.Service.ListReviewsByEntity(r.Context(), tenantID, entityType, entityID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": reviews})

		return
	}

	// List by reviewer.
	if reviewerIDStr != "" {
		var reviewerID pgtype.UUID
		if err := reviewerID.Scan(reviewerIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_reviewer_id", "Invalid reviewer_id")
			return
		}

		reviews, err := h.Service.ListReviewsByReviewer(r.Context(), tenantID, reviewerID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": reviews})

		return
	}

	writeError(w, http.StatusBadRequest, "missing_params", "entity_type+entity_id or reviewer_id required")
}

// GetReview handles GET /api/v1/reviews/{id}.
func (h *Handlers) GetReview(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	review, err := h.Service.GetReview(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": review})
}

// UpdateReview handles PATCH /api/v1/reviews/{id}.
func (h *Handlers) UpdateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	var input struct {
		Rating int16  `json:"rating"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Rating < RatingMin || input.Rating > RatingMax {
		writeError(w, http.StatusBadRequest, "invalid_rating", "Rating must be between 1 and 5")
		return
	}

	review, err := h.Service.UpdateReview(r.Context(), id, tenantID, user.UserID, input.Rating, input.Title, input.Body)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": review})
}

// DeleteReview handles DELETE /api/v1/reviews/{id}.
func (h *Handlers) DeleteReview(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	if err := h.Service.DeleteReview(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// FlagReview handles POST /api/v1/reviews/{id}/flag.
func (h *Handlers) FlagReview(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	if err := h.Service.FlagReview(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "flagged"}})
}

// GetAggregate handles GET /api/v1/reviews/aggregate.
// Supports ?entity_type= and ?entity_id=.
func (h *Handlers) GetAggregate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// catalog: GET /reviews/:entity_type/:entity_id/aggregate
	entityType := chi.URLParam(r, "entity_type")
	entityIDStr := chi.URLParam(r, "entity_id")

	if entityType == "" || entityIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "entity_type and entity_id required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
		return
	}

	aggregate, err := h.Service.GetAggregate(r.Context(), tenantID, entityType, entityID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": aggregate})
}

// ModerateReview handles PATCH /api/v1/admin/reviews/:id.
// Changes a review's status (admin moderation).
func (h *Handlers) ModerateReview(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	var input struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Status == "" {
		writeError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	review, err := h.Service.ModerateReview(r.Context(), id, tenantID, input.Status)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": review})
}
