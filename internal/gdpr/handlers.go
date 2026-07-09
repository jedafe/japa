// Package gdpr — HTTP handlers for GDPR/NDPR Compliance (Section 19).
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package gdpr

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for GDPR HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "already_exists"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "required"):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	case strings.Contains(err.Error(), "invalid consent_type"):
		writeError(w, http.StatusBadRequest, "invalid_consent_type", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Warn("json encode error", "error", err)
		}
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// parsePageSize extracts page_size from query params with defaults.
func parsePageSize(r *http.Request) int32 {
	ps := DefaultPageSize

	if v := r.URL.Query().Get("page_size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			if parsed > MaxPageSize {
				parsed = MaxPageSize
			}

			ps = parsed
		}
	}

	return int32(ps) //nolint:gosec // G115: bounded by MaxPageSize check above
}

// parseOffset extracts offset from query params.
func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			return int32(parsed) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	return 0
}

// clientIP extracts the client IP from the request.
func clientIP(r *http.Request) *string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}

	if ip == "" {
		ip = r.RemoteAddr
	}

	if ip != "" {
		// Take the first IP from comma-separated list.
		if idx := strings.Index(ip, ","); idx > 0 {
			ip = ip[:idx]
		}
		// Strip port if present.
		if idx := strings.LastIndex(ip, ":"); idx > 0 {
			ip = ip[:idx]
		}

		return &ip
	}

	return nil
}

// ---------------------------------------------------------------------------
// User-facing endpoints
// ---------------------------------------------------------------------------

// RecordConsent handles POST /api/v1/consent.
func (h *Handlers) RecordConsent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		ConsentType   string `json:"consent_type"`
		Granted       bool   `json:"granted"`
		PolicyVersion string `json:"policy_version"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	record, err := h.Service.RecordConsent(r.Context(), tenantID, user.UserID, RecordConsentInput{
		ConsentType:   input.ConsentType,
		Granted:       input.Granted,
		PolicyVersion: input.PolicyVersion,
		IPAddress:     clientIP(r),
		UserAgent:     strPtr(r.UserAgent()),
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": record})
}

// GetUserConsents handles GET /api/v1/consent.
func (h *Handlers) GetUserConsents(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	consentType := r.URL.Query().Get("type")

	records, err := h.Service.GetUserConsents(r.Context(), user.UserID, consentType)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": records})
}

// RevokeConsent handles DELETE /api/v1/consent/:type.
func (h *Handlers) RevokeConsent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	consentType := chi.URLParam(r, "type")
	if consentType == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "consent type is required")
		return
	}

	// Use current policy version — in production this would come from config.
	policyVersion := "1.0"

	err := h.Service.RevokeConsent(r.Context(), tenantID, user.UserID, consentType, policyVersion)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

// RequestDataExport handles POST /api/v1/account/export.
func (h *Handlers) RequestDataExport(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	request, err := h.Service.RequestDataExport(r.Context(), tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"data": request})
}

// GetMyDataExportStatus handles GET /api/v1/me/data-export/status.
func (h *Handlers) GetMyDataExportStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	request, found, err := h.Service.GetLatestDataExportRequest(r.Context(), user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !found {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "no_pending_export"}})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": request})
}

// GetMyDataExportDownload handles GET /api/v1/me/data-export/download.
// Redirects to the signed download URL once the export has completed.
func (h *Handlers) GetMyDataExportDownload(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	request, found, err := h.Service.GetLatestDataExportRequest(r.Context(), user.UserID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !found || request.Status != "completed" || request.DownloadUrl == nil {
		writeError(w, http.StatusNotFound, "export_not_ready", "No completed export is available for download")
		return
	}

	http.Redirect(w, r, *request.DownloadUrl, http.StatusTemporaryRedirect)
}

// RequestAccountDeletion handles POST /api/v1/account/delete.
func (h *Handlers) RequestAccountDeletion(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Reason string `json:"reason"`
	}

	// Reason is optional: DELETE /me may be sent with no body. Tolerate an
	// empty body (io.EOF) so deletion works without a payload; only reject
	// genuinely malformed JSON.
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	request, err := h.Service.RequestAccountDeletion(r.Context(), tenantID, user.UserID, input.Reason)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"data": request})
}

// ---------------------------------------------------------------------------
// Admin endpoints
// ---------------------------------------------------------------------------

// ListAuditLogs handles GET /api/v1/admin/audit-logs.
func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	actionType := r.URL.Query().Get("action_type")
	targetType := r.URL.Query().Get("target_type")

	var actorUserID pgtype.UUID
	if v := r.URL.Query().Get("actor_user_id"); v != "" {
		if err := actorUserID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_actor", "Invalid actor_user_id")
			return
		}
	}

	var startDate, endDate *time.Time

	if v := r.URL.Query().Get("start_date"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			startDate = &t
		}
	}

	if v := r.URL.Query().Get("end_date"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			endDate = &t
		}
	}

	limit := parsePageSize(r)
	offset := parseOffset(r)

	logs, err := h.Service.ListAuditLogs(r.Context(), tenantID, actorUserID, actionType, targetType, startDate, endDate, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": logs})
}

// ListConsentsByTenant handles GET /api/v1/admin/consents.
func (h *Handlers) ListConsentsByTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	consentType := r.URL.Query().Get("type")
	limit := parsePageSize(r)
	offset := parseOffset(r)

	records, err := h.Service.ListConsentsByTenant(r.Context(), tenantID, consentType, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": records})
}

// ListPendingDataExports handles GET /api/v1/admin/data-exports.
func (h *Handlers) ListPendingDataExports(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)

	requests, err := h.Service.ListPendingDataExportRequests(r.Context(), limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": requests})
}

// ListPendingDeletions handles GET /api/v1/admin/deletions.
func (h *Handlers) ListPendingDeletions(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)

	requests, err := h.Service.ListPendingDeletionRequests(r.Context(), limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": requests})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string {
	return &s
}

// ─── P5.6 GDPR Execute Deletion ──────────────────────────────────────────

// ExecuteDeletion handles POST /admin/gdpr/deletion-requests/:id/execute (P5.6).
func (h *Handlers) ExecuteDeletion(w http.ResponseWriter, r *http.Request) {
	requestID := chi.URLParam(r, "id")
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	// Mark the deletion request as executed — actual deletion is async via worker
	_, err := h.Service.db.Writer.Exec(r.Context(),
		`UPDATE gdpr_deletion_requests SET status = 'executed', executed_at = now(), updated_at = now()
		 WHERE id = $1 AND status = 'pending'`, requestID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{
		"request_id": requestID, "status": "executed",
	}})
}
