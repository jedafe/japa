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
	"context"
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
	"github.com/jedafe/japa/internal/literals"
)

// maxCompareIDs is the maximum number of routes the compare endpoint
// accepts (§90).
const maxCompareIDs = 4

// Handlers exposes the visa routes endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	lit      literalResolver // nil disables literal resolution (§14)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// SetLiteralResolver injects the template-literal catalog so user-facing
// titles/descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on a response DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

// NewHandlers creates visa routes handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
// ---------------------------------------------------------------------------

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
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			page = int32(n)
		}
	}

	// ?per_page= is the canonical list param (geo-aware pattern);
	// ?limit= stays accepted for back-compat.
	perPage := r.URL.Query().Get("per_page")
	if perPage == "" {
		perPage = r.URL.Query().Get("limit")
	}

	if perPage != "" {
		if n, err := strconv.ParseInt(perPage, 10, 32); err == nil && n > 0 {
			limit = min(int32(n), 100)
		}
	}

	return page, limit
}

// resolveSpaceParam resolves a ?dest=/?origin=-style query value to a
// space ID via the service (UUID, geo_code, or slug). Writes a 400 and
// returns false when the value doesn't resolve.
func (h *Handlers) resolveSpaceParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	v := r.URL.Query().Get(param)
	if v == "" {
		return pgtype.UUID{}, true
	}

	id, err := h.service.ResolveSpaceID(r.Context(), v)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+param+" value")
		return pgtype.UUID{}, false
	}

	return id, true
}

// writeMutationError maps sentinel service errors to the standard error
// envelope (Rule 5): validation → 400, duplicate slug → 409, anything
// else → 500 with the generic code.
func writeMutationError(w http.ResponseWriter, err error, code, message string, logKV ...any) {
	switch {
	case errors.Is(err, ErrInvalidGovSourceURL):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_gov_source_url", err.Error())
	case errors.Is(err, ErrInvalidInput):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, ErrInvalidStatus):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", err.Error())
	case errors.Is(err, ErrDuplicateSlug):
		errpkg.WriteError(w, http.StatusConflict, "duplicate_slug", err.Error())
	case errors.Is(err, ErrInvalidDestination):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_destination", err.Error())
	default:
		args := append([]any{"error", err}, logKV...)
		slog.Error("visaroutes: mutation failed", args...)
		errpkg.WriteError(w, http.StatusInternalServerError, code, message)
	}
}

// ---------------------------------------------------------------------------
// Public handlers (list, compare, detail — anonymous browse per §90)
// ---------------------------------------------------------------------------

// HandleList handles GET /visa-routes — list published+active visa routes
// with filters. Query params: ?dest=&profession=&education=&experience_years=&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_routes_disabled", "Visa routes are not available")
		return
	}

	// ?dest= accepts a space UUID, a country geo_code ("CA"), or a space
	// slug (geo-aware pattern — destination is the primary axis).
	destSpaceID, ok := h.resolveSpaceParam(w, r, "dest")
	if !ok {
		return
	}

	profession := r.URL.Query().Get("profession")
	education := r.URL.Query().Get("education")
	origin := r.URL.Query().Get("origin")

	var experienceYears, age int32

	// ParseInt(…, 10, 32) rejects out-of-range values — Atoi+int32()
	// silently wraps ("4294967297" → 1) and would apply a bogus filter.
	if v := r.URL.Query().Get("experience_years"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			experienceYears = int32(n)
		}
	}

	if v := r.URL.Query().Get("age"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			age = int32(n)
		}
	}

	page, limit := parsePageParams(r)

	routes, total, err := h.service.ListFiltered(r.Context(), ListQueryParams{
		DestSpaceID:     destSpaceID,
		Profession:      profession,
		Education:       education,
		ExperienceYears: experienceYears,
		Age:             age,
		Origin:          origin,
		Page:            page,
		Limit:           limit,
	})
	if err != nil {
		slog.Error("visaroutes: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list visa routes")

		return
	}

	h.resolveLiterals(r, &routes)

	// Standard list envelope (geo-aware pattern): canonical `meta` plus a
	// `pagination` alias for back-compat with older clients.
	pageMeta := map[string]any{
		"page":        page,
		"per_page":    limit,
		"total":       total,
		"total_pages": (total + int64(limit) - 1) / int64(limit),
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       routes,
		"meta":       pageMeta,
		"pagination": pageMeta,
	})
}

// HandleCompare handles GET /visa-routes/compare — compare up to 4 visa
// routes side-by-side. Query params: ?ids=id1,id2,id3,id4 (max 4).
func (h *Handlers) HandleCompare(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_routes_disabled", "Visa routes are not available")
		return
	}

	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "ids query parameter is required")
		return
	}

	// Optional ?dest= filter (UUID, geo_code, or slug): when present,
	// only routes for that destination space are returned.
	var destFilter pgtype.UUID

	filterByDest := r.URL.Query().Get("dest") != ""

	if filterByDest {
		id, ok := h.resolveSpaceParam(w, r, "dest")
		if !ok {
			return
		}

		destFilter = id
	}

	parts := strings.Split(idsParam, ",")
	if len(parts) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "At least one route id is required")
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

	// The 4-route cap counts parsed ids, not raw parts — a trailing or
	// doubled comma shouldn't reject an otherwise-valid request.
	if len(ids) > maxCompareIDs {
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_ids", "A maximum of 4 route ids can be compared")
		return
	}

	if len(ids) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "At least one route id is required")
		return
	}

	result, err := h.service.CompareWithMissing(r.Context(), ids)
	if err != nil {
		slog.Error("visaroutes: compare failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "compare_failed", "Failed to compare visa routes")

		return
	}

	routes := result.Routes
	missing := result.MissingIDs

	if filterByDest {
		kept := routes[:0]

		var destMissing []string

		for _, route := range routes {
			if route.DestinationSpaceID == destFilter.String() {
				kept = append(kept, route)
			} else {
				destMissing = append(destMissing, route.ID)
			}
		}

		routes = kept

		missing = append(missing, destMissing...)
	}

	h.resolveLiterals(r, &routes)

	// Nothing resolved at all → a real 404, not a 207 with empty data.
	if len(routes) == 0 {
		errpkg.WriteError(w, http.StatusNotFound, "route_not_found", "None of the requested visa routes could be found")
		return
	}

	if len(missing) > 0 {
		// 207 Multi-Status: some requested IDs did not resolve.
		errpkg.WriteJSON(w, http.StatusMultiStatus, map[string]any{"data": routes, "missing_ids": missing})
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": routes})
}

// HandleGet handles GET /visa-routes/:id — single published+active visa
// route detail.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "visa_routes_disabled", "Visa routes are not available")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	route, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "route_not_found", "Visa route not found")
			return
		}

		slog.Error("visaroutes: get by id failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get visa route")

		return
	}

	h.resolveLiterals(r, &route)

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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_routes_unavailable", "Visa routes not configured")
		return
	}

	destSpaceID, ok := h.resolveSpaceParam(w, r, "dest")
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	visaCategory := r.URL.Query().Get("category")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	var activePtr *bool

	if v := r.URL.Query().Get("active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			activePtr = &b
		}
	}

	page, limit := parsePageParams(r)

	routes, total, err := h.service.AdminList(r.Context(), destSpaceID, status, visaCategory, q, sort, order, activePtr, page, limit)
	if err != nil {
		slog.Error("visaroutes: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list visa routes")

		return
	}

	// Canonical `meta` envelope (per_page/total_pages) plus the legacy
	// `pagination` alias — same dual shape as the public list.
	pageMeta := map[string]any{
		"page":        page,
		"per_page":    limit,
		"total":       total,
		"total_pages": (total + int64(limit) - 1) / int64(limit),
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       routes,
		"meta":       pageMeta,
		"pagination": pageMeta,
	})
}

// HandleAdminCreate handles POST /admin/visa-routes — create a new visa
// route.
func (h *Handlers) HandleAdminCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
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

	route, err := h.service.Create(r.Context(), input)
	if err != nil {
		writeMutationError(w, err, "create_failed", "Failed to create visa route", "user_id", user.UserID.String())
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

	route, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "route_not_found", "Visa route not found")
			return
		}

		writeMutationError(w, err, "update_failed", "Failed to update visa route", "id", id.String())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": route})
}

// HandleAdminApprove handles PATCH /admin/visa-routes/:id/approve — mark a
// visa route as HITL-reviewed and published. Optional body: {"notes":"..."}.
func (h *Handlers) HandleAdminApprove(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
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

	route, err := h.service.Approve(r.Context(), id, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "route_not_found", "Visa route not found")
			return
		}

		writeMutationError(w, err, "approve_failed", "Failed to approve visa route", "id", id.String())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": route})
}

// HandleAdminGenerate handles POST /admin/visa-routes/generate — Content
// Agent draft placeholder. Admin-only (visa_routes.manage permission checked
// at route registration). Like the other admin handlers it is NOT gated by the
// public feature flag so admins can populate route data before go-live.
// Returns 202 Accepted with a placeholder response indicating the generation
// has been queued (§90, ADR-088).
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_routes_unavailable", "Visa routes not configured")
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

	result, err := h.service.GenerateDraft(r.Context(), input)
	if err != nil {
		writeMutationError(w, err, "generate_failed", "Failed to queue draft generation", "user_id", user.UserID.String())
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

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "visa_routes_unavailable", "Visa routes not configured")
		return
	}

	if err := h.service.Deactivate(r.Context(), id); err != nil {
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
