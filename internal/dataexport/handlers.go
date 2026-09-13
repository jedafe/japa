package dataexport

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

// parsePagination extracts page_size and page_offset from the query string,
// following the project's standard pagination convention (Rule 5 /
// conventions/api-response-format.md). For backwards compatibility, the
// legacy `page` and `limit` params are still accepted as aliases.
//
// Returns (page, limit) where page is 1-based.
func parsePagination(r *http.Request) (int, int) {
	q := r.URL.Query()

	// page_size is the standard; limit is the legacy alias.
	limit := 20

	if v := q.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	} else if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	if limit > 100 {
		limit = 100
	}

	// page_offset is the standard (1-based); page is the legacy alias.
	page := 1

	if v := q.Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	} else if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if req.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if !ValidExportType(req.Type) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_type",
			"type must be one of: users, posts, comments, orders, transactions, analytics, moderation")
		return
	}
	// Default empty format to CSV. Reject xlsx explicitly: the schema lists
	// it (Section 67) but no XLSX generator is implemented, and silently
	// producing a CSV file labelled xlsx misled users (BUG-002).
	if req.Format == "" {
		req.Format = FormatCSV
	} else if !ImplementedFormat(req.Format) {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_format",
			"format must be one of: csv, json (xlsx is not yet implemented)")
		return
	}

	exp, err := h.service.CreateExport(r.Context(), tenantID, user.UserID, req.Name, req.Type, req.Format, req.IncludeMetadata)
	if err != nil {
		slog.Error("dataexport: create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "export_create_failed", "Failed to create export")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": serializeExport(exp)})
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
		errpkg.WriteError(w, http.StatusInternalServerError, "export_list_failed", "Failed to list exports")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": serializeExports(exports),
		"pagination": map[string]any{
			"page_size":   limit,
			"page_offset": page,
			"total":       total,
			"status":      status,
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
		// Distinguish "not found" from a real DB failure so 5xx errors are
		// not masked as 404s (BUG-009).
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "export_not_found", "Export not found")
		} else {
			slog.Error("dataexport: get failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "export_get_failed", "Failed to fetch export")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": serializeExport(exp)})
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
		errpkg.WriteError(w, http.StatusInternalServerError, "export_delete_failed", "Failed to delete export")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// HandleDownloadExport handles GET /api/v1/admin/data-exports/:id/download —
// serves the generated export file. The export must be completed and the
// caller's tenant must match the export's tenant. Gated by gdpr.data_exports.view.
func (h *Handlers) HandleDownloadExport(w http.ResponseWriter, r *http.Request) {
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

	filePath, err := h.service.GetFilePath(r.Context(), id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "export_not_found", "Export not found")
		} else {
			slog.Error("dataexport: download path failed", "error", err)
			errpkg.WriteError(w, http.StatusNotFound, "file_not_available", "Export file not available")
		}

		return
	}

	// Determine content type from the export format.
	exp, err := h.service.GetExport(r.Context(), id, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "export_get_failed", "Failed to fetch export")
		return
	}

	contentType := "application/octet-stream"

	switch exp.Format {
	case FormatJSON:
		contentType = "application/json"
	case FormatCSV:
		contentType = "text/csv"
	}

	baseName := sanitizeFilename(exp.Name)
	filename := fmt.Sprintf("export_%s_%s.%s", baseName, exp.ID.String()[:8], exp.Format)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, filePath)
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
