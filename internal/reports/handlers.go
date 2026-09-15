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
	errpkg "github.com/jedafe/japa/internal/errors"
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


// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
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

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	entityID, err := parseUUIDParam(r, idParam)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var body reportRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	report, err := h.service.CreateReport(r.Context(), user.UserID, CreateReportInput{
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
		})
		if err := h.enqueuer.Enqueue(worker.TypeModerationAutoHide, payload); err != nil {
			slog.Warn("reports: failed to enqueue moderation:auto_hide", "entity_type", entityType, "entity_id", entityID.String(), "error", err)
		}

		// Enqueue admin notification (Section 51 report:notify_admin).
		// Notifies every user with reports.review that a new report was filed.
		notifyPayload, _ := json.Marshal(map[string]string{
			"report_id": report.ID.String(),

			"entity_type": entityType,
			"entity_id":   entityID.String(),
			"reason":      body.Reason,
		})
		if err := h.enqueuer.Enqueue(worker.TypeReportNotifyAdmin, notifyPayload); err != nil {
			slog.Warn("reports: failed to enqueue report:notify_admin", "report_id", report.ID.String(), "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": report})
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

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	myReports, err := h.service.ListMyReports(r.Context(), user.UserID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list your reports")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": myReports})
}

// ListReports handles GET /api/v1/admin/reports. Supports ?status= and
// ?entity_type= filters.
func (h *Handlers) ListReports(w http.ResponseWriter, r *http.Request) {

	var status, entityType, q *string
	if s := r.URL.Query().Get("status"); s != "" {
		status = &s
	}

	if et := r.URL.Query().Get("entity_type"); et != "" {
		entityType = &et
	}

	if s := r.URL.Query().Get("q"); s != "" {
		q = &s
	}

	// Frontend sends a 0-indexed `page`; derive the absolute offset.
	page := int32(0)

	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.ParseInt(p, 10, 32); err == nil && v >= 0 {
			page = int32(v)
		}
	}

	limit := parsePageSize(r)
	offset := page * limit

	allReports, err := h.service.ListReports(r.Context(), status, entityType, q, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list reports")
		return
	}

	total, err := h.service.CountReports(r.Context(), status, entityType, q)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to count reports")
		return
	}

	totalPages := int32(0)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       allReports,
			"page":        page,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// DeleteReport handles DELETE /api/v1/admin/reports/{id}.
func (h *Handlers) DeleteReport(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.service.DeleteReport(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// GetReport handles GET /api/v1/admin/reports/{id}.
func (h *Handlers) GetReport(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	report, err := h.service.GetReport(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": report})
}

// UpdateReport handles PATCH /api/v1/admin/reports/{id}.
// It records the report resolution and, for warn/ban actions, also fires the
// matching user-moderation side effect (warn the reporter / ban the reported user).
func (h *Handlers) UpdateReport(w http.ResponseWriter, r *http.Request) {

	reviewer, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Action string `json:"action"` // "dismiss", "warn", "ban", "remove"
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Action == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_action", "action is required")
		return
	}

	// Load the report so we can resolve warn/ban targets.
	report, err := h.service.GetReport(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	status := "actioned"
	if input.Action == "dismiss" {
		status = "dismissed"
	}

	var note *string
	if input.Reason != "" {
		note = &input.Reason
	}

	updated, err := h.service.UpdateReportStatus(r.Context(), id, reviewer.UserID, status, note)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// Fire the user-moderation side effects for warn/ban actions. Both
	// target the reported user (the offender), resolved via ResolveTargetUser
	// — never the reporter. Space reports and unknown types resolve to no
	// target, in which case the side effect is skipped.
	switch input.Action {
	case "warn":
		target, found, e := h.service.ResolveTargetUser(r.Context(), report.EntityType, report.EntityID)
		if e != nil {
			slog.Warn("reports: resolve warn target failed", "id", id.String(), "error", e)
		} else if found {
			if e2 := h.service.WarnUser(r.Context(), target, input.Reason); e2 != nil {
				slog.Warn("reports: warn side-effect failed", "id", id.String(), "error", e2)
			}
		}
	case "ban":
		// "Ban Target": ban the reported user (content author / reported account).
		target, found, e := h.service.ResolveTargetUser(r.Context(), report.EntityType, report.EntityID)
		if e != nil {
			slog.Warn("reports: resolve ban target failed", "id", id.String(), "error", e)
		} else if found {
			if e2 := h.service.BanUser(r.Context(), target, input.Reason); e2 != nil {
				slog.Warn("reports: ban side-effect failed", "id", id.String(), "error", e2)
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// writeServiceError maps Service sentinel errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidEntityType), errors.Is(err, ErrInvalidReason), errors.Is(err, ErrInvalidStatus):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, ErrAlreadyReported):
		errpkg.WriteError(w, http.StatusConflict, "already_reported", err.Error())
	case errors.Is(err, errpkg.ErrNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Report not found")
	default:
		errpkg.HandleServiceError(w, err)
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
