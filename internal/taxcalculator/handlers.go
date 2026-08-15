// Package taxcalculator — handlers.go provides the HTTP handlers for the
// Tax & Financial Obligations Calculator routes (§100, ADR-108).
//
// Routes:
//
//	GET   /api/v1/tax-calculator                      — list published guides ?dest=&origin=&employment=
//	GET   /api/v1/tax-calculator/treaty               — treaty status ?dest=&origin=
//	GET   /api/v1/admin/tax-calculator                — manage guides + treaties (auth, tax_calculator.manage)
//	POST  /api/v1/admin/tax-calculator/guides         — create guide (auth, tax_calculator.manage)
//	POST  /api/v1/admin/tax-calculator/guides/generate — Content Agent draft placeholder (auth, tax_calculator.manage)
//	PATCH /api/v1/admin/tax-calculator/guides/:id/approve — HITL approve (auth, tax_calculator.manage)
//	POST  /api/v1/admin/tax-calculator/treaties       — create/update treaty pair (auth, tax_calculator.manage)
//
// Ref: japaconnect-engineering/sections/100-tax-calculator.md
//
//	japaconnect-engineering/adr/ADR-108-tax-calculator.md
package taxcalculator

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the tax calculator endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates tax calculator handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
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
// Public handlers (list + treaty — anonymous browse per §100)
// ---------------------------------------------------------------------------

// HandleList handles GET /tax-calculator — list published tax obligation
// guides with filters. Query params: ?dest=&origin=&employment=&page=&limit=
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "tax_calculator_disabled", "Tax calculator is not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var originSpaceID pgtype.UUID
	if v := r.URL.Query().Get("origin"); v != "" {
		if err := originSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
			return
		}
	}

	employmentStatus := r.URL.Query().Get("employment")

	page, limit := parsePageParams(r)

	guides, total, err := h.service.List(r.Context(), tenantID, destSpaceID, originSpaceID, employmentStatus, page, limit)
	if err != nil {
		slog.Error("taxcalculator: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list tax guides")
		return
	}

	totalPages := int32(1)
	if limit > 0 && total > 0 {
		tp := (total + int64(limit) - 1) / int64(limit)
		if tp > int64(math.MaxInt32) {
			tp = int64(math.MaxInt32)
		}
		totalPages = int32(tp) //nolint:gosec // G115: bounded to MaxInt32 above
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": guides,
		"meta": map[string]any{
			"page":        page,
			"per_page":    limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// HandleTreaty handles GET /tax-calculator/treaty — treaty status for an
// origin/destination pair. Query params: ?dest=&origin=
func (h *Handlers) HandleTreaty(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "tax_calculator_disabled", "Tax calculator is not available")
		return
	}

	destStr := r.URL.Query().Get("dest")
	originStr := r.URL.Query().Get("origin")
	if destStr == "" || originStr == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_params", "dest and origin query params are required")
		return
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(destStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
		return
	}
	var originSpaceID pgtype.UUID
	if err := originSpaceID.Scan(originStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
		return
	}

	treaty, err := h.service.GetTreaty(r.Context(), tenantID, originSpaceID, destSpaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteJSON(w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"origin_space_id":      originStr,
					"destination_space_id": destStr,
					"treaty_exists":        false,
					"found":                false,
				},
			})
			return
		}
		slog.Error("taxcalculator: treaty lookup failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "treaty_lookup_failed", "Failed to look up treaty")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": treaty})
}

// ---------------------------------------------------------------------------
// Admin handlers (manage guides + treaties — tax_calculator.manage
// permission checked at route registration)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/tax-calculator — management view
// listing all guides (any status) plus all treaty pairs. Admin-only.
// Supports ?status=&employment=&dest=&origin=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tax_calculator_unavailable", "Tax calculator not configured")
		return
	}

	status := r.URL.Query().Get("status")
	employmentStatus := r.URL.Query().Get("employment")
	q := r.URL.Query().Get("q")

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		_ = destSpaceID.Scan(v)
	}
	var originSpaceID pgtype.UUID
	if v := r.URL.Query().Get("origin"); v != "" {
		_ = originSpaceID.Scan(v)
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	guides, treaties, total, err := h.service.AdminList(r.Context(), tenantID, status, employmentStatus, q, destSpaceID, originSpaceID, page, limit)
	if err != nil {
		slog.Error("taxcalculator: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list tax guides")
		return
	}

	totalPages := int32(1)
	if limit > 0 && total > 0 {
		tp := (total + int64(limit) - 1) / int64(limit)
		if tp > int64(math.MaxInt32) {
			tp = int64(math.MaxInt32)
		}
		totalPages = int32(tp) //nolint:gosec // G115: bounded to MaxInt32 above
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"guides":   guides,
			"treaties": treaties,
		},
		"meta": map[string]any{
			"page":        page,
			"per_page":    limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// HandleCreateGuide handles POST /admin/tax-calculator/guides — create a
// new tax obligation guide. Admin-only (tax_calculator.manage permission
// checked at route registration).
func (h *Handlers) HandleCreateGuide(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "tax_calculator_disabled", "Tax calculator is not available")
		return
	}

	var input CreateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.CreateGuide(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("taxcalculator: create guide failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create tax guide")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": guide})
}

// HandleGenerateGuide handles POST /admin/tax-calculator/guides/generate —
// Content Agent draft placeholder. Admin-only. Returns 202 Accepted with
// a placeholder response describing the pending draft state.
func (h *Handlers) HandleGenerateGuide(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "tax_calculator_disabled", "Tax calculator is not available")
		return
	}

	var input GenerateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	result, err := h.service.GenerateGuide(r.Context(), tenantID, input)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "generate_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": result})
}

// HandleApproveGuide handles PATCH /admin/tax-calculator/guides/:id/approve
// — Human-in-the-Loop approval. Sets hitl_reviewed_by, hitl_reviewed_at,
// and status='published'. Admin-only.
func (h *Handlers) HandleApproveGuide(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
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

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "tax_calculator_disabled", "Tax calculator is not available")
		return
	}

	var input ApproveGuideRequest
	// Body is optional; ignore decode errors so an empty body is accepted.
	_ = json.NewDecoder(r.Body).Decode(&input)

	guide, err := h.service.ApproveGuide(r.Context(), id, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Tax guide not found")
			return
		}
		slog.Error("taxcalculator: approve guide failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve tax guide")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guide})
}

// HandleUpsertTreaty handles POST /admin/tax-calculator/treaties — create
// or update a tax treaty pair. Admin-only. Uses upsert semantics so
// repeated calls for the same pair update the mutable fields.
func (h *Handlers) HandleUpsertTreaty(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "tax_calculator_disabled", "Tax calculator is not available")
		return
	}

	var input UpsertTreatyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	treaty, err := h.service.UpsertTreaty(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("taxcalculator: upsert treaty failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "upsert_treaty_failed", "Failed to create or update treaty")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": treaty})
}
