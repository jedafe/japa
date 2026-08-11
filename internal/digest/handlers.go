// Package digest — handlers.go provides the HTTP handlers for the Community
// Digest Widget routes (§84, ADR-092).
//
// Routes:
//
//	GET   /api/v1/digest/latest              — latest published digest (public)
//	GET   /api/v1/digest/:id                 — single published digest (public)
//	GET   /api/v1/admin/digest               — admin list (auth, digest.manage)
//	GET   /api/v1/admin/digest/:id           — admin get (auth, digest.manage)
//	PATCH /api/v1/admin/digest/:id           — edit draft (auth, digest.manage)
//	POST  /api/v1/admin/digest/:id/publish   — publish (auth, digest.manage)
//	POST  /api/v1/admin/digest/generate      — trigger generation (auth, digest.manage)
//
// Ref: japaconnect-engineering/sections/84-community-digest.md
//      japaconnect-engineering/adr/ADR-092-community-digest.md
package digest

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

// Handlers exposes the digest endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates digest handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("digest: encode response", "error", err)
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
// Public handlers (browse — anonymous)
// ---------------------------------------------------------------------------

// HandleGetLatest handles GET /digest/latest — latest published digest.
func (h *Handlers) HandleGetLatest(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var spaceID pgtype.UUID
	// Accept ?dest= (geo-aware standard) or ?space_id= (legacy alias).
	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("space_id")
	}
	if destParam != "" {
		if err := spaceID.Scan(destParam); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	edition, err := h.service.GetLatest(r.Context(), tenantID, spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no_digest", "No published digest available")
			return
		}
		slog.Error("digest: get latest failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_latest_failed", "Failed to get latest digest")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": edition})
}

// HandleGet handles GET /digest/:id — single published digest (public).
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	edition, err := h.service.Get(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "digest_not_found", "Digest edition not found")
			return
		}
		slog.Error("digest: get failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get digest edition")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": edition})
}

// ---------------------------------------------------------------------------
// Admin handlers (manage, generate — digest.manage)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/digest — admin list of all editions.
// Supports ?status=, ?space_id=, ?sort=, ?order=, ?page=, ?limit= query params
// (§84 admin pattern: filter + sort + paginate, no infinite scroll).
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	// Parse filter/sort params.
	f := AdminListFilter{
		TenantID: tenantID,
		Status:   r.URL.Query().Get("status"),
		Sort:     r.URL.Query().Get("sort"),
		Order:    r.URL.Query().Get("order"),
	}
	if spaceParam := r.URL.Query().Get("space_id"); spaceParam != "" {
		if err := f.SpaceID.Scan(spaceParam); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid space_id")
			return
		}
	}

	editions, total, err := h.service.AdminList(r.Context(), f, page, limit)
	if err != nil {
		slog.Error("digest: admin list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list digest editions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": editions,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminGet handles GET /admin/digest/:id — admin get single edition.
func (h *Handlers) HandleAdminGet(w http.ResponseWriter, r *http.Request) {
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

	edition, err := h.service.AdminGet(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "digest_not_found", "Digest edition not found")
			return
		}
		slog.Error("digest: admin get failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_get_failed", "Failed to get digest edition")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": edition})
}

// HandleAdminUpdate handles PATCH /admin/digest/:id — edit draft content.
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

	var input UpdateEditionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(input.Content) == 0 {
		writeError(w, http.StatusBadRequest, "missing_content", "Content is required")
		return
	}

	if err := h.service.AdminUpdate(r.Context(), tenantID, id, input.Content); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "digest_not_found", "Digest edition not found or not editable")
			return
		}
		slog.Error("digest: admin update failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update digest edition")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
}

// HandleAdminPublish handles POST /admin/digest/:id/publish — publish edition.
func (h *Handlers) HandleAdminPublish(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.AdminPublish(r.Context(), tenantID, user.UserID, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "digest_not_found", "Digest edition not found or not publishable")
			return
		}
		slog.Error("digest: admin publish failed", "error", err)
		writeError(w, http.StatusInternalServerError, "publish_failed", "Failed to publish digest edition")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"published": true}})
}

// HandleAdminGenerate handles POST /admin/digest/generate — trigger digest
// generation. Enqueues the digest:generate Asynq task that assembles a draft
// edition from platform analytics (no AI generation — purely metrics-driven,
// ADR-092). The edition enters the 1-hour admin review window on completion.
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	status, err := h.service.AdminGenerate(r.Context(), tenantID)
	if err != nil {
		slog.Error("digest: admin generate failed", "error", err)
		writeError(w, http.StatusInternalServerError, "generate_failed", "Failed to trigger digest generation")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status": status,
		},
	})
}
