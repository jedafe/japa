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
// Unlike the other entity routes it verifies the reporter is a participant in
// the conversation and that the message belongs to it — reports can't be
// filed against arbitrary message UUIDs.
func (h *Handlers) ReportDMMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	conversationID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid conversation ID format")
		return
	}

	messageID, err := parseUUIDParam(r, "mid")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid message ID format")
		return
	}

	participant, err := h.service.IsDMParticipant(r.Context(), conversationID, messageID, user.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify conversation access")
		return
	}
	if !participant {
		errpkg.WriteError(w, http.StatusForbidden, "not_participant", "You are not a participant in this conversation")
		return
	}

	h.submitReport(w, r, EntityTypeMessage, "mid")
}

// ReportForumReply handles POST /api/v1/forum/replies/{id}/report.
func (h *Handlers) ReportForumReply(w http.ResponseWriter, r *http.Request) {
	h.submitReport(w, r, EntityTypeForumReply, "id")
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
	reviewer, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.service.DeleteReport(r.Context(), id, reviewer.UserID); err != nil {
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
// Actions: dismiss/remove/warn/ban resolve the report (remove also archives or
// soft-deletes the reported entity; warn/ban fire the matching user-moderation
// side effect) via the shared resolveWithAction flow; review marks the report
// 'reviewing' and assigns it to the caller.
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
		Action string `json:"action"` // "dismiss", "warn", "ban", "remove", "review"
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

	updated, err := h.service.ApplyAdminAction(r.Context(), id, reviewer.UserID, input.Action, input.Reason)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// writeServiceError maps Service sentinel errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidAction):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_action", err.Error())
	case errors.Is(err, ErrInvalidEntityType), errors.Is(err, ErrInvalidReason), errors.Is(err, ErrInvalidStatus),
		errors.Is(err, ErrNotesTooLong), errors.Is(err, ErrTooManyEvidenceURLs), errors.Is(err, ErrInvalidEvidenceURL):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, ErrSelfReport):
		errpkg.WriteError(w, http.StatusBadRequest, "self_report", err.Error())
	case errors.Is(err, ErrAlreadyReported):
		errpkg.WriteError(w, http.StatusConflict, "already_reported", err.Error())
	case errors.Is(err, ErrAlreadyResolved):
		errpkg.WriteError(w, http.StatusConflict, "already_resolved", err.Error())
	case errors.Is(err, ErrNotParticipant):
		errpkg.WriteError(w, http.StatusForbidden, "not_participant", err.Error())
	case errors.Is(err, ErrNoTarget):
		errpkg.WriteError(w, http.StatusBadRequest, "no_target", err.Error())
	case errors.Is(err, ErrActionNotApplicable):
		errpkg.WriteError(w, http.StatusBadRequest, "action_not_applicable", err.Error())
	case errors.Is(err, ErrEntityNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "entity_not_found", "The reported item no longer exists")
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
