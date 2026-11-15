// Ref: japaconnect-engineering/sections/127-atproto-native.md (A5 — Jetstream inbound)
// Jetstream consumer. Subscribes to app.bsky.feed.post/like/follow commits,
// resolves their subjects against the local projection index, and normalizes
// matched events into comments / remote likes / remote follows plus §35
// notifications. Every event is ledgered in atproto_inbound; dedup rides the
// (event_uri, operation) unique key plus a 72h Valkey fast-path.
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
		s.handleCommit(ctx, &ev)
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

// handleCommit applies one commit event: dedup → ledger → dispatch → status.
// Any upstream weirdness is contained here so the stream never stops on a
// single bad record.
func (s *Service) handleCommit(ctx context.Context, ev *jetstreamEvent) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("atproto: jetstream event panic", "did", ev.DID, "panic", r)
		}
	}()

	c := ev.Commit
	uri := fmt.Sprintf("at://%s/%s/%s", ev.DID, c.Collection, c.Rkey)

	// Valkey fast-path dedupe — replayed events short-circuit before the DB.
	// The seq is part of the material so a legit create→delete→create
	// sequence isn't swallowed; replays carry identical time_us.
	dk := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", uri, c.Operation, ev.TimeUS)))
	dedupKey := "jc:atp:in:dedup:" + hex.EncodeToString(dk[:])
	if ok, err := s.vk.SetNX(ctx, dedupKey, "1", jetstreamDedupTTL).Result(); err == nil && !ok {
		return
	}

	raw, _ := json.Marshal(c.Record)
	q := gen.New(s.db.Writer)
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
			return // ledger says duplicate — processed already
		}
		// Transient failure — roll the fast-path dedup key back so redelivery
		// can retry; without this the event is dropped for the 72h TTL.
		_ = s.vk.Del(ctx, dedupKey).Err()
		slog.Warn("atproto: inbound ledger insert failed", "uri", uri, "error", err)
		return
	}

	status, msg := s.dispatch(ctx, ev, uri)
	var errStr *string
	if msg != "" {
		errStr = &msg
	}
	if uerr := q.UpdateAtprotoInboundStatus(ctx, gen.UpdateAtprotoInboundStatusParams{
		ID: inID, Status: status, Error: errStr,
	}); uerr != nil {
		slog.Warn("atproto: inbound status update failed", "id", inID.String(), "error", uerr)
	}
}

// dispatch routes a commit to its normalizer. Returns the ledger status and
// an optional detail string recorded alongside it.
func (s *Service) dispatch(ctx context.Context, ev *jetstreamEvent, uri string) (string, string) {
	c := ev.Commit
	switch c.Collection {
	case colFeedPost:
		return s.dispatchPost(ctx, ev, uri)
	case colFeedLike:
		return s.dispatchLike(ctx, ev, uri)
	case colFollow:
		return s.dispatchFollow(ctx, ev, uri)
	}
	return "ignored", "unsupported collection"
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
func (s *Service) dispatchPost(ctx context.Context, ev *jetstreamEvent, uri string) (string, string) {
	c := ev.Commit
	q := gen.New(s.db.Writer)
	switch c.Operation {
	case "create", "update":
		if c.Operation == "update" {
			if status, msg, handled := s.updateRemoteComment(ctx, q, uri, c.Record); handled {
				return status, msg
			}
			// Update for a comment we never saw — the record is complete, so
			// fall through to the create path (missed-create catch-up).
		}
		return s.createRemoteComment(ctx, ev, uri)
	case "delete":
		// Tombstone + unwind the counters the create path bumped.
		prev, perr := q.GetAtprotoCommentByRemoteURI(ctx, &uri)
		if _, err := q.SoftDeleteAtprotoCommentByRemoteURI(ctx, &uri); err != nil {
			return "error", "comment delete: " + err.Error()
		}
		if perr == nil && !prev.DeletedAt.Valid {
			s.bumpCounter(q.DecrementContentCommentCount(ctx, prev.ContentID))
			s.bumpCounter(q.DecrementLinkedContentCommentCount(ctx, prev.ContentID))
			if prev.ParentID.Valid {
				s.bumpCounter(q.DecrementCommentReplyCount(ctx, prev.ParentID))
			}
		}
		return "processed", ""
	}
	return "ignored", "unsupported operation"
}

// createRemoteComment normalizes a reply-create (or a create-fallback update)
// into a remote comment + notification.
func (s *Service) createRemoteComment(ctx context.Context, ev *jetstreamEvent, uri string) (string, string) {
	c := ev.Commit
	q := gen.New(s.db.Writer)
	parentURI := recStr(c.Record, "reply", "parent", "uri")
	if parentURI == "" {
		return "ignored", "not a reply"
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
		return "ignored", "reply target is not a member record"
	}
	actor, err := s.resolveRemoteActor(ctx, ev.DID)
	if err != nil {
		return "error", "remote actor: " + err.Error()
	}
	body := recStr(c.Record, "text")
	if body == "" {
		return "rejected", "empty reply text"
	}
	if len(body) > maxRemoteCommentText {
		return "rejected", "reply text too large"
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
			return "ignored", "comment already ingested"
		}
		return "error", "comment insert: " + err.Error()
	}
	// Mirror the local create path's denormalized counters (§49 BUG-008,
	// unified-discussion bubbling) — a trigger-free path must bump them.
	s.bumpCounter(q.IncrementContentCommentCount(ctx, rec.ContentID))
	s.bumpCounter(q.IncrementLinkedContentCommentCount(ctx, rec.ContentID))
	if parentID.Valid {
		s.bumpCounter(q.IncrementCommentReplyCount(ctx, parentID))
	}
	s.notifyInbound(ctx, rec, actor, "post_reply", "Bluesky reply", "replied to your post on Bluesky")
	return "processed", ""
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
func (s *Service) dispatchLike(ctx context.Context, ev *jetstreamEvent, uri string) (string, string) {
	c := ev.Commit
	q := gen.New(s.db.Writer)
	switch c.Operation {
	case "create":
		subject := recStr(c.Record, "subject", "uri")
		if subject == "" {
			return "rejected", "missing subject"
		}
		rec, err := q.GetAtprotoRecordByURI(ctx, subject)
		if err != nil || !rec.ContentID.Valid {
			return "ignored", "like target is not a member record"
		}
		// Replay guard: a create replayed upstream with a fresh time_us still
		// carries the same record URI — an already-active like is a no-op, not
		// a second notification.
		if ex, xerr := q.GetAtprotoRemoteLikeByURI(ctx, uri); xerr == nil && !ex.UndoneAt.Valid {
			return "ignored", "like already active"
		}
		actor, err := s.resolveRemoteActor(ctx, ev.DID)
		if err != nil {
			return "error", "remote actor: " + err.Error()
		}
		if _, err := q.UpsertAtprotoRemoteLike(ctx, gen.UpsertAtprotoRemoteLikeParams{
			AccountID:     rec.AccountID,
			RemoteActorID: actor.ID,
			LikeUri:       uri,
			ContentID:     rec.ContentID,
		}); err != nil {
			return "error", "like upsert: " + err.Error()
		}
		s.notifyInbound(ctx, rec, actor, "reaction", "Bluesky like", "liked your post on Bluesky")
		return "processed", ""
	case "delete":
		if _, err := q.UndoAtprotoRemoteLike(ctx, uri); err != nil {
			return "error", "like undo: " + err.Error()
		}
		return "processed", ""
	}
	return "ignored", "unsupported operation"
}

// dispatchFollow normalizes app.bsky.graph.follow commits whose subject is a
// member DID.
func (s *Service) dispatchFollow(ctx context.Context, ev *jetstreamEvent, uri string) (string, string) {
	c := ev.Commit
	q := gen.New(s.db.Writer)
	switch c.Operation {
	case "create":
		subject := recStr(c.Record, "subject")
		if !strings.HasPrefix(subject, "did:") {
			return "rejected", "missing subject did"
		}
		acct, err := q.GetAtprotoAccountByDID(ctx, subject)
		if err != nil {
			return "ignored", "follow target is not a member"
		}
		if ex, xerr := q.GetAtprotoRemoteFollowByURI(ctx, uri); xerr == nil && !ex.UndoneAt.Valid {
			return "ignored", "follow already active"
		}
		actor, err := s.resolveRemoteActor(ctx, ev.DID)
		if err != nil {
			return "error", "remote actor: " + err.Error()
		}
		if _, err := q.UpsertAtprotoRemoteFollow(ctx, gen.UpsertAtprotoRemoteFollowParams{
			AccountID:     acct.ID,
			RemoteActorID: actor.ID,
			FollowUri:     uri,
		}); err != nil {
			return "error", "follow upsert: " + err.Error()
		}
		s.notifyAccount(ctx, acct.UserID, actor, "new_follower", "user", acct.UserID, "Bluesky follower", actorHandleBody(actor)+" followed you on Bluesky", "/atproto")
		return "processed", ""
	case "delete":
		if _, err := q.UndoAtprotoRemoteFollow(ctx, uri); err != nil {
			return "error", "follow undo: " + err.Error()
		}
		return "processed", ""
	}
	return "ignored", "unsupported operation"
}

// notifyInbound delivers a §35 notification to the member whose content was
// acted on. actor carries 'atproto_actor' provenance — the remote actor row's
// UUID, not a users row.
func (s *Service) notifyInbound(ctx context.Context, rec gen.GetAtprotoRecordByURIRow, actor gen.AtprotoRemoteActor, nType, title, verb string) {
	if s.notifier == nil || !rec.ContentID.Valid {
		return
	}
	owner, err := gen.New(s.db.Reader).GetContentByID(ctx, rec.ContentID)
	if err != nil || !owner.AuthorID.Valid {
		return
	}
	handle := didShort(actor)
	if actor.Handle != nil && *actor.Handle != "" {
		handle = "@" + *actor.Handle
	}
	body := handle + " " + verb
	action := "/content/" + owner.Slug
	s.notifyAccount(ctx, owner.AuthorID, actor, nType, "content", rec.ContentID, title, body, action)
}

func (s *Service) notifyAccount(ctx context.Context, recipientID pgtype.UUID, actor gen.AtprotoRemoteActor, nType, subjectType string, subjectID pgtype.UUID, title, body, action string) {
	if s.notifier == nil || !recipientID.Valid {
		return
	}
	if err := s.notifier.CreateNotification(ctx, recipientID, nType,
		"atproto_actor", actor.ID, subjectType, subjectID, title, body, action); err != nil {
		slog.Warn("atproto: inbound notification failed", "type", nType, "error", err)
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
