// Package visaroutes — handlers.go provides the HTTP handlers for the
// Visa Route Comparison Tool routes (§90, ADR-098).
//
// Routes:
//
//	GET   /api/v1/visa-routes                   — list ?dest=&profession=&education=&experience_years=
//	GET   /api/v1/visa-routes/compare           — compare ?dest=&ids=id1,id2,id3,id4 (max 4)
//	GET   /api/v1/visa-routes/:id               — route detail
//	GET   /api/v1/admin/visa-routes             — all routes (visa_routes.manage)
//	POST  /api/v1/admin/visa-routes             — create route (visa_routes.manage)
//	POST  /api/v1/admin/visa-routes/generate    — Content Agent draft (visa_routes.manage)
//	PATCH /api/v1/admin/visa-routes/:id         — update route (visa_routes.manage)
//	PATCH /api/v1/admin/visa-routes/:id/approve — HITL approve (visa_routes.manage)
//	DELETE /api/v1/admin/visa-routes/:id        — deactivate route (visa_routes.manage)
//
// Ref: japaconnect-engineering/sections/90-visa-route-comparison.md
//
//	japaconnect-engineering/adr/ADR-098-visa-route-comparison.md
package visaroutes

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// maxCompareIDs is the maximum number of routes the compare endpoint
// accepts (§90).
const maxCompareIDs = 4

// Handlers exposes the visa routes endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates visa routes handlers bound to the provided service.
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
// Public handlers (list, compare, detail — anonymous browse per §90)
// ---------------------------------------------------------------------------

// HandleList handles GET /visa-routes — list published+active visa routes
// with filters. Query params: ?dest=&profession=&education=&experience_years=&page=&limit=
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_routes_disabled", "Visa routes are not available")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	profession := r.URL.Query().Get("profession")
	education := r.URL.Query().Get("education")

	var experienceYears int32
	if v := r.URL.Query().Get("experience_years"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			experienceYears = int32(n)
		}
	}

	page, limit := parsePageParams(r)

	routes, total, err := h.service.List(r.Context(), tenantID, destSpaceID, profession, education, experienceYears, page, limit)
	if err != nil {
		slog.Error("visaroutes: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list visa routes")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": routes,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleCompare handles GET /visa-routes/compare — compare up to 4 visa
// routes side-by-side. Query params: ?ids=id1,id2,id3,id4 (max 4).
func (h *Handlers) HandleCompare(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_routes_disabled", "Visa routes are not available")
		return
	}

	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "ids query parameter is required")
		return
	}

	parts := strings.Split(idsParam, ",")
	if len(parts) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "At least one route id is required")
		return
	}
	if len(parts) > maxCompareIDs {
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_ids", "A maximum of 4 route ids can be compared")
		return
	}

	ids := make([]pgtype.UUID, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var id pgtype.UUID
		if err := id.Scan(p); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid route id: "+p)
			return
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "At least one route id is required")
		return
	}

	routes, err := h.service.Compare(r.Context(), tenantID, ids)
	if err != nil {
		slog.Error("visaroutes: compare failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "compare_failed", "Failed to compare visa routes")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": routes})
}

// HandleGet handles GET /visa-routes/:id — single published+active visa
// route detail.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_routes_disabled", "Visa routes are not available")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	route, err := h.service.GetByID(r.Context(), id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "route_not_found", "Visa route not found")
			return
		}
		slog.Error("visaroutes: get by id failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get visa route")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": route})
}

// ---------------------------------------------------------------------------
// Admin handlers (visa_routes.manage permission checked at route
// registration)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/visa-routes — list all visa routes
// (any status). Supports ?dest=&status=&active=&category=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_routes_unavailable", "Visa routes not configured")
		return
	}

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	status := r.URL.Query().Get("status")
	visaCategory := r.URL.Query().Get("category")
	q := r.URL.Query().Get("q")

	var activePtr *bool
	if v := r.URL.Query().Get("active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			activePtr = &b
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	routes, total, err := h.service.AdminList(r.Context(), tenantID, destSpaceID, status, visaCategory, q, activePtr, page, limit)
	if err != nil {
		slog.Error("visaroutes: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list visa routes")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": routes,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminCreate handles POST /admin/visa-routes — create a new visa
// route.
func (h *Handlers) HandleAdminCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_routes_unavailable", "Visa routes not configured")
		return
	}

	var input CreateRouteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}
	if input.VisaCategory == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_visa_category", "visa_category is required")
		return
	}
	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}
	if input.GovSourceURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_gov_source_url", "gov_source_url is required")
		return
	}

	route, err := h.service.Create(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("visaroutes: admin create failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create visa route")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": route})
}

// HandleAdminUpdate handles PATCH /admin/visa-routes/:id — update an
// existing visa route. Only provided fields are updated.
func (h *Handlers) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_routes_unavailable", "Visa routes not configured")
		return
	}

	var input UpdateRouteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	route, err := h.service.Update(r.Context(), id, tenantID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "route_not_found", "Visa route not found")
			return
		}
		slog.Error("visaroutes: admin update failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update visa route")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": route})
}

// HandleAdminApprove handles PATCH /admin/visa-routes/:id/approve — mark a
// visa route as HITL-reviewed and published. Optional body: {"notes":"..."}
func (h *Handlers) HandleAdminApprove(w http.ResponseWriter, r *http.Request) {
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_routes_unavailable", "Visa routes not configured")
		return
	}

	var input ApproveInput
	// Body is optional; ignore decode errors for empty bodies.
	_ = json.NewDecoder(r.Body).Decode(&input)

	if err := h.service.Approve(r.Context(), id, tenantID, user.UserID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "route_not_found", "Visa route not found")
			return
		}
		slog.Error("visaroutes: admin approve failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve visa route")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"approved": true}})
}

// HandleAdminGenerate handles POST /admin/visa-routes/generate — Content
// Agent draft placeholder. Admin-only (visa_routes.manage permission checked
// at route registration). Returns 202 Accepted with a placeholder response
// indicating the generation has been queued (§90, ADR-088).
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_routes_disabled", "Visa routes are not available")
		return
	}

	var input GenerateRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination_space_id", "destination_space_id is required")
		return
	}

	result, err := h.service.GenerateDraft(r.Context(), tenantID, input)
	if err != nil {
		slog.Error("visaroutes: admin generate failed", "error", err, "user_id", user.UserID)
		errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to queue draft generation")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": result})
}

// HandleAdminDelete handles DELETE /admin/visa-routes/:id — soft-deactivate
// a visa route (sets is_active = false, does not hard-delete).
func (h *Handlers) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_routes_unavailable", "Visa routes not configured")
		return
	}

	if err := h.service.Deactivate(r.Context(), id, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "route_not_found", "Visa route not found")
			return
		}
		slog.Error("visaroutes: admin delete failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to deactivate visa route")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deactivated": true}})
}
