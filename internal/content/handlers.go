// Package content — HTTP handlers for the Content Type System.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

const defaultPageSize int32 = 20

// defaultMaxTags is the fallback for content.max_tags when the setting is
// unset or unparseable. Matches the settings catalog default.
const defaultMaxTags = 5

// TaskEnqueueClient is the interface for enqueueing background tasks.
// The Asynq client satisfies this interface. A nil client means tasks
// are silently skipped (useful for tests and development).
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// MentionNotifier dispatches mention notifications via the batch system
// (Section 35 — mentions:notify consolidated into notification:dispatch_batch
// per Section 11). Fire-and-forget per Rule 8.
type MentionNotifier interface {
	DispatchNotification(
		ctx context.Context,
		tenantID, recipientID pgtype.UUID,
		nType, subjectTitle string,
		actorID pgtype.UUID,
		subjectType string,
		subjectID pgtype.UUID,
		subjectURL, actorName, actorAvatar string,
	) error
}

// Handlers holds dependencies for content HTTP handlers.
type Handlers struct {
	Service      *Service
	AsynqClient  TaskEnqueueClient
	SpamPipeline *spam.SpamPipeline
	settings     *config.Settings
	notifier     MentionNotifier // nil disables mention notifications (test-only)
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// SetSpamPipeline injects the spam detection pipeline for content safety checks.
func (h *Handlers) SetSpamPipeline(p *spam.SpamPipeline) {
	h.SpamPipeline = p
}

// SetSettings injects the settings service for reading content.max_tags.
func (h *Handlers) SetSettings(s *config.Settings) {
	h.settings = s
}

// SetMentionNotifier wires the notification dispatcher used to notify
// @mentioned users after content creation (BL-002 — mentions:notify →
// notification:dispatch_batch).
func (h *Handlers) SetMentionNotifier(n MentionNotifier) { h.notifier = n }

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
	case errors.Is(err, errpkg.ErrDuplicate), errors.Is(err, ErrDuplicateContent):
		writeError(w, http.StatusConflict, "duplicate_content", "Content with this body already exists")
	case errors.Is(err, ErrOrgPostForbidden):
		writeError(w, http.StatusForbidden, "org_post_forbidden", "Not authorized to post as this organization")
	case errors.Is(err, ErrEditWindowExpired):
		writeError(w, http.StatusForbidden, "edit_window_expired", "The edit window for this content has expired")
	default:
		slog.Error("content service error", "error", err, "error_type", fmt.Sprintf("%T", err))
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
// for the given content. Payload contains only IDs — the worker handler re-fetches
// fresh content text at processing time to avoid stale-on-edit issues.
// Fire-and-forget: errors are logged but never propagated to the caller.
func (h *Handlers) enqueueContentEmbedding(contentID, tenantID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
		"tenant_id":  tenantID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal embedding task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeAIGenerateContentEmbedding, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeAIGenerateContentEmbedding)),
	); err != nil {
		slog.Warn("content: failed to enqueue embedding task",
			"content_id", contentID.String(), "error", err)
	}
}

// enqueueAffiliateLinkScan enqueues the content:scan_affiliate_links task
// (Section 55). Fire-and-forget: errors are logged but never propagated.
// AUDIT-BE-TASK-001: the task was defined and registered but never enqueued.
func (h *Handlers) enqueueAffiliateLinkScan(contentID, tenantID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
		"tenant_id":  tenantID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal affiliate scan task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeContentScanAffiliateLinks, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeContentScanAffiliateLinks)),
	); err != nil {
		slog.Warn("content: failed to enqueue affiliate scan task",
			"content_id", contentID.String(), "error", err)
	}
}

// enqueueSearchIndex enqueues an async task to sync content to the Meilisearch
// search index. operation can be "index" (create/update) or "delete".
func (h *Handlers) enqueueSearchIndex(contentID, tenantID pgtype.UUID, operation string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id":  contentID.String(),
		"entity_type": "content",
		"tenant_id":   tenantID.String(),
		"operation":   operation,
	})
	if err != nil {
		slog.Warn("content: failed to marshal search index payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeSearchIndexSync, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeSearchIndexSync)),
	); err != nil {
		slog.Warn("content: failed to enqueue search index task",
			"content_id", contentID.String(), "error", err)
	}
}

// enqueueModerationScan enqueues an async content moderation scan task.
// Fire-and-forget: errors are logged but never propagated to the caller.
func (h *Handlers) enqueueModerationScan(entityType string, entityID, tenantID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"entity_type": entityType,
		"entity_id":   entityID.String(),
		"tenant_id":   tenantID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal moderation scan payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeModerationScan, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeModerationScan)),
	); err != nil {
		slog.Warn("content: failed to enqueue moderation scan task",
			"entity_id", entityID.String(), "error", err)
	}
}

// enqueueFeedFanOut enqueues a content:fan_out_to_feeds task for published
// content (BL-005). The fan-out type is "space" if the post is in a space,
// "followers" otherwise. The worker's "bubble" branch is triggered
// automatically by the space fan-out when the space has parent spaces.
// Fire-and-forget per Rule 8.
func (h *Handlers) enqueueFeedFanOut(contentID, tenantID, spaceID pgtype.UUID, contentType string) {
	if h.AsynqClient == nil {
		return
	}
	fanOutType := "followers"
	if spaceID.Valid {
		fanOutType = "space"
	}
	payload, err := json.Marshal(map[string]string{
		"content_id":   contentID.String(),
		"tenant_id":    tenantID.String(),
		"space_id":     spaceID.String(),
		"content_type": contentType,
		"fan_out_type": fanOutType,
	})
	if err != nil {
		slog.Warn("content: failed to marshal fan-out payload", "error", err)
		return
	}
	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeContentFanOutToFeeds, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeContentFanOutToFeeds)),
	); err != nil {
		slog.Warn("content: failed to enqueue fan-out task",
			"content_id", contentID.String(), "error", err)
	}
}

// dispatchMentionNotifications extracts @mentioned users from the content
// body (TipTap JSON) and fires a content_mention notification to each via
// the batch system (Section 35 — BL-002). Fire-and-forget per Rule 8:
// errors are logged but never propagated to the caller.
//
// Self-mentions (mentioning the content author) are skipped.
func (h *Handlers) dispatchMentionNotifications(ctx context.Context, tenantID, authorID, contentID pgtype.UUID, body string) {
	if h.notifier == nil {
		return
	}
	mentioned, err := mentions.ExtractMentions([]byte(body))
	if err != nil {
		// Body may not be valid TipTap JSON (e.g., plain text); skip mentions.
		return
	}
	for _, m := range mentioned {
		if m.EntityType != "user" || !m.ID.Valid || m.ID == authorID {
			continue
		}
		if err := h.notifier.DispatchNotification(
			ctx, tenantID, m.ID,
			"content_mention", "",
			authorID,
			"content", contentID,
			"", "", "",
		); err != nil {
			slog.Warn("content: failed to dispatch content_mention notification",
				"user_id", m.ID.String(), "error", err)
		}
	}
}

// enqueueLinkPreview enqueues an async link preview fetch task for a URL
// found in a content block. Fire-and-forget.
func (h *Handlers) enqueueLinkPreview(url, blockID string, tenantID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"url":       url,
		"block_id":  blockID,
		"tenant_id": tenantID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal link preview payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeLinkPreviewFetch, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeLinkPreviewFetch)),
	); err != nil {
		slog.Warn("content: failed to enqueue link preview task",
			"url", url, "error", err)
	}
}

// enqueueEngagementUpdate enqueues an async engagement count update task.
// Fire-and-forget.
func (h *Handlers) enqueueEngagementUpdate(entityType string, entityID pgtype.UUID, engagementType string, delta int) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]interface{}{
		"entity_type":     entityType,
		"entity_id":       entityID.String(),
		"engagement_type": engagementType,
		"delta":           delta,
	})
	if err != nil {
		slog.Warn("content: failed to marshal engagement update payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeEngagementCountUpdate, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeEngagementCountUpdate)),
	); err != nil {
		slog.Warn("content: failed to enqueue engagement update task",
			"entity_id", entityID.String(), "error", err)
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
		ContentType     string          `json:"content_type"`
		Title           string          `json:"title"`
		Slug            string          `json:"slug"`
		Body            string          `json:"body"`
		BodyHtml        string          `json:"body_html"` //nolint:revive // matches sqlc-generated field name
		FieldValues     json.RawMessage `json:"field_values"`
		CanonicalUrl    *string         `json:"canonical_url"` //nolint:revive // matches sqlc-generated field name
		HeaderImageID   *string         `json:"header_image_id"`
		FeaturedImageID *string         `json:"featured_image_id"`
		MetaTitle       *string         `json:"meta_title"`
		MetaDescription *string         `json:"meta_description"`
		OgImageUrl      *string         `json:"og_image_url"` //nolint:revive // matches sqlc-generated field name
		SeoIndexable    *bool           `json:"seo_indexable"`
		Status          string          `json:"status"`
		Visibility      string          `json:"visibility"`
		PublishAt       *string         `json:"publish_at"`
		SpaceID         *string         `json:"space_id"`
		GeoSpaceID      *string         `json:"geo_space_id"`
		OrganizationID  *string         `json:"organization_id"`
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

	// Derive plain-text body from body_html when the client sends only
	// body_html (the Tiptap editor always produces HTML, never plain text).
	// Without this, body is always "" → ComputeContentHash("") is the same
	// for every empty-body draft → unique constraint violation on
	// idx_contents_tenant_content_hash (Rule 12 / §41 spam prevention).
	body := input.Body
	if body == "" && input.BodyHtml != "" {
		body = SanitizeText(input.BodyHtml)
	}

	createInput := CreateContentInput{
		AuthorID:        user.UserID,
		ContentType:     input.ContentType,
		Title:           input.Title,
		Slug:            input.Slug,
		Body:            SanitizeText(body),
		BodyHtml:        SanitizeHTML(input.BodyHtml),
		FieldValues:     input.FieldValues,
		CanonicalUrl:    input.CanonicalUrl,
		MetaTitle:       input.MetaTitle,
		MetaDescription: input.MetaDescription,
		OgImageUrl:      input.OgImageUrl,
		SeoIndexable:    true, // default: indexable; caller can override
		Status:          input.Status,
		Visibility:      input.Visibility,
	}
	if input.SeoIndexable != nil {
		createInput.SeoIndexable = *input.SeoIndexable
	}

	if input.SpaceID != nil && *input.SpaceID != "" {
		if err := createInput.SpaceID.Scan(*input.SpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
	}

	if input.OrganizationID != nil && *input.OrganizationID != "" {
		if err := createInput.OrganizationID.Scan(*input.OrganizationID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id format")
			return
		}
	}

	if input.GeoSpaceID != nil && *input.GeoSpaceID != "" {
		if err := createInput.GeoSpaceID.Scan(*input.GeoSpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo_space_id format")
			return
		}
	}

	if input.HeaderImageID != nil && *input.HeaderImageID != "" {
		if err := createInput.HeaderImageID.Scan(*input.HeaderImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_header_image_id", "Invalid header_image_id format")
			return
		}
	}

	if input.FeaturedImageID != nil && *input.FeaturedImageID != "" {
		if err := createInput.FeaturedImageID.Scan(*input.FeaturedImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_featured_image_id", "Invalid featured_image_id format")
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

	// Spam pipeline check — multi-layer content safety (BE-034).
	if h.SpamPipeline != nil {
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		spamResult, err := h.SpamPipeline.Check(r.Context(), tenantID, spam.SpamCheckRequest{
			Content:    input.Body,
			UserID:     user.UserID,
			IP:         clientIP,
			UserType:   "existing",
			EntityType: "post",
		})
		if err != nil {
			slog.Warn("spam check failed", "error", err)
		} else if spamResult.Blocked {
			writeError(w, http.StatusForbidden, "content_blocked", "Content blocked by safety filters")
			return
		} else if spamResult.Flagged {
			createInput.Status = StatusPending // hold for moderation review
		}
	}

	content, err := h.Service.CreateContent(r.Context(), tenantID, createInput)
	if err != nil {
		if errors.Is(err, ErrOrgPostForbidden) {
			writeError(w, http.StatusForbidden, "org_post_forbidden", "You are not authorized to post as this organization")
			return
		}
		handleServiceError(w, err)
		return
	}

	// Enqueue async embedding generation — fire-and-forget.
	h.enqueueContentEmbedding(content.ID, tenantID)

	// Enqueue search index sync — fire-and-forget.
	h.enqueueSearchIndex(content.ID, tenantID, "index")

	// Enqueue moderation scan — fire-and-forget.
	h.enqueueModerationScan("content", content.ID, tenantID)

	// Dispatch @mention notifications — fire-and-forget (BL-002).
	h.dispatchMentionNotifications(r.Context(), tenantID, user.UserID, content.ID, content.Body)

	// Enqueue feed fan-out for published content (BL-005). Scheduled posts
	// are fanned out by the scheduled-publish handler when they go live.
	if content.Status == StatusPublished {
		h.enqueueFeedFanOut(content.ID, tenantID, content.SpaceID, content.ContentType)
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
	if err := h.Service.IncrementContentView(r.Context(), content.ID, tenantID); err != nil {
		slog.Error("incrementing content view", "error", err, "content_id", content.ID.String())
	}

	// Increment engagement view for velocity scoring (fire-and-forget).
	if err := h.Service.IncrementEngagementView(r.Context(), content.ID); err != nil {
		slog.Error("incrementing engagement view", "error", err, "content_id", content.ID.String())
	}

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

	q := r.URL.Query().Get("q")

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
		contents, err = h.Service.ListContents(r.Context(), tenantID, q, cursor, limit)
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
		Title           string          `json:"title"`
		Slug            string          `json:"slug"`
		Body            string          `json:"body"`
		BodyHtml        string          `json:"body_html"` //nolint:revive // matches sqlc-generated field name
		FieldValues     json.RawMessage `json:"field_values"`
		CanonicalUrl    *string         `json:"canonical_url"` //nolint:revive // matches sqlc-generated field name
		HeaderImageID   *string         `json:"header_image_id"`
		FeaturedImageID *string         `json:"featured_image_id"`
		MetaTitle       *string         `json:"meta_title"`
		MetaDescription *string         `json:"meta_description"`
		OgImageUrl      *string         `json:"og_image_url"` //nolint:revive // matches sqlc-generated field name
		SeoIndexable    *bool           `json:"seo_indexable"`
		Status          string          `json:"status"`
		Visibility      string          `json:"visibility"`
		PublishAt       *string         `json:"publish_at"`
		GeoSpaceID      *string         `json:"geo_space_id"`
		OrganizationID  *string         `json:"organization_id"`
		IsLocked        bool            `json:"is_locked"`
		IsPinned        bool            `json:"is_pinned"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Derive plain-text body from body_html when the client sends only
	// body_html (same fix as CreateContent — see §41 / Rule 12).
	body := input.Body
	if body == "" && input.BodyHtml != "" {
		body = SanitizeText(input.BodyHtml)
	}

	updateInput := UpdateContentInput{
		Title:           input.Title,
		Slug:            input.Slug,
		Body:            SanitizeText(body),
		BodyHtml:        SanitizeHTML(input.BodyHtml),
		FieldValues:     input.FieldValues,
		CanonicalUrl:    input.CanonicalUrl,
		MetaTitle:       input.MetaTitle,
		MetaDescription: input.MetaDescription,
		OgImageUrl:      input.OgImageUrl,
		SeoIndexable:    input.SeoIndexable,
		Status:          input.Status,
		Visibility:      input.Visibility,
		IsLocked:        input.IsLocked,
		IsPinned:        input.IsPinned,
	}

	// Compute content hash for duplicate detection (Rule 12).
	// Skip hashing for empty bodies — NULL content_hash doesn't
	// collide (unique index has WHERE content_hash IS NOT NULL).
	if strings.TrimSpace(body) != "" {
		hash := ComputeContentHash(body)
		updateInput.ContentHash = &hash
	}

	if input.GeoSpaceID != nil && *input.GeoSpaceID != "" {
		if err := updateInput.GeoSpaceID.Scan(*input.GeoSpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo_space_id format")
			return
		}
	}

	if input.OrganizationID != nil && *input.OrganizationID != "" {
		if err := updateInput.OrganizationID.Scan(*input.OrganizationID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id format")
			return
		}
	}

	if input.HeaderImageID != nil && *input.HeaderImageID != "" {
		if err := updateInput.HeaderImageID.Scan(*input.HeaderImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_header_image_id", "Invalid header_image_id format")
			return
		}
	}

	if input.FeaturedImageID != nil && *input.FeaturedImageID != "" {
		if err := updateInput.FeaturedImageID.Scan(*input.FeaturedImageID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_featured_image_id", "Invalid featured_image_id format")
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
		if errors.Is(err, ErrOrgPostForbidden) {
			writeError(w, http.StatusForbidden, "org_post_forbidden", "You are not authorized to edit as this organization")
			return
		}
		handleServiceError(w, err)
		return
	}

	// Enqueue async embedding generation — fire-and-forget.
	h.enqueueContentEmbedding(id, tenantID)

	// Enqueue search index sync — fire-and-forget.
	h.enqueueSearchIndex(id, tenantID, "index")

	// Enqueue moderation scan — fire-and-forget.
	h.enqueueModerationScan("content", id, tenantID)

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

	// Enqueue search index deletion — fire-and-forget.
	h.enqueueSearchIndex(id, tenantID, "delete")

	writeJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Lifecycle Handlers (publish / archive / schedule / view / share-count)
// ---------------------------------------------------------------------------

// PublishContent handles POST /api/v1/content/{id}/publish.
func (h *Handlers) PublishContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_ = user // used for RBAC audit trail

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	updated, err := h.Service.UpdateContentStatus(r.Context(), id, tenantID, StatusPublished)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// AUDIT-BE-TASK-001: enqueue the affiliate link scan task on publish
	// (Section 55 — fire-and-forget, low tier). The worker handler reads
	// the affiliate.scan_on_publish system setting; if false, it exits early.
	h.enqueueAffiliateLinkScan(id, tenantID)

	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// ArchiveContent handles POST /api/v1/content/{id}/archive.
func (h *Handlers) ArchiveContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_ = user // used for RBAC audit trail

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	updated, err := h.Service.UpdateContentStatus(r.Context(), id, tenantID, StatusArchived)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// ScheduleContent handles POST /api/v1/content/{id}/schedule. Sets
// status='scheduled' and publish_at, then enqueues the scheduled_publish
// Asynq task (matching internal/worker's registered handler) to fire at
// publish_at.
func (h *Handlers) ScheduleContent(w http.ResponseWriter, r *http.Request) {
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
		PublishAt time.Time `json:"publish_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PublishAt.IsZero() || input.PublishAt.Before(time.Now()) {
		writeError(w, http.StatusBadRequest, "publish_at_in_past", "publish_at must be in the future")
		return
	}

	updated, err := h.Service.ScheduleContent(r.Context(), id, tenantID, input.PublishAt)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.enqueueScheduledPublish(id, tenantID, input.PublishAt)

	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// RecordContentView handles POST /api/v1/content/{id}/view.
func (h *Handlers) RecordContentView(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.IncrementContentView(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async engagement count update — fire-and-forget.
	h.enqueueEngagementUpdate("content", id, "view", 1)

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"recorded": true}})
}

// GetContentShareCount handles GET /api/v1/content/{id}/share-count.
func (h *Handlers) GetContentShareCount(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	count, err := h.Service.CountContentShares(r.Context(), tenantID, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]int32{"share_count": count}})
}

// enqueueScheduledPublish enqueues the scheduled_publish Asynq task to fire
// at publishAt. Fire-and-forget: logs and returns on failure rather than
// failing the request — the content is already saved as 'scheduled'.
func (h *Handlers) enqueueScheduledPublish(contentID, tenantID pgtype.UUID, publishAt time.Time) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
		"tenant_id":  tenantID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal scheduled_publish task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeScheduledPublish, payload),
		asynq.ProcessAt(publishAt),
		asynq.Queue(worker.QueueForTask(worker.TypeScheduledPublish)),
		asynq.Unique(24*time.Hour),
	); err != nil {
		slog.Warn("content: failed to enqueue scheduled_publish task", "content_id", contentID.String(), "error", err)
	}
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

	pin, err := h.Service.PinContent(r.Context(), tenantID, spaceID, contentID, user.UserID, input.Position)
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

	q := r.URL.Query().Get("q")
	templates, err := h.Service.ListJSONLDTemplates(r.Context(), tenantID, pgtype.Text{String: q, Valid: true})
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
		Folder   string `json:"folder"`
		FolderID string `json:"folder_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var folderID pgtype.UUID
	if input.FolderID != "" {
		if err := folderID.Scan(input.FolderID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_folder_id", "Invalid folder_id format")
			return
		}
	}

	bm, err := h.Service.CreateBookmark(r.Context(), tenantID, user.UserID, EntityContent, contentID, input.Folder, folderID)
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

	maxTags := defaultMaxTags
	if h.settings != nil {
		maxTags = h.settings.GetIntOr(r.Context(), tenantID, "content.max_tags", defaultMaxTags)
	}

	existing, err := h.Service.ListContentTags(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if len(existing) >= maxTags {
		writeError(w, http.StatusBadRequest, "too_many_tags", "Content has reached the maximum number of tags")
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
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_ = user // used for RBAC audit trail

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

	// Enqueue async engagement count update — fire-and-forget.
	h.enqueueEngagementUpdate("content", contentID, "share", 1)

	writeJSON(w, http.StatusCreated, map[string]any{"data": share})
}

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

// ListContentTypes handles GET /content-types — returns available content types.
func (h *Handlers) ListContentTypes(w http.ResponseWriter, r *http.Request) {
	types := []map[string]string{
		{ "value": "discussion", "label": "Discussion" },
		{ "value": "blog_post", "label": "Blog Post" },
		{ "value": "page", "label": "Page" },
		{ "value": "comparison", "label": "Comparison" },
		{ "value": "question", "label": "Question" },
		{ "value": "article", "label": "Article" },
		{ "value": "guide", "label": "Guide" },
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}
