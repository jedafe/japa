package federation

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds dependencies for the federation HTTP handlers.
type Handlers struct {
	service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// writeTypedJSON writes an AP/JRD document with its protocol content type
// (the public federation surface does not use the API error envelope for
// 2xx — remote servers parse by content type).
func writeTypedJSON(w http.ResponseWriter, status int, contentType string, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Body may be partially written; log path — the handler that calls
		// this already committed the status.
		_ = err
	}
}

// handleServiceErr maps service sentinels to the standard error envelope.
func handleServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, ErrHandleDomain):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_resource", "Unsupported resource")
	case errors.Is(err, ErrInvalidDomain):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "invalid_domain", "Invalid instance domain")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Internal error")
	}
}

// ---------------------------------------------------------------------------
// Public surface (§126 URL Layout — unversioned /federation/* + /.well-known)
// ---------------------------------------------------------------------------

// GetWebfinger serves GET /.well-known/webfinger?resource=acct:{u}@{domain}.
func (h *Handlers) GetWebfinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_resource", "resource parameter is required")
		return
	}
	doc, err := h.service.Webfinger(r.Context(), resource)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeJRD, doc)
}

// GetWellKnownNodeInfo serves GET /.well-known/nodeinfo.
func (h *Handlers) GetWellKnownNodeInfo(w http.ResponseWriter, r *http.Request) {
	writeTypedJSON(w, http.StatusOK, "application/json", h.service.WellKnownNodeInfo())
}

// GetNodeInfo serves GET /federation/nodeinfo/2.1.
func (h *Handlers) GetNodeInfo(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.NodeInfo(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeNodeInfo, doc)
}

// GetActor serves GET /federation/actors/{username}.
func (h *Handlers) GetActor(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.ActorDocument(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetOutbox serves GET /federation/actors/{username}/outbox[?page=N].
func (h *Handlers) GetOutbox(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.Outbox(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetFollowers serves GET /federation/actors/{username}/followers[?page=N].
func (h *Handlers) GetFollowers(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.Followers(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetFollowing serves GET /federation/actors/{username}/following[?page=N].
func (h *Handlers) GetFollowing(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.Following(r.Context(), chi.URLParam(r, "username"), parsePage(r))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// GetObject serves GET /federation/objects/contents/{id}.
func (h *Handlers) GetObject(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid object id")
		return
	}
	doc, err := h.service.ObjectDocument(r.Context(), id)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, ContentTypeActivity, doc)
}

// ---------------------------------------------------------------------------
// Member opt-in (/api/v1/me/federation — auth + federation.use)
// ---------------------------------------------------------------------------

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// GetMyFederation serves GET /api/v1/me/federation.
func (h *Handlers) GetMyFederation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.GetMyFederation(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// EnableMyFederation serves POST /api/v1/me/federation.
func (h *Handlers) EnableMyFederation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.EnableFederation(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// DisableMyFederation serves DELETE /api/v1/me/federation.
func (h *Handlers) DisableMyFederation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.DisableFederation(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// ---------------------------------------------------------------------------
// Admin (/api/v1/admin/federation/*)
// ---------------------------------------------------------------------------

// actorSortKeys is the DataTable sort whitelist for the admin actors list —
// each key has a CASE arm in ListFederatedActors (unlisted sorts fall back
// to created_at DESC server-side).
var actorSortKeys = map[string]bool{
	"created_at": true,
	"username":   true,
	"status":     true,
	"actor_url":  true,
}

// ListActors serves GET /api/v1/admin/federation/actors.
func (h *Handlers) ListActors(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := parseLimitOffset(r)
	sortKey := r.URL.Query().Get("sort")
	if !actorSortKeys[sortKey] {
		sortKey = "created_at"
	}
	sortDir := r.URL.Query().Get("dir")
	if sortDir == "" {
		sortDir = r.URL.Query().Get("sort_dir")
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}
	rows, total, err := h.service.ListActors(r.Context(), r.URL.Query().Get("q"), sortKey, sortDir, limit, offset)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]any{"total": total, "page": page, "total_pages": totalPages(total, limit)},
	})
}

// GetStats serves GET /api/v1/admin/federation/stats.
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.Stats(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, stats)
}

// ListBlocks serves GET /api/v1/admin/federation/instances.
func (h *Handlers) ListBlocks(w http.ResponseWriter, r *http.Request) {
	limit, offset, page := parseLimitOffset(r)
	rows, total, err := h.service.ListBlocks(r.Context(), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]any{"total": total, "page": page, "total_pages": totalPages(total, limit)},
	})
}

// CreateBlock serves POST /api/v1/admin/federation/instances.
func (h *Handlers) CreateBlock(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Domain string `json:"domain"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	block, err := h.service.CreateBlock(r.Context(), body.Domain, body.Reason, user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, block)
}

// DeleteBlock serves DELETE /api/v1/admin/federation/instances/{id}.
func (h *Handlers) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid block id")
		return
	}
	if err := h.service.DeleteBlock(r.Context(), id); err != nil {
		handleServiceErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func parsePage(r *http.Request) int {
	raw := r.URL.Query().Get("page")
	// Mastodon's outbox walker uses ?page=true for the first page.
	if raw == "true" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// parseLimitOffset resolves the admin DataTable contract (?page=&limit=;
// ?offset= still honored for older callers) into limit + row offset + the
// 1-based page number echoed back in meta.
func parseLimitOffset(r *http.Request) (limit, offset, page int32) {
	limit = 20
	if v, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32); err == nil && v > 0 && v <= 100 {
		limit = int32(v)
	}
	// Explicit offset wins; page= maps to (page-1)*limit per the repo's
	// DataTable convention (page is 1-based).
	if v, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32); err == nil && v >= 0 {
		offset = int32(v)
		return limit, offset, offset/limit + 1
	}
	page = 1
	// Bound page — (page-1)*limit must not overflow int32.
	if v, err := strconv.ParseInt(r.URL.Query().Get("page"), 10, 32); err == nil && v > 1 {
		if v > 1_000_000 {
			v = 1_000_000
		}
		page = int32(v)
	}
	return limit, (page - 1) * limit, page
}

func totalPages(total int64, limit int32) int32 {
	if limit <= 0 || total <= 0 {
		return 1
	}
	return int32((total + int64(limit) - 1) / int64(limit))
}
