package dataimport

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the data import service over HTTP.
type Handlers struct {
	service *Service
}

// NewHandlers creates data-import handlers bound to the provided service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
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

func parsePagination(r *http.Request) (int, int) {
	pageSize := 20
	pageOffset := 0

	if q := r.URL.Query().Get("page_size"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			pageSize = v
		}
	}

	if q := r.URL.Query().Get("page_offset"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 0 {
			pageOffset = v
		}
	}

	if pageSize > 100 {
		pageSize = 100
	}
	// Convert offset to page number for the service layer.
	page := (pageOffset / pageSize) + 1

	return page, pageSize
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// createImportRequest is the JSON body for POST /admin/data-imports.
type createImportRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Format  string `json:"format"`
	FileURL string `json:"file_url"`
	Mode    string `json:"mode"`
}

// HandleCreateImport handles POST /api/v1/admin/data-imports — create a new
// platform data import. Gated by gdpr.data_imports.manage.
func (h *Handlers) HandleCreateImport(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var req createImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if req.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if !ValidImportType(req.Type) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_type",
			"type must be one of: users, posts, comments, orders, transactions, analytics, moderation")
		return
	}

	if !ValidFormat(req.Format) {
		req.Format = FormatCSV // default
	}

	if req.Mode == "" {
		req.Mode = ModeCreate
	}

	imp, err := h.service.CreateImport(r.Context(), tenantID, user.UserID, req.Name, req.Type, req.Format, req.FileURL, req.Mode)
	if err != nil {
		slog.Error("dataimport: create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_create_failed", "Failed to create import")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": serializeImport(imp)})
}

// HandleListImports handles GET /api/v1/admin/data-imports.
func (h *Handlers) HandleListImports(w http.ResponseWriter, r *http.Request) {
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

	imports, total, err := h.service.ListImports(r.Context(), tenantID, status, page, limit)
	if err != nil {
		slog.Error("dataimport: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_list_failed", "Failed to list imports")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": serializeImports(imports),
		"pagination": map[string]any{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"total":       total,
			"status":      status,
		},
	})
}

// HandleGetImport handles GET /api/v1/admin/data-imports/:id.
func (h *Handlers) HandleGetImport(w http.ResponseWriter, r *http.Request) {
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

	imp, err := h.service.GetImport(r.Context(), id, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "import_not_found", "Import not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": serializeImport(imp)})
}

// HandleDeleteImport handles DELETE /api/v1/admin/data-imports/:id.
func (h *Handlers) HandleDeleteImport(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.DeleteImport(r.Context(), id, tenantID); err != nil {
		slog.Error("dataimport: delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_delete_failed", "Failed to delete import")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Serialization helpers
// ---------------------------------------------------------------------------

type importResponse struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Format       string  `json:"format"`
	Status       string  `json:"status"`
	FileURL      string  `json:"fileUrl"`
	TotalRows    int32   `json:"totalRows"`
	ImportedRows int32   `json:"importedRows"`
	FailedRows   int32   `json:"failedRows"`
	ErrorLog     any     `json:"errorLog"`
	Mode         string  `json:"mode"`
	CreatedBy    string  `json:"createdBy"`
	CreatedAt    string  `json:"createdAt"`
	CompletedAt  *string `json:"completedAt"`
}

func serializeImport(i gen.PlatformDataImport) importResponse {
	resp := importResponse{
		ID:           i.ID.String(),
		Name:         i.Name,
		Type:         i.ImportType,
		Format:       i.Format,
		Status:       i.Status,
		FileURL:      i.FileUrl,
		TotalRows:    i.TotalRows,
		ImportedRows: i.ImportedRows,
		FailedRows:   i.FailedRows,
		Mode:         i.Mode,
		CreatedBy:    i.CreatedBy.String(),
		CreatedAt:    formatTs(i.CreatedAt),
	}

	// Parse error_log JSON for the response.
	var errorLog any
	if len(i.ErrorLog) > 0 {
		_ = json.Unmarshal(i.ErrorLog, &errorLog)
	}

	resp.ErrorLog = errorLog

	if i.CompletedAt.Valid {
		s := formatTs(i.CompletedAt)
		resp.CompletedAt = &s
	}

	return resp
}

func serializeImports(rows []gen.PlatformDataImport) []importResponse {
	result := make([]importResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, serializeImport(row))
	}

	return result
}

func formatTs(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}

	return ts.Time.UTC().Format("2006-01-02T15:04:05Z")
}
