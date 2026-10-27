// Package roadmap — HTTP handlers for the public product roadmap (§113).
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Feature-flag
// gating and permission enforcement are handled by middleware wired at the
// router level.
//
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
package roadmap

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds dependencies for roadmap HTTP handlers.
type Handlers struct {
	Service  *Service
	SiteURL  string // set at wiring time — used by the releases RSS feed
	SiteName string
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, errors.New("invalid " + param)
	}

	return id, nil
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate
// HTTP response. Roadmap-specific sentinels are checked first, then the
// shared errpkg set handles the rest (not_found → 404,
// invalid_status_transition → 409, feature_disabled → 404, …).
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAlreadyVoted):
		errpkg.WriteError(w, http.StatusConflict, "already_voted", "You have already voted on this item")
	case errors.Is(err, ErrNotVoted):
		errpkg.WriteError(w, http.StatusConflict, "not_voted", "You have not voted on this item")
	case errors.Is(err, ErrUnknownFlagKey):
		errpkg.WriteError(w, http.StatusBadRequest, "unknown_feature_flag", "feature_flag_key does not match a known feature flag")
	case errors.Is(err, ErrFlagAlreadyLinked):
		errpkg.WriteError(w, http.StatusConflict, "flag_already_linked", "Another roadmap item is already linked to this feature flag")
	case errors.Is(err, ErrItemNotShipped):
		errpkg.WriteError(w, http.StatusConflict, "item_not_shipped", "Only shipped roadmap items can be attached to a release")
	case errors.Is(err, ErrInvalidReleaseVersion):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_version", "Mobile releases must use semver (e.g. v2.4.0)")
	case errors.Is(err, ErrInvalidCommitType):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_commit_type", "Unknown commit_type")
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return DefaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	if n > math.MaxInt32 {
		return math.MaxInt32
	}

	// #nosec G109 // value bounded by MaxInt32 check
	return int32(n)
}

// adminListFilter reads the shared admin DataTable query parameters.
func adminListFilter(r *http.Request) (AdminListFilter, error) {
	q := r.URL.Query()

	f := AdminListFilter{
		Status:         q.Get("status"),
		Search:         q.Get("q"),
		Sort:           q.Get("sort"),
		IncludeDeleted: q.Get("include_deleted") == "true",
		PageSize:       parsePageSize(r),
		PageOffset:     parsePageSizeOffset(r),
	}

	if cid := q.Get("category_id"); cid != "" {
		if err := f.CategoryID.Scan(cid); err != nil {
			return AdminListFilter{}, errors.New("invalid category_id")
		}
	}

	return f, nil
}

// ---------------------------------------------------------------------------
// Public handlers
// ---------------------------------------------------------------------------

// GetBoard handles GET /api/v1/roadmap/board.
// OptionalAuth route — has_voted reflects the viewer when authenticated.
func (h *Handlers) GetBoard(w http.ResponseWriter, r *http.Request) {
	var viewerID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		viewerID = user.UserID
	}

	board, err := h.Service.GetBoard(r.Context(), viewerID, r.URL.Query().Get("category"))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": board})
}

// GetChangelog handles GET /api/v1/roadmap/changelog.
func (h *Handlers) GetChangelog(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.GetChangelog(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// GetItem handles GET /api/v1/roadmap/items/{slug}.
func (h *Handlers) GetItem(w http.ResponseWriter, r *http.Request) {
	var viewerID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		viewerID = user.UserID
	}

	item, err := h.Service.GetItem(r.Context(), chi.URLParam(r, "slug"), viewerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// handleVote runs a vote mutation (cast or remove) for the authenticated
// user and responds with the item's updated vote count.
func (h *Handlers) handleVote(w http.ResponseWriter, r *http.Request, vote func(context.Context, pgtype.UUID, pgtype.UUID) (int32, error)) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	count, err := vote(r.Context(), id, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"vote_count": count}})
}

// CastVote handles POST /api/v1/roadmap/items/{id}/vote.
func (h *Handlers) CastVote(w http.ResponseWriter, r *http.Request) {
	h.handleVote(w, r, h.Service.CastVote)
}

// RemoveVote handles DELETE /api/v1/roadmap/items/{id}/vote.
func (h *Handlers) RemoveVote(w http.ResponseWriter, r *http.Request) {
	h.handleVote(w, r, h.Service.RemoveVote)
}

// CreateSubmission handles POST /api/v1/roadmap/submissions.
func (h *Handlers) CreateSubmission(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Title      string `json:"title"`
		Summary    string `json:"summary"`
		Body       string `json:"body"`
		CategoryID string `json:"category_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var categoryID pgtype.UUID
	if input.CategoryID != "" {
		if err := categoryID.Scan(input.CategoryID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_category_id", "Invalid category_id format")
			return
		}
	}

	item, err := h.Service.CreateSubmission(r.Context(), SubmissionInput{
		Title:      input.Title,
		Summary:    input.Summary,
		Body:       input.Body,
		CategoryID: categoryID,
		UserID:     user.UserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": item})
}

// ---------------------------------------------------------------------------
// Admin item handlers
// ---------------------------------------------------------------------------

// AdminListItems handles GET /api/v1/admin/roadmap/items.
func (h *Handlers) AdminListItems(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	f, err := adminListFilter(r)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_category_id", "Invalid category_id format")
		return
	}

	items, err := h.Service.AdminListItems(r.Context(), f)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// AdminListSubmissions handles GET /api/v1/admin/roadmap/submissions —
// the pending triage queue (AdminListItems hardcoded to status=pending).
func (h *Handlers) AdminListSubmissions(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	f, err := adminListFilter(r)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_category_id", "Invalid category_id format")
		return
	}

	items, err := h.Service.AdminListSubmissions(r.Context(), f)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// adminItemByID runs an admin operation that takes an item ID from the URL
// and returns the item row (GET detail, restore).
func (h *Handlers) adminItemByID(w http.ResponseWriter, r *http.Request, fn func(context.Context, pgtype.UUID) (gen.RoadmapItem, error)) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	item, err := fn(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// AdminGetItem handles GET /api/v1/admin/roadmap/items/{id}.
func (h *Handlers) AdminGetItem(w http.ResponseWriter, r *http.Request) {
	h.adminItemByID(w, r, h.Service.AdminGetItem)
}

// adminItemRequest is the JSON body shared by item create and update.
// Pointer fields distinguish "absent" from "set to zero/empty" for PATCH.
type adminItemRequest struct {
	Title          *string `json:"title"`
	Slug           *string `json:"slug"`
	Summary        *string `json:"summary"`
	Body           *string `json:"body"`
	CategoryID     *string `json:"category_id"`
	Status         *string `json:"status"`
	SortOrder      *int32  `json:"sort_order"`
	IsFlagship     *bool   `json:"is_flagship"`
	TargetWindow   *string `json:"target_window"`
	FeatureFlagKey *string `json:"feature_flag_key"`
	ReleaseID      *string `json:"release_id"`
}

// toInput converts the decoded request into a service-layer input.
// A non-nil empty category_id string means "clear the category".
func (in adminItemRequest) toInput(actorID pgtype.UUID) (AdminItemInput, error) {
	out := AdminItemInput{ActorID: actorID}

	if in.Title != nil {
		out.Title = *in.Title
	}

	if in.Slug != nil {
		out.Slug = *in.Slug
	}

	out.Summary = in.Summary
	out.Body = in.Body

	if in.CategoryID != nil {
		var categoryID pgtype.UUID
		if *in.CategoryID != "" {
			if err := categoryID.Scan(*in.CategoryID); err != nil {
				return AdminItemInput{}, errors.New("invalid category_id")
			}
		}

		out.CategoryID = &categoryID
	}

	if in.Status != nil {
		out.Status = *in.Status
	}

	out.SortOrder = in.SortOrder
	out.IsFlagship = in.IsFlagship
	out.TargetWindow = in.TargetWindow
	out.FeatureFlagKey = in.FeatureFlagKey
	out.ReleaseID = in.ReleaseID

	return out, nil
}

// AdminCreateItem handles POST /api/v1/admin/roadmap/items.
func (h *Handlers) AdminCreateItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input adminItemRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == nil || *input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	in, err := input.toInput(user.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_category_id", "Invalid category_id format")
		return
	}

	item, err := h.Service.AdminCreateItem(r.Context(), in)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": item})
}

// AdminUpdateItem handles PATCH /api/v1/admin/roadmap/items/{id}.
func (h *Handlers) AdminUpdateItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	var input adminItemRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	in, err := input.toInput(user.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_category_id", "Invalid category_id format")
		return
	}

	item, err := h.Service.AdminUpdateItem(r.Context(), id, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// AdminSetStatus handles PATCH /api/v1/admin/roadmap/items/{id}/status —
// drives the lifecycle state machine (§113).
func (h *Handlers) AdminSetStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	var input struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	item, err := h.Service.AdminSetStatus(r.Context(), id, ItemStatus(input.Status), user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// AdminDeleteItem handles DELETE /api/v1/admin/roadmap/items/{id}.
// Soft delete always; ?force=true permanently removes an already-soft-
// deleted row — the admin.hard_delete gate is rbac.RequireForceDelete
// middleware on the route (Rule 11).
func (h *Handlers) AdminDeleteItem(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	if err := h.Service.AdminDeleteItem(r.Context(), id, r.URL.Query().Get("force") == "true"); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// AdminRestoreItem handles POST /api/v1/admin/roadmap/items/{id}/restore.
func (h *Handlers) AdminRestoreItem(w http.ResponseWriter, r *http.Request) {
	h.adminItemByID(w, r, h.Service.AdminRestoreItem)
}

// AdminReorderItems handles POST /api/v1/admin/roadmap/items/reorder.
func (h *Handlers) AdminReorderItems(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Items []struct {
			ID        string `json:"id"`
			SortOrder int32  `json:"sort_order"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Items) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_items", "items is required")
		return
	}

	ids := make([]pgtype.UUID, 0, len(input.Items))
	orders := make([]int32, 0, len(input.Items))

	for _, it := range input.Items {
		var id pgtype.UUID
		if err := id.Scan(it.ID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID in items")
			return
		}

		ids = append(ids, id)
		orders = append(orders, it.SortOrder)
	}

	if err := h.Service.AdminReorderItems(r.Context(), ids, orders); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Admin category handlers
// ---------------------------------------------------------------------------

// AdminListCategories handles GET /api/v1/admin/roadmap/categories.
func (h *Handlers) AdminListCategories(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	cats, err := h.Service.AdminListCategories(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if cats == nil {
		cats = make([]gen.RoadmapCategory, 0)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// AdminCreateCategory handles POST /api/v1/admin/roadmap/categories.
func (h *Handlers) AdminCreateCategory(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Slug        string  `json:"slug"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
		SortOrder   *int32  `json:"sort_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	cat, err := h.Service.AdminCreateCategory(r.Context(), CategoryInput{
		Slug:        input.Slug,
		Name:        input.Name,
		Description: input.Description,
		Color:       input.Color,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": cat})
}

// AdminUpdateCategory handles PATCH /api/v1/admin/roadmap/categories/{id}.
func (h *Handlers) AdminUpdateCategory(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid category ID")
		return
	}

	var input struct {
		Slug        *string `json:"slug"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
		SortOrder   *int32  `json:"sort_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var slug, name string
	if input.Slug != nil {
		slug = *input.Slug
	}

	if input.Name != nil {
		name = *input.Name
	}

	cat, err := h.Service.AdminUpdateCategory(r.Context(), id, CategoryInput{
		Slug:        slug,
		Name:        name,
		Description: input.Description,
		Color:       input.Color,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cat})
}

// AdminDeleteCategory handles DELETE /api/v1/admin/roadmap/categories/{id}.
func (h *Handlers) AdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid category ID")
		return
	}

	if err := h.Service.AdminDeleteCategory(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
