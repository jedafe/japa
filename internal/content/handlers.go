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
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/worker"
)

const defaultPageSize int32 = 20

// defaultMaxTags is the fallback for content.max_tags when the setting is
// unset or unparseable. Matches the settings catalog default.
const defaultMaxTags = 5

// defaultMinLength is the fallback for content.min_length when the setting
// is unset or unparseable. Matches the settings catalog default (§14).
const defaultMinLength = 10

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
	Service             *Service
	AsynqClient         TaskEnqueueClient
	SpamPipeline        *spam.SpamPipeline
	settings            *config.Settings
	notifier            MentionNotifier      // nil disables mention notifications (test-only)
	spaceChecker        SpacePostChecker     // nil disables space posting-perm checks (BUG-003)
	spaceMatcher        SpaceKeywordMatcher  // nil disables auto-assign by keyword matching
	tagResolver         TagSlugResolver      // nil disables bulk tag_slugs resolution (BUG-006)
	mentionCheckers     *mentions.DBCheckers // nil disables privacy filtering (BUG-004)
	guideCategoryLister GuideCategoryLister  // nil disables guide category listing
}

// TagSlugResolver resolves tag slugs to tag IDs within a tenant (Section 14).
// Implemented by tags.Service to avoid an import cycle (tags imports content).
type TagSlugResolver interface {
	ResolveTagSlugs(ctx context.Context, tenantID pgtype.UUID, slugs []string) (map[string]pgtype.UUID, error)
	// GetOrCreateTagSlugs resolves slugs to IDs, creating any slug that does
	// not yet exist as a new tag. Used by the composer's tag_slugs flow where
	// users type new tag names that may not exist yet.
	GetOrCreateTagSlugs(ctx context.Context, tenantID pgtype.UUID, slugs []string) (map[string]pgtype.UUID, error)
}

// SpacePostChecker checks whether a user may post into a space, based on the
// space's posting_perm axis (Section 05 lines 145-154, BUG-003). Implemented
// by spaces.Service.CanPostInSpace via an adapter.
type SpacePostChecker interface {
	CanPostInSpace(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error)
}

// SpaceKeywordMatcher finds primary+public spaces whose keywords match the
// content title, for auto-assigning content to spaces when the user didn't
// select one. Implemented by spaces.Service.
type SpaceKeywordMatcher interface {
	FindSpacesByKeywords(ctx context.Context, tenantID pgtype.UUID, keywords []string) ([]gen.FindSpacesByKeywordsRow, error)
}

// SetSpacePostChecker injects the space posting-permission checker so the
// content creation handler can enforce the space's posting_perm axis.
func (h *Handlers) SetSpacePostChecker(c SpacePostChecker) {
	h.spaceChecker = c
}

// SetSpaceMatcher injects the space keyword matcher for auto-assigning
// content to spaces based on title keyword matching.
func (h *Handlers) SetSpaceMatcher(m SpaceKeywordMatcher) {
	h.spaceMatcher = m
}

// SetTagResolver injects the tag slug resolver for bulk tag_slugs support (BUG-006).
func (h *Handlers) SetTagResolver(r TagSlugResolver) {
	h.tagResolver = r
}

// GuideCategory is a evergreen guide category (Visa, Travel, Scholarship, etc.)
// stored in custom_field_entity_types with value prefix 'guide_category:'.
type GuideCategory struct {
	Slug  string
	Label string
}

// GuideCategoryLister lists guide categories for a tenant. Implemented by
// an adapter in server.go wrapping customfields.Service to avoid an import
// cycle (customfields cannot import content).
type GuideCategoryLister interface {
	ListGuideCategories(ctx context.Context, tenantID pgtype.UUID) ([]GuideCategory, error)
}

// SetGuideCategoryLister injects the guide category lister so ListContentTypes
// can dynamically return admin-configured guide categories.
func (h *Handlers) SetGuideCategoryLister(l GuideCategoryLister) {
	h.guideCategoryLister = l
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

// SetMentionCheckers wires the DB-backed privacy checkers (blocks, erased
// users, private-space membership) used by FilterMentions (BUG-004).
// nil disables privacy filtering (backward compatibility / tests).
func (h *Handlers) SetMentionCheckers(c *mentions.DBCheckers) { h.mentionCheckers = c }

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
	case errors.Is(err, ErrDuplicateContent):
		writeError(w, http.StatusConflict, "duplicate_content", "Content with this body already exists")
	case errors.Is(err, ErrOrgPostForbidden):
		writeError(w, http.StatusForbidden, "org_post_forbidden", "Not authorized to post as this organization")
	case errors.Is(err, ErrEditWindowExpired):
		writeError(w, http.StatusForbidden, "edit_window_expired", "The edit window for this content has expired")
	default:
		errpkg.HandleServiceError(w, err)
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

// autoAssignSpaces finds primary+public spaces whose keywords appear in the
// content title and links the content to those spaces via content_spaces.
// The deepest match (city > state > country) becomes the primary space
// (set on contents.space_id). This only runs when the user didn't explicitly
// select a space. Errors are logged but never block content creation.
func (h *Handlers) autoAssignSpaces(ctx context.Context, tenantID, contentID pgtype.UUID, title string) {
	// Extract keywords from the title: lowercase, split on word boundaries,
	// filter out very short words and common stopwords.
	words := strings.Fields(strings.ToLower(title))
	keywords := make([]string, 0, len(words))
	for _, w := range words {
		// Strip punctuation.
		w = strings.Trim(w, ".,!?;:\"'()[]{}—–-")
		if len(w) < 3 {
			continue // skip very short words
		}
		keywords = append(keywords, w)
	}
	if len(keywords) == 0 {
		return
	}

	matches, err := h.spaceMatcher.FindSpacesByKeywords(ctx, tenantID, keywords)
	if err != nil {
		slog.Warn("content: auto-assign space keyword match failed", "error", err)
		return
	}
	if len(matches) == 0 {
		return
	}

	// The first match is the deepest (city > state > country) due to ORDER BY.
	// Set it as the primary space_id on the content, and link all matches
	// via content_spaces.
	primarySpaceID := matches[0].ID
	var additionalIDs []pgtype.UUID
	for _, m := range matches[1:] {
		additionalIDs = append(additionalIDs, m.ID)
	}

	if err := h.Service.UpdateContentSpaces(ctx, contentID, primarySpaceID, additionalIDs); err != nil {
		slog.Warn("content: auto-assign UpdateContentSpaces failed",
			"content_id", contentID.String(), "error", err)
		return
	}

	// Also update the content's space_id column for backward-compat queries.
	if err := h.Service.SetContentSpaceID(ctx, contentID, primarySpaceID); err != nil {
		slog.Warn("content: auto-assign SetContentSpaceID failed",
			"content_id", contentID.String(), "error", err)
	}

	slog.Info("content: auto-assigned spaces",
		"content_id", contentID.String(),
		"primary_space", matches[0].Name,
		"match_count", len(matches))
}

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

// enqueueFeedFanOut enqueues content:fan_out_to_feeds tasks for published
// content (BL-005). Per Section 32, fan-out fires:
//  1. To the author's followers (always).
//  2. To each assigned space's members (once per space in content_spaces,
//     Section 14). If the content has no content_spaces rows, falls back to
//     the primary contents.space_id.
//  3. Bubble fan-out to parent spaces for each assigned space.
//
// The worker decides push vs pull per space based on feed.fanout_threshold.
// Fire-and-forget per Rule 8.
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md
func (h *Handlers) enqueueFeedFanOut(ctx context.Context, contentID, tenantID, authorID, primarySpaceID pgtype.UUID, contentType string) {
	if h.AsynqClient == nil {
		return
	}

	// 1. Always fan out to the author's followers.
	h.enqueueFanOutTask(contentID, tenantID, authorID, pgtype.UUID{}, contentType, "followers")

	// 2. Fan out to each assigned space via content_spaces (Section 14).
	//    Falls back to the primary contents.space_id if no content_spaces rows
	//    exist (backwards compatibility with pre-content_spaces content).
	spaceIDs := []pgtype.UUID{}
	if h.Service != nil && h.Service.read != nil {
		rows, err := h.Service.read.GetContentSpaces(ctx, contentID)
		if err != nil {
			slog.Warn("content: failed to fetch content_spaces for fan-out, using primary space_id",
				"content_id", contentID.String(), "error", err)
		} else {
			for _, r := range rows {
				spaceIDs = append(spaceIDs, r.SpaceID)
			}
		}
	}
	if len(spaceIDs) == 0 && primarySpaceID.Valid {
		spaceIDs = append(spaceIDs, primarySpaceID)
	}

	// 3. Enqueue a "space" task and a "bubble" task per assigned space.
	for _, sid := range spaceIDs {
		h.enqueueFanOutTask(contentID, tenantID, authorID, sid, contentType, "space")
		h.enqueueFanOutTask(contentID, tenantID, authorID, sid, contentType, "bubble")
	}
}

// enqueueFanOutTask marshals and enqueues a single fan-out task.
func (h *Handlers) enqueueFanOutTask(contentID, tenantID, authorID, spaceID pgtype.UUID, contentType, fanOutType string) {
	payload, err := json.Marshal(map[string]string{
		"content_id":   contentID.String(),
		"tenant_id":    tenantID.String(),
		"author_id":    authorID.String(),
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
			"content_id", contentID.String(), "fan_out_type", fanOutType, "error", err)
	}
}

// enqueueRSSRegenerate enqueues the rss:regenerate task for a tenant so the
// universal + blog RSS feeds reflect newly published content without waiting
// for the hourly cron. Fire-and-forget per Rule 8 (RSS-013).
//
// Ref: japaconnect-engineering/sections/26-seo-performance.md
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md
func (h *Handlers) enqueueRSSRegenerate(tenantID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal rss:regenerate payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeRSSRegenerate, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeRSSRegenerate)),
	); err != nil {
		slog.Warn("content: failed to enqueue rss:regenerate task",
			"tenant_id", tenantID.String(), "error", err)
	}
}

// dispatchMentionNotifications extracts @mentioned users from the content
// body (TipTap JSON) and fires a mention.content notification to each via
// the batch system (Section 35 — BL-002). Fire-and-forget per Rule 8:
// errors are logged but never propagated to the caller.
//
// Self-mentions (mentioning the content author) are skipped.
// Privacy filtering (blocks, erased users, private-space membership) is
// applied via mentions.FilterMentions when checkers are wired (Section 15).
func (h *Handlers) dispatchMentionNotifications(ctx context.Context, tenantID, authorID, contentID, spaceID pgtype.UUID, body string) {
	if h.notifier == nil {
		return
	}
	mentioned, err := mentions.ExtractMentions([]byte(body))
	if err != nil {
		// Body may not be valid TipTap JSON (e.g., plain text); skip mentions.
		return
	}

	// Apply privacy filtering when DB checkers are wired (BUG-004):
	// skip erased users (GDPR), blocked users (privacy), and non-members
	// of private spaces (visibility leak). Section 15 mention edge cases.
	if h.mentionCheckers != nil {
		isPrivateSpace := false
		if spaceID.Valid {
			space, err := h.Service.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
				ID:       spaceID,
				TenantID: tenantID,
			})
			if err == nil && space.Visibility != "public" {
				isPrivateSpace = true
			}
		}

		mentioned, err = mentions.FilterMentions(
			ctx, mentioned, tenantID, authorID, spaceID, isPrivateSpace,
			h.mentionCheckers, h.mentionCheckers, h.mentionCheckers,
		)
		if err != nil {
			// FilterMentions logs individual checker errors internally and
			// never returns a fatal error. This branch is retained for
			// forward-compatibility but is not expected to fire.
			slog.Warn("content: FilterMentions returned unexpected error",
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
			slog.Warn("content: failed to dispatch mention.content notification",
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

// hrefURLRegex extracts http(s) URLs from href attributes in HTML.
var hrefURLRegex = regexp.MustCompile(`(?i)href="(https?://[^"]+)"`)

// enqueueLinkPreviewsForBody extracts external http(s) URLs from the HTML
// body and enqueues a link preview fetch task for each. Fire-and-forget
// (Section 15 — external links get async OG/oEmbed scrape). Called after
// content create/update so previews are cached for editor display.
func (h *Handlers) enqueueLinkPreviewsForBody(bodyHTML string, contentID, tenantID pgtype.UUID) {
	if h.AsynqClient == nil || bodyHTML == "" {
		return
	}

	matches := hrefURLRegex.FindAllStringSubmatch(bodyHTML, -1)
	seen := make(map[string]bool, len(matches))
	blockID := contentID.String()

	for _, m := range matches {
		url := m[1]
		if seen[url] {
			continue
		}
		seen[url] = true
		h.enqueueLinkPreview(url, blockID, tenantID)
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
		GuideCategory   *string         `json:"guide_category"`
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

	// Enforce content.min_length on non-draft content (§14).
	// Drafts may have empty bodies; published/scheduled content must meet
	// the minimum length configured in system_settings.
	if input.Status != StatusDraft {
		minLength := defaultMinLength
		if h.settings != nil {
			minLength = h.settings.GetIntOr(r.Context(), tenantID, "content.min_length", defaultMinLength)
		}
		if len(strings.TrimSpace(body)) < minLength {
			writeError(w, http.StatusBadRequest, "body_too_short",
				fmt.Sprintf("Content body must be at least %d characters", minLength))
			return
		}
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
		GuideCategory:   input.GuideCategory,
	}
	if input.SeoIndexable != nil {
		createInput.SeoIndexable = *input.SeoIndexable
	}

	if input.SpaceID != nil && *input.SpaceID != "" {
		if err := createInput.SpaceID.Scan(*input.SpaceID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}

		// BUG-003: enforce the space's posting_perm axis (Section 05 lines
		// 145-154). The check is skipped when no space checker is wired
		// (backward compatibility / tests).
		if h.spaceChecker != nil {
			allowed, err := h.spaceChecker.CanPostInSpace(r.Context(), createInput.SpaceID, user.UserID, tenantID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to check space posting permission")
				return
			}
			if !allowed {
				writeError(w, http.StatusForbidden, "space_post_denied",
					"You do not have permission to post in this space")
				return
			}
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
		clientIP := clientip.FromRequest(r)
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

	// Record the creation for the spam rate-limit layer (BUG-004). Without
	// this the jc:spam:rate:{tenant}:{user} sorted set is never populated and
	// the rate_limit scoring layer always returns 0.
	if h.SpamPipeline != nil {
		h.SpamPipeline.RecordContentCreation(r.Context(), tenantID, user.UserID)
	}

	// Auto-assign content to spaces based on keyword matching in the title.
	// Only runs when no space_id was explicitly selected by the user and a
	// space matcher is wired. Matches primary (country/state/city) and public
	// spaces only — the deepest match (city > state > country) becomes the
	// primary space, and all matches are linked via content_spaces.
	if !createInput.SpaceID.Valid && h.spaceMatcher != nil {
		h.autoAssignSpaces(r.Context(), tenantID, content.ID, input.Title)
	}

	// Enqueue async embedding generation — fire-and-forget.
	h.enqueueContentEmbedding(content.ID, tenantID)

	// Enqueue search index sync — fire-and-forget.
	h.enqueueSearchIndex(content.ID, tenantID, "index")

	// Enqueue moderation scan — fire-and-forget.
	h.enqueueModerationScan("content", content.ID, tenantID)

	// Dispatch @mention notifications — fire-and-forget (BL-002).
	h.dispatchMentionNotifications(r.Context(), tenantID, user.UserID, content.ID, content.SpaceID, content.Body)

	// Enqueue feed fan-out for published content (BL-005). Scheduled posts
	// are fanned out by the scheduled-publish handler when they go live.
	if content.Status == StatusPublished {
		h.enqueueFeedFanOut(r.Context(), content.ID, tenantID, user.UserID, content.SpaceID, content.ContentType)
		// RSS-013: regenerate RSS feeds so new content appears without
		// waiting for the hourly cron.
		h.enqueueRSSRegenerate(tenantID)
	}

	// Enqueue link preview fetches for external URLs in the body — fire-and-forget
	// (Section 15 — external links get async OG/oEmbed scrape, BUG-003).
	h.enqueueLinkPreviewsForBody(input.BodyHtml, content.ID, tenantID)

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

	// Visibility gate: unauthenticated users may only view published
	// content. Drafts, scheduled, archived, and removed content require
	// authentication (and the author/admin permission for non-public
	// statuses). Ref: Section 14 — public content SSR.
	if user := auth.FromContext(r.Context()); user == nil {
		if content.Status != StatusPublished {
			writeError(w, http.StatusNotFound, "not_found", "Content not found")
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

	// JSON-LD structured data (Section 14 / Section 26). Resolve the
	// admin-managed template matching the content's own content_type,
	// interpolating the content's field values. Best-effort: a missing
	// template or resolution error degrades to no json_ld field. Emitted as
	// a sibling of `data` for frontend consumption.
	resp := map[string]any{"data": content}
	if jsonLd, jErr := h.Service.ResolveJSONLD(r.Context(), tenantID, content.ContentType, contentJSONLDFields(content)); jErr == nil && len(jsonLd) > 0 {
		resp["json_ld"] = json.RawMessage(jsonLd)
	} else if jErr != nil {
		slog.Warn("content: JSON-LD template resolution failed", "slug", slug, "content_type", content.ContentType, "error", jErr)
	}

	// GDPR / ADR-029: redact mentions of erased users in body_html and
	// expose deleted_user_ids so the frontend can render @[deleted user]
	// when re-rendering from TipTap JSON (Section 15). Best-effort: a
	// failure logs a warning but does not fail the request.
	if h.mentionCheckers != nil {
		redactedHTML, erasedIDs, redErr := mentions.RedactErasedMentionsInBody(
			r.Context(),
			[]byte(content.Body),
			content.BodyHtml,
			tenantID,
			h.mentionCheckers,
		)
		if redErr != nil {
			slog.Warn("content: RedactErasedMentionsInBody failed",
				"content_id", content.ID.String(), "error", redErr)
		} else {
			content.BodyHtml = redactedHTML
			if len(erasedIDs) > 0 {
				resp["deleted_user_ids"] = erasedIDs
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// contentJSONLDFields builds the {{field_key}} → value map for interpolating
// a JSON-LD template from a generic content row. The keys cover the common
// placeholders used across Article, QAPage, Review, FAQPage, and JobPosting
// templates; unused keys are simply ignored by the interpolator.
func contentJSONLDFields(c gen.GetContentBySlugRow) map[string]string {
	desc := ""
	if c.MetaDescription != nil {
		desc = *c.MetaDescription
	}
	canonical := ""
	if c.CanonicalUrl != nil {
		canonical = *c.CanonicalUrl
	}
	return map[string]string{
		"title":           c.Title,
		"description":     desc,
		"body":            c.Body,
		"canonical_url":   canonical,
		"published_at":    tsFormat(c.PublishedAt),
		"updated_at":      tsFormat(c.UpdatedAt),
		"created_at":      tsFormat(c.CreatedAt),
		"cover_image_url": "",
		"author_name":     "",
	}
}

// tsFormat renders a pgtype.Timestamptz as RFC3339 or empty.
func tsFormat(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
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
		GuideCategory   *string         `json:"guide_category"`
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

	// Enforce content.min_length on non-draft content (§14).
	// Only check when a body is actually being updated (non-empty).
	if body != "" && input.Status != StatusDraft {
		minLength := defaultMinLength
		if h.settings != nil {
			minLength = h.settings.GetIntOr(r.Context(), tenantID, "content.min_length", defaultMinLength)
		}
		if len(strings.TrimSpace(body)) < minLength {
			writeError(w, http.StatusBadRequest, "body_too_short",
				fmt.Sprintf("Content body must be at least %d characters", minLength))
			return
		}
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
		GuideCategory:   input.GuideCategory,
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

	// Capture previous status to detect draft→published transitions for fan-out.
	prevContent, prevErr := h.Service.GetContent(r.Context(), id, tenantID)

	// Super admins bypass the edit-window restriction even on their own
	// content — without this, a super_admin who authored a post more than
	// content.edit_window_minutes ago is locked out of editing it (BL-008).
	bypassEditWindow := false
	if resolver := rbac.FromContext(r.Context()); resolver != nil {
		bypassEditWindow = resolver.IsSuperAdmin(r.Context(), user.UserID, user.TenantID)
	}

	content, err := h.Service.UpdateContent(r.Context(), id, tenantID, user.UserID, updateInput, bypassEditWindow)
	if err != nil {
		if errors.Is(err, ErrOrgPostForbidden) {
			writeError(w, http.StatusForbidden, "org_post_forbidden", "You are not authorized to edit as this organization")
			return
		}
		handleServiceError(w, err)
		return
	}

	// Enqueue async embedding generation — fire-and-forget.
	// BUG-002 fix: if the content transitioned from published to non-published,
	// purge RAG embeddings instead of re-indexing (ADR-069 §2: unpublished
	// content must not be retrievable in RAG).
	if prevErr == nil && prevContent.Status == StatusPublished && content.Status != StatusPublished {
		worker.PurgeRAGEntity(r.Context(), tenantID, "content", id)
	} else {
		h.enqueueContentEmbedding(id, tenantID)
	}

	// Enqueue search index sync — fire-and-forget.
	h.enqueueSearchIndex(id, tenantID, "index")

	// Enqueue moderation scan — fire-and-forget.
	h.enqueueModerationScan("content", id, tenantID)

	// Dispatch @mention notifications for newly added mentions — fire-and-forget
	// (BUG-006). The notification batch system deduplicates by (recipient, type,
	// subject) so mentions that were already notified on create won't produce
	// duplicate notifications — the existing batch window is extended instead.
	h.dispatchMentionNotifications(r.Context(), tenantID, user.UserID, content.ID, content.SpaceID, content.Body)

	// Enqueue feed fan-out when content transitions to published (BL-005).
	// This covers the draft→published path that CreateContent doesn't handle.
	if prevErr == nil && prevContent.Status != StatusPublished && content.Status == StatusPublished {
		h.enqueueFeedFanOut(r.Context(), id, tenantID, content.AuthorID, content.SpaceID, content.ContentType)
		// RSS-013: regenerate RSS feeds on draft→published transition.
		h.enqueueRSSRegenerate(tenantID)
	}

	// Enqueue link preview fetches for external URLs in the body — fire-and-forget
	// (Section 15 — external links get async OG/oEmbed scrape, BUG-003).
	h.enqueueLinkPreviewsForBody(input.BodyHtml, id, tenantID)

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

	// BUG-002 fix: purge RAG embeddings on content delete (ADR-069 §2:
	// "Content is unpublished or deleted — deletes its content_embeddings
	// rows"). Fire-and-forget — errors are logged, never propagated.
	worker.PurgeRAGEntity(r.Context(), tenantID, "content", id)

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

	// Capture previous status to detect draft→published transitions for fan-out.
	prevContent, prevErr := h.Service.GetContent(r.Context(), id, tenantID)

	updated, err := h.Service.UpdateContentStatus(r.Context(), id, tenantID, StatusPublished)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// AUDIT-BE-TASK-001: enqueue the affiliate link scan task on publish
	// (Section 55 — fire-and-forget, low tier). The worker handler reads
	// the affiliate.scan_on_publish system setting; if false, it exits early.
	h.enqueueAffiliateLinkScan(id, tenantID)

	// Enqueue feed fan-out when content transitions to published (BL-005).
	// This covers the draft→published path that CreateContent doesn't handle.
	if prevErr == nil && prevContent.Status != StatusPublished {
		h.enqueueFeedFanOut(r.Context(), id, tenantID, updated.AuthorID, updated.SpaceID, updated.ContentType)
		// RSS-013: regenerate RSS feeds on explicit publish.
		h.enqueueRSSRegenerate(tenantID)
	}

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

	// BUG-002 fix: purge RAG embeddings on archive (ADR-069 §2: content that
	// is no longer published must not be retrievable in RAG). Fire-and-forget.
	worker.PurgeRAGEntity(r.Context(), tenantID, "content", id)

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
// Uses OptionalAuth so both authenticated and anonymous viewers are
// counted. For authenticated users, the user_id is recorded; for
// anonymous users, the session_id (or a fingerprint fallback) is used
// for deduplication. The country_code is resolved server-side from the
// client IP via the GeoIP middleware (Section 14 — view tracking).
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

	// Resolve user (optional — anonymous views are allowed) and session.
	var userID pgtype.UUID
	var sessionID *string
	if user := auth.FromContext(r.Context()); user != nil {
		userID = user.UserID
		if user.SessionID != "" {
			s := user.SessionID
			sessionID = &s
		}
	}

	// Resolve country code from GeoIP context (set by GeoIPMiddleware).
	var countryCode *string
	if geo := geoip.FromContext(r.Context()); geo.IsValid && geo.CountryCode != "" {
		cc := geo.CountryCode
		countryCode = &cc
	}

	// For anonymous users without a session ID, use the client IP as a
	// fingerprint for deduplication (prevents inflate-by-refresh).
	if !userID.Valid && sessionID == nil {
		if ip := clientip.FromRequest(r); ip != "" {
			fp := "ip:" + ip
			sessionID = &fp
		}
	}

	if err := h.Service.RecordContentViewWithDetails(r.Context(), id, tenantID, userID, sessionID, countryCode); err != nil {
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
		// ON CONFLICT DO NOTHING returns ErrNoRows when the user already has
		// this reaction — treat as idempotent success, not a 404.
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
				"tenant_id":       tenantID,
				"entity_type":     EntityContent,
				"entity_id":       contentID,
				"user_id":         user.UserID,
				"reaction_key":    input.ReactionKey,
				"already_reacted": true,
			}})
			return
		}
		handleServiceError(w, err)
		return
	}

	// A new reaction was created (not an idempotent duplicate) — update the
	// content_scores.vote_score so feed cards show an accurate vote count.
	// Fire-and-forget per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "vote", 1)

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
	// Fall back to the query string for the singular /content/{id}/react
	// route (catalog line 260), which has no {key} path parameter.
	if reactionKey == "" {
		reactionKey = r.URL.Query().Get("key")
	}
	if reactionKey == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "Reaction key is required")
		return
	}

	if err := h.Service.RemoveReaction(r.Context(), tenantID, EntityContent, contentID, user.UserID, reactionKey); err != nil {
		handleServiceError(w, err)
		return
	}

	// Decrement the vote_score to keep feed card counts accurate.
	// Fire-and-forget per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "vote", -1)

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

// ListUserReactions handles GET /api/v1/content/{id}/reactions/me.
// Returns the current user's reactions for a content item (used by the
// frontend to initialise the like button state on page load).
func (h *Handlers) ListUserReactions(w http.ResponseWriter, r *http.Request) {
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

	reactions, err := h.Service.GetUserReactions(r.Context(), tenantID, EntityContent, contentID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": reactions})
}

// ---------------------------------------------------------------------------
// JSON-LD Template Handlers
// ---------------------------------------------------------------------------

// jsonldTemplateResponse is the wire shape for a JSON-LD template. The sqlc
// generated struct stores `template` as []byte, which encoding/json marshals
// as a base64 string — not a JSON object. Re-mapping to json.RawMessage
// preserves the raw JSON so clients receive a parsed object (§24 data types,
// §3.3 contract drift).
type jsonldTemplateResponse struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	ContentType string          `json:"content_type"`
	Template    json.RawMessage `json:"template"`
	IsActive    bool            `json:"is_active"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// toJSONLDResponse maps a sqlc-generated template row to the wire DTO. A nil
// template body is normalised to `{}` so the field is always valid JSON.
func toJSONLDResponse(t gen.ContentTypeJsonldTemplate) jsonldTemplateResponse {
	body := json.RawMessage(t.Template)
	if len(body) == 0 {
		body = json.RawMessage("{}")
	}
	return jsonldTemplateResponse{
		ID:          t.ID.String(),
		TenantID:    t.TenantID.String(),
		ContentType: t.ContentType,
		Template:    body,
		IsActive:    t.IsActive,
		CreatedAt:   t.CreatedAt.Time.Format(time.RFC3339Nano),
		UpdatedAt:   t.UpdatedAt.Time.Format(time.RFC3339Nano),
	}
}

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

	// A nil slice marshals to `null` in Go, which frontends interpret as "no
	// key" rather than "empty list". Normalise to an empty array so the
	// response is always a well-formed list (§6.2 nil-slice bug class).
	out := make([]jsonldTemplateResponse, 0, len(templates))
	for _, t := range templates {
		out = append(out, toJSONLDResponse(t))
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": out})
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

	writeJSON(w, http.StatusOK, map[string]any{"data": toJSONLDResponse(tmpl)})
}

// DeleteJSONLDTemplate handles DELETE /api/v1/content/jsonld-templates/{id}.
// It removes a single JSON-LD template by its primary key, scoped to the
// caller's tenant (§21 missing CRUD leg, §26 RBAC CRUD completeness).
func (h *Handlers) DeleteJSONLDTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteJSONLDTemplate(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
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
		// Validate folder ownership (BL-013): the folder must belong to the
		// caller and tenant before associating a bookmark with it.
		if err := h.Service.ValidateFolderOwnership(r.Context(), folderID, tenantID, user.UserID); err != nil {
			writeError(w, http.StatusNotFound, "folder_not_found", "Bookmark folder not found")
			return
		}
	}

	bm, err := h.Service.CreateBookmark(r.Context(), tenantID, user.UserID, EntityContent, contentID, input.Folder, folderID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Update content_scores.bookmark_count so feed cards show an accurate
	// bookmark count. Fire-and-forget per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "bookmark", 1)

	writeJSON(w, http.StatusCreated, map[string]any{"data": bm})
}

// RemoveBookmark handles DELETE /api/v1/content/{id}/bookmark.
func (h *Handlers) RemoveBookmark(w http.ResponseWriter, r *http.Request) {
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

	if err := h.Service.RemoveBookmark(r.Context(), tenantID, user.UserID, EntityContent, contentID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Decrement the bookmark_count to keep feed card counts accurate.
	// Fire-and-forget per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "bookmark", -1)

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

// MoveBookmark handles PATCH /api/v1/content/{id}/bookmark/folder.
// Moves an existing bookmark into a different folder (BL-003).
// Body: { "folder_id": "uuid" } — pass empty string or omit to unfile.
func (h *Handlers) MoveBookmark(w http.ResponseWriter, r *http.Request) {
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
		// Validate folder ownership (BL-013).
		if err := h.Service.ValidateFolderOwnership(r.Context(), folderID, tenantID, user.UserID); err != nil {
			writeError(w, http.StatusNotFound, "folder_not_found", "Bookmark folder not found")
			return
		}
	}

	if err := h.Service.SetBookmarkFolder(r.Context(), tenantID, user.UserID, EntityContent, contentID, folderID); err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"folder_id": input.FolderID}})
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
// Accepts the spec-compliant body {tag_slugs: string[]} (Section 14) for bulk
// slug-based tagging, and the legacy {tag_id: string} for backward compatibility.
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
		TagSlugs []string `json:"tag_slugs"`
		TagID    string   `json:"tag_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Resolve input to a list of tag IDs.
	var tagIDs []pgtype.UUID
	if len(input.TagSlugs) > 0 {
		if h.tagResolver == nil {
			writeError(w, http.StatusServiceUnavailable, "tag_resolver_unavailable", "Tag slug resolution is not configured")
			return
		}
		// Get-or-create: user-typed slugs that don't exist yet are auto-
		// created as tenant-scoped tags (Section 14 — composer flow).
		slugMap, err := h.tagResolver.GetOrCreateTagSlugs(r.Context(), tenantID, input.TagSlugs)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_tag_slugs", "One or more tag slugs could not be resolved")
			return
		}
		for _, slug := range input.TagSlugs {
			if id, exists := slugMap[slug]; exists {
				tagIDs = append(tagIDs, id)
			}
		}
	} else if input.TagID != "" {
		// Legacy single tag_id support (backward compatibility).
		var tagID pgtype.UUID
		if err := tagID.Scan(input.TagID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag_id format")
			return
		}
		tagIDs = append(tagIDs, tagID)
	} else {
		writeError(w, http.StatusBadRequest, "missing_tags", "tag_slugs or tag_id is required")
		return
	}

	if len(tagIDs) == 0 {
		writeError(w, http.StatusBadRequest, "no_valid_tags", "No valid tags provided")
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
	if len(existing)+len(tagIDs) > maxTags {
		writeError(w, http.StatusBadRequest, "too_many_tags", "Content would exceed the maximum number of tags")
		return
	}

	added := 0
	for _, tagID := range tagIDs {
		if err := h.Service.AddContentTag(r.Context(), contentID, tagID); err != nil {
			handleServiceError(w, err)
			return
		}
		added++
	}

	_ = user // permission check handled by middleware

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]int{"added": added}})
}

// RemoveContentTag handles DELETE /api/v1/content/{id}/tags/{tagId}.
// The {tagId} path segment accepts either a tag UUID or a tag slug. UUID is
// tried first (zero allocations); if it fails and a tag slug resolver is
// wired, the slug is resolved to a tag UUID within the tenant (Section 14).
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
		// Not a valid UUID — try slug resolution if a resolver is wired.
		if h.tagResolver == nil {
			writeError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag ID format")
			return
		}
		tenantID, ok := h.requireTenant(w, r)
		if !ok {
			return
		}
		slug := chi.URLParam(r, "tagId")
		slugMap, rerr := h.tagResolver.ResolveTagSlugs(r.Context(), tenantID, []string{slug})
		if rerr != nil {
			writeError(w, http.StatusBadRequest, "invalid_tag_slug", "Tag slug could not be resolved")
			return
		}
		var found bool
		tagID, found = slugMap[slug]
		if !found {
			writeError(w, http.StatusNotFound, "tag_not_found", "Tag not found")
			return
		}
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
// The `guide` type has `has_category: true` and a `categories` array listing
// the evergreen guide categories (Visa, Travel, Scholarship, etc.) loaded from
// custom_field_entity_types (value prefix 'guide_category:'). Admins can add
// new categories from the admin panel — no code change required.
func (h *Handlers) ListContentTypes(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := h.requireTenant(w, r)

	guideCategories := []map[string]string{}
	if h.guideCategoryLister != nil && tenantID.Valid {
		cats, err := h.guideCategoryLister.ListGuideCategories(r.Context(), tenantID)
		if err == nil {
			for _, c := range cats {
				guideCategories = append(guideCategories, map[string]string{
					"Slug":  c.Slug,
					"Label": c.Label,
				})
			}
		}
	}

	types := []map[string]any{
		{"value": "discussion", "label": "Discussion"},
		{"value": "blog_post", "label": "Blog Post"},
		{"value": "page", "label": "Page"},
		{"value": "comparison", "label": "Comparison"},
		{"value": "question", "label": "Question"},
		{"value": "guide", "label": "Guide", "has_category": true, "categories": guideCategories},
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}

// UpdateContentSpaces handles PATCH /content/{id}/spaces — replaces the
// full set of space assignments for a content item (Section 14:
// content_spaces). The body must include a primary_space_id and may
// include additional_space_ids. At least one primary space is required.
func (h *Handlers) UpdateContentSpaces(w http.ResponseWriter, r *http.Request) {
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
		PrimarySpaceID    string   `json:"primary_space_id"`
		AdditionalSpaceIDs []string `json:"additional_space_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PrimarySpaceID == "" {
		writeError(w, http.StatusBadRequest, "missing_primary_space",
			"A primary_space_id is required")
		return
	}

	var primarySpaceID pgtype.UUID
	if err := primarySpaceID.Scan(input.PrimarySpaceID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_primary_space_id",
			"Invalid primary_space_id format")
		return
	}

	// Verify the content exists and belongs to the caller's tenant.
	// The route middleware already enforces content.edit permission.
	existing, err := h.Service.GetContent(r.Context(), contentID, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	// Ensure the caller is the author (admins with content.edit_any
	// bypass this via the RBAC layer's _any scope).
	if existing.AuthorID.Valid && existing.AuthorID.Bytes != user.UserID.Bytes {
		writeError(w, http.StatusForbidden, "not_author",
			"Only the author can update space assignments")
		return
	}

	var additionalSpaceIDs []pgtype.UUID
	for _, sidStr := range input.AdditionalSpaceIDs {
		var sid pgtype.UUID
		if err := sid.Scan(sidStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_space_id",
				fmt.Sprintf("Invalid space_id format: %s", sidStr))
			return
		}
		additionalSpaceIDs = append(additionalSpaceIDs, sid)
	}

	if err := h.Service.UpdateContentSpaces(r.Context(), contentID, primarySpaceID, additionalSpaceIDs); err != nil {
		slog.Error("updating content spaces",
			"error", err, "content_id", contentID.String(),
			"user_id", user.UserID.String())
		writeError(w, http.StatusInternalServerError, "internal_error",
			"Failed to update space assignments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"updated": true},
	})
}
