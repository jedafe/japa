// Package content — HTTP handlers for the Content Type System.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package content

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
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for content HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
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
// should abort.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

// requireUser resolves the authenticated user from the request context and
// writes a 401 error when none is available.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response.
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
		return DefaultPageSize
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return int32(n)
}

// ---------------------------------------------------------------------------
// Content CRUD Handlers
// ---------------------------------------------------------------------------

// CreateContent handles POST /api/v1/content.
func (h *Handlers) CreateContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentType  string          `json:"content_type"`
		Title        string          `json:"title"`
		Slug         string          `json:"slug"`
		Body         string          `json:"body"`
		BodyHtml     string          `json:"body_html"`
		FieldValues  json.RawMessage `json:"field_values"`
		CanonicalUrl *string         `json:"canonical_url"`
		Status       string          `json:"status"`
		Visibility   string          `json:"visibility"`
		PublishAt    *string         `json:"publish_at"`
		SpaceID      *string         `json:"space_id"`
		GeoSpaceID   *string         `json:"geo_space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		writeError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}
	if input.ContentType == "" {
		input.ContentType = TypeDiscussion
	}
	if input.Status == "" {
		input.Status = StatusDraft
	}
	if input.Visibility == "" {
		input.Visibility = VisibilityPublic
	}

	createInput := CreateContentInput{
		AuthorID:     user.UserID,
		ContentType:  input.ContentType,
		Title:        input.Title,
		Slug:         input.Slug,
		Body:         input.Body,
		BodyHtml:     input.BodyHtml,
		FieldValues:  input.FieldValues,
		CanonicalUrl: input.CanonicalUrl,
		Status:       input.Status,
		Visibility:   input.Visibility,
	}

	if input.SpaceID != nil && *input.SpaceID != "" {
		if err := createInput.SpaceID.Scan(*input.SpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
	}
	if input.GeoSpaceID != nil && *input.GeoSpaceID != "" {
		if err := createInput.GeoSpaceID.Scan(*input.GeoSpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo_space_id format")
			return
		}
	}
	if input.PublishAt != nil && *input.PublishAt != "" {
		if err := createInput.PublishAt.Scan(*input.PublishAt); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_publish_at", "Invalid publish_at format")
			return
		}
	}

	content, err := h.Service.CreateContent(r.Context(), tenantID, createInput)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": content})
}

// GetContent handles GET /api/v1/content/{slug}.
func (h *Handlers) GetContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	content, err := h.Service.GetContentBySlug(r.Context(), slug, tenantID)
	if err != nil {
		// Try slug redirect before returning not found.
		if newSlug, found := h.Service.ResolveSlugRedirect(r.Context(), EntityContent, slug); found {
			content, err = h.Service.GetContentBySlug(r.Context(), newSlug, tenantID)
		}
		if err != nil {
			handleServiceError(w, err)
			return
		}
	}

	// Increment view count (fire-and-forget, don't fail the request on error).
	_ = h.Service.IncrementContentView(r.Context(), content.ID, tenantID)

	writeJSON(w, http.StatusOK, map[string]any{"data": content})
}

// ListContent handles GET /api/v1/content with optional type and space filters.
func (h *Handlers) ListContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	limit := parsePageSize(r)
	var cursor pgtype.Timestamptz
	if cs := r.URL.Query().Get("cursor"); cs != "" {
		if err := cursor.Scan(cs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid cursor timestamp")
			return
		}
	}

	contentType := r.URL.Query().Get("type")
	spaceIDStr := r.URL.Query().Get("space_id")

	var contents any
	var err error
	if spaceIDStr != "" {
		var spaceID pgtype.UUID
		if err := spaceID.Scan(spaceIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
		contents, err = h.Service.ListContentsBySpace(r.Context(), tenantID, spaceID, cursor, limit)
	} else if contentType != "" {
		contents, err = h.Service.ListContentsByType(r.Context(), tenantID, contentType, cursor, limit)
	} else {
		contents, err = h.Service.ListContents(r.Context(), tenantID, cursor, limit)
	}
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": contents})
}

// UpdateContent handles PATCH /api/v1/content/{id}.
func (h *Handlers) UpdateContent(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Title        string          `json:"title"`
		Slug         string          `json:"slug"`
		Body         string          `json:"body"`
		BodyHtml     string          `json:"body_html"`
		FieldValues  json.RawMessage `json:"field_values"`
		CanonicalUrl *string         `json:"canonical_url"`
		Status       string          `json:"status"`
		Visibility   string          `json:"visibility"`
		PublishAt    *string         `json:"publish_at"`
		GeoSpaceID   *string         `json:"geo_space_id"`
		IsLocked     bool            `json:"is_locked"`
		IsPinned     bool            `json:"is_pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	updateInput := UpdateContentInput{
		Title:        input.Title,
		Slug:         input.Slug,
		Body:         input.Body,
		BodyHtml:     input.BodyHtml,
		FieldValues:  input.FieldValues,
		CanonicalUrl: input.CanonicalUrl,
		Status:       input.Status,
		Visibility:   input.Visibility,
		IsLocked:     input.IsLocked,
		IsPinned:     input.IsPinned,
	}
	if input.GeoSpaceID != nil && *input.GeoSpaceID != "" {
		if err := updateInput.GeoSpaceID.Scan(*input.GeoSpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo_space_id format")
			return
		}
	}
	if input.PublishAt != nil && *input.PublishAt != "" {
		if err := updateInput.PublishAt.Scan(*input.PublishAt); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_publish_at", "Invalid publish_at format")
			return
		}
	}

	content, err := h.Service.UpdateContent(r.Context(), id, tenantID, user.UserID, updateInput)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": content})
}

// DeleteContent handles DELETE /api/v1/content/{id}.
func (h *Handlers) DeleteContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	if err := h.Service.DeleteContent(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Revision Handlers
// ---------------------------------------------------------------------------

// ListRevisions handles GET /api/v1/content/{id}/revisions.
func (h *Handlers) ListRevisions(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	revisions, err := h.Service.ListContentRevisions(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": revisions})
}

// ---------------------------------------------------------------------------
// Pin Handlers
// ---------------------------------------------------------------------------

// PinContent handles POST /api/v1/content/{id}/pin.
// The space_id is provided in the request body.
func (h *Handlers) PinContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		SpaceID  string `json:"space_id"`
		Position int32  `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.SpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_space_id", "space_id is required")
		return
	}

	var spaceID pgtype.UUID
	if err := spaceID.Scan(input.SpaceID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
		return
	}

	pin, err := h.Service.PinContent(r.Context(), spaceID, contentID, user.UserID, input.Position)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": pin})
}

// UnpinContent handles DELETE /api/v1/content/{id}/pin.
// The space_id is provided as a query parameter.
func (h *Handlers) UnpinContent(w http.ResponseWriter, r *http.Request) {
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	spaceIDStr := r.URL.Query().Get("space_id")
	if spaceIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing_space_id", "space_id query parameter is required")
		return
	}
	var spaceID pgtype.UUID
	if err := spaceID.Scan(spaceIDStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
		return
	}
	if err := h.Service.UnpinContent(r.Context(), spaceID, contentID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Reaction Handlers
// ---------------------------------------------------------------------------

// AddReaction handles POST /api/v1/content/{id}/reactions.
func (h *Handlers) AddReaction(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		ReactionKey string `json:"reaction_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.ReactionKey == "" {
		writeError(w, http.StatusBadRequest, "missing_reaction_key", "reaction_key is required")
		return
	}

	reaction, err := h.Service.AddReaction(r.Context(), EntityContent, contentID, user.UserID, input.ReactionKey)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": reaction})
}

// RemoveReaction handles DELETE /api/v1/content/{id}/reactions/{key}.
func (h *Handlers) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}
	reactionKey := chi.URLParam(r, "key")
	if reactionKey == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Reaction key is required")
		return
	}

	if err := h.Service.RemoveReaction(r.Context(), EntityContent, contentID, user.UserID, reactionKey); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ListReactions handles GET /api/v1/content/{id}/reactions.
func (h *Handlers) ListReactions(w http.ResponseWriter, r *http.Request) {
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	counts, err := h.Service.CountReactionsByEntity(r.Context(), EntityContent, contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": counts})
}

// ---------------------------------------------------------------------------
// JSON-LD Template Handlers
// ---------------------------------------------------------------------------

// ListJSONLDTemplates handles GET /api/v1/content/jsonld-templates.
func (h *Handlers) ListJSONLDTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	templates, err := h.Service.ListJSONLDTemplates(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": templates})
}

// UpsertJSONLDTemplate handles PUT /api/v1/content/jsonld-templates.
func (h *Handlers) UpsertJSONLDTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentType string          `json:"content_type"`
		Template    json.RawMessage `json:"template"`
		IsActive    bool            `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.ContentType == "" {
		writeError(w, http.StatusBadRequest, "missing_content_type", "content_type is required")
		return
	}

	tmpl, err := h.Service.UpsertJSONLDTemplate(r.Context(), tenantID, input.ContentType, input.Template, input.IsActive)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tmpl})
}