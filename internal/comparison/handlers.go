// Package comparison — HTTP handlers for the Generic Comparison System.
// // Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package comparison

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
)

const defaultPageSize int32 = 20

// Handlers holds dependencies for comparison HTTP handlers.
type Handlers struct {
	Service *Service
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
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
}

// parseComparisonItemID extracts the item identifier from either the canonical
// route (DELETE /comparisons/items/{id}) or the catalog alias
// (DELETE /comparisons/{id}/items/{itemId}). The alias is registered in
// server.go and must work for the same handler.
func parseComparisonItemID(r *http.Request) (pgtype.UUID, error) {
	if itemID := chi.URLParam(r, "itemId"); itemID != "" {
		return parseUUIDParam(r, "itemId")
	}

	return parseUUIDParam(r, "id")
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

// hasAnyPermission checks if the authenticated user has the given permission
// via the RBAC resolver stored in the request context by the middleware.
// Returns false if the resolver is not in the context (e.g. the route was
// not wrapped in RequirePermission/RequireAnyPermission middleware).
func hasAnyPermission(r *http.Request, user *auth.AuthenticatedUser, permKey string) bool {
	resolver, ok := r.Context().Value(rbac.ResolverKey).(*rbac.RBAC)
	if !ok || resolver == nil {
		return false
	}

	return resolver.HasPermission(r.Context(), user.UserID, permKey)
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// verifyOwnership is a helper that calls the service's ownership verification
// and writes the appropriate error response if the user is not the author.
// Returns true if the caller should abort (error written), false if ownership
// is confirmed. Ref: §5.3 RBAC escape, BUG-003.
func (h *Handlers) verifyOwnership(w http.ResponseWriter, _ *http.Request, _ *auth.AuthenticatedUser, verify func() error) bool {
	if err := verify(); err != nil {
		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only modify your own comparisons")
			return true
		}

		handleServiceError(w, err)

		return true
	}

	return false
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > int64(MaxPageSize) {
		return MaxPageSize
	}

	return int32(n) // #nosec G115 -- bounded by MaxPageSize and ParseInt bitSize 32
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) // #nosec G115 -- non-negative and bounded by ParseInt bitSize 32
}

// ---------------------------------------------------------------------------
// Comparison Type Definition Handlers
// ---------------------------------------------------------------------------

// CreateComparisonType handles POST /api/v1/comparisons/types.
func (h *Handlers) CreateComparisonType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	var input struct {
		Name         string          `json:"name"`
		Slug         string          `json:"slug"`
		Description  string          `json:"description"`
		Icon         string          `json:"icon"`
		FieldGroupID string          `json:"field_group_id"`
		ItemLabel    string          `json:"item_label"`
		Config       json.RawMessage `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	if input.FieldGroupID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field_group_id", "field_group_id is required")
		return
	}

	var fieldGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id")
			return
		}
	}

	itemLabel := input.ItemLabel
	if itemLabel == "" {
		itemLabel = "Item"
	}

	result, err := h.Service.CreateComparisonType(r.Context(), CreateComparisonTypeInput{
		Name:         input.Name,
		Slug:         input.Slug,
		Description:  input.Description,
		Icon:         input.Icon,
		FieldGroupID: fieldGroupID,
		ItemLabel:    itemLabel,
		Config:       input.Config,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ListComparisonTypes handles GET /api/v1/comparisons/types.
func (h *Handlers) ListComparisonTypes(w http.ResponseWriter, r *http.Request) {

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	types, err := h.Service.ListComparisonTypesWithCounts(r.Context(), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// ListComparisons handles GET /api/v1/comparisons — a public page of
// published, publicly-visible comparison content rows for the tenant,
// optionally filtered by type and space_id (Section 30, BUG-007/BUG-008).
// No authentication required; the SQL layer filters to status='published'
// AND visibility='public' so drafts/private comparisons are never exposed.
func (h *Handlers) ListComparisons(w http.ResponseWriter, r *http.Request) {

	filter, err := h.buildListComparisonsFilter(r)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	comparisons, err := h.Service.ListComparisons(r.Context(), filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": comparisons})
}

// buildListComparisonsFilter parses query parameters into a ListComparisonsFilter.
// It resolves a slug-based type filter to a type_id.
func (h *Handlers) buildListComparisonsFilter(r *http.Request) (ListComparisonsFilter, error) {
	q := r.URL.Query()

	var filter ListComparisonsFilter
	filter.Limit = parsePageSize(r)
	filter.Offset = parsePageSizeOffset(r)

	// type filter: accept either type_id (uuid) or type (slug) and resolve to id.
	if v := q.Get("type_id"); v != "" {
		if err := filter.TypeID.Scan(v); err != nil {
			return filter, fmt.Errorf("%w: invalid type_id", errpkg.ErrInvalid)
		}
	} else if v := q.Get("type"); v != "" {
		td, err := h.Service.GetComparisonTypeBySlug(r.Context(), v)
		if err != nil {
			return filter, err
		}

		filter.TypeID = td.ID
	}

	if v := q.Get("space_id"); v != "" {
		if err := filter.SpaceID.Scan(v); err != nil {
			return filter, fmt.Errorf("%w: invalid space_id", errpkg.ErrInvalid)
		}
	}

	filter.Depth = q.Get("depth")
	filter.Status = q.Get("status")
	filter.Visibility = q.Get("visibility")

	return filter, nil
}

// ListComparisonsAdmin handles GET /api/v1/comparisons/admin.
// // Returns all non-deleted comparison content rows for the tenant (including
// drafts and private comparisons), optionally filtered by type and space_id.
// Requires the comparison.edit_any permission (Section 30).
func (h *Handlers) ListComparisonsAdmin(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	filter, err := h.buildListComparisonsFilter(r)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	comparisons, err := h.Service.ListComparisonsAdmin(r.Context(), filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": comparisons})
}

// GetComparisonType handles GET /api/v1/comparisons/types/{slug}.
func (h *Handlers) GetComparisonType(w http.ResponseWriter, r *http.Request) {

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	result, err := h.Service.GetComparisonTypeDetail(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// UpdateComparisonType handles PATCH /api/v1/comparisons/types/{id}.
func (h *Handlers) UpdateComparisonType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	var input struct {
		Name         string          `json:"name"`
		Slug         string          `json:"slug"`
		Description  string          `json:"description"`
		Icon         string          `json:"icon"`
		FieldGroupID string          `json:"field_group_id"`
		ItemLabel    string          `json:"item_label"`
		Config       json.RawMessage `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	var fieldGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id")
			return
		}
	}

	result, err := h.Service.UpdateComparisonType(r.Context(), id, UpdateComparisonTypeInput{
		Name:         input.Name,
		Slug:         input.Slug,
		Description:  input.Description,
		Icon:         input.Icon,
		FieldGroupID: fieldGroupID,
		ItemLabel:    input.ItemLabel,
		Config:       input.Config,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteComparisonType handles DELETE /api/v1/comparisons/types/{id}.
func (h *Handlers) DeleteComparisonType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	if err := h.Service.DeleteComparisonType(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Comparison Type Access Handlers
// ---------------------------------------------------------------------------

// SetComparisonTypeAccess handles PUT /api/v1/comparisons/types/{id}/access.
func (h *Handlers) SetComparisonTypeAccess(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	typeID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	var input struct {
		UserGroupID string `json:"user_group_id"`
		CanCreate   bool   `json:"can_create"`
		CanFill     bool   `json:"can_fill"`
		AICanFill   bool   `json:"ai_can_fill"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Empty user_group_id is intentionally left as pgtype.UUID{Valid:false}
	// so NULL is stored — Section 30: "NULL = AI agent only".
	var userGroupID pgtype.UUID
	if input.UserGroupID != "" {
		if err := userGroupID.Scan(input.UserGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_group_id", "Invalid user_group_id")
			return
		}
	}

	if err := h.Service.SetComparisonTypeAccess(r.Context(), typeID, SetComparisonTypeAccessInput{
		UserGroupID: userGroupID,
		CanCreate:   input.CanCreate,
		CanFill:     input.CanFill,
		AICanFill:   input.AICanFill,
	}); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// ListComparisonTypeAccess handles GET /api/v1/comparisons/types/{id}/access.
func (h *Handlers) ListComparisonTypeAccess(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	typeID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	access, err := h.Service.ListComparisonTypeAccess(r.Context(), typeID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": access})
}

// ---------------------------------------------------------------------------
// Comparison CRUD Handlers
// ---------------------------------------------------------------------------

// CreateComparison handles POST /api/v1/comparisons.
func (h *Handlers) CreateComparison(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	var input struct {
		Title      string `json:"title"`
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
		Status     string `json:"status"`
		SpaceID    string `json:"space_id"`
		TypeID     string `json:"type_id"`
		Items      []struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"items"`
		Dimensions []string `json:"dimensions"`
		// Optional journey targeting for the backing content row (§14 / §71).
		JourneyStage  *string `json:"journey_stage"`
		VisaRoute     *string `json:"visa_route"`
		SourceCountry *string `json:"source_country"`
		TargetCountry *string `json:"target_country"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var spaceID pgtype.UUID
	if input.SpaceID != "" {
		if err := spaceID.Scan(input.SpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}
	}

	var typeID pgtype.UUID
	if input.TypeID != "" {
		if err := typeID.Scan(input.TypeID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
			return
		}
	}

	items := make([]ComparisonItemInput, 0, len(input.Items))

	for _, it := range input.Items {
		if it.Name == "" {
			continue
		}

		items = append(items, ComparisonItemInput{Name: it.Name, Slug: it.Slug})
	}

	result, err := h.Service.CreateComparison(r.Context(), user.UserID, CreateComparisonInput{
		Title:         input.Title,
		Body:          input.Body,
		Visibility:    input.Visibility,
		Status:        input.Status,
		SpaceID:       spaceID,
		TypeID:        typeID,
		Items:         items,
		Dimensions:    input.Dimensions,
		JourneyStage:  input.JourneyStage,
		VisaRoute:     input.VisaRoute,
		SourceCountry: input.SourceCountry,
		TargetCountry: input.TargetCountry,
	})
	if err != nil {
		if errors.Is(err, content.ErrMissingPrimarySpace) {
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "missing_primary_space", "Content requires a primary space before publishing or scheduling")
			return
		}

		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// GetComparison handles GET /api/v1/comparisons/{id}. The {id} segment may
// be either a content UUID or a slug (Section 30 —
// GET /api/v1/comparisons/:slug). Public for published and publicly-visible
// comparisons; unpublished or non-public comparisons require the authenticated
// user to be the author.
func (h *Handlers) GetComparison(w http.ResponseWriter, r *http.Request) {

	contentID, err := h.Service.ResolveComparisonContentID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	detail, err := h.Service.GetComparisonDetail(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enforce visibility: published+public is open to all; anything else
	// requires the requester to be the author.
	if detail.Status != "published" || detail.Visibility != "public" {
		user := auth.FromContext(r.Context())
		if user == nil || (detail.AuthorID.Valid && user.UserID != detail.AuthorID) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Comparison not found")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": detail})
}

// UpdateComparison handles PATCH /api/v1/comparisons/{id} — updates the
// comparison's metadata (title/slug/body/visibility) on the backing content
// row. Previously this route was wired to UpdateComparisonItem, which treated
// {id} as an item id and corrupted comparison items.
// // Ref: japaconnect-engineering/sections/30-generic-comparison-system.md.
func (h *Handlers) UpdateComparison(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	var input struct {
		Title      string `json:"title"`
		Slug       string `json:"slug"`
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	result, err := h.Service.UpdateComparison(r.Context(), id, UpdateComparisonInput{
		Title:      input.Title,
		Slug:       input.Slug,
		Body:       input.Body,
		Visibility: input.Visibility,
		EditedBy:   user.UserID,
	}, hasAnyPermission(r, user, "comparison.edit_any"))
	if err != nil {
		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only update your own comparisons")
			return
		}

		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteComparison handles DELETE /api/v1/comparisons/{id} — soft-deletes the
// backing content row. Previously this route was wired to DeleteComparisonItem,
// which hard-deleted a comparison item instead of the comparison itself.
// // Ref: japaconnect-engineering/sections/30-generic-comparison-system.md.
func (h *Handlers) DeleteComparison(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	if err := h.Service.DeleteComparison(r.Context(), id, user.UserID, hasAnyPermission(r, user, "comparison.delete_any")); err != nil {
		if errors.Is(err, ErrNotOwner) {
			errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only delete your own comparisons")
			return
		}

		handleServiceError(w, err)

		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// GetComparisonMatrix handles GET /api/v1/comparisons/{id}/matrix. The {id}
// segment may be either a content UUID or a slug (Section 30). Public for
// published and publicly-visible comparisons; unpublished or non-public
// comparisons require the authenticated user to be the author.
func (h *Handlers) GetComparisonMatrix(w http.ResponseWriter, r *http.Request) {

	contentID, err := h.Service.ResolveComparisonContentID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Verify visibility before returning the matrix. Fetching the detail is
	// one extra query but avoids exposing private/draft matrices to guests.
	detail, err := h.Service.GetComparisonDetail(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if detail.Status != "published" || detail.Visibility != "public" {
		user := auth.FromContext(r.Context())
		if user == nil || (detail.AuthorID.Valid && user.UserID != detail.AuthorID) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Comparison not found")
			return
		}
	}

	matrix, err := h.Service.GetComparisonMatrix(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": matrix})
}

// ---------------------------------------------------------------------------
// Comparison Item Handlers
// ---------------------------------------------------------------------------

// CreateComparisonItem handles POST /api/v1/comparisons/{id}/items.
func (h *Handlers) CreateComparisonItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	if h.verifyOwnership(w, r, user, func() error {
		return h.Service.VerifyComparisonOwnership(r.Context(), contentID, user.UserID, hasAnyPermission(r, user, "comparison.edit_any"))
	}) {
		return
	}

	var input struct {
		Title       string          `json:"title"`
		Description string          `json:"description"`
		ImageID     string          `json:"image_id"`
		GeoSpaceID  string          `json:"geo_space_id"`
		Position    int32           `json:"position"`
		ItemRef     json.RawMessage `json:"item_ref"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var imageID, geoSpaceID pgtype.UUID
	if input.ImageID != "" {
		if err := imageID.Scan(input.ImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_image_id", "Invalid image ID")
			return
		}
	}

	if input.GeoSpaceID != "" {
		if err := geoSpaceID.Scan(input.GeoSpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo space ID")
			return
		}
	}

	item, err := h.Service.CreateComparisonItem(r.Context(), CreateComparisonItemInput{
		ContentID:   contentID,
		Title:       input.Title,
		Description: input.Description,
		ImageID:     imageID,
		GeoSpaceID:  geoSpaceID,
		Position:    input.Position,
		ItemRef:     []byte(input.ItemRef),
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": item})
}

// UpdateComparisonItem handles PATCH /api/v1/comparisons/items/{id}.
func (h *Handlers) UpdateComparisonItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseComparisonItemID(r)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	if h.verifyOwnership(w, r, user, func() error {
		return h.Service.VerifyComparisonItemOwnership(r.Context(), id, user.UserID, hasAnyPermission(r, user, "comparison.edit_any"))
	}) {
		return
	}

	var input struct {
		Title       string          `json:"title"`
		Description string          `json:"description"`
		ImageID     string          `json:"image_id"`
		GeoSpaceID  string          `json:"geo_space_id"`
		Position    int32           `json:"position"`
		ItemRef     json.RawMessage `json:"item_ref"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var imageID, geoSpaceID pgtype.UUID
	if input.ImageID != "" {
		if err := imageID.Scan(input.ImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_image_id", "Invalid image ID")
			return
		}
	}

	if input.GeoSpaceID != "" {
		if err := geoSpaceID.Scan(input.GeoSpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo space ID")
			return
		}
	}

	item, err := h.Service.UpdateComparisonItem(r.Context(), id, UpdateComparisonItemInput{
		Title:       input.Title,
		Description: input.Description,
		ImageID:     imageID,
		GeoSpaceID:  geoSpaceID,
		Position:    input.Position,
		ItemRef:     []byte(input.ItemRef),
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// deleteOwnedResource deletes a comparison sub-resource after auth/ownership checks.
// It factors out the identical auth, parse, ownership, and delete flow used by
// DeleteComparisonItem and DeleteComparisonDimension so the code does not dupl.
func (h *Handlers) deleteOwnedResource(
	w http.ResponseWriter,
	r *http.Request,
	invalidIDMsg string,
	getID func(*http.Request) (pgtype.UUID, error),
	verify func(ctx context.Context, id, userID pgtype.UUID, bypass bool) error,
	deleteFn func(ctx context.Context, id pgtype.UUID) error,
) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := getID(r)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", invalidIDMsg)
		return
	}

	if h.verifyOwnership(w, r, user, func() error {
		return verify(r.Context(), id, user.UserID, hasAnyPermission(r, user, "comparison.edit_any"))
	}) {
		return
	}

	if err := deleteFn(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// DeleteComparisonItem handles DELETE /api/v1/comparisons/items/{id}
// and the catalog alias DELETE /api/v1/comparisons/{id}/items/{itemId}.
func (h *Handlers) DeleteComparisonItem(w http.ResponseWriter, r *http.Request) {
	h.deleteOwnedResource(w, r, "Invalid item ID", parseComparisonItemID,
		h.Service.VerifyComparisonItemOwnership,
		h.Service.DeleteComparisonItem)
}

// ---------------------------------------------------------------------------
// Comparison Dimension Handlers
// ---------------------------------------------------------------------------

// CreateComparisonDimension handles POST /api/v1/comparisons/{id}/dimensions.
func (h *Handlers) CreateComparisonDimension(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	if h.verifyOwnership(w, r, user, func() error {
		return h.Service.VerifyComparisonOwnership(r.Context(), contentID, user.UserID, hasAnyPermission(r, user, "comparison.edit_any"))
	}) {
		return
	}

	var input struct {
		Label         string `json:"label"`
		DimensionType string `json:"dimension_type"`
		Position      int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_label", "label is required")
		return
	}

	if input.DimensionType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_dimension_type", "dimension_type is required")
		return
	}

	if !IsValidDimensionType(input.DimensionType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_dimension_type", "Invalid dimension type")
		return
	}

	dim, err := h.Service.CreateComparisonDimension(r.Context(), CreateComparisonDimensionInput{
		ContentID:     contentID,
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": dim})
}

// UpdateComparisonDimension handles PATCH /api/v1/comparisons/dimensions/{id}.
func (h *Handlers) UpdateComparisonDimension(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dimension ID")
		return
	}

	if h.verifyOwnership(w, r, user, func() error {
		return h.Service.VerifyComparisonDimensionOwnership(r.Context(), id, user.UserID, hasAnyPermission(r, user, "comparison.edit_any"))
	}) {
		return
	}

	var input struct {
		Label         string `json:"label"`
		DimensionType string `json:"dimension_type"`
		Position      int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_label", "label is required")
		return
	}

	if !IsValidDimensionType(input.DimensionType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_dimension_type", "Invalid dimension type")
		return
	}

	dim, err := h.Service.UpdateComparisonDimension(r.Context(), id, UpdateComparisonDimensionInput{
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": dim})
}

// DeleteComparisonDimension handles DELETE /api/v1/comparisons/dimensions/{id}.
func (h *Handlers) DeleteComparisonDimension(w http.ResponseWriter, r *http.Request) {
	h.deleteOwnedResource(w, r, "Invalid dimension ID",
		func(r *http.Request) (pgtype.UUID, error) { return parseUUIDParam(r, "id") },
		h.Service.VerifyComparisonDimensionOwnership,
		h.Service.DeleteComparisonDimension)
}

// ---------------------------------------------------------------------------
// Comparison Value Handlers
// ---------------------------------------------------------------------------

// UpsertComparisonValues handles PUT /api/v1/comparisons/dimensions/{id}/values.
func (h *Handlers) UpsertComparisonValues(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	dimensionID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dimension ID")
		return
	}

	if h.verifyOwnership(w, r, user, func() error {
		return h.Service.VerifyComparisonDimensionOwnership(r.Context(), dimensionID, user.UserID, hasAnyPermission(r, user, "comparison.edit_any"))
	}) {
		return
	}

	var input struct {
		Values []struct {
			ItemID       string   `json:"item_id"`
			Value        string   `json:"value"`
			ValueRating  *float64 `json:"value_rating"`
			ValueBoolean *bool    `json:"value_boolean"`
		} `json:"values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Batch upsert: one round-trip for all values (BUG-027).
	inputs := make([]UpsertComparisonValueInput, 0, len(input.Values))

	for _, v := range input.Values {
		var itemID pgtype.UUID
		if err := itemID.Scan(v.ItemID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_item_id", "Invalid item_id")
			return
		}

		inputs = append(inputs, UpsertComparisonValueInput{
			DimensionID:  dimensionID,
			ItemID:       itemID,
			Value:        v.Value,
			ValueRating:  v.ValueRating,
			ValueBoolean: v.ValueBoolean,
		})
	}

	results, err := h.Service.BatchUpsertComparisonValues(r.Context(), dimensionID, inputs)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": results})
}

// ReorderComparisonItems handles POST /api/v1/comparisons/{id}/items/reorder
// — bulk position update across a comparison's items.
// // Request body (Section 30 contract): { "ordered_item_ids": ["uuid", ...] }
// The position of each item is derived from its index in the array (0-based).
// The legacy { "items": [{ "id", "position" }] } shape is still accepted for
// backward compatibility but is deprecated.
// //nolint:gocyclo // Supports both deprecated legacy body and canonical ordered_item_ids body.
func (h *Handlers) ReorderComparisonItems(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	comparisonID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	if h.verifyOwnership(w, r, user, func() error {
		return h.Service.VerifyComparisonOwnership(r.Context(), comparisonID, user.UserID, hasAnyPermission(r, user, "comparison.edit_any"))
	}) {
		return
	}

	// Accept both the canonical Section 30 shape ({ordered_item_ids:[]}) and
	// the legacy shape ({items:[{id,position}]}) for backward compatibility.
	var input struct {
		OrderedItemIDs []string `json:"ordered_item_ids"`
		Items          []struct {
			ID       string `json:"id"`
			Position int32  `json:"position"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.OrderedItemIDs) == 0 && len(input.Items) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_reorder", "ordered_item_ids (or items) must not be empty")
		return
	}

	// Canonical path: derive positions from array index.
	if len(input.OrderedItemIDs) > 0 {
		ids := make([]pgtype.UUID, 0, len(input.OrderedItemIDs))
		positions := make([]int32, 0, len(input.OrderedItemIDs))

		for idx, id := range input.OrderedItemIDs {
			var itemID pgtype.UUID
			if err := itemID.Scan(id); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_item_id", "Invalid item id: "+id)
				return
			}

			ids = append(ids, itemID)
			positions = append(positions, toInt32(idx))
		}

		if err := h.Service.BatchReorderComparisonItems(r.Context(), comparisonID, ids, positions); err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})

		return
	}

	// Legacy path: explicit positions per item.
	ids := make([]pgtype.UUID, 0, len(input.Items))
	positions := make([]int32, 0, len(input.Items))

	for _, item := range input.Items {
		var itemID pgtype.UUID
		if err := itemID.Scan(item.ID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_item_id", "Invalid item id: "+item.ID)
			return
		}

		ids = append(ids, itemID)
		positions = append(positions, item.Position)
	}

	if err := h.Service.BatchReorderComparisonItems(r.Context(), comparisonID, ids, positions); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
}

// GetDimensionTypes handles GET /api/v1/comparisons/dimension-types — returns
// the valid dimension types supported by the comparison system (Section 30).
// The frontend uses this to populate the dimension type selector in the
// ComparisonBuilder instead of hardcoding the values (Rule 16).
// // Ref: japaconnect-engineering/sections/30-generic-comparison-system.md.
func (h *Handlers) GetDimensionTypes(w http.ResponseWriter, _ *http.Request) {
	types := []map[string]string{
		{"value": DimensionTypeText, "label": "Text"},
		{"value": DimensionTypeRating, "label": "Rating"},
		{"value": DimensionTypeBoolean, "label": "Boolean"},
		{"value": DimensionTypeNumber, "label": "Number"},
		{"value": DimensionTypeCurrency, "label": "Currency"},
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// AutoGenerateDimensions handles POST /api/v1/comparisons/auto-dimensions.
// Given a list of item names, it returns a set of suggested comparison
// dimensions derived from a configurable heuristic (no AI tokens). The
// frontend presents these as editable defaults in the comparison creation
// wizard (Phase 16.05).
// // Ref: japaconnect-engineering/sections/16-comparisons.md (Phase 16.05 — HITL).
func (h *Handlers) AutoGenerateDimensions(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	var input struct {
		Items []string `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Items) < 2 {
		errpkg.WriteError(w, http.StatusBadRequest, "insufficient_items", "At least 2 items are required")
		return
	}

	dimensions, err := h.Service.SuggestDimensions(r.Context(), input.Items)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"dimensions": dimensions}})
}
