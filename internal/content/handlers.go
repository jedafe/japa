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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

const defaultPageSize int32 = 20

// Task type constant for embedding generation — matches worker.TypeAIGenerateContentEmbedding.
const taskTypeAIGenerateContentEmbedding = "ai:generate_content_embedding"

// TaskEnqueueClient is the interface for enqueueing background tasks.
// The Asynq client satisfies this interface. A nil client means tasks
// are silently skipped (useful for tests and development).
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handlers holds dependencies for content HTTP handlers.
type Handlers struct {
	Service     *Service
	AsynqClient TaskEnqueueClient
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

// enqueueContentEmbedding enqueues an async task to generate a vector embedding
// for the given content. The embedding text is a combination of title and body.
// Fire-and-forget: errors are logged but never propagated to the caller.
func (h *Handlers) enqueueContentEmbedding(contentID, tenantID pgtype.UUID, title, body string) {
	if h.AsynqClient == nil {
		return
	}

	text := title
	if body != "" {
		if text != "" {
			text += "\n\n"
		}

		text += body
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
		"tenant_id":  tenantID.String(),
		"text":       text,
	})
	if err != nil {
		slog.Warn("content: failed to marshal embedding task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(taskTypeAIGenerateContentEmbedding, payload),
		asynq.Queue("default"),
	); err != nil {
		slog.Warn("content: failed to enqueue embedding task",
			"content_id", contentID.String(), "error", err)
	}
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
		BodyHtml     string          `json:"body_html"` //nolint:revive // matches sqlc-generated field name
		FieldValues  json.RawMessage `json:"field_values"`
		CanonicalUrl *string         `json:"canonical_url"` //nolint:revive // matches sqlc-generated field name
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
		Body:         SanitizeText(input.Body),
		BodyHtml:     SanitizeHTML(input.BodyHtml),
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
		// Validate that publish_at is in the future.
		if createInput.PublishAt.Valid && createInput.PublishAt.Time.Before(time.Now()) {
			writeError(w, http.StatusBadRequest, "publish_at_in_past", "publish_at must be in the future")
			return
		}
	}

	content, err := h.Service.CreateContent(r.Context(), tenantID, createInput)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async embedding generation — fire-and-forget.
	h.enqueueContentEmbedding(content.ID, tenantID, input.Title, createInput.Body)

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

	// Increment engagement view for velocity scoring (fire-and-forget).
	_ = h.Service.IncrementEngagementView(r.Context(), content.ID)

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
		BodyHtml     string          `json:"body_html"` //nolint:revive // matches sqlc-generated field name
		FieldValues  json.RawMessage `json:"field_values"`
		CanonicalUrl *string         `json:"canonical_url"` //nolint:revive // matches sqlc-generated field name
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
		Body:         SanitizeText(input.Body),
		BodyHtml:     SanitizeHTML(input.BodyHtml),
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
		// Validate that publish_at is in the future when setting a new schedule.
		if updateInput.PublishAt.Valid && updateInput.PublishAt.Time.Before(time.Now()) {
			writeError(w, http.StatusBadRequest, "publish_at_in_past", "publish_at must be in the future")
			return
		}
	}

	content, err := h.Service.UpdateContent(r.Context(), id, tenantID, user.UserID, updateInput)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async embedding generation — fire-and-forget.
	h.enqueueContentEmbedding(id, tenantID, input.Title, updateInput.Body)

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

	tenantID, ok := h.requireTenant(w, r)
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

	reaction, err := h.Service.AddReaction(r.Context(), tenantID, EntityContent, contentID, user.UserID, input.ReactionKey)
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

	tenantID, ok := h.requireTenant(w, r)
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

	if err := h.Service.RemoveReaction(r.Context(), tenantID, EntityContent, contentID, user.UserID, reactionKey); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ListReactions handles GET /api/v1/content/{id}/reactions.
func (h *Handlers) ListReactions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	counts, err := h.Service.CountReactionsByEntity(r.Context(), tenantID, EntityContent, contentID)
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

// ---------------------------------------------------------------------------
// Content Bookmark Handlers (Task 12.09)
// ---------------------------------------------------------------------------

// AddBookmark handles POST /api/v1/content/{id}/bookmark.
func (h *Handlers) AddBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Folder string `json:"folder"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	bm, err := h.Service.CreateBookmark(r.Context(), tenantID, user.UserID, EntityContent, contentID, input.Folder)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": bm})
}

// RemoveBookmark handles DELETE /api/v1/content/{id}/bookmark.
func (h *Handlers) RemoveBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.RemoveBookmark(r.Context(), user.UserID, EntityContent, contentID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ListBookmarks handles GET /api/v1/content/{id}/bookmark/status.
func (h *Handlers) GetBookmarkStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	bookmarked, err := h.Service.IsBookmarked(r.Context(), user.UserID, EntityContent, contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"bookmarked": bookmarked}})
}

// ---------------------------------------------------------------------------
// Content Tags Handlers (Task 12.10)
// ---------------------------------------------------------------------------

// ListContentTags handles GET /api/v1/content/{id}/tags.
func (h *Handlers) ListContentTags(w http.ResponseWriter, r *http.Request) {
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tags, err := h.Service.ListContentTags(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tags})
}

// AddContentTag handles POST /api/v1/content/{id}/tags.
func (h *Handlers) AddContentTag(w http.ResponseWriter, r *http.Request) {
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
		TagID string `json:"tag_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var tagID pgtype.UUID
	if err := tagID.Scan(input.TagID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag_id format")
		return
	}

	if err := h.Service.AddContentTag(r.Context(), contentID, tagID); err != nil {
		handleServiceError(w, err)
		return
	}

	_ = user // permission check handled by middleware

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]bool{"added": true}})
}

// RemoveContentTag handles DELETE /api/v1/content/{id}/tags/{tagId}.
func (h *Handlers) RemoveContentTag(w http.ResponseWriter, r *http.Request) {
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tagID, err := parseUUIDParam(r, "tagId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag ID format")
		return
	}

	if err := h.Service.RemoveContentTag(r.Context(), contentID, tagID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Content Share Handler (Task 12.12)
// ---------------------------------------------------------------------------

// ShareContent handles POST /api/v1/content/{id}/share.
func (h *Handlers) ShareContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Platform string `json:"platform"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	share, err := h.Service.ShareContent(r.Context(), tenantID, contentID, user.UserID, input.Platform)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": share})
}

// ---------------------------------------------------------------------------
// Related Content Handler (Task 12.13)
// ---------------------------------------------------------------------------

// GetRelatedContent handles GET /api/v1/content/{id}/related.
func (h *Handlers) GetRelatedContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	limit := parsePageSize(r)
	if limit > 12 {
		limit = 12
	}

	// Get the content's embedding from content_scores.
	score, err := h.Service.GetContentScore(r.Context(), contentID)
	if err != nil || score.RelevanceEmbedding == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	contents, err := h.Service.GetRelatedContent(r.Context(), tenantID, contentID, *score.RelevanceEmbedding, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": contents})
}
