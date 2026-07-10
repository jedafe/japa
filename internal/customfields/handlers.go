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
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	errpkg "github.com/jedafe/japa/internal/errors"
	"time"

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

// writeJSON serializes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

// writeError writes a JSON error response in the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{
		Error: errorBody{Code: code, Message: message},
	})
}

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
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
	case errors.Is(err, errpkg.ErrDuplicate):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": groups})
}

// GetFieldGroup handles GET /api/v1/custom-field-groups/{id}. It returns the
// group together with its fields so callers can render a complete editor in a
// single round trip.
func (h *Handlers) GetFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	writeJSON(w, http.StatusOK, map[string]any{
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

	group, err := h.Service.CreateFieldGroup(r.Context(), tenantID, req.Name, req.Slug, req.Description, false)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": group})
}

// UpdateFieldGroup handles PATCH /api/v1/custom-field-groups/{id}.
func (h *Handlers) UpdateFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
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

	group, err := h.Service.UpdateFieldGroup(r.Context(), id, tenantID, req.Name, req.Description)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": group})
}

// DeleteFieldGroup handles DELETE /api/v1/custom-field-groups/{id}.
func (h *Handlers) DeleteFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
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
	groupID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": fields})
}

// CreateField handles POST /api/v1/custom-field-groups/{id}/fields.
func (h *Handlers) CreateField(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req fieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Label) == "" || strings.TrimSpace(req.Key) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Label and key are required")
		return
	}

	if !IsValidFieldType(req.FieldType) {
		writeError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type")
		return
	}

	if (req.FieldType == FieldTypeSelect || req.FieldType == FieldTypeMultiselect) && len(req.Options) == 0 {
		writeError(w, http.StatusBadRequest, "missing_options", "Select and multiselect fields require options")
		return
	}

	optionsBytes, err := marshalOptions(req.Options)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_options", "Field options are not valid JSON")
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

	writeJSON(w, http.StatusCreated, map[string]any{"data": field})
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
	id, err := parseUUIDParam(r, idParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req fieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Label) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Label is required")
		return
	}

	if !IsValidFieldType(req.FieldType) {
		writeError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type")
		return
	}

	if (req.FieldType == FieldTypeSelect || req.FieldType == FieldTypeMultiselect) && len(req.Options) == 0 {
		writeError(w, http.StatusBadRequest, "missing_options", "Select and multiselect fields require options")
		return
	}

	optionsBytes, err := marshalOptions(req.Options)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_options", "Field options are not valid JSON")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": field})
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
	id, err := parseUUIDParam(r, idParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ReorderFields handles POST /api/v1/admin/field-groups/{id}/fields/reorder
// — bulk position update across a group's fields, one call per field to
// Service.UpdateFieldPosition. Mirrors the comparison package's
// ReorderComparisonItems handler (no new per-field validation beyond ID
// parsing; the database scopes each update to the given group and tenant).
func (h *Handlers) ReorderFields(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Fields []struct {
			ID       string `json:"id"`
			Position int32  `json:"position"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	for _, f := range req.Fields {
		var fieldID pgtype.UUID
		if err := fieldID.Scan(f.ID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_field_id", "Invalid field id: "+f.ID)
			return
		}

		if err := h.Service.UpdateFieldPosition(r.Context(), fieldID, groupID, tenantID, f.Position); err != nil {
			handleServiceError(w, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
}

// ---------------------------------------------------------------------------
// Field Values
// ---------------------------------------------------------------------------

// ListFieldValues handles GET /api/v1/custom-field-values/{entity_type}/{entity_id}.
func (h *Handlers) ListFieldValues(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entity_type")

	var entityID pgtype.UUID
	if err := entityID.Scan(chi.URLParam(r, "entity_id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID format")
		return
	}

	values, err := h.Service.ListFieldValuesByEntity(r.Context(), entityType, entityID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": values})
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var fieldID pgtype.UUID
	if err := fieldID.Scan(req.FieldID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid field ID format")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(req.EntityID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var numeric pgtype.Numeric
	if req.ValueNumber != "" {
		if err := numeric.Scan(string(req.ValueNumber)); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_value", "value_number is not a valid number")
			return
		}
	}

	var date pgtype.Timestamptz

	if req.ValueDate != "" {
		t, err := time.Parse(time.RFC3339, req.ValueDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_value", "value_date must be RFC 3339 format")
			return
		}

		date = pgtype.Timestamptz{Time: t, Valid: true}
	}

	var jsonBytes []byte

	if req.ValueJson != nil {
		var err error

		jsonBytes, err = json.Marshal(req.ValueJson)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_value", "value_json is not serializable")
			return
		}
	}

	// Validate embed URLs before persisting so invalid links never reach the
	// database. When no EmbedResolver is configured the check is skipped,
	// allowing test setups that only exercise value storage.
	if h.Embed != nil {
		field, err := h.Service.GetField(r.Context(), fieldID, tenantID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if field.FieldType == FieldTypePlatformEmbed && req.ValueText != nil && *req.ValueText != "" {
			if _, err := h.Embed.Resolve(r.Context(), *req.ValueText, field.EmbedPlatforms); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_embed", "The provided URL is not a valid embed for this field")
				return
			}
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

	writeJSON(w, http.StatusOK, map[string]any{"data": value})
}

// ---------------------------------------------------------------------------
// Field Permissions
// ---------------------------------------------------------------------------

// SetFieldPermission handles PUT /api/v1/custom-fields/{id}/permissions.
func (h *Handlers) SetFieldPermission(w http.ResponseWriter, r *http.Request) {
	fieldID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		UserGroupID string `json:"user_group_id"`
		CanView     bool   `json:"can_view"`
		CanEdit     bool   `json:"can_edit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var groupID pgtype.UUID
	if err := groupID.Scan(req.UserGroupID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user group ID format")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "set"}})
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

	forms, err := h.Service.ListForms(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": forms})
}

// GetForm handles GET /api/v1/forms/{id}. It returns the form together with its
// steps so a single request can render a complete multi-step editor.
func (h *Handlers) GetForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	steps, err := h.Service.ListFormSteps(r.Context(), form.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"form": form, "steps": steps},
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
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

	writeJSON(w, http.StatusCreated, map[string]any{"data": form})
}

// UpdateForm handles PATCH /api/v1/forms/{id}. Slug is immutable and therefore
// not accepted in the request body.
func (h *Handlers) UpdateForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": form})
}

// DeleteForm handles DELETE /api/v1/forms/{id}.
func (h *Handlers) DeleteForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
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
		writeError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	form, err := h.Service.GetFormBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":          form.ID,
			"slug":        form.Slug,
			"name":        form.Name,
			"description": form.Description,
			"status":      form.Status,
			"is_multi_step": form.IsMultiStep,
			"submit_label": form.SubmitLabel,
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
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		FieldValues map[string]any `json:"field_values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	fieldValues, err := json.Marshal(req.FieldValues)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "field_values could not be serialized")
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

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"id": submission.ID, "status": "new"},
	})
}

// ListFormSubmissions handles GET /api/v1/forms/{id}/submissions with
// cursor-based pagination (newest first).
func (h *Handlers) ListFormSubmissions(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": submissions})
}

// GetFormSubmission handles GET /api/v1/admin/forms/{id}/submissions/{sub_id}.
func (h *Handlers) GetFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := parseUUIDParam(r, "sub_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": submission})
}

// UpdateFormSubmission handles PATCH /api/v1/admin/forms/{id}/submissions/{sub_id}.
// Only the processing status is mutable; the submitted field values are
// immutable once recorded.
func (h *Handlers) UpdateFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := parseUUIDParam(r, "sub_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Status) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Status is required")
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
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	if err := h.Service.UpdateFormSubmissionStatus(r.Context(), id, tenantID, req.Status); err != nil {
		handleServiceError(w, err)
		return
	}

	submission.Status = req.Status
	writeJSON(w, http.StatusOK, map[string]any{"data": submission})
}

// DeleteFormSubmission handles DELETE /api/v1/admin/forms/{id}/submissions/{sub_id}.
func (h *Handlers) DeleteFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := parseUUIDParam(r, "sub_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	if err := h.Service.DeleteFormSubmission(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ListEntityTypes handles GET /custom-field-entity-types — returns available entity types for custom fields.
func (h *Handlers) ListEntityTypes(w http.ResponseWriter, r *http.Request) {
	entityTypes := []map[string]string{
		{ "value": "user_profile", "label": "User Profile" },
		{ "value": "content", "label": "Content" },
		{ "value": "space", "label": "Space" },
		{ "value": "organization", "label": "Organization" },
		{ "value": "tag", "label": "Tag" },
		{ "value": "ticket_type", "label": "Ticket Type" },
		{ "value": "form", "label": "Form" },
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entityTypes})
}
