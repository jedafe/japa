// Package reviews — HTTP handlers for the Universal Ratings & Reviews system.
// // Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package reviews

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

const defaultPageSize int32 = 20

// TaskEnqueueClient is the interface for enqueueing background tasks.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handlers holds dependencies for review HTTP handlers.
type Handlers struct {
	Service     *Service
	AsynqClient TaskEnqueueClient
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// enqueueReviewAggregateRecalc enqueues an async review aggregate recalculation
// task. Fire-and-forget: errors are logged but never propagated.
func (h *Handlers) enqueueReviewAggregateRecalc(entityType string, entityID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{

		"entity_type": entityType,
		"entity_id":   entityID.String(),
	})
	if err != nil {
		slog.Warn("reviews: failed to marshal aggregate recalc payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeReviewAggregateRecalc, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeReviewAggregateRecalc)),
	); err != nil {
		slog.Warn("reviews: failed to enqueue aggregate recalc task", "entity_type", entityType, "entity_id", entityID.String(), "error", err)
	}
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


// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
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

	// #nosec G109 // value bounded by MaxPageSize check
	// #nosec G115 // value bounded by caller
	return int32(n)
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
	ok = true
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if entityType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_entity_type", "entity_type is required")
		return
	}

	if entityIDStr == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_entity_id", "entity_id is required")
		return
	}

	if input.Rating < RatingMin || input.Rating > RatingMax {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rating", "Rating must be between 1 and 5")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
		return
	}

	review, err := h.Service.CreateReview(r.Context(), user.UserID, CreateReviewInput{
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

	// Enqueue async review aggregate recalculation — fire-and-forget.
	h.enqueueReviewAggregateRecalc(entityType, entityID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": review})
}

// ListReviews handles GET /api/v1/reviews.
// Supports ?entity_type=, ?entity_id=, ?reviewer_id=, ?page_size=, ?page_offset=.
func (h *Handlers) ListReviews(w http.ResponseWriter, r *http.Request) {

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	// Entity-scoped listing (catalog: GET /reviews/:entity_type/:entity_id)
	// takes URL params; bare GET /reviews falls back to ?reviewer_id= (kept
	// as a documented extra — the catalog only specifies entity-scoped list).
	entityType := chi.URLParam(r, "entity_type")
	entityIDStr := chi.URLParam(r, "entity_id")
	reviewerIDStr := r.URL.Query().Get("reviewer_id")
	sortParam := r.URL.Query().Get("sort") // spec §29: newest|highest|lowest

	// List by entity.
	if entityType != "" && entityIDStr != "" {
		var entityID pgtype.UUID
		if err := entityID.Scan(entityIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
			return
		}

		reviews, err := h.Service.ListReviewsByEntity(r.Context(), entityType, entityID, limit, offset, sortParam)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if reviews == nil {
			reviews = []gen.Review{}
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": reviews})

		return
	}

	// List by reviewer.
	if reviewerIDStr != "" {
		var reviewerID pgtype.UUID
		if err := reviewerID.Scan(reviewerIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_reviewer_id", "Invalid reviewer_id")
			return
		}

		reviews, err := h.Service.ListReviewsByReviewer(r.Context(), reviewerID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if reviews == nil {
			reviews = []gen.Review{}
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": reviews})

		return
	}

	errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "entity_type+entity_id or reviewer_id required")
}

// ListSpaceReviews handles GET /api/v1/spaces/{id}/reviews.
// Universities are spaces, so this is a convenience route that lists published
// reviews for entity_type='university' + entity_id={id} with author names joined.
// Ref: Section 29 (polymorphic reviews), Section 39 (university as space).
func (h *Handlers) ListSpaceReviews(w http.ResponseWriter, r *http.Request) {

	spaceID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid space ID")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	rows, err := h.Service.ListReviewsByEntityWithAuthor(r.Context(), "university", spaceID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(rows))

	for _, row := range rows {
		authorName := ""
		if row.AuthorUsername != nil {
			authorName = *row.AuthorUsername
		}

		if row.AuthorDisplayName != nil && *row.AuthorDisplayName != "" {
			authorName = *row.AuthorDisplayName
		}

		items = append(items, map[string]any{
			"id":          row.ID,
			"rating":      row.Rating,
			"title":       row.Title,
			"body":        row.Body,
			"author_name": authorName,
			"created_at":  row.CreatedAt,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ListAllReviews handles GET /api/v1/admin/reviews.
// Admin moderation view: lists all reviews for the platform (all statuses),
// regardless of entity or reviewer. Gated by reviews.moderate at the router.
// Supports ?status=, ?entity_type=, ?page_size=, ?page_offset=.
// Returns {"data": [...], "total": N} envelope with entity_name resolved.
func (h *Handlers) ListAllReviews(w http.ResponseWriter, r *http.Request) {

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)
	status := r.URL.Query().Get("status")
	entityType := r.URL.Query().Get("entity_type")

	var (
		reviews []gen.ListAdminReviewsFilteredRow
		total   int32
		err     error
	)

	if status != "" || entityType != "" {
		reviews, err = h.Service.ListAdminReviewsFiltered(r.Context(), status, entityType, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		total, err = h.Service.CountAdminReviewsFiltered(r.Context(), status, entityType)
	} else {
		rows, listErr := h.Service.ListAdminReviews(r.Context(), limit, offset)
		if listErr != nil {
			handleServiceError(w, listErr)
			return
		}

		total, err = h.Service.CountAdminReviews(r.Context())
		// Convert []gen.ListAdminReviewsRow to []gen.ListAdminReviewsFilteredRow
		// since they share the same shape.
		reviews = make([]gen.ListAdminReviewsFilteredRow, len(rows))
		for i, r := range rows {
			reviews[i] = gen.ListAdminReviewsFilteredRow{
				ID: r.ID, EntityType: r.EntityType,
				EntityID: r.EntityID, ReviewerUserID: r.ReviewerUserID,
				Rating: r.Rating, Title: r.Title, Body: r.Body,
				Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
				EntityName: r.EntityName,
			}
		}
	}

	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"reviews": reviews, "total": total}})
}

// GetReview handles GET /api/v1/reviews/{id}.
func (h *Handlers) GetReview(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	review, err := h.Service.GetReview(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": review})
}

// UpdateReview handles PATCH /api/v1/reviews/{id}.
func (h *Handlers) UpdateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	var input struct {
		Rating int16  `json:"rating"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Rating < RatingMin || input.Rating > RatingMax {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_rating", "Rating must be between 1 and 5")
		return
	}

	review, err := h.Service.UpdateReview(r.Context(), id, user.UserID, input.Rating, input.Title, input.Body)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async review aggregate recalculation — fire-and-forget.
	h.enqueueReviewAggregateRecalc(review.EntityType, review.EntityID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": review})
}

// DeleteReview handles DELETE /api/v1/reviews/{id}.
func (h *Handlers) DeleteReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	// Fetch review before deleting to get entity info for aggregate recalc.
	review, err := h.Service.GetReview(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if err := h.Service.DeleteReview(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async review aggregate recalculation — fire-and-forget.
	h.enqueueReviewAggregateRecalc(review.EntityType, review.EntityID)

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// HardDeleteReview handles DELETE /api/v1/admin/reviews/{id}.
// Permanently removes a review from the database (admin only).
func (h *Handlers) HardDeleteReview(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	// Fetch review before deleting to get entity info for aggregate recalc.
	review, err := h.Service.GetReview(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if err := h.Service.HardDeleteReview(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async review aggregate recalculation — fire-and-forget.
	h.enqueueReviewAggregateRecalc(review.EntityType, review.EntityID)

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// FlagReview handles POST /api/v1/reviews/{id}/flag.
func (h *Handlers) FlagReview(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	if err := h.Service.FlagReview(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "flagged"}})
}

// GetAggregate handles GET /api/v1/reviews/aggregate.
// Supports ?entity_type= and ?entity_id=.
func (h *Handlers) GetAggregate(w http.ResponseWriter, r *http.Request) {

	// catalog: GET /reviews/:entity_type/:entity_id/aggregate
	entityType := chi.URLParam(r, "entity_type")
	entityIDStr := chi.URLParam(r, "entity_id")

	if entityType == "" || entityIDStr == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "entity_type and entity_id required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
		return
	}

	aggregate, err := h.Service.GetAggregate(r.Context(), entityType, entityID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": aggregate})
}

// ModerateReview handles PATCH /api/v1/admin/reviews/:id.
// Changes a review's status (admin moderation). The reason is recorded in
// audit_logs per spec §29 (moderation reuses Section 22 Reporting & Appeals).
func (h *Handlers) ModerateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	var input struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	review, err := h.Service.ModerateReview(r.Context(), id, user.UserID, input.Status, input.Reason)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": review})
}
