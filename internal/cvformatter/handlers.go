// Package cvformatter — handlers.go provides the HTTP handlers for the
// International CV / Resume Formatter routes (§95, ADR-103).
//
// Routes:
//
//	GET  /api/v1/cv-formatter/standards           — list destination standards (public)
//	GET  /api/v1/cv-formatter/standards/:dest     — single destination standard (public)
//	POST /api/v1/cv-formatter/format              — submit CV for formatting (auth, cv_formatter.use)
//	GET  /api/v1/cv-formatter/sessions/:id        — poll session status + output (auth, owner)
//	GET  /api/v1/cv-formatter/sessions/:id/download — download as PDF or DOCX (auth, owner)
//	GET  /api/v1/admin/cv-formatter/standards     — list standards (admin, cv_formatter.manage)
//	POST /api/v1/admin/cv-formatter/standards     — create/update standard (admin, cv_formatter.manage)
//
// Ref: japaconnect-engineering/sections/95-cv-formatter.md
//      japaconnect-engineering/adr/ADR-103-cv-formatter.md
package cvformatter

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the CV formatter endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates CV formatter handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("cvformatter: encode response", "error", err)
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

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n)
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (standards — anonymous browse per §95)
// ---------------------------------------------------------------------------

// HandleListStandards handles GET /cv-formatter/standards — list all
// destination CV standards. Public; returns 404 with cv_formatter_disabled
// when the feature flag is off. Accepts ?dest= to filter by destination.
func (h *Handlers) HandleListStandards(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	// If ?dest= is provided, redirect to the single-standard endpoint logic.
	if destParam := r.URL.Query().Get("dest"); destParam != "" {
		var destSpaceID pgtype.UUID
		if err := destSpaceID.Scan(destParam); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
		standard, err := h.service.GetStandardByDest(r.Context(), destSpaceID)
		if err != nil {
			writeError(w, http.StatusNotFound, "standard_not_found", "CV standard not found for this destination")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": standard})
		return
	}

	standards, err := h.service.ListStandards(r.Context())
	if err != nil {
		slog.Error("cvformatter: list standards failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list standards")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": standards})
}

// HandleGetStandard handles GET /cv-formatter/standards/:dest — single
// destination CV standard. Public; :dest is the destination_space_id.
func (h *Handlers) HandleGetStandard(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	destSpaceID, ok := scanUUIDParam(w, r, "dest")
	if !ok {
		return
	}

	standard, err := h.service.GetStandardByDest(r.Context(), destSpaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "standard_not_found", "CV standard not found for this destination")
			return
		}
		slog.Error("cvformatter: get standard failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get standard")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": standard})
}

// ---------------------------------------------------------------------------
// Authenticated handler (format — auth + quota required)
// ---------------------------------------------------------------------------

// HandleFormat handles POST /cv-formatter/format — submit a CV for
// formatting. Auth required (cv_formatter.use permission checked at route
// registration). Quota-checked via CheckQuota. Creates a session with
// status='pending' and returns 202 Accepted. The actual AI processing is
// an Asynq task (not implemented here).
func (h *Handlers) HandleFormat(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	var input FormatInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}
	if input.InputText == "" {
		writeError(w, http.StatusBadRequest, "missing_input", "input_text is required")
		return
	}

	if err := h.service.CheckQuota(r.Context(), user.UserID, tenantID); err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			writeError(w, http.StatusTooManyRequests, "quota_exceeded", "Monthly CV formatting quota reached")
			return
		}
		slog.Error("cvformatter: quota check failed", "error", err)
		writeError(w, http.StatusInternalServerError, "quota_check_failed", "Failed to check quota")
		return
	}

	session, err := h.service.CreateSession(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		slog.Error("cvformatter: create session failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create format session")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"data": session})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (session poll + download — auth + owner only)
// ---------------------------------------------------------------------------

// HandleGetSession handles GET /cv-formatter/sessions/:id — poll session
// status and retrieve output. Auth required; owner only.
func (h *Handlers) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	sessionID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	row, err := h.service.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "session_not_found", "Format session not found")
			return
		}
		slog.Error("cvformatter: get session failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get session")
		return
	}

	if row.UserID != user.UserID {
		writeError(w, http.StatusForbidden, "not_owner", "You can only access your own format sessions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": sessionRowToDTO(row)})
}

// HandleDownload handles GET /cv-formatter/sessions/:id/download —
// download the formatted CV as a file. Auth required; owner only. The
// response body is the output_text with a Content-Type header matching
// the requested format (docx or pdf). Returns 409 if the session is not
// yet complete or has failed.
func (h *Handlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	sessionID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	row, err := h.service.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "session_not_found", "Format session not found")
			return
		}
		slog.Error("cvformatter: download get session failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get session")
		return
	}

	if row.UserID != user.UserID {
		writeError(w, http.StatusForbidden, "not_owner", "You can only access your own format sessions")
		return
	}

	if row.Status != SessionStatusComplete {
		writeError(w, http.StatusConflict, "not_ready", "Session output is not ready (status: "+row.Status+")")
		return
	}

	if row.OutputText == nil {
		writeError(w, http.StatusConflict, "no_output", "Session has no output text")
		return
	}

	contentType := "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	if row.OutputFormat == OutputFormatPDF {
		contentType = "application/pdf"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\"cv-formatted."+row.OutputFormat+"\"")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(*row.OutputText)); err != nil {
		slog.Error("cvformatter: write download body", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Admin handlers (standards management — cv_formatter.manage permission)
// ---------------------------------------------------------------------------

// HandleAdminListStandards handles GET /admin/cv-formatter/standards —
// list all CV standards for management. Admin-only (cv_formatter.manage
// permission checked at route registration).
func (h *Handlers) HandleAdminListStandards(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if _, ok := requireTenant(w, r); !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "cv_formatter_unavailable", "CV formatter not configured")
		return
	}

	// Admin list works even if the public feature flag is off, so
	// administrators can configure standards before enabling the feature.

	standards, err := h.service.ListStandards(r.Context())
	if err != nil {
		slog.Error("cvformatter: admin list standards failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list standards")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": standards})
}

// HandleAdminUpsertStandard handles POST /admin/cv-formatter/standards —
// create or update a CV standard for a destination space. Admin-only
// (cv_formatter.manage permission checked at route registration).
func (h *Handlers) HandleAdminUpsertStandard(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if _, ok := requireTenant(w, r); !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "cv_formatter_unavailable", "CV formatter not configured")
		return
	}

	var input StandardInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	standard, err := h.service.UpsertStandard(r.Context(), input)
	if err != nil {
		slog.Error("cvformatter: admin upsert standard failed", "error", err)
		writeError(w, http.StatusInternalServerError, "upsert_failed", "Failed to create or update standard")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": standard})
}
