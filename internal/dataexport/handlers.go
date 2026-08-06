package dataexport

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the data export service over HTTP.
type Handlers struct {
	service *Service
}

// NewHandlers creates data-export handlers bound to the provided service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as contentimport/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("dataexport: encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

func parsePagination(r *http.Request) (int, int) {
	page := 1
	limit := 20
	if q := r.URL.Query().Get("page"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			page = v
		}
	}
	if q := r.URL.Query().Get("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// createExportRequest is the JSON body for POST /admin/data-exports.
type createExportRequest struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Format          string `json:"format"`
	IncludeMetadata bool   `json:"includeMetadata"`
}

// HandleCreateExport handles POST /api/v1/admin/data-exports — create a new
// platform data export. Gated by gdpr.data_exports.view.
func (h *Handlers) HandleCreateExport(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req createExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}
	if !ValidExportType(req.Type) {
		writeError(w, http.StatusBadRequest, "invalid_type",
			"type must be one of: users, posts, comments, orders, transactions, analytics, moderation")
		return
	}
	if !ValidFormat(req.Format) {
		req.Format = FormatCSV // default to CSV
	}

	exp, err := h.service.CreateExport(r.Context(), tenantID, user.UserID, req.Name, req.Type, req.Format, req.IncludeMetadata)
	if err != nil {
		slog.Error("dataexport: create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "export_create_failed", "Failed to create export")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"data": serializeExport(exp)})
}

// HandleListExports handles GET /api/v1/admin/data-exports — list all exports.
func (h *Handlers) HandleListExports(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePagination(r)
	status := r.URL.Query().Get("status")

	exports, total, err := h.service.ListExports(r.Context(), tenantID, status, page, limit)
	if err != nil {
		slog.Error("dataexport: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "export_list_failed", "Failed to list exports")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": serializeExports(exports),
		"pagination": map[string]any{
			"page":   page,
			"limit":  limit,
			"total":  total,
			"status": status,
		},
	})
}

// HandleGetExport handles GET /api/v1/admin/data-exports/:id.
func (h *Handlers) HandleGetExport(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	exp, err := h.service.GetExport(r.Context(), id, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "export_not_found", "Export not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": serializeExport(exp)})
}

// HandleDeleteExport handles DELETE /api/v1/admin/data-exports/:id.
func (h *Handlers) HandleDeleteExport(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.DeleteExport(r.Context(), id, tenantID); err != nil {
		slog.Error("dataexport: delete failed", "error", err)
		writeError(w, http.StatusInternalServerError, "export_delete_failed", "Failed to delete export")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Serialization helpers
// ---------------------------------------------------------------------------

type exportResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	Format          string  `json:"format"`
	Status          string  `json:"status"`
	FileURL         *string `json:"fileUrl"`
	FileSize        *int64  `json:"fileSize"`
	IncludeMetadata bool    `json:"includeMetadata"`
	ErrorMessage    *string `json:"errorMessage"`
	CreatedBy       string  `json:"createdBy"`
	CreatedAt       string  `json:"createdAt"`
	CompletedAt     *string `json:"completedAt"`
}

func serializeExport(e gen.PlatformDataExport) exportResponse {
	resp := exportResponse{
		ID:              e.ID.String(),
		Name:            e.Name,
		Type:            e.ExportType,
		Format:          e.Format,
		Status:          e.Status,
		FileURL:         e.FileUrl,
		FileSize:        e.FileSize,
		IncludeMetadata: e.IncludeMetadata,
		ErrorMessage:    e.ErrorMessage,
		CreatedBy:       e.CreatedBy.String(),
		CreatedAt:       formatTs(e.CreatedAt),
	}
	if e.CompletedAt.Valid {
		s := formatTs(e.CompletedAt)
		resp.CompletedAt = &s
	}
	return resp
}

func serializeExports(rows []gen.PlatformDataExport) []exportResponse {
	result := make([]exportResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, serializeExport(row))
	}
	return result
}

func formatTs(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format("2006-01-02T15:04:05Z")
}
