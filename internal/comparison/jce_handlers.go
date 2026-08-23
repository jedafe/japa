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
	"github.com/jedafe/japa/internal/content"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// JCE Comparison Handlers
// ---------------------------------------------------------------------------

// listJCEComparisons handles GET /api/v1/jce/comparisons.
func (h *Handlers) ListJCEComparisons(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())

	query := r.URL.Query()

	var spaceID, typeID pgtype.UUID
	if v := query.Get("space_id"); v != "" {
		if err := spaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}
	}
	if v := query.Get("type_id"); v != "" {
		if err := typeID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
			return
		}
	} else if v := query.Get("type"); v != "" {
		// Support legacy slug-based type filter (e.g. ?type=university).
		td, err := h.Service.GetComparisonTypeBySlug(r.Context(), v, tenantID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Comparison type not found")
				return
			}
			handleServiceError(w, err)
			return
		}
		typeID = td.ID
	}

	limit := int32(defaultPageSize)
	if v := query.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = int32(n)
		}
	}
	offset := int32(0)
	if v := query.Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}

	filter := JCEComparisonFilter{
		SpaceID:    spaceID,
		TypeID:     typeID,
		Depth:      query.Get("depth"),
		Status:     query.Get("status"),
		Visibility: query.Get("visibility"),
		Mine:       query.Get("mine") == "true",
		Query:      query.Get("q"),
		Limit:      limit,
		Offset:     offset,
	}

	// Public directory default: published, public, detailed.
	if user == nil {
		filter.Status = content.StatusPublished
		filter.Visibility = content.VisibilityPublic
		filter.Depth = DepthDetailed
	} else {
		filter.UserID = user.UserID
		if filter.Status == "" && filter.Visibility == "" && filter.Depth == "" && !filter.Mine {
			filter.Status = content.StatusPublished
			filter.Visibility = content.VisibilityPublic
			filter.Depth = DepthDetailed
		}
	}

	rows, err := h.Service.ListJCEComparisons(r.Context(), tenantID, filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// createJCEComparisonBody is the expected body for POST /api/v1/jce/comparisons.
type createJCEComparisonBody struct {
	Title      string        `json:"title"`
	Body       string        `json:"body"`
	SpaceID    string        `json:"space_id"`
	TypeID     string        `json:"type_id"`
	Depth      string        `json:"depth"`
	Items      []jceItemBody `json:"items"`
	Dimensions []string      `json:"dimensions"`
}

type jceItemBody struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	TypeID      string `json:"type_id"`
	ImageID     string `json:"image_id"`
	GeoSpaceID  string `json:"geo_space_id"`
	Position    int32  `json:"position"`
	ItemRef     any    `json:"item_ref"`
}

func parseOptionalUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if s == "" {
		return id, nil
	}
	if err := id.Scan(s); err != nil {
		return id, err
	}
	return id, nil
}

func marshalItemRef(ref any) []byte {
	if ref == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(ref)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// CreateJCEComparison handles POST /api/v1/jce/comparisons.
func (h *Handlers) CreateJCEComparison(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var body createJCEComparisonBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	spaceID, err := parseOptionalUUID(body.SpaceID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
		return
	}
	typeID, err := parseOptionalUUID(body.TypeID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
		return
	}

	items := make([]JCEItemInput, 0, len(body.Items))
	for _, it := range body.Items {
		itemTypeID, err := parseOptionalUUID(it.TypeID)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_item_type_id", "Invalid item type_id")
			return
		}
		imageID, err := parseOptionalUUID(it.ImageID)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_image_id", "Invalid image_id")
			return
		}
		geoSpaceID, err := parseOptionalUUID(it.GeoSpaceID)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo_space_id")
			return
		}

		items = append(items, JCEItemInput{
			Title:       it.Title,
			Description: it.Description,
			TypeID:      itemTypeID,
			ImageID:     imageID,
			GeoSpaceID:  geoSpaceID,
			Position:    it.Position,
			ItemRef:     marshalItemRef(it.ItemRef),
		})
	}

	// Quick depth is forced for unauthenticated callers, but requireUser above prevents that.
	depth := body.Depth
	if depth == "" {
		depth = DepthQuick
	}

	res, err := h.Service.CreateJCEComparison(r.Context(), tenantID, user.UserID, CreateJCEComparisonInput{
		Title:      body.Title,
		Body:       body.Body,
		SpaceID:    spaceID,
		TypeID:     typeID,
		Depth:      depth,
		Items:      items,
		Dimensions: body.Dimensions,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": res})
}

// GetJCEComparison handles GET /api/v1/jce/comparisons/{id|slug}.
func (h *Handlers) GetJCEComparison(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	idOrSlug := chi.URLParam(r, "idOrSlug")
	if idOrSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID or slug")
		return
	}

	cmp, err := h.Service.GetJCEComparison(r.Context(), idOrSlug, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Comparison not found")
			return
		}
		handleServiceError(w, err)
		return
	}

	// Visibility gating.
	user := auth.FromContext(r.Context())
	if !canAccessJCEComparison(cmp, user) {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Comparison not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": jceComparisonResponse(cmp)})
}

// canAccessJCEComparison returns true if the requester may view the comparison.
func canAccessJCEComparison(cmp JCEComparison, user *auth.AuthenticatedUser) bool {
	// Public published detailed comparisons are always visible.
	if cmp.Content.Status == content.StatusPublished &&
		cmp.Content.Visibility == content.VisibilityPublic &&
		cmp.Metadata.Depth == DepthDetailed {
		return true
	}

	// Authenticated owner or admin can view non-public or quick.
	if user != nil {
		if cmp.Content.AuthorID.Valid && cmp.Content.AuthorID.Bytes == user.UserID.Bytes {
			return true
		}
	}

	return false
}

// UpdateJCEComparison handles PATCH /api/v1/jce/comparisons/{id}.
func (h *Handlers) UpdateJCEComparison(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	var body updateJCEComparisonBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	var config []byte
	if body.Config != nil {
		config, _ = json.Marshal(body.Config)
	}

	bypass := hasAnyPermission(r, user, "comparison.edit_any")
	cmp, err := h.Service.UpdateJCEComparison(r.Context(), id, tenantID, user.UserID, UpdateJCEComparisonInput{
		Title:      body.Title,
		Body:       body.Body,
		Slug:       body.Slug,
		Visibility: body.Visibility,
		Status:     body.Status,
		Config:     config,
	}, bypass)
	if err != nil {
		if errors.Is(err, errpkg.ErrForbidden) {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You can only update your own comparisons")
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": jceComparisonResponse(cmp)})
}

type updateJCEComparisonBody struct {
	Title      string `json:"title"`
	Body       string `json:"body"`
	Slug       string `json:"slug"`
	Visibility string `json:"visibility"`
	Status     string `json:"status"`
	Config     any    `json:"config"`
}

// DeleteJCEComparison handles DELETE /api/v1/jce/comparisons/{id}.
func (h *Handlers) DeleteJCEComparison(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	bypass := hasAnyPermission(r, user, "comparison.delete_any")
	if err := h.Service.DeleteJCEComparison(r.Context(), id, tenantID, user.UserID, bypass); err != nil {
		if errors.Is(err, errpkg.ErrForbidden) {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You can only delete your own comparisons")
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// PromoteJCEComparison handles POST /api/v1/jce/comparisons/{id}/promote.
func (h *Handlers) PromoteJCEComparison(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID")
		return
	}

	bypass := hasAnyPermission(r, user, "comparison.edit_any")
	res, err := h.Service.PromoteJCEComparison(r.Context(), id, tenantID, user.UserID, bypass)
	if err != nil {
		if errors.Is(err, errpkg.ErrForbidden) {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You can only promote your own comparisons")
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": res})
}

// GetJCEComparisonMatrix handles GET /api/v1/jce/comparisons/{id|slug}/matrix.
func (h *Handlers) GetJCEComparisonMatrix(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	idOrSlug := chi.URLParam(r, "id")
	if idOrSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid comparison ID or slug")
		return
	}

	// Resolve content + metadata to enforce visibility.
	cmp, err := h.Service.GetJCEComparison(r.Context(), idOrSlug, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Comparison not found")
			return
		}
		handleServiceError(w, err)
		return
	}

	user := auth.FromContext(r.Context())
	if !canAccessJCEComparison(cmp, user) {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Comparison not found")
		return
	}

	matrix, err := h.Service.GetJCEComparisonMatrix(r.Context(), cmp.Content.ID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": jceMatrixResponse(matrix)})
}

// SearchJCEEntities handles GET /api/v1/jce/entities/{type}.
func (h *Handlers) SearchJCEEntities(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	entityType := chi.URLParam(r, "type")
	if entityType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_type", "Entity type is required")
		return
	}

	q := r.URL.Query()
	search := q.Get("q")
	var limit int32 = DefaultPageSize
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = int32(n)
		}
	}
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	filters := make(map[string]any)
	for k, v := range q {
		if k == "q" || k == "limit" {
			continue
		}
		if len(v) > 0 && v[0] != "" {
			filters[k] = v[0]
		}
	}

	items, err := h.Service.SearchJCEEntities(r.Context(), entityType, search, filters, tenantID, limit)
	if err != nil {
		if err.Error() == "resolver not configured" {
			errpkg.WriteError(w, http.StatusNotImplemented, "not_implemented", "Entity resolver not configured")
			return
		}
		errpkg.WriteError(w, http.StatusBadRequest, "search_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// ResolveJCEEntity handles GET /api/v1/jce/entities/{type}/{id}.
func (h *Handlers) ResolveJCEEntity(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	entityType := chi.URLParam(r, "type")
	idOrSlug := chi.URLParam(r, "id")
	if entityType == "" || idOrSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "Entity type and id are required")
		return
	}

	item, err := h.Service.ResolveJCEEntity(r.Context(), entityType, idOrSlug, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": item})
}

// CreateSelectionToken handles POST /api/v1/jce/selections.
func (h *Handlers) CreateSelectionToken(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		TypeSlug string           `json:"type_slug"`
		ItemRefs []map[string]any `json:"item_refs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.TypeSlug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_type_slug", "type_slug is required")
		return
	}

	token, err := h.Service.CreateSelectionToken(r.Context(), CreateSelectionTokenInput{
		TypeSlug: req.TypeSlug,
		ItemRefs: req.ItemRefs,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "token_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"token": token}})
}

// CreateJCEComparisonFromToken handles POST /api/v1/jce/comparisons/from-token.
func (h *Handlers) CreateJCEComparisonFromToken(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	var req struct {
		Token   string `json:"token"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		SpaceID string `json:"space_id"`
		TypeID  string `json:"type_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.Token == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_token", "token is required")
		return
	}
	if req.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_title", "title is required")
		return
	}

	input := CreateJCEComparisonInput{
		Title: req.Title,
		Body:  req.Body,
		Depth: DepthQuick,
	}
	if req.SpaceID != "" {
		if err := input.SpaceID.Scan(req.SpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}
	}
	if req.TypeID != "" {
		if err := input.TypeID.Scan(req.TypeID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
			return
		}
	}

	result, err := h.Service.CreateJCEComparisonFromToken(r.Context(), tenantID, user.UserID, req.Token, input)
	if err != nil {
		if err.Error() == "valkey not configured" {
			errpkg.WriteError(w, http.StatusNotImplemented, "not_implemented", "Selection token flow is not configured")
			return
		}
		if err.Error() == "selection token not found or expired" {
			errpkg.WriteError(w, http.StatusNotFound, "token_not_found", err.Error())
			return
		}
		errpkg.WriteError(w, http.StatusBadRequest, "creation_failed", err.Error())
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}
