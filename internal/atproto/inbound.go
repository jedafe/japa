// Ref: japaconnect-engineering/sections/127-atproto-native.md (A5 — Jetstream inbound)
// Jetstream consumer. Subscribes to app.bsky.feed.post/like/follow commits,
// resolves their subjects against the local projection index, and normalizes
// matched events into comments / remote likes / remote follows plus §35
// notifications. Actionable events are ledgered in atproto_inbound in the
// same transaction as their domain writes; irrelevant traffic is dropped
// without a ledger row or dedup key (network-wide volume makes both
// unaffordable). Dedup authority is the (event_uri, operation, event_seq)
// unique key — the 72h Valkey marker is a post-commit advisory fast-path.
//
// Spec note: §127 says "filtered to member DIDs" — Jetstream's wantedDids
// filters on the event's repo DID, which would only echo our own commits.
// Inbound events come from remote repos, so the subscription is
// collections-only and the member check is the subject→index resolution.
package atproto

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

const (
	// jetstreamDedupTTL is the §127 dedup retention for inbound events.
	jetstreamDedupTTL = 72 * time.Hour
	// cursorCheckpointEvery throttles cursor writes — every event is a
	// valkey Set, so checkpoint roughly once per second of stream flow.
	cursorCheckpointEvery = 500 * time.Millisecond
	// reconnectMaxBackoff bounds the supervise loop's retry sleep.
	reconnectMaxBackoff = 30 * time.Second

	colFeedPost = "app.bsky.feed.post"
	colFeedLike = "app.bsky.feed.like"
	colFollow   = "app.bsky.graph.follow"
)

// jetstreamEvent is the v1 subscribe wire shape (uncompressed JSON).
type jetstreamEvent struct {
	DID    string `json:"did"`
	TimeUS int64  `json:"time_us"`
	Kind   string `json:"kind"`
	Commit *struct {
		Rev        string         `json:"rev"`
		Operation  string         `json:"operation"` // create | update | delete
		Collection string         `json:"collection"`
		Rkey       string         `json:"rkey"`
		Record     map[string]any `json:"record"`
		CID        string         `json:"cid"`
	} `json:"commit"`
}

// StartJetstreamConsumer supervises the consume loop until ctx cancels.
// Feature-gated on feature.atproto and a non-empty JC_ATPROTO_JETSTREAM_URL —
// both absent means the daemon simply never starts.
func (s *Service) StartJetstreamConsumer(ctx context.Context) {
	if s.cfg.JetstreamURL == "" || s.vk == nil {
		slog.Info("atproto: jetstream consumer disabled (no URL/valkey)")
		return
	}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		// §42 feature gate — re-polled each supervise iteration so toggling
		// the flag off disconnects and toggling on reconnects.
		if s.flags != nil && !s.flags.IsEnabled(ctx, "feature.atproto") {
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
				continue
			}
		}
		err := s.consume(ctx)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("atproto: jetstream disconnected, reconnecting", "error", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < reconnectMaxBackoff {
			backoff *= 2
		}
	}
}

// consume runs one connection's read loop until it breaks.
func (s *Service) consume(ctx context.Context) error {
	subURL, err := url.Parse(s.cfg.JetstreamURL)
	if err != nil {
		return fmt.Errorf("atproto: bad jetstream URL: %w", err)
	}
	q := subURL.Query()
	for _, col := range []string{colFeedPost, colFeedLike, colFollow} {
		q.Add("wantedCollections", col)
	}
	if cur, err := s.vk.Get(ctx, "jc:atp:jetstream:cursor").Result(); err == nil && cur != "" {
		q.Set("cursor", cur)
	}
	subURL.RawQuery = q.Encode()

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, subURL.String(), nil)
	if err != nil {
		return fmt.Errorf("atproto: jetstream dial: %w", err)
	}
	defer conn.Close()
	slog.Info("atproto: jetstream connected", "url", subURL.Host)

	var lastCursor atomic.Int64
	// Flush on exit — a disconnect shouldn't lose up to 500ms of cursor.
	defer s.flushCursor(context.Background(), lastCursor.Load())

	lastFlush := time.Now()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("atproto: jetstream read: %w", err)
		}
		var ev jetstreamEvent
		if err := json.Unmarshal(raw, &ev); err != nil || ev.Commit == nil {
			continue // malformed frame — never crash the loop
		}
		if err := s.handleCommit(ctx, &ev); err != nil {
			// Persistence failure — drop the connection. The cursor hasn't
			// advanced past this event, so reconnecting forces redelivery
			// instead of silently skipping it.
			return fmt.Errorf("atproto: commit handling: %w", err)
		}
		lastCursor.Store(ev.TimeUS)
		if time.Since(lastFlush) >= cursorCheckpointEvery {
			s.flushCursor(ctx, lastCursor.Load())
			lastFlush = time.Now()
		}
	}
}

func (s *Service) flushCursor(ctx context.Context, timeUS int64) {
	if timeUS <= 0 {
		return
	}
	_ = s.vk.Set(ctx, "jc:atp:jetstream:cursor", timeUS, 0).Err()
}

// notifyIntent defers a §35 notification until after the dispatch transaction
// commits — firing inside the tx would notify on work that can still roll
// back, and push/email fan-out never belongs inside a db transaction.
type notifyIntent struct {
	recipientID pgtype.UUID
	nType       string
	actorID     pgtype.UUID
	subjectType string
	subjectID   pgtype.UUID
	title       string
	body        string
	action      string
}

// handleCommit applies one commit event atomically: the ledger row and every
// domain write share a single transaction, so a crash can never leave
// partially-applied state or a stuck 'received' row. 'ignored' verdicts roll
// back — irrelevant upstream traffic is neither audited nor dedup-keyed
// (network-wide stream volume makes both unaffordable); replays re-evaluate
// cheaply. The Valkey marker is written post-commit as an advisory fast-path
// only — the ledger unique constraint is the authoritative dedupe.
//
// Returns a non-nil error only when persistence itself fails — the caller
// drops the connection so upstream redelivers from the cursor rather than
// silently skipping the event. Domain failures are ledgered 'error' and
// return nil; a panic is contained (skipped, loudly logged) so one poison
// event can't wedge the stream in a reconnect loop.
func (s *Service) handleCommit(ctx context.Context, ev *jetstreamEvent) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("atproto: jetstream event panic", "did", ev.DID, "panic", r)
			err = nil
		}
	}()

	c := ev.Commit
	uri := fmt.Sprintf("at://%s/%s/%s", ev.DID, c.Collection, c.Rkey)
	dk := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", uri, c.Operation, ev.TimeUS)))
	dedupKey := "jc:atp:in:dedup:" + hex.EncodeToString(dk[:])
	if got, gerr := s.vk.Get(ctx, dedupKey).Result(); gerr == nil && got != "" {
		return nil
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inbound tx begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := gen.New(tx)

	raw, _ := json.Marshal(c.Record)
	inID, err := q.InsertAtprotoInbound(ctx, gen.InsertAtprotoInboundParams{
		EventUri:   uri,
		EventSeq:   ev.TimeUS,
		Did:        ev.DID,
		Collection: c.Collection,
		Operation:  c.Operation,
		Payload:    raw,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already ledgered — drop
		}
		return fmt.Errorf("inbound ledger insert: %w", err)
	}

	status, msg, intents := s.dispatch(ctx, q, ev, uri)
	if status == "ignored" {
		return nil // rollback — irrelevant traffic is neither audited nor keyed
	}
	var errStr *string
	if msg != "" {
		errStr = &msg
	}
	if uerr := q.UpdateAtprotoInboundStatus(ctx, gen.UpdateAtprotoInboundStatusParams{
		ID: inID, Status: status, Error: errStr,
	}); uerr != nil {
		return fmt.Errorf("inbound status update: %w", uerr)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("inbound tx commit: %w", err)
	}

	_ = s.vk.Set(ctx, dedupKey, "1", jetstreamDedupTTL).Err()
	for _, in := range intents {
		if s.notifier == nil {
			break
		}
		if nerr := s.notifier.CreateNotification(ctx, in.recipientID, in.nType,
			"atproto_actor", in.actorID, in.subjectType, in.subjectID,
			in.title, in.body, in.action); nerr != nil {
			slog.Warn("atproto: inbound notification failed", "type", in.nType, "error", nerr)
		}
	}
	return nil
}

// dispatch routes a commit to its normalizer. Returns the ledger status, an
// optional detail string, and any notification intents for post-commit
// delivery. q is the caller's transaction — all domain writes share it.
func (s *Service) dispatch(ctx context.Context, q *gen.Queries, ev *jetstreamEvent, uri string) (string, string, []notifyIntent) {
	c := ev.Commit
	switch c.Collection {
	case colFeedPost:
		return s.dispatchPost(ctx, q, ev, uri)
	case colFeedLike:
		return s.dispatchLike(ctx, q, ev, uri)
	case colFollow:
		return s.dispatchFollow(ctx, q, ev, uri)
	}
	return "ignored", "unsupported collection", nil
}

// recStr pulls a dotted string field out of the generic record map.
func recStr(rec map[string]any, path ...string) string {
	var cur any = rec
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[p]
	}
	s, _ := cur.(string)
	return s
}

// maxRemoteCommentText caps remote reply text — far above the lexicon's 3000
// bytes but well under the 200KB TipTap body ceiling the local path enforces.
const maxRemoteCommentText = 50_000

// maxCommentDepth mirrors the local §49 reply cap (store.go: depth > 3 →
// ErrMaxDepth); remote replies deeper than the cap attach top-level instead.
const maxCommentDepth = 3

// dispatchPost normalizes app.bsky.feed.post commits: creates/updates that
// reply to a member post become remote comments; deletes tombstone them.
func (s *Service) dispatchPost(ctx context.Context, q *gen.Queries, ev *jetstreamEvent, uri string) (string, string, []notifyIntent) {
	c := ev.Commit
	switch c.Operation {
	case "create", "update":
		if c.Operation == "update" {
			if status, msg, handled := s.updateRemoteComment(ctx, q, uri, c.Record); handled {
				return status, msg, nil
			}
			// Update for a comment we never saw — the record is complete, so
			// fall through to the create path (missed-create catch-up).
		}
		return s.createRemoteComment(ctx, q, ev, uri)
	case "delete":
		// Tombstone + unwind the counters the create path bumped.
		prev, perr := q.GetAtprotoCommentByRemoteURI(ctx, &uri)
		if _, err := q.SoftDeleteAtprotoCommentByRemoteURI(ctx, &uri); err != nil {
			return "error", "comment delete: " + err.Error(), nil
		}
		if perr == nil && !prev.DeletedAt.Valid {
			s.bumpCounter(q.DecrementContentCommentCount(ctx, prev.ContentID))
			s.bumpCounter(q.DecrementLinkedContentCommentCount(ctx, prev.ContentID))
			if prev.ParentID.Valid {
				s.bumpCounter(q.DecrementCommentReplyCount(ctx, prev.ParentID))
			}
		}
		return "processed", "", nil
	}
	return "ignored", "unsupported operation", nil
}

// createRemoteComment normalizes a reply-create (or a create-fallback update)
// into a remote comment + notification intent.
func (s *Service) createRemoteComment(ctx context.Context, q *gen.Queries, ev *jetstreamEvent, uri string) (string, string, []notifyIntent) {
	c := ev.Commit
	parentURI := recStr(c.Record, "reply", "parent", "uri")
	if parentURI == "" {
		return "ignored", "not a reply", nil
	}
	// Reply targets: the parent is a member post → attach to its content;
	// otherwise the root might be ours (reply deeper in the thread).
	rec, err := q.GetAtprotoRecordByURI(ctx, parentURI)
	if err != nil {
		if root := recStr(c.Record, "reply", "root", "uri"); root != "" && root != parentURI {
			rec, err = q.GetAtprotoRecordByURI(ctx, root)
		}
	}
	if err != nil || !rec.ContentID.Valid {
		return "ignored", "reply target is not a member record", nil
	}
	actor, err := s.resolveRemoteActor(ctx, ev.DID)
	if err != nil {
		return "error", "remote actor: " + err.Error(), nil
	}
	body := recStr(c.Record, "text")
	if body == "" {
		return "rejected", "empty reply text", nil
	}
	if len(body) > maxRemoteCommentText {
		return "rejected", "reply text too large", nil
	}
	// Nest under a prior remote comment when the parent URI is one — but only
	// on the same content row, and only while the local depth cap allows;
	// deeper remote threads attach top-level instead of violating §49.
	var parentID pgtype.UUID
	depth := int32(0)
	if pc, perr := q.GetAtprotoCommentByRemoteURI(ctx, &parentURI); perr == nil &&
		pc.ContentID == rec.ContentID && !pc.DeletedAt.Valid && pc.Depth < maxCommentDepth {
		parentID = pc.ID
		depth = pc.Depth + 1
	}
	_, err = q.InsertAtprotoRemoteComment(ctx, gen.InsertAtprotoRemoteCommentParams{
		ContentID:            rec.ContentID,
		Body:                 proseMirrorDoc(body),
		Depth:                depth,
		RemoteAtprotoActorID: actor.ID,
		RemoteUri:            &uri,
		ParentID:             parentID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "ignored", "comment already ingested", nil
		}
		return "error", "comment insert: " + err.Error(), nil
	}
	// Mirror the local create path's denormalized counters (§49 BUG-008,
	// unified-discussion bubbling) — a trigger-free path must bump them.
	s.bumpCounter(q.IncrementContentCommentCount(ctx, rec.ContentID))
	s.bumpCounter(q.IncrementLinkedContentCommentCount(ctx, rec.ContentID))
	if parentID.Valid {
		s.bumpCounter(q.IncrementCommentReplyCount(ctx, parentID))
	}
	in := s.inboundIntent(ctx, q, rec, actor, "post_reply", "Bluesky reply", "replied to your post on Bluesky")
	if in == nil {
		return "processed", "", nil
	}
	return "processed", "", []notifyIntent{*in}
}

// updateRemoteComment refreshes an existing remote comment's body. The
// boolean reports whether the op was handled (true) or should fall through
// to the create path because no local row matched.
func (s *Service) updateRemoteComment(ctx context.Context, q *gen.Queries, uri string, record map[string]any) (string, string, bool) {
	body := recStr(record, "text")
	if body == "" {
		return "rejected", "empty reply text", true
	}
	if len(body) > maxRemoteCommentText {
		return "rejected", "reply text too large", true
	}
	n, err := q.UpdateAtprotoRemoteCommentBody(ctx, gen.UpdateAtprotoRemoteCommentBodyParams{
		RemoteUri: &uri, Body: proseMirrorDoc(body),
	})
	if err != nil {
		return "error", "comment update: " + err.Error(), true
	}
	if n == 0 {
		return "", "", false // never saw the create — treat as catch-up
	}
	return "processed", "", true
}

// proseMirrorDoc wraps remote plain text into the TipTap/ProseMirror doc
// shape comments.body stores.
func proseMirrorDoc(text string) []byte {
	docJSON, _ := json.Marshal(map[string]any{
		"type": "doc",
		"content": []any{map[string]any{
			"type":    "paragraph",
			"content": []any{map[string]any{"type": "text", "text": text}},
		}},
	})
	return docJSON
}

// bumpCounter logs denormalized-counter failures — a drift here silently
// desyncs feed cards and thread counters.
func (s *Service) bumpCounter(err error) {
	if err != nil {
		slog.Warn("atproto: inbound counter bump failed", "error", err)
	}
}

// dispatchLike normalizes app.bsky.feed.like commits whose subject resolves
// to a projected member record.
func (s *Service) dispatchLike(ctx context.Context, q *gen.Queries, ev *jetstreamEvent, uri string) (string, string, []notifyIntent) {
	c := ev.Commit
	switch c.Operation {
	case "create":
		subject := recStr(c.Record, "subject", "uri")
		if subject == "" {
			return "rejected", "missing subject", nil
		}
		rec, err := q.GetAtprotoRecordByURI(ctx, subject)
		if err != nil || !rec.ContentID.Valid {
			return "ignored", "like target is not a member record", nil
		}
		// Replay guard: a create replayed upstream with a fresh time_us still
		// carries the same record URI — an already-active like is a no-op, not
		// a second notification.
		if ex, xerr := q.GetAtprotoRemoteLikeByURI(ctx, uri); xerr == nil && !ex.UndoneAt.Valid {
			return "ignored", "like already active", nil
		}
		actor, err := s.resolveRemoteActor(ctx, ev.DID)
		if err != nil {
			return "error", "remote actor: " + err.Error(), nil
		}
		if _, err := q.UpsertAtprotoRemoteLike(ctx, gen.UpsertAtprotoRemoteLikeParams{
			AccountID:     rec.AccountID,
			RemoteActorID: actor.ID,
			LikeUri:       uri,
			ContentID:     rec.ContentID,
		}); err != nil {
			return "error", "like upsert: " + err.Error(), nil
		}
		in := s.inboundIntent(ctx, q, rec, actor, "reaction", "Bluesky like", "liked your post on Bluesky")
		if in == nil {
			return "processed", "", nil
		}
		return "processed", "", []notifyIntent{*in}
	case "delete":
		if _, err := q.UndoAtprotoRemoteLike(ctx, uri); err != nil {
			return "error", "like undo: " + err.Error(), nil
		}
		return "processed", "", nil
	}
	return "ignored", "unsupported operation", nil
}

// dispatchFollow normalizes app.bsky.graph.follow commits whose subject is a
// member DID.
func (s *Service) dispatchFollow(ctx context.Context, q *gen.Queries, ev *jetstreamEvent, uri string) (string, string, []notifyIntent) {
	c := ev.Commit
	switch c.Operation {
	case "create":
		subject := recStr(c.Record, "subject")
		if !strings.HasPrefix(subject, "did:") {
			return "rejected", "missing subject did", nil
		}
		acct, err := q.GetAtprotoAccountByDID(ctx, subject)
		if err != nil {
			return "ignored", "follow target is not a member", nil
		}
		if ex, xerr := q.GetAtprotoRemoteFollowByURI(ctx, uri); xerr == nil && !ex.UndoneAt.Valid {
			return "ignored", "follow already active", nil
		}
		actor, err := s.resolveRemoteActor(ctx, ev.DID)
		if err != nil {
			return "error", "remote actor: " + err.Error(), nil
		}
		if _, err := q.UpsertAtprotoRemoteFollow(ctx, gen.UpsertAtprotoRemoteFollowParams{
			AccountID:     acct.ID,
			RemoteActorID: actor.ID,
			FollowUri:     uri,
		}); err != nil {
			return "error", "follow upsert: " + err.Error(), nil
		}
		// Action lands on the member's own profile — remote actors have no
		// local page, and the frontend only navigates in-app paths.
		action := ""
		if owner, oerr := q.GetUserByID(ctx, acct.UserID); oerr == nil && owner.Username != "" {
			action = "/u/" + owner.Username
		}
		return "processed", "", []notifyIntent{{
			recipientID: acct.UserID, nType: "new_follower", actorID: actor.ID,
			subjectType: "user", subjectID: acct.UserID,
			title:  "Bluesky follower",
			body:   actorHandleBody(actor) + " followed you on Bluesky",
			action: action,
		}}
	case "delete":
		if _, err := q.UndoAtprotoRemoteFollow(ctx, uri); err != nil {
			return "error", "follow undo: " + err.Error(), nil
		}
		return "processed", "", nil
	}
	return "ignored", "unsupported operation", nil
}

// inboundIntent resolves the content owner and shapes the §35 notification
// for delivery after the dispatch transaction commits. Returns nil when the
// content or its author can't be resolved (the event still processes).
func (s *Service) inboundIntent(ctx context.Context, q *gen.Queries, rec gen.GetAtprotoRecordByURIRow, actor gen.AtprotoRemoteActor, nType, title, verb string) *notifyIntent {
	if !rec.ContentID.Valid {
		return nil
	}
	owner, err := q.GetContentByID(ctx, rec.ContentID)
	if err != nil || !owner.AuthorID.Valid {
		return nil
	}
	return &notifyIntent{
		recipientID: owner.AuthorID, nType: nType, actorID: actor.ID,
		subjectType: "content", subjectID: rec.ContentID,
		title:  title,
		body:   actorHandleBody(actor) + " " + verb,
		action: "/content/" + owner.Slug,
	}
}

// actorHandleBody renders the actor's @handle or a shortened DID for
// notification body text.
func actorHandleBody(actor gen.AtprotoRemoteActor) string {
	if actor.Handle != nil && *actor.Handle != "" {
		return "@" + *actor.Handle
	}
	return didShort(actor)
}

func didShort(actor gen.AtprotoRemoteActor) string {
	if len(actor.Did) > 24 {
		return actor.Did[:24] + "…"
	}
	return actor.Did
}
