// Package forum — HTTP handlers for the Forum Q&A / Solved system.
// // Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package forum

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
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
	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/seo"
	"github.com/jedafe/japa/internal/worker"
)

const defaultPageSize int32 = 20

// Abuse-control caps: TipTap bodies are capped at 200KB (create+update,
// 413 body_too_large on exceed); titles/slugs have length caps.
const (
	maxBodyBytes  = 200 * 1024
	maxTitleRunes = 200
	maxSlugRunes  = 200
	maxForumRunes = 10000
)

// rejectOversizedBody reports whether body exceeds the TipTap cap, writing
// the 413 response when it does.
func rejectOversizedBody(w http.ResponseWriter, body []byte) bool {
	if len(body) > maxBodyBytes {
		errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "body_too_large", "Body exceeds size limit")
		return true
	}

	return false
}

// rejectOverlongField writes a 400 when s exceeds max runes, naming field.
func rejectOverlongField(w http.ResponseWriter, field, s string, max int) bool {
	if len([]rune(s)) > max {
		errpkg.WriteError(w, http.StatusBadRequest, "field_too_long", field+" exceeds length limit")
		return true
	}

	return false
}

// JSONLDResolver resolves admin-managed JSON-LD templates (Section 14 /
// Section 26). The forum uses it to emit QAPage structured data from the
// active template, falling back to the hardcoded generator when no template
// exists.
type JSONLDResolver interface {
	ResolveJSONLD(ctx context.Context, contentType string, fields map[string]string) (json.RawMessage, error)
}

// MentionNotifier dispatches mention notifications via the batch system
// (Section 35 — mention.content/mention.comment consolidated into
// notification:dispatch_batch per Section 11). Fire-and-forget per Rule 8.
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

// RAGEntityIndexer is the subset of the RAG indexer that forum needs to
// index thread content into content_embeddings (§69/ADR-069). Implemented
// by rag.Service; nil disables RAG indexing (graceful degradation).
type RAGEntityIndexer interface {
	IndexEntity(ctx context.Context, entityType string, entityID pgtype.UUID, visibility string, spaceID, authorID pgtype.UUID, body, language string) error
	PurgeEntity(ctx context.Context, entityType string, entityID pgtype.UUID) error
}

// settingsReader is the subset of config.Settings needed by the forum handlers
// to resolve hot-reloadable system_settings (e.g. i18n.default_language for
// RAG indexing). Nil falls back to compiled-in defaults.
type settingsReader interface {
	GetStringOr(ctx context.Context, key, fallback string) string
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// ContentStore is the subset of the content service the forum handlers need
// to keep a backing contents row in sync with each forum thread. A nil value
// disables the sync (test-only, backward compat).
type ContentStore interface {
	CreateContent(ctx context.Context, input content.CreateContentInput) (gen.CreateContentRow, error)
	UpdateContent(ctx context.Context, id, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error)
	GetContent(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error)
	DeleteContent(ctx context.Context, id pgtype.UUID) error
	EnsureContentScore(ctx context.Context, contentID pgtype.UUID) error
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14 Global Template Literals). Implemented by *literals.Catalog;
// nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers holds dependencies for forum HTTP handlers.
type Handlers struct {
	Service         *Service
	jsonldResolver  JSONLDResolver
	content         ContentStore         // nil disables contents-row sync (test-only)
	notifier        MentionNotifier      // nil disables mention notifications (test-only)
	mentionCheckers *mentions.DBCheckers // nil disables erased-mention redaction
	ragIndexer      RAGEntityIndexer     // nil disables RAG indexing (§69)
	settings        settingsReader       // nil falls back to defaults
	asynqClient     *asynq.Client        // nil disables search index enqueue (test-only)
	literals        literalResolver      // nil disables literal resolution
}

// NewHandlers creates Handlers with the given Service and optional JSON-LD
// resolver. A nil resolver is acceptable — the forum falls back to its
// hardcoded QAPage generator.
func NewHandlers(service *Service, resolver JSONLDResolver) *Handlers {
	return &Handlers{Service: service, jsonldResolver: resolver}
}

// SetContentStore injects the content service so forum handlers can create a
// backing contents row for each new thread. Ref: .jeos/sections/14-content-type-system.md.
func (h *Handlers) SetContentStore(c ContentStore) { h.content = c }

// SetMentionNotifier wires the notification dispatcher for @mention
// notifications in forum threads and replies (Section 15 / ADR-029).
// nil disables mention notifications (backward compat / tests).
func (h *Handlers) SetMentionNotifier(n MentionNotifier) { h.notifier = n }

// SetMentionCheckers wires the DB-backed erased-user checker used to
// redact mentions of deleted users in thread/reply responses (Section 15 /
// ADR-029). nil disables redaction (backward compat / tests).
func (h *Handlers) SetMentionCheckers(c *mentions.DBCheckers) { h.mentionCheckers = c }

// SetRAGIndexer injects the RAG indexer for embedding forum threads into
// content_embeddings (§69/ADR-069 BUG-001 fix). nil disables RAG indexing.
func (h *Handlers) SetRAGIndexer(r RAGEntityIndexer) { h.ragIndexer = r }

// SetSettings injects the hot-reloadable settings reader for resolving
// system_settings (e.g. i18n.default_language for RAG indexing). nil falls
// back to compiled-in defaults.
func (h *Handlers) SetSettings(s settingsReader) { h.settings = s }

// sitemapCfg resolves §43 quality thresholds from system_settings
// (seo.sitemap.*), falling back to spec defaults — matching the sitemap gate.
func (h *Handlers) sitemapCfg(ctx context.Context) bots.SitemapQualityConfig {
	cfg := bots.DefaultSitemapQualityConfig()
	if h.settings != nil {
		cfg.ForumMinReplies = h.settings.GetIntOr(ctx, "seo.sitemap.forum_min_replies", cfg.ForumMinReplies)
		cfg.QuestionMinAnswers = h.settings.GetIntOr(ctx, "seo.sitemap.question_min_answers", cfg.QuestionMinAnswers)
	}
	return cfg
}

// SetAsynqClient injects the Asynq client so thread/tag CRUD can enqueue
// search:index_entity tasks (§12 stale-index fix). nil disables enqueue
// (test-only).
func (h *Handlers) SetAsynqClient(c *asynq.Client) { h.asynqClient = c }

// SetLiteralResolver injects the template-literal catalog so thread/reply
// titles and bodies render {{...}} placeholders (§14). nil disables.
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.literals = l }

// resolveLiterals resolves {{...}} placeholders on a thread/reply row or
// slice. Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.literals == nil {
		return
	}

	h.literals.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

// enqueueSearchIndex enqueues a Meilisearch re-index task for a forum entity
// (thread or tag). operation is "index" (create/update) or "delete".
// Fire-and-forget per Rule 8.
func (h *Handlers) enqueueSearchIndex(entityType, operation string, entityID pgtype.UUID) {
	if h.asynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"entity_id":   entityID.String(),
		"entity_type": entityType,

		"operation": operation,
	})
	if err != nil {
		slog.Warn("forum: failed to marshal search index payload", "error", err)
		return
	}

	if _, err := h.asynqClient.Enqueue(
		asynq.NewTask(worker.TypeSearchIndexSync, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeSearchIndexSync)),
	); err != nil {
		slog.Warn("forum: failed to enqueue search index task", "entity_type", entityType, "entity_id", entityID.String(), "error", err)
	}
}

// enqueueRSSRegenerate enqueues rss:regenerate so the universal + forum RSS
// feeds reflect a newly published or deleted thread without waiting for the
// hourly cron. Fire-and-forget per Rule 8 (RSS-013).
func (h *Handlers) enqueueRSSRegenerate() {
	if h.asynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{})
	if err != nil {
		slog.Warn("forum: failed to marshal rss:regenerate payload", "error", err)
		return
	}

	if _, err := h.asynqClient.Enqueue(
		asynq.NewTask(worker.TypeRSSRegenerate, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeRSSRegenerate)),
	); err != nil {
		slog.Warn("forum: failed to enqueue rss:regenerate task", "error", err)
	}
}

// enqueueContentEmbedding fires a content:generate_embedding task for the
// backing contents row of a forum thread. Required so content_scores exists
// and GET /content/{id}/related can resolve vectors (BUG-004).
func (h *Handlers) enqueueContentEmbedding(contentID pgtype.UUID) {
	if h.asynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
	})
	if err != nil {
		slog.Warn("forum: failed to marshal content embedding payload", "error", err)
		return
	}

	if _, err := h.asynqClient.Enqueue(
		asynq.NewTask(worker.TypeAIGenerateContentEmbedding, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeAIGenerateContentEmbedding)),
	); err != nil {
		slog.Warn("forum: failed to enqueue content embedding task", "content_id", contentID.String(), "error", err)
	}
}

// indexThreadRAG fires-and-forgets a RAG indexing call for a forum thread.
// Errors are logged, never propagated (Rule 8). The body is the thread title
// + body concatenated for embedding.
func (h *Handlers) indexThreadRAG(ctx context.Context, threadID, spaceID, authorID pgtype.UUID, title string, body []byte) {
	if h.ragIndexer == nil {
		return
	}

	text := title
	if len(body) > 0 {
		if text != "" {
			text += "\n\n"
		}

		text += string(body)
	}

	if text == "" {
		return
	}
	// BUG-002 (rag-audit): resolve the indexing language from
	// i18n.default_language (§7 i18n) instead of hardcoding "en". Falls back
	// to "en" when settings are not injected or the key is unset.
	language := "en"
	if h.settings != nil {
		language = h.settings.GetStringOr(ctx, "i18n.default_language", "en")
	}

	go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
		bgCtx := context.Background()
		// Threads inherit their parent space's visibility (§69 scoped
		// retrieval); a missing space fails closed to private.
		visibility := "private"

		if spaceID.Valid {
			if space, err := h.Service.store.read.GetSpaceByID(bgCtx, spaceID); err == nil {
				visibility = space.Visibility
			}
		}

		if err := h.ragIndexer.IndexEntity(bgCtx, "forum_thread", threadID, visibility, spaceID, authorID, text, language); err != nil {
			slog.Warn("forum: RAG index failed for thread", "thread_id", threadID.String(), "error", err)
		}
	}()
}

// purgeThreadRAG fires-and-forgets a RAG purge for a deleted forum thread.
func (h *Handlers) purgeThreadRAG(ctx context.Context, threadID pgtype.UUID) {
	if h.ragIndexer == nil {
		return
	}

	go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
		bgCtx := context.Background()
		if err := h.ragIndexer.PurgeEntity(bgCtx, "forum_thread", threadID); err != nil {
			slog.Warn("forum: RAG purge failed for thread", "thread_id", threadID.String(), "error", err)
		}
	}()
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
	switch {
	case errors.Is(err, ErrTooManyTags):
		errpkg.WriteError(w, http.StatusConflict, "too_many_tags", err.Error())
	case errors.Is(err, ErrTagInUse):
		errpkg.WriteError(w, http.StatusConflict, "tag_in_use", err.Error())
	case errors.Is(err, ErrEditWindowExpired):
		errpkg.WriteError(w, http.StatusForbidden, "edit_window_expired", err.Error())
	case errors.Is(err, ErrMembersOnly):
		errpkg.WriteError(w, http.StatusForbidden, "members_only", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// isThreadModerator reports whether userID may auto-publish a space-scoped
// thread: space moderators and holders of the forum.thread moderator-override
// node publish immediately; everyone else starts pending review.
func (h *Handlers) isThreadModerator(ctx context.Context, userID, spaceID pgtype.UUID) bool {
	if h.Service.canModerateAny(ctx, spaceID, userID, "forum.thread.edit_any") {
		return true
	}

	return false
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

	return int32(n) // #nosec G109 G115 -- bounded by MaxPageSize check above
}

// parseCursorCreatedAt extracts the cursor_created_at query parameter.
// The frontend sends RFC3339 timestamps (e.g. 2026-08-07T15:33:40.117513+01:00),
// which pgtype.Timestamptz.Scan does not accept, so we parse with time.RFC3339.
func parseCursorCreatedAt(r *http.Request) pgtype.Timestamptz {
	s := r.URL.Query().Get("cursor_created_at")
	if s == "" {
		return pgtype.Timestamptz{}
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return pgtype.Timestamptz{}
	}

	return pgtype.Timestamptz{Time: t, Valid: true, InfinityModifier: pgtype.Finite}
}

// parseCursorID extracts the cursor_id query parameter into a typed UUID.
func parseCursorID(r *http.Request) pgtype.UUID {
	s := r.URL.Query().Get("cursor_id")
	if s == "" {
		return pgtype.UUID{}
	}

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}
	}

	return id
}

// parseCursorVoteScore extracts the cursor_vote_score query parameter for
// top-sort keyset pagination. Returns nil when absent.
// requesterID returns the caller's user id or an invalid UUID for guests.
func requesterID(r *http.Request) pgtype.UUID {
	if u := auth.FromContext(r.Context()); u != nil {
		return u.UserID
	}
	return pgtype.UUID{}
}

func parseCursorVoteScore(r *http.Request) *int32 {
	s := r.URL.Query().Get("cursor_vote_score")
	if s == "" {
		return nil
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return nil
	}

	v := int32(n) // #nosec G109 G115 -- bounded by non-negative check, int fits int32

	return &v
}

// forumBodyHTML extracts a plain-text preview from a raw TipTap JSON body and
// wraps it in a safe <p> tag. This is the canonical body_html for the backing
// contents row of a forum thread (Section 14).
func forumBodyHTML(body []byte) string {
	text, _ := content.ExtractTipTapText(body)
	if text == "" {
		return "<p></p>"
	}

	return "<p>" + html.EscapeString(text) + "</p>"
}

// forumBodyText extracts a plain-text representation of a TipTap/ProseMirror
// JSON body. Used for JSON-LD and excerpts where raw JSON is invalid.
func forumBodyText(body []byte) string {
	text, _ := content.ExtractTipTapText(body)
	return text
}

// userVoteMap builds a simple map of entity_id -> vote value from the rows
// returned by ListUserVotesForThread. The frontend passes these to VoteRail.
func userVoteMap(votes []gen.ListUserVotesForThreadRow) map[string]int {
	m := make(map[string]int, len(votes))
	for _, v := range votes {
		m[v.EntityID.String()] = int(v.Value)
	}

	return m
}

// ---------------------------------------------------------------------------
// Thread Handlers
// ---------------------------------------------------------------------------

// pollInputJSON is the wire shape for an embedded poll inside a new thread
// request. It is decoded directly by the CreateThread handler and converted to
// the service's PollInput.
type pollInputJSON struct {
	Question      string                `json:"question"`
	IsMultiSelect bool                  `json:"is_multi_select"`
	CloseAt       string                `json:"close_at"`
	Options       []pollOptionInputJSON `json:"options"`
}

type pollOptionInputJSON struct {
	Label    string `json:"label"`
	Position int32  `json:"position"`
}

// CreateThread handles POST /api/v1/forum/threads.
func (h *Handlers) CreateThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Title             string         `json:"title"`
		Slug              string         `json:"slug"`
		Body              []byte         `json:"body"`
		SpaceID           *string        `json:"space_id"`
		ContentID         *string        `json:"content_id"`
		ParentReplyID     *string        `json:"parent_reply_id"`
		IsAnonymous       bool           `json:"is_anonymous"`
		EditWindowMinutes *int32         `json:"edit_window_minutes"`
		Poll              *pollInputJSON `json:"poll"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes+4096)
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

	if rejectOverlongField(w, "title", input.Title, maxTitleRunes) {
		return
	}

	if rejectOverlongField(w, "slug", input.Slug, maxSlugRunes) {
		return
	}

	if len(input.Body) > 0 && rejectOversizedBody(w, input.Body) {
		return
	}

	var spaceID, contentID, parentReplyID pgtype.UUID
	if input.SpaceID != nil {
		if err := spaceID.Scan(*input.SpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id format")
			return
		}
	}

	if input.ContentID != nil {
		if err := contentID.Scan(*input.ContentID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_content_id", "Invalid content_id format")
			return
		}
	}

	if input.ParentReplyID != nil {
		if err := parentReplyID.Scan(*input.ParentReplyID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent_reply_id", "Invalid parent_reply_id format")
			return
		}
	}

	// Pass 0 when the client does not specify an edit window so the service
	// applies the hot-reloadable default (forum.edit_window_minutes).
	editWindow := int32(0)
	if input.EditWindowMinutes != nil {
		editWindow = *input.EditWindowMinutes
	}

	var pollInput *PollInput
	if input.Poll != nil {
		pollInput = &PollInput{
			Question:      input.Poll.Question,
			IsMultiSelect: input.Poll.IsMultiSelect,
			Options:       make([]PollOptionInput, len(input.Poll.Options)),
		}
		for i, opt := range input.Poll.Options {
			pollInput.Options[i] = PollOptionInput(opt)
		}

		if input.Poll.CloseAt != "" {
			pollInput.CloseAt = pgtype.Timestamptz{}
			if t, err := time.Parse(time.RFC3339, input.Poll.CloseAt); err == nil {
				pollInput.CloseAt = pgtype.Timestamptz{Time: t, Valid: true, InfinityModifier: pgtype.Finite}
			}
		}
	}

	// BUG-004: create the backing contents row so reporting, related content,
	// and content-type integrations can resolve the thread by content_id.
	// Clients that already supply a content_id (e.g. persona pre-create flow)
	// can bypass this step. The row is created pending review whenever the
	// thread is space-scoped and the author is not a moderator, so the
	// auto-publish path cannot bypass moderation; RAG/search indexing only
	// runs after publish (see below).
	if h.content != nil && !contentID.Valid {
		needsReview := spaceID.Valid && !h.isThreadModerator(r.Context(), user.UserID, spaceID)

		status := content.StatusDraft

		if spaceID.Valid {
			if needsReview {
				// Moderation-hold state is "in_review" per the contents
				// CHECK constraint (there is no "pending_review" value).
				status = content.StatusInReview
			} else {
				status = content.StatusPublished
			}
		}

		contentRow, err := h.content.CreateContent(r.Context(), content.CreateContentInput{
			AuthorID:     user.UserID,
			ContentType:  "forum_thread",
			Title:        content.SanitizeText(input.Title),
			Slug:         content.SanitizeText(input.Slug),
			Body:         string(input.Body),
			BodyHtml:     forumBodyHTML(input.Body),
			FieldValues:  []byte("{}"),
			SeoIndexable: true,
			Status:       status,
			Visibility:   content.VisibilityPublic,
			SpaceID:      spaceID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}

		contentID = contentRow.ID

		if status == content.StatusPublished {
			if err := h.content.EnsureContentScore(r.Context(), contentID); err != nil {
				slog.Warn("forum: failed to ensure content score", "content_id", contentID.String(), "error", err)
			}

			h.enqueueContentEmbedding(contentID)
		}
	}

	thread, err := h.Service.CreateThread(r.Context(), user.UserID, CreateThreadInput{
		ContentID:         contentID,
		SpaceID:           spaceID,
		ParentReplyID:     parentReplyID,
		Title:             content.SanitizeText(input.Title),
		Slug:              content.SanitizeText(input.Slug),
		Body:              input.Body,
		IsAnonymous:       input.IsAnonymous,
		EditWindowMinutes: editWindow,
		Poll:              pollInput,
	})
	if err != nil {
		// Roll back the backing content row so we don't leave an orphan.
		if h.content != nil && contentID.Valid {
			if delErr := h.content.DeleteContent(r.Context(), contentID); delErr != nil {
				slog.Warn("forum: failed to clean up backing content after thread creation failure", "content_id", contentID.String(), "error", delErr)
			}
		}

		handleServiceError(w, err)

		return
	}

	// Indexing gates: RAG + search only run once the thread is publicly
	// visible. Space-scoped threads from non-moderators start as
	// pending review (backing contents row is draft/in_review), so indexing
	// is deferred until publish/moderation.
	published := !spaceID.Valid || h.isThreadModerator(r.Context(), user.UserID, spaceID)

	// Dispatch @mention notifications — fire-and-forget (BL-002, Section 15).
	h.dispatchMentionNotifications(r.Context(), user.UserID, thread.ID, spaceID, input.Body, "mention.content")

	if published {
		// RAG indexing — fire-and-forget (§69/ADR-069 BUG-001 fix: forum threads
		// are part of the RAG corpus).
		h.indexThreadRAG(r.Context(), thread.ID, spaceID, user.UserID, input.Title, input.Body)

		// §12 stale-index fix: enqueue Meilisearch re-index for the new thread.
		h.enqueueSearchIndex("forum_thread", "index", thread.ID)

		// RSS-013: the new thread belongs in the universal + forum feeds now,
		// not after the hourly cron.
		h.enqueueRSSRegenerate()
	}

	h.resolveLiterals(r, &thread)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": thread})
}

// ListThreads handles GET /api/v1/forum/threads.
// Supports ?space_id=, ?author_id=, ?tag_id=, ?page_size=, ?cursor_created_at=,
// ?sort=latest|top and ?solved=true|false.
// Public: when no user is present, lists all public threads for the platform.
func (h *Handlers) ListThreads(w http.ResponseWriter, r *http.Request) {
	limit := parsePageSize(r)
	cursor := parseCursorCreatedAt(r)
	cursorID := parseCursorID(r)
	cursorVoteScore := parseCursorVoteScore(r)

	// Parse optional solved filter.
	var solved *bool

	if v := r.URL.Query().Get("solved"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_solved", "solved must be true or false")
			return
		}

		solved = &b
	}

	// Parse sort; "top" and "latest" are supported here. "new" is accepted
	// as an alias for "latest" (spec Section 08 uses new|top).
	sortBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if sortBy == "new" {
		sortBy = "latest"
	}

	switch sortBy {
	case "", "latest", "top":
		// OK
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_sort", "sort must be one of: latest, new, top")
		return
	}

	spaceIDStr := r.URL.Query().Get("space_id")
	authorIDStr := r.URL.Query().Get("author_id")
	tagIDStr := r.URL.Query().Get("tag_id")
	tagSlug := r.URL.Query().Get("tag")

	switch {
	case spaceIDStr != "":
		var spaceID pgtype.UUID
		if err := spaceID.Scan(spaceIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_space_id", "Invalid space_id")
			return
		}

		// F-009 / §05: 'private'-space listings are invisible to non-members;
		// 'members'-space listings show titles but mask bodies.
		requester := requesterID(r)
		accessLevel, err := h.Service.SpaceAccessLevel(r.Context(), spaceID, requester)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if accessLevel == ThreadAccessNone {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Space not found")
			return
		}

		threads, err := h.Service.ListThreadsBySpaceWithDetails(r.Context(), spaceID, solved, sortBy, cursor, cursorID, cursorVoteScore, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if threads == nil {
			threads = []gen.ListForumThreadsBySpaceWithDetailsRow{}
		}

		// §08: withhold the author's identity on anonymous threads — the
		// display fields are nulled in SQL but author_user_id survives;
		// zero it for anyone who is neither the author nor a
		// space/platform moderator (admins keep attribution).
		for i := range threads {
			if threads[i].IsAnonymous && threads[i].AuthorUserID != requester &&
				!(requester.Valid && h.Service.canModerateAny(r.Context(), threads[i].SpaceID, requester, "forum.thread.edit_any")) {
				threads[i].AuthorUserID = pgtype.UUID{}
			}
			// F-009: title only for non-members — bodies never leave the API.
			if accessLevel == ThreadAccessTitleOnly {
				threads[i].Body = nil
			}
		}

		h.resolveLiterals(r, &threads)

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": threads})

	case authorIDStr != "":
		var authorID pgtype.UUID
		if err := authorID.Scan(authorIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_author_id", "Invalid author_id")
			return
		}

		requester := requesterID(r)
		threads, err := h.Service.ListThreadsByAuthorWithDetails(r.Context(), requester, authorID, solved, sortBy, cursor, cursorID, cursorVoteScore, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if threads == nil {
			threads = []gen.ListForumThreadsByAuthorWithDetailsRow{}
		}

		// §08: withhold the author's identity on anonymous threads — the
		// display fields are nulled in SQL but author_user_id survives;
		// zero it for anyone who is neither the author nor a
		// space/platform moderator (admins keep attribution).
		for i := range threads {
			if threads[i].IsAnonymous && threads[i].AuthorUserID != requester &&
				!(requester.Valid && h.Service.canModerateAny(r.Context(), threads[i].SpaceID, requester, "forum.thread.edit_any")) {
				threads[i].AuthorUserID = pgtype.UUID{}
			}
			// F-009: mask bodies on non-public spaces the viewer isn't in.
			if !threads[i].SpaceIsMember && threads[i].SpaceVisibility != "public" {
				threads[i].Body = nil
			}
		}

		h.resolveLiterals(r, &threads)

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": threads})

	case tagIDStr != "":
		var tagID pgtype.UUID
		if err := tagID.Scan(tagIDStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag_id")
			return
		}

		requester := requesterID(r)
		threads, err := h.Service.ListThreadsByTagWithDetails(r.Context(), requester, tagID, solved, sortBy, cursor, cursorID, cursorVoteScore, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if threads == nil {
			threads = []gen.ListForumThreadsByTagWithDetailsRow{}
		}

		// §08: withhold the author's identity on anonymous threads — the
		// display fields are nulled in SQL but author_user_id survives;
		// zero it for anyone who is neither the author nor a
		// space/platform moderator (admins keep attribution).
		for i := range threads {
			if threads[i].IsAnonymous && threads[i].AuthorUserID != requester &&
				!(requester.Valid && h.Service.canModerateAny(r.Context(), threads[i].SpaceID, requester, "forum.thread.edit_any")) {
				threads[i].AuthorUserID = pgtype.UUID{}
			}
			// F-009: mask bodies on non-public spaces the viewer isn't in.
			if !threads[i].SpaceIsMember && threads[i].SpaceVisibility != "public" {
				threads[i].Body = nil
			}
		}

		h.resolveLiterals(r, &threads)

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": threads})

	case tagSlug != "":
		// Resolve the slug to a tag ID, then filter threads by that tag.
		// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (?tag=slug)
		tag, err := h.Service.GetTagBySlug(r.Context(), tagSlug)
		if err != nil {
			errpkg.WriteError(w, http.StatusNotFound, "tag_not_found", "Tag not found")
			return
		}

		requester := requesterID(r)
		threads, err := h.Service.ListThreadsByTagWithDetails(r.Context(), requester, tag.ID, solved, sortBy, cursor, cursorID, cursorVoteScore, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if threads == nil {
			threads = []gen.ListForumThreadsByTagWithDetailsRow{}
		}

		// §08: withhold the author's identity on anonymous threads — the
		// display fields are nulled in SQL but author_user_id survives;
		// zero it for anyone who is neither the author nor a
		// space/platform moderator (admins keep attribution).
		for i := range threads {
			if threads[i].IsAnonymous && threads[i].AuthorUserID != requester &&
				!(requester.Valid && h.Service.canModerateAny(r.Context(), threads[i].SpaceID, requester, "forum.thread.edit_any")) {
				threads[i].AuthorUserID = pgtype.UUID{}
			}
			// F-009: mask bodies on non-public spaces the viewer isn't in.
			if !threads[i].SpaceIsMember && threads[i].SpaceVisibility != "public" {
				threads[i].Body = nil
			}
		}

		h.resolveLiterals(r, &threads)

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": threads})

	default:
		// List all public threads for the platform regardless of auth — the
		// forum index shows everybody's threads. Callers wanting their own
		// threads pass author_id explicitly. Uses the WithDetails variant so
		// the frontend receives joined author + space data in one response.
		requester := requesterID(r)
		threads, err := h.Service.ListThreadsByPlatformWithDetails(r.Context(), requester, solved, sortBy, cursor, cursorID, cursorVoteScore, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		if threads == nil {
			threads = []gen.ListForumThreadsWithDetailsRow{}
		}

		// §08: withhold the author's identity on anonymous threads — the
		// display fields are nulled in SQL but author_user_id survives;
		// zero it for anyone who is neither the author nor a
		// space/platform moderator (admins keep attribution).
		for i := range threads {
			if threads[i].IsAnonymous && threads[i].AuthorUserID != requester &&
				!(requester.Valid && h.Service.canModerateAny(r.Context(), threads[i].SpaceID, requester, "forum.thread.edit_any")) {
				threads[i].AuthorUserID = pgtype.UUID{}
			}
			// F-009: mask bodies on non-public spaces the viewer isn't in.
			if !threads[i].SpaceIsMember && threads[i].SpaceVisibility != "public" {
				threads[i].Body = nil
			}
		}

		h.resolveLiterals(r, &threads)

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": threads})
	}
}

// GetThread handles GET /api/v1/forum/{slug}.
func (h *Handlers) GetThread(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	// Use the WithDetails variant so the frontend receives joined author +
	// space data in one response. Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
	thread, err := h.Service.GetThreadBySlugWithDetails(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// F-009 / §05: 'private'-space threads 404 for non-members; 'members'-
	// space threads return the title with the body withheld (restricted).
	accessLevel, accErr := h.Service.ThreadAccessLevel(r.Context(), thread.ID, requesterID(r))
	if accErr != nil {
		handleServiceError(w, accErr)
		return
	}

	if accessLevel == ThreadAccessNone {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Thread not found")
		return
	}

	restricted := accessLevel == ThreadAccessTitleOnly
	if restricted {
		thread.Body = nil
	}

	// Resolve {{...}} literals in title/body before the JSON-LD is built so
	// structured data also carries resolved text.
	h.resolveLiterals(r, &thread)

	// BUG-021: enqueue the view-count increment to a shared background
	// worker instead of spawning a per-request goroutine. Non-blocking.
	h.Service.IncrementThreadView(r.Context(), thread.ID)

	// BL-012 / Section 08: emit QAPage JSON-LD for solved threads so search
	// engines can index the accepted answer. The accepted-answer body is the
	// reply referenced by forum_threads.parent_reply_id; we fall back to the
	// top-voted reply for legacy threads without an explicit accepted reply.
	// For unsolved threads we emit a QAPage with answerCount=0.
	// §43 indexability — sibling keys alongside data so the detail page can
	// emit noindex for thin/unpublished threads. Word gate skipped
	// (SkipWordGate): short questions are legitimate forum content; the
	// reply/solved gates still apply. A thread with no backing contents row
	// (NULL joins) fails closed → not indexable.
	contentStatus, contentVisibility, seoIndexable := "published", "public", false
	if thread.ContentStatus != nil {
		contentStatus = *thread.ContentStatus
	}
	if thread.ContentVisibility != nil {
		contentVisibility = *thread.ContentVisibility
	}
	if thread.SeoIndexable != nil {
		seoIndexable = *thread.SeoIndexable
	}
	response := map[string]any{
		"data":          thread,
		"seo_indexable": seoIndexable,
		"indexable": bots.IsContentIndexableByType(bots.ContentIndexableInput{
			ContentType:  "forum_thread",
			Status:       contentStatus,
			Visibility:   contentVisibility,
			WordCount:    len(strings.Fields(forumBodyText(thread.Body))),
			ReplyCount:   int(thread.ReplyCount),
			IsQuestion:   !thread.Solved,
			Solved:       thread.Solved,
			SeoIndexable: seoIndexable,
			SkipWordGate: true,
		}, h.sitemapCfg(r.Context())),
	}
	authorName := ""

	if thread.AuthorUsername != nil {
		if name, ok := thread.AuthorUsername.(string); ok {
			authorName = name
		}
	}

	questionBody := forumBodyText(thread.Body)

	// BUG-020: cache the JSON-LD for solved threads to avoid a synchronous
	// DB round-trip on every request. The cache has a 5-minute TTL; on a
	// cache miss we fetch the accepted reply, generate the JSON-LD, and
	// store it. For unsolved threads we generate inline (no DB fetch).
	// // Section 14 / Section 26: prefer the admin-managed QAPage template
	// (interpolated with the thread's field values) over the hardcoded
	// generator. The generator remains the fallback when no active template
	// exists or the resolver is unavailable.
	var jsonLd any

	if thread.Solved && !restricted {
		if cached, ok := h.Service.getJSONLDCache(r.Context(), thread.ID); ok {
			var parsed any
			if err := json.Unmarshal([]byte(cached), &parsed); err == nil {
				jsonLd = parsed
			} else {
				jsonLd = cached
			}
		} else {
			var answerBody string

			var answerAuthor string

			var answerCreatedAt string

			var answerUpvotes string
			// BL-012: prefer the explicit accepted reply (parent_reply_id).
			// Fall back to the top-voted reply when no explicit accepted
			// reply exists.
			if thread.ParentReplyID.Valid {
				if accepted, accErr := h.Service.GetReplyByIDWithAuthor(r.Context(), thread.ParentReplyID); accErr == nil {
					answerBody = forumBodyText(accepted.Body)
					answerAuthor = ifaceString(accepted.AuthorDisplayName, accepted.AuthorUsername)
					answerCreatedAt = tsString(accepted.CreatedAt)
					answerUpvotes = strconv.Itoa(int(accepted.VoteScore))
				} else if !errors.Is(accErr, pgx.ErrNoRows) {
					slog.Warn("QAPage JSON-LD: failed to fetch accepted reply", "thread_id", thread.ID, "reply_id", thread.ParentReplyID, "error", accErr)
				}
			}

			if answerBody == "" {
				if top, topErr := h.Service.GetTopReplyForThread(r.Context(), thread.ID); topErr == nil {
					answerBody = forumBodyText(top.Body)
					answerAuthor = ifaceString(top.AuthorDisplayName, top.AuthorUsername)
					answerCreatedAt = tsString(top.CreatedAt)
					answerUpvotes = strconv.Itoa(int(top.VoteScore))
				} else if !errors.Is(topErr, pgx.ErrNoRows) {
					slog.Warn("QAPage JSON-LD: failed to fetch top reply", "thread_id", thread.ID, "error", topErr)
				}
			}

			jsonLd = h.resolveQAPageJSONLD(r, thread, questionBody, authorName, answerBody, answerAuthor, answerCreatedAt, answerUpvotes)
			if b, err := json.Marshal(jsonLd); err == nil {
				h.Service.setJSONLDCache(r.Context(), thread.ID, string(b))
			}
		}
	} else if !restricted {
		jsonLd = h.resolveQAPageJSONLD(r, thread, questionBody, authorName, "", "", "", "")
	}

	if restricted {
		response["restricted"] = true
	}

	response["json_ld"] = jsonLd

	// Add can_solve flag for the frontend so it can show the solve/unsolve
	// affordance to the OP or moderators (BUG-014).
	// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md ("OP or moderator")
	canSolve := false
	canEdit := false
	canDelete := false
	canModerate := false
	isBookmarked := false
	bookmarkID := ""

	if user := auth.FromContext(r.Context()); user != nil {
		threadModel := gen.ForumThread{
			AuthorUserID: thread.AuthorUserID,
			SpaceID:      thread.SpaceID,
		}
		canSolve = h.Service.canSolveThread(r.Context(), threadModel, user.UserID)
		// The author, a space moderator, or a holder of forum.thread.edit_any
		// may edit. Mirrors the check in UpdateThread so the frontend can show
		// the edit affordance only when the PATCH would succeed.
		// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Editing)
		canEdit = h.Service.canModerateThread(r.Context(), threadModel, user.UserID, "forum.thread.edit_any")
		// Delete uses the delete_any node; author and delete_own also allow it.
		canDelete = h.Service.canModerateThread(r.Context(), threadModel, user.UserID, "forum.thread.delete_any")
		// Moderator scope for replies (edit/delete any), separate from thread
		// ownership. Used by ReplyItem to show edit/delete affordances.
		canModerate = h.Service.canModerateAny(r.Context(), thread.SpaceID, user.UserID, "forum.reply.edit_any") ||
			h.Service.canModerateAny(r.Context(), thread.SpaceID, user.UserID, "forum.reply.delete_any")

		// Expose whether the current user has bookmarked this thread so the
		// frontend can render the correct bookmark toggle state, plus the
		// bookmark's ID so DELETE /forum/bookmarks/{id} can be called
		// without first listing all bookmarks.
		// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Bookmarks)
		if bm, err := h.Service.GetBookmarkByEntity(r.Context(), user.UserID, EntityTypeThread, thread.ID); err == nil {
			isBookmarked = true
			bookmarkID = bm.ID.String()
		}

		// BUG-006: return the current user's vote state for the thread and
		// its replies so VoteRail can render the selected direction.
		if votes, err := h.Service.GetUserVotesForThread(r.Context(), thread.ID, user.UserID); err == nil {
			response["user_votes"] = userVoteMap(votes)
		}
	}

	response["can_solve"] = canSolve
	response["can_edit"] = canEdit
	response["can_delete"] = canDelete
	response["can_moderate"] = canModerate
	response["is_bookmarked"] = isBookmarked
	response["bookmark_id"] = bookmarkID

	// §08: anonymous threads withhold the author's identity from other
	// users (admins/moderators still see it). The WithDetails queries null
	// the display fields in SQL but author_user_id survives — zero it for
	// anyone who is neither the author nor a moderator/permission-holder.
	if thread.IsAnonymous {
		viewerIsAuthor := false
		if u := auth.FromContext(r.Context()); u != nil && u.UserID == thread.AuthorUserID {
			viewerIsAuthor = true
		}
		privileged := canSolve || canEdit || canDelete || canModerate
		if !viewerIsAuthor && !privileged {
			thread.AuthorUserID = pgtype.UUID{}
		}
	}

	// GDPR / ADR-029: expose deleted_user_ids so the frontend can render
	// @[deleted user] for mentions of erased users in the thread body
	// (Section 15). Best-effort: a failure logs a warning but does not
	// fail the request.
	if h.mentionCheckers != nil && len(thread.Body) > 0 {
		erasedIDs, err := mentions.ComputeErasedMentionIDs(
			r.Context(), thread.Body, h.mentionCheckers,
		)
		if err != nil {
			slog.Warn("forum: ComputeErasedMentionIDs failed", "thread_id", thread.ID.String(), "error", err)
		} else if len(erasedIDs) > 0 {
			response["deleted_user_ids"] = erasedIDs
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, response)
}

// dispatchMentionNotifications extracts @mentioned users from a forum thread
// or reply body (TipTap JSON) and fires a mention.content notification to
// each via the batch system (Section 35 — BL-002). Fire-and-forget per
// Rule 8: errors are logged but never propagated to the caller.
// // Self-mentions (mentioning the author) are skipped.
// Privacy filtering (blocks, erased users, private-space membership) is
// applied via mentions.FilterMentions when checkers are wired (Section 15).
// The notificationType parameter distinguishes thread mentions
// (mention.content) from reply mentions (mention.comment).
func (h *Handlers) dispatchMentionNotifications(
	ctx context.Context,
	authorID, subjectID, spaceID pgtype.UUID,
	body []byte,
	notificationType string,
) {
	if h.notifier == nil {
		return
	}

	mentioned, err := mentions.ExtractMentions(body)
	if err != nil {
		// Body may not be valid TipTap JSON (e.g., plain text); skip mentions.
		return
	}

	// Apply privacy filtering when DB checkers are wired (BUG-004):
	// skip erased users (GDPR), blocked users (privacy), and non-members
	// of private spaces (visibility leak). Section 15 mention edge cases.
	if h.mentionCheckers != nil {
		isPrivateSpace, err := h.mentionCheckers.IsPrivateSpace(ctx, spaceID)
		if err != nil {
			slog.Warn("forum: IsPrivateSpace check failed", "space_id", spaceID.String(), "error", err)

			isPrivateSpace = false
		}

		mentioned, err = mentions.FilterMentions(
			ctx, mentioned, authorID, spaceID, isPrivateSpace,
			h.mentionCheckers, h.mentionCheckers, h.mentionCheckers,
		)
		if err != nil {
			slog.Warn("forum: FilterMentions returned unexpected error", "subject_id", subjectID.String(), "error", err)
		}
	}

	for _, m := range mentioned {
		if m.EntityType != "user" || !m.ID.Valid || m.ID == authorID {
			continue
		}

		if err := h.notifier.DispatchNotification(
			ctx, m.ID,
			notificationType, "",
			authorID,
			"forum", subjectID,
			"", "", "",
		); err != nil {
			slog.Warn("forum: failed to dispatch mention notification", "user_id", m.ID.String(), "type", notificationType, "error", err)
		}
	}
}

// resolveQAPageJSONLD resolves the QAPage structured data for a forum thread.
// It prefers the admin-managed JSON-LD template (Section 14 / Section 26),
// interpolating the thread and accepted-answer field values into the
// {{field_key}} placeholders. When no active template exists (or the resolver
// is unavailable), it falls back to the hardcoded seo.GenerateQAPageJSONLD
// generator so existing behaviour is preserved.
func (h *Handlers) resolveQAPageJSONLD(
	r *http.Request,

	thread gen.GetForumThreadBySlugWithDetailsRow,
	questionBody, authorName, answerBody, answerAuthor, answerCreatedAt, answerUpvotes string,
) any {
	isSolved := answerBody != ""
	fields := map[string]string{
		"title":                      thread.Title,
		"body":                       questionBody,
		"answer_count":               strconv.Itoa(int(thread.ReplyCount)),
		"accepted_answer_body":       answerBody,
		"accepted_answer_author":     answerAuthor,
		"accepted_answer_created_at": answerCreatedAt,
		"accepted_answer_upvotes":    answerUpvotes,
		"author_name":                authorName,
		"created_at":                 tsString(thread.CreatedAt),
	}

	if h.jsonldResolver != nil {
		if raw, err := h.jsonldResolver.ResolveJSONLD(r.Context(), "QAPage", fields); err == nil && len(raw) > 0 {
			var parsed any
			if err := json.Unmarshal(raw, &parsed); err == nil {
				return parsed
			}

			return string(raw)
		} else if err != nil {
			slog.Warn("QAPage JSON-LD: template resolution failed, falling back to generator", "thread_id", thread.ID, "error", err)
		}
	}

	return seo.GenerateQAPageJSONLD(thread.Title, questionBody, answerBody, authorName, tsString(thread.CreatedAt), isSolved)
}

// ifaceString returns the first non-empty string representation of the given
// interface{} values (used for author display name / username fallbacks).
func ifaceString(values ...any) string {
	for _, v := range values {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}

	return ""
}

// tsString renders a pgtype.Timestamptz as RFC3339 or empty.
func tsString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}

// UpdateThread handles PATCH /api/v1/forum/{id}.
func (h *Handlers) UpdateThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid thread ID")
		return
	}

	var input struct {
		Title       string `json:"title"`
		Body        []byte `json:"body"`
		IsAnonymous bool   `json:"is_anonymous"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes+4096)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Title != "" && rejectOverlongField(w, "title", input.Title, maxTitleRunes) {
		return
	}

	if len(input.Body) > 0 && rejectOversizedBody(w, input.Body) {
		return
	}

	if input.Title != "" {
		input.Title = content.SanitizeText(input.Title)
	}

	thread, err := h.Service.UpdateThread(r.Context(), id, user.UserID, input.Title, input.Body, input.IsAnonymous)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// BUG-004: keep the backing contents row in sync with title/body changes.
	if thread.ContentID.Valid {
		h.syncThreadContent(r.Context(), user.UserID, thread.ContentID, thread.Title, thread.Slug, input.Body)
	}

	// RAG re-indexing — fire-and-forget (§69/ADR-069 BUG-001 fix).
	h.indexThreadRAG(r.Context(), id, thread.SpaceID, thread.AuthorUserID, input.Title, input.Body)

	// §12 stale-index fix: enqueue Meilisearch re-index for the updated thread.
	h.enqueueSearchIndex("forum_thread", "index", id)

	// The QAPage JSON-LD for this thread is now stale; clear it so the next
	// GET regenerates with the updated title/body.
	h.Service.deleteJSONLDCache(r.Context(), id)

	h.resolveLiterals(r, &thread)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": thread})
}

// DeleteThread handles DELETE /api/v1/forum/{id}.
func (h *Handlers) DeleteThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid thread ID")
		return
	}

	// BUG-004: fetch the thread first so we can soft-delete the backing
	// contents row after the forum thread is deleted.
	thread, err := h.Service.GetThread(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if err := h.Service.DeleteThread(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	if h.content != nil && thread.ContentID.Valid {
		if err := h.content.DeleteContent(r.Context(), thread.ContentID); err != nil {
			slog.Warn("forum: failed to delete backing content for thread", "content_id", thread.ContentID.String(), "error", err)
		}
	}

	// RAG purge — fire-and-forget (§69/ADR-069 BUG-001 fix: deleted threads
	// must not be retrievable in RAG).
	h.purgeThreadRAG(r.Context(), id)

	// §12 stale-index fix: remove the deleted thread from the Meilisearch index.
	h.enqueueSearchIndex("forum_thread", "delete", id)

	// RSS-013: drop the deleted thread from cached feeds without waiting for
	// the hourly cron.
	h.enqueueRSSRegenerate()

	// The QAPage JSON-LD for a deleted thread must not be served again.
	h.Service.deleteJSONLDCache(r.Context(), id)

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// SolveThread handles POST /api/v1/forum/{id}/solve.
// The request body may include a reply_id to mark as the accepted answer;
// if omitted, the service defaults to the top-voted reply.
func (h *Handlers) SolveThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid thread ID")
		return
	}

	var input struct {
		ReplyID *string `json:"reply_id"`
	}
	// Body is optional; ignore decode errors for backward-compat empty bodies.
	_ = json.NewDecoder(r.Body).Decode(&input)

	var parentReplyID pgtype.UUID
	if input.ReplyID != nil && *input.ReplyID != "" {
		if err := parentReplyID.Scan(*input.ReplyID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_reply_id", "Invalid reply_id format")
			return
		}
	}

	if err := h.Service.SolveThread(r.Context(), id, user.UserID, parentReplyID); err != nil {
		handleServiceError(w, err)
		return
	}

	h.Service.deleteJSONLDCache(r.Context(), id)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "solved"}})
}

// UnsolveThread handles DELETE /api/v1/forum/{id}/solve.
// Ref: api-routes-catalog L590.
func (h *Handlers) UnsolveThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid thread ID")
		return
	}

	if err := h.Service.UnsolveThread(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	h.Service.deleteJSONLDCache(r.Context(), id)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "unsolved"}})
}

// ---------------------------------------------------------------------------
// Reply Handlers
// ---------------------------------------------------------------------------

// CreateReply handles POST /api/v1/forum/{id}/replies.
func (h *Handlers) CreateReply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	var input struct {
		Body          []byte  `json:"body"`
		ParentReplyID *string `json:"parent_reply_id"`
		IsAnonymous   bool    `json:"is_anonymous"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes+4096)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Body) > 0 && rejectOversizedBody(w, input.Body) {
		return
	}

	var parentReplyID pgtype.UUID
	if input.ParentReplyID != nil {
		if err := parentReplyID.Scan(*input.ParentReplyID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_parent_reply_id", "Invalid parent_reply_id format")
			return
		}
	}

	reply, err := h.Service.CreateReply(r.Context(), threadID, user.UserID, CreateReplyInput{
		Body:          input.Body,
		ParentReplyID: parentReplyID,
		IsAnonymous:   input.IsAnonymous,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Dispatch @mention notifications — fire-and-forget (BL-002, Section 15).
	// Fetch the thread to get its spaceID for private-space membership filtering.
	// F-002 / C-003 parity: mention notifications name the actor — suppress
	// them entirely for anonymous replies so the author stays anonymous.
	if !reply.IsAnonymous {
		var replySpaceID pgtype.UUID
		if thread, tErr := h.Service.GetThread(r.Context(), threadID); tErr == nil {
			replySpaceID = thread.SpaceID
		}

		h.dispatchMentionNotifications(r.Context(), user.UserID, reply.ID, replySpaceID, input.Body, "mention.comment")
	}

	// C-020 parity: return the joined row so the new reply renders with
	// author name/avatar without a refetch. The joined query already
	// NULL-wraps identity fields for anonymous replies.
	if row, getErr := h.Service.GetReplyByIDWithAuthor(r.Context(), reply.ID); getErr == nil {
		h.resolveLiterals(r, &row)
		errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": row})
		return
	}

	h.resolveLiterals(r, &reply)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": reply})
}

// ListReplies handles GET /api/v1/forum/{id}/replies.
func (h *Handlers) ListReplies(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	// F-007 / C-001 parity: replies on a deleted (or nonexistent) thread are
	// not exposed — the thread's soft-delete would otherwise leave its
	// replies world-readable.
	if _, err := h.Service.GetThread(r.Context(), threadID); err != nil {
		handleServiceError(w, err)
		return
	}

	// F-009 / §05: replies are content — 'private'-space threads 404 for
	// non-members; 'members'-space threads return 403 members_only so the
	// frontend can render the join prompt (the title is still visible via
	// the thread endpoint).
	switch level, accErr := h.Service.ThreadAccessLevel(r.Context(), threadID, requesterID(r)); {
	case accErr != nil:
		handleServiceError(w, accErr)
		return
	case level == ThreadAccessNone:
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Thread not found")
		return
	case level == ThreadAccessTitleOnly:
		errpkg.WriteError(w, http.StatusForbidden, "members_only", "Join the space to read replies")
		return
	}

	// Use the WithAuthor variant so the frontend receives joined author data
	// (username, display_name, avatar_url) in the same response.
	replies, err := h.Service.ListRepliesByThreadWithAuthor(r.Context(), threadID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if replies == nil {
		replies = []gen.ListForumRepliesByThreadWithAuthorRow{}
	}

	// §08: withhold the author's identity on anonymous replies (same rule
	// as threads — author_user_id is zeroed for viewers who are neither
	// the author nor a moderator of the thread's space / _any node holder).
	requester := requesterID(r)
	privileged := false
	if requester.Valid {
		if th, err := h.Service.GetThread(r.Context(), threadID); err == nil {
			privileged = h.Service.canModerateAny(r.Context(), th.SpaceID, requester, "forum.reply.edit_any") ||
				h.Service.canModerateAny(r.Context(), th.SpaceID, requester, "forum.reply.delete_any") ||
				h.Service.canModerateAny(r.Context(), th.SpaceID, requester, "forum.thread.edit_any")
		}
	}
	for i := range replies {
		if replies[i].IsAnonymous && replies[i].AuthorUserID != requester && !privileged {
			replies[i].AuthorUserID = pgtype.UUID{}
		}
		// F-004 / C-009 parity: tombstone deleted replies — the row stays so
		// live children remain reachable, but the original body must not leak.
		if replies[i].DeletedAt.Valid {
			replies[i].Body = nil
		}
	}

	// Resolve {{...}} literals in reply bodies (best-effort).
	h.resolveLiterals(r, &replies)

	// GDPR / ADR-029: compute erased-mention IDs across all reply bodies so
	// the frontend can render @[deleted user] for mentions of erased users
	// in replies (Section 15). The thread-body deleted_user_ids exposed by
	// GetThread only covers the thread body — replies may mention different
	// erased users. Best-effort: a failure logs a warning but does not fail
	// the request.
	resp := map[string]any{"data": replies}

	if h.mentionCheckers != nil {
		erasedSet := make(map[string]struct{})

		for _, reply := range replies {
			if len(reply.Body) == 0 {
				continue
			}

			erased, err := mentions.ComputeErasedMentionIDs(r.Context(), reply.Body, h.mentionCheckers)
			if err != nil {
				slog.Warn("forum: ComputeErasedMentionIDs failed for reply", "reply_id", reply.ID.String(), "error", err)
				continue
			}

			for _, id := range erased {
				erasedSet[id] = struct{}{}
			}
		}

		if len(erasedSet) > 0 {
			erasedIDs := make([]string, 0, len(erasedSet))
			for id := range erasedSet {
				erasedIDs = append(erasedIDs, id)
			}

			resp["deleted_user_ids"] = erasedIDs
		}
	}

	// BUG-006: include the current user's reply votes so the frontend can
	// render each VoteRail in the selected state.
	if user := auth.FromContext(r.Context()); user != nil {
		if votes, err := h.Service.GetUserVotesForThread(r.Context(), threadID, user.UserID); err == nil {
			resp["user_votes"] = userVoteMap(votes)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// UpdateReply handles PATCH /api/v1/forum/replies/{id}.
func (h *Handlers) UpdateReply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid reply ID")
		return
	}

	var input struct {
		Body []byte `json:"body"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes+4096)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.Body) > 0 && rejectOversizedBody(w, input.Body) {
		return
	}

	reply, err := h.Service.UpdateReply(r.Context(), id, user.UserID, input.Body)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	h.resolveLiterals(r, &reply)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": reply})
}

// DeleteReply handles DELETE /api/v1/forum/replies/{id}.
func (h *Handlers) DeleteReply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid reply ID")
		return
	}

	if err := h.Service.DeleteReply(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// RecordReplyView handles POST /api/v1/forum/replies/{id}/view.
// Idempotently records that the current user viewed a reply. The view count is
// materialised from the reply_views table via trigger (Section 08).
// Anonymous viewers are accepted but not recorded (the table requires a user_id).
func (h *Handlers) RecordReplyView(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid reply ID")
		return
	}

	if user == nil {
		errpkg.WriteJSON(w, http.StatusNoContent, nil)
		return
	}

	if err := h.Service.RecordReplyView(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Vote Handler
// ---------------------------------------------------------------------------

// VoteOnThread handles POST /api/v1/forum/{id}/vote.
// The entity id is resolved from the URL param; the entity_type defaults to
// "thread" but may be overridden to "reply" via the body for reply votes.
// Ref: api-routes-catalog L591.
func (h *Handlers) VoteOnThread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	entityID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID")
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		Value      int16  `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.EntityType == "" {
		input.EntityType = EntityTypeThread
	}

	if input.EntityType != EntityTypeThread && input.EntityType != EntityTypeReply {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_type", "entity_type must be 'thread' or 'reply'")
		return
	}

	if input.Value != VoteDown && input.Value != VoteUp {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_vote_value", "value must be -1 or 1")
		return
	}

	vote, err := h.Service.Vote(r.Context(), user.UserID, VoteInput{
		EntityType: input.EntityType,
		EntityID:   entityID,
		Value:      input.Value,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": vote})
}

// RemoveThreadVote handles DELETE /api/v1/forum/{id}/vote.
// Retracts the user's vote on the entity resolved from the URL param.
// Ref: api-routes-catalog L591.
func (h *Handlers) RemoveThreadVote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	entityID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid entity ID")
		return
	}

	entityType := r.URL.Query().Get("entity_type")
	if entityType == "" {
		entityType = EntityTypeThread
	}

	if entityType != EntityTypeThread && entityType != EntityTypeReply {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_type", "entity_type must be 'thread' or 'reply'")
		return
	}

	if err := h.Service.RemoveVote(r.Context(), user.UserID, entityType, entityID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Bookmark Handlers
// ---------------------------------------------------------------------------

// AddBookmark handles POST /api/v1/forum/bookmarks.
func (h *Handlers) AddBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Folder     string `json:"folder"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.EntityType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_entity_type", "entity_type is required")
		return
	}

	if input.EntityID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_entity_id", "entity_id is required")
		return
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(input.EntityID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_id", "Invalid entity_id")
		return
	}

	bm, err := h.Service.AddBookmark(r.Context(), user.UserID, input.EntityType, entityID, input.Folder)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": bm})
}

// RemoveBookmark handles DELETE /api/v1/forum/bookmarks/{id}.
// Resolves the bookmark by its primary key from the URL param — no query
// params required. Ref: api-routes-catalog L610.
func (h *Handlers) RemoveBookmark(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid bookmark ID")
		return
	}

	if err := h.Service.RemoveBookmarkByID(r.Context(), id, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ListBookmarks handles GET /api/v1/forum/bookmarks.
func (h *Handlers) ListBookmarks(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	bookmarks, err := h.Service.ListBookmarksByUser(r.Context(), user.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Resolve {{...}} literals in bookmarked thread titles (best-effort).
	h.resolveLiterals(r, &bookmarks)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": bookmarks})
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

	return int32(n) // #nosec G109 G115 -- bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Poll Handlers
// ---------------------------------------------------------------------------

// VotePoll handles POST /api/v1/forum/polls/{id}/vote.
func (h *Handlers) VotePoll(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	pollID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_poll_id", "Invalid poll ID")
		return
	}

	var input struct {
		OptionID string `json:"option_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.OptionID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_option_id", "option_id is required")
		return
	}

	var optionID pgtype.UUID
	if err := optionID.Scan(input.OptionID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_option_id", "Invalid option_id")
		return
	}

	if err := h.Service.VotePoll(r.Context(), pollID, optionID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "voted"}})
}

// GetPoll handles GET /api/v1/forum/polls/{id}.
// The {id} may be either a poll_id (preferred) or a thread_id (legacy).
// The handler tries the poll_id first and falls back to a thread lookup so
// both PollBlock refresh (poll.id) and the server load (thread.id) work.
func (h *Handlers) GetPoll(w http.ResponseWriter, r *http.Request) {
	pollID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_poll_id", "Invalid poll ID")
		return
	}

	ctx := r.Context()

	// Try poll_id first.
	poll, err := h.Service.GetPollByID(ctx, pollID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			handleServiceError(w, err)
			return
		}
		// Fall back to legacy thread_id lookup.
		poll, err = h.Service.GetPollByThreadID(ctx, pollID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
	}

	results, err := h.Service.GetPollResults(ctx, poll.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	data := map[string]any{
		"poll":    poll,
		"results": results,
	}

	// BUG-006: return the current user's selected option IDs so the UI can
	// pre-check chosen options and show hasVoted on multi-select polls.
	if user := auth.FromContext(r.Context()); user != nil {
		if votes, err := h.Service.GetUserPollVotes(ctx, poll.ID, user.UserID); err == nil {
			optionIDs := make([]string, 0, len(votes))
			for _, v := range votes {
				optionIDs = append(optionIDs, v.OptionID.String())
			}

			data["user_votes"] = optionIDs
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// CreatePoll handles POST /api/v1/forum/{id}/poll.
// Adds a poll to an existing thread. The thread must not already have a poll.
func (h *Handlers) CreatePoll(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	var input struct {
		Question      string `json:"question"`
		IsMultiSelect bool   `json:"is_multi_select"`
		CloseAt       string `json:"close_at"`
		Options       []struct {
			Label    string `json:"label"`
			Position int32  `json:"position"`
		} `json:"options"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Question == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_question", "Poll question is required")
		return
	}

	if len(input.Options) < 2 {
		errpkg.WriteError(w, http.StatusBadRequest, "too_few_options", "Poll requires at least 2 options")
		return
	}

	for i, opt := range input.Options {
		if opt.Label == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_option_label", fmt.Sprintf("Option %d label is required", i+1))
			return
		}
	}

	pollInput := PollInput{
		Question:      input.Question,
		IsMultiSelect: input.IsMultiSelect,
		Options:       make([]PollOptionInput, len(input.Options)),
	}

	for i, opt := range input.Options {
		pollInput.Options[i] = PollOptionInput{Label: opt.Label, Position: opt.Position}
	}

	if input.CloseAt != "" {
		if t, err := time.Parse(time.RFC3339, input.CloseAt); err == nil {
			pollInput.CloseAt = pgtype.Timestamptz{Time: t, Valid: true, InfinityModifier: pgtype.Finite}
		}
	}

	poll, err := h.Service.CreatePollForThread(r.Context(), threadID, user.UserID, pollInput)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	results, err := h.Service.GetPollResults(r.Context(), poll.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	data := map[string]any{
		"poll":    poll,
		"results": results,
	}

	if votes, err := h.Service.GetUserPollVotes(r.Context(), poll.ID, user.UserID); err == nil {
		optionIDs := make([]string, 0, len(votes))
		for _, v := range votes {
			optionIDs = append(optionIDs, v.OptionID.String())
		}

		data["user_votes"] = optionIDs
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// VotePollOption handles POST /api/v1/poll-options/{id}/vote.
// The URL param is the option_id (not the poll_id). The poll is resolved
// from the option. Ref: api-routes-catalog L612.
func (h *Handlers) VotePollOption(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	optionID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_option_id", "Invalid poll option ID")
		return
	}

	if err := h.Service.VotePollByOptionID(r.Context(), optionID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "voted"}})
}

// RetractPollVoteHandler handles DELETE /api/v1/poll-options/{id}/vote.
// The URL param is the option_id; the poll is resolved from the option.
// Ref: api-routes-catalog L613.
func (h *Handlers) RetractPollVoteHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	optionID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_option_id", "Invalid poll option ID")
		return
	}

	if err := h.Service.RetractPollVote(r.Context(), optionID, user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ---------------------------------------------------------------------------
// Tag Handlers
// ---------------------------------------------------------------------------

// CreateTag handles POST /api/v1/forum/tags.
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Name            string  `json:"name"`
		Slug            string  `json:"slug"`
		Description     string  `json:"description"`
		Rules           string  `json:"rules"`
		CoverImageID    *string `json:"cover_image_id"`
		ModeratorUserID *string `json:"moderator_user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	var coverImageID, moderatorUserID pgtype.UUID
	if input.CoverImageID != nil {
		if err := coverImageID.Scan(*input.CoverImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cover_image_id", "Invalid cover_image_id format")
			return
		}
	}

	if input.ModeratorUserID != nil {
		if err := moderatorUserID.Scan(*input.ModeratorUserID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_moderator_user_id", "Invalid moderator_user_id format")
			return
		}
	}

	tag, err := h.Service.CreateTag(r.Context(), CreateTagInput{
		Name:            input.Name,
		Slug:            input.Slug,
		Description:     input.Description,
		Rules:           input.Rules,
		CoverImageID:    coverImageID,
		ModeratorUserID: moderatorUserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// §12 stale-index fix: enqueue Meilisearch re-index for the new tag.
	h.enqueueSearchIndex("tag", "index", tag.ID)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": tag})
}

// ListTags handles GET /api/v1/forum/tags.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	tags, err := h.Service.ListTags(r.Context(), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tags})
}

// GetTagBySlug handles GET /api/v1/forum/tags/{slug}.
func (h *Handlers) GetTagBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	tag, err := h.Service.GetTagBySlug(r.Context(), slug)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tag})
}

// UpdateTag handles PATCH /api/v1/forum/tags/{id}.
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	var input struct {
		Name            string  `json:"name"`
		Slug            string  `json:"slug"`
		Description     string  `json:"description"`
		Rules           string  `json:"rules"`
		CoverImageID    *string `json:"cover_image_id"`
		ModeratorUserID *string `json:"moderator_user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	var coverImageID, moderatorUserID pgtype.UUID
	if input.CoverImageID != nil {
		if err := coverImageID.Scan(*input.CoverImageID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_cover_image_id", "Invalid cover image ID")
			return
		}
	}

	if input.ModeratorUserID != nil {
		if err := moderatorUserID.Scan(*input.ModeratorUserID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_moderator_user_id", "Invalid moderator user ID")
			return
		}
	}

	tag, err := h.Service.UpdateTag(r.Context(), id, UpdateTagInput{
		Name:            input.Name,
		Slug:            input.Slug,
		Description:     input.Description,
		Rules:           input.Rules,
		CoverImageID:    coverImageID,
		ModeratorUserID: moderatorUserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// §12 stale-index fix: enqueue Meilisearch re-index for the updated tag.
	h.enqueueSearchIndex("tag", "index", tag.ID)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tag})
}

// DeleteTag handles DELETE /api/v1/forum/tags/{id}.
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid tag ID")
		return
	}

	if err := h.Service.DeleteTag(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	// §12 stale-index fix: remove the deleted tag from the Meilisearch index.
	h.enqueueSearchIndex("tag", "delete", id)

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// AddThreadTag handles POST /api/v1/forum/{id}/tags/{tagId}.
func (h *Handlers) AddThreadTag(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	tagID, err := parseUUIDParam(r, "tagId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag ID")
		return
	}

	if err := h.Service.AddThreadTag(r.Context(), threadID, tagID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "added"}})
}

// AddThreadTagsBulk handles POST /api/v1/forum/{id}/tags.
// Accepts a JSON array of tag ids and assigns them all to the thread,
// enforcing the MaxTagsPerThread cap. Ref: api-routes-catalog L596.
func (h *Handlers) AddThreadTagsBulk(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	var input struct {
		TagIDs []string `json:"tag_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if len(input.TagIDs) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_tag_ids", "tag_ids array is required")
		return
	}

	tagIDs := make([]pgtype.UUID, 0, len(input.TagIDs))

	for _, idStr := range input.TagIDs {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag_id in list")
			return
		}

		tagIDs = append(tagIDs, id)
	}

	if err := h.Service.AddThreadTagsBulk(r.Context(), threadID, tagIDs); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "added"}})
}

// RemoveThreadTag handles DELETE /api/v1/forum/{id}/tags/{tagId}.
func (h *Handlers) RemoveThreadTag(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	tagID, err := parseUUIDParam(r, "tagId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_tag_id", "Invalid tag ID")
		return
	}

	if err := h.Service.RemoveThreadTag(r.Context(), threadID, tagID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ListThreadTags handles GET /api/v1/forum/{id}/tags.
// Returns the tags attached to a thread. Public — no auth required.
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (tags are
// first-class on discussions).
func (h *Handlers) ListThreadTags(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_thread_id", "Invalid thread ID")
		return
	}

	tags, err := h.Service.ListThreadTags(r.Context(), threadID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tags})
}

// syncThreadContent updates the backing contents row for a forum thread
// when its title or body changes. It preserves existing content metadata and
// re-queues embedding generation. Errors are logged, never propagated.
func (h *Handlers) syncThreadContent(ctx context.Context, userID, contentID pgtype.UUID, title, slug string, body []byte) {
	if h.content == nil {
		return
	}

	existing, err := h.content.GetContent(ctx, contentID)
	if err != nil {
		slog.Warn("forum: failed to fetch backing content for sync", "content_id", contentID.String(), "error", err)
		return
	}

	status := existing.Status
	if status == "" {
		status = content.StatusDraft
	}

	visibility := existing.Visibility
	if visibility == "" {
		visibility = content.VisibilityPublic
	}

	spaceID := existing.SpaceID
	if !spaceID.Valid {
		spaceID = existing.GeoSpaceID
	}

	fieldValues := existing.FieldValues
	if len(fieldValues) == 0 {
		fieldValues = []byte("{}")
	}

	seo := existing.SeoIndexable

	// Rule 12: recompute the backing content_hash from the new body so
	// duplicate detection stays accurate when a forum thread is edited.
	var contentHash *string

	if strings.TrimSpace(string(body)) != "" {
		hash := content.ComputeContentHash(string(body))
		contentHash = &hash
	}

	input := content.UpdateContentInput{
		Title:           title,
		Slug:            slug,
		Body:            string(body),
		BodyHtml:        forumBodyHTML(body),
		FieldValues:     fieldValues,
		ContentHash:     contentHash,
		SpaceID:         spaceID,
		GeoSpaceID:      existing.GeoSpaceID,
		HeaderImageID:   existing.HeaderImageID,
		FeaturedImageID: existing.FeaturedImageID,
		CanonicalUrl:    existing.CanonicalUrl,
		MetaTitle:       existing.MetaTitle,
		MetaDescription: existing.MetaDescription,
		OgImageUrl:      existing.OgImageUrl,
		SeoIndexable:    &seo,
		Status:          status,
		Visibility:      visibility,
		IsLocked:        &existing.IsLocked,
		IsPinned:        &existing.IsPinned,
		OrganizationID:  existing.OrganizationID,
		GuideCategory:   existing.GuideCategory,
		AIGeneratedAt:   existing.AiGeneratedAt,
		AIModel:         existing.AiModel,
		JourneyStage:    existing.JourneyStage,
		VisaRoute:       existing.VisaRoute,
		SourceCountry:   existing.SourceCountry,
		TargetCountry:   existing.TargetCountry,
	}

	if _, err := h.content.UpdateContent(ctx, contentID, userID, input, true); err != nil {
		slog.Warn("forum: failed to sync backing content for thread", "content_id", contentID.String(), "error", err)
		return
	}

	h.enqueueContentEmbedding(contentID)
}
