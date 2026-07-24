// Package comparison — HTTP handlers for the Generic Comparison System.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package comparison

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
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

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
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

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Comparison Type Definition Handlers
// ---------------------------------------------------------------------------

// CreateComparisonType handles POST /api/v1/comparison/types.
func (h *Handlers) CreateComparisonType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		Description  string `json:"description"`
		Icon         string `json:"icon"`
		FieldGroupID string `json:"field_group_id"`
		ItemLabel    string `json:"item_label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.Slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	var fieldGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id")
			return
		}
	}

	itemLabel := input.ItemLabel
	if itemLabel == "" {
		itemLabel = "Item"
	}

	result, err := h.Service.CreateComparisonType(r.Context(), tenantID, CreateComparisonTypeInput{
		Name:         input.Name,
		Slug:         input.Slug,
		Description:  input.Description,
		Icon:         input.Icon,
		FieldGroupID: fieldGroupID,
		ItemLabel:    itemLabel,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ListComparisonTypes handles GET /api/v1/comparison/types.
func (h *Handlers) ListComparisonTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	types, err := h.Service.ListComparisonTypes(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// ListComparisons handles GET /api/v1/comparisons — a page of comparison
// content rows for the tenant. A type_slug query parameter is accepted for
// forward compatibility but not applied: comparison content carries no
// comparison-type linkage in the schema.
func (h *Handlers) ListComparisons(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	comparisons, err := h.Service.ListComparisons(r.Context(), tenantID, parsePageSize(r))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": comparisons})
}

// GetComparisonType handles GET /api/v1/comparison/types/{slug}.
func (h *Handlers) GetComparisonType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	result, err := h.Service.GetComparisonTypeBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// UpdateComparisonType handles PATCH /api/v1/comparison/types/{id}.
func (h *Handlers) UpdateComparisonType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
		ItemLabel   string `json:"item_label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	result, err := h.Service.UpdateComparisonType(r.Context(), id, tenantID, UpdateComparisonTypeInput{
		Name:        input.Name,
		Description: input.Description,
		Icon:        input.Icon,
		ItemLabel:   input.ItemLabel,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteComparisonType handles DELETE /api/v1/comparison/types/{id}.
func (h *Handlers) DeleteComparisonType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	if err := h.Service.DeleteComparisonType(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Comparison Type Access Handlers
// ---------------------------------------------------------------------------

// SetComparisonTypeAccess handles PUT /api/v1/comparison/types/{id}/access.
func (h *Handlers) SetComparisonTypeAccess(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	typeID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	var input struct {
		UserGroupID string `json:"user_group_id"`
		CanCreate   bool   `json:"can_create"`
		CanFill     bool   `json:"can_fill"`
		AICanFill   bool   `json:"ai_can_fill"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var userGroupID pgtype.UUID
	if input.UserGroupID != "" {
		if err := userGroupID.Scan(input.UserGroupID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_user_group_id", "Invalid user_group_id")
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// ListComparisonTypeAccess handles GET /api/v1/comparison/types/{id}/access.
func (h *Handlers) ListComparisonTypeAccess(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	typeID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	access, err := h.Service.ListComparisonTypeAccess(r.Context(), typeID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": access})
}

// ---------------------------------------------------------------------------
// Comparison CRUD Handlers
// ---------------------------------------------------------------------------

// CreateComparison handles POST /api/v1/comparison.
func (h *Handlers) CreateComparison(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Title      string `json:"title"`
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
		SpaceID    string `json:"space_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var spaceID pgtype.UUID
	if input.SpaceID != "" {
		if err := spaceID.Scan(input.SpaceID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}
	}

	result, err := h.Service.CreateComparison(r.Context(), tenantID, user.UserID, CreateComparisonInput{
		Title:      input.Title,
		Body:       input.Body,
		Visibility: input.Visibility,
		SpaceID:    spaceID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// GetComparison handles GET /api/v1/comparison/{id}.
func (h *Handlers) GetComparison(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	items, err := h.Service.ListComparisonItems(r.Context(), contentID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	dimensions, err := h.Service.ListComparisonDimensions(r.Context(), contentID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":         contentID,
			"items":      items,
			"dimensions": dimensions,
		},
	})
}

// GetComparisonMatrix handles GET /api/v1/comparison/{id}/matrix.
func (h *Handlers) GetComparisonMatrix(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	matrix, err := h.Service.GetComparisonMatrix(r.Context(), contentID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": matrix})
}

// ---------------------------------------------------------------------------
// Comparison Item Handlers
// ---------------------------------------------------------------------------

// CreateComparisonItem handles POST /api/v1/comparison/{id}/items.
func (h *Handlers) CreateComparisonItem(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		ImageID     string `json:"image_id"`
		GeoSpaceID  string `json:"geo_space_id"`
		Position    int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var imageID, geoSpaceID pgtype.UUID
	if input.ImageID != "" {
		_ = imageID.Scan(input.ImageID)
	}

	if input.GeoSpaceID != "" {
		_ = geoSpaceID.Scan(input.GeoSpaceID)
	}

	item, err := h.Service.CreateComparisonItem(r.Context(), tenantID, CreateComparisonItemInput{
		ContentID:   contentID,
		Title:       input.Title,
		Description: input.Description,
		ImageID:     imageID,
		GeoSpaceID:  geoSpaceID,
		Position:    input.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": item})
}

// UpdateComparisonItem handles PATCH /api/v1/comparison/items/{id}.
func (h *Handlers) UpdateComparisonItem(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		ImageID     string `json:"image_id"`
		GeoSpaceID  string `json:"geo_space_id"`
		Position    int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var imageID, geoSpaceID pgtype.UUID
	if input.ImageID != "" {
		_ = imageID.Scan(input.ImageID)
	}

	if input.GeoSpaceID != "" {
		_ = geoSpaceID.Scan(input.GeoSpaceID)
	}

	item, err := h.Service.UpdateComparisonItem(r.Context(), id, tenantID, UpdateComparisonItemInput{
		Title:       input.Title,
		Description: input.Description,
		ImageID:     imageID,
		GeoSpaceID:  geoSpaceID,
		Position:    input.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// DeleteComparisonItem handles DELETE /api/v1/comparison/items/{id}.
func (h *Handlers) DeleteComparisonItem(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	if err := h.Service.DeleteComparisonItem(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Comparison Dimension Handlers
// ---------------------------------------------------------------------------

// CreateComparisonDimension handles POST /api/v1/comparison/{id}/dimensions.
func (h *Handlers) CreateComparisonDimension(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	var input struct {
		Label         string `json:"label"`
		DimensionType string `json:"dimension_type"`
		Position      int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Label == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_label", "label is required")
		return
	}

	if input.DimensionType == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_dimension_type", "dimension_type is required")
		return
	}

	if !IsValidDimensionType(input.DimensionType) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_dimension_type", "Invalid dimension type")
		return
	}

	dim, err := h.Service.CreateComparisonDimension(r.Context(), tenantID, CreateComparisonDimensionInput{
		ContentID:     contentID,
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": dim})
}

// UpdateComparisonDimension handles PATCH /api/v1/comparison/dimensions/{id}.
func (h *Handlers) UpdateComparisonDimension(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dimension ID")
		return
	}

	var input struct {
		Label         string `json:"label"`
		DimensionType string `json:"dimension_type"`
		Position      int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Label == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_label", "label is required")
		return
	}

	if !IsValidDimensionType(input.DimensionType) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_dimension_type", "Invalid dimension type")
		return
	}

	dim, err := h.Service.UpdateComparisonDimension(r.Context(), id, tenantID, UpdateComparisonDimensionInput{
		Label:         input.Label,
		DimensionType: input.DimensionType,
		Position:      input.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": dim})
}

// DeleteComparisonDimension handles DELETE /api/v1/comparison/dimensions/{id}.
func (h *Handlers) DeleteComparisonDimension(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dimension ID")
		return
	}

	if err := h.Service.DeleteComparisonDimension(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Comparison Value Handlers
// ---------------------------------------------------------------------------

// UpsertComparisonValues handles PUT /api/v1/comparison/dimensions/{id}/values.
func (h *Handlers) UpsertComparisonValues(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	dimensionID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid dimension ID")
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	results := make([]gen.ComparisonValue, 0, len(input.Values))

	for _, v := range input.Values {
		var itemID pgtype.UUID
		if err := itemID.Scan(v.ItemID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_item_id", "Invalid item_id")
			return
		}

		val, err := h.Service.UpsertComparisonValue(r.Context(), tenantID, UpsertComparisonValueInput{
			DimensionID:  dimensionID,
			ItemID:       itemID,
			Value:        v.Value,
			ValueRating:  v.ValueRating,
			ValueBoolean: v.ValueBoolean,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}

		results = append(results, val)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": results})
}

// ReorderComparisonItems handles POST /api/v1/comparisons/{id}/items/reorder
// — bulk position update across a comparison's items, one call per item to
// the existing Service.ReorderComparisonItem (no new service logic).
func (h *Handlers) ReorderComparisonItems(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	comparisonID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	var input struct {
		Items []struct {
			ID       string `json:"id"`
			Position int32  `json:"position"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	for _, item := range input.Items {
		var itemID pgtype.UUID
		if err := itemID.Scan(item.ID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_item_id", "Invalid item id: "+item.ID)
			return
		}

		if err := h.Service.ReorderComparisonItem(r.Context(), itemID, comparisonID, tenantID, item.Position); err != nil {
			handleServiceError(w, err)
			return
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
}
