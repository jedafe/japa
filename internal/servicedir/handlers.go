// Package servicedir — handlers.go provides the HTTP handlers for the
// Service Directory routes (§98, ADR-098).
//
// Routes:
//
//	GET   /api/v1/services                     — list ?dest=&category=&verified=&sort=top|latest
//	GET   /api/v1/services/:slug               — single service listing
//	POST  /api/v1/services                     — create service listing (auth, service.create)
//	PATCH /api/v1/services/:id                 — edit own service listing (auth, service.edit_own)
//	POST  /api/v1/services/:id/claim           — initiate org claim (auth, service.claim)
//	GET   /api/v1/admin/services               — moderation queue (auth, service.verify)
//	PATCH /api/v1/admin/services/:id/verify    — mark verified (auth, service.verify)
//	PATCH /api/v1/admin/services/:id/claim/:claim_id — resolve claim (auth, service.verify)
//
// Ref: japaconnect-engineering/sections/98-service-directory.md
//      japaconnect-engineering/adr/ADR-098-service-directory.md
package servicedir

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

// Handlers exposes the service directory endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates service directory handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("servicedir: encode response", "error", err)
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
// Public handlers (list + single — anonymous browse per §98)
// ---------------------------------------------------------------------------

// HandleList handles GET /services — list published service listings with
// filters. Query params: ?dest=&category=&verified=&sort=top|latest&page=&limit=
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	serviceCategory := r.URL.Query().Get("category")
	sort := r.URL.Query().Get("sort")

	var verifiedPtr *bool
	if v := r.URL.Query().Get("verified"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			verifiedPtr = &b
		}
	}

	page, limit := parsePageParams(r)

	services, total, err := h.service.List(r.Context(), tenantID, destSpaceID, serviceCategory, verifiedPtr, sort, page, limit)
	if err != nil {
		slog.Error("servicedir: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list services")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": services,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGet handles GET /services/:slug — single published service listing.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Service slug is required")
		return
	}

	service, err := h.service.GetBySlug(r.Context(), slug, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}
		slog.Error("servicedir: get by slug failed", "slug", slug, "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get service")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": service})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (create, edit, claim — auth required)
// ---------------------------------------------------------------------------

// HandleCreate handles POST /services — create a new service listing. Auth
// required (service.create permission checked at route registration).
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	var input CreateServiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	service, err := h.service.Create(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		slog.Error("servicedir: create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create service listing")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": service})
}

// HandleEdit handles PATCH /services/:id — edit own service listing. Auth
// required (service.edit_own permission checked at route registration).
func (h *Handlers) HandleEdit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	var input EditServiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	service, err := h.service.EditOwn(r.Context(), contentID, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only edit your own service listings")
			return
		}
		slog.Error("servicedir: edit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "edit_failed", "Failed to edit service listing")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": service})
}

// HandleClaim handles POST /services/:id/claim — initiate an organization
// claim on a service listing. Auth required (service.claim permission
// checked at route registration).
func (h *Handlers) HandleClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "service_directory_disabled", "Service directory is not available")
		return
	}

	var body struct {
		OrgID string `json:"org_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.OrgID == "" {
		writeError(w, http.StatusBadRequest, "missing_org_id", "org_id is required")
		return
	}

	var orgID pgtype.UUID
	if err := orgID.Scan(body.OrgID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid org_id")
		return
	}

	if err := h.service.Claim(r.Context(), contentID, tenantID, orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}
		slog.Error("servicedir: claim failed", "error", err, "user_id", user.UserID)
		writeError(w, http.StatusInternalServerError, "claim_failed", "Failed to initiate claim")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_status": ClaimStatusPending}})
}

// ---------------------------------------------------------------------------
// Admin handlers (verify + moderation queue + claim resolution —
// service.verify permission)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/services — moderation queue listing
// all service listings (any status). Admin-only (service.verify permission
// checked at route registration). Supports
// ?status=&verified=&claim_status=&category=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	status := r.URL.Query().Get("status")
	claimStatus := r.URL.Query().Get("claim_status")
	serviceCategory := r.URL.Query().Get("category")
	q := r.URL.Query().Get("q")

	var verifiedPtr *bool
	if v := r.URL.Query().Get("verified"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			verifiedPtr = &b
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	services, total, err := h.service.AdminList(r.Context(), tenantID, status, claimStatus, serviceCategory, q, verifiedPtr, page, limit)
	if err != nil {
		slog.Error("servicedir: admin list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list services")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": services,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminVerify handles PATCH /admin/services/:id/verify — mark a
// service listing as verified. Admin-only (service.verify permission
// checked at route registration). Optional body: {"verification_type":"manual"}
func (h *Handlers) HandleAdminVerify(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	verificationType := VerificationTypeManual
	var body struct {
		VerificationType *string `json:"verification_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.VerificationType != nil && *body.VerificationType != "" {
		verificationType = *body.VerificationType
	}

	if err := h.service.AdminVerify(r.Context(), contentID, tenantID, verificationType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}
		slog.Error("servicedir: admin verify failed", "error", err)
		writeError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify service")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"verified": true, "verification_type": verificationType}})
}

// HandleAdminResolveClaim handles PATCH /admin/services/:id/claim/:claim_id
// — resolve a pending organization claim. Admin-only (service.verify
// permission checked at route registration). The :claim_id URL param is the
// org_id of the claimant. Body: {"approve": true|false}
func (h *Handlers) HandleAdminResolveClaim(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	contentID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}
	orgID, ok := scanUUIDParam(w, r, "claim_id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "service_directory_unavailable", "Service directory not configured")
		return
	}

	var body struct {
		Approve *bool `json:"approve"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.Approve == nil {
		writeError(w, http.StatusBadRequest, "missing_approve", "approve field is required")
		return
	}

	if err := h.service.AdminResolveClaim(r.Context(), contentID, tenantID, orgID, *body.Approve); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service_not_found", "Service not found")
			return
		}
		slog.Error("servicedir: admin resolve claim failed", "error", err)
		writeError(w, http.StatusInternalServerError, "resolve_claim_failed", "Failed to resolve claim")
		return
	}

	claimStatus := ClaimStatusApproved
	if !*body.Approve {
		claimStatus = ClaimStatusRejected
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"claim_status": claimStatus}})
}
