package reports

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

const defaultPageSize int32 = 20

// TaskEnqueuer is the interface for enqueuing background tasks.
// *worker.Enqueuer satisfies this structurally.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Handlers holds dependencies for report HTTP handlers.
type Handlers struct {
	service  *Service
	enqueuer TaskEnqueuer
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// SetEnqueuer injects the task enqueuer for background job dispatch.
func (h *Handlers) SetEnqueuer(e TaskEnqueuer) {
	h.enqueuer = e
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

// reportRequestBody is the shared submission body for every report route.
type reportRequestBody struct {
	Reason       string   `json:"reason"`
	Notes        *string  `json:"notes"`
	EvidenceURLs []string `json:"evidence_urls"`
}

// submitReport is the shared implementation behind every entity-specific
// report handler: parse the entity ID from the given URL param, decode the
// body, and create the report.
func (h *Handlers) submitReport(w http.ResponseWriter, r *http.Request, entityType, idParam string) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	entityID, err := parseUUIDParam(r, idParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var body reportRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	report, err := h.service.CreateReport(r.Context(), tenantID, user.UserID, CreateReportInput{
		EntityType:   entityType,
		EntityID:     entityID,
		Reason:       body.Reason,
		Notes:        body.Notes,
		EvidenceURLs: body.EvidenceURLs,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// Enqueue auto-hide check after each report. Best-effort — log and
	// continue if enqueue fails (the report was already persisted).
	if h.enqueuer != nil {
		payload, _ := json.Marshal(map[string]string{
			"entity_type": entityType,
			"entity_id":   entityID.String(),
			"tenant_id":   tenantID.String(),
		})
		if err := h.enqueuer.Enqueue(worker.TypeModerationAutoHide, payload); err != nil {
			slog.Warn("reports: failed to enqueue moderation:auto_hide",
				"entity_type", entityType, "entity_id", entityID.String(), "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": report})
}

// ReportContent handles POST /api/v1/content/{id}/report.
func (h *Handlers) ReportContent(w http.ResponseWriter, r *http.Request) {
	h.submitReport(w, r, EntityTypeContent, "id")
}

// ReportComment handles POST /api/v1/comments/{id}/report.
func (h *Handlers) ReportComment(w http.ResponseWriter, r *http.Request) {
	h.submitReport(w, r, EntityTypeComment, "id")
}

// ReportUser handles POST /api/v1/users/{id}/report.
func (h *Handlers) ReportUser(w http.ResponseWriter, r *http.Request) {
	h.submitReport(w, r, EntityTypeUser, "id")
}

// ReportSpace handles POST /api/v1/spaces/{id}/report.
func (h *Handlers) ReportSpace(w http.ResponseWriter, r *http.Request) {
	h.submitReport(w, r, EntityTypeSpace, "id")
}

// ReportDMMessage handles POST /api/v1/dm/conversations/{id}/messages/{mid}/report.
func (h *Handlers) ReportDMMessage(w http.ResponseWriter, r *http.Request) {
	h.submitReport(w, r, EntityTypeMessage, "mid")
}

// ListMyReports handles GET /api/v1/me/reports.
func (h *Handlers) ListMyReports(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	myReports, err := h.service.ListMyReports(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list your reports")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": myReports})
}

// ListReports handles GET /api/v1/admin/reports. Supports ?status= and
// ?entity_type= filters.
func (h *Handlers) ListReports(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var status, entityType *string
	if s := r.URL.Query().Get("status"); s != "" {
		status = &s
	}

	if et := r.URL.Query().Get("entity_type"); et != "" {
		entityType = &et
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	allReports, err := h.service.ListReports(r.Context(), tenantID, status, entityType, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list reports")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": allReports})
}

// GetReport handles GET /api/v1/admin/reports/{id}.
func (h *Handlers) GetReport(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	report, err := h.service.GetReport(r.Context(), id, tenantID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": report})
}

// UpdateReport handles PATCH /api/v1/admin/reports/{id}.
func (h *Handlers) UpdateReport(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	reviewer, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Status         string  `json:"status"`
		ResolutionNote *string `json:"resolution_note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	report, err := h.service.UpdateReportStatus(r.Context(), id, tenantID, reviewer.UserID, input.Status, input.ResolutionNote)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": report})
}

// writeServiceError maps Service sentinel errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidEntityType), errors.Is(err, ErrInvalidReason), errors.Is(err, ErrInvalidStatus):
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, ErrAlreadyReported):
		writeError(w, http.StatusConflict, "already_reported", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Report not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Report operation failed")
	}
}

// parseUUIDParam extracts a chi URL param and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(chi.URLParam(r, name))

	return id, err
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v <= 0 || v > 100 {
		return defaultPageSize
	}

	return int32(v)
}

// parsePageOffset extracts the page_offset query parameter.
func parsePageOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < 0 {
		return 0
	}

	return int32(v)
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Warn("json encode error", "error", err)
		}
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
