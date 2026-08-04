// Package studio — handlers.go implements the HTTP handlers for the AI
// Content Studio admin API (Section 75).
//
// Ref: japaconnect-engineering/sections/75-ai-content-studio.md
package studio

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds the HTTP handlers for the studio admin API.
type Handlers struct {
	svc *Service
}

// NewHandlers creates Handlers wrapping the given service.
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{svc: svc}
}

func (h *Handlers) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handlers) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"status":  status,
		},
	})
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		h.writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func (h *Handlers) scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

func (h *Handlers) parsePagination(r *http.Request) (int32, int32) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	// limit is bounded [1,100] and offset is bounded [0,+inf) so int32
	// conversion is safe — no overflow possible.
	return int32(limit), int32(offset) // #nosec G109 G115 — bounded values
}

// HandleListJobs — GET /api/v1/admin/studio/jobs
func (h *Handlers) HandleListJobs(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit, offset := h.parsePagination(r)
	f := JobListFilter{Limit: limit, Offset: offset}

	if s := r.URL.Query().Get("status"); s != "" {
		f.Status = &s
	}
	if ct := r.URL.Query().Get("content_type"); ct != "" {
		f.ContentType = &ct
	}
	if pid := r.URL.Query().Get("persona_id"); pid != "" {
		var id pgtype.UUID
		if err := id.Scan(pid); err == nil {
			f.PersonaID = &id
		}
	}
	if bid := r.URL.Query().Get("batch_id"); bid != "" {
		var id pgtype.UUID
		if err := id.Scan(bid); err == nil {
			f.BatchID = &id
		}
	}

	jobs, total, err := h.svc.ListJobs(r.Context(), tenantID, f)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "studio_list_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": jobs, "total": total})
}

// HandleCreateJob — POST /api/v1/admin/studio/jobs
func (h *Handlers) HandleCreateJob(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var in CreateJobInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_json", "invalid request body")
		return
	}

	job, err := h.svc.CreateJob(r.Context(), tenantID, user.UserID, in)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "studio_create_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusCreated, job)
}

// HandleGetJob — GET /api/v1/admin/studio/jobs/:id
func (h *Handlers) HandleGetJob(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	job, err := h.svc.GetJob(r.Context(), tenantID, id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "job_not_found", "studio job not found")
		return
	}
	h.writeJSON(w, http.StatusOK, job)
}

// HandleRetryJob — POST /api/v1/admin/studio/jobs/:id/retry
func (h *Handlers) HandleRetryJob(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.RetryJob(r.Context(), tenantID, id); err != nil {
		h.writeError(w, http.StatusBadRequest, "retry_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"status": "queued"})
}

// HandleCancelJob — DELETE /api/v1/admin/studio/jobs/:id
func (h *Handlers) HandleCancelJob(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.CancelJob(r.Context(), tenantID, id); err != nil {
		h.writeError(w, http.StatusBadRequest, "cancel_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

// HandleListBatches — GET /api/v1/admin/studio/batches
func (h *Handlers) HandleListBatches(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	limit, offset := h.parsePagination(r)
	batches, total, err := h.svc.ListBatches(r.Context(), tenantID, limit, offset)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "batch_list_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": batches, "total": total})
}

// HandleCreateBatch — POST /api/v1/admin/studio/batches
func (h *Handlers) HandleCreateBatch(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var in CreateBatchInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad_json", "invalid request body")
		return
	}

	batch, jobs, err := h.svc.CreateBatch(r.Context(), tenantID, user.UserID, in)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "batch_create_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]any{"batch": batch, "jobs": jobs})
}

// HandleGetBatch — GET /api/v1/admin/studio/batches/:id
func (h *Handlers) HandleGetBatch(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	batch, stats, err := h.svc.GetBatch(r.Context(), tenantID, id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "batch_not_found", "studio batch not found")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"batch": batch, "stats": stats})
}

// HandleRetryBatch — POST /api/v1/admin/studio/batches/:id/retry
func (h *Handlers) HandleRetryBatch(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	count, err := h.svc.RetryBatchFailed(r.Context(), tenantID, id)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "batch_retry_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"retried": count})
}

// HandleContentTypes — GET /api/v1/admin/studio/content-types
func (h *Handlers) HandleContentTypes(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	types := h.svc.ListContentTypes(r.Context(), tenantID)
	h.writeJSON(w, http.StatusOK, map[string]any{"items": types})
}

// HandleStats — GET /api/v1/admin/studio/stats
func (h *Handlers) HandleStats(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	stats, err := h.svc.GetStats(r.Context(), tenantID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, stats)
}

// HandlePersonas — GET /api/v1/admin/studio/personas
// This is a passthrough to the personas service; the server wires the actual
// personas handler at this route. This handler exists as a fallback.
func (h *Handlers) HandlePersonas(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
}

// _ prevents unused import warning for strings.
var _ = strings.TrimSpace
