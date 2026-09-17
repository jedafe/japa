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
//
//	japaconnect-engineering/adr/ADR-093-mutual-aid-board.md
package mutualaid

import (
	"context"
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
	errpkg "github.com/jedafe/japa/internal/errors"
)

// permissionChecker is the minimal RBAC interface needed for in-handler
// permission checks (post_type-specific create nodes). Kept as an interface
// so the handler stays testable with a stub.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// pointsChecker returns the user's lifetime points (reputation) for the
// reputation gate on need posts (§85: create_need is reputation-gated).
type pointsChecker interface {
	GetPointsBalance(ctx context.Context, userID pgtype.UUID) (int64, error)
}

// DMConversationResult is the minimal return from dmInitiator — just the
// conversation ID and whether it was newly created. Avoids importing the
// websocket package (which would create an import cycle).
type DMConversationResult struct {
	ConversationID pgtype.UUID
	IsNew          bool
}

// dmInitiator opens or creates a DM conversation and sends the initial
// message. Used by HandleRespond to wire the §48 DM system (ADR-093).
// Implemented by an adapter around websocket.DMService.
type dmInitiator interface {
	GetOrCreateConversationForInitiator(ctx context.Context, initiatorID, recipientID pgtype.UUID, previewMessage string) (DMConversationResult, error)
	SendMessage(ctx context.Context, conversationID, senderID pgtype.UUID, body string) error
}

// Handlers exposes the mutual aid board endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	perms    permissionChecker
	points   pointsChecker
	dm       dmInitiator
}

// NewHandlers creates mutual aid handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// SetPermissionChecker injects the RBAC resolver for post_type-specific
// permission checks (create_need, create_skill_swap). Called once during
// server wiring.
func (h *Handlers) SetPermissionChecker(pc permissionChecker) {
	h.perms = pc
}

// SetPointsChecker injects the wallet service for reputation checks on
// need posts. Called once during server wiring.
func (h *Handlers) SetPointsChecker(pc pointsChecker) {
	h.points = pc
}

// SetDMInitiator injects the DM service for the respond handler. Called
// once during server wiring.
func (h *Handlers) SetDMInitiator(dm dmInitiator) {
	h.dm = dm
}

// permKeyForPostType maps a post_type to its RBAC node per §85.
func permKeyForPostType(postType string) string {
	switch postType {
	case PostTypeNeed:
		return "mutual_aid.create_need"
	case PostTypeOffer:
		return "mutual_aid.create_offer"
	case PostTypeSkillSwap:
		return "mutual_aid.create_skill_swap"
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as story/handlers.go)
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
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}
	// Accept both "limit" and "page_size" — the frontend sends page_size
	// (BUG-008: param mismatch).
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// parseSortParam returns the sort key from the query string. "latest" (default)
// sorts by created_at DESC; "nearest" is reserved for geo-distance sorting
// (not yet implemented — falls back to latest).
func parseSortParam(r *http.Request) string {
	v := r.URL.Query().Get("sort")
	if v == "" {
		return "latest"
	}

	return v
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

// HandleCategories handles GET /mutual-aid/categories — returns the canonical
// category option list (Rule 16).
func (h *Handlers) HandleCategories(w http.ResponseWriter, r *http.Request) {

	cats := h.service.Categories()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// HandlePostTypes handles GET /mutual-aid/post-types — returns the canonical
// post type option list (Rule 16).
func (h *Handlers) HandlePostTypes(w http.ResponseWriter, r *http.Request) {

	types := h.service.PostTypes()
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// HandleList handles GET /mutual-aid — list active posts with filters.
// Query params: ?type=&category=&dest=&city=&page=&limit=.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "Mutual aid service is not available")
		return
	}

	postType := r.URL.Query().Get("type")
	category := r.URL.Query().Get("category")

	destSpaceID, err := scanOptionalUUIDParam(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
		return
	}

	citySpaceID, err := scanOptionalUUIDParam(r, "city")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid city space ID")
		return
	}

	page, limit := parsePageParams(r)

	posts, total, err := h.service.List(r.Context(), postType, category, destSpaceID, citySpaceID, page, limit)
	if err != nil {
		slog.Error("mutualaid: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list posts")

		return
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}

	// Geo-aware tool pattern response envelope: { data, meta: { page,
	// per_page, total, total_pages } }. The frontend apiList helper reads
	// `meta` (not `pagination`), so emitting `meta` is required for the
	// pagination nav to render. BUG-006.
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": posts,
		"meta": map[string]any{
			"page":        page,
			"per_page":    limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// HandleGet handles GET /mutual-aid/:id — single active post.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "Mutual aid service is not available")
		return
	}

	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	post, err := h.service.Get(r.Context(), postID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}

		slog.Error("mutualaid: get failed", "id", postID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get post")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": post})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (create, update, respond, fulfill, renew, delete)
// ---------------------------------------------------------------------------

// HandleCreate handles POST /mutual-aid — create a new post. Auth required.
// The route middleware checks mutual_aid.create_offer as a baseline gate;
// the handler then checks the post_type-specific permission (create_need,
// create_skill_swap) and the reputation gate for need posts (§85).
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "Mutual aid service is not available")
		return
	}

	var input CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "Destination space ID is required")
		return
	}

	if input.PostType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_type", "post_type is required")
		return
	}

	if input.Category == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}
	// BUG-014: validate category against the §85 enum before hitting the
	// DB CHECK constraint — otherwise an invalid value surfaces as a raw
	// 500 instead of a structured 400.
	if !IsValidCategory(input.Category) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_category", "Invalid category")
		return
	}

	if input.Description == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_description", "Description is required")
		return
	}

	// Post_type-specific RBAC check (BUG-001): the route middleware checks
	// create_offer as a baseline; here we check the specific node for the
	// requested post_type.
	permKey := permKeyForPostType(input.PostType)
	if permKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_type", "Invalid post_type")
		return
	}

	if permKey != "mutual_aid.create_offer" && h.perms != nil {
		if !h.perms.HasPermission(r.Context(), user.UserID, permKey) {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to create this post type")
			return
		}
	}

	// Reputation gate for need posts (BUG-002): §85 specifies create_need
	// is "member with reputation ≥ threshold".
	if input.PostType == PostTypeNeed && h.points != nil {
		threshold := h.service.NeedReputationThreshold(r.Context())
		if threshold > 0 {
			points, err := h.points.GetPointsBalance(r.Context(), user.UserID)
			if err != nil {
				slog.Error("mutualaid: reputation check failed", "error", err)
				errpkg.WriteError(w, http.StatusInternalServerError, "reputation_check_failed", "Failed to verify reputation")

				return
			}

			if points < threshold {
				errpkg.WriteError(w, http.StatusForbidden, "insufficient_reputation", "You need more reputation to post a need")
				return
			}
		}
	}

	post, err := h.service.Create(r.Context(), user.UserID, input)
	if err != nil {
		slog.Error("mutualaid: create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create post")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": post})
}

// HandleUpdate handles PATCH /mutual-aid/:id — edit own post. Auth required
// (mutual_aid.* permission checked at route registration).
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "Mutual aid service is not available")
		return
	}

	var input UpdatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// BUG-014: validate category against the §85 enum when provided.
	if input.Category != nil && *input.Category != "" && !IsValidCategory(*input.Category) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_category", "Invalid category")
		return
	}

	post, err := h.service.UpdateOwn(r.Context(), postID, user.UserID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only edit your own posts")
			return
		}

		slog.Error("mutualaid: update failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update post")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": post})
}

// HandleRespond handles POST /mutual-aid/:id/respond — open a DM thread with
// the post author. Auth required. Returns 200 with the conversation_id, or
// 202 if the DM subsystem is not configured (graceful degradation).
func (h *Handlers) HandleRespond(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "Mutual aid service is not available")
		return
	}

	post, err := h.service.Get(r.Context(), postID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}

		slog.Error("mutualaid: respond get failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get post")

		return
	}

	if post.AuthorID == user.UserID.String() {
		errpkg.WriteError(w, http.StatusBadRequest, "cannot_respond_own", "You cannot respond to your own post")
		return
	}

	// Parse optional message body.
	var req RespondRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Wire the DM system (BUG-003): create or get conversation with the
	// post author and send the initial message. Per §85 responses go
	// through DM (§48) rather than public comments.
	if h.dm == nil {
		// Graceful degradation: if DM subsystem is not wired, acknowledge.
		errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{
			"data": map[string]any{
				"accepted":  true,
				"post_id":   post.ID,
				"author_id": post.AuthorID,
			},
		})

		return
	}

	var authorID pgtype.UUID
	if err := authorID.Scan(post.AuthorID); err != nil {
		slog.Error("mutualaid: parse author id failed", "author_id", post.AuthorID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to parse author ID")

		return
	}

	// Send the initial message if one was provided.
	message := req.Message
	if message == "" {
		message = "I'm responding to your mutual aid post: " + post.Title
	}

	result, err := h.dm.GetOrCreateConversationForInitiator(r.Context(), user.UserID, authorID, message)
	if err != nil {
		slog.Error("mutualaid: dm create failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "dm_failed", "Failed to open conversation")

		return
	}

	if err := h.dm.SendMessage(r.Context(), result.ConversationID, user.UserID, message); err != nil {
		slog.Error("mutualaid: dm send failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "dm_send_failed", "Failed to send initial message")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"conversation_id": result.ConversationID.String(),
			"is_new":          result.IsNew,
			"post_id":         post.ID,
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

	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "Mutual aid service is not available")
		return
	}

	if err := h.service.MarkFulfilled(r.Context(), postID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only fulfill your own posts")
			return
		}

		slog.Error("mutualaid: fulfill failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "fulfill_failed", "Failed to mark post fulfilled")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"fulfilled": true}})
}

// HandleRenew handles POST /mutual-aid/:id/renew — extend a post's expiry by
// 30 days. Auth required (author only).
func (h *Handlers) HandleRenew(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "Mutual aid service is not available")
		return
	}

	post, err := h.service.Renew(r.Context(), postID, user.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only renew your own posts")
			return
		}

		slog.Error("mutualaid: renew failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "renew_failed", "Failed to renew post")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": post})
}

// HandleDelete handles DELETE /mutual-aid/:id — delete own post. Auth
// required. Soft-delete per Rule 11.
func (h *Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "Mutual aid service is not available")
		return
	}

	if err := h.service.DeleteOwn(r.Context(), postID, user.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}

		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only delete your own posts")
			return
		}

		slog.Error("mutualaid: delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete post")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
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

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "mutual_aid_unavailable", "Mutual aid service not configured")
		return
	}

	postType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	destSpaceID, err := scanOptionalUUIDParam(r, "dest")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
		return
	}

	citySpaceID, err := scanOptionalUUIDParam(r, "city")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid city space ID")
		return
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	posts, total, err := h.service.AdminList(r.Context(), postType, status, q, destSpaceID, citySpaceID, page, limit)
	if err != nil {
		slog.Error("mutualaid: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list posts")

		return
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}

	// Geo-aware tool pattern admin list envelope: { data, meta: { page,
	// per_page, total, total_pages } }. The admin EntityPage reads
	// `meta.total` / `meta.total_pages` for pagination. BUG-007.
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": posts,
		"meta": map[string]any{
			"page":        page,
			"per_page":    limit,
			"total":       total,
			"total_pages": totalPages,
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

	postID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "mutual_aid_unavailable", "Mutual aid service not configured")
		return
	}

	var body struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	if err := h.service.AdminModerate(r.Context(), postID, body.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "post_not_found", "Post not found")
			return
		}

		slog.Error("mutualaid: admin moderate failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "moderate_failed", "Failed to moderate post")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": body.Status}})
}
