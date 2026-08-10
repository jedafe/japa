// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073).
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
package scholarships

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the scholarship finder HTTP routes.
type Handlers struct {
	svc *Service
}

// NewHandlers creates scholarship finder handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// HandleFindScholarships searches published scholarship guides.
// GET /scholarships
func (h *Handlers) HandleFindScholarships(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Global killswitch (scholarship_finder.enabled, ADR-073 §73).
	if !h.svc.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "scholarship_finder_disabled", "Scholarship Finder is not available")
		return
	}

	q := r.URL.Query()
	filters := ScholarshipFilters{
		Country:      q.Get("country"),
		Level:        q.Get("level"),
		Q:            q.Get("q"),
		Sort:         q.Get("sort"),
		DeadlineFrom: q.Get("deadline_from"),
		DeadlineTo:   q.Get("deadline_to"),
	}
	if v := q.Get("min_amount"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			filters.MinAmount = f
		}
	}
	if v := q.Get("max_amount"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			filters.MaxAmount = f
		}
	}
	if v := q.Get("renewable"); v != "" {
		b, _ := strconv.ParseBool(v)
		filters.Renewable = &b
	}
	// Offset pagination per conventions/api-response-format.md
	// (?page_size=N&page_offset=N). Back-compat: also accept `page` + `limit`.
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize, _ = strconv.Atoi(q.Get("limit"))
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	if pageOffset < 0 {
		pageOffset, _ = strconv.Atoi(q.Get("page"))
		pageOffset = (pageOffset - 1) * pageSize
		if pageOffset < 0 {
			pageOffset = 0
		}
	}
	filters.Page = (pageOffset / pageSize) + 1
	filters.Limit = pageSize

	results, total, err := h.svc.FindScholarships(r.Context(), tenantID, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", err.Error())
		return
	}
	if results == nil {
		results = []ScholarshipResult{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": results,
		"meta": map[string]interface{}{
			"total":       total,
			"page_size":   pageSize,
			"page_offset": pageOffset,
		},
	})
}

// HandleScholarshipDetail returns a single scholarship guide.
// GET /scholarships/{id}
func (h *Handlers) HandleScholarshipDetail(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Global killswitch (scholarship_finder.enabled, ADR-073 §73).
	if !h.svc.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "scholarship_finder_disabled", "Scholarship Finder is not available")
		return
	}

	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	result, err := h.svc.GetScholarshipDetail(r.Context(), tenantID, guideID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Scholarship not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

// HandleSaveScholarship saves a scholarship to the user's tracker.
// POST /me/scholarships/{id}/save
func (h *Handlers) HandleSaveScholarship(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// Global killswitch (scholarship_finder.enabled, ADR-073 §73).
	if !h.svc.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "scholarship_finder_disabled", "Scholarship Finder is not available")
		return
	}

	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	saved, err := h.svc.SaveScholarship(r.Context(), tenantID, userID, guideID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"id":                 pgtypeUUIDString(saved.ID),
			"guide_id":           pgtypeUUIDString(saved.GuideID),
			"status":             saved.Status,
			"remind_before_days": saved.RemindBeforeDays,
		},
	})
}

// HandleUpdateSavedScholarship updates a saved scholarship's status/notes.
// PATCH /me/scholarships/{id}
func (h *Handlers) HandleUpdateSavedScholarship(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Status           string `json:"status"`
		Notes            string `json:"notes"`
		RemindBeforeDays *int32 `json:"remind_before_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	saved, err := h.svc.UpdateSavedScholarship(r.Context(), tenantID, userID, guideID, body.Status, body.Notes, body.RemindBeforeDays)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": saved})
}

// HandleDeleteSavedScholarship removes a scholarship from the tracker.
// DELETE /me/scholarships/{id}
func (h *Handlers) HandleDeleteSavedScholarship(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteSavedScholarship(r.Context(), tenantID, userID, guideID); err != nil {
		writeError(w, http.StatusBadRequest, "delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": map[string]interface{}{"deleted": true}})
}

// HandleListSavedScholarships lists the user's saved scholarships.
// GET /me/scholarships
func (h *Handlers) HandleListSavedScholarships(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	page, limit := parsePagination(r)

	rows, total, err := h.svc.ListSavedScholarships(r.Context(), tenantID, userID, status, page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []SavedScholarshipResponse{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": rows,
		"meta": map[string]interface{}{
			"total":   total,
			"page":    page,
			"limit":   limit,
		},
	})
}

// HandleAdminStatusDistribution returns admin aggregate stats
// (total_scholarships, upcoming_deadlines, reminders_sent, unique_users,
// and the per-status distribution).
// GET /admin/scholarships/stats
func (h *Handlers) HandleAdminStatusDistribution(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	stats, err := h.svc.AdminStats(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": stats})
}

// HandleAdminTopSaved returns admin top-saved report.
// GET /admin/scholarships/top-saved
func (h *Handlers) HandleAdminTopSaved(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	rows, err := h.svc.AdminTopSaved(r.Context(), tenantID, page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "report_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []gen.AdminScholarshipTopSavedRow{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": rows})
}

// HandleAdminUpcomingDeadlines returns admin upcoming deadlines report
// mapped to the AdminDeadlineItem shape (id, title, deadline, days_until,
// status) that the admin panel's DeadlineItem contract expects.
// GET /admin/scholarships/deadlines
func (h *Handlers) HandleAdminUpcomingDeadlines(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	rows, err := h.svc.AdminDeadlines(r.Context(), tenantID, page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "report_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []AdminDeadlineItem{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": rows})
}

// HandleAdminReminderLog returns admin reminder delivery history.
// GET /admin/scholarships/reminder-log
func (h *Handlers) HandleAdminReminderLog(w http.ResponseWriter, r *http.Request) {
	// Auth before tenant: an unauthenticated request must get 401, not 503.
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	rows, err := h.svc.AdminReminderLog(r.Context(), tenantID, page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "report_failed", err.Error())
		return
	}
	if rows == nil {
		rows = []gen.AdminScholarshipReminderLogRow{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": rows})
}

// --- shared helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"status":  status,
		},
	})
}

type tenantCtxKey struct{}
type userCtxKey struct{}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	id := tenant.IDFromContext(r.Context())
	if !id.Valid {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return id, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}
	return u.UserID, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		writeError(w, http.StatusBadRequest, "missing_param", param+" is required")
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_uuid", "Invalid "+param)
		return pgtype.UUID{}, false
	}
	return id, true
}

func parsePagination(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return page, limit
}
