// Package forum implements the Forum Q&A / Solved system (Section 08).
//
// Nested threads with per-thread solved status, upvote/downvote voting,
// tag system with moderators, polls, anonymous posting, bookmarks,
// and reply view tracking. Solved threads are eligible for Google's
// Q&A rich-result structured-data treatment (Section 26).
package forum

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
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

// permissionChecker is the subset of rbac.RBAC the forum service needs to
// resolve runtime OP/moderator scoping (Section 08 "OP or moderator") and
// the _any moderator-override nodes (§26). Local interface so forum does
// not import rbac.
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
	rbac       permissionChecker
	permCache  permCacheInvalidator
	webhooks   webhookEmitter
	broadcaster spaceBroadcaster
	valkey     valkeycompat.Cmdable
	asynq      *asynq.Client
	notifier   NotificationDispatcher

	// BUG-020: in-memory JSON-LD cache for solved threads.
	jsonLDCache sync.Map

	// BUG-021: shared buffered channel for view-count increments.
	viewChan chan viewIncrement
}

// jsonLDCacheTTL is how long a cached JSON-LD entry is considered fresh.
const jsonLDCacheTTL = 5 * time.Minute

// viewIncrementBufferSize is the capacity of the view-increment channel.
const viewIncrementBufferSize = 256

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
		viewChan: make(chan viewIncrement, viewIncrementBufferSize),
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

// SetValkey injects the Valkey client for distributed caching (view counts,
// rate limiting). Optional — when nil, in-memory fallbacks are used.
func (s *Service) SetValkey(c valkeycompat.Cmdable) { s.valkey = c }

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

// emitWebhook fires a forum.* event fire-and-forget (Rule 8). Errors are
// never returned to the caller; a missing emitter is a no-op.
func (s *Service) emitWebhook(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any) {
	if s.webhooks == nil {
		return
	}
	s.webhooks.EmitEvent(ctx, tenantID, eventType, payload)
}

// broadcastSpace publishes a live update to a space's WS room. No-op when
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
// view-count increments from the shared channel (BUG-021).
func (s *Service) drainViewIncrements() {
	for inc := range s.viewChan {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.store.incrementThreadViewCount(ctx, inc.threadID, inc.tenantID); err != nil { // #nosec G118 -- background worker, no request context
			slog.Warn("forum: failed to increment thread view count",
				"thread_id", inc.threadID.String(), "error", err)
		}
		cancel()
	}
}

// enqueueViewIncrement sends a view-count increment to the background worker
// (BUG-021). Non-blocking — if the channel is full, the increment is dropped.
func (s *Service) enqueueViewIncrement(threadID, tenantID pgtype.UUID) {
	select {
	case s.viewChan <- viewIncrement{threadID: threadID, tenantID: tenantID}:
	default:
	}
}

// getJSONLDCache returns a cached JSON-LD string for the thread if fresh.
// BUG-020: avoids a synchronous DB round-trip for the top-reply fetch.
func (s *Service) getJSONLDCache(threadID pgtype.UUID) (string, bool) {
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
func (s *Service) setJSONLDCache(threadID pgtype.UUID, jsonLd string) {
	s.jsonLDCache.Store(threadID, jsonLDCacheEntry{
		jsonLd:  jsonLd,
		expires: time.Now().Add(jsonLDCacheTTL),
	})
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
