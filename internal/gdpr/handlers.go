// Package gdpr — HTTP handlers for GDPR/NDPR Compliance (Section 19).
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package gdpr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
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
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, errpkg.ErrRequired):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, errpkg.ErrInvalid):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_consent_type", err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
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

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": record})
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": records})
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
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "consent type is required")
		return
	}

	// Use current policy version — in production this would come from config.
	policyVersion := "1.0"

	err := h.Service.RevokeConsent(r.Context(), tenantID, user.UserID, consentType, policyVersion)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
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

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"data": request})
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
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "no_pending_export"}})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": request})
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
		httpx.WriteError(w, http.StatusNotFound, "export_not_ready", "No completed export is available for download")
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	request, err := h.Service.RequestAccountDeletion(r.Context(), tenantID, user.UserID, input.Reason)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"data": request})
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
			httpx.WriteError(w, http.StatusBadRequest, "invalid_actor", "Invalid actor_user_id")
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
	offset := httpx.ParseOffset(r)

	logs, err := h.Service.ListAuditLogsWithQuery(r.Context(), tenantID, actorUserID, actionType, targetType, r.URL.Query().Get("q"), startDate, endDate, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": logs})
}

// ListConsentsByTenant handles GET /api/v1/admin/consents.
func (h *Handlers) ListConsentsByTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	consentType := r.URL.Query().Get("type")
	limit := parsePageSize(r)
	offset := httpx.ParseOffset(r)

	records, err := h.Service.ListConsentsByTenantWithQuery(r.Context(), tenantID, consentType, r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": records})
}

// DeleteConsent handles DELETE /api/v1/admin/consents/{id}.
func (h *Handlers) DeleteConsent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid consent ID")
		return
	}

	record, err := h.Service.DeleteConsent(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": record})
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": requests})
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": requests})
}

// ---------------------------------------------------------------------------
// Service glue (same package) — threads GDPR search/filter params through to
// the Store without touching service.go.
// ---------------------------------------------------------------------------

// DeleteConsent deletes a single consent record by ID (RBAC: gdpr.consents.delete).
func (s *Service) DeleteConsent(ctx context.Context, id, tenantID pgtype.UUID) (gen.ConsentRecord, error) {
	return s.store.deleteConsent(ctx, id, tenantID)
}

// ListConsentsByTenantWithQuery lists consent records with an optional `q` filter.
func (s *Service) ListConsentsByTenantWithQuery(ctx context.Context, tenantID pgtype.UUID, consentType, q string, limit, offset int32) ([]gen.ConsentRecord, error) {
	return s.store.listConsentsByTenantFiltered(ctx, tenantID, consentType, q, limit, offset)
}

// ListAuditLogsWithQuery lists audit logs with an optional `q` filter.
func (s *Service) ListAuditLogsWithQuery(ctx context.Context, tenantID, actorUserID pgtype.UUID, actionType, targetType, q string, startDate, endDate *time.Time, limit, offset int32) ([]gen.AuditLog, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	var startDateTZ, endDateTZ pgtype.Timestamptz
	if startDate != nil {
		startDateTZ = pgtype.Timestamptz{Time: *startDate, Valid: true}
	}

	if endDate != nil {
		endDateTZ = pgtype.Timestamptz{Time: *endDate, Valid: true}
	}

	var targetTypePtr *string
	if targetType != "" {
		targetTypePtr = &targetType
	}

	zeroUUID := pgtype.UUID{}
	_ = zeroUUID.Scan("00000000-0000-0000-0000-000000000000")

	actorFilter := zeroUUID
	if actorUserID.Valid {
		actorFilter = actorUserID
	}

	return s.store.listAuditLogs(ctx, gen.ListAuditLogsParams{
		TenantID:    tenantID,
		ActorUserID: actorFilter,
		ActionType:  actionType,
		TargetType:  targetTypePtr,
		StartDate:   startDateTZ,
		EndDate:     endDateTZ,
		Q:           q,
		PageLimit:   limit,
		PageOffset:  offset,
	})
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
	if _, ok := h.requireTenant(w, r); !ok {
		return
	}

	var requestID pgtype.UUID
	if err := requestID.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid deletion request ID")
		return
	}

	// Process the actual account deletion pipeline (anonymize user, revoke tokens,
	// delete sessions, purge Honcho AI context, mark request as completed).
	if err := h.Service.ProcessAccountDeletion(r.Context(), requestID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{
		"request_id": requestID.String(), "status": "executed",
	}})
}
