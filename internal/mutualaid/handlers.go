// Package mutualaid — handlers.go provides the HTTP handlers for the Mutual
// Aid Board routes (§85, ADR-093).
//
// Routes:
//
//	GET    /api/v1/mutual-aid                     — list ?type=&category=&dest=&city=&page=&limit=
//	GET    /api/v1/mutual-aid/:id                 — single post
//	POST   /api/v1/mutual-aid                     — create post (auth, mutual_aid.create_*)
//	PATCH  /api/v1/mutual-aid/:id                 — edit own post (auth)
//	POST   /api/v1/mutual-aid/:id/respond         — open DM thread (auth) → 202
//	POST   /api/v1/mutual-aid/:id/fulfill         — mark fulfilled (auth)
//	POST   /api/v1/mutual-aid/:id/renew           — renew 30 days (auth)
//	DELETE /api/v1/mutual-aid/:id                 — delete own post (auth)
//	GET    /api/v1/admin/mutual-aid               — moderation queue (auth, mutual_aid.moderate)
//	PATCH  /api/v1/admin/mutual-aid/:id/status    — moderate (auth, mutual_aid.moderate)
//
// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
//      japaconnect-engineering/adr/ADR-093-mutual-aid-board.md
package mutualaid

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

// Handlers exposes the mutual aid board endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates mutual aid handlers bound to the provided service.
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
		slog.Error("mutualaid: encode response", "error", err)
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

func scanOptionalUUIDParam(r *http.Request, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if v := r.URL.Query().Get(name); v != "" {
		if err := id.Scan(v); err != nil {
			return pgtype.UUID{}, err
		}
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// Public handlers (list + single — anonymous browse per §85)
// ---------------------------------------------------------------------------

// HandleList handles GET /mutual-aid — list active posts with filters.
// Query params: ?type=&category=&dest=&city=&page=&limit=
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "mutual_aid_disabled", "Mutual aid board is not available")
		return
	}

	postType := r.URL.Query().Get("type")
	category := r.URL.Query().Get("category")

	destSpaceID, err := scanOptionalUUIDParam(r, "dest")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
		return
	}
	citySpaceID, err := scanOptionalUUIDParam(r, "city")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid city space ID")
		return
	}

	page, limit := parsePageParams(r)

	posts, total, err := h.service.List(r.Context(), tenantID, postType, category, destSpaceID, citySpaceID, page, limit)
	if err != nil {
		slog.Error("mutualaid: list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list posts")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": posts,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGet handles GET /mutual-aid/:id — single active post.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "mutual_aid_disabled", "Mutual aid board is not available")
		return
	}

	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	post, err := h.service.Get(r.Context(), postID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}
		slog.Error("mutualaid: get failed", "id", postID, "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get post")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": post})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (create, update, respond, fulfill, renew, delete)
// ---------------------------------------------------------------------------

// HandleCreate handles POST /mutual-aid — create a new post. Auth required
// (mutual_aid.create_offer | create_need | create_skill_swap permission
// checked at route registration based on post_type).
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
		writeError(w, http.StatusNotFound, "mutual_aid_disabled", "Mutual aid board is not available")
		return
	}

	var input CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if input.DestinationSpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}
	if input.PostType == "" {
		writeError(w, http.StatusBadRequest, "missing_type", "post_type is required")
		return
	}

	post, err := h.service.Create(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		slog.Error("mutualaid: create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create post")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": post})
}

// HandleUpdate handles PATCH /mutual-aid/:id — edit own post. Auth required
// (mutual_aid.* permission checked at route registration).
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "mutual_aid_disabled", "Mutual aid board is not available")
		return
	}

	var input UpdatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	post, err := h.service.UpdateOwn(r.Context(), postID, tenantID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only edit your own posts")
			return
		}
		slog.Error("mutualaid: update failed", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update post")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": post})
}

// HandleRespond handles POST /mutual-aid/:id/respond — open a DM thread with
// the post author. Auth required. Returns 202 Accepted; the actual DM thread
// creation is a placeholder pending the messaging subsystem.
func (h *Handlers) HandleRespond(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "mutual_aid_disabled", "Mutual aid board is not available")
		return
	}

	post, err := h.service.Get(r.Context(), postID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}
		slog.Error("mutualaid: respond get failed", "error", err)
		writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get post")
		return
	}

	if post.AuthorID == user.UserID.String() {
		writeError(w, http.StatusBadRequest, "cannot_respond_own", "You cannot respond to your own post")
		return
	}

	// Placeholder: DM thread creation will be wired to the messaging
	// subsystem once available. For now we acknowledge the request.
	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"accepted":   true,
			"post_id":    post.ID,
			"author_id":  post.AuthorID,
			"thread_id":  nil,
		},
	})
}

// HandleFulfill handles POST /mutual-aid/:id/fulfill — mark own post as
// fulfilled. Auth required.
func (h *Handlers) HandleFulfill(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "mutual_aid_disabled", "Mutual aid board is not available")
		return
	}

	if err := h.service.MarkFulfilled(r.Context(), postID, tenantID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only fulfill your own posts")
			return
		}
		slog.Error("mutualaid: fulfill failed", "error", err)
		writeError(w, http.StatusInternalServerError, "fulfill_failed", "Failed to mark post fulfilled")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"fulfilled": true}})
}

// HandleRenew handles POST /mutual-aid/:id/renew — extend a post's expiry by
// 30 days. Auth required (author only).
func (h *Handlers) HandleRenew(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "mutual_aid_disabled", "Mutual aid board is not available")
		return
	}

	post, err := h.service.Renew(r.Context(), postID, tenantID, user.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only renew your own posts")
			return
		}
		slog.Error("mutualaid: renew failed", "error", err)
		writeError(w, http.StatusInternalServerError, "renew_failed", "Failed to renew post")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": post})
}

// HandleDelete handles DELETE /mutual-aid/:id — delete own post. Auth
// required. Soft-delete per Rule 11.
func (h *Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context(), tenantID) {
		writeError(w, http.StatusNotFound, "mutual_aid_disabled", "Mutual aid board is not available")
		return
	}

	if err := h.service.DeleteOwn(r.Context(), postID, tenantID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}
		if errors.Is(err, ErrNotOwner) {
			writeError(w, http.StatusForbidden, "not_owner", "You can only delete your own posts")
			return
		}
		slog.Error("mutualaid: delete failed", "error", err)
		writeError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete post")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// ---------------------------------------------------------------------------
// Admin handlers (moderation queue + status update — mutual_aid.moderate)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/mutual-aid — moderation queue listing
// all posts (any status). Admin-only (mutual_aid.moderate permission checked
// at route registration). Supports ?type=&status=&dest=&city=&q=&page=&limit=.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "mutual_aid_unavailable", "Mutual aid service not configured")
		return
	}

	postType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	destSpaceID, err := scanOptionalUUIDParam(r, "dest")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
		return
	}
	citySpaceID, err := scanOptionalUUIDParam(r, "city")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid city space ID")
		return
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	posts, total, err := h.service.AdminList(r.Context(), tenantID, postType, status, q, destSpaceID, citySpaceID, page, limit)
	if err != nil {
		slog.Error("mutualaid: admin list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list posts")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": posts,
		"pagination": map[string]any{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleAdminModerate handles PATCH /admin/mutual-aid/:id/status — update a
// post's status (moderation action). Admin-only (mutual_aid.moderate
// permission checked at route registration).
func (h *Handlers) HandleAdminModerate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "mutual_aid_unavailable", "Mutual aid service not configured")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if body.Status == "" {
		writeError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	if err := h.service.AdminModerate(r.Context(), postID, tenantID, body.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}
		slog.Error("mutualaid: admin moderate failed", "error", err)
		writeError(w, http.StatusInternalServerError, "moderate_failed", "Failed to moderate post")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": body.Status}})
}
