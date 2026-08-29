// Package forum implements the Forum Q&A / Solved system (Section 08).
//
// Nested threads with per-thread solved status, upvote/downvote voting,
// tag system with moderators, polls, anonymous posting, bookmarks,
// and reply view tracking. Solved threads are eligible for Google's
// Q&A rich-result structured-data treatment (Section 26).
package forum

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Vote values for forum_threads and forum_replies.
const (
	VoteDown int16 = -1
	VoteUp   int16 = 1
)

// Entity types for bookmarks and votes.
const (
	EntityTypeThread = "thread"
	EntityTypeReply  = "reply"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// DefaultEditWindowMinutes is the default editable window after posting.
const DefaultEditWindowMinutes = 30

// MaxTagsPerThread is the per-post tag cap enforced at the handler/service
// layer. Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Tags).
const MaxTagsPerThread = 5

// Cache and view-count flush tuning.
const (
	jsonLDCacheTTL    = 5 * time.Minute
	viewCountCacheTTL = 10 * time.Minute
	viewFlushInterval = 30 * time.Second
	viewFlushTimeout  = 10 * time.Second
	valkeyOpTimeout   = 2 * time.Second
)

// permissionChecker is the subset of rbac.RBAC the forum service needs to
// resolve runtime OP/moderator scoping (Section 08 "OP or moderator") and
// the _any moderator-override nodes (§26). Local interface so forum does not
// import rbac.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// permCacheInvalidator invalidates a user's permission cache after a
// capability-granting mutation (Rule 6). Local interface so forum does not
// import rbac/valkey directly.
type permCacheInvalidator interface {
	InvalidatePermCache(ctx context.Context, userID pgtype.UUID) error
}

// webhookEmitter fans a forum.* event out to all subscribed webhook
// endpoints for the tenant (fire-and-forget). Ref: Section 12 (webhooks).
type webhookEmitter interface {
	EmitEvent(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any)
}

// spaceBroadcaster publishes a live update to a space's WebSocket room
// (Section 10). Used for poll/vote count live updates. The tenantID scopes
// the subscription so cross-tenant space ID collisions cannot leak events
// (Rule 4: tenant-scoped). Ref: Section 08.
type spaceBroadcaster interface {
	BroadcastToSpace(tenantID, spaceID string, event WSEvent)
}

// WSEvent is the minimal event shape the forum service emits. Kept local to
// avoid importing the websocket package (which would create a cycle via
// server wiring).
type WSEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// Service provides forum operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	settings forumSettingsReader

	// Optional collaborators injected after construction. Each is nil-safe:
	// when unset the service skips the corresponding cross-cutting behavior
	// (RBAC scoping, perm-cache invalidation, webhook emit, WS broadcast).
	rbac        permissionChecker
	permCache   permCacheInvalidator
	webhooks    webhookEmitter
	broadcaster spaceBroadcaster
	valkey      valkeycompat.Cmdable
	asynq       *asynq.Client
	notifier    NotificationDispatcher
	amaQuestions AMAQuestionHandler

	// In-memory JSON-LD cache for solved threads. Used only when Valkey is
	// not available so the service still degrades gracefully.
	jsonLDCache sync.Map

	// viewChan is the in-memory fallback for view-count increments when
	// Valkey is not attached.
	viewChan chan viewIncrement

	// viewKeys tracks the Valkey view-count keys that need flushing to the
	// database. Guarded by viewKeysMu.
	viewKeysMu sync.Mutex
	viewKeys   map[string]struct{}

	// viewFlushOnce ensures the periodic flush goroutine is started exactly
	// once after the Valkey client is injected.
	viewFlushOnce sync.Once
}

// jsonLDCacheEntry holds a cached JSON-LD string and its expiry time.
type jsonLDCacheEntry struct {
	jsonLd  string
	expires time.Time
}

// viewIncrement is a single view-count increment queued for batch processing.
type viewIncrement struct {
	threadID pgtype.UUID
	tenantID pgtype.UUID
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	s := &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
			db:    database,
		},
		db:       database,
		viewChan: make(chan viewIncrement, 256),
	}
	go s.drainViewIncrements()
	return s
}

// SetRBAC injects the permission checker used for OP/moderator scoping
// (Section 08) and the _any moderator-override nodes (§26).
func (s *Service) SetRBAC(r permissionChecker) { s.rbac = r }

// SetPermCacheInvalidator injects the Rule 6 cache invalidator used after
// tag moderator assignment.
func (s *Service) SetPermCacheInvalidator(p permCacheInvalidator) { s.permCache = p }

// SetWebhookEmitter injects the forum.* event fan-out (Section 12).
func (s *Service) SetWebhookEmitter(w webhookEmitter) { s.webhooks = w }

// SetSpaceBroadcaster injects the WebSocket space broadcaster for live
// poll/vote updates (Section 10).
func (s *Service) SetSpaceBroadcaster(b spaceBroadcaster) { s.broadcaster = b }

// SetValkey injects the Valkey client for distributed caching (view counts
// and JSON-LD). Optional — when nil, in-memory fallbacks are used. When a
// non-nil client is provided, a background worker is started to flush
// batched view counts into the database every 30 seconds.
func (s *Service) SetValkey(c valkeycompat.Cmdable) {
	s.valkey = c
	if c != nil {
		s.viewFlushOnce.Do(func() { go s.runViewCountFlusher() })
	}
}

// SetAsynqClient injects the Asynq client for fire-and-forget fan-out tasks
// (content:fan_out_to_feeds, search:index_entity). Optional — when nil,
// fan-out is skipped.
func (s *Service) SetAsynqClient(c *asynq.Client) { s.asynq = c }

// NotificationDispatcher dispatches reply-to-author notifications
// (Section 35). Fire-and-forget per Rule 8. Matches the
// commentNotificationAdapter signature in server.go.
type NotificationDispatcher interface {
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

// SetNotificationDispatcher injects the notification adapter for reply-to-author
// notifications (BUG-013, Section 35). Optional — when nil, notifications are
// skipped.
func (s *Service) SetNotificationDispatcher(n NotificationDispatcher) { s.notifier = n }

// AMAQuestionHandler is the subset of the AMA service the forum needs to
// enforce §88 question_approval_required. When a reply is created on an AMA
// thread with question_approval_required = true and the reply author is not
// the host, the forum service calls MarkPending to hold the question for
// host approval.
type AMAQuestionHandler interface {
	MarkReplyPending(ctx context.Context, replyID, tenantID pgtype.UUID) error
}

// SetAMAQuestionHandler injects the AMA question-approval hook (§88).
// Optional — when nil, question_approval_required is not enforced.
func (s *Service) SetAMAQuestionHandler(h AMAQuestionHandler) { s.amaQuestions = h }

// emitWebhook fires a forum.* event fire-and-forget (Rule 8). Errors are
// never returned to the caller; a missing emitter is a no-op.
func (s *Service) emitWebhook(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any) {
	if s.webhooks == nil {
		return
	}
	s.webhooks.EmitEvent(ctx, tenantID, eventType, payload)
}

// broadcastSpace publishes a live update to a space's WebSocket room. No-op when
// the broadcaster is unset or the space id is empty. The tenantID scopes the
// subscription (Rule 4: tenant-scoped).
func (s *Service) broadcastSpace(tenantID, spaceID pgtype.UUID, eventType string, payload any) {
	if s.broadcaster == nil || !spaceID.Valid {
		return
	}
	tenantStr := ""
	if tenantID.Valid {
		tenantStr = tenantID.String()
	}
	s.broadcaster.BroadcastToSpace(tenantStr, spaceID.String(), WSEvent{Type: eventType, Payload: payload})
}

// drainViewIncrements is the single background worker that processes
// view-count increments from the shared channel. Used only as a fallback when
// no Valkey client is attached.
func (s *Service) drainViewIncrements() {
	for inc := range s.viewChan {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.store.incrementThreadViewCount(ctx, inc.threadID, inc.tenantID, 1); err != nil { // #nosec G118 -- background worker, no request context
			slog.Warn("forum: failed to increment thread view count",
				"thread_id", inc.threadID.String(), "error", err)
		}
		cancel()
	}
}

// runViewCountFlusher periodically flushes batched view counts from Valkey to
// the database (valkey-keys-catalog.md: jc:forum:views:{tenant_id}:{thread_id}).
func (s *Service) runViewCountFlusher() {
	ticker := time.NewTicker(viewFlushInterval)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), viewFlushTimeout)
		s.flushViewCounts(ctx)
		cancel()
	}
}

// enqueueViewIncrement buffers a view count in Valkey and records the key so
// the flush worker will persist it. Errors are logged but not returned.
func (s *Service) enqueueViewIncrement(ctx context.Context, threadID, tenantID pgtype.UUID) {
	if s.valkey == nil {
		return
	}
	key := valkey.MustKey("forum:views:%s:%s", tenantID.String(), threadID.String())
	pipe := s.valkey.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, viewCountCacheTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Warn("forum: failed to buffer view count in valkey", "key", key, "error", err)
		return
	}

	s.viewKeysMu.Lock()
	if s.viewKeys == nil {
		s.viewKeys = make(map[string]struct{})
	}
	s.viewKeys[key] = struct{}{}
	s.viewKeysMu.Unlock()
}

// flushViewCounts reads every buffered view-count key this service has
// recorded, removes it from Valkey, and applies the accumulated delta to the
// database in a single UPDATE per thread.
func (s *Service) flushViewCounts(ctx context.Context) {
	s.viewKeysMu.Lock()
	keys := make([]string, 0, len(s.viewKeys))
	for k := range s.viewKeys {
		keys = append(keys, k)
	}
	s.viewKeys = make(map[string]struct{})
	s.viewKeysMu.Unlock()

	for _, key := range keys {
		countStr, err := s.valkey.GetDel(ctx, key).Result()
		if errors.Is(err, valkeycompat.Nil) {
			continue
		}
		if err != nil {
			slog.Warn("forum: failed to read view count from valkey", "key", key, "error", err)
			continue
		}

		count, err := strconv.ParseInt(countStr, 10, 32)
		if err != nil || count <= 0 {
			continue
		}

		// Key pattern: jc:forum:views:{tenant_id}:{thread_id}
		parts := strings.Split(key, ":")
		if len(parts) != 5 {
			continue
		}

		var tenantID, threadID pgtype.UUID
		if err := tenantID.Scan(parts[3]); err != nil {
			continue
		}
		if err := threadID.Scan(parts[4]); err != nil {
			continue
		}

		if err := s.store.incrementThreadViewCount(ctx, threadID, tenantID, int32(count)); err != nil {
			slog.Warn("forum: failed to flush view count to database",
				"thread_id", threadID.String(), "delta", count, "error", err)
		}
	}
}

// getJSONLDCache returns a cached JSON-LD string for the thread if fresh.
// Checks the cataloged Valkey key first, then falls back to an in-memory map
// when Valkey is unavailable.
func (s *Service) getJSONLDCache(ctx context.Context, threadID pgtype.UUID) (string, bool) {
	if s.valkey != nil {
		key := valkey.MustKey("forum:jsonld:%s", threadID.String())
		val, err := s.valkey.Get(ctx, key).Result()
		if err == nil && val != "" {
			return val, true
		}
		if err != nil && !errors.Is(err, valkeycompat.Nil) {
			slog.Warn("forum: failed to read json-ld cache", "key", key, "error", err)
		}
	}

	v, ok := s.jsonLDCache.Load(threadID)
	if !ok {
		return "", false
	}
	entry := v.(jsonLDCacheEntry)
	if time.Now().After(entry.expires) {
		s.jsonLDCache.Delete(threadID)
		return "", false
	}
	return entry.jsonLd, true
}

// setJSONLDCache stores a JSON-LD string for the thread with the standard TTL.
// Uses the cataloged Valkey key when available; otherwise the in-memory map.
func (s *Service) setJSONLDCache(ctx context.Context, threadID pgtype.UUID, jsonLd string) {
	if s.valkey != nil {
		key := valkey.MustKey("forum:jsonld:%s", threadID.String())
		if err := s.valkey.SetEX(ctx, key, jsonLd, jsonLDCacheTTL).Err(); err != nil {
			slog.Warn("forum: failed to write json-ld cache", "key", key, "error", err)
		}
		return
	}

	s.jsonLDCache.Store(threadID, jsonLDCacheEntry{
		jsonLd:  jsonLd,
		expires: time.Now().Add(jsonLDCacheTTL),
	})
}

// deleteJSONLDCache clears the cached JSON-LD so the next GET can regenerate it
// with the current accepted answer.
func (s *Service) deleteJSONLDCache(ctx context.Context, threadID pgtype.UUID) {
	if s.valkey != nil {
		key := valkey.MustKey("forum:jsonld:%s", threadID.String())
		if err := s.valkey.Del(ctx, key).Err(); err != nil {
			slog.Warn("forum: failed to delete json-ld cache", "key", key, "error", err)
		}
		return
	}
	s.jsonLDCache.Delete(threadID)
}

// invalidatePermCache wraps the perm cache invalidator with nil-safety.
func (s *Service) invalidatePermCache(ctx context.Context, userID pgtype.UUID) {
	if s.permCache == nil || !userID.Valid {
		return
	}
	if err := s.permCache.InvalidatePermCache(ctx, userID); err != nil {
		// Log but don't fail — cache invalidation is best-effort.
		_ = ctx
	}
}
