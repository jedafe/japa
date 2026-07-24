// Package customfields — HTTP handlers for the Universal Custom Field
// Architecture.
//
// Every handler is a method on Handlers so dependencies (Service and
// EmbedResolver) are injected via the constructor, never read from globals.
// Responses follow the JapaConnect API envelope: success bodies are wrapped
// in {"data": ...} and errors in {"error":{"code":"...","message":"..."}}.
// Permission enforcement is handled by middleware wired at the router level;
// these handlers focus on parsing, validation, and Service delegation.
package customfields

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"time"

	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for custom field HTTP handlers. The Service
// wraps the reader/writer database pools; the EmbedResolver validates
// platform_embed field values at write time.
type Handlers struct {
	Service *Service
	Embed   *EmbedResolver
}

// NewHandlers creates Handlers with the given Service and EmbedResolver.
func NewHandlers(service *Service, embed *EmbedResolver) *Handlers {
	return &Handlers{Service: service, Embed: embed}
}

// errorResponse is the JSON body returned for error responses, following the
// JapaConnect API error format: {"error": {"code": "...", "message": "..."}}.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// clientIP extracts the client IP from the request, respecting X-Forwarded-For
// and X-Real-IP headers set by the reverse proxy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	idx := strings.LastIndex(r.RemoteAddr, ":")
	if idx > 0 {
		return r.RemoteAddr[:idx]
	}

	return r.RemoteAddr
}

// requireTenant resolves the tenant from the request context and writes a 503
// error when none is available. The boolean return is false when the caller
// should abort (the error has already been written).
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
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
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, errpkg.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// ---------------------------------------------------------------------------
// Custom Field Groups
// ---------------------------------------------------------------------------

// ListFieldGroups handles GET /api/v1/custom-field-groups.
func (h *Handlers) ListFieldGroups(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	groups, err := h.Service.ListFieldGroups(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": groups})
}

// GetFieldGroup handles GET /api/v1/custom-field-groups/{id}. It returns the
// group together with its fields so callers can render a complete editor in a
// single round trip.
func (h *Handlers) GetFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	group, err := h.Service.GetFieldGroup(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	fields, err := h.Service.ListFieldsByGroup(r.Context(), group.ID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"group": group, "fields": fields},
	})
}

// CreateFieldGroup handles POST /api/v1/custom-field-groups. API-created groups
// are never system groups.
func (h *Handlers) CreateFieldGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		Description *string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	group, err := h.Service.CreateFieldGroup(r.Context(), tenantID, req.Name, req.Slug, req.Description, false)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": group})
}

// UpdateFieldGroup handles PATCH /api/v1/custom-field-groups/{id}.
func (h *Handlers) UpdateFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	group, err := h.Service.UpdateFieldGroup(r.Context(), id, tenantID, req.Name, req.Description)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": group})
}

// DeleteFieldGroup handles DELETE /api/v1/custom-field-groups/{id}.
func (h *Handlers) DeleteFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteFieldGroup(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Custom Fields
// ---------------------------------------------------------------------------

// fieldRequest is the shared body shape for create and update field handlers.
// Key and GroupID are immutable after creation and are only read by CreateField.
type fieldRequest struct {
	Label          string        `json:"label"`
	Key            string        `json:"key"`
	FieldType      string        `json:"field_type"`
	Options        []FieldOption `json:"options"`
	EmbedPlatforms []string      `json:"embed_platforms"`
	Placeholder    *string       `json:"placeholder"`
	DefaultValue   *string       `json:"default_value"`
	IsRequired     bool          `json:"is_required"`
	ShowInFrontend bool          `json:"show_in_frontend"`
	Filterable     bool          `json:"filterable"`
	Sortable       bool          `json:"sortable"`
	Searchable     bool          `json:"searchable"`
	Position       int32         `json:"position"`
}

// marshalOptions encodes select/multiselect options to the JSON bytes the
// database column expects. Returns nil when no options are provided.
func marshalOptions(opts []FieldOption) ([]byte, error) {
	if len(opts) == 0 {
		return nil, nil
	}

	return json.Marshal(opts)
}

// ListFieldsByGroup handles GET /api/v1/custom-field-groups/{id}/fields.
func (h *Handlers) ListFieldsByGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	fields, err := h.Service.ListFieldsByGroup(r.Context(), groupID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": fields})
}

// CreateField handles POST /api/v1/custom-field-groups/{id}/fields.
func (h *Handlers) CreateField(w http.ResponseWriter, r *http.Request) {
	groupID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req fieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Label) == "" || strings.TrimSpace(req.Key) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "Label and key are required")
		return
	}

	if !IsValidFieldType(req.FieldType) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type")
		return
	}

	if msg, ok := ValidOptionsShape(req.FieldType, req.Options); !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_options", msg)
		return
	}

	if (req.FieldType == FieldTypeSelect || req.FieldType == FieldTypeMultiselect) && len(req.Options) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "missing_options", "Select and multiselect fields require options")
		return
	}

	optionsBytes, err := marshalOptions(req.Options)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_options", "Field options are not valid JSON")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	field, err := h.Service.CreateField(r.Context(), gen.CreateCustomFieldParams{
		GroupID:        groupID,
		TenantID:       tenantID,
		Label:          req.Label,
		Key:            req.Key,
		FieldType:      req.FieldType,
		Options:        optionsBytes,
		EmbedPlatforms: req.EmbedPlatforms,
		Placeholder:    req.Placeholder,
		DefaultValue:   req.DefaultValue,
		IsRequired:     req.IsRequired,
		IsSystem:       false,
		ShowInFrontend: req.ShowInFrontend,
		Filterable:     req.Filterable,
		Sortable:       req.Sortable,
		Searchable:     req.Searchable,
		Position:       req.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": field})
}

// UpdateField handles PATCH /api/v1/custom-fields/{id}. Key and group_id are
// immutable and therefore not accepted in the request body.
func (h *Handlers) UpdateField(w http.ResponseWriter, r *http.Request) {
	h.updateFieldByParam(w, r, "id")
}

// UpdateFieldInGroup handles PATCH /api/v1/admin/field-groups/{id}/fields/{fid}.
// The admin route nests the field under its owning group in the URL, so the
// field ID travels in the "fid" param instead of "id" (which is already
// taken by the group ID) — the rest of the logic is identical to UpdateField.
func (h *Handlers) UpdateFieldInGroup(w http.ResponseWriter, r *http.Request) {
	h.updateFieldByParam(w, r, "fid")
}

// updateFieldByParam implements the shared PATCH-field logic for UpdateField
// and UpdateFieldInGroup, reading the field ID from idParam.
func (h *Handlers) updateFieldByParam(w http.ResponseWriter, r *http.Request, idParam string) {
	id, err := httpx.ParseUUIDParam(r, idParam)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req fieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Label) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "Label is required")
		return
	}

	if !IsValidFieldType(req.FieldType) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type")
		return
	}

	if msg, ok := ValidOptionsShape(req.FieldType, req.Options); !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_options", msg)
		return
	}

	if (req.FieldType == FieldTypeSelect || req.FieldType == FieldTypeMultiselect) && len(req.Options) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "missing_options", "Select and multiselect fields require options")
		return
	}

	optionsBytes, err := marshalOptions(req.Options)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_options", "Field options are not valid JSON")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	field, err := h.Service.UpdateField(r.Context(), gen.UpdateCustomFieldParams{
		Label:          req.Label,
		FieldType:      req.FieldType,
		Options:        optionsBytes,
		EmbedPlatforms: req.EmbedPlatforms,
		Placeholder:    req.Placeholder,
		DefaultValue:   req.DefaultValue,
		IsRequired:     req.IsRequired,
		ShowInFrontend: req.ShowInFrontend,
		Filterable:     req.Filterable,
		Sortable:       req.Sortable,
		Searchable:     req.Searchable,
		Position:       req.Position,
		ID:             id,
		TenantID:       tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": field})
}

// DeleteField handles DELETE /api/v1/custom-fields/{id}.
func (h *Handlers) DeleteField(w http.ResponseWriter, r *http.Request) {
	h.deleteFieldByParam(w, r, "id")
}

// DeleteFieldInGroup handles DELETE /api/v1/admin/field-groups/{id}/fields/{fid}.
// See UpdateFieldInGroup for why the field ID is read from "fid" here.
func (h *Handlers) DeleteFieldInGroup(w http.ResponseWriter, r *http.Request) {
	h.deleteFieldByParam(w, r, "fid")
}

// deleteFieldByParam implements the shared DELETE-field logic for DeleteField
// and DeleteFieldInGroup, reading the field ID from idParam.
func (h *Handlers) deleteFieldByParam(w http.ResponseWriter, r *http.Request, idParam string) {
	id, err := httpx.ParseUUIDParam(r, idParam)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteField(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ReorderFields handles POST /api/v1/admin/field-groups/{id}/fields/reorder
// — bulk position update across a group's fields, one call per field to
// Service.UpdateFieldPosition. Mirrors the comparison package's
// ReorderComparisonItems handler (no new per-field validation beyond ID
// parsing; the database scopes each update to the given group and tenant).
func (h *Handlers) ReorderFields(w http.ResponseWriter, r *http.Request) {
	groupID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Fields []struct {
			ID       string `json:"id"`
			Position int32  `json:"position"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	for _, f := range req.Fields {
		var fieldID pgtype.UUID
		if err := fieldID.Scan(f.ID); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_field_id", "Invalid field id: "+f.ID)
			return
		}

		if err := h.Service.UpdateFieldPosition(r.Context(), fieldID, groupID, tenantID, f.Position); err != nil {
			handleServiceError(w, err)
			return
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
}

// ---------------------------------------------------------------------------
// Field Values
// ---------------------------------------------------------------------------

// ListFieldValues handles GET /api/v1/custom-field-values/{entity_type}/{entity_id}.
func (h *Handlers) ListFieldValues(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entity_type")

	var entityID pgtype.UUID
	if err := entityID.Scan(chi.URLParam(r, "entity_id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID format")
		return
	}

	values, err := h.Service.ListFieldValuesByEntity(r.Context(), entityType, entityID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// SetFieldValue handles PUT /api/v1/custom-field-values. For platform_embed
// fields the value_text URL is validated against the field's allowed platforms
// before the value is persisted.
func (h *Handlers) SetFieldValue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FieldID     string         `json:"field_id"`
		EntityType  string         `json:"entity_type"`
		EntityID    string         `json:"entity_id"`
		ValueText   *string        `json:"value_text"`
		ValueNumber json.Number    `json:"value_number"`
		ValueBool   *bool          `json:"value_bool"`
		ValueDate   string         `json:"value_date"`
		ValueJson   map[string]any `json:"value_json"` //nolint:revive // matches sqlc-generated field name
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var fieldID pgtype.UUID
	if err := fieldID.Scan(req.FieldID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid field ID format")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(req.EntityID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var numeric pgtype.Numeric
	if req.ValueNumber != "" {
		if err := numeric.Scan(string(req.ValueNumber)); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_value", "value_number is not a valid number")
			return
		}
	}

	var date pgtype.Timestamptz

	if req.ValueDate != "" {
		t, err := time.Parse(time.RFC3339, req.ValueDate)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_value", "value_date must be RFC 3339 format")
			return
		}

		date = pgtype.Timestamptz{Time: t, Valid: true}
	}

	var jsonBytes []byte

	if req.ValueJson != nil {
		var err error

		jsonBytes, err = json.Marshal(req.ValueJson)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_value", "value_json is not serializable")
			return
		}
	}

	// Fetch the field definition so we can validate the value column matches
	// the field type (e.g., text fields must write to value_text, not value_number).
	// Also used to validate platform_embed URLs against allowed platforms.
	field, err := h.Service.GetField(r.Context(), fieldID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Validate the value was written to the correct column for this field type.
	expectedCol, known := FieldTypeValueColumn(field.FieldType)
	if known {
		switch expectedCol {
		case "value_text":
			if req.ValueNumber != "" || req.ValueBool != nil || req.ValueDate != "" || req.ValueJson != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_number":
			if req.ValueText != nil || req.ValueBool != nil || req.ValueDate != "" || req.ValueJson != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_bool":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueDate != "" || req.ValueJson != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_date":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueBool != nil || req.ValueJson != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_json":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueBool != nil || req.ValueDate != "" {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		}
	}

	// Validate embed URLs before persisting so invalid links never reach the
	// database. When no EmbedResolver is configured the check is skipped,
	// allowing test setups that only exercise value storage.
	if h.Embed != nil && field.FieldType == FieldTypePlatformEmbed && req.ValueText != nil && *req.ValueText != "" {
		if _, err := h.Embed.Resolve(r.Context(), *req.ValueText, field.EmbedPlatforms); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_embed", "The provided URL is not a valid embed for this field")
			return
		}
	}

	value, err := h.Service.UpsertFieldValue(r.Context(), gen.UpsertCustomFieldValueParams{
		FieldID:     fieldID,
		EntityType:  req.EntityType,
		EntityID:    entityID,
		ValueText:   req.ValueText,
		ValueNumber: numeric,
		ValueBool:   req.ValueBool,
		ValueDate:   date,
		ValueJson:   jsonBytes,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": value})
}

// ---------------------------------------------------------------------------
// Field Permissions
// ---------------------------------------------------------------------------

// SetFieldPermission handles PUT /api/v1/custom-fields/{id}/permissions.
func (h *Handlers) SetFieldPermission(w http.ResponseWriter, r *http.Request) {
	fieldID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		UserGroupID string `json:"user_group_id"`
		CanView     bool   `json:"can_view"`
		CanEdit     bool   `json:"can_edit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var groupID pgtype.UUID
	if err := groupID.Scan(req.UserGroupID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user group ID format")
		return
	}

	if err := h.Service.UpsertFieldPermission(r.Context(), gen.UpsertFieldPermissionParams{
		FieldID:     fieldID,
		UserGroupID: groupID,
		CanView:     req.CanView,
		CanEdit:     req.CanEdit,
	}); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "set"}})
}

// ---------------------------------------------------------------------------
// Forms
// ---------------------------------------------------------------------------

// ListForms handles GET /api/v1/forms.
func (h *Handlers) ListForms(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// When space_id is provided, filter forms to that space and include
	// submission counts. The space-scoped forms builder UI relies on this.
	if spaceIDStr := r.URL.Query().Get("space_id"); spaceIDStr != "" {
		var spaceID pgtype.UUID
		if err := spaceID.Scan(spaceIDStr); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space ID format")
			return
		}

		forms, err := h.Service.ListFormsBySpace(r.Context(), tenantID, spaceID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": forms})
		return
	}

	forms, err := h.Service.ListForms(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": forms})
}

// GetForm handles GET /api/v1/forms/{id}. It returns the form together with
// its fields (via the custom field group assignment) so the frontend can
// render the form definition and field list.
func (h *Handlers) GetForm(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	form, err := h.Service.GetForm(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Fetch fields attached via the custom field group assignment.
	fields, err := h.Service.GetFormFields(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Map custom fields to the shape the frontend expects.
	type fieldJSON struct {
		ID        string   `json:"id"`
		Label     string   `json:"label"`
		Type      string   `json:"type"`
		Required  bool     `json:"required"`
		SortOrder int32    `json:"sort_order"`
		Options   []string `json:"options"`
	}

	fieldList := make([]fieldJSON, 0, len(fields))
	for _, f := range fields {
		var options []string
		if f.Options != nil {
			// Options is stored as JSONB; parse into string slice for radio/select.
			var raw []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			}
			if json.Unmarshal(f.Options, &raw) == nil && len(raw) > 0 {
				options = make([]string, len(raw))
				for i, o := range raw {
					options[i] = o.Label
				}
			}
		}

		fieldList = append(fieldList, fieldJSON{
			ID:        f.ID.String(),
			Label:     f.Label,
			Type:      f.FieldType,
			Required:  f.IsRequired,
			SortOrder: f.Position,
			Options:   options,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":          form.ID,
			"name":        form.Name,
			"description": form.Description,
			"fields":      fieldList,
			"created_at":  form.CreatedAt,
		},
	})
}

// CreateForm handles POST /api/v1/forms. API-created forms are never system
// forms and default to "draft" status when none is supplied.
func (h *Handlers) CreateForm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string  `json:"name"`
		Slug           string  `json:"slug"`
		Description    *string `json:"description"`
		IsMultiStep    bool    `json:"is_multi_step"`
		SubmitLabel    string  `json:"submit_label"`
		SuccessMessage *string `json:"success_message"`
		Status         string  `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
		return
	}

	status := req.Status
	if status == "" {
		status = "draft"
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	form, err := h.Service.CreateForm(r.Context(), gen.CreateFormParams{
		TenantID:       tenantID,
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		IsMultiStep:    req.IsMultiStep,
		SubmitLabel:    req.SubmitLabel,
		SuccessMessage: req.SuccessMessage,
		Status:         status,
		IsSystem:       false,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": form})
}

// CreateSpaceForm handles POST /api/v1/spaces/{spaceId}/forms.
// It creates a form scoped to the given space, auto-generates a slug from the
// name, and creates the form's field definitions in one transaction.
func (h *Handlers) CreateSpaceForm(w http.ResponseWriter, r *http.Request) {
	spaceID, err := httpx.ParseUUIDParam(r, "spaceId")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Fields      []struct {
			Label     string   `json:"label"`
			Type      string   `json:"type"`
			Required  bool     `json:"required"`
			SortOrder int32    `json:"sort_order"`
			Options   []string `json:"options"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	if len(req.Fields) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "At least one field is required")
		return
	}

	// Auto-generate slug from name: lowercase, replace non-alphanum with
	// hyphens, collapse runs, trim edges.
	slug := strings.ToLower(strings.TrimSpace(req.Name))
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == ' ' {
			return r
		}
		return -1
	}, slug)
	slug = strings.ReplaceAll(slug, " ", "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "form"
	}

	// Build field definitions, mapping the frontend's simple options []string
	// to the JSONB format the custom_fields table expects. Legacy frontend
	// types "checkbox" and "radio" are mapped to the canonical backend types
	// "boolean" and "select".
	fieldDefs := make([]FormFieldDef, len(req.Fields))
	for i, f := range req.Fields {
		ft := f.Type
		// Legacy mapping: checkbox → boolean, radio → select
		if ft == "checkbox" {
			ft = FieldTypeBoolean
		} else if ft == "radio" {
			ft = FieldTypeSelect
		}

		if !IsValidFieldType(ft) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type: "+f.Type)
			return
		}

		fieldDefs[i] = FormFieldDef{
			Label:     strings.TrimSpace(f.Label),
			Key:       strings.ToLower(strings.ReplaceAll(strings.TrimSpace(f.Label), " ", "_")),
			FieldType: ft,
			Required:  f.Required,
			Position:  f.SortOrder,
		}

		if f.Type == "radio" && len(f.Options) > 0 {
			type option struct {
				Value string `json:"value"`
				Label string `json:"label"`
			}
			opts := make([]option, len(f.Options))
			for j, o := range f.Options {
				opts[j] = option{Value: o, Label: o}
			}
			optsJSON, err := json.Marshal(opts)
			if err == nil {
				fieldDefs[i].Options = optsJSON
			}
		}
	}

	var desc *string
	if d := strings.TrimSpace(req.Description); d != "" {
		desc = &d
	}

	form, err := h.Service.CreateFormWithFields(r.Context(), tenantID, spaceID, req.Name, slug, desc, fieldDefs)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"id": form.ID, "slug": form.Slug},
	})
}

// UpdateForm handles PATCH /api/v1/forms/{id}. Slug is immutable and therefore
// not accepted in the request body.
func (h *Handlers) UpdateForm(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Name           string  `json:"name"`
		Description    *string `json:"description"`
		IsMultiStep    bool    `json:"is_multi_step"`
		SubmitLabel    string  `json:"submit_label"`
		SuccessMessage *string `json:"success_message"`
		Status         string  `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	form, err := h.Service.UpdateForm(r.Context(), gen.UpdateFormParams{
		Name:           req.Name,
		Description:    req.Description,
		IsMultiStep:    req.IsMultiStep,
		SubmitLabel:    req.SubmitLabel,
		SuccessMessage: req.SuccessMessage,
		Status:         req.Status,
		ID:             id,
		TenantID:       tenantID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": form})
}

// DeleteForm handles DELETE /api/v1/forms/{id}.
func (h *Handlers) DeleteForm(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteForm(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// GetFormEmbed handles GET /forms/{slug}/embed.
// Returns form definition data suitable for rendering an embedded form.
func (h *Handlers) GetFormEmbed(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	form, err := h.Service.GetFormBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":            form.ID,
			"slug":          form.Slug,
			"name":          form.Name,
			"description":   form.Description,
			"status":        form.Status,
			"is_multi_step": form.IsMultiStep,
			"submit_label":  form.SubmitLabel,
		},
	})
}

// ---------------------------------------------------------------------------
// Custom Field Value Handlers
// ---------------------------------------------------------------------------
// Form Submissions
// ---------------------------------------------------------------------------

// SubmitForm handles POST /api/v1/forms/{id}/submissions. This endpoint is
// public: authentication is optional, so an authenticated user is recorded
// when present but anonymous submissions are accepted.
func (h *Handlers) SubmitForm(w http.ResponseWriter, r *http.Request) {
	formID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		FieldValues map[string]any `json:"field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	fieldValues, err := json.Marshal(req.FieldValues)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "field_values could not be serialized")
		return
	}

	// Tenant is required even for public submissions so the row is scoped.
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Optional auth: record the submitter when a valid session exists.
	var userID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		userID = user.UserID
	}

	// Parse the client IP for abuse tracking; an unparseable address is
	// recorded as nil rather than failing the submission.
	var ipAddr *netip.Addr

	if ip := clientIP(r); ip != "" {
		if addr, err := netip.ParseAddr(ip); err == nil {
			ipAddr = &addr
		}
	}

	submission, err := h.Service.CreateFormSubmission(r.Context(), gen.CreateFormSubmissionParams{
		FormID:      formID,
		TenantID:    tenantID,
		UserID:      userID,
		IpAddress:   ipAddr,
		FieldValues: fieldValues,
		Status:      "new",
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"id": submission.ID, "status": "new"},
	})
}

// ListFormSubmissions handles GET /api/v1/forms/{id}/submissions with
// cursor-based pagination (newest first).
func (h *Handlers) ListFormSubmissions(w http.ResponseWriter, r *http.Request) {
	formID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := int32(20)

	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, perr := strconv.Atoi(raw); perr == nil && n > 0 && n <= 100 {
			limit = int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	params := gen.ListFormSubmissionsByFormParams{
		FormID:    formID,
		TenantID:  tenantID,
		PageLimit: limit,
	}

	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		if t, perr := time.Parse(time.RFC3339, cursor); perr == nil {
			params.CursorCreatedAt = pgtype.Timestamptz{Time: t, Valid: true}
		}
	}

	submissions, err := h.Service.ListFormSubmissions(r.Context(), params)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": submissions})
}

// GetFormSubmission handles GET /api/v1/admin/forms/{id}/submissions/{sub_id}.
func (h *Handlers) GetFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := httpx.ParseUUIDParam(r, "sub_id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	submission, err := h.Service.GetFormSubmission(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if submission.FormID != formID {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": submission})
}

// UpdateFormSubmission handles PATCH /api/v1/admin/forms/{id}/submissions/{sub_id}.
// Only the processing status is mutable; the submitted field values are
// immutable once recorded.
func (h *Handlers) UpdateFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := httpx.ParseUUIDParam(r, "sub_id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Status) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_fields", "Status is required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	submission, err := h.Service.GetFormSubmission(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if submission.FormID != formID {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	if err := h.Service.UpdateFormSubmissionStatus(r.Context(), id, tenantID, req.Status); err != nil {
		handleServiceError(w, err)
		return
	}

	submission.Status = req.Status
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": submission})
}

// DeleteFormSubmission handles DELETE /api/v1/admin/forms/{id}/submissions/{sub_id}.
func (h *Handlers) DeleteFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := httpx.ParseUUIDParam(r, "sub_id")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	submission, err := h.Service.GetFormSubmission(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if submission.FormID != formID {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	if err := h.Service.DeleteFormSubmission(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ListEntityTypes handles GET /custom-field-entity-types — returns available entity types for custom fields.
func (h *Handlers) ListEntityTypes(w http.ResponseWriter, r *http.Request) {
	entityTypes := []map[string]string{
		{"value": "user_profile", "label": "User Profile"},
		{"value": "content", "label": "Content"},
		{"value": "space", "label": "Space"},
		{"value": "organization", "label": "Organization"},
		{"value": "tag", "label": "Tag"},
		{"value": "ticket_type", "label": "Ticket Type"},
		{"value": "form", "label": "Form"},
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": entityTypes})
}
