// Package ecaguide — handlers.go provides the HTTP handlers for the ECA
// Guide routes (§109, ADR-120).
//
// Routes:
//
//	GET   /api/v1/eca/providers                       — ?dest= (public)
//	GET   /api/v1/eca/guides                          — ?dest=&purpose=&provider= (public)
//	GET   /api/v1/eca/guides/:id                      — detail + approved updates (public)
//	GET   /api/v1/eca/compare                         — ?dest=&provider_ids= (public)
//	POST  /api/v1/eca/guides/:id/updates              — community update (auth, eca_guide.update)
//	GET   /api/v1/admin/eca-guides                    — manage guides (admin)
//	POST  /api/v1/admin/eca-guides                    — create guide (admin)
//	POST  /api/v1/admin/eca-guides/generate           — Content Agent draft placeholder (admin)
//	PATCH /api/v1/admin/eca-guides/:id                — update guide (admin)
//	PATCH /api/v1/admin/eca-guides/:id/approve        — HITL approve (admin)
//	GET   /api/v1/admin/eca-guides/updates            — moderation list (admin)
//	PATCH /api/v1/admin/eca-guides/updates/:id        — approve/reject update (admin)
//
// Ref: japaconnect-engineering/sections/109-eca-guide.md
//
//	japaconnect-engineering/adr/ADR-120-eca-guide.md
package ecaguide

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

// Handlers exposes the ECA guide endpoints.
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

// NewHandlers creates ECA handlers bound to the service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
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

func scanDestQuery(w http.ResponseWriter, r *http.Request, required bool) (pgtype.UUID, bool) {
	var id pgtype.UUID
	v := r.URL.Query().Get("dest")
	if v == "" {
		if required {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_dest", "dest query param is required")
			return id, false
		}
		return id, true
	}
	if err := id.Scan(v); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_dest", "Invalid destination space ID")
		return id, false
	}
	return id, true
}

func parsePageParams(r *http.Request) (page, limit int32) {
	page, limit = 1, 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}
	return page, limit
}

func unavailable(w http.ResponseWriter) {
	errpkg.WriteError(w, http.StatusServiceUnavailable, "eca_guide_unavailable", "ECA guide not configured")
}

// HandleListProviders handles GET /eca/providers. Public.
func (h *Handlers) HandleListProviders(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	dest, ok := scanDestQuery(w, r, false)
	if !ok {
		return
	}

	providers, err := h.service.ListProviders(r.Context(), dest)
	if err != nil {
		slog.Error("ecaguide: list providers failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list providers")
		return
	}

	h.resolveLiterals(r, &providers)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// HandleListGuides handles GET /eca/guides. Public.
func (h *Handlers) HandleListGuides(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	dest, ok := scanDestQuery(w, r, false)
	if !ok {
		return
	}

	guides, err := h.service.ListGuides(r.Context(), dest,
		r.URL.Query().Get("purpose"), r.URL.Query().Get("provider"))
	if err != nil {
		slog.Error("ecaguide: list guides failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list guides")
		return
	}

	h.resolveLiterals(r, &guides)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guides})
}

// HandleGetGuide handles GET /eca/guides/:id — detail + approved
// community updates. Public.
func (h *Handlers) HandleGetGuide(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	detail, err := h.service.GetGuide(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Guide not found")
			return
		}
		slog.Error("ecaguide: get guide failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get guide")
		return
	}

	h.resolveLiterals(r, &detail)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": detail})
}

// HandleCompare handles GET /eca/compare — ?dest=&provider_ids=csv. Public.
func (h *Handlers) HandleCompare(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		unavailable(w)
		return
	}

	dest, ok := scanDestQuery(w, r, true)
	if !ok {
		return
	}

	var providerIDs []pgtype.UUID
	for _, raw := range strings.Split(r.URL.Query().Get("provider_ids"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var id pgtype.UUID
		if err := id.Scan(raw); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_provider_id", "Invalid provider id in provider_ids")
			return
		}
		providerIDs = append(providerIDs, id)
	}

	guides, err := h.service.CompareGuides(r.Context(), dest, providerIDs)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "compare_failed", err.Error())
		return
	}

	h.resolveLiterals(r, &guides)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": guides})
}

// HandleCreateUpdate handles POST /eca/guides/:id/updates — member
// community update. Auth required (eca_guide.update at registration).
func (h *Handlers) HandleCreateUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	guideID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	id, err := h.service.CreateCommunityUpdate(r.Context(), guideID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Guide not found")
			return
		}
		errpkg.WriteError(w, http.StatusBadRequest, "create_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"id": id, "status": StatusPending}})
}

// HandleAdminList handles GET /admin/eca-guides.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	dest, ok := scanDestQuery(w, r, false)
	if !ok {
		return
	}

	page, limit := parsePageParams(r)
	guides, total, err := h.service.AdminListGuides(r.Context(), dest, r.URL.Query().Get("status"), page, limit)
	if err != nil {
		slog.Error("ecaguide: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list guides")
		return
	}

	h.resolveLiterals(r, &guides)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       guides,
		"pagination": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleAdminCreate handles POST /admin/eca-guides.
func (h *Handlers) HandleAdminCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	var input GuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	guide, err := h.service.CreateGuide(r.Context(), input)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "create_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": guide})
}

// HandleAdminGenerate handles POST /admin/eca-guides/generate — enqueues
// tool:generate_draft; the Content Agent writes an ai_draft guide for
// HITL review (ADR-088). Body requires destination_space_id + provider_id.
func (h *Handlers) HandleAdminGenerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	var body map[string]string
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	if err := h.service.EnqueueGenerate(r.Context(), body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "generate_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"status": "queued"}})
}

// HandleAdminUpdate handles PATCH /admin/eca-guides/:id.
func (h *Handlers) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input GuideRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdateGuide(r.Context(), id, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Guide not found")
			return
		}
		slog.Error("ecaguide: update failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update guide")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id")}})
}

// HandleAdminApprove handles PATCH /admin/eca-guides/:id/approve.
func (h *Handlers) HandleAdminApprove(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		ReviewNotes string `json:"review_notes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.service.ApproveGuide(r.Context(), id, user.UserID, body.ReviewNotes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Guide not found")
			return
		}
		slog.Error("ecaguide: approve failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve guide")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": StatusPublished}})
}

// HandleAdminListUpdates handles GET /admin/eca-guides/updates — the
// moderation queue.
func (h *Handlers) HandleAdminListUpdates(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	page, limit := parsePageParams(r)
	updates, total, err := h.service.AdminListUpdates(r.Context(), r.URL.Query().Get("status"), page, limit)
	if err != nil {
		slog.Error("ecaguide: admin updates failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list updates")
		return
	}

	h.resolveLiterals(r, &updates)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       updates,
		"pagination": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// HandleAdminModerateUpdate handles PATCH /admin/eca-guides/updates/:id.
func (h *Handlers) HandleAdminModerateUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input ModerateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.ModerateUpdate(r.Context(), id, user.UserID, input.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "update_not_found", "Update not found")
			return
		}
		errpkg.WriteError(w, http.StatusBadRequest, "moderate_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": input.Status}})
}

// HandleAdminDelete handles DELETE /admin/eca-guides/:id — removes a
// non-published guide. Admin-only.
func (h *Handlers) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.DeleteGuide(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Guide not found or is published")
			return
		}
		slog.Error("ecaguide: delete failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete guide")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id")}})
}

// HandleAdminReject handles PATCH /admin/eca-guides/:id/reject — HITL
// reject. Admin-only.
func (h *Handlers) HandleAdminReject(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		ReviewNotes string `json:"review_notes"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	if err := h.service.RejectGuide(r.Context(), id, user.UserID, body.ReviewNotes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "guide_not_found", "Guide not found")
			return
		}
		slog.Error("ecaguide: reject failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject guide")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": StatusRejected}})
}

// HandleAdminListProviders handles GET /admin/eca-providers — all
// providers including inactive. Admin-only.
func (h *Handlers) HandleAdminListProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	providers, err := h.service.AdminListProviders(r.Context())
	if err != nil {
		slog.Error("ecaguide: list providers failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list providers")
		return
	}
	h.resolveLiterals(r, &providers)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// HandleAdminCreateProvider handles POST /admin/eca-providers. Admin-only.
func (h *Handlers) HandleAdminCreateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	var input ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	provider, err := h.service.CreateProvider(r.Context(), input)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "create_failed", err.Error())
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": provider})
}

// HandleAdminUpdateProvider handles PATCH /admin/eca-providers/:id.
// Admin-only.
func (h *Handlers) HandleAdminUpdateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.service.UpdateProvider(r.Context(), id, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "provider_not_found", "Provider not found")
			return
		}
		slog.Error("ecaguide: update provider failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update provider")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id")}})
}

// HandleAdminDeleteProvider handles DELETE /admin/eca-providers/:id —
// removes a provider only when no guides reference it. Admin-only.
func (h *Handlers) HandleAdminDeleteProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	if h.service == nil {
		unavailable(w)
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.DeleteProvider(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusConflict, "provider_in_use", "Provider not found or has guides referencing it")
			return
		}
		slog.Error("ecaguide: delete provider failed", "id", id, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete provider")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": chi.URLParam(r, "id")}})
}
