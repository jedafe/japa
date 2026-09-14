// Package content — HTTP handlers for the Content Type System.
// // Every handler is a method on Handlers so dependencies (Service) are
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
	"io"
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
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/visitor"
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
		recipientID pgtype.UUID,
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
	ResolveTagSlugs(ctx context.Context, slugs []string) (map[string]pgtype.UUID, error)
	// GetOrCreateTagSlugs resolves slugs to IDs, creating any slug that does
	// not yet exist as a new tag. Used by the composer's tag_slugs flow where
	// users type new tag names that may not exist yet.
	GetOrCreateTagSlugs(ctx context.Context, slugs []string) (map[string]pgtype.UUID, error)
}

// SpacePostChecker checks whether a user may post into a space, based on the
// space's posting_perm axis (Section 05 lines 145-154, BUG-003). Implemented
// by spaces.Service.CanPostInSpace via an adapter.
type SpacePostChecker interface {
	CanPostInSpace(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error)
}

// SpaceKeywordMatcher finds primary+public spaces (country/state/city and
// public universities) whose keywords, full name, or university initials match
// the content title, for auto-assigning content to spaces. Implemented by
// spaces.Service.
type SpaceKeywordMatcher interface {
	FindSpacesByKeywords(ctx context.Context, keywords []string) ([]gen.FindSpacesByKeywordsRow, error)
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
	ListGuideCategories(ctx context.Context) ([]GuideCategory, error)
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
	return pgtype.UUID{}, true
}

// requireUser resolves the authenticated user from the request context and
// writes a 401 error when none is available.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// canModifyContent reports whether the caller may edit or delete a content
// item. Authors may modify their own content. Super-admins and users with the
// corresponding _any node (content.edit_any / content.delete_any) may modify
// any content. This is the ownership enforcement for the _own/_any RBAC split
// (BUG-001 / BUG-002 / BUG-003).
func canModifyContent(r *http.Request, userID, authorID pgtype.UUID, anyPerm string) bool {
	if userID.Valid && authorID.Valid && userID.Bytes == authorID.Bytes {
		return true
	}

	if resolver := rbac.FromContext(r.Context()); resolver != nil {
		if resolver.IsSuperAdmin(r.Context(), userID) {
			return true
		}

		if resolver.HasPermission(r.Context(), userID, anyPerm) {
			return true
		}
	}

	return false
}

// isStoryModerator reports whether the caller may publish/schedule stories
// directly, bypassing the §79 in_review queue: super-admins and holders of
// story.verify or content.edit_any.
func isStoryModerator(r *http.Request, userID pgtype.UUID) bool {
	resolver := rbac.FromContext(r.Context())
	if resolver == nil {
		return false
	}
	return resolver.IsSuperAdmin(r.Context(), userID) ||
		resolver.HasPermission(r.Context(), userID, "story.verify") ||
		resolver.HasPermission(r.Context(), userID, "content.edit_any")
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrDuplicateContent):
		errpkg.WriteError(w, http.StatusConflict, "duplicate_content", "Content with this body already exists")
	case errors.Is(err, ErrOrgPostForbidden):
		errpkg.WriteError(w, http.StatusForbidden, "org_post_forbidden", "Not authorized to post as this organization")
	case errors.Is(err, ErrEditWindowExpired):
		errpkg.WriteError(w, http.StatusForbidden, "edit_window_expired", "The edit window for this content has expired")
	case errors.Is(err, ErrMissingPrimarySpace):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "missing_primary_space", "Content requires a primary space before publishing or scheduling")
	case errors.Is(err, ErrInvalidStatus):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", err.Error())
	case errors.Is(err, ErrInvalidVisibility):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_visibility", err.Error())
	case errors.Is(err, ErrNotSpaceMember):
		errpkg.WriteError(w, http.StatusForbidden, "not_space_member", "You must be a member of each assigned space")
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	return int32(n)
}

// autoAssignSpaces finds primary+public spaces (country/state/city and
// universities) whose keywords, full name, or university initials appear in the
// content title and links the content to those spaces via content_spaces.
// When the user has already selected a primary space, it is preserved as the
// primary and all title matches become additional spaces. When no space was
// selected, the most specific title match becomes the primary space. Errors
// are logged but never block content creation. Returns the final primary space
// ID (selected or auto-assigned) so the caller can keep its local content row
// in sync for downstream fan-out / notifications.
func (h *Handlers) autoAssignSpaces(ctx context.Context, contentID, selectedSpaceID pgtype.UUID, title string) pgtype.UUID {
	keywords := extractTitleKeywords(title)
	if len(keywords) == 0 {
		return selectedSpaceID
	}

	matches, err := h.spaceMatcher.FindSpacesByKeywords(ctx, keywords)
	if err != nil {
		slog.Warn("content: auto-assign space keyword match failed", "error", err)
		return selectedSpaceID
	}

	if len(matches) == 0 {
		return selectedSpaceID
	}

	// Preserve an explicit user-selected primary space. Otherwise the SQL
	// ordering has already picked the most specific match (initials > name >
	// keywords, then city > state > country > education).
	var primarySpaceID pgtype.UUID

	additionalSet := make(map[pgtype.UUID]struct{})

	if selectedSpaceID.Valid {
		primarySpaceID = selectedSpaceID

		for _, m := range matches {
			if !isSameUUID(m.ID, selectedSpaceID) {
				additionalSet[m.ID] = struct{}{}
			}
		}
	} else {
		primarySpaceID = matches[0].ID

		for _, m := range matches[1:] {
			additionalSet[m.ID] = struct{}{}
		}
	}

	// Also inherit the geo parents of the source space so a post in
	// "Lagos Tech Community" surfaces in Lagos and Nigeria feeds even if the
	// title has no geo keywords.
	if selectedSpaceID.Valid {
		parents, err := h.Service.GetSpacePrimaryGeoParents(ctx, selectedSpaceID)
		if err != nil {
			slog.Warn("content: failed to load source space geo parents", "error", err)
		} else {
			for _, pid := range parents {
				if !isSameUUID(pid, primarySpaceID) {
					additionalSet[pid] = struct{}{}
				}
			}
		}
	}

	additionalIDs := make([]pgtype.UUID, 0, len(additionalSet))
	for id := range additionalSet {
		additionalIDs = append(additionalIDs, id)
	}

	// Auto-assignment is system-driven, so bypass the per-user membership
	// check while keeping the tenant-level space validation.
	if err := h.Service.UpdateContentSpaces(ctx, contentID, pgtype.UUID{}, primarySpaceID, additionalIDs, true); err != nil {
		slog.Warn("content: auto-assign UpdateContentSpaces failed", "content_id", contentID.String(), "error", err)
		return selectedSpaceID
	}

	// Also update the content's space_id column for backward-compat queries.
	if err := h.Service.SetContentSpaceID(ctx, contentID, primarySpaceID); err != nil {
		slog.Warn("content: auto-assign SetContentSpaceID failed", "content_id", contentID.String(), "error", err)
		return selectedSpaceID
	}

	primaryName := ""

	for _, m := range matches {
		if isSameUUID(m.ID, primarySpaceID) {
			primaryName = m.Name
			break
		}
	}

	slog.Info("content: auto-assigned spaces", "content_id", contentID.String(), "primary_space", primaryName, "match_count", len(matches))

	return primarySpaceID
}

func (h *Handlers) enqueueContentEmbedding(contentID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal embedding task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeAIGenerateContentEmbedding, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeAIGenerateContentEmbedding)),
	); err != nil {
		slog.Warn("content: failed to enqueue embedding task", "content_id", contentID.String(), "error", err)
	}
}

// enqueueAffiliateLinkScan enqueues the content:scan_affiliate_links task
// (Section 55). Fire-and-forget: errors are logged but never propagated.
// AUDIT-BE-TASK-001: the task was defined and registered but never enqueued.
func (h *Handlers) enqueueAffiliateLinkScan(contentID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal affiliate scan task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeContentScanAffiliateLinks, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeContentScanAffiliateLinks)),
	); err != nil {
		slog.Warn("content: failed to enqueue affiliate scan task", "content_id", contentID.String(), "error", err)
	}
}

// enqueueSearchIndex enqueues an async task to sync content to the Meilisearch
// search index. operation can be "index" (create/update) or "delete".
func (h *Handlers) enqueueSearchIndex(contentID pgtype.UUID, operation string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id":  contentID.String(),
		"entity_type": "content",

		"operation": operation,
	})
	if err != nil {
		slog.Warn("content: failed to marshal search index payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeSearchIndexSync, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeSearchIndexSync)),
	); err != nil {
		slog.Warn("content: failed to enqueue search index task", "content_id", contentID.String(), "error", err)
	}
}

// enqueueScholarshipSyncFromGuide enqueues the scholarship:sync_from_guide Asynq
// task when a guide is in the 'scholarship' category. It is a no-op for other
// content types and is idempotent by ( content_id). Fire-and-forget.
func (h *Handlers) enqueueScholarshipSyncFromGuide(contentID pgtype.UUID, guideCategory *string) {
	if h.AsynqClient == nil {
		return
	}

	if guideCategory == nil || *guideCategory != "scholarship" {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal scholarship sync payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeScholarshipSyncFromGuide, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeScholarshipSyncFromGuide)),
	); err != nil {
		slog.Warn("content: failed to enqueue scholarship sync task", "content_id", contentID.String(), "error", err)
	}
}

// enqueueModerationScan enqueues an async content moderation scan task.
// Fire-and-forget: errors are logged but never propagated to the caller.
func (h *Handlers) enqueueModerationScan(entityType string, entityID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"entity_type": entityType,
		"entity_id":   entityID.String(),
	})
	if err != nil {
		slog.Warn("content: failed to marshal moderation scan payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeModerationScan, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeModerationScan)),
	); err != nil {
		slog.Warn("content: failed to enqueue moderation scan task", "entity_id", entityID.String(), "error", err)
	}
}

// enqueueFeedFanOut enqueues content:fan_out_to_feeds tasks for published
// content (BL-005). Per Section 32, fan-out fires:
// 1. To the author's followers (always).
// 2. To each assigned space's members (once per space in content_spaces,
// Section 14). If the content has no content_spaces rows, falls back to
// the primary contents.space_id.
// 3. Bubble fan-out to parent spaces for each assigned space.
// // The worker decides push vs pull per space based on feed.fanout_threshold.
// Fire-and-forget per Rule 8.
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md.
func (h *Handlers) enqueueFeedFanOut(ctx context.Context, contentID, authorID, primarySpaceID pgtype.UUID, contentType string) {
	if h.AsynqClient == nil {
		return
	}

	// 1. Always fan out to the author's followers.
	h.enqueueFanOutTask(contentID, authorID, pgtype.UUID{}, contentType, "followers")

	// 2. Fan out to each assigned space via content_spaces (Section 14).
	// Falls back to the primary contents.space_id if no content_spaces rows
	// exist (backwards compatibility with pre-content_spaces content).
	spaceIDs := []pgtype.UUID{}

	if h.Service != nil && h.Service.read != nil {
		rows, err := h.Service.read.GetContentSpaces(ctx, contentID)
		if err != nil {
			slog.Warn("content: failed to fetch content_spaces for fan-out, using primary space_id", "content_id", contentID.String(), "error", err)
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
		h.enqueueFanOutTask(contentID, authorID, sid, contentType, "space")
		h.enqueueFanOutTask(contentID, authorID, sid, contentType, "bubble")
	}
}

// enqueueFanOutTask marshals and enqueues a single fan-out task.
func (h *Handlers) enqueueFanOutTask(contentID, authorID, spaceID pgtype.UUID, contentType, fanOutType string) {
	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),

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
		slog.Warn("content: failed to enqueue fan-out task", "content_id", contentID.String(), "fan_out_type", fanOutType, "error", err)
	}
}

// enqueueRSSRegenerate enqueues the rss:regenerate task for a tenant so the
// universal + blog RSS feeds reflect newly published content without waiting
// for the hourly cron. Fire-and-forget per Rule 8 (RSS-013).
// // Ref: japaconnect-engineering/sections/26-seo-performance.md
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md.
func (h *Handlers) enqueueRSSRegenerate() {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{})
	if err != nil {
		slog.Warn("content: failed to marshal rss:regenerate payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeRSSRegenerate, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeRSSRegenerate)),
	); err != nil {
		slog.Warn("content: failed to enqueue rss:regenerate task", "error", err)
	}
}

// dispatchMentionNotifications extracts @mentioned users from the content
// body (TipTap JSON) and fires a mention.content notification to each via
// the batch system (Section 35 — BL-002). Fire-and-forget per Rule 8:
// errors are logged but never propagated to the caller.
// // Self-mentions (mentioning the content author) are skipped.
// Privacy filtering (blocks, erased users, private-space membership) is
// applied via mentions.FilterMentions when checkers are wired (Section 15).
func (h *Handlers) dispatchMentionNotifications(ctx context.Context, authorID, contentID, spaceID pgtype.UUID, body string) {
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
			space, err := h.Service.read.GetSpaceByID(ctx,
				spaceID,
			)
			if err == nil && space.Visibility != "public" {
				isPrivateSpace = true
			}
		}

		mentioned, err = mentions.FilterMentions(
			ctx, mentioned, authorID, spaceID, isPrivateSpace,
			h.mentionCheckers, h.mentionCheckers, h.mentionCheckers,
		)
		if err != nil {
			// FilterMentions logs individual checker errors internally and
			// never returns a fatal error. This branch is retained for
			// forward-compatibility but is not expected to fire.
			slog.Warn("content: FilterMentions returned unexpected error", "content_id", contentID.String(), "error", err)
		}
	}

	for _, m := range mentioned {
		if m.EntityType != "user" || !m.ID.Valid || m.ID == authorID {
			continue
		}

		if err := h.notifier.DispatchNotification(
			ctx, m.ID,
			"mention.content", "",
			authorID,
			"content", contentID,
			"", "", "",
		); err != nil {
			slog.Warn("content: failed to dispatch mention.content notification", "user_id", m.ID.String(), "error", err)
		}
	}
}

// enqueueLinkPreview enqueues an async link preview fetch task for a URL
// found in a content block. Fire-and-forget.
func (h *Handlers) enqueueLinkPreview(url, blockID string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"url":      url,
		"block_id": blockID,
	})
	if err != nil {
		slog.Warn("content: failed to marshal link preview payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeLinkPreviewFetch, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeLinkPreviewFetch)),
	); err != nil {
		slog.Warn("content: failed to enqueue link preview task", "url", url, "error", err)
	}
}

// hrefURLRegex extracts http(s) URLs from href attributes in HTML.
var hrefURLRegex = regexp.MustCompile(`(?i)href="(https?://[^"]+)"`)

// dataURLRegex extracts http(s) URLs from data-url attributes in HTML.
// Link preview nodes are stored as <div data-type="link-preview"
// data-url="https://..."> and use data-url, not href (BUG-004).
var dataURLRegex = regexp.MustCompile(`(?i)data-url="(https?://[^"]+)"`)

// titleWordSeparator normalises title text to whitespace-separated words,
// dropping punctuation so multi-word names like "New-York" split into two
// tokens and acronyms like "UoPeople" stay intact.
var titleWordSeparator = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// maxTitleNgramLength limits contiguous n-gram generation from the title.
// University and country/state names rarely exceed 20 words.
const maxTitleNgramLength = 20

// excludedOneGrams filters out 1-gram n-grams that are common English
// stop-words, generic institution words, or short words that are also
// country/state codes (e.g. "in", "at", "to") to avoid false keyword
// matches. The words are still retained inside longer n-grams, so
// multi-word names like "University of Lagos" continue to work.
var excludedOneGrams = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "this": {}, "that": {}, "these": {}, "those": {},
	"and": {}, "or": {}, "but": {}, "nor": {}, "so": {}, "yet": {}, "if": {},
	"about": {}, "above": {}, "across": {}, "after": {}, "against": {}, "along": {}, "among": {}, "around": {}, "at": {}, "before": {}, "behind": {}, "below": {}, "beneath": {}, "beside": {}, "between": {}, "beyond": {}, "by": {}, "down": {}, "during": {}, "except": {}, "for": {}, "from": {}, "in": {}, "inside": {}, "into": {}, "like": {}, "near": {}, "of": {}, "off": {}, "on": {}, "onto": {}, "out": {}, "outside": {}, "over": {}, "past": {}, "regarding": {}, "round": {}, "since": {}, "through": {}, "throughout": {}, "till": {}, "to": {}, "toward": {}, "towards": {}, "under": {}, "until": {}, "up": {}, "upon": {}, "with": {}, "within": {}, "without": {},
	"i": {}, "me": {}, "my": {}, "myself": {}, "we": {}, "us": {}, "our": {}, "ours": {}, "ourselves": {}, "you": {}, "your": {}, "yours": {}, "yourself": {}, "yourselves": {}, "he": {}, "him": {}, "his": {}, "himself": {}, "she": {}, "her": {}, "hers": {}, "herself": {}, "it": {}, "its": {}, "itself": {}, "they": {}, "them": {}, "their": {}, "theirs": {}, "themselves": {},
	"am": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {}, "have": {}, "has": {}, "had": {}, "do": {}, "does": {}, "did": {}, "will": {}, "would": {}, "shall": {}, "should": {}, "can": {}, "could": {}, "may": {}, "might": {}, "must": {}, "ought": {},
	"all": {}, "any": {}, "both": {}, "each": {}, "every": {}, "few": {}, "more": {}, "most": {}, "much": {}, "many": {}, "no": {}, "some": {}, "such": {}, "what": {}, "which": {}, "who": {}, "whom": {}, "whose": {}, "where": {}, "when": {}, "why": {}, "how": {}, "here": {}, "there": {}, "now": {}, "then": {}, "very": {}, "too": {}, "just": {}, "only": {}, "also": {}, "even": {}, "still": {}, "already": {},
	"university": {}, "college": {}, "school": {}, "institute": {}, "academy": {}, "polytechnic": {}, "state": {}, "city": {}, "country": {}, "community": {}, "national": {}, "international": {}, "federal": {}, "private": {}, "public": {}, "global": {}, "world": {},
}

// extractTitleKeywords converts a title into a set of lowercased contiguous
// n-grams (1 to maxTitleNgramLength words). Multi-word n-grams preserve
// stop-words so full names like "University of Lagos" match; only 1-gram
// stop-words are dropped, preventing false keyword hits for "in", "at", etc.
func extractTitleKeywords(title string) []string {
	if title == "" {
		return nil
	}

	normalised := strings.ToLower(titleWordSeparator.ReplaceAllString(title, " "))

	rawWords := strings.Fields(strings.TrimSpace(normalised))
	if len(rawWords) == 0 {
		return nil
	}

	words := make([]string, 0, len(rawWords))

	for _, w := range rawWords {
		if len(w) < 2 {
			continue
		}

		words = append(words, w)
	}

	if len(words) == 0 {
		return nil
	}

	maxN := maxTitleNgramLength
	if len(words) < maxN {
		maxN = len(words)
	}

	seen := make(map[string]struct{})
	keywords := make([]string, 0, maxN*len(words))

	for length := 1; length <= maxN; length++ {
		for start := 0; start+length <= len(words); start++ {
			phrase := strings.Join(words[start:start+length], " ")
			if length == 1 {
				if _, skip := excludedOneGrams[phrase]; skip {
					continue
				}
			}

			if _, ok := seen[phrase]; ok {
				continue
			}

			seen[phrase] = struct{}{}

			keywords = append(keywords, phrase)
		}
	}

	return keywords
}

// enqueueLinkPreviewsForBody extracts external http(s) URLs from the HTML
// body and enqueues a link preview fetch task for each. Fire-and-forget
// (Section 15 — external links get async OG/oEmbed scrape). Called after
// content create/update so previews are cached for editor display.
// BUG-004: also extracts URLs from data-url attributes used by link
// preview block nodes, not just href attributes on inline links.
func (h *Handlers) enqueueLinkPreviewsForBody(bodyHTML string, contentID pgtype.UUID) {
	if h.AsynqClient == nil || bodyHTML == "" {
		return
	}

	seen := make(map[string]bool)
	blockID := contentID.String()

	// Extract URLs from both href (inline links) and data-url (link preview
	// block nodes) attributes. Deduplicate across both sources.
	allMatches := [][]string{}
	allMatches = append(allMatches, hrefURLRegex.FindAllStringSubmatch(bodyHTML, -1)...)
	allMatches = append(allMatches, dataURLRegex.FindAllStringSubmatch(bodyHTML, -1)...)

	for _, m := range allMatches {
		u := m[1]
		if seen[u] {
			continue
		}

		seen[u] = true

		h.enqueueLinkPreview(u, blockID)
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
		slog.Warn("content: failed to enqueue engagement update task", "entity_id", entityID.String(), "error", err)
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

	_, ok = h.requireTenant(w, r)
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
		VideoSourceID   *string         `json:"video_source_id"`
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
		// Journey-aware recommendation metadata.
		JourneyStage  *string `json:"journey_stage"`
		VisaRoute     *string `json:"visa_route"`
		SourceCountry *string `json:"source_country"`
		TargetCountry *string `json:"target_country"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
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

	// Validate enum values against the §14 defined sets. The contents table
	// has no DB-level CHECK constraint on these columns, so handler-level
	// validation is the only guard against invalid values being stored.
	if !IsValidContentType(input.ContentType) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_type",
			fmt.Sprintf("Invalid content_type: %q", input.ContentType))
		return
	}

	// §79: story creation through the generic contents endpoint still
	// requires the story-specific permission (POST /stories is gated by
	// story.create at route level — the composer path must not bypass it).
	if input.ContentType == TypeStory {
		allowed := false
		if resolver := rbac.FromContext(r.Context()); resolver != nil {
			allowed = resolver.IsSuperAdmin(r.Context(), user.UserID) ||
				resolver.HasPermission(r.Context(), user.UserID, "story.create")
		}
		if !allowed {
			errpkg.WriteError(w, http.StatusForbidden, "forbidden",
				"You do not have permission to create stories")
			return
		}
	}

	if !IsValidStatus(input.Status) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status",
			fmt.Sprintf("Invalid status: %q", input.Status))
		return
	}

	if !IsValidVisibility(input.Visibility) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_visibility",
			fmt.Sprintf("Invalid visibility: %q", input.Visibility))
		return
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
			minLength = h.settings.GetIntOr(r.Context(), "content.min_length", defaultMinLength)
		}

		if len(strings.TrimSpace(body)) < minLength {
			errpkg.WriteError(w, http.StatusBadRequest, "body_too_short",
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
		JourneyStage:    input.JourneyStage,
		VisaRoute:       input.VisaRoute,
		SourceCountry:   input.SourceCountry,
		TargetCountry:   input.TargetCountry,
	}
	if input.SeoIndexable != nil {
		createInput.SeoIndexable = *input.SeoIndexable
	}

	// §79: story moderators may create stories already published/scheduled;
	// everyone else is clamped to in_review by the service.
	if input.ContentType == TypeStory {
		createInput.SkipStoryModeration = isStoryModerator(r, user.UserID)
	}

	if input.SpaceID != nil && *input.SpaceID != "" {
		if err := createInput.SpaceID.Scan(*input.SpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}

		// BUG-003: enforce the space's posting_perm axis (Section 05 lines
		// 145-154). The check is skipped when no space checker is wired
		// (backward compatibility / tests).
		if h.spaceChecker != nil {
			allowed, err := h.spaceChecker.CanPostInSpace(r.Context(), createInput.SpaceID, user.UserID)
			if err != nil {
				errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to check space posting permission")
				return
			}

			if !allowed {
				errpkg.WriteError(w, http.StatusForbidden, "space_post_denied",
					"You do not have permission to post in this space")
				return
			}
		}
	}

	if input.OrganizationID != nil && *input.OrganizationID != "" {
		if err := createInput.OrganizationID.Scan(*input.OrganizationID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id format")
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

	if input.VideoSourceID != nil && *input.VideoSourceID != "" {
		if err := createInput.VideoSourceID.Scan(*input.VideoSourceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_video_source_id", "Invalid video_source_id format")
			return
		}
	}

	if input.PublishAt != nil && *input.PublishAt != "" {
		if err := createInput.PublishAt.Scan(*input.PublishAt); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_publish_at", "Invalid publish_at format")
			return
		}
		// Validate that publish_at is in the future.
		if createInput.PublishAt.Valid && createInput.PublishAt.Time.Before(time.Now()) {
			errpkg.WriteError(w, http.StatusBadRequest, "publish_at_in_past", "publish_at must be in the future")
			return
		}
	}

	// Spam pipeline check — multi-layer content safety (BE-034).
	if h.SpamPipeline != nil {
		clientIP := clientip.FromRequest(r)

		spamResult, err := h.SpamPipeline.Check(r.Context(), spam.SpamCheckRequest{
			Content:    input.Body,
			UserID:     user.UserID,
			IP:         clientIP,
			UserType:   "existing",
			EntityType: "post",
		})
		if err != nil {
			slog.Warn("spam check failed", "error", err)
		} else if spamResult.Blocked {
			errpkg.WriteError(w, http.StatusForbidden, "content_blocked", "Content blocked by safety filters")
			return
		} else if spamResult.Flagged {
			createInput.Status = StatusInReview // hold for moderation review
		}
	}

	content, err := h.Service.CreateContent(r.Context(), createInput)
	if err != nil {
		if errors.Is(err, ErrOrgPostForbidden) {
			errpkg.WriteError(w, http.StatusForbidden, "org_post_forbidden", "You are not authorized to post as this organization")
			return
		}

		handleServiceError(w, err)

		return
	}

	// Record the creation for the spam rate-limit layer (BUG-004). Without
	// this the jc:spam:rate:{tenant}:{user} sorted set is never populated and
	// the rate_limit scoring layer always returns 0.
	if h.SpamPipeline != nil {
		h.SpamPipeline.RecordContentCreation(r.Context(), user.UserID)
	}

	// Auto-assign content to primary spaces (country/state/city and public
	// universities) based on keyword, full name, or university initials in the
	// title. Runs regardless of whether the user selected a space; when a
	// space is already selected it stays primary and matched spaces are added
	// as additional. The most specific match becomes the primary otherwise.
	if h.spaceMatcher != nil {
		content.SpaceID = h.autoAssignSpaces(r.Context(), content.ID, content.SpaceID, input.Title)
	}

	// Enqueue async embedding generation — fire-and-forget.
	h.enqueueContentEmbedding(content.ID)

	// Enqueue search index sync — fire-and-forget.
	h.enqueueSearchIndex(content.ID, "index")

	// Enqueue moderation scan — fire-and-forget.
	h.enqueueModerationScan("content", content.ID)

	// §73: sync published scholarship guides to the scholarship finder
	// projection. Fire-and-forget; the worker is a no-op for other categories.
	h.enqueueScholarshipSyncFromGuide(content.ID, content.GuideCategory)

	// Dispatch @mention notifications — fire-and-forget (BL-002).
	h.dispatchMentionNotifications(r.Context(), user.UserID, content.ID, content.SpaceID, content.Body)

	// Enqueue feed fan-out for published content (BL-005). Scheduled posts
	// are fanned out by the scheduled-publish handler when they go live.
	// When content is created directly as published (bypassing the dedicated
	// POST /content/{id}/publish flow), we must also: (1) ensure a
	// content_scores row exists — feed queries INNER JOIN content_scores so
	// a missing row makes the content invisible (§32); (2) enqueue the
	// affiliate link scan (§55); (3) emit the content.published webhook
	// (the store only emits content.created on insert; content.published is
	// emitted by UpdateContentStatus, which the direct-publish path skips).
	if content.Status == StatusPublished {
		if err := h.Service.EnsureContentScore(r.Context(), content.ID); err != nil {
			slog.Warn("content: failed to ensure content_scores row on direct publish", "content_id", content.ID.String(), "error", err)
		}

		h.enqueueAffiliateLinkScan(content.ID)
		h.enqueueFeedFanOut(r.Context(), content.ID, user.UserID, content.SpaceID, content.ContentType)
		// RSS-013: regenerate RSS feeds so new content appears without
		// waiting for the hourly cron.
		h.enqueueRSSRegenerate()
	}

	// Enqueue link preview fetches for external URLs in the body — fire-and-forget
	// (Section 15 — external links get async OG/oEmbed scrape, BUG-003).
	h.enqueueLinkPreviewsForBody(input.BodyHtml, content.ID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": content})
}

// GetContent handles GET /api/v1/content/{slug}.
func (h *Handlers) GetContent(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	// Enriched detail row (author/space/tags/images/series) so the reading
	// page gets the same shape GET /blog/{slug} returns. Ref: §14.
	content, err := h.Service.GetContentDetailBySlug(r.Context(), slug)
	if err != nil {
		// Try slug redirect before returning not found.
		if newSlug, found := h.Service.ResolveSlugRedirect(r.Context(), EntityContent, slug); found {
			content, err = h.Service.GetContentDetailBySlug(r.Context(), newSlug)
		}

		if err != nil {
			handleServiceError(w, err)
			return
		}
	}

	// Visibility/status gate (§5.3 RBAC / §14 public content SSR):
	// - Published public content is visible to everyone (authed or not).
	// - Published members-only content requires an authenticated user.
	// - Published private content requires the author or a user holding
	// content.edit_any (admins/moderators).
	// - Non-published content (draft, scheduled, archived, in_review) is
	// only visible to the author or a user holding content.edit_any.
	user := auth.FromContext(r.Context())
	if content.Status != StatusPublished {
		if user == nil || !canModifyContent(r, user.UserID, content.AuthorID, "content.edit_any") {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
			return
		}
	} else {
		switch content.Visibility {
		case VisibilityPublic:
			// always allowed
		case VisibilityMembers:
			if user == nil {
				errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
				return
			}
		case VisibilityPrivate:
			if user == nil || !canModifyContent(r, user.UserID, content.AuthorID, "content.edit_any") {
				errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
				return
			}
		default:
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
			return
		}
	}

	// Resolve template literals in the content title, body, and meta fields.
	// This is best-effort: failures are logged but do not fail the request.
	rc := buildRenderContext(r, "")

	if err := h.Service.ResolveContentFields(r.Context(), &content, rc); err != nil {
		slog.Warn("content: failed to resolve content literals", "error", err, "content_id", content.ID.String())
	}

	// View tracking is handled by the dedicated POST /content/{id}/view
	// endpoint (§14 — rate-limited, once per user/session per hour with
	// DB-level dedup via user_content_views PK). The GET handler must NOT
	// increment view counts — doing so inflates counts with no dedup on
	// every page load and double-counts when the frontend also calls /view.

	// GDPR / ADR-029: redact mentions of erased users in body_html and
	// expose deleted_user_ids so the frontend can render @[deleted user]
	// when re-rendering from TipTap JSON (Section 15). Best-effort: a
	// failure logs a warning but does not fail the request.
	var erasedIDs []string

	if h.mentionCheckers != nil {
		redactedHTML, ids, redErr := mentions.RedactErasedMentionsInBody(
			r.Context(),
			[]byte(content.Body),
			content.BodyHtml,

			h.mentionCheckers,
		)
		if redErr != nil {
			slog.Warn("content: RedactErasedMentionsInBody failed", "content_id", content.ID.String(), "error", redErr)
		} else {
			content.BodyHtml = redactedHTML
			erasedIDs = ids
		}
	}

	// Enriched response DTO — mirrors GET /blog/{slug} (author name, avatar,
	// space, tags, excerpt, featured image URL, series navigation) so the
	// shared /content/[slug] page can render every CPT single the same way.
	dto := map[string]any{
		"id":                content.ID,
		"author_id":         content.AuthorID,
		"space_id":          content.SpaceID,
		"organization_id":   content.OrganizationID,
		"content_type":      content.ContentType,
		"title":             content.Title,
		"slug":              content.Slug,
		"body":              content.Body,
		"body_html":         content.BodyHtml,
		"content_hash":      content.ContentHash,
		"geo_space_id":      content.GeoSpaceID,
		"header_image_id":   content.HeaderImageID,
		"featured_image_id": content.FeaturedImageID,
		"canonical_url":     content.CanonicalUrl,
		"meta_title":        content.MetaTitle,
		"meta_description":  content.MetaDescription,
		"og_image_url":      content.OgImageUrl,
		"seo_indexable":     content.SeoIndexable,
		"status":            content.Status,
		"visibility":        content.Visibility,
		"is_locked":         content.IsLocked,
		"is_pinned":         content.IsPinned,
		"read_time_minutes": content.ReadTimeMinutes,
		"view_count":        content.ViewCount,
		"comment_count":     content.CommentCount,
		"share_count":       content.ShareCount,
		"reshare_count":     content.ReshareCount,
		"publish_at":        content.PublishAt,
		"published_at":      content.PublishedAt,
		"created_at":        content.CreatedAt,
		"updated_at":        content.UpdatedAt,
		"guide_category":    content.GuideCategory,
		"journey_stage":     content.JourneyStage,
		"visa_route":        content.VisaRoute,
		"source_country":    content.SourceCountry,
		"target_country":    content.TargetCountry,
		"video_source_id":   content.VideoSourceID,
		"author_name":       content.AuthorName,
		"author_username":   content.AuthorUsername,
		"author_avatar_url": content.AuthorAvatarUrl,
		"space_name":        content.SpaceName,
		"space_slug":        content.SpaceSlug,
		"excerpt":           makeExcerpt(content.BodyHtml, 160),
	}

	// field_values is stored as JSONB — emit the decoded object, not the
	// base64 blob a raw []byte would serialize as.
	if len(content.FieldValues) > 0 && json.Valid(content.FieldValues) {
		dto["field_values"] = json.RawMessage(content.FieldValues)
	} else {
		dto["field_values"] = map[string]any{}
	}

	// Image URLs resolved from storage refs (same rule as the feed mapper).
	if content.FeaturedStorageBucket != nil && content.FeaturedStoragePath != nil {
		dto["featured_image_url"] = media.MediaURL(*content.FeaturedStorageBucket, *content.FeaturedStoragePath, media.ImageProxyOpts{})
	}

	if content.HeaderStorageBucket != nil && content.HeaderStoragePath != nil {
		dto["header_image_url"] = media.MediaURL(*content.HeaderStorageBucket, *content.HeaderStoragePath, media.ImageProxyOpts{})
	}

	// Tags — split the CSV string into an array.
	if content.TagsCsv != "" {
		dto["tags"] = strings.Split(content.TagsCsv, ",")
	} else {
		dto["tags"] = []string{}
	}

	// Series navigation (guides and other CPTs can join blog series).
	if content.SeriesID.Valid {
		dto["series_id"] = content.SeriesID
		if content.SeriesTitle != nil {
			dto["series_title"] = *content.SeriesTitle
		}

		if content.SeriesSlug != nil {
			dto["series_slug"] = *content.SeriesSlug
		}

		if content.SeriesPart != nil {
			dto["series_part"] = *content.SeriesPart
		}

		dto["series_total"] = content.SeriesTotal
		if content.SeriesPrevSlug != "" {
			dto["series_prev_slug"] = content.SeriesPrevSlug
		}

		if content.SeriesNextSlug != "" {
			dto["series_next_slug"] = content.SeriesNextSlug
		}
	}

	// Source attribution for content promoted from the intelligence spine.
	if content.SourceID.Valid {
		src := map[string]any{
			"id":          content.SourceID.String(),
			"name":        derefString(content.SourceName),
			"slug":        derefString(content.SourceSlug),
			"logo_url":    content.SourceLogoUrl,
			"source_url":  derefString(content.SourceSourceUrl),
			"trust_level": 0,
		}
		if content.SourceTrustLevel != nil {
			src["trust_level"] = *content.SourceTrustLevel
		}
		dto["source"] = src
	}

	// §14 guide amendment: resolve the admin-configured category label so the
	// public page can render "Visa Guide" instead of the raw `visa` slug —
	// GET /content-types is auth-required, so guests cannot map it themselves.
	if content.ContentType == TypeGuide && content.GuideCategory != nil && h.guideCategoryLister != nil {
		if cats, catErr := h.guideCategoryLister.ListGuideCategories(r.Context()); catErr == nil {
			for _, cat := range cats {
				if cat.Slug == *content.GuideCategory {
					dto["guide_category_label"] = cat.Label

					break
				}
			}
		}
	}

	resp := map[string]any{"data": dto}

	if len(erasedIDs) > 0 {
		resp["deleted_user_ids"] = erasedIDs
	}

	// JSON-LD structured data (Section 14 / Section 26). Resolve the
	// admin-managed template matching the content's own content_type,
	// interpolating the content's field values. Best-effort: a missing
	// template or resolution error degrades to no json_ld field. Emitted as
	// a sibling of `data` for frontend consumption.
	if jsonLd, jErr := h.Service.ResolveJSONLD(r.Context(), content.ContentType, contentJSONLDFields(content)); jErr == nil && len(jsonLd) > 0 {
		resp["json_ld"] = json.RawMessage(jsonLd)
	} else if jErr != nil {
		slog.Warn("content: JSON-LD template resolution failed", "slug", slug, "content_type", content.ContentType, "error", jErr)
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// contentJSONLDFields builds the {{field_key}} → value map for interpolating
// a JSON-LD template from a generic content row. The keys cover the common
// placeholders used across Article, QAPage, Review, FAQPage, and JobPosting
// templates; unused keys are simply ignored by the interpolator.
func contentJSONLDFields(c gen.GetContentDetailBySlugRow) map[string]string {
	desc := ""
	if c.MetaDescription != nil {
		desc = *c.MetaDescription
	}

	canonical := ""
	if c.CanonicalUrl != nil {
		canonical = *c.CanonicalUrl
	}

	coverImageURL := ""
	if c.FeaturedStorageBucket != nil && c.FeaturedStoragePath != nil {
		coverImageURL = media.MediaURL(*c.FeaturedStorageBucket, *c.FeaturedStoragePath, media.ImageProxyOpts{})
	}

	return map[string]string{
		"title":           c.Title,
		"description":     desc,
		"body":            c.Body,
		"canonical_url":   canonical,
		"published_at":    tsFormat(c.PublishedAt),
		"updated_at":      tsFormat(c.UpdatedAt),
		"created_at":      tsFormat(c.CreatedAt),
		"cover_image_url": coverImageURL,
		"author_name":     c.AuthorName,
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
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)

	var cursor pgtype.Timestamptz

	var cursorID pgtype.UUID

	if cs := r.URL.Query().Get("cursor"); cs != "" {
		// Composite cursor: RFC3339 timestamp optionally followed by a '|' and the
		// content UUID. The id part is required to paginate correctly when many
		// rows share the same created_at timestamp.
		parts := strings.SplitN(cs, "|", 2)
		ts, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			ts, err = time.Parse(time.RFC3339, parts[0])
		}

		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cursor", "Invalid cursor timestamp")
			return
		}

		cursor = pgtype.Timestamptz{Time: ts.UTC(), Valid: true}

		if len(parts) == 2 {
			if err := cursorID.Scan(parts[1]); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_cursor_id", "Invalid cursor id")
				return
			}
		}
	}

	q := r.URL.Query().Get("q")

	contentType := r.URL.Query().Get("type")
	guideCategory := r.URL.Query().Get("guide_category")
	spaceIDStr := r.URL.Query().Get("space_id")

	var contents any

	var err error

	filter := visibility.VisibilityFilterFromRequest(r)

	// §14 generic featured list: ?featured=1 returns rows from the shared
	// content_featured junction (ordered by featured_order), optionally
	// filtered by type. Powers the Featured tabs on /guides, /stories, /forum.
	featured := r.URL.Query().Get("featured") == "1" || r.URL.Query().Get("featured") == "true"

	var offset int32

	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, perr := strconv.ParseInt(o, 10, 32); perr == nil && n > 0 {
			offset = int32(n)
		}
	}

	if featured {
		contents, err = h.Service.ListFeaturedContentsByType(r.Context(), contentType, offset, limit, filter)
	} else if spaceIDStr != "" {
		var spaceID pgtype.UUID
		if err := spaceID.Scan(spaceIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}

		contents, err = h.Service.ListContentsBySpace(r.Context(), spaceID, cursor, limit, filter)
	} else if contentType != "" {
		// §14: ?sort=popular orders by view_count (offset pagination);
		// ?sort=roadmap ranks by the resolved VisitorContext journey signals
		// (works for guests via the jc_journey_token cookie). Powers the
		// Popular and Roadmap tabs on /guides and other per-type hubs.
		sort := r.URL.Query().Get("sort")
		if sort != "popular" && sort != "roadmap" {
			sort = ""
		}
		vctx := visitor.FromContext(r.Context())
		contents, err = h.Service.ListContentsByType(r.Context(), contentType, guideCategory, cursor, cursorID, offset, limit, sort, vctx, filter)
	} else if q != "" && h.Service.HasSearcher() {
		// Use Meilisearch for full-text search (typo tolerance, ranking)
		// instead of ILIKE. Falls back to ILIKE on Meilisearch failure.
		results, searchErr := h.Service.SearchContents(r.Context(), q, limit, filter)
		if searchErr != nil {
			slog.Warn("content: Meilisearch search failed, falling back to ILIKE", "error", searchErr, "query", q)
			contents, err = h.Service.ListContents(r.Context(), q, cursor, limit, filter)
		} else {
			contents = results
		}
	} else {
		contents, err = h.Service.ListContents(r.Context(), q, cursor, limit, filter)
	}

	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Resolve template literals in list items best-effort.
	rc := buildRenderContext(r, "")

	if resErr := h.Service.ResolveContentFields(r.Context(), contents, rc); resErr != nil {
		slog.Warn("content: failed to resolve list literals", "error", resErr)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": contents})
}

// UpdateContent handles PATCH /api/v1/content/{id}.
func (h *Handlers) UpdateContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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
		VideoSourceID   *string         `json:"video_source_id"`
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
		IsLocked        bool            `json:"is_locked"`
		IsPinned        bool            `json:"is_pinned"`
		GuideCategory   *string         `json:"guide_category"`
		// Journey-aware recommendation metadata.
		JourneyStage  *string `json:"journey_stage"`
		VisaRoute     *string `json:"visa_route"`
		SourceCountry *string `json:"source_country"`
		TargetCountry *string `json:"target_country"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Validate enum values when provided (§14). The contents table has no
	// DB-level CHECK constraint on these columns, so handler-level validation
	// is the only guard against invalid values being stored. Empty values are
	// allowed here because UpdateContent only updates fields that are set.
	if input.Status != "" && !IsValidStatus(input.Status) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status",
			fmt.Sprintf("Invalid status: %q", input.Status))
		return
	}

	if input.Visibility != "" && !IsValidVisibility(input.Visibility) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_visibility",
			fmt.Sprintf("Invalid visibility: %q", input.Visibility))
		return
	}

	// Load the existing row before applying defaults. PATCH updates are
	// partial, so omitted top-level fields fall back to current values.
	prevContent, prevErr := h.Service.GetContent(r.Context(), id)
	if prevErr != nil {
		handleServiceError(w, prevErr)
		return
	}

	if input.Title == "" {
		input.Title = prevContent.Title
	}

	if input.Slug == "" {
		input.Slug = prevContent.Slug
	}

	if input.Status == "" {
		input.Status = prevContent.Status
	}

	if input.Visibility == "" {
		input.Visibility = prevContent.Visibility
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
			minLength = h.settings.GetIntOr(r.Context(), "content.min_length", defaultMinLength)
		}

		if len(strings.TrimSpace(body)) < minLength {
			errpkg.WriteError(w, http.StatusBadRequest, "body_too_short",
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
		JourneyStage:    input.JourneyStage,
		VisaRoute:       input.VisaRoute,
		SourceCountry:   input.SourceCountry,
		TargetCountry:   input.TargetCountry,
	}

	if input.SpaceID != nil && *input.SpaceID != "" {
		if err := updateInput.SpaceID.Scan(*input.SpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
	}

	// Compute content hash for duplicate detection (Rule 12).
	// Skip hashing for empty bodies — NULL content_hash doesn't
	// collide (unique index has WHERE content_hash IS NOT NULL).
	if strings.TrimSpace(body) != "" {
		videoSourceID := updateInput.VideoSourceID
		if !videoSourceID.Valid {
			videoSourceID = prevContent.VideoSourceID
		}

		var hash string
		if prevContent.ContentType == TypeVideo && videoSourceID.Valid {
			hash = ComputeContentHashWithDistinguisher(body, videoSourceID.String())
		} else {
			hash = ComputeContentHash(body)
		}
		updateInput.ContentHash = &hash
	}

	if input.GeoSpaceID != nil && *input.GeoSpaceID != "" {
		if err := updateInput.GeoSpaceID.Scan(*input.GeoSpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_geo_space_id", "Invalid geo_space_id format")
			return
		}
	}

	if input.OrganizationID != nil && *input.OrganizationID != "" {
		if err := updateInput.OrganizationID.Scan(*input.OrganizationID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id format")
			return
		}
	}

	if input.HeaderImageID != nil && *input.HeaderImageID != "" {
		if err := updateInput.HeaderImageID.Scan(*input.HeaderImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_header_image_id", "Invalid header_image_id format")
			return
		}
	}

	if input.FeaturedImageID != nil && *input.FeaturedImageID != "" {
		if err := updateInput.FeaturedImageID.Scan(*input.FeaturedImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_featured_image_id", "Invalid featured_image_id format")
			return
		}
	}

	if input.VideoSourceID != nil && *input.VideoSourceID != "" {
		if err := updateInput.VideoSourceID.Scan(*input.VideoSourceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_video_source_id", "Invalid video_source_id format")
			return
		}
	}

	if input.PublishAt != nil && *input.PublishAt != "" {
		if err := updateInput.PublishAt.Scan(*input.PublishAt); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_publish_at", "Invalid publish_at format")
			return
		}
		// Validate that publish_at is in the future when setting a new schedule.
		if updateInput.PublishAt.Valid && updateInput.PublishAt.Time.Before(time.Now()) {
			errpkg.WriteError(w, http.StatusBadRequest, "publish_at_in_past", "publish_at must be in the future")
			return
		}
	}

	// BUG-001: enforce author-or-admin before allowing edits. Type-specific
	// moderator nodes (story.edit_any for §79, comparison.edit_any for §30)
	// are also honoured so those moderators can use the unified composer.
	if !canModifyContent(r, user.UserID, prevContent.AuthorID, "content.edit_any") {
		letCanEdit := false
		if resolver := rbac.FromContext(r.Context()); resolver != nil {
			switch prevContent.ContentType {
			case "story":
				letCanEdit = resolver.HasPermission(r.Context(), user.UserID, "story.edit_any")
			case "comparison":
				letCanEdit = resolver.HasPermission(r.Context(), user.UserID, "comparison.edit_any")
			}
		}
		if !letCanEdit {
			errpkg.WriteError(w, http.StatusForbidden, "not_author", "You can only edit your own content")
			return
		}
	}

	// Super admins and content.edit_any holders bypass the edit-window
	// restriction even on their own content, and also bypass the
	// per-space membership gate when changing the primary space.
	bypassEditWindow := false
	bypassSpaceCheck := false

	if resolver := rbac.FromContext(r.Context()); resolver != nil {
		bypassEditWindow = resolver.IsSuperAdmin(r.Context(), user.UserID) ||
			resolver.HasPermission(r.Context(), user.UserID, "content.edit_any")
		bypassSpaceCheck = bypassEditWindow
	}

	updateInput.BypassSpaceCheck = bypassSpaceCheck
	// §79: story moderators publish/schedule directly; other callers are
	// clamped to in_review by the service.
	updateInput.SkipStoryModeration = isStoryModerator(r, user.UserID)

	content, err := h.Service.UpdateContent(r.Context(), id, user.UserID, updateInput, bypassEditWindow)
	if err != nil {
		if errors.Is(err, ErrOrgPostForbidden) {
			errpkg.WriteError(w, http.StatusForbidden, "org_post_forbidden", "You are not authorized to edit as this organization")
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
		worker.PurgeRAGEntity(r.Context(), "content", id)
	} else {
		h.enqueueContentEmbedding(id)
	}

	// Enqueue search index sync — fire-and-forget.
	h.enqueueSearchIndex(id, "index")

	// Enqueue moderation scan — fire-and-forget.
	h.enqueueModerationScan("content", id)

	// §73: keep the scholarship finder projection in sync with the published
	// scholarship guide. Fire-and-forget; the worker is a no-op for other
	// categories and for unpublished content.
	h.enqueueScholarshipSyncFromGuide(id, content.GuideCategory)

	// Dispatch @mention notifications for newly added mentions — fire-and-forget
	// (BUG-006). The notification batch system deduplicates by (recipient, type,
	// subject) so mentions that were already notified on create won't produce
	// duplicate notifications — the existing batch window is extended instead.
	h.dispatchMentionNotifications(r.Context(), user.UserID, content.ID, content.SpaceID, content.Body)

	// Enqueue feed fan-out when content transitions to published (BL-005).
	// This covers the draft→published path that CreateContent doesn't handle.
	if prevErr == nil && prevContent.Status != StatusPublished && content.Status == StatusPublished {
		h.enqueueFeedFanOut(r.Context(), id, content.AuthorID, content.SpaceID, content.ContentType)
		// RSS-013: regenerate RSS feeds on draft→published transition.
		h.enqueueRSSRegenerate()
	}

	// Enqueue link preview fetches for external URLs in the body — fire-and-forget
	// (Section 15 — external links get async OG/oEmbed scrape, BUG-003).
	h.enqueueLinkPreviewsForBody(input.BodyHtml, id)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": content})
}

// DeleteContent handles DELETE /api/v1/content/{id}.
func (h *Handlers) DeleteContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// BUG-002: verify authorship before soft-deleting.
	content, err := h.Service.GetContent(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !canModifyContent(r, user.UserID, content.AuthorID, "content.delete_any") {
		errpkg.WriteError(w, http.StatusForbidden, "not_author", "You can only delete your own content")
		return
	}

	if err := h.Service.DeleteContent(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue search index deletion — fire-and-forget.
	h.enqueueSearchIndex(id, "delete")

	// BUG-002 fix: purge RAG embeddings on content delete (ADR-069 §2:
	// "Content is unpublished or deleted — deletes its content_embeddings
	// rows"). Fire-and-forget — errors are logged, never propagated.
	worker.PurgeRAGEntity(r.Context(), "content", id)

	// §73: a deleted guide may still have a scholarships projection. The worker
	// will see the content is gone and remove the row.
	h.enqueueScholarshipSyncFromGuide(id, content.GuideCategory)

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
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

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// Capture previous status to detect draft→published transitions for fan-out.
	prevContent, prevErr := h.Service.GetContent(r.Context(), id)
	if prevErr != nil {
		handleServiceError(w, prevErr)
		return
	}

	// BUG-003: only the author, super-admins, or content.edit_any holders
	// may publish this content.
	if !canModifyContent(r, user.UserID, prevContent.AuthorID, "content.edit_any") {
		errpkg.WriteError(w, http.StatusForbidden, "not_author", "You can only publish your own content")
		return
	}

	// §79: author-facing publish of a story lands in in_review (moderation
	// queue); story moderators publish directly.
	if prevContent.ContentType == TypeStory && !isStoryModerator(r, user.UserID) {
		updated, err := h.Service.UpdateContentStatus(r.Context(), id, StatusInReview)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
		return
	}

	updated, err := h.Service.UpdateContentStatus(r.Context(), id, StatusPublished)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Auto-assign spaces on the first publish (e.g. draft→published) using the
	// content title, preserving any previously selected primary space.
	// Ref: sections/14-content-type-system.md (Content Auto-Assignment to Spaces)
	if h.spaceMatcher != nil && prevContent.Status != StatusPublished {
		updated.SpaceID = h.autoAssignSpaces(r.Context(), updated.ID, updated.SpaceID, updated.Title)
	}

	// AUDIT-BE-TASK-001: enqueue the affiliate link scan task on publish
	// (Section 55 — fire-and-forget, low tier). The worker handler reads
	// the affiliate.scan_on_publish system setting; if false, it exits early.
	h.enqueueAffiliateLinkScan(id)

	// Enqueue feed fan-out when content transitions to published (BL-005).
	// This covers the draft→published path that CreateContent doesn't handle.
	if prevErr == nil && prevContent.Status != StatusPublished {
		h.enqueueFeedFanOut(r.Context(), id, updated.AuthorID, updated.SpaceID, updated.ContentType)
		// RSS-013: regenerate RSS feeds on explicit publish.
		h.enqueueRSSRegenerate()
	}

	// §73: a published scholarship guide must be visible in the finder.
	h.enqueueScholarshipSyncFromGuide(id, updated.GuideCategory)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// ArchiveContent handles POST /api/v1/content/{id}/archive.
func (h *Handlers) ArchiveContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	_ = user
	_, // used for RBAC audit trail

		ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// BUG-003: only the author, super-admins, or content.edit_any holders
	// may archive this content.
	prevContent, err := h.Service.GetContent(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !canModifyContent(r, user.UserID, prevContent.AuthorID, "content.edit_any") {
		errpkg.WriteError(w, http.StatusForbidden, "not_author", "You can only archive your own content")
		return
	}

	updated, err := h.Service.UpdateContentStatus(r.Context(), id, StatusArchived)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-002 fix: purge RAG embeddings on archive (ADR-069 §2: content that
	// is no longer published must not be retrievable in RAG). Fire-and-forget.
	worker.PurgeRAGEntity(r.Context(), "content", id)

	// §73: archived guides must not appear in the finder.
	h.enqueueScholarshipSyncFromGuide(id, updated.GuideCategory)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// ScheduleContent handles POST /api/v1/content/{id}/schedule. Sets
// status='scheduled' and publish_at, then enqueues the scheduled_publish
// Asynq task (matching internal/worker's registered handler) to fire at
// publish_at.
func (h *Handlers) ScheduleContent(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		PublishAt time.Time `json:"publish_at"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PublishAt.IsZero() || input.PublishAt.Before(time.Now()) {
		errpkg.WriteError(w, http.StatusBadRequest, "publish_at_in_past", "publish_at must be in the future")
		return
	}

	// BUG-003: only the author, super-admins, or content.edit_any holders
	// may schedule this content.
	prevContent, err := h.Service.GetContent(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !canModifyContent(r, user.UserID, prevContent.AuthorID, "content.edit_any") {
		errpkg.WriteError(w, http.StatusForbidden, "not_author", "You can only schedule your own content")
		return
	}

	// §79: stories cannot be scheduled by their author — scheduling would
	// bypass the moderation queue when the scheduled_publish task fires.
	// Author-facing schedule submits for review; story moderators may
	// schedule directly.
	if prevContent.ContentType == TypeStory && !isStoryModerator(r, user.UserID) {
		updated, err := h.Service.UpdateContentStatus(r.Context(), id, StatusInReview)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		// §73: in_review stories are not published; keep finder in sync.
		h.enqueueScholarshipSyncFromGuide(id, updated.GuideCategory)

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
		return
	}

	updated, err := h.Service.ScheduleContent(r.Context(), id, input.PublishAt)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.enqueueScheduledPublish(id, input.PublishAt)

	// §73: scheduled content is not published, so remove any stale finder projection.
	h.enqueueScholarshipSyncFromGuide(id, updated.GuideCategory)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// RecordContentView handles POST /api/v1/content/{id}/view.
// Uses OptionalAuth so both authenticated and anonymous viewers are
// counted. For authenticated users, the user_id is recorded; for
// anonymous users, the session_id (or a fingerprint fallback) is used
// for deduplication. The country_code is resolved server-side from the
// client IP via the GeoIP middleware (Section 14 — view tracking).
func (h *Handlers) RecordContentView(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
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

	// Capture the referrer (traffic source) from the request body or the
	// standard Referer header. Optional and best-effort — Section 40 traffic
	// sources fall back to "Direct" when no referrer is recorded.
	var referrer string

	if r.Header.Get("Content-Type") == "application/json" {
		var payload struct {
			Referrer string `json:"referrer"`
		}

		_ = json.NewDecoder(r.Body).Decode(&payload)
		referrer = payload.Referrer
	}

	if referrer == "" {
		referrer = r.Header.Get("Referer")
	}

	var referrerPtr *string
	if referrer != "" {
		referrerPtr = &referrer
	}

	if err := h.Service.RecordContentViewWithDetails(r.Context(), id, userID, sessionID, countryCode, referrerPtr); err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async engagement count update — fire-and-forget.
	h.enqueueEngagementUpdate("content", id, "view", 1)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"recorded": true}})
}

// GetContentShareCount handles GET /api/v1/content/{id}/share-count.
func (h *Handlers) GetContentShareCount(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	count, err := h.Service.CountContentShares(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]int32{"share_count": count}})
}

// enqueueScheduledPublish enqueues the scheduled_publish Asynq task to fire
// at publishAt. Fire-and-forget: logs and returns on failure rather than
// failing the request — the content is already saved as 'scheduled'.
func (h *Handlers) enqueueScheduledPublish(contentID pgtype.UUID, publishAt time.Time) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
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
// Tenant-scoped and permission-gated (BUG-012).
func (h *Handlers) ListRevisions(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	revisions, err := h.Service.ListContentRevisions(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Defense-in-depth: confirm the content exists and the requester can see it
	// in this tenant (BUG-012 closes the tenant-scoping gap on the query itself).
	_, err = h.Service.GetContent(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": revisions})
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
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		SpaceID  string `json:"space_id"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.SpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_space_id", "space_id is required")
		return
	}

	var spaceID pgtype.UUID
	if err := spaceID.Scan(input.SpaceID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
		return
	}

	pin, err := h.Service.PinContent(r.Context(), spaceID, contentID, user.UserID, input.Position)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": pin})
}

// UnpinContent handles DELETE /api/v1/content/{id}/pin.
// The space_id is provided as a query parameter.
func (h *Handlers) UnpinContent(w http.ResponseWriter, r *http.Request) {
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	spaceIDStr := r.URL.Query().Get("space_id")
	if spaceIDStr == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_space_id", "space_id query parameter is required")
		return
	}

	var spaceID pgtype.UUID
	if err := spaceID.Scan(spaceIDStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
		return
	}

	if err := h.Service.UnpinContent(r.Context(), spaceID, contentID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
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

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		ReactionKey string `json:"reaction_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ReactionKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_reaction_key", "reaction_key is required")
		return
	}

	// §89 Reaction Set: achievement posts use a curated reaction set
	// configured via system_settings.achievement_reactions (JSONB array of
	// {emoji, label} pairs). When the content is an achievement, validate
	// the reaction_key (emoji) against the curated set. If the setting is
	// empty or unparseable, fall back to the spec defaults: 🎉 🔥 💪.
	if h.settings != nil {
		existing, err := h.Service.GetContent(r.Context(), contentID)
		if err == nil && existing.ContentType == TypeAchievement {
			if !isAllowedAchievementReaction(r.Context(), h.settings, input.ReactionKey) {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_reaction_key", "This reaction is not available for achievement posts")
				return
			}
		}
	}

	reaction, err := h.Service.AddReaction(r.Context(), EntityContent, contentID, user.UserID, input.ReactionKey)
	if err != nil {
		// ON CONFLICT DO NOTHING returns ErrNoRows when the user already has
		// this reaction — treat as idempotent success, not a 404.
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{

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
	// content_scores.reaction_count so feed cards show an accurate reaction count.
	// Fire-and-forget per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "reaction", 1)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": reaction})
}

// defaultAchievementReactions is the spec-defined curated reaction set for
// achievement posts (§89 Reaction Set), used as a fallback when the
// system_settings.achievement_reactions setting is empty or unparseable.
var defaultAchievementReactions = []string{"🎉", "🔥", "💪"}

// isAllowedAchievementReaction checks whether the given reaction_key (emoji)
// is in the curated achievement reaction set. The set is read from
// system_settings.achievement_reactions (JSONB array of {emoji, label}
// pairs); if the setting is empty or unparseable, the spec defaults
// (🎉 🔥 💪) are used. Ref: §89 Reaction Set.
func isAllowedAchievementReaction(ctx context.Context, settings *config.Settings, key string) bool {
	raw := settings.GetStringOr(ctx, "achievement_reactions", "")
	if raw != "" && raw != "[]" {
		var pairs []struct {
			Emoji string `json:"emoji"`
			Label string `json:"label"`
		}

		if err := json.Unmarshal([]byte(raw), &pairs); err == nil && len(pairs) > 0 {
			for _, p := range pairs {
				if p.Emoji == key {
					return true
				}
			}

			return false
		}
	}
	// Fall back to spec defaults when the setting is empty/unparseable.
	for _, k := range defaultAchievementReactions {
		if k == key {
			return true
		}
	}

	return false
}

// RemoveReaction handles DELETE /api/v1/content/{id}/reactions/{key}.
func (h *Handlers) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	reactionKey := chi.URLParam(r, "key")
	// Fall back to the query string for the singular /content/{id}/react
	// route (catalog line 260), which has no {key} path parameter.
	if reactionKey == "" {
		reactionKey = r.URL.Query().Get("key")
	}

	if reactionKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_key", "Reaction key is required")
		return
	}

	if err := h.Service.RemoveReaction(r.Context(), EntityContent, contentID, user.UserID, reactionKey); err != nil {
		handleServiceError(w, err)
		return
	}

	// Decrement the reaction_count to keep feed card counts accurate.
	// Fire-and-forget per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "reaction", -1)

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ListReactions handles GET /api/v1/content/{id}/reactions.
func (h *Handlers) ListReactions(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	counts, err := h.Service.CountReactionsByEntity(r.Context(), EntityContent, contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-FIX: a nil slice serialises as JSON null, which causes the
	// ReactionRail to skip initialising counts. Return an empty array so
	// guest visitors and posts without reactions see a consistent shape.
	if counts == nil {
		counts = []gen.CountReactionsByEntityRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": counts})
}

// ListUserReactions handles GET /api/v1/content/{id}/reactions/me.
// Returns the current user's reactions for a content item (used by the
// frontend to initialise the like button state on page load).
func (h *Handlers) ListUserReactions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	reactions, err := h.Service.GetUserReactions(r.Context(), EntityContent, contentID, user.UserID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": reactions})
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
	ID string `json:"id"`

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
		ID: t.ID.String(),

		ContentType: t.ContentType,
		Template:    body,
		IsActive:    t.IsActive,
		CreatedAt:   t.CreatedAt.Time.Format(time.RFC3339Nano),
		UpdatedAt:   t.UpdatedAt.Time.Format(time.RFC3339Nano),
	}
}

// ListJSONLDTemplates handles GET /api/v1/content/jsonld-templates.
func (h *Handlers) ListJSONLDTemplates(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")

	templates, err := h.Service.ListJSONLDTemplates(r.Context(), pgtype.Text{String: q, Valid: true})
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// UpsertJSONLDTemplate handles PUT /api/v1/content/jsonld-templates.
func (h *Handlers) UpsertJSONLDTemplate(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		ContentType string          `json:"content_type"`
		Template    json.RawMessage `json:"template"`
		IsActive    bool            `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ContentType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content_type", "content_type is required")
		return
	}

	tmpl, err := h.Service.UpsertJSONLDTemplate(r.Context(), input.ContentType, input.Template, input.IsActive)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toJSONLDResponse(tmpl)})
}

// DeleteJSONLDTemplate handles DELETE /api/v1/content/jsonld-templates/{id}.
// It removes a single JSON-LD template by its primary key, scoped to the
// caller's tenant (§21 missing CRUD leg, §26 RBAC CRUD completeness).
func (h *Handlers) DeleteJSONLDTemplate(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.DeleteJSONLDTemplate(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
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
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Folder   string `json:"folder"`
		FolderID string `json:"folder_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var folderID pgtype.UUID
	if input.FolderID != "" {
		if err := folderID.Scan(input.FolderID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_folder_id", "Invalid folder_id format")
			return
		}
		// Validate folder ownership (BL-013): the folder must belong to the
		// caller and tenant before associating a bookmark with it.
		if err := h.Service.ValidateFolderOwnership(r.Context(), folderID, user.UserID); err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "folder_not_found", "Bookmark folder not found")
			return
		}
	}

	bm, err := h.Service.CreateBookmark(r.Context(), user.UserID, EntityContent, contentID, input.Folder, folderID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Update content_scores.bookmark_count so feed cards show an accurate
	// bookmark count. Fire-and-forget per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "bookmark", 1)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": bm})
}

// RemoveBookmark handles DELETE /api/v1/content/{id}/bookmark.
func (h *Handlers) RemoveBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.RemoveBookmark(r.Context(), user.UserID, EntityContent, contentID); err != nil {
		handleServiceError(w, err)
		return
	}

	// Decrement the bookmark_count to keep feed card counts accurate.
	// Fire-and-forget per Rule 8. Ref: Section 32, Guide §21.
	h.enqueueEngagementUpdate("content", contentID, "bookmark", -1)

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// GetBookmarkStatus handles GET /api/v1/content/{id}/bookmark.
// Returns whether the authenticated user has bookmarked the content item.
func (h *Handlers) GetBookmarkStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	bookmarked, err := h.Service.IsBookmarked(r.Context(), user.UserID, EntityContent, contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"bookmarked": bookmarked}})
}

// MoveBookmark handles PATCH /api/v1/content/{id}/bookmark/folder.
// Moves an existing bookmark into a different folder (BL-003).
// Body: { "folder_id": "uuid" } — pass empty string or omit to unfile.
func (h *Handlers) MoveBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		FolderID string `json:"folder_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var folderID pgtype.UUID
	if input.FolderID != "" {
		if err := folderID.Scan(input.FolderID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_folder_id", "Invalid folder_id format")
			return
		}
		// Validate folder ownership (BL-013).
		if err := h.Service.ValidateFolderOwnership(r.Context(), folderID, user.UserID); err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "folder_not_found", "Bookmark folder not found")
			return
		}
	}

	if err := h.Service.SetBookmarkFolder(r.Context(), user.UserID, EntityContent, contentID, folderID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"folder_id": input.FolderID}})
}

// ---------------------------------------------------------------------------
// Content Tags Handlers (Task 12.10)
// ---------------------------------------------------------------------------

// ListContentTags handles GET /api/v1/content/{id}/tags.
func (h *Handlers) ListContentTags(w http.ResponseWriter, r *http.Request) {
	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	tags, err := h.Service.ListContentTags(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tags})
}

// AddContentTag handles POST /api/v1/content/{id}/tags.
// Accepts the spec-compliant body {tag_slugs: string[]} (Section 14) for bulk
// slug-based tagging, and the legacy {tag_id: string} for backward compatibility.
func (h *Handlers) AddContentTag(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// BUG-001: only the content author or content.edit_any holder may tag it.
	content, err := h.Service.GetContent(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !canModifyContent(r, user.UserID, content.AuthorID, "content.edit_any") {
		errpkg.WriteError(w, http.StatusForbidden, "not_author", "You can only tag your own content")
		return
	}

	var input struct {
		TagSlugs []string `json:"tag_slugs"`
		TagID    string   `json:"tag_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Resolve input to a list of tag IDs.
	var tagIDs []pgtype.UUID

	if len(input.TagSlugs) > 0 {
		if h.tagResolver == nil {
			errpkg.WriteError(w, http.StatusServiceUnavailable, "tag_resolver_unavailable", "Tag slug resolution is not configured")
			return
		}
		// Get-or-create: user-typed slugs that don't exist yet are auto-
		// created as tenant-scoped tags (Section 14 — composer flow).
		slugMap, err := h.tagResolver.GetOrCreateTagSlugs(r.Context(), input.TagSlugs)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_tag_slugs", "One or more tag slugs could not be resolved")
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
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag_id format")
			return
		}

		tagIDs = append(tagIDs, tagID)
	} else {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_tags", "tag_slugs or tag_id is required")
		return
	}

	if len(tagIDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "no_valid_tags", "No valid tags provided")
		return
	}

	maxTags := defaultMaxTags
	if h.settings != nil {
		maxTags = h.settings.GetIntOr(r.Context(), "content.max_tags", defaultMaxTags)
	}

	existing, err := h.Service.ListContentTags(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if len(existing)+len(tagIDs) > maxTags {
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_tags", "Content would exceed the maximum number of tags")
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]int{"added": added}})
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
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// BUG-001: only the content author or content.edit_any holder may untag it.
	content, err := h.Service.GetContent(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !canModifyContent(r, user.UserID, content.AuthorID, "content.edit_any") {
		errpkg.WriteError(w, http.StatusForbidden, "not_author", "You can only remove tags from your own content")
		return
	}

	tagID, err := parseUUIDParam(r, "tagId")
	if err != nil {
		// Not a valid UUID — try slug resolution if a resolver is wired.
		if h.tagResolver == nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag ID format")
			return
		}

		slug := chi.URLParam(r, "tagId")

		slugMap, rerr := h.tagResolver.ResolveTagSlugs(r.Context(), []string{slug})
		if rerr != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_tag_slug", "Tag slug could not be resolved")
			return
		}

		var found bool

		tagID, found = slugMap[slug]
		if !found {
			errpkg.WriteError(w, http.StatusNotFound, "tag_not_found", "Tag not found")
			return
		}
	}

	if err := h.Service.RemoveContentTag(r.Context(), contentID, tagID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Generic Featuring (Section 14 — content_featured junction, content.feature node)
// ---------------------------------------------------------------------------

// FeatureContent handles POST /api/v1/content/{id}/feature — the generic
// counterpart of POST /blog/{id}/feature, for any content_type.
func (h *Handlers) FeatureContent(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if _, err := h.Service.GetContent(r.Context(), contentID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
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

	featured, err := h.Service.FeatureContent(r.Context(), contentID, input.FeaturedOrder, featuredUntil)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": featured})
}

// UnfeatureContent handles DELETE /api/v1/content/{id}/feature.
func (h *Handlers) UnfeatureContent(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	if err := h.Service.UnfeatureContent(r.Context(), contentID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
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
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		Platform string `json:"platform"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	share, err := h.Service.ShareContent(r.Context(), contentID, user.UserID, input.Platform)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Enqueue async engagement count update — fire-and-forget.
	h.enqueueEngagementUpdate("content", contentID, "share", 1)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": share})
}

// GetRelatedContent handles GET /api/v1/content/{id}/related.
func (h *Handlers) GetRelatedContent(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	limit := parsePageSize(r)
	if limit > 12 {
		limit = 12
	}

	// Get the content's embedding from content_scores.
	score, err := h.Service.GetContentScore(r.Context(), contentID)
	if err != nil || score.RelevanceEmbedding == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	contents, err := h.Service.GetRelatedContent(r.Context(), contentID, *score.RelevanceEmbedding, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": contents})
}

// ListContentTypes handles GET /content-types — returns available content types.
// The `guide` type has `has_category: true` and a `categories` array listing
// the evergreen guide categories (Visa, Travel, Scholarship, etc.) loaded from
// custom_field_entity_types (value prefix 'guide_category:'). Admins can add
// new categories from the admin panel — no code change required.
// // Ref: japaconnect-engineering/sections/14-content-type-system.md.
func (h *Handlers) ListContentTypes(w http.ResponseWriter, r *http.Request) {
	_, _ = h.requireTenant(w, r)

	guideCategories := []map[string]string{}

	if h.guideCategoryLister != nil && false {
		cats, err := h.guideCategoryLister.ListGuideCategories(r.Context())
		if err == nil {
			for _, c := range cats {
				guideCategories = append(guideCategories, map[string]string{
					"Slug":  c.Slug,
					"Label": c.Label,
				})
			}
		}
	}

	types := make([]map[string]any, 0)

	for _, t := range ContentTypes() {
		entry := map[string]any{
			"value": t,
			"label": ContentTypeLabels[t],
		}
		if ContentTypeHasCategory(t) {
			entry["has_category"] = true
			entry["categories"] = guideCategories
		}

		types = append(types, entry)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// GetVisibilityOptions handles GET /api/v1/content/visibility-options —
// returns the valid visibility values for content (Section 14). The frontend
// uses this to populate visibility selectors instead of hardcoding the values
// (Rule 16).
func (h *Handlers) GetVisibilityOptions(w http.ResponseWriter, r *http.Request) {
	options := []map[string]string{
		{"value": VisibilityPublic, "label": "Public"},
		{"value": VisibilityMembers, "label": "Members"},
		{"value": VisibilityPrivate, "label": "Private"},
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": options})
}

// GetStatusOptions handles GET /api/v1/content/status-options — returns the
// valid status values for content (Section 14). The frontend uses this to
// populate status selectors instead of hardcoding the values (Rule 16).
// BUG-007/BUG-008: the admin blog pages hardcoded status dropdown options,
// missing scheduled/archived and diverging from §14's canonical set.
func (h *Handlers) GetStatusOptions(w http.ResponseWriter, r *http.Request) {
	options := []map[string]string{
		{"value": StatusDraft, "label": "Draft"},
		{"value": StatusInReview, "label": "In Review"},
		{"value": StatusScheduled, "label": "Scheduled"},
		{"value": StatusPublished, "label": "Published"},
		{"value": StatusArchived, "label": "Archived"},
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": options})
}

// GetContentSpaces handles GET /content/{id}/spaces — returns all space
// assignments for a content item (Section 14: content_spaces). Primary
// space is returned first.
func (h *Handlers) GetContentSpaces(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	// BUG-005: the author may view/edit spaces; content.edit_any holders may too.
	existing, err := h.Service.GetContent(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !canModifyContent(r, user.UserID, existing.AuthorID, "content.edit_any") {
		errpkg.WriteError(w, http.StatusForbidden, "not_author",
			"Only the author can view space assignments")
		return
	}

	rows, err := h.Service.GetContentSpaces(r.Context(), contentID)
	if err != nil {
		slog.Error("getting content spaces", "error", err, "content_id", contentID.String())
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error",
			"Failed to fetch space assignments")

		return
	}

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"space_id":   row.SpaceID.String(),
			"is_primary": row.IsPrimary,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
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

	_, ok = h.requireTenant(w, r)
	if !ok {
		return
	}

	contentID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ID format")
		return
	}

	var input struct {
		PrimarySpaceID     string   `json:"primary_space_id"`
		AdditionalSpaceIDs []string `json:"additional_space_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.PrimarySpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_primary_space",
			"A primary_space_id is required")
		return
	}

	var primarySpaceID pgtype.UUID
	if err := primarySpaceID.Scan(input.PrimarySpaceID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_primary_space_id",
			"Invalid primary_space_id format")
		return
	}

	// Verify the content exists and belongs to the caller's tenant.
	// BUG-005: the author may update spaces; content.edit_any holders may too.
	existing, err := h.Service.GetContent(r.Context(), contentID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if !canModifyContent(r, user.UserID, existing.AuthorID, "content.edit_any") {
		errpkg.WriteError(w, http.StatusForbidden, "not_author",
			"Only the author can update space assignments")
		return
	}

	var additionalSpaceIDs []pgtype.UUID

	for _, sidStr := range input.AdditionalSpaceIDs {
		var sid pgtype.UUID
		if err := sid.Scan(sidStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id",
				"Invalid space_id format: "+sidStr)
			return
		}

		additionalSpaceIDs = append(additionalSpaceIDs, sid)
	}

	bypassSpaceCheck := false
	if resolver := rbac.FromContext(r.Context()); resolver != nil {
		bypassSpaceCheck = resolver.IsSuperAdmin(r.Context(), user.UserID) ||
			resolver.HasPermission(r.Context(), user.UserID, "content.edit_any")
	}

	if err := h.Service.UpdateContentSpaces(r.Context(), contentID, user.UserID, primarySpaceID, additionalSpaceIDs, bypassSpaceCheck); err != nil {
		slog.Error("updating content spaces", "error", err, "content_id", contentID.String(), "user_id", user.UserID.String())

		switch {
		case errors.Is(err, ErrInvalidSpace):
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "invalid_space",
				"One or more space IDs do not belong to this tenant")
		case errors.Is(err, ErrNotSpaceMember):
			errpkg.WriteError(w, http.StatusForbidden, "not_space_member",
				"You must be a member of each assigned space")
		default:
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error",
				"Failed to update space assignments")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"updated": true},
	})
}
