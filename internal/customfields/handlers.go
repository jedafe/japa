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
	"context"
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
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

// SuperAdminChecker reports whether a user is a super-admin (bypasses
// field-level RBAC). Implemented by *rbac.RBAC.
type SuperAdminChecker interface {
	IsSuperAdmin(ctx context.Context, userID, tenantID pgtype.UUID) bool
}

// Handlers holds dependencies for custom field HTTP handlers. The Service
// wraps the reader/writer database pools; the EmbedResolver validates
// platform_embed field values at write time. The RBAC resolver enforces
// field-level visibility/editability per user group (Section 06).
// Handlers struct
type Handlers struct {
	Service *Service
	Embed   *EmbedResolver
	rbac    SuperAdminChecker // nil disables field-level RBAC (test-only)

	// AsynqClient is used to enqueue form:fire_submission_triggers after a
	// form submission is created (Section 06). Nil in tests — SubmitForm
	// gracefully skips trigger enqueuing when not wired.
	AsynqClient *asynq.Client
}

// NewHandlers creates Handlers with the given Service and EmbedResolver.
func NewHandlers(service *Service, embed *EmbedResolver) *Handlers {
	return &Handlers{Service: service, Embed: embed}
}

// SetRBAC wires the RBAC resolver used for field-level visibility/editability
// checks (Section 06). Required in production; tests may omit it.
func (h *Handlers) SetRBAC(rbac SuperAdminChecker) { h.rbac = rbac }

// SetAsynqClient wires the Asynq client used to enqueue form submission
// trigger tasks (Section 06). Required in production for trigger firing;
// tests may omit it.
func (h *Handlers) SetAsynqClient(c *asynq.Client) { h.AsynqClient = c }

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

// fieldGroupEnriched is the enriched field group response that includes the
// derived entity_type (from assignments) and the group's fields. Both the
// main app content composer and the admin panel expect these fields.
type fieldGroupEnriched struct {
	ID          pgtype.UUID        `json:"id"`
	TenantID    pgtype.UUID        `json:"tenant_id"`
	Name        string             `json:"name"`
	Slug        string             `json:"slug"`
	Description *string            `json:"description"`
	IsSystem    bool               `json:"is_system"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	EntityType  string             `json:"entity_type"`
	FieldCount  int                `json:"field_count"`
	Fields      []fieldSummary     `json:"fields"`
}

// fieldSummary is a frontend-friendly custom field representation. Unlike
// gen.CustomField (which serializes Options []byte as base64), this type
// uses json.RawMessage so the JSONB options are passed through as raw JSON.
type fieldSummary struct {
	ID           pgtype.UUID        `json:"id"`
	GroupID      pgtype.UUID        `json:"group_id"`
	Label        string             `json:"label"`
	Key          string             `json:"key"`
	FieldType    string             `json:"field_type"`
	Options      json.RawMessage    `json:"options"`
	EmbedPlatforms []string         `json:"embed_platforms"`
	Placeholder  *string            `json:"placeholder"`
	DefaultValue *string            `json:"default_value"`
	IsRequired   bool               `json:"is_required"`
	IsSystem     bool               `json:"is_system"`
	ShowInFrontend bool             `json:"show_in_frontend"`
	Filterable   bool               `json:"filterable"`
	Sortable     bool               `json:"sortable"`
	Searchable   bool               `json:"searchable"`
	Position     int32              `json:"position"`
	CreatedAt    pgtype.Timestamptz `json:"created_at"`
}

// toFieldSummary converts a gen.CustomField to a fieldSummary, preserving
// the Options JSONB as raw JSON instead of base64-encoded []byte.
func toFieldSummary(f gen.CustomField) fieldSummary {
	var opts json.RawMessage
	if f.Options != nil {
		opts = json.RawMessage(f.Options)
	}
	return fieldSummary{
		ID:             f.ID,
		GroupID:        f.GroupID,
		Label:          f.Label,
		Key:            f.Key,
		FieldType:      f.FieldType,
		Options:        opts,
		EmbedPlatforms: f.EmbedPlatforms,
		Placeholder:    f.Placeholder,
		DefaultValue:   f.DefaultValue,
		IsRequired:     f.IsRequired,
		IsSystem:       f.IsSystem,
		ShowInFrontend: f.ShowInFrontend,
		Filterable:     f.Filterable,
		Sortable:       f.Sortable,
		Searchable:     f.Searchable,
		Position:       f.Position,
		CreatedAt:      f.CreatedAt,
	}
}

// enrichFieldGroups assembles enriched field group objects from the bare
// group structs, their assignments (for entity_type), and their fields.
// Uses bulk queries to avoid N+1 per-group lookups.
func (h *Handlers) enrichFieldGroups(
	ctx context.Context,
	tenantID pgtype.UUID,
	groups []gen.CustomFieldGroup,
) []fieldGroupEnriched {
	// Bulk-fetch all assignments and fields for the tenant's groups.
	assignments, _ := h.Service.ListAllAssignmentsByTenant(ctx, tenantID)
	allFields, _ := h.Service.ListAllFieldsByTenant(ctx, tenantID)

	// Build lookup maps: group_id → entity_type (from first assignment),
	// group_id → fields[].
	assignmentByGroup := make(map[pgtype.UUID]string)
	for _, a := range assignments {
		if _, exists := assignmentByGroup[a.GroupID]; !exists {
			assignmentByGroup[a.GroupID] = EntityTypeFromClass(a.EntityClass)
		}
	}

	fieldsByGroup := make(map[pgtype.UUID][]fieldSummary)
	for _, f := range allFields {
		fieldsByGroup[f.GroupID] = append(fieldsByGroup[f.GroupID], toFieldSummary(f))
	}

	enriched := make([]fieldGroupEnriched, 0, len(groups))
	for _, g := range groups {
		entityType := assignmentByGroup[g.ID]
		fields := fieldsByGroup[g.ID]
		if fields == nil {
			fields = []fieldSummary{}
		}
		enriched = append(enriched, fieldGroupEnriched{
			ID:          g.ID,
			TenantID:    g.TenantID,
			Name:        g.Name,
			Slug:        g.Slug,
			Description: g.Description,
			IsSystem:    g.IsSystem,
			CreatedAt:   g.CreatedAt,
			EntityType:  entityType,
			FieldCount:  len(fields),
			Fields:      fields,
		})
	}
	return enriched
}

// ListFieldGroups handles GET /api/v1/custom-field-groups.
func (h *Handlers) ListFieldGroups(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	entityClass := r.URL.Query().Get("entity_class")

	var groups []gen.CustomFieldGroup
	var err error

	if entityClass != "" {
		groups, err = h.Service.ListFieldGroupsByEntityClass(r.Context(), tenantID, entityClass)
	} else {
		groups, err = h.Service.ListFieldGroups(r.Context(), tenantID)
	}
	if err != nil {
		handleServiceError(w, err)
		return
	}

	enriched := h.enrichFieldGroups(r.Context(), tenantID, groups)
	writeJSON(w, http.StatusOK, map[string]any{"data": enriched})
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

	if msg, ok := ValidOptionsShape(req.FieldType, req.Options); !ok {
		writeError(w, http.StatusBadRequest, "invalid_options", msg)
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

	if msg, ok := ValidOptionsShape(req.FieldType, req.Options); !ok {
		writeError(w, http.StatusBadRequest, "invalid_options", msg)
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

	// Accept the §06 canonical shape { ordered_field_ids: uuid[] } where
	// position is assigned by index, as well as the legacy shape
	// { fields: [{id, position}] } for backward compatibility with existing
	// admin frontend callers.
	var req struct {
		OrderedFieldIDs []string `json:"ordered_field_ids"`
		Fields          []struct {
			ID       string `json:"id"`
			Position int32   `json:"position"`
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

	// §06 canonical shape: ordered_field_ids. Position is the array index.
	if len(req.OrderedFieldIDs) > 0 {
		for i, idStr := range req.OrderedFieldIDs {
			var fieldID pgtype.UUID
			if err := fieldID.Scan(idStr); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_field_id", "Invalid field id: "+idStr)
				return
			}
			if err := h.Service.UpdateFieldPosition(r.Context(), fieldID, groupID, tenantID, int32(i)); err != nil {
				handleServiceError(w, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
		return
	}

	// Legacy shape: fields with explicit positions.
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
// Field-level RBAC (Section 06) is enforced: non-super-admin users only see
// values for fields their user group has can_view=true on.
func (h *Handlers) ListFieldValues(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entity_type")

	var entityID pgtype.UUID
	if err := entityID.Scan(chi.URLParam(r, "entity_id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID format")
		return
	}

	// Field-level RBAC (Section 06): super-admins see all fields; other users
	// only see fields their user group has can_view=true on.
	user := auth.FromContext(r.Context())
	if h.rbac != nil && user != nil && user.UserID.Valid {
		tenantID, ok := h.requireTenant(w, r)
		if !ok {
			return
		}
		if !h.rbac.IsSuperAdmin(r.Context(), user.UserID, tenantID) {
			visibleIDs, err := h.Service.ListVisibleFieldIDsForUser(r.Context(), user.UserID)
			if err != nil {
				handleServiceError(w, err)
				return
			}
			values, err := h.Service.ListFieldValuesByEntityForUser(r.Context(), tenantID, entityType, entityID, visibleIDs)
			if err != nil {
				handleServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": values})
			return
		}
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

	// Fetch the field definition so we can validate the value column matches
	// the field type (e.g., text fields must write to value_text, not value_number).
	// Also used to validate platform_embed URLs against allowed platforms.
	field, err := h.Service.GetField(r.Context(), fieldID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Field-level RBAC (Section 06): super-admins bypass; other users must
	// have can_edit=true on this field via their user group membership.
	if h.rbac != nil {
		user := auth.FromContext(r.Context())
		if user != nil && user.UserID.Valid {
			if !h.rbac.IsSuperAdmin(r.Context(), user.UserID, tenantID) {
				_, canEdit, pErr := h.Service.CheckFieldPermissionForUser(r.Context(), fieldID, user.UserID)
				if pErr != nil {
					handleServiceError(w, pErr)
					return
				}
				if !canEdit {
					writeError(w, http.StatusForbidden, "field_not_editable", "You do not have permission to edit this custom field")
					return
				}
			}
		}
	}

	// Validate the value was written to the correct column for this field type.
	expectedCol, known := FieldTypeValueColumn(field.FieldType)
	if known {
		switch expectedCol {
		case "value_text":
			if req.ValueNumber != "" || req.ValueBool != nil || req.ValueDate != "" || req.ValueJson != nil {
				writeError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_number":
			if req.ValueText != nil || req.ValueBool != nil || req.ValueDate != "" || req.ValueJson != nil {
				writeError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_bool":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueDate != "" || req.ValueJson != nil {
				writeError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_date":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueBool != nil || req.ValueJson != nil {
				writeError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_json":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueBool != nil || req.ValueDate != "" {
				writeError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		}
	}

	// Validate embed URLs before persisting so invalid links never reach the
	// database. When no EmbedResolver is configured the check is skipped,
	// allowing test setups that only exercise value storage.
	if h.Embed != nil && field.FieldType == FieldTypePlatformEmbed && req.ValueText != nil && *req.ValueText != "" {
		if _, err := h.Embed.Resolve(r.Context(), *req.ValueText, field.EmbedPlatforms); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_embed", "The provided URL is not a valid embed for this field")
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

	writeJSON(w, http.StatusOK, map[string]any{"data": value})
}

// ---------------------------------------------------------------------------
// Field Permissions
// ---------------------------------------------------------------------------

// SetFieldPermission handles PUT /api/v1/custom-fields/{id}/permissions.
//
// BUG-015/BUG-016 note: field-permission checks (CheckFieldPermissionForUser,
// ListVisibleFieldIDsForUser) are cached in Valkey under
// jc:field_perm:{user_id} with a 5-minute TTL. This handler upserts a
// permission for a user_group (not a specific user), so invalidating the
// cache would require finding all users in the group — the 5-minute TTL
// is the staleness bound. If a cache layer is added later that requires
// immediate invalidation, query user_group_members here and call
// Service.InvalidateFieldPermissionCache for each member.
// ListFieldPermissions handles GET /custom-fields/{id}/permissions — returns
// all RBAC permission rows for the given field (one per user group).
func (h *Handlers) ListFieldPermissions(w http.ResponseWriter, r *http.Request) {
	fieldID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	perms, err := h.Service.ListPermissionsByField(r.Context(), fieldID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": perms})
}

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

	// When space_id is provided, filter forms to that space and include
	// submission counts. The space-scoped forms builder UI relies on this.
	if spaceIDStr := r.URL.Query().Get("space_id"); spaceIDStr != "" {
		var spaceID pgtype.UUID
		if err := spaceID.Scan(spaceIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space ID format")
			return
		}

		forms, err := h.Service.ListFormsBySpace(r.Context(), tenantID, spaceID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": forms})
		return
	}

	forms, err := h.Service.ListForms(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": forms})
}

// GetForm handles GET /api/v1/forms/{id}. It returns the form together with
// its fields (via the custom field group assignment) so the frontend can
// render the form definition and field list.
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

	// Also return field_group_id for frontend field management.
	fieldGroupID := h.Service.GetFormGroupID(r.Context(), id)

	// Return all form fields including slug, status etc.
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":             form.ID,
			"name":           form.Name,
			"description":    form.Description,
			"slug":           form.Slug,
			"status":         form.Status,
			"is_multi_step":  form.IsMultiStep,
			"submit_label":   form.SubmitLabel,
			"success_message": form.SuccessMessage,
			"is_system":      form.IsSystem,
			"fields":         fieldList,
			"field_group_id": fieldGroupID,
			"space_id":       form.SpaceID,
			"created_at":     form.CreatedAt,
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

	// Auto-create a backing field group so the admin UI can manage fields.
	_ = h.Service.EnsureFormGroup(r.Context(), tenantID, form.ID, req.Name, req.Slug)

	writeJSON(w, http.StatusCreated, map[string]any{"data": form})
}

// CreateSpaceForm handles POST /api/v1/spaces/{spaceId}/forms.
// It creates a form scoped to the given space, auto-generates a slug from the
// name, and creates the form's field definitions in one transaction.
func (h *Handlers) CreateSpaceForm(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "spaceId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space ID format")
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
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	if len(req.Fields) == 0 {
		writeError(w, http.StatusBadRequest, "missing_fields", "At least one field is required")
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
			writeError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type: "+f.Type)
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

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"id": form.ID, "slug": form.Slug},
	})
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

// EnsureFormGroup handles POST /api/v1/forms/{id}/ensure-field-group.
// It creates a backing field group and assignment for a form if one doesn't exist.
// This is called by the admin UI before adding fields to a form.
func (h *Handlers) EnsureFormGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Get the form to find its name/slug
	form, err := h.Service.GetForm(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Ensure the form has a backing field group
	err = h.Service.EnsureFormGroup(r.Context(), tenantID, id, form.Name, form.Slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Return the field_group_id
	fieldGroupID := h.Service.GetFormGroupID(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"field_group_id": fieldGroupID,
		},
	})
}

// GetFormEmbed handles GET /forms/{slug}/embed.
// Returns form definition data (including fields + success message) suitable
// for rendering an embedded form publicly. Fields are returned so a public
// form (e.g. /feedback) can be rendered without authentication.
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

	// Fetch fields attached via the custom field group assignment so the
	// public embed can render the full form without authentication.
	fields, err := h.Service.GetFormFields(r.Context(), form.ID, tenantID)
	if err != nil {
		slog.Warn("get form embed fields", "slug", slug, "error", err)
		fields = nil
	}

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

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":              form.ID,
			"slug":            form.Slug,
			"name":            form.Name,
			"description":     form.Description,
			"status":          form.Status,
			"is_multi_step":   form.IsMultiStep,
			"submit_label":    form.SubmitLabel,
			"success_message": form.SuccessMessage,
			"fields":          fieldList,
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

	// Enqueue form:fire_submission_triggers so the worker dispatches any
	// triggers configured on this form (Section 06). Fire-and-forget per
	// Rule 8 — a failed enqueue is logged but does not fail the submission,
	// which has already been persisted.
	h.enqueueFormTriggers(formID, tenantID, submission.ID, userID, fieldValues)

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"id": submission.ID, "status": "new"},
	})
}

// enqueueFormTriggers enqueues a form:fire_submission_triggers task for the
// given submission. It is a no-op when AsynqClient is nil (tests). Errors are
// logged, not returned — the submission is already persisted and the user
// should not be penalized for an infra issue.
func (h *Handlers) enqueueFormTriggers(formID, tenantID, submissionID, userID pgtype.UUID, fieldValues []byte) {
	if h.AsynqClient == nil {
		return
	}

	payload := map[string]string{
		"submission_id": submissionID.String(),
		"tenant_id":     tenantID.String(),
		"form_id":       formID.String(),
	}
	if userID.Valid {
		payload["user_id"] = userID.String()
	}

	// field_values is included as a raw JSON fragment so the worker can pass
	// it through to the trigger dispatcher without re-parsing.
	payloadBytes, err := json.Marshal(struct {
		SubmissionID string `json:"submission_id"`
		TenantID     string `json:"tenant_id"`
		FormID       string `json:"form_id"`
		UserID       string `json:"user_id,omitempty"`
		FieldValues  []byte `json:"field_values"`
	}{
		SubmissionID: payload["submission_id"],
		TenantID:     payload["tenant_id"],
		FormID:       payload["form_id"],
		UserID:       payload["user_id"],
		FieldValues:  fieldValues,
	})
	if err != nil {
		slog.Warn("customfields: encoding form trigger payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypeFormFireSubmissionTriggers, payloadBytes)
	if _, err := h.AsynqClient.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeFormFireSubmissionTriggers))); err != nil {
		slog.Warn("customfields: enqueuing form trigger task",
			"submission_id", submissionID.String(), "error", err)
	}
}

// ListFormSubmissions handles GET /api/v1/forms/{id}/submissions with
// offset pagination (Rule 5: ?page_size=N&page_offset=N, newest first).
// Supports an optional ?status=new|reviewed filter per §06.
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

	// Offset pagination per Rule 5 / api-response-format.md.
	pageSize := int32(20)
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		if n, perr := strconv.Atoi(raw); perr == nil && n > 0 && n <= 100 {
			pageSize = int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	pageOffset := int32(0)
	if raw := r.URL.Query().Get("page_offset"); raw != "" {
		if n, perr := strconv.Atoi(raw); perr == nil && n >= 0 {
			pageOffset = int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	// Optional status filter (§06: ?status=new|reviewed).
	var statusFilter interface{}
	if status := r.URL.Query().Get("status"); status != "" {
		statusFilter = status
	}

	params := gen.ListFormSubmissionsByFormParams{
		FormID:       formID,
		TenantID:     tenantID,
		StatusFilter: statusFilter,
		PageLimit:    pageSize,
		PageOffset:   pageOffset,
	}

	submissions, err := h.Service.ListFormSubmissions(r.Context(), params)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Total count for the pagination envelope.
	total, err := h.Service.CountFormSubmissions(r.Context(), gen.CountFormSubmissionsByFormParams{
		FormID:       formID,
		TenantID:     tenantID,
		StatusFilter: statusFilter,
	})
	if err != nil {
		total = 0
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  submissions,
		"total": total,
	})
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

// ---------------------------------------------------------------------------
// Form Submission Triggers (Section 06)
// ---------------------------------------------------------------------------

// validTriggerTypes is the set of trigger_type values supported by the form
// submission trigger system (Section 06).
var validTriggerTypes = map[string]bool{
	"assign_user_group": true,
	"send_email":        true,
	"send_webhook":      true,
	"create_ticket":     true,
	"notify_admin":      true,
}

// ListFormTriggers handles GET /forms/{id}/triggers — returns the triggers
// configured on a form, ordered by position.
func (h *Handlers) ListFormTriggers(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	triggers, err := h.Service.ListFormTriggers(r.Context(), formID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": triggers})
}

// CreateFormTrigger handles POST /forms/{id}/triggers — creates a new trigger.
func (h *Handlers) CreateFormTrigger(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		TriggerType string          `json:"trigger_type"`
		Config      json.RawMessage `json:"config"`
		Position    int32           `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if !validTriggerTypes[req.TriggerType] {
		writeError(w, http.StatusBadRequest, "invalid_trigger_type",
			"trigger_type must be one of: assign_user_group, send_email, send_webhook, create_ticket, notify_admin")
		return
	}

	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	trigger, err := h.Service.CreateFormTrigger(r.Context(), gen.CreateFormSubmissionTriggerParams{
		FormID:      formID,
		TriggerType: req.TriggerType,
		Config:      req.Config,
		Position:    req.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": trigger})
}

// UpdateFormTrigger handles PATCH /forms/{id}/triggers/{triggerId}.
func (h *Handlers) UpdateFormTrigger(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	triggerID, err := parseUUIDParam(r, "triggerId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		TriggerType string          `json:"trigger_type"`
		Config      json.RawMessage `json:"config"`
		Position    int32           `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if !validTriggerTypes[req.TriggerType] {
		writeError(w, http.StatusBadRequest, "invalid_trigger_type",
			"trigger_type must be one of: assign_user_group, send_email, send_webhook, create_ticket, notify_admin")
		return
	}

	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	trigger, err := h.Service.UpdateFormTrigger(r.Context(), gen.UpdateFormSubmissionTriggerParams{
		ID:          triggerID,
		FormID:      formID,
		TriggerType: req.TriggerType,
		Config:      req.Config,
		Position:    req.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": trigger})
}

// DeleteFormTrigger handles DELETE /forms/{id}/triggers/{triggerId}.
func (h *Handlers) DeleteFormTrigger(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	triggerID, err := parseUUIDParam(r, "triggerId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteFormTrigger(r.Context(), triggerID, formID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ReorderFormTriggers handles POST /forms/{id}/triggers/reorder — updates the
// position of each trigger by index in the trigger_ids array.
func (h *Handlers) ReorderFormTriggers(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		TriggerIDs []string `json:"trigger_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if len(req.TriggerIDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_body", "trigger_ids must not be empty")
		return
	}

	triggerIDs := make([]pgtype.UUID, 0, len(req.TriggerIDs))
	for _, idStr := range req.TriggerIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_id", "Invalid trigger ID format")
			return
		}
		triggerIDs = append(triggerIDs, id)
	}

	if err := h.Service.ReorderFormTriggers(r.Context(), formID, triggerIDs); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ListEntityTypes handles GET /custom-field-entity-types — returns available
// entity types for custom fields, read from the custom_field_entity_types
// table (tenant-scoped). Falls back to a hardcoded list if the table is
// empty (e.g. before the seed migration runs).
func (h *Handlers) ListEntityTypes(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	types, err := h.Service.ListEntityTypes(r.Context(), tenantID)
	if err != nil || len(types) == 0 {
		// Fallback to the hardcoded list if the table is empty or the
		// query fails (e.g. migration not yet applied).
		fallback := []map[string]string{
			{"value": "user_profile", "label": "User Profile"},
			{"value": "content", "label": "Content"},
			{"value": "space", "label": "Space"},
			{"value": "organization", "label": "Organization"},
			{"value": "tag", "label": "Tag"},
			{"value": "ticket_type", "label": "Ticket Type"},
			{"value": "form", "label": "Form"},
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": fallback})
		return
	}

	// Map DB rows to the {value, label} shape the frontend expects.
	result := make([]map[string]string, 0, len(types))
	for _, t := range types {
		result = append(result, map[string]string{
			"value": t.Value,
			"label": t.Label,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ---------------------------------------------------------------------------
// Custom Field Entity Types CRUD (Section 06)
// ---------------------------------------------------------------------------

// ListEntityTypesAdmin handles GET /admin/custom-field-entity-types — returns
// the full entity type rows (including is_system, sort_order) for the admin UI.
func (h *Handlers) ListEntityTypesAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	types, err := h.Service.ListEntityTypes(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}

// CreateEntityType handles POST /admin/custom-field-entity-types.
func (h *Handlers) CreateEntityType(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		Value     string `json:"value"`
		Label     string `json:"label"`
		SortOrder int32  `json:"sort_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Value == "" || req.Label == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "value and label are required")
		return
	}

	et, err := h.Service.CreateEntityType(r.Context(), gen.CreateCustomFieldEntityTypeParams{
		TenantID:  tenantID,
		Value:     req.Value,
		Label:     req.Label,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": et})
}

// UpdateEntityType handles PATCH /admin/custom-field-entity-types/{id}.
func (h *Handlers) UpdateEntityType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		Label     string `json:"label"`
		SortOrder int32  `json:"sort_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Label == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "label is required")
		return
	}

	et, err := h.Service.UpdateEntityType(r.Context(), gen.UpdateCustomFieldEntityTypeParams{
		ID:        id,
		TenantID:  tenantID,
		Label:     req.Label,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": et})
}

// DeleteEntityType handles DELETE /admin/custom-field-entity-types/{id}.
// System entity types (is_system=true) cannot be deleted.
func (h *Handlers) DeleteEntityType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteEntityType(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Guide Category Admin Handlers (Section 14 — Guide Content Type System)
// ---------------------------------------------------------------------------

// ListGuideCategoriesAdmin handles GET /admin/guide-categories.
func (h *Handlers) ListGuideCategoriesAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	cats, err := h.Service.ListGuideCategories(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// CreateGuideCategoryAdmin handles POST /admin/guide-categories.
// Creates a guide category (entity_type + field group + assignment) in one
// transaction. After creation, the admin can add fields to the group via the
// existing custom field group editor (/custom-fields/{groupId}).
func (h *Handlers) CreateGuideCategoryAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var req struct {
		Slug  string `json:"slug"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	if req.Slug == "" || req.Label == "" {
		writeError(w, http.StatusBadRequest, "invalid_body", "slug and label are required")
		return
	}
	cat, err := h.Service.CreateGuideCategory(r.Context(), tenantID, CreateGuideCategoryInput{
		Slug:  req.Slug,
		Label: req.Label,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": cat})
}

// DeleteGuideCategoryAdmin handles DELETE /admin/guide-categories/{slug}.
// System categories (seeded) cannot be deleted.
func (h *Handlers) DeleteGuideCategoryAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "invalid_slug", "Slug is required")
		return
	}
	if err := h.Service.DeleteGuideCategory(r.Context(), tenantID, slug); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
