// Package blog — HTTP handlers for the Blog Extension system.
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// deleteEntity is a shared helper for DELETE handlers that follow the pattern:
// require user → require tenant → parse UUID → call delete function → return 204.
func deleteEntity(w http.ResponseWriter, r *http.Request, h *Handlers, paramName, entityName string, deleteFn func(ctx context.Context, tenantID, id pgtype.UUID) error) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, paramName)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+entityName+" ID")
		return
	}

	if err := deleteFn(r.Context(), tenantID, id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

const defaultPageSize int32 = 20

// contentTypeBlogPost is the content_type used for blog posts — blog posts
// are regular content rows (Section 24 is an extension over Section 14's
// content system), not a separate table.
const contentTypeBlogPost = "blog_post"

// statusInReview is the content status used while a post awaits editorial review.
const statusInReview = "in_review"

// ContentService is the subset of content.Service the blog post handlers need.
type ContentService interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, input content.CreateContentInput) (gen.CreateContentRow, error)
	GetContent(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetContentByIDRow, error)
	GetContentBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetContentBySlugRow, error)
	ListContentsByType(ctx context.Context, tenantID pgtype.UUID, contentType string, cursor pgtype.Timestamptz, limit int32, filter visibility.VisibilityFilter) ([]gen.ListContentsByTypeRow, error)
	UpdateContent(ctx context.Context, id, tenantID, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error)
	UpdateContentStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) (gen.UpdateContentStatusRow, error)
	DeleteContent(ctx context.Context, id, tenantID pgtype.UUID) error
	// ResolveJSONLD resolves the admin-managed JSON-LD template for a content
	// type (Section 14 / Section 26). Returns nil when no active template
	// exists so the caller can fall back to a hardcoded generator.
	ResolveJSONLD(ctx context.Context, tenantID pgtype.UUID, contentType string, fields map[string]string) (json.RawMessage, error)
}

// MentionNotifier dispatches mention notifications via the batch system
// (Section 35 — mention.content consolidated into notification:dispatch_batch
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

// Handlers holds dependencies for blog HTTP handlers.
type Handlers struct {
	Service         *Service
	content         ContentService
	siteURL         string
	siteName        string
	AsynqClient     TaskEnqueueClient       // C6: enqueue rss:regenerate on publish
	Redis           CacheReader             // C6: serve live feed from Valkey cache
	notifier        MentionNotifier         // nil disables mention notifications (test-only)
	mentionCheckers *mentions.DBCheckers    // nil disables erased-mention redaction
	spamRecorder    ContentCreationRecorder // nil disables spam rate-limit recording (test-only)
}

// TaskEnqueueClient is the subset of asynq.Client used by blog handlers.
type TaskEnqueueClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// ContentCreationRecorder is the subset of *spam.SpamPipeline used by blog
// handlers to record content creation events for the spam rate-limit layer
// (§41 Layer "rate_limit"). Narrow interface so the blog package stays
// testable without importing spam.
type ContentCreationRecorder interface {
	RecordContentCreation(ctx context.Context, tenantID, userID pgtype.UUID)
}

// CacheReader is the subset of valkeycompat.Cmdable used by blog handlers for
// reading the cached RSS feed.
type CacheReader interface {
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service, contentService ContentService, siteURL string, siteName string) *Handlers {
	return &Handlers{Service: service, content: contentService, siteURL: siteURL, siteName: siteName}
}

// SetAsynqClient injects the Asynq client for enqueuing background tasks
// (C6: rss:regenerate on publish).
func (h *Handlers) SetAsynqClient(c TaskEnqueueClient) {
	h.AsynqClient = c
}

// SetRedis injects the Valkey/Redis client for reading cached feeds (C6).
func (h *Handlers) SetRedis(r CacheReader) {
	h.Redis = r
}

// SetSpamRecorder injects the spam pipeline recorder so blog post creation
// is counted toward the spam rate-limit layer (§41, BUG-004). Nil disables
// recording (test-only).
func (h *Handlers) SetSpamRecorder(r ContentCreationRecorder) {
	h.spamRecorder = r
}

// SetMentionCheckers wires the DB-backed erased-user checker used to
// redact mentions of deleted users in blog post responses (Section 15 /
// ADR-029). nil disables redaction (backward compat / tests).
func (h *Handlers) SetMentionCheckers(c *mentions.DBCheckers) { h.mentionCheckers = c }

// SetMentionNotifier wires the notification dispatcher for @mention
// notifications in blog posts (Section 15 / ADR-029). nil disables
// mention notifications (backward compat / tests).
func (h *Handlers) SetMentionNotifier(n MentionNotifier) { h.notifier = n }

// dispatchMentionNotifications extracts @mentioned users from a blog post
// body (TipTap JSON) and fires a mention.content notification to each via
// the batch system (Section 35 — BL-002). Fire-and-forget per Rule 8:
// errors are logged but never propagated to the caller.
//
// Self-mentions (mentioning the author) are skipped.
// Privacy filtering (blocks, erased users, private-space membership) is
// applied via mentions.FilterMentions when checkers are wired (Section 15).
func (h *Handlers) dispatchMentionNotifications(
	ctx context.Context,
	tenantID, authorID, contentID, spaceID pgtype.UUID,
	body string,
) {
	if h.notifier == nil {
		return
	}

	mentioned, err := mentions.ExtractMentions([]byte(body))
	if err != nil {
		// Body may not be valid TipTap JSON (e.g., plain text); skip mentions.
		return
	}

	if h.mentionCheckers != nil {
		isPrivateSpace, err := h.mentionCheckers.IsPrivateSpace(ctx, tenantID, spaceID)
		if err != nil {
			slog.Warn("blog: IsPrivateSpace check failed",
				"space_id", spaceID.String(), "error", err)
			isPrivateSpace = false
		}

		mentioned, err = mentions.FilterMentions(
			ctx, mentioned, tenantID, authorID, spaceID, isPrivateSpace,
			h.mentionCheckers, h.mentionCheckers, h.mentionCheckers,
		)
		if err != nil {
			slog.Warn("blog: FilterMentions returned unexpected error",
				"content_id", contentID.String(), "error", err)
		}
	}

	for _, m := range mentioned {
		if m.EntityType != "user" || !m.ID.Valid || m.ID == authorID {
			continue
		}
		if err := h.notifier.DispatchNotification(
			ctx, tenantID, m.ID,
			"mention.content", "",
			authorID,
			"content", contentID,
			"", "", "",
		); err != nil {
			slog.Warn("blog: failed to dispatch mention.content notification",
				"user_id", m.ID.String(), "error", err)
		}
	}
}

// verifyBlogPostContent fetches the content row for the given ID and
// verifies it is a blog_post (content_type). Returns an error if the
// content is missing or is not a blog post. Used by ApproveReview,
// RejectReview, and FeaturePost to prevent the blog-specific endpoints
// from acting on non-blog content that shares the content_reviews or
// content_featured tables (BUG-002/BUG-004).
func (h *Handlers) verifyBlogPostContent(ctx context.Context, contentID, tenantID pgtype.UUID) error {
	existing, err := h.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		return err
	}
	if existing.ContentType != contentTypeBlogPost {
		return errWrongContentType
	}
	return nil
}

// errWrongContentType is returned when a blog-specific endpoint receives
// a content_id that does not belong to a blog_post.
var errWrongContentType = errors.New("not a blog post")

// enqueueRSSRegenerate enqueues the rss:regenerate task for a tenant (C6).
// Fire-and-forget: errors are logged but never propagated (Rule 8).
func (h *Handlers) enqueueRSSRegenerate(tenantID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
	})
	if err != nil {
		slog.Warn("blog: failed to marshal rss:regenerate payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeRSSRegenerate, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeRSSRegenerate)),
	); err != nil {
		slog.Warn("blog: failed to enqueue rss:regenerate task",
			"tenant_id", tenantID.String(), "error", err)
	}
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

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// mediaURL builds a signed imgproxy URL (or local serve fallback) for a media
// item identified by its storage bucket and path. Used by the enriched blog
// post DTOs to surface featured_image_url without storing it in the DB.
func mediaURL(storageBucket, storagePath string) string {
	return media.MediaURL(storageBucket, storagePath, media.ImageProxyOpts{})
}

// excerptMaxLen is the maximum length of an auto-generated excerpt.
const excerptMaxLen = 160

// stringValue coerces a sqlc-scalar (interface{}) value to a string. This is
// needed for columns that come from scalar subqueries (excerpt, tags_csv) and
// are typed as interface{} by sqlc.
func stringValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// excerptFromBody returns a plain-text excerpt from the post body, used as
// a fallback when no explicit excerpt is stored. It handles three body
// formats:
//   - Tiptap/ProseMirror JSON (starts with `{`) — recursively extracts text
//     from "text" nodes.
//   - HTML (starts with `<`) — strips tags via a simple scanner.
//   - Plain text — used as-is.
//
// The result is trimmed and truncated to excerptMaxLen characters with an
// ellipsis. Empty/whitespace-only input returns "".
func excerptFromBody(body string) string {
	plain := plainTextFromContent(body)
	plain = strings.TrimSpace(plain)
	if len(plain) <= excerptMaxLen {
		return plain
	}
	return plain[:excerptMaxLen] + "…"
}

// plainTextFromContent extracts readable text from a content body that may
// be Tiptap JSON, HTML, or plain text.
func plainTextFromContent(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}

	// Tiptap/ProseMirror JSON document.
	if trimmed[0] == '{' {
		if text, ok := extractTiptapText([]byte(trimmed)); ok {
			return text
		}
		// Not valid Tiptap JSON — fall through to plain text.
	}

	// HTML — strip tags.
	if trimmed[0] == '<' {
		return stripHTMLTags(trimmed)
	}

	// Plain text.
	return trimmed
}

// extractTiptapText recursively walks a Tiptap/ProseMirror JSON document and
// concatenates the "text" field of all text nodes.
func extractTiptapText(data []byte) (string, bool) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", false
	}
	if t, ok := doc["type"].(string); !ok || t != "doc" {
		return "", false
	}
	var sb strings.Builder
	walkTiptapNodes(doc, &sb)
	return sb.String(), true
}

// walkTiptapNodes recursively traverses a Tiptap node, appending text from
// "text" leaf nodes.
func walkTiptapNodes(node any, sb *strings.Builder) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	if text, ok := m["text"].(string); ok {
		sb.WriteString(text)
		sb.WriteByte(' ')
	}
	if content, ok := m["content"].([]any); ok {
		for _, child := range content {
			walkTiptapNodes(child, sb)
		}
	}
}

// stripHTMLTags removes HTML tags from a string, preserving the text content.
func stripHTMLTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Series Handlers
// ---------------------------------------------------------------------------

// CreateSeries handles POST /api/v1/blog/series.
func (h *Handlers) CreateSeries(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Title        string  `json:"title"`
		Slug         string  `json:"slug"`
		Description  string  `json:"description"`
		CoverImageID *string `json:"cover_image_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	var coverImageID pgtype.UUID
	if input.CoverImageID != nil {
		if err := coverImageID.Scan(*input.CoverImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cover_image_id", "Invalid cover_image_id format")
			return
		}
	}

	series, err := h.Service.CreateSeries(r.Context(), tenantID, user.UserID, CreateSeriesInput{
		Title:        input.Title,
		Slug:         input.Slug,
		Description:  input.Description,
		CoverImageID: coverImageID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": series})
}

// ListSeries handles GET /api/v1/blog/series.
// Supports ?author_id=, ?q=, ?page_size=, ?page_offset=.
// L2: the q filter is now applied in SQL (ListBlogSeriesByTenant) rather than
// in Go memory after fetching all rows.
// JEOS §24: returns the public Series DTO with cover_image_url and pagination
// metadata (total, page_size, page_offset).
func (h *Handlers) ListSeries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	authorIDStr := r.URL.Query().Get("author_id")
	if authorIDStr != "" {
		var authorID pgtype.UUID
		if err := authorID.Scan(authorIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_author_id", "Invalid author_id")
			return
		}

		series, err := h.Service.ListSeriesByAuthor(r.Context(), tenantID, authorID, limit, offset)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		total, err := h.Service.CountSeriesByAuthor(r.Context(), tenantID, authorID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{
			"data": series,
			"meta": map[string]any{
				"total":       total,
				"page_size":   limit,
				"page_offset": offset,
			},
		})

		return
	}

	// L2: q filter is now applied in SQL via ListBlogSeriesByTenant.
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	series, err := h.Service.ListSeriesByTenant(r.Context(), tenantID, q, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	total, err := h.Service.CountSeriesByTenant(r.Context(), tenantID, q)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": series,
		"meta": map[string]any{
			"total":       total,
			"page_size":   limit,
			"page_offset": offset,
		},
	})
}

// GetSeries handles GET /api/v1/blog/series/{slug}.
// Returns the public Series DTO with a computed cover_image_url.
func (h *Handlers) GetSeries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	series, err := h.Service.GetSeriesBySlug(r.Context(), slug, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": series})
}

// ListSeriesPosts handles GET /api/v1/blog/series/{id}/posts — lists all posts in a series.
func (h *Handlers) ListSeriesPosts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid series ID")
		return
	}

	// Verify the series exists and belongs to this tenant.
	_, err = h.Service.GetSeries(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	posts, err := h.Service.ListPostsInSeries(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": posts})
}

// UpdateSeries handles PATCH /api/v1/blog/series/{id}.
func (h *Handlers) UpdateSeries(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid series ID")
		return
	}

	var input struct {
		Title        string  `json:"title"`
		Slug         string  `json:"slug"`
		Description  string  `json:"description"`
		CoverImageID *string `json:"cover_image_id"`
		Status       string  `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	var coverImageID pgtype.UUID
	if input.CoverImageID != nil {
		if err := coverImageID.Scan(*input.CoverImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cover_image_id", "Invalid cover_image_id format")
			return
		}
	}

	series, err := h.Service.UpdateSeries(r.Context(), id, tenantID, input.Title, input.Slug, input.Description, coverImageID, input.Status)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": series})
}

// DeleteSeries handles DELETE /api/v1/blog/series/{id}.
func (h *Handlers) DeleteSeries(w http.ResponseWriter, r *http.Request) {
	deleteEntity(w, r, h, "id", "series", func(ctx context.Context, tenantID, id pgtype.UUID) error {
		return h.Service.DeleteSeries(ctx, id, tenantID)
	})
}

// ---------------------------------------------------------------------------
// Series Posts Handlers
// ---------------------------------------------------------------------------

// AddPostToSeries handles POST /api/v1/blog/series/{id}/posts.
// H7: verifies the content belongs to the same tenant before linking.
func (h *Handlers) AddPostToSeries(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	seriesID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_series_id", "Invalid series ID")
		return
	}

	var input struct {
		ContentID string `json:"content_id"`
		Position  int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content_id", "content_id is required")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	if err := h.Service.AddPostToSeries(r.Context(), seriesID, contentID, tenantID, input.Position); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "added"}})
}

// RemovePostFromSeries handles DELETE /api/v1/blog/series/{id}/posts/{postId}.
// BUG-003: now requires tenant context and the service verifies the series
// belongs to the caller's tenant before deleting, preventing cross-tenant
// post removal by an attacker who knows the series_id.
func (h *Handlers) RemovePostFromSeries(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	seriesID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_series_id", "Invalid series ID")
		return
	}

	contentID, err := parseUUIDParam(r, "postId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid post ID")
		return
	}

	if err := h.Service.RemovePostFromSeries(r.Context(), seriesID, contentID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// GetContentSeries handles GET /api/v1/content/{id}/series.
// Returns the series_id for a given content item, or null if the content is
// not part of any series. Used by the editor load to restore the series
// selector when editing existing content.
// Note: the route param is named "id" but chi requires it to match the
// existing /content/{slug} route's param name at the same path level.
// The route is registered as /content/{id}/series which works because chi
// allows different param names for sub-paths (the {slug} route only matches
// exactly one segment, while {id}/series matches two).
func (h *Handlers) GetContentSeries(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content ID")
		return
	}

	// BUG-009: pass tenantID so the query is tenant-scoped, preventing
	// cross-tenant information disclosure via series membership probing.
	seriesID, err := h.Service.GetSeriesForContent(r.Context(), contentID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"series_id": nil}})
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"series_id": seriesID.String()}})
}

// ReorderSeriesPosts handles PATCH /api/v1/blog/series/{id}/posts/reorder.
// BUG-004: now requires tenant context and the service verifies the series
// belongs to the caller's tenant before mutating, preventing cross-tenant
// post reorder by an attacker who knows the series_id.
func (h *Handlers) ReorderSeriesPosts(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	seriesID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_series_id", "Invalid series ID")
		return
	}

	var input struct {
		Posts []struct {
			ContentID string `json:"content_id"`
			Position  int32  `json:"position"`
		} `json:"posts"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Posts) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_posts", "At least one post required")
		return
	}

	posts := make([]struct {
		ContentID pgtype.UUID
		Position  int32
	}, len(input.Posts))
	for i, p := range input.Posts {
		if err := posts[i].ContentID.Scan(p.ContentID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id in posts array")
			return
		}

		posts[i].Position = p.Position
	}

	if err := h.Service.BulkReorderSeriesPosts(r.Context(), seriesID, tenantID, posts); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ---------------------------------------------------------------------------
// Review Handlers
// ---------------------------------------------------------------------------

// CreateReview handles POST /api/v1/blog/reviews.
func (h *Handlers) CreateReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentID string `json:"content_id"`
		Status    string `json:"status"`
		Feedback  string `json:"feedback"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content_id", "content_id is required")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	review, err := h.Service.CreateReview(r.Context(), tenantID, user.UserID, CreateReviewInput{
		ContentID: contentID,
		Status:    input.Status,
		Feedback:  input.Feedback,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": review})
}

// ListReviews handles GET /api/v1/blog/reviews and GET /api/v1/admin/blog/review-queue.
// Supports ?content_id=, ?pending=true, ?page_size=, ?page_offset=.
// C3: the admin review-queue path returns joined summaries (title/slug/author/excerpt).
// L1: the default branch now returns pending reviews (same as ?pending=true);
// the dead ?pending=false branch is removed.
func (h *Handlers) ListReviews(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	contentIDStr := r.URL.Query().Get("content_id")
	if contentIDStr != "" {
		var contentID pgtype.UUID
		if err := contentID.Scan(contentIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
			return
		}

		reviews, err := h.Service.ListReviewsByContent(r.Context(), contentID, tenantID)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": reviews})

		return
	}

	// C3: return enriched review summaries (with post title/slug/author/excerpt)
	// for the admin review-queue. This is the default and ?pending=true path.
	summaries, err := h.Service.ListPendingReviewSummaries(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": summaries})
}

// UpdateReview handles PATCH /api/v1/blog/reviews/{id}.
func (h *Handlers) UpdateReview(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	var input struct {
		Status   string `json:"status"`
		Feedback string `json:"feedback"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "Status is required")
		return
	}

	review, err := h.Service.UpdateReviewStatus(r.Context(), id, tenantID, user.UserID, input.Status, input.Feedback)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": review})
}

// DeleteReview handles DELETE /api/v1/blog/reviews/{id}.
func (h *Handlers) DeleteReview(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	if err := h.Service.DeleteReview(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}

// ApproveReview handles POST /api/v1/admin/blog/review-queue/{id}/approve (C2).
// Dedicated handler that sets status='approved' from the route, reading only
// feedback from the body — does not require a status field.
func (h *Handlers) ApproveReview(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	var input struct {
		Feedback string `json:"feedback"`
	}

	// Body is optional — approve can be called with empty body.
	_ = json.NewDecoder(r.Body).Decode(&input)

	// BUG-002: verify the review belongs to a blog_post before acting.
	// content_reviews is shared by ads, persona actions, and studio; the
	// blog review-queue must not publish non-blog content.
	existingReview, err := h.Service.GetReview(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if vErr := h.verifyBlogPostContent(r.Context(), existingReview.ContentID, tenantID); vErr != nil {
		if errors.Is(vErr, errWrongContentType) {
			errpkg.WriteError(w, http.StatusBadRequest, "wrong_content_type", "Not a blog post")
			return
		}
		handleServiceError(w, vErr)
		return
	}

	review, err := h.Service.UpdateReviewStatus(r.Context(), id, tenantID, user.UserID, ReviewStatusApproved, input.Feedback)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Flip the content status to published on approval.
	if _, err := h.content.UpdateContentStatus(r.Context(), review.ContentID, tenantID, content.StatusPublished); err != nil {
		slog.Error("failed to publish content on approve", "error", err, "content_id", review.ContentID)
	}

	// C6: enqueue rss:regenerate so the feed reflects the newly published post.
	h.enqueueRSSRegenerate(tenantID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": review})
}

// RejectReview handles POST /api/v1/admin/blog/review-queue/{id}/reject (C2).
// Dedicated handler that sets status='rejected' from the route, reading only
// feedback from the body — does not require a status field.
func (h *Handlers) RejectReview(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid review ID")
		return
	}

	var input struct {
		Feedback string `json:"feedback"`
		Reason   string `json:"reason"` // frontend sends 'reason' — map to feedback
	}

	_ = json.NewDecoder(r.Body).Decode(&input)

	feedback := input.Feedback
	if feedback == "" {
		feedback = input.Reason
	}

	// BUG-002: verify the review belongs to a blog_post before acting.
	existingReview, err := h.Service.GetReview(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if vErr := h.verifyBlogPostContent(r.Context(), existingReview.ContentID, tenantID); vErr != nil {
		if errors.Is(vErr, errWrongContentType) {
			errpkg.WriteError(w, http.StatusBadRequest, "wrong_content_type", "Not a blog post")
			return
		}
		handleServiceError(w, vErr)
		return
	}

	review, err := h.Service.UpdateReviewStatus(r.Context(), id, tenantID, user.UserID, ReviewStatusRejected, feedback)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Flip the content status back to draft on rejection.
	if _, err := h.content.UpdateContentStatus(r.Context(), review.ContentID, tenantID, "draft"); err != nil {
		slog.Error("failed to revert content to draft on reject", "error", err, "content_id", review.ContentID)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": review})
}

// ---------------------------------------------------------------------------
// Featured Handlers
// ---------------------------------------------------------------------------

// FeaturePost handles POST /api/v1/blog/featured.
func (h *Handlers) FeaturePost(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentID     string `json:"content_id"`
		FeaturedOrder int32  `json:"featured_order"`
		FeaturedUntil string `json:"featured_until"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content_id", "content_id is required")
		return
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(input.ContentID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id")
		return
	}

	// BUG-004: verify the content is a blog_post before featuring it.
	// content_featured is a generic junction; without this check an admin
	// with blog.feature could feature forum threads, pages, or any other
	// content type, which would then appear in the blog featured tab.
	if vErr := h.verifyBlogPostContent(r.Context(), contentID, tenantID); vErr != nil {
		if errors.Is(vErr, errWrongContentType) {
			errpkg.WriteError(w, http.StatusBadRequest, "wrong_content_type", "Not a blog post")
			return
		}
		handleServiceError(w, vErr)
		return
	}

	var featuredUntil pgtype.Timestamptz
	if input.FeaturedUntil != "" {
		if err := featuredUntil.Scan(input.FeaturedUntil); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_featured_until", "Invalid featured_until date format")
			return
		}
	}

	featured, err := h.Service.FeaturePost(r.Context(), tenantID, FeaturePostInput{
		ContentID:     contentID,
		FeaturedOrder: input.FeaturedOrder,
		FeaturedUntil: featuredUntil,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": featured})
}

// UnfeaturePost handles DELETE /api/v1/blog/featured/{contentId}.
func (h *Handlers) UnfeaturePost(w http.ResponseWriter, r *http.Request) {
	deleteEntity(w, r, h, "contentId", "content", func(ctx context.Context, tenantID, id pgtype.UUID) error {
		return h.Service.UnfeaturePost(ctx, tenantID, id)
	})
}

// ListFeatured handles GET /api/v1/blog/featured.
// H6: returns enriched blog post cards (with title/slug/author/cover image)
// instead of raw content_featured junction rows.
func (h *Handlers) ListFeatured(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	cards, err := h.Service.ListFeaturedBlogPostCards(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	total, err := h.Service.CountFeaturedBlogPostCards(r.Context(), tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Construct featured_image_url from storage_bucket/storage_path.
	result := make([]map[string]any, 0, len(cards))
	for _, c := range cards {
		card := map[string]any{
			"featured_id":       c.FeaturedID,
			"content_id":        c.ContentID,
			"featured_order":    c.FeaturedOrder,
			"featured_until":    c.FeaturedUntil,
			"title":             c.Title,
			"slug":              c.Slug,
			"body":              c.Body,
			"body_html":         c.BodyHtml,
			"excerpt":           excerptFromBody(c.Body),
			"read_time_minutes": c.ReadTimeMinutes,
			"view_count":        c.ViewCount,
			"comment_count":     c.CommentCount,
			"reshare_count":     c.ReshareCount,
			"author_id":         c.AuthorID,
			"author_name":       c.AuthorName,
			"author_avatar_url": c.AuthorAvatarUrl,
			"space_name":        c.SpaceName,
			"space_slug":        c.SpaceSlug,
			"id":                c.ContentID, // for card compatibility
		}
		if c.FeaturedStorageBucket != nil && c.FeaturedStoragePath != nil {
			card["featured_image_url"] = mediaURL(*c.FeaturedStorageBucket, *c.FeaturedStoragePath)
		}
		if tagsCsv := stringValue(c.TagsCsv); tagsCsv != "" {
			card["tags"] = strings.Split(tagsCsv, ",")
		} else {
			card["tags"] = []string{}
		}
		result = append(result, card)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": result,
		"meta": map[string]any{
			"total":       total,
			"page_size":   limit,
			"page_offset": offset,
		},
	})
}

// ReorderFeatured handles PATCH /api/v1/blog/featured/reorder.
// M3: uses a transactional bulk update so partial failure leaves consistent order.
func (h *Handlers) ReorderFeatured(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Items []struct {
			ID    string `json:"id"`
			Order int32  `json:"order"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Items) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_items", "At least one item required")
		return
	}

	items := make([]struct {
		ID    pgtype.UUID
		Order int32
	}, len(input.Items))
	for i, item := range input.Items {
		if err := items[i].ID.Scan(item.ID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid featured ID in items array")
			return
		}

		items[i].Order = item.Order
	}

	if err := h.Service.BulkUpdateFeaturedOrder(r.Context(), tenantID, items); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ---------------------------------------------------------------------------
// RSS Feed Handler
// ---------------------------------------------------------------------------

// GetRSSFeed handles GET /api/v1/blog/feed.xml (public, no auth required).
// C6: serves the live feed from the Valkey cache (jc:cache:feed:rss:{tenant_id})
// populated by the rss:regenerate worker task. Falls back to live DB generation
// when the cache is empty or Redis is unavailable.
// BUG-006: respects X-Forwarded-Host to generate public-facing URLs instead
// of the internal backend port. Forwarding headers are gated by the
// trusted-proxy model (clientip.ForwardedBaseURL) to prevent host-header
// injection.
func (h *Handlers) GetRSSFeed(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	// Resolve the public-facing site URL. X-Forwarded-Host/Proto are
	// honoured only when the immediate peer is a trusted proxy
	// (JC_TRUSTED_PROXIES) — otherwise an attacker could spoof
	// X-Forwarded-Host: evil.com and inject their domain into RSS feed
	// links (host-header injection). Falls back to the configured
	// JC_SITE_URL (h.siteURL) when the peer is untrusted or no forwarding
	// headers are present.
	siteURL := clientip.ForwardedBaseURL(r, h.siteURL)

	// RSS-001: use a blog-specific cache key. The universal feed cache
	// (jc:cache:feed:rss:{tenant_id}) is populated by the worker from
	// ListRSSFeedItems (all content types). Reading from it here would
	// serve non-blog content (forum posts, etc.) as the blog feed.
	cacheKey := valkey.MustKey("cache:feed:rss:blog:%s", tenantID.String())
	if h.Redis != nil {
		if cached, err := h.Redis.Get(r.Context(), cacheKey).Result(); err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
			// RSS-004: §26 requires Cache-Control: max-age=900 on all feeds.
			w.Header().Set("Cache-Control", "public, max-age=900")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(cached))
			return
		}
	}

	// Cache miss — generate live from the DB.
	posts, err := h.Service.ListPublishedPostsForRSS(r.Context(), tenantID, 50)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	// RSS-004: §26 requires Cache-Control: max-age=900 on all feeds.
	w.Header().Set("Cache-Control", "public, max-age=900")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)

	if err := generateRSSXML(w, posts, siteURL, h.siteName); err != nil {
		// Headers already sent, just log the error.
		return
	}
}

// GetPost handles GET /api/v1/blog/{slug} — a single blog post.
// Public (optional auth via the API group middleware); published posts are
// visible to everyone. Authors can also view their own drafts so the
// submit-for-review and editor flows are reachable from the public URL.
// Super-admins/moderators use /admin/blog/{id} for moderation.
// C4: returns an enriched DTO with author name, avatar, featured image URL,
// tags, and series navigation (prev/next within a series).
func (h *Handlers) GetPost(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	// Optional auth lets the owner of a draft load it on the public post page.
	var viewerUserID pgtype.UUID
	if user := auth.FromContext(r.Context()); user != nil {
		viewerUserID = user.UserID
	}

	post, err := h.Service.GetBlogPostDetail(r.Context(), slug, tenantID, viewerUserID, false)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Post not found")
			return
		}
		slog.Error("blog: failed to get post detail", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load post")
		return
	}

	// Build the enriched response DTO.
	dto := map[string]any{
		"id":                post.ID,
		"tenant_id":         post.TenantID,
		"author_id":         post.AuthorID,
		"space_id":          post.SpaceID,
		"content_type":      post.ContentType,
		"title":             post.Title,
		"slug":              post.Slug,
		"body":              post.Body,
		"body_html":         post.BodyHtml,
		"field_values":      post.FieldValues,
		"content_hash":      post.ContentHash,
		"geo_space_id":      post.GeoSpaceID,
		"header_image_id":   post.HeaderImageID,
		"featured_image_id": post.FeaturedImageID,
		"canonical_url":     post.CanonicalUrl,
		"meta_title":        post.MetaTitle,
		"meta_description":  post.MetaDescription,
		"og_image_url":      post.OgImageUrl,
		"seo_indexable":     post.SeoIndexable,
		"status":            post.Status,
		"visibility":        post.Visibility,
		"is_locked":         post.IsLocked,
		"is_pinned":         post.IsPinned,
		"read_time_minutes": post.ReadTimeMinutes,
		"view_count":        post.ViewCount,
		"comment_count":     post.CommentCount,
		"share_count":       post.ShareCount,
		"reshare_count":     post.ReshareCount,
		"publish_at":        post.PublishAt,
		"published_at":      post.PublishedAt,
		"created_at":        post.CreatedAt,
		"updated_at":        post.UpdatedAt,
		"author_name":       post.AuthorName,
		"author_avatar_url": post.AuthorAvatarUrl,
		"space_name":        post.SpaceName,
		"space_slug":        post.SpaceSlug,
		"excerpt":           excerptFromBody(post.Body),
	}

	// Construct featured_image_url from storage_bucket/storage_path.
	if post.FeaturedStorageBucket != nil && post.FeaturedStoragePath != nil {
		dto["featured_image_url"] = mediaURL(*post.FeaturedStorageBucket, *post.FeaturedStoragePath)
	}

	// Tags — split the CSV string into an array.
	if post.TagsCsv != "" {
		dto["tags"] = strings.Split(post.TagsCsv, ",")
	} else {
		dto["tags"] = []string{}
	}

	// Series navigation.
	if post.SeriesID.Valid {
		dto["series_id"] = post.SeriesID
		if post.SeriesTitle != nil {
			dto["series_title"] = *post.SeriesTitle
		}
		dto["series_slug"] = post.SeriesSlug
		if post.SeriesPart != nil {
			dto["series_part"] = *post.SeriesPart
		}
		dto["series_total"] = post.SeriesTotal
		if post.SeriesPrevSlug != "" {
			dto["series_prev_slug"] = post.SeriesPrevSlug
		}
		if post.SeriesNextSlug != "" {
			dto["series_next_slug"] = post.SeriesNextSlug
		}
	}

	// JSON-LD structured data (Section 14 / Section 26). Resolve the
	// admin-managed Article template, interpolating the post's field values
	// into the {{field_key}} placeholders. Best-effort: a missing template or
	// resolution error degrades to no json_ld field (the frontend falls back
	// to its client-side generator). Emitted as a sibling of `data` so the
	// frontend load function can read envelope.json_ld directly.
	resp := map[string]any{"data": dto}
	if jsonLd, jErr := h.content.ResolveJSONLD(r.Context(), tenantID, "Article", blogPostJSONLDFields(post, dto)); jErr == nil && len(jsonLd) > 0 {
		resp["json_ld"] = json.RawMessage(jsonLd)
	} else if jErr != nil {
		slog.Warn("blog: JSON-LD template resolution failed", "slug", slug, "error", jErr)
	}

	// GDPR / ADR-029: redact mentions of erased users in body_html and
	// expose deleted_user_ids for frontend rendering (Section 15).
	// Best-effort: a failure logs a warning but does not fail the request.
	if h.mentionCheckers != nil {
		redactedHTML, erasedIDs, redErr := mentions.RedactErasedMentionsInBody(
			r.Context(),
			[]byte(post.Body),
			post.BodyHtml,
			tenantID,
			h.mentionCheckers,
		)
		if redErr != nil {
			slog.Warn("blog: RedactErasedMentionsInBody failed",
				"post_id", post.ID.String(), "error", redErr)
		} else {
			dto["body_html"] = redactedHTML
			if len(erasedIDs) > 0 {
				resp["deleted_user_ids"] = erasedIDs
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// blogPostJSONLDFields builds the {{field_key}} → value map for interpolating
// an Article JSON-LD template from a blog post detail row and the enriched DTO
// (which carries the computed featured_image_url and excerpt).
func blogPostJSONLDFields(post gen.GetBlogPostDetailRow, dto map[string]any) map[string]string {
	desc := ""
	if post.MetaDescription != nil && *post.MetaDescription != "" {
		desc = *post.MetaDescription
	} else if e, ok := dto["excerpt"].(string); ok {
		desc = e
	}
	coverURL, _ := dto["featured_image_url"].(string)
	canonical := ""
	if post.CanonicalUrl != nil {
		canonical = *post.CanonicalUrl
	}
	return map[string]string{
		"title":              post.Title,
		"description":        desc,
		"cover_image_url":    coverURL,
		"author_name":        post.AuthorName,
		"author_profile_url": "",
		"published_at":       formatTimestamptz(post.PublishedAt),
		"updated_at":         formatTimestamptz(post.UpdatedAt),
		"canonical_url":      canonical,
	}
}

// formatTimestamptz renders a pgtype.Timestamptz as RFC3339 or empty.
func formatTimestamptz(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// ListPosts handles GET /api/v1/blog — published blog posts, newest first.
// Supports ?page_size= and ?page_offset= (Rule 5 offset pagination).
// BUG-011: §24-mandated filters — ?space_id, ?tag (tag slug), ?series_id,
// ?sort=latest|top, ?author_id. sort=featured is handled by the frontend
// via the separate /blog/featured endpoint, so it is normalized to "latest"
// here to avoid a silent empty result.
// C5/H3: uses the enriched ListPublishedBlogPosts query that filters
// status='published' and deleted_at IS NULL in SQL (not Go memory), and
// JOINs users + media for author name and featured image URL.
// BUG-005: switched from cursor-based to offset-based pagination per Rule 5.
func (h *Handlers) ListPosts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	// BUG-011: build the §24 filter from query params. Unknown/invalid
	// UUIDs return 400; an unknown sort value falls back to "latest"
	// (the spec only defines latest|top|featured, and featured is routed
	// to /blog/featured by the frontend).
	filter := BlogListFilter{TenantID: tenantID}

	if v := r.URL.Query().Get("space_id"); v != "" {
		if err := filter.SpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}
	}
	if v := r.URL.Query().Get("author_id"); v != "" {
		if err := filter.AuthorID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_author_id", "Invalid author_id")
			return
		}
	}
	if v := r.URL.Query().Get("series_id"); v != "" {
		if err := filter.SeriesID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_series_id", "Invalid series_id")
			return
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("tag")); v != "" {
		filter.Tag = &v
	}
	switch sort := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort"))); sort {
	case "", "latest", "top":
		filter.Sort = sort
	case "featured":
		// Frontend routes sort=featured to /blog/featured; normalize to
		// latest here so a direct API call still returns results.
		filter.Sort = "latest"
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_sort", "sort must be one of: latest, top, featured")
		return
	}

	posts, err := h.Service.ListPublishedBlogPosts(r.Context(), filter, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	total, err := h.Service.CountPublishedBlogPosts(r.Context(), filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Build enriched card DTOs with featured_image_url.
	result := make([]map[string]any, 0, len(posts))
	for _, p := range posts {
		card := map[string]any{
			"id":                p.ID,
			"tenant_id":         p.TenantID,
			"author_id":         p.AuthorID,
			"space_id":          p.SpaceID,
			"content_type":      p.ContentType,
			"title":             p.Title,
			"slug":              p.Slug,
			"body":              p.Body,
			"body_html":         p.BodyHtml,
			"field_values":      p.FieldValues,
			"content_hash":      p.ContentHash,
			"geo_space_id":      p.GeoSpaceID,
			"header_image_id":   p.HeaderImageID,
			"featured_image_id": p.FeaturedImageID,
			"canonical_url":     p.CanonicalUrl,
			"meta_title":        p.MetaTitle,
			"meta_description":  p.MetaDescription,
			"og_image_url":      p.OgImageUrl,
			"seo_indexable":     p.SeoIndexable,
			"status":            p.Status,
			"visibility":        p.Visibility,
			"is_locked":         p.IsLocked,
			"is_pinned":         p.IsPinned,
			"read_time_minutes": p.ReadTimeMinutes,
			"view_count":        p.ViewCount,
			"comment_count":     p.CommentCount,
			"share_count":       p.ShareCount,
			"reshare_count":     p.ReshareCount,
			"publish_at":        p.PublishAt,
			"published_at":      p.PublishedAt,
			"created_at":        p.CreatedAt,
			"updated_at":        p.UpdatedAt,
			"author_name":       p.AuthorName,
			"author_avatar_url": p.AuthorAvatarUrl,
			"space_name":        p.SpaceName,
			"space_slug":        p.SpaceSlug,
			"excerpt":           excerptFromBody(p.Body),
		}
		if p.FeaturedStorageBucket != nil && p.FeaturedStoragePath != nil {
			card["featured_image_url"] = mediaURL(*p.FeaturedStorageBucket, *p.FeaturedStoragePath)
		}
		if tagsCsv := stringValue(p.TagsCsv); tagsCsv != "" {
			card["tags"] = strings.Split(tagsCsv, ",")
		} else {
			card["tags"] = []string{}
		}
		result = append(result, card)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": result,
		"meta": map[string]any{
			"total":       total,
			"page_size":   limit,
			"page_offset": offset,
		},
	})
}

// AdminListPosts handles GET /api/v1/admin/blog — all blog posts
// (any status: draft, scheduled, published, archived) for the admin
// panel. Reuses the content listing; unlike the public ListPosts it does
// NOT filter out non-published posts.
func (h *Handlers) AdminListPosts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	// BUG-B: use the enriched ListAdminBlogPosts query (joins users +
	// user_profiles) so the admin /blog table gets author_name. The
	// generic ListContentsByType returned no author join, showing "—"
	// for every author. Offset-paginated per Rule 5.
	posts, err := h.Service.ListAdminBlogPosts(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": posts})
}

// AdminDeletePost handles DELETE /api/v1/admin/blog/{id} — soft-deletes
// a blog post (Rule 11: user content is soft-deleted, never hard-deleted).
// H2: returns 204 No Content on success (REST DELETE convention).
func (h *Handlers) AdminDeletePost(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.content.DeleteContent(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// CreatePost handles POST /api/v1/blog — creates a blog post (content_type
// forced to blog_post regardless of any type field in the request body).
// M1: accepts all content fields (excerpt, meta, SEO, scheduling, space).
func (h *Handlers) CreatePost(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
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
		SpaceID         *string         `json:"space_id"`
		GeoSpaceID      *string         `json:"geo_space_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Status == "" {
		input.Status = "draft"
	}

	if input.Visibility == "" {
		input.Visibility = "public"
	}

	createInput := content.CreateContentInput{
		AuthorID:        user.UserID,
		ContentType:     contentTypeBlogPost,
		Title:           input.Title,
		Slug:            input.Slug,
		Body:            content.SanitizeText(input.Body),
		BodyHtml:        content.SanitizeHTML(input.BodyHtml),
		FieldValues:     input.FieldValues,
		CanonicalUrl:    input.CanonicalUrl,
		MetaTitle:       input.MetaTitle,
		MetaDescription: input.MetaDescription,
		OgImageUrl:      input.OgImageUrl,
		SeoIndexable:    true,
		Status:          input.Status,
		Visibility:      input.Visibility,
	}
	if input.SeoIndexable != nil {
		createInput.SeoIndexable = *input.SeoIndexable
	}

	if input.SpaceID != nil && *input.SpaceID != "" {
		if err := createInput.SpaceID.Scan(*input.SpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
	}

	if input.GeoSpaceID != nil && *input.GeoSpaceID != "" {
		if err := createInput.GeoSpaceID.Scan(*input.GeoSpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo_space_id format")
			return
		}
	}

	if input.HeaderImageID != nil && *input.HeaderImageID != "" {
		if err := createInput.HeaderImageID.Scan(*input.HeaderImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_header_image_id", "Invalid header_image_id format")
			return
		}
	}

	if input.FeaturedImageID != nil && *input.FeaturedImageID != "" {
		if err := createInput.FeaturedImageID.Scan(*input.FeaturedImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_featured_image_id", "Invalid featured_image_id format")
			return
		}
	}

	if input.PublishAt != nil && *input.PublishAt != "" {
		if err := createInput.PublishAt.Scan(*input.PublishAt); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_publish_at", "Invalid publish_at format")
			return
		}
	}

	post, err := h.content.CreateContent(r.Context(), tenantID, createInput)
	if err != nil {
		slog.Error("failed to create blog post", "error", err, "title", input.Title, "slug", input.Slug)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create blog post")
		return
	}

	// Record the creation event for the spam rate-limit layer (§41, BUG-004).
	// Fire-and-forget — the pipeline records a timestamp in Valkey and never
	// blocks the response.
	if h.spamRecorder != nil {
		h.spamRecorder.RecordContentCreation(r.Context(), tenantID, user.UserID)
	}

	// Dispatch @mention notifications — fire-and-forget (BL-002, Section 15).
	h.dispatchMentionNotifications(r.Context(), tenantID, user.UserID, post.ID, post.SpaceID, input.Body)

	// BUG-009: enqueue rss:regenerate when a blog post is created directly as
	// published, so the feed reflects it without waiting for the hourly cron.
	// The content HTTP handler does this at content/handlers.go:914, but the
	// blog CreatePost calls the service directly, bypassing that enqueue.
	// Ref: §24 "RSS feed (static XML, regenerated via the new rss_regenerate
	// task on every publish)".
	if post.Status == content.StatusPublished {
		h.enqueueRSSRegenerate(tenantID)
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": post})
}

// SubmitForReview handles POST /api/v1/blog/{id}/submit-for-review —
// transitions the post's content status to in_review AND creates a pending
// content_reviews row so the post appears in the admin review-queue (C1).
func (h *Handlers) SubmitForReview(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	existing, err := h.content.GetContent(r.Context(), contentID, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}

	// A blog-specific action must not be usable to change the status of
	// unrelated content (forum threads, pages, etc.) reached by guessing IDs.
	if existing.ContentType != contentTypeBlogPost {
		errpkg.WriteError(w, http.StatusBadRequest, "wrong_content_type", "Not a blog post")
		return
	}

	post, err := h.content.UpdateContentStatus(r.Context(), contentID, tenantID, statusInReview)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to submit post for review")
		return
	}

	// C1: create a pending content_reviews row so the admin review-queue
	// (ListPendingReviewSummaries) can list this submission. BUG-003: the
	// reviewer is unassigned (zero UUID → NULL) — the actual reviewer is
	// recorded when approve/reject is called.
	if _, err := h.Service.CreateReview(r.Context(), tenantID, pgtype.UUID{}, CreateReviewInput{
		ContentID: contentID,
		Status:    ReviewStatusPending,
	}); err != nil {
		slog.Error("failed to create content review on submit", "error", err, "content_id", contentID)
		// Don't fail the request — the status flip already succeeded.
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": post})
}

// FeaturePostByID handles POST /api/v1/blog/{id}/feature — the catalog's
// path-based featuring, alongside the existing body-based POST /blog/featured.
func (h *Handlers) FeaturePostByID(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	existing, err := h.content.GetContent(r.Context(), contentID, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Post not found")
		return
	}

	if existing.ContentType != contentTypeBlogPost {
		errpkg.WriteError(w, http.StatusBadRequest, "wrong_content_type", "Not a blog post")
		return
	}

	var input struct {
		FeaturedOrder int32  `json:"featured_order"`
		FeaturedUntil string `json:"featured_until"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var featuredUntil pgtype.Timestamptz
	if input.FeaturedUntil != "" {
		if err := featuredUntil.Scan(input.FeaturedUntil); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_featured_until", "Invalid featured_until date format")
			return
		}
	}

	featured, err := h.Service.FeaturePost(r.Context(), tenantID, FeaturePostInput{
		ContentID:     contentID,
		FeaturedOrder: input.FeaturedOrder,
		FeaturedUntil: featuredUntil,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": featured})
}

// verifyOwnBlogPost loads the content row for the given id, verifies it is a
// blog_post in the caller's tenant, and confirms the caller is the author OR a
// super admin. Returns the loaded row on success. Used by UpdatePost and
// DeletePost to enforce the "own" scoping of blog.edit_own (BUG-001/BUG-002:
// the generic contentHandlers.UpdateContent/DeleteContent do not check
// authorship, so without this any blog.edit_own holder could edit/delete ANY
// blog post by id). Ref: japaconnect-engineering/sections/24-blog-extension.md
// ("blog.edit_own" — edit OWN posts).
func (h *Handlers) verifyOwnBlogPost(w http.ResponseWriter, r *http.Request, user *auth.AuthenticatedUser, tenantID, id pgtype.UUID) (gen.GetContentByIDRow, bool) {
	existing, err := h.content.GetContent(r.Context(), id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Blog post not found")
			return gen.GetContentByIDRow{}, false
		}
		handleServiceError(w, err)
		return gen.GetContentByIDRow{}, false
	}

	if existing.ContentType != contentTypeBlogPost {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Blog post not found")
		return gen.GetContentByIDRow{}, false
	}

	isAuthor := existing.AuthorID.Valid && existing.AuthorID.Bytes == user.UserID.Bytes
	if !isAuthor {
		// Super admins bypass the own-post check (they hold all permissions
		// implicitly). blog.edit_any is intentionally NOT honored here — the
		// §24 "edit any" path is the admin surface PATCH /admin/blog/{id}
		// gated by blog.review.manage. The user-facing PATCH /blog/{id} is
		// strictly own-post per §24.
		if resolver := rbac.FromContext(r.Context()); resolver != nil &&
			resolver.IsSuperAdmin(r.Context(), user.UserID, user.TenantID) {
			return existing, true
		}
		errpkg.WriteError(w, http.StatusForbidden, "not_author", "You can only edit or delete your own blog posts")
		return gen.GetContentByIDRow{}, false
	}

	return existing, true
}

// UpdatePost handles PATCH /api/v1/blog/{id} — edits the caller's own blog
// post. BUG-001: the route previously delegated to contentHandlers.UpdateContent
// which performs NO authorship check, so any user holding blog.edit_own could
// edit ANY blog post by id. This handler enforces own-post scoping before
// delegating to the content service. Ref: §24 blog.edit_own.
func (h *Handlers) UpdatePost(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	existing, ok := h.verifyOwnBlogPost(w, r, user, tenantID, id)
	if !ok {
		return
	}

	var raw struct {
		Title           string          `json:"title"`
		Slug            string          `json:"slug"`
		Body            string          `json:"body"`
		BodyHtml        string          `json:"body_html"`
		FieldValues     json.RawMessage `json:"field_values"`
		CanonicalUrl    *string         `json:"canonical_url"`
		HeaderImageID   *string         `json:"header_image_id"`
		FeaturedImageID *string         `json:"featured_image_id"`
		MetaTitle       *string         `json:"meta_title"`
		MetaDescription *string         `json:"meta_description"`
		OgImageUrl      *string         `json:"og_image_url"`
		SeoIndexable    *bool           `json:"seo_indexable"`
		Status          string          `json:"status"`
		Visibility      string          `json:"visibility"`
		PublishAt       *string         `json:"publish_at"`
		GeoSpaceID      *string         `json:"geo_space_id"`
		OrganizationID  *string         `json:"organization_id"`
		IsLocked        bool            `json:"is_locked"`
		IsPinned        bool            `json:"is_pinned"`
		GuideCategory   *string         `json:"guide_category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if raw.Status != "" && !content.IsValidStatus(raw.Status) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status",
			fmt.Sprintf("Invalid status: %q", raw.Status))
		return
	}
	if raw.Visibility != "" && !content.IsValidVisibility(raw.Visibility) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_visibility",
			fmt.Sprintf("Invalid visibility: %q", raw.Visibility))
		return
	}

	body := raw.Body
	if body == "" && raw.BodyHtml != "" {
		body = content.SanitizeText(raw.BodyHtml)
	}

	input := content.UpdateContentInput{
		Title:           raw.Title,
		Slug:            raw.Slug,
		Body:            content.SanitizeText(body),
		BodyHtml:        content.SanitizeHTML(raw.BodyHtml),
		FieldValues:     raw.FieldValues,
		CanonicalUrl:    raw.CanonicalUrl,
		MetaTitle:       raw.MetaTitle,
		MetaDescription: raw.MetaDescription,
		OgImageUrl:      raw.OgImageUrl,
		SeoIndexable:    raw.SeoIndexable,
		Status:          raw.Status,
		Visibility:      raw.Visibility,
		IsLocked:        raw.IsLocked,
		IsPinned:        raw.IsPinned,
		GuideCategory:   raw.GuideCategory,
	}

	if strings.TrimSpace(body) != "" {
		hash := content.ComputeContentHash(body)
		input.ContentHash = &hash
	}

	if raw.GeoSpaceID != nil && *raw.GeoSpaceID != "" {
		if err := input.GeoSpaceID.Scan(*raw.GeoSpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo_space_id format")
			return
		}
	}
	if raw.OrganizationID != nil && *raw.OrganizationID != "" {
		if err := input.OrganizationID.Scan(*raw.OrganizationID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id format")
			return
		}
	}
	if raw.HeaderImageID != nil && *raw.HeaderImageID != "" {
		if err := input.HeaderImageID.Scan(*raw.HeaderImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_header_image_id", "Invalid header_image_id format")
			return
		}
	}
	if raw.FeaturedImageID != nil && *raw.FeaturedImageID != "" {
		if err := input.FeaturedImageID.Scan(*raw.FeaturedImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_featured_image_id", "Invalid featured_image_id format")
			return
		}
	}
	if raw.PublishAt != nil && *raw.PublishAt != "" {
		if err := input.PublishAt.Scan(*raw.PublishAt); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_publish_at", "Invalid publish_at format")
			return
		}
		if input.PublishAt.Valid && input.PublishAt.Time.Before(time.Now()) {
			errpkg.WriteError(w, http.StatusBadRequest, "publish_at_in_past", "publish_at must be in the future")
			return
		}
	}

	bypassEditWindow := false
	if resolver := rbac.FromContext(r.Context()); resolver != nil {
		bypassEditWindow = resolver.IsSuperAdmin(r.Context(), user.UserID, user.TenantID)
	}

	updated, err := h.content.UpdateContent(r.Context(), id, tenantID, user.UserID, input, bypassEditWindow)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-009: enqueue rss:regenerate on draft→published transition. The
	// content HTTP handler does this at content/handlers.go:1308, but the
	// blog UpdatePost calls the service directly. Ref: §24.
	if existing.Status != content.StatusPublished && updated.Status == content.StatusPublished {
		h.enqueueRSSRegenerate(tenantID)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// DeletePost handles DELETE /api/v1/blog/{id} — soft-deletes the caller's own
// blog post. BUG-002: the route previously delegated to contentHandlers.DeleteContent
// which performs NO authorship check, so any user holding blog.edit_own could
// delete ANY blog post by id. This handler enforces own-post scoping before
// delegating to the content service. Ref: §24 blog.edit_own.
func (h *Handlers) DeletePost(w http.ResponseWriter, r *http.Request) {
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if _, ok := h.verifyOwnBlogPost(w, r, user, tenantID, id); !ok {
		return
	}

	if err := h.content.DeleteContent(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
