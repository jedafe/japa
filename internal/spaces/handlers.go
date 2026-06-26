// Package spaces — HTTP handlers for the Geo-First Space System.
//
// Every handler is a method on Handlers so dependencies (Service and
// UnreadCounter) are injected via the constructor, never read from globals.
// Responses follow the JapaConnect API envelope: success bodies are wrapped
// in {"data": ...} and errors in {"error":{"code":"...","message":"..."}}.
// Permission enforcement is handled by middleware wired at the router level;
// these handlers focus on parsing, validation, and Service delegation.
package spaces

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for space HTTP handlers. The Service wraps the
// reader/writer database pools; the UnreadCounter tracks per-user unread
// post counts via Valkey (nil is safe — it degrades to no-op).
type Handlers struct {
	Service *Service
	Unread  *UnreadCounter
}

// NewHandlers creates Handlers with the given Service and UnreadCounter.
func NewHandlers(service *Service, unread *UnreadCounter) *Handlers {
	return &Handlers{Service: service, Unread: unread}
}

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}
	return id, nil
}

// requireTenant resolves the tenant from the request context and writes a 503
// error when none is available. The boolean return is false when the caller
// should abort (the error has already been written).
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response. It centralizes the not-found, duplicate, and fallback mapping so
// every handler stays consistent.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// spaceFromContext writes a 404 error and returns false when no resolved space
// is present in the request context. Callers should abort when the boolean is
// false.
func spaceFromContext(w http.ResponseWriter, r *http.Request) (*gen.Space, bool) {
	space := FromContext(r.Context())
	if space == nil {
		writeError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return nil, false
	}
	return space, true
}

// ---------------------------------------------------------------------------
// Space Type Handlers
// ---------------------------------------------------------------------------

// ListSpaceTypes handles GET /api/v1/space-types.
func (h *Handlers) ListSpaceTypes(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	types, err := h.Service.ListSpaceTypes(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}

// GetSpaceType handles GET /api/v1/space-types/{id}.
func (h *Handlers) GetSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	st, err := h.Service.GetSpaceType(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": st})
}

// CreateSpaceType handles POST /api/v1/space-types. API-created types are
// never system types.
func (h *Handlers) CreateSpaceType(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string  `json:"name"`
		Slug         string  `json:"slug"`
		Description  *string `json:"description"`
		Icon         *string `json:"icon"`
		FieldGroupID string  `json:"field_group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var fieldGroupID pgtype.UUID
	if req.FieldGroupID != "" {
		if err := fieldGroupID.Scan(req.FieldGroupID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid field_group_id format")
			return
		}
	}
	st, err := h.Service.CreateSpaceType(r.Context(), gen.CreateSpaceTypeParams{
		TenantID:     tenantID,
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Icon:         req.Icon,
		FieldGroupID: fieldGroupID,
		IsSystem:     false,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": st})
}

// UpdateSpaceType handles PATCH /api/v1/space-types/{id}. Slug is immutable
// and therefore not accepted in the request body.
func (h *Handlers) UpdateSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	var req struct {
		Name         string  `json:"name"`
		Description  *string `json:"description"`
		Icon         *string `json:"icon"`
		FieldGroupID string  `json:"field_group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var fieldGroupID pgtype.UUID
	if req.FieldGroupID != "" {
		if err := fieldGroupID.Scan(req.FieldGroupID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid field_group_id format")
			return
		}
	}
	st, err := h.Service.UpdateSpaceType(r.Context(), gen.UpdateSpaceTypeParams{
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		FieldGroupID: fieldGroupID,
		ID:           id,
		TenantID:     tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": st})
}

// DeleteSpaceType handles DELETE /api/v1/space-types/{id}.
func (h *Handlers) DeleteSpaceType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if err := h.Service.DeleteSpaceType(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Space Handlers
// ---------------------------------------------------------------------------

// ListSpaces handles GET /api/v1/spaces. Optional query params filter the
// result set: level (by hierarchy level), type (by space_type_id), and
// parent (by parent_id). With no filters, all spaces for the tenant are
// returned.
func (h *Handlers) ListSpaces(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	var spaces []gen.Space
	var err error
	switch {
	case q.Get("level") != "":
		spaces, err = h.Service.ListSpacesByLevel(r.Context(), tenantID, q.Get("level"))
	case q.Get("type") != "":
		var spaceTypeID pgtype.UUID
		if scanErr := spaceTypeID.Scan(q.Get("type")); scanErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid type filter format")
			return
		}
		spaces, err = h.Service.ListSpacesByType(r.Context(), tenantID, spaceTypeID)
	case q.Get("parent") != "":
		var parentID pgtype.UUID
		if scanErr := parentID.Scan(q.Get("parent")); scanErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid parent filter format")
			return
		}
		spaces, err = h.Service.ListChildSpaces(r.Context(), tenantID, parentID)
	default:
		spaces, err = h.Service.ListSpaces(r.Context(), tenantID)
	}
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": spaces})
}

// GetSpace handles GET /api/v1/spaces/{slug}. It returns the space together
// with its rules so a single request can render a complete space view.
func (h *Handlers) GetSpace(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Space slug is required")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	space, err := h.Service.GetSpaceBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	rules, err := h.Service.ListSpaceRules(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"space": space, "rules": rules},
	})
}

// CreateSpace handles POST /api/v1/spaces. When preset_name is provided the
// five permission axes are loaded from the named preset; otherwise the
// default axes are applied.
func (h *Handlers) CreateSpace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		SpaceTypeID string  `json:"space_type_id"`
		Level       string  `json:"level"`
		ParentID    string  `json:"parent_id"`
		Description *string `json:"description"`
		GeoCode     *string `json:"geo_code"`
		PresetName  string  `json:"preset_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" ||
		strings.TrimSpace(req.SpaceTypeID) == "" || strings.TrimSpace(req.Level) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name, slug, space_type_id, and level are required")
		return
	}
	if !IsValidLevel(req.Level) {
		writeError(w, http.StatusBadRequest, "invalid_level", "Level must be country, state, city, or custom")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var spaceTypeID pgtype.UUID
	if err := spaceTypeID.Scan(req.SpaceTypeID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid space_type_id format")
		return
	}
	var parentID pgtype.UUID
	if req.ParentID != "" {
		if err := parentID.Scan(req.ParentID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid parent_id format")
			return
		}
	}

	// Resolve permission axes: from the named preset, or the defaults.
	axes := DefaultPermAxes()
	if req.PresetName != "" {
		preset, err := h.Service.GetPermissionPreset(r.Context(), req.PresetName, tenantID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_preset", "Preset not found")
			return
		}
		axes = SpacePermAxes{
			Visibility:  preset.Visibility,
			Posting:     preset.PostingPerm,
			Replying:    preset.ReplyingPerm,
			Commenting:  preset.CommentingPerm,
			Joining:     preset.JoiningPerm,
		}
	}

	space, err := h.Service.CreateSpace(r.Context(), gen.CreateSpaceParams{
		TenantID:       tenantID,
		ParentID:       parentID,
		SpaceTypeID:    spaceTypeID,
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		GeoCode:        req.GeoCode,
		Level:          req.Level,
		Visibility:     axes.Visibility,
		PostingPerm:    axes.Posting,
		ReplyingPerm:   axes.Replying,
		CommentingPerm: axes.Commenting,
		JoiningPerm:    axes.Joining,
		Status:         StatusActive,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": space})
}

// UpdateSpace handles PATCH /api/v1/spaces/{id}. It loads the existing space
// first so unspecified fields keep their current values (true PATCH
// semantics), then validates any provided permission axes before saving.
func (h *Handlers) UpdateSpace(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	existing, err := h.Service.GetSpace(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	var req struct {
		Name           string  `json:"name"`
		Description    *string `json:"description"`
		WikiContent    string  `json:"wiki_content"`
		GeoCode        *string `json:"geo_code"`
		Visibility     string  `json:"visibility"`
		PostingPerm    string  `json:"posting_perm"`
		ReplyingPerm   string  `json:"replying_perm"`
		CommentingPerm string  `json:"commenting_perm"`
		JoiningPerm    string  `json:"joining_perm"`
		Status         string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	// Validate each provided permission axis individually.
	if req.Visibility != "" && !validVisibility[req.Visibility] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid visibility value")
		return
	}
	if req.PostingPerm != "" && !validPostingPerm[req.PostingPerm] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid posting_perm value")
		return
	}
	if req.ReplyingPerm != "" && !validPostingPerm[req.ReplyingPerm] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid replying_perm value")
		return
	}
	if req.CommentingPerm != "" && !validPostingPerm[req.CommentingPerm] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid commenting_perm value")
		return
	}
	if req.JoiningPerm != "" && !validJoiningPerm[req.JoiningPerm] {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "Invalid joining_perm value")
		return
	}

	// Overlay provided fields onto the existing space, keeping current values
	// for anything the request left blank.
	name := existing.Name
	if strings.TrimSpace(req.Name) != "" {
		name = req.Name
	}
	description := existing.Description
	if req.Description != nil {
		description = req.Description
	}
	wikiContent := existing.WikiContent
	if req.WikiContent != "" {
		wikiContent = []byte(req.WikiContent)
	}
	geoCode := existing.GeoCode
	if req.GeoCode != nil {
		geoCode = req.GeoCode
	}
	visibility := existing.Visibility
	if req.Visibility != "" {
		visibility = req.Visibility
	}
	postingPerm := existing.PostingPerm
	if req.PostingPerm != "" {
		postingPerm = req.PostingPerm
	}
	replyingPerm := existing.ReplyingPerm
	if req.ReplyingPerm != "" {
		replyingPerm = req.ReplyingPerm
	}
	commentingPerm := existing.CommentingPerm
	if req.CommentingPerm != "" {
		commentingPerm = req.CommentingPerm
	}
	joiningPerm := existing.JoiningPerm
	if req.JoiningPerm != "" {
		joiningPerm = req.JoiningPerm
	}
	status := existing.Status
	if req.Status != "" {
		status = req.Status
	}

	space, err := h.Service.UpdateSpace(r.Context(), gen.UpdateSpaceParams{
		Name:           name,
		Description:    description,
		WikiContent:    wikiContent,
		GeoCode:        geoCode,
		Visibility:     visibility,
		PostingPerm:    postingPerm,
		ReplyingPerm:   replyingPerm,
		CommentingPerm: commentingPerm,
		JoiningPerm:    joiningPerm,
		Status:         status,
		ID:             id,
		TenantID:       tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": space})
}

// DeleteSpace handles DELETE /api/v1/spaces/{id}.
func (h *Handlers) DeleteSpace(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	if err := h.Service.DeleteSpace(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Space Rule Handlers
// ---------------------------------------------------------------------------

// ListSpaceRules handles GET /api/v1/spaces/{slug}/rules. The space is
// resolved by the SpaceResolver middleware and read from the request context.
func (h *Handlers) ListSpaceRules(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}
	rules, err := h.Service.ListSpaceRules(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rules})
}

// CreateSpaceRule handles POST /api/v1/spaces/{slug}/rules.
func (h *Handlers) CreateSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}
	var req struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Position int32  `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Title and body are required")
		return
	}
	rule, err := h.Service.CreateSpaceRule(r.Context(), gen.CreateSpaceRuleParams{
		SpaceID:  space.ID,
		Title:    req.Title,
		Body:     req.Body,
		Position: req.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": rule})
}

// UpdateSpaceRule handles PATCH /api/v1/spaces/{slug}/rules/{ruleId}.
func (h *Handlers) UpdateSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}
	ruleID, err := parseUUIDParam(r, "ruleId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID format")
		return
	}
	var req struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Position int32  `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Title and body are required")
		return
	}
	rule, err := h.Service.UpdateSpaceRule(r.Context(), gen.UpdateSpaceRuleParams{
		Title:    req.Title,
		Body:     req.Body,
		Position: req.Position,
		ID:       ruleID,
		SpaceID:  space.ID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rule})
}

// DeleteSpaceRule handles DELETE /api/v1/spaces/{slug}/rules/{ruleId}.
func (h *Handlers) DeleteSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}
	ruleID, err := parseUUIDParam(r, "ruleId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID format")
		return
	}
	if err := h.Service.DeleteSpaceRule(r.Context(), ruleID, space.ID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Permission Preset Handlers
// ---------------------------------------------------------------------------

// ListPermissionPresets handles GET /api/v1/space-permission-presets.
func (h *Handlers) ListPermissionPresets(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	presets, err := h.Service.ListPermissionPresets(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": presets})
}

// UpdatePermissionPreset handles PUT /api/v1/space-permission-presets/{name}.
// The preset name is taken from the URL path; the five axis values come from
// the request body.
func (h *Handlers) UpdatePermissionPreset(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Preset name is required")
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	existing, err := h.Service.GetPermissionPreset(r.Context(), name, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	var req struct {
		Visibility     string `json:"visibility"`
		PostingPerm    string `json:"posting_perm"`
		ReplyingPerm   string `json:"replying_perm"`
		CommentingPerm string `json:"commenting_perm"`
		JoiningPerm    string `json:"joining_perm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	axes := SpacePermAxes{
		Visibility:  req.Visibility,
		Posting:     req.PostingPerm,
		Replying:    req.ReplyingPerm,
		Commenting:  req.CommentingPerm,
		Joining:     req.JoiningPerm,
	}
	if !ValidPermissionAxes(axes) {
		writeError(w, http.StatusBadRequest, "invalid_permission_axes", "One or more permission axes are invalid")
		return
	}
	preset, err := h.Service.UpdatePermissionPreset(r.Context(), gen.UpdatePermissionPresetParams{
		Visibility:     req.Visibility,
		PostingPerm:    req.PostingPerm,
		ReplyingPerm:   req.ReplyingPerm,
		CommentingPerm: req.CommentingPerm,
		JoiningPerm:    req.JoiningPerm,
		ID:             existing.ID,
		TenantID:       tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": preset})
}
