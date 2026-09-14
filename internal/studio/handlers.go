// Package studio — handlers.go implements the HTTP handlers for the AI
// Content Studio admin API (Section 75).
//
// Ref: japaconnect-engineering/sections/75-ai-content-studio.md
package studio

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/personas"
)

// personaLister is the subset of the personas service that studio needs to
// populate the persona dropdown in the create-job modal.
type personaLister interface {
	ListPersonas(ctx context.Context, status, q, sort, dir string, limit, offset int32) ([]personas.PersonaListItem, int64, error)
}

// intelRecordGetter is the subset of the intelligence store needed to
// validate that an intel_record_id provided to the create-job API exists
// and belongs to the same tenant.
type intelRecordGetter interface {
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
}

// Handlers holds the HTTP handlers for the studio admin API.
type Handlers struct {
	svc        *Service
	personas   personaLister
	intelStore intelRecordGetter
}

// NewHandlers creates Handlers wrapping the given service.
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{svc: svc}
}

// SetPersonaService injects the personas service for the studio persona
// dropdown endpoint (called at server wiring time).
func (h *Handlers) SetPersonaService(p personaLister) { h.personas = p }

// SetIntelStore injects the intelligence store for intel_record_id
// validation on the create-job endpoint (called at server wiring time).
func (h *Handlers) SetIntelStore(s intelRecordGetter) { h.intelStore = s }

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
	return pgtype.UUID{}, true
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

// HandleListJobs — GET /api/v1/admin/studio/jobs.
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

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": jobs,
		"meta": map[string]int64{"total": total, "limit": int64(limit), "offset": int64(offset)},
	})
}

// HandleCreateJob — POST /api/v1/admin/studio/jobs.
func (h *Handlers) HandleCreateJob(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
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

	// Approach B: validate intel_record_id if provided (optional field).
	if in.IntelRecordID.Valid && h.intelStore != nil {
		if _, err := h.intelStore.GetIntelRecord(r.Context(), in.IntelRecordID); err != nil {
			h.writeError(w, http.StatusBadRequest, "intel_record_not_found", "intel_record_id does not exist or does not belong to this tenant")
			return
		}
	}

	job, err := h.svc.CreateJob(r.Context(), user.UserID, in)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "studio_create_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]any{"data": job})
}

// HandleGetJob — GET /api/v1/admin/studio/jobs/:id.
func (h *Handlers) HandleGetJob(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	job, err := h.svc.GetJob(r.Context(), id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "job_not_found", "studio job not found")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": job})
}

// HandleRetryJob — POST /api/v1/admin/studio/jobs/:id/retry.
func (h *Handlers) HandleRetryJob(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.RetryJob(r.Context(), id); err != nil {
		h.writeError(w, http.StatusBadRequest, "retry_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"status": "queued"})
}

// HandleCancelJob — DELETE /api/v1/admin/studio/jobs/:id.
func (h *Handlers) HandleCancelJob(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.CancelJob(r.Context(), id); err != nil {
		if errors.Is(err, ErrJobNotCancellable) {
			h.writeError(w, http.StatusConflict, "job_not_cancellable", err.Error())
			return
		}

		h.writeError(w, http.StatusBadRequest, "cancel_failed", err.Error())

		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

// HandleListBatches — GET /api/v1/admin/studio/batches.
func (h *Handlers) HandleListBatches(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit, offset := h.parsePagination(r)

	batches, total, err := h.svc.ListBatches(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "batch_list_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": batches,
		"meta": map[string]int64{"total": total, "limit": int64(limit), "offset": int64(offset)},
	})
}

// HandleCreateBatch — POST /api/v1/admin/studio/batches.
func (h *Handlers) HandleCreateBatch(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
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

	batch, jobs, err := h.svc.CreateBatch(r.Context(), user.UserID, in)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "batch_create_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"batch": batch, "jobs": jobs}})
}

// HandleGetBatch — GET /api/v1/admin/studio/batches/:id.
func (h *Handlers) HandleGetBatch(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	batch, stats, err := h.svc.GetBatch(r.Context(), id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "batch_not_found", "studio batch not found")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"batch": batch, "stats": stats}})
}

// HandleRetryBatch — POST /api/v1/admin/studio/batches/:id/retry
// Enqueues a studio:retry_failed task for asynchronous processing (ADR-076 §7).
// The task retries all failed jobs in the batch without blocking the HTTP
// request — batches may contain many failed jobs and retrying them
// synchronously would time out.
func (h *Handlers) HandleRetryBatch(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := h.scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.EnqueueRetryFailed(r.Context(), id); err != nil {
		h.writeError(w, http.StatusBadRequest, "batch_retry_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusAccepted, map[string]any{"status": "retry_queued"})
}

// HandleContentTypes — GET /api/v1/admin/studio/content-types.
func (h *Handlers) HandleContentTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	resp := h.svc.ListContentTypes(r.Context())
	h.writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// HandleStats — GET /api/v1/admin/studio/stats.
func (h *Handlers) HandleStats(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	stats, err := h.svc.GetStats(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "stats_failed", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": stats})
}

// HandlePersonas — GET /api/v1/admin/studio/personas
// Returns active personas in {id, label} format for the create-job dropdown.
// Delegates to the injected personas service.
func (h *Handlers) HandlePersonas(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if h.personas == nil {
		h.writeError(w, http.StatusServiceUnavailable, "personas_unavailable", "Persona service not configured")
		return
	}

	// Fetch active personas only — studio jobs require an active persona.
	items, _, err := h.personas.ListPersonas(r.Context(), "active", "", "created_at", "desc", 100, 0)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "personas_list_failed", "Failed to list personas")
		return
	}

	// Map to the {id, label} shape the frontend dropdown expects.
	type personaOption struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}

	opts := make([]personaOption, 0, len(items))

	for _, p := range items {
		label := p.ProfileName
		if label == "" {
			label = p.BackingUsername
		}

		if label == "" {
			label = p.ID[:8]
		}

		opts = append(opts, personaOption{ID: p.ID, Label: label})
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"data": opts})
}
