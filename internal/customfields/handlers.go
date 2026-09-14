// Package customfields — HTTP handlers for the Universal Custom Field
// Architecture.
// // Every handler is a method on Handlers so dependencies (Service and
// EmbedResolver) are injected via the constructor, never read from globals.
// Responses follow the JapaConnect API envelope: success bodies are wrapped
// in {"data": ...} and errors in {"error":{"code":"...","message":"..."}}.
// Permission enforcement is handled by middleware wired at the router level;
// these handlers focus on parsing, validation, and Service delegation.
package customfields

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	errpkg "github.com/jedafe/japa/internal/errors"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/payments"
	"github.com/jedafe/japa/internal/wallet"
	"github.com/jedafe/japa/internal/worker"
)

// SuperAdminChecker reports whether a user is a super-admin (bypasses
// field-level RBAC). Implemented by *rbac.RBAC.
type SuperAdminChecker interface {
	IsSuperAdmin(ctx context.Context, userID pgtype.UUID) bool
}

// Handlers holds dependencies for custom field HTTP handlers. The Service
// wraps the reader/writer database pools; the EmbedResolver validates
// platform_embed field values at write time. The RBAC resolver enforces
// field-level visibility/editability per user group (Section 06).
// Handlers struct.
type Handlers struct {
	Service *Service
	Embed   *EmbedResolver
	rbac    SuperAdminChecker // nil disables field-level RBAC (test-only)

	// AsynqClient is used to enqueue form:fire_submission_triggers after a
	// form submission is created (Section 06). Nil in tests — SubmitForm
	// gracefully skips trigger enqueuing when not wired.
	AsynqClient *asynq.Client

	// WalletService is used to deduct credits for paid form submissions
	// (Section 06 Paid Form Submissions). Nil in tests — paid submissions
	// are not testable without it.
	wallet WalletSpender

	// PaymentsService is used to create payment intents for gateway-paid
	// form submissions (Section 06). Nil in tests.
	payments PaymentCreator

	// Settings is used to read system_settings for form fee overrides.
	// Nil in tests — the form's submission_fee_credits column is used directly.
	settings SettingsReader

	// richTextSanitizer sanitizes HTML in rich_text field values to prevent
	// stored XSS (BUG-001). Injected from server.go to avoid an import cycle
	// between customfields and content. When nil, rich_text values are stored
	// as-is (test-only — production MUST wire this).
	richTextSanitizer RichTextSanitizer
}

// RichTextSanitizer sanitizes user-supplied HTML for safe storage and display.
// Implemented by content.SanitizeHTML; injected via SetRichTextSanitizer to
// avoid an import cycle (customfields cannot import content).
type RichTextSanitizer func(raw string) string

// WalletSpender deducts credits from a user's wallet.
type WalletSpender interface {
	SpendCredits(ctx context.Context, tenantID, userID pgtype.UUID, input wallet.SpendCreditsInput) (int64, error)
}

// PaymentCreator creates payment intents for paid submissions.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, tenantID, userID pgtype.UUID, input payments.CreatePaymentInput) (payments.CreatePaymentResult, error)
}

// SettingsReader reads system_settings values.
type SettingsReader interface {
	GetSetting(ctx context.Context, key string) (string, error)
	GetIntOr(ctx context.Context, key string, fallback int) int
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

// SetWalletService wires the wallet service for paid form submissions
// (Section 06 Paid Form Submissions).
func (h *Handlers) SetWalletService(w WalletSpender) { h.wallet = w }

// SetPaymentsService wires the payments service for gateway-paid form
// submissions (Section 06 Paid Form Submissions).
func (h *Handlers) SetPaymentsService(p PaymentCreator) { h.payments = p }

// SetSettings wires the settings reader for system_settings lookups
// (Section 06 Paid Form Submissions — fee overrides).
func (h *Handlers) SetSettings(s SettingsReader) { h.settings = s }

// SetRichTextSanitizer wires the HTML sanitizer used to scrub rich_text
// field values before storage (BUG-001 — stored XSS prevention). Required
// in production; tests may omit it.
func (h *Handlers) SetRichTextSanitizer(s RichTextSanitizer) { h.richTextSanitizer = s }

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
}

// parseIPPrefix parses an IP string into a netip.Prefix.
// Plain IPs are expanded to their host-prefix (e.g. 192.168.0.1/32, ::1/128).
func parseIPPrefix(s string) (netip.Prefix, error) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, nil
	}

	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}

	bits := 128
	if addr.Is4() {
		bits = 32
	}
	return addr.Prefix(bits)
}

// requireTenant resolves the tenant from the request context and writes a 503
// error when none is available. The boolean return is false when the caller
// should abort (the error has already been written).
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response. It centralizes the not-found, duplicate, and fallback mapping so
// every handler stays consistent.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// ---------------------------------------------------------------------------
// Custom Field Groups
// ---------------------------------------------------------------------------

// fieldGroupEnriched is the enriched field group response that includes the
// derived entity_type (from assignments) and the group's fields. Both the
// main app content composer and the admin panel expect these fields.
type fieldGroupEnriched struct {
	ID          pgtype.UUID `json:"id"`
	TenantID    pgtype.UUID
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
	ID             pgtype.UUID        `json:"id"`
	GroupID        pgtype.UUID        `json:"group_id"`
	Label          string             `json:"label"`
	Key            string             `json:"key"`
	FieldType      string             `json:"field_type"`
	Options        json.RawMessage    `json:"options"`
	EmbedPlatforms []string           `json:"embed_platforms"`
	Placeholder    *string            `json:"placeholder"`
	DefaultValue   *string            `json:"default_value"`
	IsRequired     bool               `json:"is_required"`
	IsSystem       bool               `json:"is_system"`
	ShowInFrontend bool               `json:"show_in_frontend"`
	Filterable     bool               `json:"filterable"`
	Sortable       bool               `json:"sortable"`
	Searchable     bool               `json:"searchable"`
	Position       int32              `json:"position"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
}

// toFieldSummary converts a gen.CustomField to a fieldSummary, preserving
// the Options JSONB as raw JSON instead of base64-encoded []byte.
func toFieldSummary(f gen.ListAllCustomFieldsByTenantRow) fieldSummary {
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

// toFieldSummaryFromGroup converts a gen.ListCustomFieldsByGroupRow to a
// fieldSummary, preserving the Options JSONB as raw JSON instead of
// base64-encoded []byte. This is the group-scoped variant of toFieldSummary.
func toFieldSummaryFromGroup(f gen.ListCustomFieldsByGroupRow) fieldSummary {
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

	groups []gen.CustomFieldGroup,
) []fieldGroupEnriched {
	// Bulk-fetch all assignments and fields for the tenant's groups.
	assignments, _ := h.Service.ListAllAssignmentsByTenant(ctx)
	allFields, _ := h.Service.ListAllFieldsByTenant(ctx)

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
			ID: g.ID,

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
// It accepts optional query params:
// - entity_class: exact match on the assignment entity_class column
// - entity_type: simplified token (e.g. "content", "user_profile") that
// is matched against the derived EntityTypeFromClass value
// - q: case-insensitive search on group name, slug, and description
// - sort: "key:dir" e.g. "name:asc", "created_at:desc"
// - page: 1-based page number (default 1)
// - page_size: page size (default 25, max 100)
// // When both entity_class and entity_type are provided, entity_class takes
// precedence. The entity_type filter, search, sort, and pagination are all
// applied post-enrichment so they work with the derived/simplified values
// the frontend uses (Section 06).
func (h *Handlers) ListFieldGroups(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	entityClass := r.URL.Query().Get("entity_class")
	entityType := r.URL.Query().Get("entity_type")
	searchQuery := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	sortParam := r.URL.Query().Get("sort")

	page := 1

	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	pageSize := 25

	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}

	var groups []gen.CustomFieldGroup

	var err error

	if entityClass != "" {
		groups, err = h.Service.ListFieldGroupsByEntityClass(r.Context(), entityClass)
	} else {
		groups, err = h.Service.ListFieldGroups(r.Context())
	}

	if err != nil {
		handleServiceError(w, err)
		return
	}

	enriched := h.enrichFieldGroups(r.Context(), groups)

	// Post-filter by simplified entity_type token when requested.
	if entityType != "" {
		filtered := enriched[:0]

		for _, g := range enriched {
			if g.EntityType == entityType {
				filtered = append(filtered, g)
			}
		}

		enriched = filtered
	}

	// Post-filter by search query (case-insensitive on name, slug, description).
	if searchQuery != "" {
		filtered := enriched[:0]

		for _, g := range enriched {
			if strings.Contains(strings.ToLower(g.Name), searchQuery) ||
				strings.Contains(strings.ToLower(g.Slug), searchQuery) ||
				(g.Description != nil && strings.Contains(strings.ToLower(*g.Description), searchQuery)) {
				filtered = append(filtered, g)
			}
		}

		enriched = filtered
	}

	// Sort: parse "key:dir" e.g. "name:asc", "created_at:desc".
	sortKey := "created_at"
	sortDir := "desc"

	if sortParam != "" {
		parts := strings.SplitN(sortParam, ":", 2)
		if parts[0] != "" {
			sortKey = parts[0]
		}

		if len(parts) > 1 && (parts[1] == "asc" || parts[1] == "desc") {
			sortDir = parts[1]
		}
	}

	sort.Slice(enriched, func(i, j int) bool {
		less := false

		switch sortKey {
		case "name":
			less = enriched[i].Name < enriched[j].Name
		case "slug":
			less = enriched[i].Slug < enriched[j].Slug
		case "entity_type":
			less = enriched[i].EntityType < enriched[j].EntityType
		case "field_count":
			less = enriched[i].FieldCount < enriched[j].FieldCount
		case "created_at":
			less = enriched[i].CreatedAt.Time.Before(enriched[j].CreatedAt.Time)
		default:
			less = enriched[i].Name < enriched[j].Name
		}

		if sortDir == "desc" {
			return !less
		}

		return less
	})

	// Pagination: slice the enriched result.
	total := len(enriched)
	totalPages := 1

	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if totalPages < 1 {
			totalPages = 1
		}
	}

	start := (page - 1) * pageSize
	if start > total {
		start = total
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	paged := enriched[start:end]

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": paged,
		"meta": map[string]any{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// GetFieldGroup handles GET /api/v1/custom-field-groups/{id}. It returns the
// group together with its fields so callers can render a complete editor in a
// single round trip.
func (h *Handlers) GetFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	group, err := h.Service.GetFieldGroup(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	fields, err := h.Service.ListFieldsByGroup(r.Context(), group.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Convert to frontend-friendly fieldSummary so Options JSONB is
	// serialized as raw JSON, not base64-encoded []byte.
	fieldSummaries := make([]fieldSummary, 0, len(fields))
	for _, f := range fields {
		fieldSummaries = append(fieldSummaries, toFieldSummaryFromGroup(f))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"group": group, "fields": fieldSummaries},
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	group, err := h.Service.CreateFieldGroup(r.Context(), req.Name, req.Slug, req.Description, false)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": group})
}

// CreateFieldGroupAdmin handles POST /api/v1/admin/field-groups. Unlike the
// public CreateFieldGroup handler, this accepts an optional entity_type and
// slug, auto-generates the slug from the name when omitted, and creates a
// custom_field_group_assignment row so the group is immediately attached to
// the selected entity class (Section 06).
func (h *Handlers) CreateFieldGroupAdmin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		Description *string `json:"description"`
		EntityType  string  `json:"entity_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	// Auto-generate slug from name when not provided.
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(req.Name)
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	group, err := h.Service.CreateFieldGroup(r.Context(), req.Name, slug, req.Description, false)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Create the assignment row so the group is attached to the selected
	// entity class. When entity_type is empty, skip the assignment — the
	// admin can attach it later via the entity-types management page.
	entityType := strings.TrimSpace(req.EntityType)
	if entityType != "" {
		entityClass := EntityClassFromType(entityType)
		if _, err := h.Service.CreateAssignment(r.Context(), gen.CreateAssignmentParams{
			GroupID:     group.ID,
			EntityClass: entityClass,
			EntityID:    pgtype.UUID{},
			Position:    0,
		}); err != nil {
			slog.Error("creating field group assignment", "group_id", group.ID, "entity_class", entityClass, "error", err)
			// Don't fail the whole request — the group was created
			// successfully; the assignment can be retried. Log and
			// continue so the admin sees the group in the list.
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": group})
}

// slugify converts a display name into a URL-safe slug (lowercase, hyphen-
// separated). Used by CreateFieldGroupAdmin when the client omits the slug.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	// Collapse non-alphanumeric runs into single hyphens.
	var b strings.Builder

	prevDash := false

	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)

			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')

			prevDash = true
		}
	}

	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "group"
	}

	return result
}

// UpdateFieldGroup handles PATCH /api/v1/custom-field-groups/{id}.
func (h *Handlers) UpdateFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	group, err := h.Service.UpdateFieldGroup(r.Context(), id, req.Name, req.Description)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": group})
}

// DeleteFieldGroup handles DELETE /api/v1/custom-field-groups/{id}.
func (h *Handlers) DeleteFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteFieldGroup(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	fields, err := h.Service.ListFieldsByGroup(r.Context(), groupID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Convert to frontend-friendly fieldSummary so Options JSONB is
	// serialized as raw JSON, not base64-encoded []byte.
	fieldSummaries := make([]fieldSummary, 0, len(fields))
	for _, f := range fields {
		fieldSummaries = append(fieldSummaries, toFieldSummaryFromGroup(f))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": fieldSummaries})
}

// CreateField handles POST /api/v1/custom-field-groups/{id}/fields.
func (h *Handlers) CreateField(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req fieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Label) == "" || strings.TrimSpace(req.Key) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Label and key are required")
		return
	}

	if !IsValidFieldType(req.FieldType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type")
		return
	}

	if msg, ok := ValidOptionsShape(req.FieldType, req.Options); !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_options", msg)
		return
	}

	if (req.FieldType == FieldTypeSelect || req.FieldType == FieldTypeMultiselect) && len(req.Options) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_options", "Select and multiselect fields require options")
		return
	}

	optionsBytes, err := marshalOptions(req.Options)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_options", "Field options are not valid JSON")
		return
	}

	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	field, err := h.Service.CreateField(r.Context(), gen.CreateCustomFieldParams{
		GroupID: groupID,

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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": field})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req fieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Label) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Label is required")
		return
	}

	if !IsValidFieldType(req.FieldType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type")
		return
	}

	if msg, ok := ValidOptionsShape(req.FieldType, req.Options); !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_options", msg)
		return
	}

	if (req.FieldType == FieldTypeSelect || req.FieldType == FieldTypeMultiselect) && len(req.Options) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_options", "Select and multiselect fields require options")
		return
	}

	optionsBytes, err := marshalOptions(req.Options)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_options", "Field options are not valid JSON")
		return
	}

	_, ok := h.requireTenant(w, r)
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
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": field})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteField(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ReorderFields handles POST /api/v1/admin/field-groups/{id}/fields/reorder
// — bulk position update across a group's fields, one call per field to
// Service.UpdateFieldPosition. Mirrors the comparison package's
// ReorderComparisonItems handler (no new per-field validation beyond ID
// parsing; the database scopes each update to the given group and tenant).
func (h *Handlers) ReorderFields(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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
			Position int32  `json:"position"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// §06 canonical shape: ordered_field_ids. Position is the array index.
	if len(req.OrderedFieldIDs) > 0 {
		for i, idStr := range req.OrderedFieldIDs {
			var fieldID pgtype.UUID
			if err := fieldID.Scan(idStr); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_id", "Invalid field id: "+idStr)
				return
			}

			// nolint:gosec // G115: bounded
			if err := h.Service.UpdateFieldPosition(r.Context(), fieldID, groupID, int32(i)); err != nil { // #nosec //nolint:gosec // G115: bounded
				handleServiceError(w, err)
				return
			}
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})

		return
	}

	// Legacy shape: fields with explicit positions.
	for _, f := range req.Fields {
		var fieldID pgtype.UUID
		if err := fieldID.Scan(f.ID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_id", "Invalid field id: "+f.ID)
			return
		}

		if err := h.Service.UpdateFieldPosition(r.Context(), fieldID, groupID, f.Position); err != nil {
			handleServiceError(w, err)
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Field-level RBAC (Section 06): super-admins see all fields; other users
	// only see fields their user group has can_view=true on.
	user := auth.FromContext(r.Context())
	if h.rbac != nil && user != nil && user.UserID.Valid {
		if !h.rbac.IsSuperAdmin(r.Context(), user.UserID) {
			visibleIDs, err := h.Service.ListVisibleFieldIDsForUser(r.Context(), user.UserID)
			if err != nil {
				handleServiceError(w, err)
				return
			}

			values, err := h.Service.ListFieldValuesByEntityForUser(r.Context(), entityType, entityID, visibleIDs)
			if err != nil {
				handleServiceError(w, err)
				return
			}

			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": values})

			return
		}
	}

	values, err := h.Service.ListFieldValuesByEntity(r.Context(), entityType, entityID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": values})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var fieldID pgtype.UUID
	if err := fieldID.Scan(req.FieldID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid field ID format")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(req.EntityID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var numeric pgtype.Numeric
	if req.ValueNumber != "" {
		if err := numeric.Scan(string(req.ValueNumber)); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", "value_number is not a valid number")
			return
		}
	}

	var date pgtype.Timestamptz

	if req.ValueDate != "" {
		t, err := time.Parse(time.RFC3339, req.ValueDate)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", "value_date must be RFC 3339 format")
			return
		}

		date = pgtype.Timestamptz{Time: t, Valid: true}
	}

	var jsonBytes []byte

	if req.ValueJson != nil {
		var err error

		jsonBytes, err = json.Marshal(req.ValueJson)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", "value_json is not serializable")
			return
		}
	}

	// Fetch the field definition so we can validate the value column matches
	// the field type (e.g., text fields must write to value_text, not value_number).
	// Also used to validate platform_embed URLs against allowed platforms.
	field, err := h.Service.GetField(r.Context(), fieldID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Field-level RBAC (Section 06): super-admins bypass; other users must
	// have can_edit=true on this field via their user group membership.
	if h.rbac != nil {
		user := auth.FromContext(r.Context())
		if user != nil && user.UserID.Valid {
			if !h.rbac.IsSuperAdmin(r.Context(), user.UserID) {
				_, canEdit, pErr := h.Service.CheckFieldPermissionForUser(r.Context(), fieldID, user.UserID)
				if pErr != nil {
					handleServiceError(w, pErr)
					return
				}

				if !canEdit {
					errpkg.WriteError(w, http.StatusForbidden, "field_not_editable", "You do not have permission to edit this custom field")
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
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_number":
			if req.ValueText != nil || req.ValueBool != nil || req.ValueDate != "" || req.ValueJson != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_bool":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueDate != "" || req.ValueJson != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_date":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueBool != nil || req.ValueJson != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		case "value_json":
			if req.ValueText != nil || req.ValueNumber != "" || req.ValueBool != nil || req.ValueDate != "" {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", fmt.Sprintf("Field type %q writes to %s — received a value for a different column", field.FieldType, expectedCol))
				return
			}
		}
	}

	// Validate embed URLs before persisting so invalid links never reach the
	// database. When no EmbedResolver is configured the check is skipped,
	// allowing test setups that only exercise value storage.
	if h.Embed != nil && field.FieldType == FieldTypePlatformEmbed && req.ValueText != nil && *req.ValueText != "" {
		if _, err := h.Embed.Resolve(r.Context(), *req.ValueText, field.EmbedPlatforms); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_embed", "The provided URL is not a valid embed for this field")
			return
		}
	}

	// BUG-005: type-specific value validation for text-based field types.
	if req.ValueText != nil && *req.ValueText != "" {
		fd := fieldDef{Label: field.Label, Key: field.Key, FieldType: field.FieldType, Options: field.Options, IsRequired: field.IsRequired}
		if msg, ok := validateTextValue(fd, *req.ValueText); !ok {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", msg)
			return
		}
	}

	// BUG-005: validate select/multiselect values against allowed options.
	if req.ValueJson != nil && (field.FieldType == FieldTypeSelect || field.FieldType == FieldTypeMultiselect) {
		fd := fieldDef{Label: field.Label, Key: field.Key, FieldType: field.FieldType, Options: field.Options, IsRequired: field.IsRequired}
		if msg, ok := validateSelectValue(fd, req.ValueJson); !ok {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", msg)
			return
		}
	}

	// BUG-001: sanitize rich_text HTML before storage to prevent stored XSS.
	// rich_text fields write to value_text; we scrub the HTML in-place.
	if field.FieldType == FieldTypeRichText && req.ValueText != nil && *req.ValueText != "" && h.richTextSanitizer != nil {
		sanitized := h.richTextSanitizer(*req.ValueText)
		req.ValueText = &sanitized
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": value})
}

// ---------------------------------------------------------------------------
// Field Permissions
// ---------------------------------------------------------------------------

// ListFieldPermissions — SetFieldPermission handles PUT /api/v1/custom-fields/{id}/permissions.
// // BUG-015/BUG-016: field-permission checks (CheckFieldPermissionForUser,
// ListVisibleFieldIDsForUser) are cached in Valkey under
// jc:field_perm:{user_id} with a 5-minute TTL. This handler upserts a
// permission for a user_group (not a specific user), so after the write we
// invalidate the cache for every member of that group (Rule 6 /
// valkey-keys-catalog: jc:field_perm:{user_id} invalidated by SetFieldPermission).
// ListFieldPermissions handles GET /custom-fields/{id}/permissions — returns
// all RBAC permission rows for the given field (one per user group).
func (h *Handlers) ListFieldPermissions(w http.ResponseWriter, r *http.Request) {
	fieldID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	perms, err := h.Service.ListPermissionsByField(r.Context(), fieldID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": perms})
}

// SetFieldPermission is an exported method.
func (h *Handlers) SetFieldPermission(w http.ResponseWriter, r *http.Request) {
	fieldID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		UserGroupID string `json:"user_group_id"`
		CanView     bool   `json:"can_view"`
		CanEdit     bool   `json:"can_edit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var groupID pgtype.UUID
	if err := groupID.Scan(req.UserGroupID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user group ID format")
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

	// Rule 6: invalidate the field-permission cache for every member of the
	// affected group so the change is visible immediately (not after the
	// 5-minute TTL).
	h.Service.InvalidateFieldPermissionCacheForGroup(r.Context(), groupID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "set"}})
}

// DeleteFieldPermission handles DELETE /api/v1/custom-fields/{id}/permissions.
// BUG-009: completes the field-permission CRUD surface — previously only
// PUT (upsert) and GET (list) existed, so a permission could never be
// removed once set. The request body carries the user_group_id to remove.
func (h *Handlers) DeleteFieldPermission(w http.ResponseWriter, r *http.Request) {
	fieldID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		UserGroupID string `json:"user_group_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	var groupID pgtype.UUID
	if err := groupID.Scan(req.UserGroupID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user group ID format")
		return
	}

	if err := h.Service.DeleteFieldPermission(r.Context(), fieldID, groupID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Rule 6: invalidate the field-permission cache for every member of the
	// affected group.
	h.Service.InvalidateFieldPermissionCacheForGroup(r.Context(), groupID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Forms
// ---------------------------------------------------------------------------

// ListForms handles GET /api/v1/forms.
func (h *Handlers) ListForms(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// When space_id is provided, filter forms to that space and include
	// submission counts. The space-scoped forms builder UI relies on this.
	if spaceIDStr := r.URL.Query().Get("space_id"); spaceIDStr != "" {
		var spaceID pgtype.UUID
		if err := spaceID.Scan(spaceIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space ID format")
			return
		}

		forms, err := h.Service.ListFormsBySpace(r.Context(), spaceID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": forms})

		return
	}

	forms, err := h.Service.ListForms(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-010: support the ?is_system=true|false filter documented in the
	// route catalog (api-routes-catalog.md L1151). Applied post-query since
	// the ListForms SQL does not have a filtered variant.
	if isSystem := r.URL.Query().Get("is_system"); isSystem != "" {
		filtered := forms[:0]

		for _, f := range forms {
			if isSystem == "true" && f.IsSystem {
				filtered = append(filtered, f)
			} else if isSystem == "false" && !f.IsSystem {
				filtered = append(filtered, f)
			}
		}

		forms = filtered
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": forms})
}

// GetForm handles GET /api/v1/forms/{id}. It returns the form together with
// its fields (via the custom field group assignment) so the frontend can
// render the form definition and field list.
func (h *Handlers) GetForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	form, err := h.Service.GetForm(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Fetch fields attached via the custom field group assignment.
	fields, err := h.Service.GetFormFields(r.Context(), id)
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

	// Return the form's steps for multi-step forms, each with its own
	// field_group_id and fields[] so the frontend can render and manage
	// fields per step without extra round-trips.
	type stepFieldJSON struct {
		ID        string   `json:"id"`
		Label     string   `json:"label"`
		Type      string   `json:"type"`
		Required  bool     `json:"required"`
		SortOrder int32    `json:"sort_order"`
		Options   []string `json:"options"`
	}

	type stepJSON struct {
		ID           string          `json:"id"`
		Title        string          `json:"title"`
		Position     int32           `json:"position"`
		FieldGroupID *string         `json:"field_group_id"`
		Fields       []stepFieldJSON `json:"fields"`
	}

	stepList := []stepJSON{}

	if form.IsMultiStep {
		steps, stepErr := h.Service.ListFormSteps(r.Context(), id)
		if stepErr == nil {
			stepList = make([]stepJSON, 0, len(steps))

			for _, st := range steps {
				fgid := h.Service.GetStepFormGroupID(r.Context(), st.ID)

				var stepFields []stepFieldJSON

				if fgid != nil {
					rawFields, _ := h.Service.GetStepFields(r.Context(), st.ID)
					stepFields = make([]stepFieldJSON, 0, len(rawFields))

					for _, f := range rawFields {
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

						stepFields = append(stepFields, stepFieldJSON{
							ID:        f.ID.String(),
							Label:     f.Label,
							Type:      f.FieldType,
							Required:  f.IsRequired,
							SortOrder: f.Position,
							Options:   options,
						})
					}
				}

				stepList = append(stepList, stepJSON{
					ID:           st.ID.String(),
					Title:        st.Title,
					Position:     st.Position,
					FieldGroupID: fgid,
					Fields:       stepFields,
				})
			}
		}
	}

	// Return all form fields including slug, status etc.
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":              form.ID,
			"name":            form.Name,
			"description":     form.Description,
			"slug":            form.Slug,
			"status":          form.Status,
			"is_multi_step":   form.IsMultiStep,
			"submit_label":    form.SubmitLabel,
			"success_message": form.SuccessMessage,
			"is_system":       form.IsSystem,
			"fields":          fieldList,
			"field_group_id":  fieldGroupID,
			"space_id":        form.SpaceID,
			"steps":           stepList,
			"created_at":      form.CreatedAt,
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name and slug are required")
		return
	}

	status := req.Status
	if status == "" {
		status = "draft"
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	form, err := h.Service.CreateForm(r.Context(), gen.CreateFormParams{

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
	_ = h.Service.EnsureFormGroup(r.Context(), form.ID, req.Name, req.Slug)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": form})
}

// CreateSpaceForm handles POST /api/v1/spaces/{spaceId}/forms.
// It creates a form scoped to the given space, auto-generates a slug from the
// name, and creates the form's field definitions in one transaction.
func (h *Handlers) CreateSpaceForm(w http.ResponseWriter, r *http.Request) {
	spaceID, err := parseUUIDParam(r, "spaceId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}

	if len(req.Fields) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "At least one field is required")
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
		switch ft {
		case "checkbox":
			ft = FieldTypeBoolean
		case "radio":
			ft = FieldTypeSelect
		}

		if !IsValidFieldType(ft) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_type", "Unsupported field type: "+f.Type)
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

	form, err := h.Service.CreateFormWithFields(r.Context(), spaceID, req.Name, slug, desc, fieldDefs)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"id": form.ID, "slug": form.Slug},
	})
}

// UpdateForm handles PATCH /api/v1/forms/{id}. Slug is immutable and therefore
// not accepted in the request body.
func (h *Handlers) UpdateForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Name                 string  `json:"name"`
		Description          *string `json:"description"`
		IsMultiStep          bool    `json:"is_multi_step"`
		SubmitLabel          string  `json:"submit_label"`
		SuccessMessage       *string `json:"success_message"`
		Status               string  `json:"status"`
		SubmissionFeeCredits *int32  `json:"submission_fee_credits"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Name is required")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// BUG-006: preserve the existing submission_fee_credits when the request
	// does not include it. Without this, the Go zero value (0) would silently
	// reset paid forms to free on every PATCH call.
	feeCredits := int32(0)

	existing, err := h.Service.GetForm(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if req.SubmissionFeeCredits != nil {
		feeCredits = *req.SubmissionFeeCredits
	} else {
		feeCredits = existing.SubmissionFeeCredits
	}

	form, err := h.Service.UpdateForm(r.Context(), gen.UpdateFormParams{
		Name:                 req.Name,
		Description:          req.Description,
		IsMultiStep:          req.IsMultiStep,
		SubmitLabel:          req.SubmitLabel,
		SuccessMessage:       req.SuccessMessage,
		Status:               req.Status,
		SubmissionFeeCredits: feeCredits,
		ID:                   id,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": form})
}

// DeleteForm handles DELETE /api/v1/forms/{id}.
func (h *Handlers) DeleteForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteForm(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// EnsureFormGroup handles POST /api/v1/forms/{id}/ensure-field-group.
// It creates a backing field group and assignment for a form if one doesn't exist.
// This is called by the admin UI before adding fields to a form.
func (h *Handlers) EnsureFormGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Get the form to find its name/slug
	form, err := h.Service.GetForm(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Ensure the form has a backing field group
	err = h.Service.EnsureFormGroup(r.Context(), id, form.Name, form.Slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Return the field_group_id
	fieldGroupID := h.Service.GetFormGroupID(r.Context(), id)
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"field_group_id": fieldGroupID,
		},
	})
}

// GetFormEmbed handles GET /forms/{slug}/embed.
// Returns form definition data (including fields + success message) suitable
// for rendering an embedded form publicly. Fields are returned so a public
// form (e.g. /feedback) can be rendered without authentication.
// // For multi-step forms, the response includes a `steps` array where each step
// has its own `fields[]` list (fields are scoped to the step's field group).
// The flat `fields` array is still returned (as the concatenation of all step
// fields) for backward compatibility with embeds that don't yet understand
// steps.
func (h *Handlers) GetFormEmbed(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	form, err := h.Service.GetFormBySlug(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Fetch fields attached via the custom field group assignment so the
	// public embed can render the full form without authentication.
	fields, err := h.Service.GetFormFields(r.Context(), form.ID)
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

	toFieldJSON := func(f gen.ListCustomFieldsByGroupRow) fieldJSON {
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

		return fieldJSON{
			ID:        f.ID.String(),
			Label:     f.Label,
			Type:      f.FieldType,
			Required:  f.IsRequired,
			SortOrder: f.Position,
			Options:   options,
		}
	}

	fieldList := make([]fieldJSON, 0, len(fields))
	for _, f := range fields {
		fieldList = append(fieldList, toFieldJSON(f))
	}

	// For multi-step forms, build a steps[] array with per-step fields.
	type stepJSON struct {
		ID           string      `json:"id"`
		Title        string      `json:"title"`
		Position     int32       `json:"position"`
		FieldGroupID *string     `json:"field_group_id"`
		Fields       []fieldJSON `json:"fields"`
	}

	stepList := []stepJSON{}

	if form.IsMultiStep {
		steps, stepErr := h.Service.ListFormSteps(r.Context(), form.ID)
		if stepErr != nil {
			slog.Warn("get form embed steps", "slug", slug, "error", stepErr)
		} else {
			stepList = make([]stepJSON, 0, len(steps))

			for _, st := range steps {
				fgid := h.Service.GetStepFormGroupID(r.Context(), st.ID)

				var stepFields []fieldJSON

				if fgid != nil {
					rawFields, sfErr := h.Service.GetStepFields(r.Context(), st.ID)
					if sfErr != nil {
						slog.Warn("get form embed step fields", "slug", slug, "step_id", st.ID, "error", sfErr)
					}

					stepFields = make([]fieldJSON, 0, len(rawFields))
					for _, f := range rawFields {
						stepFields = append(stepFields, toFieldJSON(f))
					}
				}

				stepList = append(stepList, stepJSON{
					ID:           st.ID.String(),
					Title:        st.Title,
					Position:     st.Position,
					FieldGroupID: fgid,
					Fields:       stepFields,
				})
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":                     form.ID,
			"slug":                   form.Slug,
			"name":                   form.Name,
			"description":            form.Description,
			"status":                 form.Status,
			"is_multi_step":          form.IsMultiStep,
			"submit_label":           form.SubmitLabel,
			"success_message":        form.SuccessMessage,
			"submission_fee_credits": form.SubmissionFeeCredits,
			"fields":                 fieldList,
			"steps":                  stepList,
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
// // For paid forms (submission_fee_credits > 0), the request must include a
// payment_method field: "credit" (wallet credits) or "paystack"/"stripe"
// (payment gateway). Credits are deducted synchronously; gateway payments
// create a pending submission + payment intent and return a checkout_url.
// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
// (Paid Form Submissions).
func (h *Handlers) SubmitForm(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		FieldValues   map[string]any `json:"field_values"`
		PaymentMethod string         `json:"payment_method"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	fieldValues, err := json.Marshal(req.FieldValues)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "field_values could not be serialized")
		return
	}

	// Tenant is required even for public submissions so the row is scoped.
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Optional auth: record the submitter when a valid session exists.
	var userID pgtype.UUID

	hasUser := false

	if user := auth.FromContext(r.Context()); user != nil {
		userID = user.UserID
		hasUser = true
	}

	// Parse the client IP for abuse tracking; an unparseable address is
	// recorded as nil rather than failing the submission.
	var ipAddr *netip.Prefix

	if ip := clientip.FromRequest(r); ip != "" {
		if p, err := parseIPPrefix(ip); err == nil {
			ipAddr = &p
		}
	}

	// Look up the form to check submission_fee_credits.
	form, err := h.Service.GetForm(r.Context(), formID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-002: validate field_values against the form's field definitions
	// (required fields, type validation, unknown keys). Also sanitizes
	// rich_text HTML (BUG-001) in-place. When the form has no field
	// definitions, validation is skipped (lenient posture for legacy forms).
	if req.FieldValues != nil {
		if msg, ok := h.validateFormSubmission(r.Context(), formID, req.FieldValues); !ok {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_values", msg)
			return
		}
		// Re-marshal in case rich_text values were sanitized in-place.
		fieldValues, err = json.Marshal(req.FieldValues)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "field_values could not be serialized after validation")
			return
		}
	}

	// Determine the fee: use the form's submission_fee_credits, or override
	// from system_settings 'claim_ownership.fee_credits' if the form is the
	// claim-ownership form and the override is > 0.
	feeCredits := int64(form.SubmissionFeeCredits)

	if h.settings != nil && form.Slug == "claim-ownership" {
		override := h.settings.GetIntOr(r.Context(), "claim_ownership.fee_credits", 0)
		if override > 0 {
			feeCredits = int64(override)
		}
	}

	// Free submission — no payment needed.
	if feeCredits <= 0 {
		submission, sErr := h.Service.CreateFormSubmission(r.Context(), gen.CreateFormSubmissionParams{
			FormID: formID,

			UserID:      userID,
			IpAddress:   ipAddr,
			FieldValues: fieldValues,
			Status:      "new",
		})
		if sErr != nil {
			handleServiceError(w, sErr)
			return
		}

		h.enqueueFormTriggers(formID, tenantID, submission.ID, userID, fieldValues)

		errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
			"data": map[string]any{"id": submission.ID, "status": "new"},
		})

		return
	}

	// Paid submission — payment_method is required.
	if req.PaymentMethod == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "payment_required", "This form requires a payment method ('credit' or 'paystack' or 'stripe')")
		return
	}

	if !hasUser {
		errpkg.WriteError(w, http.StatusUnauthorized, "auth_required", "Authentication is required for paid form submissions")
		return
	}

	switch req.PaymentMethod {
	case payments.ProviderCredit:
		// Deduct credits from the user's wallet synchronously.
		if h.wallet == nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "wallet_unavailable", "Wallet service is not configured")
			return
		}

		_, spendErr := h.wallet.SpendCredits(r.Context(), tenantID, userID, wallet.SpendCreditsInput{
			Amount:      feeCredits,
			Source:      "form_submission",
			Reference:   "form_submission:" + formID.String(),
			PurposeType: "form_submission",
			PurposeID:   formID.String(),
		})
		if spendErr != nil {
			errpkg.WriteError(w, http.StatusPaymentRequired, "insufficient_credits", "Insufficient credits to pay the submission fee")
			return
		}

		// Create the submission with status='new' (payment confirmed).
		submission, sErr := h.Service.CreateFormSubmission(r.Context(), gen.CreateFormSubmissionParams{
			FormID: formID,

			UserID:      userID,
			IpAddress:   ipAddr,
			FieldValues: fieldValues,
			Status:      "new",
		})
		if sErr != nil {
			handleServiceError(w, sErr)
			return
		}

		h.enqueueFormTriggers(formID, tenantID, submission.ID, userID, fieldValues)

		errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
			"data": map[string]any{"id": submission.ID, "status": "new"},
		})

	case payments.ProviderPaystack, payments.ProviderStripe:
		// Create a payment intent via the gateway.
		if h.payments == nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "payments_unavailable", "Payments service is not configured")
			return
		}

		result, pErr := h.payments.CreatePayment(r.Context(), tenantID, userID, payments.CreatePaymentInput{
			Provider:    req.PaymentMethod,
			PurposeType: "form_submission",
			PurposeID:   formID.String(),
			Amount:      feeCredits,
			Metadata: map[string]interface{}{
				"form_id":      formID.String(),
				"tenant_id":    tenantID.String(),
				"field_values": req.FieldValues,
			},
		})
		if pErr != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "payment_intent_failed", "Failed to create payment intent")
			return
		}

		// Create the submission with status='pending_payment'.
		var paymentID pgtype.UUID
		if result.Payment.ID.Valid {
			paymentID = result.Payment.ID
		}

		submission, sErr := h.Service.CreateFormSubmission(r.Context(), gen.CreateFormSubmissionParams{
			FormID: formID,

			UserID:      userID,
			IpAddress:   ipAddr,
			FieldValues: fieldValues,
			Status:      "pending_payment",
			PaymentID:   paymentID,
		})
		if sErr != nil {
			handleServiceError(w, sErr)
			return
		}

		// Do NOT enqueue triggers yet — the webhook will confirm payment and
		// update the submission to 'new', at which point triggers fire.

		errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
			"data": map[string]any{
				"id":           submission.ID,
				"status":       "pending_payment",
				"checkout_url": result.CheckoutURL,
				"payment_id":   result.Payment.ID,
			},
		})

	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_payment_method", "payment_method must be 'credit', 'paystack', or 'stripe'")
	}
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
		TenantID     string
		FormID       string `json:"form_id"`
		UserID       string `json:"user_id,omitempty"`
		FieldValues  []byte `json:"field_values"`
	}{
		SubmissionID: payload["submission_id"],

		FormID:      payload["form_id"],
		UserID:      payload["user_id"],
		FieldValues: fieldValues,
	})
	if err != nil {
		slog.Warn("customfields: encoding form trigger payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypeFormFireSubmissionTriggers, payloadBytes)
	if _, err := h.AsynqClient.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeFormFireSubmissionTriggers))); err != nil {
		slog.Warn("customfields: enqueuing form trigger task", "submission_id", submissionID.String(), "error", err)
	}
}

// validateFormSubmission validates the submitted field_values against the
// form's field definitions (BUG-002). It checks:
// 1. Required fields (is_required=true) are present and non-empty.
// 2. Provided field keys/labels correspond to actual fields on the form.
// 3. Values match their field types (URL, email, select options).
// 4. rich_text values are sanitized (BUG-001).
// // The field_values map is keyed by field label (matching the frontend
// contract — see apps/main feedback/+page.svelte). Field keys are also
// accepted for API callers. The map is mutated in-place: rich_text values
// are replaced with their sanitized versions.
// // Returns an error message and false when validation fails, or "" and true
// when it passes. When the form has no field definitions (e.g. a legacy
// form with no assigned field group), validation is skipped — all values
// are accepted — to avoid breaking existing flows.
func (h *Handlers) validateFormSubmission(ctx context.Context, formID pgtype.UUID, fieldValues map[string]any) (string, bool) {
	fields, err := h.Service.GetFormFields(ctx, formID)
	if err != nil {
		// If we can't load fields, don't block the submission — log and
		// proceed. This matches the lenient posture for forms with no
		// field definitions.
		slog.Warn("customfields: loading form fields for validation", "form_id", formID.String(), "error", err)
		return "", true
	}

	if len(fields) == 0 {
		return "", true
	}

	// Build lookup maps: label→field and key→field.
	byLabel := make(map[string]fieldDef, len(fields))
	byKey := make(map[string]fieldDef, len(fields))

	for _, f := range fields {
		fd := fieldDef{Label: f.Label, Key: f.Key, FieldType: f.FieldType, Options: f.Options, IsRequired: f.IsRequired}
		byLabel[f.Label] = fd
		byKey[f.Key] = fd
	}

	// 1. Check required fields are present and non-empty.
	for _, f := range fields {
		if !f.IsRequired {
			continue
		}

		val, exists := lookupFieldValue(fieldValues, f.Label, f.Key)
		if !exists || isEmptyValue(val) {
			return fmt.Sprintf("Field %q is required", f.Label), false
		}
	}

	// 2. Check each provided value corresponds to a known field, and validate
	// the value against the field type.
	for rawKey, rawVal := range fieldValues {
		f, ok := byLabel[rawKey]
		if !ok {
			f, ok = byKey[rawKey]
		}

		if !ok {
			return fmt.Sprintf("Unknown field %q — this field is not defined on the form", rawKey), false
		}

		// BUG-001: sanitize rich_text HTML before storage. Mutate the map
		// in-place so the sanitized value is what gets persisted.
		if f.FieldType == FieldTypeRichText {
			if strVal, isStr := rawVal.(string); isStr && strVal != "" && h.richTextSanitizer != nil {
				fieldValues[rawKey] = h.richTextSanitizer(strVal)
			}
		}

		if msg, ok := h.validateFieldValue(f, rawVal); !ok {
			return msg, false
		}
	}

	return "", true
}

// lookupFieldValue finds a value in the map by label or key (case-sensitive).
func lookupFieldValue(m map[string]any, label, key string) (any, bool) {
	if v, ok := m[label]; ok {
		return v, true
	}

	if v, ok := m[key]; ok {
		return v, true
	}

	return nil, false
}

// isEmptyValue returns true for nil, empty string, or empty slice/array values.
func isEmptyValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(val) == ""
	case []any:
		return len(val) == 0
	case []string:
		return len(val) == 0
	default:
		return false
	}
}

// fieldDef captures the subset of a custom field definition needed for
// validation. Both gen.GetCustomFieldByIDRow and gen.ListCustomFieldsByGroupRow
// can be converted to this type, avoiding duplication of validation logic
// across the two code paths (SetFieldValue vs SubmitForm).
type fieldDef struct {
	Label      string
	Key        string
	FieldType  string
	Options    []byte
	IsRequired bool
}

// validateFieldValue validates a single field value against its field type
// (BUG-005). Returns an error message and false when validation fails.
// rich_text sanitization (BUG-001) is handled by the caller before this
// function is invoked.
func (h *Handlers) validateFieldValue(f fieldDef, raw any) (string, bool) {
	strVal, _ := raw.(string)

	switch f.FieldType {
	case FieldTypeURL:
		if strVal == "" {
			return "", true
		}

		u, err := neturl.Parse(strVal)
		if err != nil || u.Scheme == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Sprintf("Field %q must be a valid http or https URL", f.Label), false
		}

	case FieldTypeEmail:
		if strVal == "" {
			return "", true
		}

		if !strings.Contains(strVal, "@") || !strings.Contains(strVal, ".") {
			return fmt.Sprintf("Field %q must be a valid email address", f.Label), false
		}

	case FieldTypePhone:
		if strVal == "" {
			return "", true
		}
		// Basic phone validation: digits, spaces, +, -, ( ).
		cleaned := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", "+", "").Replace(strVal)
		if len(cleaned) < 7 || len(cleaned) > 15 {
			return fmt.Sprintf("Field %q must be a valid phone number (7-15 digits)", f.Label), false
		}

		for _, r := range cleaned {
			if r < '0' || r > '9' {
				return fmt.Sprintf("Field %q must be a valid phone number", f.Label), false
			}
		}

	case FieldTypeSelect:
		if strVal == "" {
			return "", true
		}

		if !valueInOptions(f.Options, strVal) {
			return fmt.Sprintf("Field %q value is not one of the allowed options", f.Label), false
		}

	case FieldTypeMultiselect:
		if strVal == "" {
			return "", true
		}
		// Multiselect can be a string (single) or an array.
		switch v := raw.(type) {
		case string:
			if !valueInOptions(f.Options, v) {
				return fmt.Sprintf("Field %q value is not one of the allowed options", f.Label), false
			}
		case []any:
			for _, item := range v {
				s, _ := item.(string)
				if !valueInOptions(f.Options, s) {
					return fmt.Sprintf("Field %q value %q is not one of the allowed options", f.Label, s), false
				}
			}
		case []string:
			for _, item := range v {
				if !valueInOptions(f.Options, item) {
					return fmt.Sprintf("Field %q value %q is not one of the allowed options", f.Label, item), false
				}
			}
		}
	}

	return "", true
}

// valueInOptions checks whether a value matches one of the field's select
// options. The options JSONB is [{value, label}]; we match against both
// value and label for flexibility (the frontend sometimes sends labels).
func valueInOptions(optionsJSON []byte, target string) bool {
	if optionsJSON == nil || target == "" {
		return false
	}

	var opts []struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}

	if json.Unmarshal(optionsJSON, &opts) != nil {
		return false
	}

	for _, o := range opts {
		if o.Value == target || o.Label == target {
			return true
		}
	}

	return false
}

// validateTextValue validates a text-column value against its field type
// (BUG-005). Used by SetFieldValue for field types that write to value_text.
// Returns an error message and false when validation fails.
func validateTextValue(field fieldDef, val string) (string, bool) {
	switch field.FieldType {
	case FieldTypeURL:
		u, err := neturl.Parse(val)
		if err != nil || u.Scheme == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Sprintf("Field %q must be a valid http or https URL", field.Label), false
		}
	case FieldTypeEmail:
		if !strings.Contains(val, "@") || !strings.Contains(val, ".") {
			return fmt.Sprintf("Field %q must be a valid email address", field.Label), false
		}
	case FieldTypePhone:
		cleaned := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", "+", "").Replace(val)
		if len(cleaned) < 7 || len(cleaned) > 15 {
			return fmt.Sprintf("Field %q must be a valid phone number (7-15 digits)", field.Label), false
		}

		for _, r := range cleaned {
			if r < '0' || r > '9' {
				return fmt.Sprintf("Field %q must be a valid phone number", field.Label), false
			}
		}
	}

	return "", true
}

// validateSelectValue validates a select/multiselect value_json against the
// field's allowed options (BUG-005). The value_json is expected to be a
// string (select) or array of strings (multiselect). Returns an error
// message and false when validation fails.
func validateSelectValue(field fieldDef, valueJson map[string]any) (string, bool) {
	// Select fields store the selected value(s) in value_json. The shape
	// can be {"value": "..."} or just a string array. We check each value
	// against the field's options.
	var values []string
	if v, ok := valueJson["value"].(string); ok {
		values = append(values, v)
	}

	if arr, ok := valueJson["values"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
	}
	// Also accept a top-level array.
	if arr, ok := valueJson[""].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
	}

	for _, v := range values {
		if !valueInOptions(field.Options, v) {
			return fmt.Sprintf("Field %q value %q is not one of the allowed options", field.Label, v), false
		}
	}

	return "", true
}

// ListFormSubmissions handles GET /api/v1/forms/{id}/submissions with
// offset pagination (Rule 5: ?page_size=N&page_offset=N, newest first).
// Supports an optional ?status=new|reviewed filter per §06.
func (h *Handlers) ListFormSubmissions(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Offset pagination per Rule 5 / api-response-format.md.
	pageSize := int32(20)

	if raw := r.URL.Query().Get("page_size"); raw != "" {
		if n, perr := strconv.Atoi(raw); perr == nil && n > 0 && n <= 100 {
			// #nosec G109 // value bounded by MaxPageSize check
			pageSize = int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
		}
	}

	pageOffset := int32(0)

	if raw := r.URL.Query().Get("page_offset"); raw != "" {
		if n, perr := strconv.Atoi(raw); perr == nil && n >= 0 {
			// #nosec G109 // value bounded by MaxPageSize check
			// #nosec G115 // value bounded by caller
			pageOffset = int32(n)
		}
	}

	// Optional status filter (§06: ?status=new|reviewed).
	var statusFilter interface{}
	if status := r.URL.Query().Get("status"); status != "" {
		statusFilter = status
	}

	params := gen.ListFormSubmissionsByFormParams{
		FormID: formID,

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
		FormID: formID,

		StatusFilter: statusFilter,
	})
	if err != nil {
		total = 0
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":  submissions,
		"total": total,
	})
}

// GetFormSubmission handles GET /api/v1/admin/forms/{id}/submissions/{sub_id}.
func (h *Handlers) GetFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := parseUUIDParam(r, "sub_id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	submission, err := h.Service.GetFormSubmission(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if submission.FormID != formID {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": submission})
}

// UpdateFormSubmission handles PATCH /api/v1/admin/forms/{id}/submissions/{sub_id}.
// Only the processing status is mutable; the submitted field values are
// immutable once recorded.
func (h *Handlers) UpdateFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := parseUUIDParam(r, "sub_id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Status) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Status is required")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	submission, err := h.Service.GetFormSubmission(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if submission.FormID != formID {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	if err := h.Service.UpdateFormSubmissionStatus(r.Context(), id, req.Status); err != nil {
		handleServiceError(w, err)
		return
	}

	submission.Status = req.Status
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": submission})
}

// DeleteFormSubmission handles DELETE /api/v1/admin/forms/{id}/submissions/{sub_id}.
func (h *Handlers) DeleteFormSubmission(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	id, err := parseUUIDParam(r, "sub_id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	submission, err := h.Service.GetFormSubmission(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if submission.FormID != formID {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}

	if err := h.Service.DeleteFormSubmission(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	triggers, err := h.Service.ListFormTriggers(r.Context(), formID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": triggers})
}

// CreateFormTrigger handles POST /forms/{id}/triggers — creates a new trigger.
func (h *Handlers) CreateFormTrigger(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		TriggerType string          `json:"trigger_type"`
		Config      json.RawMessage `json:"config"`
		Position    int32           `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if !validTriggerTypes[req.TriggerType] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_trigger_type",
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": trigger})
}

// UpdateFormTrigger handles PATCH /forms/{id}/triggers/{triggerId}.
func (h *Handlers) UpdateFormTrigger(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	triggerID, err := parseUUIDParam(r, "triggerId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		TriggerType string          `json:"trigger_type"`
		Config      json.RawMessage `json:"config"`
		Position    int32           `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if !validTriggerTypes[req.TriggerType] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_trigger_type",
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": trigger})
}

// DeleteFormTrigger handles DELETE /forms/{id}/triggers/{triggerId}.
func (h *Handlers) DeleteFormTrigger(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	triggerID, err := parseUUIDParam(r, "triggerId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteFormTrigger(r.Context(), triggerID, formID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ReorderFormTriggers handles POST /forms/{id}/triggers/reorder — updates the
// position of each trigger by index in the trigger_ids array.
func (h *Handlers) ReorderFormTriggers(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		TriggerIDs []string `json:"trigger_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if len(req.TriggerIDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "trigger_ids must not be empty")
		return
	}

	triggerIDs := make([]pgtype.UUID, 0, len(req.TriggerIDs))

	for _, idStr := range req.TriggerIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid trigger ID format")
			return
		}

		triggerIDs = append(triggerIDs, id)
	}

	if err := h.Service.ReorderFormTriggers(r.Context(), formID, triggerIDs); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ListFormSteps handles GET /forms/{id}/steps — returns the steps configured on
// a multi-step form, ordered by position. Each step includes its field_group_id
// so the frontend can manage fields per step.
func (h *Handlers) ListFormSteps(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	steps, err := h.Service.ListFormSteps(r.Context(), formID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	type stepJSON struct {
		ID           string  `json:"id"`
		FormID       string  `json:"form_id"`
		Title        string  `json:"title"`
		Position     int32   `json:"position"`
		FieldGroupID *string `json:"field_group_id"`
	}

	result := make([]stepJSON, 0, len(steps))

	for _, st := range steps {
		fgid := h.Service.GetStepFormGroupID(r.Context(), st.ID)
		result = append(result, stepJSON{
			ID:           st.ID.String(),
			FormID:       st.FormID.String(),
			Title:        st.Title,
			Position:     st.Position,
			FieldGroupID: fgid,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// CreateFormStep handles POST /forms/{id}/steps — adds a new step to a
// multi-step form and provisions a backing field group for the step.
func (h *Handlers) CreateFormStep(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Fetch the form to get its slug for the field group naming.
	form, err := h.Service.GetForm(r.Context(), formID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var req struct {
		Title    string `json:"title"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "title must not be empty")
		return
	}

	step, err := h.Service.CreateFormStep(r.Context(), form.Slug, gen.CreateFormStepParams{
		FormID:   formID,
		Title:    req.Title,
		Position: req.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Return the step together with its field_group_id so the frontend can
	// immediately add fields to it via the existing field-group endpoints.
	fieldGroupID := h.Service.GetStepFormGroupID(r.Context(), step.ID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":             step.ID,
			"form_id":        step.FormID,
			"title":          step.Title,
			"position":       step.Position,
			"field_group_id": fieldGroupID,
		},
	})
}

// UpdateFormStep handles PATCH /forms/{id}/steps/{stepId} — updates a step's
// title and position.
func (h *Handlers) UpdateFormStep(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	stepID, err := parseUUIDParam(r, "stepId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Title    string `json:"title"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "title must not be empty")
		return
	}

	step, err := h.Service.UpdateFormStep(r.Context(), gen.UpdateFormStepParams{
		ID:       stepID,
		FormID:   formID,
		Title:    req.Title,
		Position: req.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": step})
}

// DeleteFormStep handles DELETE /forms/{id}/steps/{stepId}.
func (h *Handlers) DeleteFormStep(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	stepID, err := parseUUIDParam(r, "stepId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteFormStep(r.Context(), stepID, formID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ReorderFormSteps handles POST /forms/{id}/steps/reorder — updates the
// position of each step by index in the step_ids array.
func (h *Handlers) ReorderFormSteps(w http.ResponseWriter, r *http.Request) {
	formID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		StepIDs []string `json:"step_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if len(req.StepIDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "step_ids must not be empty")
		return
	}

	stepIDs := make([]pgtype.UUID, 0, len(req.StepIDs))

	for _, idStr := range req.StepIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid step ID format")
			return
		}

		stepIDs = append(stepIDs, id)
	}

	if err := h.Service.ReorderFormSteps(r.Context(), formID, stepIDs); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ListEntityTypes handles GET /custom-field-entity-types — returns available
// entity types for custom fields, read from the custom_field_entity_types
// table (tenant-scoped). Falls back to a hardcoded list if the table is
// empty (e.g. before the seed migration runs).
func (h *Handlers) ListEntityTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	types, err := h.Service.ListEntityTypes(r.Context())
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
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": fallback})

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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ---------------------------------------------------------------------------
// Custom Field Entity Types CRUD (Section 06)
// ---------------------------------------------------------------------------

// ListEntityTypesAdmin handles GET /admin/custom-field-entity-types — returns
// the full entity type rows (including is_system, sort_order) for the admin UI.
// Supports optional query params:
// - q: case-insensitive search on value and label
// - sort: "key:dir" e.g. "label:asc", "sort_order:desc"
// - page: 1-based page number (default 1)
// - page_size: page size (default 25, max 100)
func (h *Handlers) ListEntityTypesAdmin(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	searchQuery := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	sortParam := r.URL.Query().Get("sort")

	page := 1

	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	pageSize := 25

	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}

	types, err := h.Service.ListEntityTypes(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Post-filter by search query (case-insensitive on value and label).
	if searchQuery != "" {
		filtered := types[:0]

		for _, t := range types {
			if strings.Contains(strings.ToLower(t.Value), searchQuery) ||
				strings.Contains(strings.ToLower(t.Label), searchQuery) {
				filtered = append(filtered, t)
			}
		}

		types = filtered
	}

	// Sort: parse "key:dir" e.g. "value:asc", "sort_order:desc".
	sortKey := "sort_order"
	sortDir := "asc"

	if sortParam != "" {
		parts := strings.SplitN(sortParam, ":", 2)
		if parts[0] != "" {
			sortKey = parts[0]
		}

		if len(parts) > 1 && (parts[1] == "asc" || parts[1] == "desc") {
			sortDir = parts[1]
		}
	}

	sort.Slice(types, func(i, j int) bool {
		less := false

		switch sortKey {
		case "value":
			less = types[i].Value < types[j].Value
		case "label":
			less = types[i].Label < types[j].Label
		case "sort_order":
			less = types[i].SortOrder < types[j].SortOrder
		case "is_system":
			less = !types[i].IsSystem && types[j].IsSystem
		case "created_at":
			less = types[i].CreatedAt.Time.Before(types[j].CreatedAt.Time)
		default:
			less = types[i].SortOrder < types[j].SortOrder
		}

		if sortDir == "desc" {
			return !less
		}

		return less
	})

	// Pagination: slice the result.
	total := len(types)
	totalPages := 1

	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if totalPages < 1 {
			totalPages = 1
		}
	}

	start := (page - 1) * pageSize
	if start > total {
		start = total
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	paged := types[start:end]

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": paged,
		"meta": map[string]any{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// CreateEntityType handles POST /admin/custom-field-entity-types.
func (h *Handlers) CreateEntityType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		Value     string `json:"value"`
		Label     string `json:"label"`
		SortOrder int32  `json:"sort_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Value == "" || req.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "value and label are required")
		return
	}

	et, err := h.Service.CreateEntityType(r.Context(), gen.CreateCustomFieldEntityTypeParams{

		Value:     req.Value,
		Label:     req.Label,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": et})
}

// UpdateEntityType handles PATCH /admin/custom-field-entity-types/{id}.
func (h *Handlers) UpdateEntityType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		Label     string `json:"label"`
		SortOrder int32  `json:"sort_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "label is required")
		return
	}

	et, err := h.Service.UpdateEntityType(r.Context(), gen.UpdateCustomFieldEntityTypeParams{
		ID: id,

		Label:     req.Label,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": et})
}

// DeleteEntityType handles DELETE /admin/custom-field-entity-types/{id}.
// System entity types (is_system=true) cannot be deleted.
func (h *Handlers) DeleteEntityType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteEntityType(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ---------------------------------------------------------------------------
// Guide Category Admin Handlers (Section 14 — Guide Content Type System)
// ---------------------------------------------------------------------------

// ListGuideCategoriesAdmin handles GET /admin/guide-categories.
func (h *Handlers) ListGuideCategoriesAdmin(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	cats, err := h.Service.ListGuideCategories(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// CreateGuideCategoryAdmin handles POST /admin/guide-categories.
// Creates a guide category (entity_type + field group + assignment) in one
// transaction. After creation, the admin can add fields to the group via the
// existing custom field group editor (/custom-fields/{groupId}).
func (h *Handlers) CreateGuideCategoryAdmin(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var req struct {
		Slug  string `json:"slug"`
		Label string `json:"label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Slug == "" || req.Label == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "slug and label are required")
		return
	}

	cat, err := h.Service.CreateGuideCategory(r.Context(), CreateGuideCategoryInput{
		Slug:  req.Slug,
		Label: req.Label,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": cat})
}

// DeleteGuideCategoryAdmin handles DELETE /admin/guide-categories/{slug}.
// System categories (seeded) cannot be deleted.
func (h *Handlers) DeleteGuideCategoryAdmin(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_slug", "Slug is required")
		return
	}

	if err := h.Service.DeleteGuideCategory(r.Context(), slug); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
