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
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
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

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	var fieldGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id")
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

	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}

// ListComparisons handles GET /api/v1/comparisons — a page of comparison
// content rows for the tenant, optionally filtered by type and space_id
// (Section 30, BUG-007/BUG-008).
func (h *Handlers) ListComparisons(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	var filter ListComparisonsFilter
	filter.Limit = parsePageSize(r)

	// type filter: accept either type_id (uuid) or type (slug) and resolve to id.
	if v := q.Get("type_id"); v != "" {
		if err := filter.TypeID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
			return
		}
	} else if v := q.Get("type"); v != "" {
		td, err := h.Service.GetComparisonTypeBySlug(r.Context(), v, tenantID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		filter.TypeID = td.ID
	}

	if v := q.Get("space_id"); v != "" {
		if err := filter.SpaceID.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}
	}

	if v := q.Get("cursor"); v != "" {
		if err := filter.Cursor.Scan(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid cursor")
			return
		}
	}

	comparisons, err := h.Service.ListComparisons(r.Context(), tenantID, filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": comparisons})
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
		writeError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	result, err := h.Service.GetComparisonTypeBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
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

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	var input struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
		ItemLabel   string `json:"item_label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	result, err := h.Service.UpdateComparisonType(r.Context(), id, tenantID, UpdateComparisonTypeInput{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
		Icon:        input.Icon,
		ItemLabel:   input.ItemLabel,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
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

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	if err := h.Service.DeleteComparisonType(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
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

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	typeID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	var input struct {
		UserGroupID string `json:"user_group_id"`
		CanCreate   bool   `json:"can_create"`
		CanFill     bool   `json:"can_fill"`
		AICanFill   bool   `json:"ai_can_fill"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Empty user_group_id is intentionally left as pgtype.UUID{Valid:false}
	// so NULL is stored — Section 30: "NULL = AI agent only".
	var userGroupID pgtype.UUID
	if input.UserGroupID != "" {
		if err := userGroupID.Scan(input.UserGroupID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_user_group_id", "Invalid user_group_id")
			return
		}
	}

	if err := h.Service.SetComparisonTypeAccess(r.Context(), typeID, tenantID, SetComparisonTypeAccessInput{
		UserGroupID: userGroupID,
		CanCreate:   input.CanCreate,
		CanFill:     input.CanFill,
		AICanFill:   input.AICanFill,
	}); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "updated"}})
}

// ListComparisonTypeAccess handles GET /api/v1/comparison/types/{id}/access.
func (h *Handlers) ListComparisonTypeAccess(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	typeID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison type ID")
		return
	}

	access, err := h.Service.ListComparisonTypeAccess(r.Context(), typeID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": access})
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
		TypeID     string `json:"type_id"`
		Items      []struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"items"`
		Dimensions []string `json:"dimensions"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var spaceID pgtype.UUID
	if input.SpaceID != "" {
		if err := spaceID.Scan(input.SpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}
	}

	var typeID pgtype.UUID
	if input.TypeID != "" {
		if err := typeID.Scan(input.TypeID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
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

	result, err := h.Service.CreateComparison(r.Context(), tenantID, user.UserID, CreateComparisonInput{
		Title:      input.Title,
		Body:       input.Body,
		Visibility: input.Visibility,
		SpaceID:    spaceID,
		TypeID:     typeID,
		Items:      items,
		Dimensions: input.Dimensions,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
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

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	detail, err := h.Service.GetComparisonDetail(r.Context(), contentID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": detail})
}

// UpdateComparison handles PATCH /api/v1/comparisons/{id} — updates the
// comparison's metadata (title/slug/body/visibility) on the backing content
// row. Previously this route was wired to UpdateComparisonItem, which treated
// {id} as an item id and corrupted comparison items.
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (h *Handlers) UpdateComparison(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	var input struct {
		Title      string `json:"title"`
		Slug       string `json:"slug"`
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	result, err := h.Service.UpdateComparison(r.Context(), id, tenantID, UpdateComparisonInput{
		Title:      input.Title,
		Slug:       input.Slug,
		Body:       input.Body,
		Visibility: input.Visibility,
		EditedBy:   user.UserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteComparison handles DELETE /api/v1/comparisons/{id} — soft-deletes the
// backing content row. Previously this route was wired to DeleteComparisonItem,
// which hard-deleted a comparison item instead of the comparison itself.
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
func (h *Handlers) DeleteComparison(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	if err := h.Service.DeleteComparison(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
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

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	matrix, err := h.Service.GetComparisonMatrix(r.Context(), contentID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": matrix})
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

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var imageID, geoSpaceID pgtype.UUID
	if input.ImageID != "" {
		if err := imageID.Scan(input.ImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_image_id", "Invalid image ID")
			return
		}
	}

	if input.GeoSpaceID != "" {
		if err := geoSpaceID.Scan(input.GeoSpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo space ID")
			return
		}
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

	writeJSON(w, http.StatusCreated, map[string]any{"data": item})
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

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "title is required")
		return
	}

	var imageID, geoSpaceID pgtype.UUID
	if input.ImageID != "" {
		if err := imageID.Scan(input.ImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_image_id", "Invalid image ID")
			return
		}
	}

	if input.GeoSpaceID != "" {
		if err := geoSpaceID.Scan(input.GeoSpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo space ID")
			return
		}
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

	writeJSON(w, http.StatusOK, map[string]any{"data": item})
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

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid item ID")
		return
	}

	if err := h.Service.DeleteComparisonItem(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
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

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	var input struct {
		Label         string `json:"label"`
		DimensionType string `json:"dimension_type"`
		Position      int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Label == "" {
		writeError(w, http.StatusBadRequest, "missing_label", "label is required")
		return
	}

	if input.DimensionType == "" {
		writeError(w, http.StatusBadRequest, "missing_dimension_type", "dimension_type is required")
		return
	}

	if !IsValidDimensionType(input.DimensionType) {
		writeError(w, http.StatusBadRequest, "invalid_dimension_type", "Invalid dimension type")
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

	writeJSON(w, http.StatusCreated, map[string]any{"data": dim})
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

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid dimension ID")
		return
	}

	var input struct {
		Label         string `json:"label"`
		DimensionType string `json:"dimension_type"`
		Position      int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Label == "" {
		writeError(w, http.StatusBadRequest, "missing_label", "label is required")
		return
	}

	if !IsValidDimensionType(input.DimensionType) {
		writeError(w, http.StatusBadRequest, "invalid_dimension_type", "Invalid dimension type")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": dim})
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

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid dimension ID")
		return
	}

	if err := h.Service.DeleteComparisonDimension(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
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

	dimensionID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid dimension ID")
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Batch upsert: one round-trip for all values (BUG-027).
	inputs := make([]UpsertComparisonValueInput, 0, len(input.Values))
	for _, v := range input.Values {
		var itemID pgtype.UUID
		if err := itemID.Scan(v.ItemID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_item_id", "Invalid item_id")
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

	results, err := h.Service.BatchUpsertComparisonValues(r.Context(), tenantID, dimensionID, inputs)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": results})
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

	comparisonID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	var input struct {
		Items []struct {
			ID       string `json:"id"`
			Position int32  `json:"position"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	for _, item := range input.Items {
		var itemID pgtype.UUID
		if err := itemID.Scan(item.ID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_item_id", "Invalid item id: "+item.ID)
			return
		}

		if err := h.Service.ReorderComparisonItem(r.Context(), itemID, comparisonID, tenantID, item.Position); err != nil {
			handleServiceError(w, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
}

// AutoGenerateDimensions handles POST /api/v1/comparisons/auto-dimensions.
// Given a list of item names, returns a set of suggested comparison dimensions
// derived from a deterministic heuristic (no AI tokens). The frontend presents
// these as editable defaults in the comparison creation wizard (Phase 16.05).
//
// Ref: japaconnect-engineering/sections/16-comparisons.md (Phase 16.05 — HITL)
func (h *Handlers) AutoGenerateDimensions(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Items []string `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Items) < 2 {
		writeError(w, http.StatusBadRequest, "insufficient_items", "At least 2 items are required")
		return
	}

	dimensions := suggestDimensions(input.Items)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"dimensions": dimensions}})
}

// suggestDimensions returns a deterministic set of comparison dimensions
// based on the item names. This is a heuristic — no AI tokens are spent.
// The frontend can override or extend these before creating the comparison.
func suggestDimensions(items []string) []string {
	// Universal dimensions that apply to most comparison contexts.
	dims := []string{
		"Overview",
		"Key features",
		"Pros",
		"Cons",
		"Overall rating",
	}

	// If all items look like products/services (contain common product keywords),
	// add price and quality dimensions.
	productHints := []string{"price", "cost", "plan", "package", "version", "edition", "model"}
	itemBlob := ""
	for _, it := range items {
		itemBlob += " " + it
	}
	itemLower := strings.ToLower(itemBlob)
	for _, hint := range productHints {
		if strings.Contains(itemLower, hint) {
			dims = append([]string{"Price", "Value for money"}, dims...)
			break
		}
	}

	// If items mention education/learning keywords, add education-specific dims.
	eduHints := []string{"course", "university", "school", "program", "degree", "certification"}
	for _, hint := range eduHints {
		if strings.Contains(itemLower, hint) {
			dims = append(dims, "Curriculum", "Duration", "Cost", "Accreditation")
			break
		}
	}

	return dims
}
