package video

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
)

// AdminHandlers exposes admin-only endpoints for the video claim review queue.
type AdminHandlers struct {
	Service *Service
	authMW  func(http.Handler) http.Handler
	rbac    *rbac.RBAC
}

// NewAdminHandlers creates admin video claim handlers.
func NewAdminHandlers(svc *Service, authMW func(http.Handler) http.Handler, rbac *rbac.RBAC) *AdminHandlers {
	return &AdminHandlers{
		Service: svc,
		authMW:  authMW,
		rbac:    rbac,
	}
}

// RegisterRoutes wires the admin claim routes under /api/v1.
func (h *AdminHandlers) RegisterRoutes(r chi.Router) {
	r.With(h.authMW, rbac.RequirePermission(h.rbac, "admin.video_claims.read")).
		Get("/admin/video-claims", h.ListClaims)
	r.With(h.authMW, rbac.RequirePermission(h.rbac, "admin.video_claims.manage")).
		Patch("/admin/video-claims/{id}", h.ReviewClaim)

	r.With(h.authMW, rbac.RequirePermission(h.rbac, "admin.video_sources.read")).
		Get("/admin/video-sources", h.ListSources)
	r.With(h.authMW, rbac.RequirePermission(h.rbac, "admin.video_sources.read")).
		Get("/admin/video-sources/{id}", h.GetSource)
	r.With(h.authMW, rbac.RequirePermission(h.rbac, "admin.video_sources.manage")).
		Patch("/admin/video-sources/{id}", h.PatchSource)

	r.With(h.authMW, rbac.RequirePermission(h.rbac, "admin.video_import_jobs.read")).
		Get("/admin/video-import-jobs", h.ListImportJobs)
	r.With(h.authMW, rbac.RequirePermission(h.rbac, "admin.video_import_jobs.read")).
		Get("/admin/video-import-jobs/{id}", h.GetImportJob)
	r.With(h.authMW, rbac.RequirePermission(h.rbac, "admin.video_import_jobs.manage")).
		Post("/admin/video-import-jobs/{id}/retry", h.RetryImportJob)
}

// adminListQueryFunc loads a tenant-scoped, paginated list for an admin endpoint.
type adminListQueryFunc func(ctx context.Context, status, typeFilter, q, sort, dir string, limit, offset int32) (any, int32, error)

// runAdminList is the shared boilerplate for admin list endpoints.
func (h *AdminHandlers) runAdminList(w http.ResponseWriter, r *http.Request, typeKey string, fetch adminListQueryFunc) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	typeFilter := r.URL.Query().Get(typeKey)
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")
	dir := r.URL.Query().Get("dir")
	limit, offset := parsePage(r, "page_size", "page_offset")

	items, total, err := fetch(r.Context(), status, typeFilter, q, sort, dir, limit, offset)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	writePagedListResponse(w, items, total, limit, offset)
}

// ListClaims handles GET /api/v1/admin/video-claims.
func (h *AdminHandlers) ListClaims(w http.ResponseWriter, r *http.Request) {
	h.runAdminList(w, r, "claim_type", func(ctx context.Context, status, typeFilter, q, sort, dir string, limit, offset int32) (any, int32, error) {
		return h.Service.ListClaimsForAdmin(ctx, status, typeFilter, q, sort, dir, limit, offset)
	})
}

// writePagedListResponse writes a standard paginated list envelope.
func writePagedListResponse(w http.ResponseWriter, items any, total, limit, offset int32) {
	totalPages := int32(0)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":       items,
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		},
	})
}

// ReviewClaim handles PATCH /api/v1/admin/video-claims/{id}.
func (h *AdminHandlers) ReviewClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid claim ID")
		return
	}

	var input struct {
		Status         string  `json:"status"`
		ResolutionNote *string `json:"resolution_note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	resolutionNote := ""
	if input.ResolutionNote != nil {
		resolutionNote = *input.ResolutionNote
	}

	claim, err := h.Service.ReviewClaim(r.Context(), user.UserID, id, input.Status, resolutionNote)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": claim})
}

// ListSources handles GET /api/v1/admin/video-sources.
func (h *AdminHandlers) ListSources(w http.ResponseWriter, r *http.Request) {
	h.runAdminList(w, r, "source_type", func(ctx context.Context, status, typeFilter, q, sort, dir string, limit, offset int32) (any, int32, error) {
		return h.Service.ListSourcesForAdmin(ctx, status, typeFilter, q, sort, dir, limit, offset)
	})
}

// adminItemFunc fetches a single admin-scoped item by ID.
type adminItemFunc func(ctx context.Context, id pgtype.UUID) (any, error)

// runAdminGet is the shared boilerplate for admin GET-by-ID endpoints.
func (h *AdminHandlers) runAdminGet(w http.ResponseWriter, r *http.Request, notFoundMsg string, fetch adminItemFunc) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", notFoundMsg)
		return
	}

	item, err := fetch(r.Context(), id)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// GetSource handles GET /api/v1/admin/video-sources/{id}.
func (h *AdminHandlers) GetSource(w http.ResponseWriter, r *http.Request) {
	h.runAdminGet(w, r, "Invalid source ID", func(ctx context.Context, id pgtype.UUID) (any, error) {
		return h.Service.GetSourceForAdmin(ctx, id)
	})
}

// PatchSource handles PATCH /api/v1/admin/video-sources/{id}.
func (h *AdminHandlers) PatchSource(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	_, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid source ID")
		return
	}

	var input struct {
		Status *string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Status == nil || *input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	source, err := h.Service.UpdateSourceStatus(r.Context(), id, *input.Status)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": source})
}

// ListImportJobs handles GET /api/v1/admin/video-import-jobs.
func (h *AdminHandlers) ListImportJobs(w http.ResponseWriter, r *http.Request) {
	h.runAdminList(w, r, "source_type", func(ctx context.Context, status, typeFilter, q, sort, dir string, limit, offset int32) (any, int32, error) {
		return h.Service.ListImportJobsForAdmin(ctx, status, typeFilter, q, sort, dir, limit, offset)
	})
}

// GetImportJob handles GET /api/v1/admin/video-import-jobs/{id}.
func (h *AdminHandlers) GetImportJob(w http.ResponseWriter, r *http.Request) {
	h.runAdminGet(w, r, "Invalid import job ID", func(ctx context.Context, id pgtype.UUID) (any, error) {
		return h.Service.GetImportJobForAdmin(ctx, id)
	})
}

// RetryImportJob handles POST /api/v1/admin/video-import-jobs/{id}/retry.
func (h *AdminHandlers) RetryImportJob(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	_, ok = requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid import job ID")
		return
	}

	job, err := h.Service.RetryImportJob(r.Context(), id, user.UserID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": job})
}
