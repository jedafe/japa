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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"net/netip"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	errpkg "github.com/jedafe/japa/internal/errors"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/payments"
	"github.com/jedafe/japa/internal/rbac"
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
	SpendCredits(ctx context.Context, userID pgtype.UUID, input wallet.SpendCreditsInput) (int64, error)
}

// PaymentCreator creates payment intents for paid submissions.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, userID pgtype.UUID, input payments.CreatePaymentInput) (payments.CreatePaymentResult, error)
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

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response. It centralizes the not-found, duplicate, and fallback mapping so
// every handler stays consistent.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// isFormPublic reports whether a form status exposes the public surface —
// the embed schema and the submission endpoint. "active" is the legacy
// alias of "published" (pre-CF13 seeds); both accept submissions, while
// "draft" and any unknown status do not.
func isFormPublic(status string) bool {
	return status == "published" || status == "active"
}

// validFormStatuses is the accepted set for form create/update.
var validFormStatuses = map[string]bool{"draft": true, "active": true, "published": true}

// validSubmissionStatuses is the accepted set for submission status updates.
var validSubmissionStatuses = map[string]bool{
	"new": true, "reviewed": true, "actioned": true, "spam": true,
	"pending_payment": true, "expired": true,
}

// validReturnURL accepts only same-origin relative paths — a hosted checkout
// must never redirect the payer to an arbitrary external site.
func validReturnURL(u string) bool {
	return strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//")
}

// ---------------------------------------------------------------------------
// Custom Field Groups
// ---------------------------------------------------------------------------

// fieldGroupEnriched is the enriched field group response that includes the
// derived entity_type (from assignments) and the group's fields. Both the
// main app content composer and the admin panel expect these fields.
type fieldGroupEnriched struct {
	ID pgtype.UUID `json:"id"`

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
	Section        *string            `json:"section,omitempty"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
}

// toFieldSummary converts a gen.CustomField to a fieldSummary, preserving
// the Options JSONB as raw JSON instead of base64-encoded []byte.
func toFieldSummary(f gen.ListAllCustomFieldsRow) fieldSummary {
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
		Section:        f.Section,
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
		Section:        f.Section,
		CreatedAt:      f.CreatedAt,
	}
}

// enrichFieldGroups assembles enriched field group objects from the bare
// group structs, their assignments (for entity_type), and their fields.
// Uses bulk queries to avoid N+1 per-group lookups.
func (h *Handlers) enrichFieldGroups(
	ctx context.Context,

	groups []gen.CustomFieldGroup,
) ([]fieldGroupEnriched, error) {
	// Bulk-fetch all assignments and fields for the platform's groups.
	assignments, err := h.Service.ListAllAssignmentsByPlatform(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing group assignments: %w", err)
	}

	allFields, err := h.Service.ListAllFieldsByPlatform(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing custom fields: %w", err)
	}

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

	return enriched, nil
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

	enriched, err := h.enrichFieldGroups(r.Context(), groups)
	if err != nil {
		handleServiceError(w, err)
		return
	}

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
// Accepts an optional entity_type: when present the group's assignment is
// upserted to the derived entity_class (ON CONFLICT DO NOTHING —
// reassignment is idempotent), so the admin can re-home a group without a
// separate assign call (CF7). The response carries an assignment_hint when
// no entity_type was sent and the group has no assignment yet.
func (h *Handlers) UpdateFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var req struct {
		Name        string  `json:"name"`
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

	group, err := h.Service.UpdateFieldGroup(r.Context(), id, req.Name, req.Description)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// CF7: accept entity_type reassignment via upsert assignment.
	// CreateAssignment is ON CONFLICT DO NOTHING, so repeating the same
	// entity_type is a safe no-op and switching types adds the new class
	// binding (old bindings are preserved — removal stays explicit via
	// DELETE /assignments).
	if entityType := strings.TrimSpace(req.EntityType); entityType != "" {
		entityClass := EntityClassFromType(entityType)
		if _, err := h.Service.CreateAssignment(r.Context(), gen.CreateAssignmentParams{
			GroupID:     group.ID,
			EntityClass: entityClass,
			EntityID:    pgtype.UUID{},
			Position:    0,
		}); err != nil {
			// Duplicate rows surface as pgx.ErrNoRows (no RETURNING row) —
			// treat as success; anything else fails the request.
			if !errors.Is(err, pgx.ErrNoRows) {
				handleServiceError(w, err)
				return
			}
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": group})

		return
	}

	resp := map[string]any{"data": group}

	if assignments, err := h.Service.ListAllAssignmentsByPlatform(r.Context()); err == nil {
		assigned := false

		for _, a := range assignments {
			if a.GroupID == group.ID {
				assigned = true
				break
			}
		}

		if !assigned {
			resp["assignment_hint"] = "Group has no entity assignment — send entity_type to attach it (e.g. via /entity-types)."
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// DeleteFieldGroup handles DELETE /api/v1/custom-field-groups/{id}.
// ?force=true bypasses the in-use guard and cascade-deletes the group's
// fields, stored values, and assignments (Section 06).
func (h *Handlers) DeleteFieldGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	force := r.URL.Query().Get("force") == "true"

	// A group bound to a system form is part of a shipped flow — deleting
	// it (even with force) orphans the form's field schema.
	if bound, err := h.Service.GroupBoundToSystemForm(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	} else if bound {
		errpkg.WriteError(w, http.StatusForbidden, "system_form_protected", "This field group backs a system form and cannot be deleted")
		return
	}

	if err := h.Service.DeleteFieldGroup(r.Context(), id, force); err != nil {
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
	Label     string `json:"label"`
	Key       string `json:"key"`
	FieldType string `json:"field_type"`
	// Options is the raw options payload — an array of {value,label} for
	// select/multiselect, an object-shaped schema for price_parts
	// ({parts, primary_part, currency_code}, §80 Wiki Topic Pages), free
	// form objects for the rest.
	Options        json.RawMessage `json:"options"`
	EmbedPlatforms []string        `json:"embed_platforms"`
	Placeholder    *string         `json:"placeholder"`
	DefaultValue   *string         `json:"default_value"`
	IsRequired     bool            `json:"is_required"`
	ShowInFrontend bool            `json:"show_in_frontend"`
	Filterable     bool            `json:"filterable"`
	Sortable       bool            `json:"sortable"`
	Searchable     bool            `json:"searchable"`
	Position       int32           `json:"position"`
	// Section groups fields sharing a token under one item table / sub-tab
	// (§80 Itemized Pricing). Pointer so PATCH distinguishes an omitted key
	// (leave untouched) from an explicit "" (clear the section).
	Section *string `json:"section"`
}

// marshalOptions validates and compacts the raw options payload for the
// custom_fields.options jsonb column. Empty or "null" payloads store NULL.
func marshalOptions(opts json.RawMessage) ([]byte, error) {
	trimmed := bytes.TrimSpace(opts)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	if !json.Valid(trimmed) {
		return nil, errors.New("options are not valid JSON")
	}

	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// optionsAbsent reports whether a request's raw options carry no usable
// payload — empty, "null", or an empty []/{}.
func optionsAbsent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return true
	}

	if string(trimmed) == "[]" || string(trimmed) == "{}" {
		return true
	}

	return false
}

// ListFieldsByGroup handles GET /api/v1/custom-field-groups/{id}/fields.
func (h *Handlers) ListFieldsByGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	if (req.FieldType == FieldTypeSelect || req.FieldType == FieldTypeMultiselect) && optionsAbsent(req.Options) {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_options", "Select and multiselect fields require options")
		return
	}

	optionsBytes, err := marshalOptions(req.Options)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_options", "Field options are not valid JSON")
		return
	}

	if req.Section != nil && !ValidSectionToken(*req.Section) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_section", "Section must match ^[a-z0-9_]+$")
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
		Section:        req.Section,
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

	if (req.FieldType == FieldTypeSelect || req.FieldType == FieldTypeMultiselect) && optionsAbsent(req.Options) {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_options", "Select and multiselect fields require options")
		return
	}

	optionsBytes, err := marshalOptions(req.Options)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_options", "Field options are not valid JSON")
		return
	}

	// §80 Itemized Pricing: section travels three-state — the pointer's
	// presence flips update_section so "" clears and a token sets.
	section := ""
	if req.Section != nil {
		section = *req.Section
	}

	if !ValidSectionToken(section) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_section", "Section must match ^[a-z0-9_]+$")
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
		UpdateSection:  req.Section != nil,
		Section:        section,
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

	// Fields on a system-form-bound group are load-bearing for a bespoke
	// page — deleting one breaks that page's bindings even when the field
	// row itself isn't flagged is_system.
	if field, ferr := h.Service.GetField(r.Context(), id); ferr == nil {
		if bound, berr := h.Service.GroupBoundToSystemForm(r.Context(), field.GroupID); berr != nil {
			handleServiceError(w, berr)
			return
		} else if bound {
			errpkg.WriteError(w, http.StatusForbidden, "system_form_protected", "This field belongs to a system form and cannot be deleted")
			return
		}
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
// parsing; the database scopes each update to the given group and platform).
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

	// §06 canonical shape: ordered_field_ids. Position is the array index.
	if len(req.OrderedFieldIDs) > 0 {
		// CF16: reject system/foreign IDs explicitly — the position update is
		// group-scoped, so an ID from another group (or a system field)
		// would no-op per row; fail the reorder instead of half-applying.
		known := map[string]bool{}

		if fields, err := h.Service.ListFieldsByGroup(r.Context(), groupID); err == nil {
			for _, f := range fields {
				known[f.ID.String()] = true
			}
		} else {
			slog.Warn("customfields: reorder pre-check failed", "group_id", groupID.String(), "error", err)
		}

		ids := make([]pgtype.UUID, 0, len(req.OrderedFieldIDs))

		for _, idStr := range req.OrderedFieldIDs {
			var fieldID pgtype.UUID
			if err := fieldID.Scan(idStr); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_id", "Invalid field id: "+idStr)
				return
			}

			if len(known) > 0 && !known[fieldID.String()] {
				errpkg.WriteError(w, http.StatusBadRequest, "unknown_field_id", "Field "+idStr+" is not in this group (system or foreign fields cannot be reordered)")
				return
			}

			ids = append(ids, fieldID)
		}

		// CF-B8: single transaction — a partial reorder is never committed.
		if err := h.Service.ReorderFields(r.Context(), groupID, ids); err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"reordered": true}})

		return
	}

	// Legacy shape: fields with explicit positions.
	legacyKnown := map[string]bool{}

	if len(req.Fields) > 0 {
		if fields, err := h.Service.ListFieldsByGroup(r.Context(), groupID); err == nil {
			for _, f := range fields {
				legacyKnown[f.ID.String()] = true
			}
		}
	}

	for _, f := range req.Fields {
		var fieldID pgtype.UUID
		if err := fieldID.Scan(f.ID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_id", "Invalid field id: "+f.ID)
			return
		}

		if len(legacyKnown) > 0 && !legacyKnown[fieldID.String()] {
			errpkg.WriteError(w, http.StatusBadRequest, "unknown_field_id", "Field "+f.ID+" is not in this group (system or foreign fields cannot be reordered)")
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
	// Fetched before payload normalization: sanitization and canonical
	// matching operate on the raw payload, then marshal produces storable bytes.
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
	// Canonical matching is value-only: a submitted label is normalized to
	// its option value at write, so readers only ever see values.
	if req.ValueJson != nil && (field.FieldType == FieldTypeSelect || field.FieldType == FieldTypeMultiselect) {
		jsonBytes = CanonicalizeSelectJSON(field.FieldType, field.Options, jsonBytes)

		var vj map[string]any

		if err := json.Unmarshal(jsonBytes, &vj); err == nil {
			fd := fieldDef{Label: field.Label, Key: field.Key, FieldType: field.FieldType, Options: field.Options, IsRequired: field.IsRequired}
			if msg, ok := validateSelectValue(fd, vj); !ok {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_value", msg)
				return
			}
		}
	}

	// CF1: rich_text stores in value_json, so scrub the JSON string leaves
	// before marshal — the text-column sanitizer below cannot reach them.
	// JSON shape validation happens in the value_json column setter path;
	// this only rewrites string leaves when a sanitizer is wired.
	if field.FieldType == FieldTypeRichText && len(jsonBytes) > 0 && h.richTextSanitizer != nil {
		jsonBytes = SanitizeRichTextJSON(jsonBytes, h.richTextSanitizer)
	}

	// BUG-001: sanitize rich_text HTML before storage to prevent stored XSS.
	// rich_text fields may write to value_text on legacy paths; scrub the
	// HTML in-place when a sanitizer is wired.
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
			"is_system":              form.IsSystem,
			"submission_fee_credits": form.SubmissionFeeCredits,
			"fields":                 fieldList,
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
	} else if !validFormStatuses[status] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "status must be one of: draft, active, published")
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
	// CF15: surface provisioning errors — a form without its group is
	// unusable in the admin field editor, so fail loudly instead of
	// returning a half-provisioned form.
	if err := h.Service.EnsureFormGroup(r.Context(), form.ID, req.Name, req.Slug); err != nil {
		slog.Error("customfields: provisioning form field group", "form_id", form.ID, "error", err)
		handleServiceError(w, err)

		return
	}

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

	// Verify the space exists before creating the form — otherwise the FK
	// violation surfaces as a raw 500 instead of a clean 404.
	if !h.Service.SpaceExists(r.Context(), spaceID) {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
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

		// Options are honoured for any resolved select-typed field —
		// "radio" maps to select and "select" is sent natively; previously
		// only radio persisted options so admin select fields lost theirs.
		if ft == FieldTypeSelect && len(f.Options) > 0 {
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

	if req.Status != "" && !validFormStatuses[req.Status] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "status must be one of: draft, active, published")
		return
	}

	// An omitted status preserves the current one — writing "" would break
	// the lifecycle gate (embed + submission only accept a known status).
	status := req.Status
	if status == "" {
		status = existing.Status
	}

	form, err := h.Service.UpdateForm(r.Context(), gen.UpdateFormParams{
		Name:                 req.Name,
		Description:          req.Description,
		IsMultiStep:          req.IsMultiStep,
		SubmitLabel:          req.SubmitLabel,
		SuccessMessage:       req.SuccessMessage,
		Status:               status,
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

	// System forms (is_system=true) are consumed by bespoke pages keyed on
	// their slug — deleting one orphans the flow and cascades its
	// submissions. Mirrors the entity-types rule: reject, don't delete.
	existing, err := h.Service.GetForm(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if existing.IsSystem {
		errpkg.WriteError(w, http.StatusForbidden, "system_form_protected", "System forms cannot be deleted")
		return
	}

	if err := h.Service.DeleteForm(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// EnsureFormGroup handles POST /api/v1/forms/{id}/ensure-field-group.
// It get-or-creates a backing field group and assignment for a form
// (CF15): the lookup runs first so repeat calls reuse the existing group,
// and a concurrent create racing this call surfaces as a conflict the
// caller can retry — never a silent duplicate.
func (h *Handlers) EnsureFormGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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
	if fieldGroupID == nil {
		// CF15: the form should always carry a backing group — surface the
		// provisioning gap explicitly instead of returning a null id the
		// admin UI cannot act on.
		errpkg.WriteError(w, http.StatusConflict, "field_group_missing", "This form has no backing field group; retry the request")
		return
	}

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
//
// filterGuestViewableFields narrows embed field rows to the public schema
// (CF13): only fields the guest group can_view AND flagged
// show_in_frontend survive. A lookup error fails closed to an empty list —
// the embed renders no fields rather than leaking admin-only ones.
func filterGuestViewableFields(ctx context.Context, svc *Service, fields []gen.ListCustomFieldsByGroupRow) []gen.ListCustomFieldsByGroupRow {
	if len(fields) == 0 || svc == nil {
		return fields
	}

	ids, err := svc.ListVisibleFieldIDsForGroupSlug(ctx, "guest")
	if err != nil {
		slog.Warn("customfields: guest-visible field lookup failed, hiding embed fields", "error", err)
		return nil
	}

	visible := make(map[string]bool, len(ids))
	for _, id := range ids {
		visible[id.String()] = true
	}

	out := fields[:0]

	for _, f := range fields {
		if !f.ShowInFrontend || !visible[f.ID.String()] {
			continue
		}

		out = append(out, f)
	}

	return out
}

func (h *Handlers) GetFormEmbed(w http.ResponseWriter, r *http.Request) {
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
	// CF13: the embed is gated to published forms carrying guest-viewable
	// fields only. A draft form has no public schema — its embed is a 404
	// (not a leak of in-progress fields). Field rows are additionally
	// narrowed to guest can_view grants + show_in_frontend, so admin-only
	// fields never reach the unauthenticated surface.
	fields, err := h.Service.GetFormFields(r.Context(), form.ID)
	if err != nil {
		slog.Warn("get form embed fields", "slug", slug, "error", err)

		fields = nil
	}

	if !isFormPublic(form.Status) {
		errpkg.WriteError(w, http.StatusNotFound, "form_not_published", "This form is not published")
		return
	}

	fields = filterGuestViewableFields(r.Context(), h.Service, fields)

	type fieldJSON struct {
		ID           string   `json:"id"`
		Key          string   `json:"key"`
		Label        string   `json:"label"`
		Type         string   `json:"type"`
		Required     bool     `json:"required"`
		SortOrder    int32    `json:"sort_order"`
		Options      []string `json:"options"`
		Placeholder  *string  `json:"placeholder"`
		DefaultValue *string  `json:"default_value"`
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
			ID:           f.ID.String(),
			Key:          f.Key,
			Label:        f.Label,
			Type:         f.FieldType,
			Required:     f.IsRequired,
			SortOrder:    f.Position,
			Options:      options,
			Placeholder:  f.Placeholder,
			DefaultValue: f.DefaultValue,
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

					// CF-A2: step fields go through the same guest-visible
					// gate as the flat fields list — otherwise admin-only
					// or frontend-hidden step fields leak on the public embed.
					rawFields = filterGuestViewableFields(r.Context(), h.Service, rawFields)

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
		// Same-origin URL the hosted checkout redirects back to after
		// payment (§17 ReturnURL) — lets flows like /verification-request
		// resume the request that the payment unlocked.
		ReturnURL string `json:"return_url"`
		// Step is the §06 multi-step resume hint — accepted for contract
		// compatibility; the server validates against all steps' fields.
		Step *int `json:"step"`
		// §06/§07: business-record intent carried into the payment metadata
		// so the payment webhook can auto-create the linked record
		// (verification request / ownership claim) even when the submitter
		// never returns from hosted checkout. Only honored on the two
		// system slugs that consume submissions this way.
		Link *struct {
			Kind           string `json:"kind"`
			EntityType     string `json:"entity_type"`
			OrganizationID string `json:"organization_id"`
			RequestedType  string `json:"requested_type"`
		} `json:"link"`
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

	// Platform is required even for public submissions so the row is scoped.

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

	// CF-S4: the checkout return target must be a same-origin relative path —
	// an arbitrary external URL would turn our hosted-checkout flow into an
	// open redirect for phishing.
	if req.ReturnURL != "" && !validReturnURL(req.ReturnURL) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_return_url", "return_url must be a same-origin relative path")
		return
	}

	// Look up the form to check submission_fee_credits.
	form, err := h.Service.GetForm(r.Context(), formID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// CF-A5: the submission endpoint shares the embed's lifecycle gate —
	// a draft or otherwise non-public form rejects submissions, otherwise
	// anyone holding the form UUID could file entries against it.
	if !isFormPublic(form.Status) {
		errpkg.WriteError(w, http.StatusNotFound, "form_not_published", "This form is not published")
		return
	}

	// BUG-002: validate field_values against the form's field definitions
	// (required fields, type validation, unknown keys). Also sanitizes
	// rich_text HTML (BUG-001) in-place. When the form has no field
	// definitions, validation is skipped (lenient posture for legacy forms).
	if req.FieldValues != nil {
		if msg, ok := h.validateFormSubmission(r.Context(), form, req.FieldValues); !ok {
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
	// §07 Verification Requests: same override for the verification form.
	if h.settings != nil && form.Slug == "verification-request" {
		override := h.settings.GetIntOr(r.Context(), "verification.fee_credits", 0)
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

		h.enqueueFormTriggers(formID, submission.ID, userID, fieldValues)

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

		// CF-A6: create the pending row first so a failed debit can't leave
		// the user charged with no submission — the hourly sweep expires
		// abandoned pending_payment rows. The wallet idempotency reference
		// carries the submission ID, which also fixes CF-A4 (per-attempt
		// uniqueness instead of per-form) and links the ledger entry to the
		// exact submission for reconciliation.
		submission, sErr := h.Service.CreateFormSubmission(r.Context(), gen.CreateFormSubmissionParams{
			FormID: formID,

			UserID:      userID,
			IpAddress:   ipAddr,
			FieldValues: fieldValues,
			Status:      "pending_payment",
		})
		if sErr != nil {
			handleServiceError(w, sErr)
			return
		}

		_, spendErr := h.wallet.SpendCredits(r.Context(), userID, wallet.SpendCreditsInput{
			Amount:      feeCredits,
			Source:      "form_submission",
			Reference:   "form_submission:" + submission.ID.String(),
			PurposeType: "form_submission",
			PurposeID:   submission.ID.String(),
		})
		if spendErr != nil {
			errpkg.WriteError(w, http.StatusPaymentRequired, "insufficient_credits", "Insufficient credits to pay the submission fee")
			return
		}

		if err := h.Service.UpdateFormSubmissionStatus(r.Context(), submission.ID, "new"); err != nil {
			handleServiceError(w, err)
			return
		}

		h.enqueueFormTriggers(formID, submission.ID, userID, fieldValues)

		errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
			"data": map[string]any{"id": submission.ID, "status": "new"},
		})

	case payments.ProviderPaystack, payments.ProviderStripe:
		// Create a payment intent via the gateway.
		if h.payments == nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "payments_unavailable", "Payments service is not configured")
			return
		}

		// CF-S3: only the form link travels to the payment provider —
		// field_values contain submitter PII and are already persisted on
		// the submission row, so they must not leave the platform.
		metadata := map[string]interface{}{
			"form_id": formID.String(),
		}
		// Only the verification-request and claim-ownership system forms
		// consume a link intent — other forms ignore it. The linked record
		// is created by the payment-webhook task (§06 Paid Form Submissions)
		// and still passes the full server-side eligibility checks. The slugs
		// are admin-configurable — resolve them, don't hardcode (Rule 3).
		if req.Link != nil {
			verificationSlug := "verification-request"
			claimSlug := "claim-ownership"
			if h.settings != nil {
				if v, sErr := h.settings.GetSetting(r.Context(), "verification.form_slug"); sErr == nil && v != "" {
					verificationSlug = v
				}
				if v, sErr := h.settings.GetSetting(r.Context(), "claim_ownership.form_slug"); sErr == nil && v != "" {
					claimSlug = v
				}
			}
			if form.Slug == verificationSlug || form.Slug == claimSlug {
				metadata["link"] = req.Link
			}
		}

		result, pErr := h.payments.CreatePayment(r.Context(), userID, payments.CreatePaymentInput{
			Provider:    req.PaymentMethod,
			PurposeType: "form_submission",
			PurposeID:   formID.String(),
			Amount:      feeCredits,
			ReturnURL:   req.ReturnURL,
			Metadata:    metadata,
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
func (h *Handlers) enqueueFormTriggers(formID, submissionID, userID pgtype.UUID, fieldValues []byte) {
	if h.AsynqClient == nil {
		return
	}

	payload := map[string]string{
		"submission_id": submissionID.String(),

		"form_id": formID.String(),
	}
	if userID.Valid {
		payload["user_id"] = userID.String()
	}

	// field_values is included as a raw JSON fragment so the worker can pass
	// it through to the trigger dispatcher without re-parsing.
	payloadBytes, err := json.Marshal(struct {
		SubmissionID string `json:"submission_id"`

		FormID      string `json:"form_id"`
		UserID      string `json:"user_id,omitempty"`
		FieldValues []byte `json:"field_values"`
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
func (h *Handlers) validateFormSubmission(ctx context.Context, form gen.GetFormByIDRow, fieldValues map[string]any) (string, bool) {
	fields, err := h.Service.GetFormFields(ctx, form.ID)
	if err != nil {
		// If we can't load fields, don't block the submission — log and
		// proceed. This matches the lenient posture for forms with no
		// field definitions.
		slog.Warn("customfields: loading form fields for validation", "form_id", form.ID.String(), "error", err)
		return "", true
	}

	// CF-A1: multi-step forms keep their fields on per-step groups —
	// merge them so submitted step values aren't rejected as unknown.
	if form.IsMultiStep {
		if steps, sErr := h.Service.ListFormSteps(ctx, form.ID); sErr == nil {
			for _, st := range steps {
				if sfs, sfErr := h.Service.GetStepFields(ctx, st.ID); sfErr == nil {
					fields = append(fields, sfs...)
				}
			}
		}
	}

	if len(fields) == 0 {
		return "", true
	}

	// CF-S6: the write path honors the same field visibility the embed
	// renders — anonymous submitters may only write guest-visible fields,
	// authenticated non-super-admins are limited to fields their group can
	// view. Super-admins bypass; nil RBAC (tests) skips the gate.
	if h.rbac != nil {
		var visible []pgtype.UUID

		var vErr error

		user := auth.FromContext(ctx)

		switch {
		case user != nil && user.UserID.Valid && h.rbac.IsSuperAdmin(ctx, user.UserID):
			visible = nil // super-admin — all fields writable
		case user != nil && user.UserID.Valid:
			visible, vErr = h.Service.ListVisibleFieldIDsForUser(ctx, user.UserID)
		default:
			visible, vErr = h.Service.ListVisibleFieldIDsForGroupSlug(ctx, "guest")
		}

		if vErr != nil {
			slog.Warn("customfields: submission visibility lookup failed, rejecting", "error", vErr)
			return "Could not validate field permissions", false
		}

		if visible != nil {
			allowed := make(map[pgtype.UUID]bool, len(visible))
			for _, vid := range visible {
				allowed[vid] = true
			}

			kept := fields[:0]

			for _, f := range fields {
				if allowed[f.ID] {
					kept = append(kept, f)
				}
			}

			fields = kept
		}
	}

	// Build lookup maps: label→field and key→field.
	byLabel := make(map[string]fieldDef, len(fields))
	byKey := make(map[string]fieldDef, len(fields))

	for _, f := range fields {
		fd := fieldDef{Label: f.Label, Key: f.Key, FieldType: f.FieldType, Options: f.Options, IsRequired: f.IsRequired}
		byLabel[f.Label] = fd
		byKey[f.Key] = fd
	}

	// 1. Check required fields are present and non-empty. Structural
	// 'group' fields store no value — requiring one would be unsatisfiable.
	for _, f := range fields {
		if !f.IsRequired || f.FieldType == FieldTypeGroup {
			continue
		}

		val, exists := lookupFieldValue(fieldValues, f.Label, f.Key)
		if !exists || isEmptyValue(val) {
			return fmt.Sprintf("Field %q is required", f.Label), false
		}
	}

	// 2. Check each provided value corresponds to a known field, and validate
	// the value against the field type. Canonical select matching is
	// value-only here too: a submitted label is normalized to its option
	// value in-place so storage converges on values (CF3).
	for rawKey, rawVal := range fieldValues {
		f, ok := byLabel[rawKey]
		if !ok {
			f, ok = byKey[rawKey]
		}

		if !ok {
			return fmt.Sprintf("Unknown field %q — this field is not defined on the form", rawKey), false
		}

		if f.FieldType == FieldTypeSelect {
			if s, isStr := rawVal.(string); isStr && s != "" {
				rawVal = NormalizeChoiceValue(f.Options, s)
				fieldValues[rawKey] = rawVal
			}
		}

		if f.FieldType == FieldTypeMultiselect {
			switch v := rawVal.(type) {
			case string:
				if v != "" {
					rawVal = NormalizeChoiceValue(f.Options, v)
					fieldValues[rawKey] = rawVal
				}
			case []any:
				norm := make([]any, len(v))

				for i, item := range v {
					if s, isStr := item.(string); isStr {
						norm[i] = NormalizeChoiceValue(f.Options, s)
					} else {
						norm[i] = item
					}
				}

				fieldValues[rawKey] = norm
				rawVal = norm
			case []string:
				norm := make([]string, len(v))
				for i, item := range v {
					norm[i] = NormalizeChoiceValue(f.Options, item)
				}

				fieldValues[rawKey] = norm
				rawVal = norm
			}
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

// isEmptyValue mirrors the backend required gate: nil, blank strings
// (whitespace-only counts as empty), and empty slices/arrays are empty.
// Booleans (including false) and numbers (including 0) are real values and
// never empty — a required boolean checkbox storing false still counts as
// filled (CF22 composer gate semantics).
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

		if !valueInOptions(f.Options, NormalizeChoiceValue(f.Options, strVal)) {
			return fmt.Sprintf("Field %q value is not one of the allowed options", f.Label), false
		}

	case FieldTypeMultiselect:
		if strVal == "" {
			return "", true
		}
		// Multiselect can be a string (single) or an array.
		switch v := raw.(type) {
		case string:
			if !valueInOptions(f.Options, NormalizeChoiceValue(f.Options, v)) {
				return fmt.Sprintf("Field %q value is not one of the allowed options", f.Label), false
			}
		case []any:
			for _, item := range v {
				s, _ := item.(string)
				if !valueInOptions(f.Options, NormalizeChoiceValue(f.Options, s)) {
					return fmt.Sprintf("Field %q value %q is not one of the allowed options", f.Label, s), false
				}
			}
		case []string:
			for _, item := range v {
				if !valueInOptions(f.Options, NormalizeChoiceValue(f.Options, item)) {
					return fmt.Sprintf("Field %q value %q is not one of the allowed options", f.Label, item), false
				}
			}
		}
	}

	return "", true
}

// valueInOptions checks whether a value matches one of the field's select
// options. Canonical matching is value-only: labels are normalized to values
// at write (NormalizeChoiceValue), so this only compares option values.
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
		if o.Value == target {
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
// field's allowed options (BUG-005). Canonical matching is value-only:
// callers normalize labels to values before invoking this, and the choice
// payload is unwrapped via ChoiceStrings so bare strings/arrays and the
// {"value": …} / {"values": […]} envelopes validate identically (CF4).
// Returns an error message and false when validation fails.
func validateSelectValue(field fieldDef, valueJson map[string]any) (string, bool) {
	raw, err := json.Marshal(valueJson)
	if err != nil {
		return fmt.Sprintf("Field %q value is not valid JSON", field.Label), false
	}

	for _, v := range ChoiceStrings(raw) {
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

	if !validSubmissionStatuses[req.Status] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "status must be one of: new, reviewed, actioned, spam, pending_payment, expired")
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
// The per-type config schema is validated here (CF14): each trigger type
// declares required config keys, and send_webhook URLs must be http/https.
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

	if msg, ok := validateTriggerConfig(req.TriggerType, req.Config); !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_trigger_config", msg)
		return
	}

	// CF-S1: assign_user_group may not target a system group — a form
	// trigger is fired by any submitter, so letting it grant super_admin,
	// members, or guest would be self-serve privilege escalation.
	if req.TriggerType == "assign_user_group" {
		if msg, ok := h.checkTriggerGroupAssignable(r.Context(), req.Config); !ok {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_trigger_config", msg)
			return
		}
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

	// PATCH semantics: an omitted trigger_type keeps the stored one so a
	// config-only edit doesn't have to resend it.
	triggerType := req.TriggerType

	if triggerType == "" {
		if trigs, terr := h.Service.ListFormTriggers(r.Context(), formID); terr == nil {
			for _, t := range trigs {
				if t.ID == triggerID {
					triggerType = t.TriggerType
					break
				}
			}
		}
	}

	if !validTriggerTypes[triggerType] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_trigger_type",
			"trigger_type must be one of: assign_user_group, send_email, send_webhook, create_ticket, notify_admin")
		return
	}

	if req.Config == nil {
		req.Config = json.RawMessage("{}")
	}

	if msg, ok := validateTriggerConfig(triggerType, req.Config); !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_trigger_config", msg)
		return
	}

	// CF-S1: assign_user_group may not target a system group — a form
	// trigger is fired by any submitter, so letting it grant super_admin,
	// members, or guest would be self-serve privilege escalation.
	if triggerType == "assign_user_group" {
		if msg, ok := h.checkTriggerGroupAssignable(r.Context(), req.Config); !ok {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_trigger_config", msg)
			return
		}
	}

	trigger, err := h.Service.UpdateFormTrigger(r.Context(), gen.UpdateFormSubmissionTriggerParams{
		ID:          triggerID,
		FormID:      formID,
		TriggerType: triggerType,
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

// checkTriggerGroupAssignable enforces the CF-S1 escalation guard on
// assign_user_group triggers: the target group must exist and must not be
// one of the system groups (super_admin / members / guest) — a trigger is
// fired by any form submitter, so a privileged target would be self-serve
// privilege escalation.
func (h *Handlers) checkTriggerGroupAssignable(ctx context.Context, raw json.RawMessage) (string, bool) {
	var cfg struct {
		UserGroupID string `json:"user_group_id"`
	}

	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.UserGroupID == "" {
		return "assign_user_group requires config.user_group_id", false
	}

	var groupID pgtype.UUID
	if err := groupID.Scan(cfg.UserGroupID); err != nil {
		return "assign_user_group config.user_group_id must be a UUID", false
	}

	group, err := h.Service.GetUserGroup(ctx, groupID)
	if err != nil {
		return "assign_user_group target user group not found", false
	}

	if rbac.IsSystemGroup(group.Slug) {
		return fmt.Sprintf("assign_user_group cannot target the %q system group", group.Slug), false
	}

	return "", true
}

// validateTriggerConfig enforces the per-type config schema (CF14) for form
// submission triggers: required keys per trigger_type, and a scheme
// allowlist for send_webhook URLs (http/https only — no file/data/javascript
// sinks). Returns a user-facing message and false when invalid.
func validateTriggerConfig(triggerType string, raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte("{}")
	}

	var cfg map[string]any
	if err := json.Unmarshal(trimmed, &cfg); err != nil {
		return "trigger config must be a JSON object", false
	}

	nonEmpty := func(key string) bool {
		v, _ := cfg[key].(string)
		return strings.TrimSpace(v) != ""
	}

	switch triggerType {
	case "assign_user_group":
		if !nonEmpty("user_group_id") {
			return "assign_user_group requires config.user_group_id", false
		}
	case "send_email":
		if !nonEmpty("to") {
			return "send_email requires config.to", false
		}

		// CF-S5: validate the recipient as a single RFC 5322 address —
		// raw strings with control characters could inject headers in
		// downstream mail transports.
		to, _ := cfg["to"].(string)
		if addr, err := mail.ParseAddress(to); err != nil || addr.Address != to {
			return "send_email config.to must be a single email address", false
		}
	case "send_webhook":
		u, _ := cfg["url"].(string)
		if strings.TrimSpace(u) == "" {
			return "send_webhook requires config.url", false
		}

		parsed, err := neturl.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return "send_webhook config.url must be an http(s) URL", false
		}
	case "create_ticket":
		// subject/description both default server-side; no required keys.
	case "notify_admin":
		// title/body/action_url all default server-side; no required keys.
	default:
		return "unknown trigger_type", false
	}

	return "", true
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

// ListFieldTypes handles GET /admin/field-types — returns the full §06
// field-type catalog (value + label + needs_options) so admin pickers
// consume the backend contract instead of a hardcoded list (Rule 16).
func (h *Handlers) ListFieldTypes(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": FieldTypeCatalog()})
}

// ListEntityTypes handles GET /custom-field-entity-types — returns available
// entity types for custom fields, read from the custom_field_entity_types
// table (platform-scoped). Falls back to a hardcoded list if the table is
// empty (e.g. before the seed migration runs).
func (h *Handlers) ListEntityTypes(w http.ResponseWriter, r *http.Request) {
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
// - page_size: page size (default 25, max 100).
func (h *Handlers) ListEntityTypesAdmin(w http.ResponseWriter, r *http.Request) {
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
