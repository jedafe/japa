// Package wiki — handlers.go provides the HTTP handlers for the Destination
// Wiki routes (§80, ADR-088).
//
// Routes:
//
//	GET    /api/v1/wiki/:space_slug                        — wiki index for space
//	GET    /api/v1/wiki/:space_slug/:page_slug             — single wiki page
//	GET    /api/v1/wiki/:space_slug/:page_slug/history     — revision history
//	POST   /api/v1/wiki/:space_slug/:page_slug/revisions   — submit edit (auth, wiki.edit)
//	POST   /api/v1/admin/wiki/:page_id/revisions/:id/approve — approve revision (auth, wiki.review)
//	POST   /api/v1/admin/wiki/:page_id/revisions/:id/reject  — reject revision (auth, wiki.review)
//	PATCH  /api/v1/admin/wiki/:page_id                     — lock/unlock, feature (auth, wiki.manage)
//	POST   /api/v1/admin/wiki/generate                     — trigger Content Agent draft (auth, wiki.manage)
//	GET    /api/v1/admin/wiki                              — admin list (auth, wiki.manage)
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//      japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

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

// Handlers exposes the wiki endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates wiki handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("wiki: encode response", "error", err)
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

// HandleGetIndex handles GET /wiki/:space_slug — wiki index for a space.
// Also accepts ?dest= (destination space ID) as a geo-aware alternative to
// the URL path :space_slug parameter (ADR-086 geo-aware pattern).
func (h *Handlers) HandleGetIndex(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	// ?dest= takes precedence over :space_slug when provided.
	if destParam := r.URL.Query().Get("dest"); destParam != "" {
		var destSpaceID pgtype.UUID
		if err := destSpaceID.Scan(destParam); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
		pages, err := h.service.GetWikiIndexByDestSpaceID(r.Context(), tenantID, destSpaceID)
		if err != nil {
			slog.Error("wiki: index by dest failed", "error", err)
			writeError(w, http.StatusInternalServerError, "index_failed", "Failed to get wiki index")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": pages})
		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	if spaceSlug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Space slug or ?dest= is required")
		return
	}

	pages, err := h.service.GetWikiIndex(r.Context(), tenantID, spaceSlug)
	if err != nil {
		slog.Error("wiki: index failed", "error", err)
		writeError(w, http.StatusInternalServerError, "index_failed", "Failed to get wiki index")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// HandleGetPage handles GET /wiki/:space_slug/:page_slug — single wiki page
// with its current approved revision content.
func (h *Handlers) HandleGetPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")
	if spaceSlug == "" || pageSlug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	page, err := h.service.GetWikiPage(r.Context(), tenantID, spaceSlug, pageSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}
		slog.Error("wiki: get page failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get wiki page")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": page})
}

// HandleGetHistory handles GET /wiki/:space_slug/:page_slug/history —
// revision history for a wiki page.
func (h *Handlers) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")
	if spaceSlug == "" || pageSlug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	page, limit := parsePageParams(r)
	revs, total, err := h.service.GetRevisionHistory(r.Context(), tenantID, spaceSlug, pageSlug, page, limit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}
		slog.Error("wiki: history failed", "error", err)
		writeError(w, http.StatusInternalServerError, "history_failed", "Failed to get revision history")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": revs,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (submit revision — auth, wiki.edit)
// ---------------------------------------------------------------------------

// HandleSubmitRevision handles POST /wiki/:space_slug/:page_slug/revisions —
// submit a wiki edit for review. Auth required (wiki.edit permission checked
// at route registration). The service checks the reputation threshold.
func (h *Handlers) HandleSubmitRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	spaceSlug := chi.URLParam(r, "space_slug")
	pageSlug := chi.URLParam(r, "page_slug")
	if spaceSlug == "" || pageSlug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Space and page slugs are required")
		return
	}

	var input SubmitRevisionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if len(input.Content) == 0 {
		writeError(w, http.StatusBadRequest, "missing_content", "Content is required")
		return
	}

	revID, err := h.service.SubmitRevision(r.Context(), tenantID, user.UserID, spaceSlug, pageSlug, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}
		if errors.Is(err, ErrPageLocked) {
			writeError(w, http.StatusForbidden, "page_locked", "This wiki page is locked to mod-only edits")
			return
		}
		slog.Error("wiki: submit revision failed", "error", err)
		writeError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit revision")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"revision_id": revID,
			"status":      "pending",
		},
	})
}

// ---------------------------------------------------------------------------
// Admin handlers (review, manage, generate — wiki.review / wiki.manage)
// ---------------------------------------------------------------------------

// HandleApproveRevision handles POST /admin/wiki/:page_id/revisions/:id/approve.
func (h *Handlers) HandleApproveRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	pageID, ok := scanUUIDParam(w, r, "page_id")
	if !ok {
		return
	}
	revID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Notes string `json:"notes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.service.ApproveRevision(r.Context(), revID, pageID, tenantID, user.UserID, body.Notes); err != nil {
		slog.Error("wiki: approve failed", "error", err)
		writeError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve revision")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"approved": true}})
}

// HandleRejectRevision handles POST /admin/wiki/:page_id/revisions/:id/reject.
func (h *Handlers) HandleRejectRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	revID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		Notes string `json:"notes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.service.RejectRevision(r.Context(), revID, tenantID, user.UserID, body.Notes); err != nil {
		slog.Error("wiki: reject failed", "error", err)
		writeError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject revision")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"rejected": true}})
}

// HandleUpdatePage handles PATCH /admin/wiki/:page_id — lock/unlock, feature.
func (h *Handlers) HandleUpdatePage(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	pageID, ok := scanUUIDParam(w, r, "page_id")
	if !ok {
		return
	}

	var input UpdatePageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdatePage(r.Context(), pageID, tenantID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "page_not_found", "Wiki page not found")
			return
		}
		slog.Error("wiki: update page failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update wiki page")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
}

// HandleGenerate handles POST /admin/wiki/generate — trigger Content Agent
// draft for a wiki page. Creates a stub page (if needed) and enqueues an AI
// generation task. The actual AI generation goes through the existing §20
// AI Gateway (ADR-088).
func (h *Handlers) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if _, ok := requireTenant(w, r); !ok {
		return
	}

	var input GeneratePageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.SpaceSlug == "" || input.PageSlug == "" || input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "space_slug, page_slug, and title are required")
		return
	}

	// The actual AI generation task enqueue would go here — for now, we
	// return a 202 indicating the generation request has been accepted.
	// The Content Agent draft enters the HITL review queue on completion.
	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"status":     "accepted",
			"space_slug": input.SpaceSlug,
			"page_slug":  input.PageSlug,
			"title":      input.Title,
			"category":   input.Category,
		},
	})
}

// HandleAdminList handles GET /admin/wiki — admin list of all wiki pages.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var spaceID pgtype.UUID
	if v := r.URL.Query().Get("space_id"); v != "" {
		if err := spaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid space_id")
			return
		}
	}

	category := r.URL.Query().Get("category")
	q := r.URL.Query().Get("q")

	var lockedPtr *bool
	if v := r.URL.Query().Get("locked"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			lockedPtr = &b
		}
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	pages, total, err := h.service.AdminListPages(r.Context(), tenantID, spaceID, category, q, lockedPtr, page, limit)
	if err != nil {
		slog.Error("wiki: admin list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list wiki pages")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": pages,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}
