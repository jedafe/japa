// Package pagebuilder — HTTP handlers for the Page Builder admin CRUD system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package pagebuilder

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/tenant"
)

// WidgetResolver resolves a widget by ID and returns its rendered data,
// widget type, and configured card type.
// Implemented by widgets.Service — defined here as an interface so the
// pagebuilder package doesn't depend on the widgets package at compile time.
type WidgetResolver interface {
	RenderWidgetByID(ctx context.Context, id pgtype.UUID) (json.RawMessage, string, string, error)
}

// Handlers holds dependencies for pagebuilder HTTP handlers.
type Handlers struct {
	Service *Service
	Widgets WidgetResolver
	Catalog *literals.Catalog
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// SetWidgetResolver injects the widget resolver (late binding to avoid
// import cycles at construction time).
func (h *Handlers) SetWidgetResolver(r WidgetResolver) {
	h.Widgets = r
}

// SetLiteralCatalog wires the shared literal catalog for resolving template
// literals in published pages.
func (h *Handlers) SetLiteralCatalog(c *literals.Catalog) {
	h.Catalog = c
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
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

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// parsePageParams extracts page/page_size from the query string.
// page is 1-based; page_size defaults to 20, max 100. Matches the
// geo-aware tool pattern used by calendar/media/admin list endpoints.
func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// totalPages computes the page count from a total row count and page size.
func totalPages(total int64, limit int32) int64 {
	if limit <= 0 {
		return 1
	}

	return (total + int64(limit) - 1) / int64(limit)
}

// ---------------------------------------------------------------------------
// Catalog Handlers
// ---------------------------------------------------------------------------

// ListPageTypes handles GET /api/v1/admin/pagebuilder/page-types.
// Returns the canonical page_type values from Section 16. Rule 16:
// the frontend must not hardcode these options.
func (h *Handlers) ListPageTypes(w http.ResponseWriter, r *http.Request) {
	// No special permission required; the caller already passed
	// RequireAuth and pages.manage on the pages list itself.
	options := []map[string]string{
		{"value": "landing", "label": "Landing"},
		{"value": "about", "label": "About"},
		{"value": "service", "label": "Service"},
		{"value": "event", "label": "Event"},
		{"value": "custom", "label": "Custom"},
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": options})
}

// ---------------------------------------------------------------------------
// Block Handlers
// ---------------------------------------------------------------------------

// ListBlocks handles GET /api/v1/admin/pagebuilder/blocks.
func (h *Handlers) ListBlocks(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePageParams(r)

	blocks, total, err := h.Service.ListBlocks(r.Context(), tenantID, r.URL.Query().Get("q"), page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        blocks,
		"total":       total,
		"total_pages": totalPages(total, limit),
		"page":        page,
		"page_size":   limit,
	})
}

// GetBlock handles GET /api/v1/admin/pagebuilder/blocks/{id}.
func (h *Handlers) GetBlock(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	block, err := h.Service.GetBlock(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": block})
}

// CreateBlock handles POST /api/v1/admin/pagebuilder/blocks.
func (h *Handlers) CreateBlock(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateBlockInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	block, err := h.Service.CreateBlock(r.Context(), tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": block})
}

// UpdateBlock handles PATCH /api/v1/admin/pagebuilder/blocks/{id}.
func (h *Handlers) UpdateBlock(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input UpdateBlockInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	block, err := h.Service.UpdateBlock(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": block})
}

// DeleteBlock handles DELETE /api/v1/admin/pagebuilder/blocks/{id}.
func (h *Handlers) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteBlock(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Template Handlers
// ---------------------------------------------------------------------------

// ListTemplates handles GET /api/v1/admin/pagebuilder/templates.
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")

	page, limit := parsePageParams(r)

	templates, total, err := h.Service.ListTemplates(r.Context(), tenantID, q, category, page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        templates,
		"total":       total,
		"total_pages": totalPages(total, limit),
		"page":        page,
		"page_size":   limit,
	})
}

// GetTemplate handles GET /api/v1/admin/pagebuilder/templates/{id}.
func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tmpl, err := h.Service.GetTemplate(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

// CreateTemplate handles POST /api/v1/admin/pagebuilder/templates.
func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateTemplateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	tmpl, err := h.Service.CreateTemplate(r.Context(), tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": tmpl})
}

// UpdateTemplate handles PATCH /api/v1/admin/pagebuilder/templates/{id}.
func (h *Handlers) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input UpdateTemplateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	tmpl, err := h.Service.UpdateTemplate(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}

// DeleteTemplate handles DELETE /api/v1/admin/pagebuilder/templates/{id}.
func (h *Handlers) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteTemplate(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Theme Handlers
// ---------------------------------------------------------------------------

// ListThemes handles GET /api/v1/admin/pagebuilder/themes.
func (h *Handlers) ListThemes(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := parsePageParams(r)

	themes, total, err := h.Service.ListThemes(r.Context(), tenantID, r.URL.Query().Get("q"), page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        themes,
		"total":       total,
		"total_pages": totalPages(total, limit),
		"page":        page,
		"page_size":   limit,
	})
}

// GetTheme handles GET /api/v1/admin/pagebuilder/themes/{id}.
func (h *Handlers) GetTheme(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	theme, err := h.Service.GetTheme(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": theme})
}

// CreateTheme handles POST /api/v1/admin/pagebuilder/themes.
func (h *Handlers) CreateTheme(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateThemeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	theme, err := h.Service.CreateTheme(r.Context(), tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": theme})
}

// UpdateTheme handles PATCH /api/v1/admin/pagebuilder/themes/{id}.
func (h *Handlers) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input UpdateThemeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	theme, err := h.Service.UpdateTheme(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": theme})
}

// DeleteTheme handles DELETE /api/v1/admin/pagebuilder/themes/{id}.
func (h *Handlers) DeleteTheme(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteTheme(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// SetDefaultTheme handles POST /api/v1/admin/pagebuilder/themes/{id}/default.
func (h *Handlers) SetDefaultTheme(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.SetDefaultTheme(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Return the updated theme so the client receives the standard {"data": ...}
	// envelope (Rule 5). The service only toggles is_default, so re-fetch.
	theme, err := h.Service.GetTheme(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": theme})
}

// ---------------------------------------------------------------------------
// Page Handlers
// ---------------------------------------------------------------------------

// ListPages handles GET /api/v1/admin/pages.
func (h *Handlers) ListPages(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	page, limit := parsePageParams(r)

	pages, total, err := h.Service.ListPages(r.Context(), tenantID, PageListFilter{
		Q:        q.Get("q"),
		PageType: q.Get("page_type"),
		Slug:     q.Get("slug"),
		Status:   q.Get("status"),
	}, page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        pages,
		"total":       total,
		"total_pages": totalPages(total, limit),
		"page":        page,
		"page_size":   limit,
	})
}

// GetPage handles GET /api/v1/admin/pages/{id}.
func (h *Handlers) GetPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	page, err := h.Service.GetPage(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// CreatePage handles POST /api/v1/admin/pages.
func (h *Handlers) CreatePage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreatePageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	page, err := h.Service.CreatePage(r.Context(), tenantID, user.UserID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": page})
}

// UpdatePage handles PATCH /api/v1/admin/pages/{id}.
func (h *Handlers) UpdatePage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input UpdatePageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	page, err := h.Service.UpdatePage(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// DeletePage handles DELETE /api/v1/admin/pages/{id}.
func (h *Handlers) DeletePage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeletePage(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// PublishPage handles POST /api/v1/admin/pages/{id}/publish.
func (h *Handlers) PublishPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	page, err := h.Service.PublishPage(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// ArchivePage handles POST /api/v1/admin/pages/{id}/archive.
func (h *Handlers) ArchivePage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	page, err := h.Service.ArchivePage(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// ReorderBlocks handles PATCH /admin/pages/{id}/blocks/reorder.
func (h *Handlers) ReorderBlocks(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	var input struct {
		Blocks []map[string]any `json:"blocks"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	page, err := h.Service.ReorderBlocks(r.Context(), id, tenantID, input.Blocks)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": page})
}

// ListPublishedPages handles GET /pages — public endpoint, no auth required.
// Returns only pages with status='published', ordered by created_at DESC.
func (h *Handlers) ListPublishedPages(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	pages, err := h.Service.ListPublishedPages(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": pages})
}

// GetPublishedPage handles GET /pages/{slug} — public endpoint, no auth required.
// Widget blocks within the page's blocks array are server-side resolved and a
// resolved literal map is attached to the response for the SSR resolver.
func (h *Handlers) GetPublishedPage(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	page, err := h.Service.GetPageBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Resolve widget blocks server-side (Section 16: "blocks + resolved
	// widget data"). Walk the blocks JSONB, find blocks with type=widget,
	// render each widget, and inject the data into props._resolved.
	if h.Widgets != nil && len(page.Blocks) > 0 {
		page.Blocks = h.resolveWidgetBlocks(r.Context(), page.Blocks)
	}

	literalsMap := h.resolvePageLiterals(r, tenantID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data":     page,
		"literals": literalsMap,
	})
}

func (h *Handlers) resolvePageLiterals(r *http.Request, tenantID pgtype.UUID) map[string]string {
	if h.Catalog == nil {
		return map[string]string{}
	}

	// ADR-064: geo/locale/tenant fields come from the request's VisitorContext
	// + GeoIP + Accept-Language via the shared request-context constructor.
	rc := literals.NewRequestContext(r, tenantID, "")

	m, err := h.Catalog.ResolveMap(r.Context(), tenantID, rc)
	if err != nil {
		slog.Warn(
			"pagebuilder: failed to resolve page literals",
			"tenant_id", tenantID.String(),
			"error", err,
		)

		return map[string]string{}
	}

	return m
}

// resolveWidgetBlocks walks a blocks JSONB array, resolves any widget blocks,
// and returns the updated JSONB. Non-widget blocks are passed through unchanged.
// Errors for individual widgets are logged but don't fail the page render —
// the frontend will show a fallback "Unable to load" state.
func (h *Handlers) resolveWidgetBlocks(ctx context.Context, blocksJSON json.RawMessage) json.RawMessage {
	var blocks []map[string]any
	if err := json.Unmarshal(blocksJSON, &blocks); err != nil {
		return blocksJSON // not a valid array — return as-is
	}

	for i, block := range blocks {
		blockType, _ := block["type"].(string)
		if blockType != "widget" {
			continue
		}

		props, ok := block["props"].(map[string]any)
		if !ok {
			continue
		}

		widgetIDStr, _ := props["widget_id"].(string)
		if widgetIDStr == "" {
			continue
		}

		var widgetID pgtype.UUID
		if err := widgetID.Scan(widgetIDStr); err != nil {
			slog.Warn("pagebuilder: invalid widget_id in block", "widget_id", widgetIDStr, "error", err)
			continue
		}

		data, widgetType, cardType, err := h.Widgets.RenderWidgetByID(ctx, widgetID)
		if err != nil {
			slog.Warn("pagebuilder: failed to resolve widget", "widget_id", widgetIDStr, "error", err)
			continue
		}

		resolved := map[string]any{
			"widget_type": widgetType,
			"data":        json.RawMessage(data),
		}
		if cardType != "" {
			resolved["card_type"] = cardType
		}

		props["_resolved"] = resolved
		blocks[i]["props"] = props
	}

	resolved, err := json.Marshal(blocks)
	if err != nil {
		return blocksJSON
	}

	return resolved
}

// ─── P5.6 Pagebuilder Extras ─────────────────────────────────────────────

// SaveAsTemplate handles POST /admin/pages/:id/save-as-template.
// It snapshots the page's current block array into a new page template
// (Section 16: "save-as-template" preserves the page's blocks).
func (h *Handlers) SaveAsTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	pageID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "Template name is required")
		return
	}

	// Fetch the source page so its block array is copied into the template.
	// The prior implementation discarded pageID and saved an empty block
	// array (§21 stub) — that lost all page content on save-as-template.
	page, err := h.Service.GetPage(r.Context(), pageID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	template, err := h.Service.CreateTemplate(r.Context(), tenantID, CreateTemplateInput{
		Name:        input.Name,
		Slug:        slugify(input.Name),
		Description: description,
		PageType:    "page",
		Blocks:      page.Blocks,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": template})
}

// DuplicatePage handles POST /admin/pages/:id/duplicate.
// It clones the source page (blocks, meta, theme, page_type) into a new draft
// with a unique slug and " (Copy)" title suffix, so an admin can redesign the
// copy for another app's homepage without touching the original.
func (h *Handlers) DuplicatePage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	pageID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}

	page, err := h.Service.DuplicatePage(r.Context(), pageID, tenantID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": page})
}

// slugify lowercases s, replaces non-alphanumeric runs with a single hyphen,
// and trims leading/trailing hyphens. "Hello World!!!" → "hello-world".
func slugify(s string) string {
	var b []byte

	prevDash := true // trim leading dashes

	for i := range len(s) {
		c := s[i]

		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b = append(b, c)
			prevDash = false
		case c >= 'A' && c <= 'Z':
			b = append(b, c+32)
			prevDash = false
		default:
			if !prevDash {
				b = append(b, '-')
				prevDash = true
			}
		}
	}
	// trim trailing dash
	if len(b) > 0 && b[len(b)-1] == '-' {
		b = b[:len(b)-1]
	}

	return string(b)
}
