// Package contentimport — handlers.go provides the HTTP handlers for the
// content import routes (Section 65, ADR-083).
//
// Routes:
//
//	POST   /api/v1/me/imports              — start import { source_type, source_url }
//	GET    /api/v1/me/imports              — list user's imports (paginated)
//	GET    /api/v1/me/imports/sources      — list allowed source types
//	GET    /api/v1/me/imports/:id          — import detail
//	GET    /api/v1/admin/imports           — all imports (admin oversight)
//	POST   /api/v1/admin/imports/:id/retry — retry failed import
//
// RBAC: import.use (all authenticated), import.manage (admin). Permission
// checks are applied by the RBAC middleware at the route registration site.
//
// Ref: japaconnect-engineering/sections/65-content-import.md
//
//	japaconnect-engineering/adr/ADR-083-content-import.md
package contentimport

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the content import HTTP endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates content-import handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as rag/handlers.go)
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

// validateSourceURL checks that a source_url is a valid http/https URL
// within the maxFeedURLLength limit. Returns an error string (empty when
// valid). This is the handler-level guard; the service layer also calls
// validateFeedURL before fetching (SSRF protection with DNS resolution).
func validateSourceURL(raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) > maxFeedURLLength {
		return "source_url is too long"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "source_url is not a valid URL"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "source_url must use http or https"
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return "source_url must have a hostname"
	}
	return ""
}

// parsePagination extracts page and limit from query parameters with sane
// defaults. Page is 1-based; limit defaults to 20 and is clamped to 100.
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

// startImportRequest is the JSON body for POST /me/imports.
type startImportRequest struct {
	SourceType string `json:"source_type"`
	SourceURL  string `json:"source_url"`
}

// adminStartImportRequest is the JSON body for POST /admin/imports.
// It extends the user-facing request with a target user_id — the admin
// starts the import on behalf of that user.
type adminStartImportRequest struct {
	UserID     string `json:"user_id"`
	SourceType string `json:"source_type"`
	SourceURL  string `json:"source_url"`
}

// HandleStartImport handles POST /api/v1/me/imports — start a content
// import. Gated by import.use permission (checked by RBAC middleware at
// the route registration site). Enqueues import:process_source
// fire-and-forget (Rule 8) and returns the import record immediately.
func (h *Handlers) HandleStartImport(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "import_unavailable", "Import service not configured")
		return
	}

	var req startImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if !isValidSourceType(req.SourceType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_type",
			"source_type must be one of: medium, substack, rss, zip")
		return
	}

	if req.SourceType != SourceTypeZIP && req.SourceURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_source_url",
			"source_url is required for RSS-based imports")
		return
	}

	if msg := validateSourceURL(req.SourceURL); msg != "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_url", msg)
		return
	}

	imp, err := h.service.StartImport(r.Context(), tenantID, user.UserID, req.SourceType, req.SourceURL)
	if err != nil {
		slog.Error("contentimport: start failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_start_failed", "Failed to start import")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": serializeImport(imp),
	})
}

// HandleListImports handles GET /api/v1/me/imports — list the authenticated
// user's imports with pagination. Gated by import.use.
func (h *Handlers) HandleListImports(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "import_unavailable", "Import service not configured")
		return
	}

	page, limit := parsePagination(r)

	imports, total, err := h.service.ListUserImports(r.Context(), tenantID, user.UserID, page, limit)
	if err != nil {
		slog.Error("contentimport: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_list_failed", "Failed to list imports")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": serializeImports(imports),
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// sourceTypeResponse describes a single allowed import source type.
type sourceTypeResponse struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	RequiresURL bool   `json:"requires_url"`
}

// allowedSourceTypes is the canonical list of import source types the
// backend accepts. This is the single source of truth — the DB CHECK
// constraint, isValidSourceType, and this list must all agree.
// Ref: japaconnect-engineering/sections/65-content-import.md
var allowedSourceTypes = []sourceTypeResponse{
	{Value: SourceTypeMedium, Label: "Medium", RequiresURL: true},
	{Value: SourceTypeSubstack, Label: "Substack", RequiresURL: true},
	{Value: SourceTypeRSS, Label: "RSS Feed", RequiresURL: true},
	{Value: SourceTypeZIP, Label: "ZIP File", RequiresURL: false},
}

// HandleListImportSources handles GET /api/v1/me/imports/sources — returns
// the list of allowed import source types. Gated by import.use so only
// authenticated users who can start imports see the options.
func (h *Handlers) HandleListImportSources(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": allowedSourceTypes,
	})
}

// HandleGetImport handles GET /api/v1/me/imports/:id — import detail.
// Gated by import.use. Only returns the import if it belongs to the
// authenticated user.
func (h *Handlers) HandleGetImport(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "import_unavailable", "Import service not configured")
		return
	}

	importID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	imp, err := h.service.GetImport(r.Context(), tenantID, importID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "import_not_found", "Import not found")
		return
	}

	// Ensure the import belongs to the authenticated user.
	if imp.UserID != user.UserID {
		errpkg.WriteError(w, http.StatusNotFound, "import_not_found", "Import not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": serializeImport(imp),
	})
}

// HandleAdminStartImport handles POST /api/v1/admin/imports — an admin
// starts a content import on behalf of a user. Gated by import.manage.
// Enqueues import:process_source fire-and-forget (Rule 8) and returns the
// import record immediately.
func (h *Handlers) HandleAdminStartImport(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "import_unavailable", "Import service not configured")
		return
	}

	var req adminStartImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if req.UserID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_user_id", "user_id is required")
		return
	}

	var targetUserID pgtype.UUID
	if err := targetUserID.Scan(req.UserID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "user_id must be a valid UUID")
		return
	}

	if !isValidSourceType(req.SourceType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_type",
			"source_type must be one of: medium, substack, rss, zip")
		return
	}

	if req.SourceType != SourceTypeZIP && req.SourceURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_source_url",
			"source_url is required for RSS-based imports")
		return
	}

	if msg := validateSourceURL(req.SourceURL); msg != "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_url", msg)
		return
	}

	imp, err := h.service.StartImport(r.Context(), tenantID, targetUserID, req.SourceType, req.SourceURL)
	if err != nil {
		slog.Error("contentimport: admin start failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_start_failed", "Failed to start import")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": serializeImport(imp),
	})
}

// HandleAdminListImports handles GET /api/v1/admin/imports — list all
// imports for the tenant with optional status filter. Gated by
// import.manage (admin).
func (h *Handlers) HandleAdminListImports(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "import_unavailable", "Import service not configured")
		return
	}

	page, limit := parsePagination(r)
	status := r.URL.Query().Get("status")

	imports, total, err := h.service.AdminListImports(r.Context(), tenantID, status, page, limit)
	if err != nil {
		slog.Error("contentimport: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_list_failed", "Failed to list imports")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": serializeAdminImports(imports),
		"pagination": map[string]any{
			"page":   page,
			"limit":  limit,
			"total":  total,
			"status": status,
		},
	})
}

// HandleAdminRetryImport handles POST /api/v1/admin/imports/:id/retry —
// retry a failed or partial import. Gated by import.manage (admin).
// Re-enqueues import:process_source fire-and-forget (Rule 8).
func (h *Handlers) HandleAdminRetryImport(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "import_unavailable", "Import service not configured")
		return
	}

	importID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	imp, err := h.service.RetryImport(r.Context(), tenantID, importID)
	if err != nil {
		slog.Error("contentimport: retry failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "import_retry_failed", "Failed to retry import")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
		"data": serializeImport(imp),
	})
}

// ---------------------------------------------------------------------------
// Serialization helpers — all timestamps RFC 3339 UTC, IDs as strings.
// ---------------------------------------------------------------------------

// importResponse is the JSON representation of a content_imports row.
type importResponse struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	UserID        string          `json:"user_id"`
	SourceType    string          `json:"source_type"`
	SourceURL     *string         `json:"source_url"`
	Status        string          `json:"status"`
	TotalItems    int32           `json:"total_items"`
	ImportedCount int32           `json:"imported_count"`
	FailedCount   int32           `json:"failed_count"`
	ErrorLog      json.RawMessage `json:"error_log"`
	CreatedAt     string          `json:"created_at"`
	CompletedAt   *string         `json:"completed_at"`
}

// adminImportResponse extends importResponse with the username of the
// importing user (from the admin list JOIN).
type adminImportResponse struct {
	importResponse
	Username string `json:"username"`
}

// serializeImport converts a gen.ContentImport to its JSON response form.
func serializeImport(imp gen.ContentImport) importResponse {
	resp := importResponse{
		ID:            imp.ID.String(),
		TenantID:      imp.TenantID.String(),
		UserID:        imp.UserID.String(),
		SourceType:    imp.SourceType,
		SourceURL:     imp.SourceUrl,
		Status:        imp.Status,
		TotalItems:    imp.TotalItems,
		ImportedCount: imp.ImportedCount,
		FailedCount:   imp.FailedCount,
		ErrorLog:      normalizeErrorLog(imp.ErrorLog),
		CreatedAt:     formatTimestamptz(imp.CreatedAt),
	}
	if imp.CompletedAt.Valid {
		s := formatTimestamptz(imp.CompletedAt)
		resp.CompletedAt = &s
	}
	return resp
}

// serializeImports converts a slice of gen.ContentImport to response form.
func serializeImports(imports []gen.ContentImport) []importResponse {
	result := make([]importResponse, 0, len(imports))
	for i := range imports {
		result = append(result, serializeImport(imports[i]))
	}
	return result
}

// serializeAdminImports converts admin list rows (with username) to
// response form.
func serializeAdminImports(rows []gen.AdminListContentImportsRow) []adminImportResponse {
	result := make([]adminImportResponse, 0, len(rows))
	for _, row := range rows {
		resp := adminImportResponse{
			importResponse: importResponse{
				ID:            row.ID.String(),
				TenantID:      row.TenantID.String(),
				UserID:        row.UserID.String(),
				SourceType:    row.SourceType,
				SourceURL:     row.SourceUrl,
				Status:        row.Status,
				TotalItems:    row.TotalItems,
				ImportedCount: row.ImportedCount,
				FailedCount:   row.FailedCount,
				ErrorLog:      normalizeErrorLog(row.ErrorLog),
				CreatedAt:     formatTimestamptz(row.CreatedAt),
			},
			Username: row.Username,
		}
		if row.CompletedAt.Valid {
			s := formatTimestamptz(row.CompletedAt)
			resp.CompletedAt = &s
		}
		result = append(result, resp)
	}
	return result
}

// normalizeErrorLog converts a sqlc-generated []byte error_log (JSONB)
// into a json.RawMessage so it serializes as raw JSON in the HTTP response,
// not as a base64-encoded string. Falls back to "[]" when the input is
// nil or invalid JSON.
func normalizeErrorLog(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	if json.Valid(raw) {
		return json.RawMessage(raw)
	}
	return json.RawMessage("[]")
}

// formatTimestamptz formats a pgtype.Timestamptz as RFC 3339 UTC.
// Returns empty string for invalid timestamps.
func formatTimestamptz(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
}
