package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M2 — inbound dispatch)
// ProcessInbound runs one ledgered, signature-verified activity to ground
// truth. Dispatch happens inside a single transaction (mirroring §127
// handleCommit): the domain writes are idempotent at the constraint layer,
// so a task replay after a crash converges instead of duplicating.
// Notifications and delivery enqueues are post-commit intents — never
// fire-and-forget inside the tx.
//
// Signer discipline: the HTTP-Signature signer is resolved by keyId before
// this runs; every state-changing activity must declare actor == signer —
// a shared-inbox member must never borrow another actor's identity.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/notifications"
)

// Notifier is the narrow §35 seam — implemented by notifications.Service.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// SetNotifier injects the notifications seam for inbound events.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// maxRemoteCommentText bounds a remote reply's text (chars, post-strip).
const maxRemoteCommentText = 8000

// maxCommentDepth mirrors §49's nesting cap — deeper remote threads attach
// top-level instead of violating the invariant.
const maxCommentDepth = 3

// notifyIntent defers a notification until after the dispatch tx commits.
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

// dispatchResult is what one activity produced inside the tx: the ledger
// status to stamp, an optional reason, deferred notifications, and a
// delivery row that needs an asynq kick once committed.
type dispatchResult struct {
	status    string // processed | ignored | rejected
	reason    string
	intents   []notifyIntent
	deliverID pgtype.UUID // optional — a delivery row just created (Accept)
}

// ProcessInbound dispatches one federation_inbound row. It returns nil
// when the row reaches (or already held) a terminal status; a non-nil
// error means the asynq task should retry (the row stays 'received').
func (s *Service) ProcessInbound(ctx context.Context, inboundID pgtype.UUID) error {
	row, err := s.store.inboundByID(ctx, inboundID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // ledger row pruned — nothing to do
		}
		return err
	}
	if row.Status != "received" {
		return nil // replayed task — already terminal
	}
	var act map[string]any
	if err := json.Unmarshal(row.Payload, &act); err != nil {
		return s.finishInbound(ctx, row.ID, "rejected", "malformed activity json", nil, pgtype.UUID{})
	}

	// The signer is the activity's author-of-record for state changes.
	var signer *gen.RemoteActor
	if row.ActorUrl != nil && *row.ActorUrl != "" {
		signer, err = s.resolveRemoteActor(ctx, *row.ActorUrl)
		if err != nil {
			if errors.Is(err, ErrRemoteBlocked) {
				return s.finishInbound(ctx, row.ID, "rejected", "signer instance blocked", nil, pgtype.UUID{})
			}
			return fmt.Errorf("resolve signer: %w", err) // retryable fetch failure
		}
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inbound tx: %w", err)
	}
	defer tx.Rollback(ctx)
	res := s.dispatch(ctx, gen.New(tx), act, signer)
	// An internal write failure must not commit: the tx has partial
	// mutations by definition, and stamping 'error' would strand the
	// half-applied state with no retry (the status CHECK excludes
	// 'error' anyway). Roll back and let asynq replay the whole
	// activity — every write is idempotent, so replays converge.
	if res.status == "error" {
		return fmt.Errorf("inbound dispatch: %s", res.reason)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("inbound commit: %w", err)
	}
	return s.finishInbound(ctx, row.ID, res.status, res.reason, res.intents, res.deliverID)
}

// finishInbound stamps the ledger row, fires deferred notifications, and
// enqueues any delivery the dispatch produced. Notifications are
// best-effort post-commit — a lost ping must not replay the activity.
func (s *Service) finishInbound(ctx context.Context, id pgtype.UUID, status, reason string, intents []notifyIntent, deliverID pgtype.UUID) error {
	var errPtr *string
	if reason != "" {
		errPtr = &reason
	}
	if err := s.store.updateInboundStatus(ctx, id, status, errPtr); err != nil {
		return fmt.Errorf("inbound status update: %w", err)
	}
	for _, in := range intents {
		if s.notifier == nil {
			break
		}
		if nerr := s.notifier.CreateNotification(ctx, in.recipientID, in.nType,
			"remote_actor", in.actorID, in.subjectType, in.subjectID, in.title, in.body, in.action); nerr != nil {
			slog.Warn("federation: inbound notification failed", "type", in.nType, "error", nerr)
		}
	}
	if deliverID.Valid {
		if err := s.enqueueDelivery(ctx, deliverID); err != nil {
			slog.Warn("federation: delivery enqueue failed — sweeper picks up pending rows", "delivery_id", deliverID, "error", err)
		}
	}
	return nil
}

// dispatch routes the activity. All writes run inside the caller's tx.
func (s *Service) dispatch(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	typ := apStr(act["type"])
	actorID := apActorID(act)
	if signer != nil && actorID != "" && !strings.EqualFold(actorID, signer.ActorUrl) {
		return dispatchResult{status: "rejected", reason: "activity actor does not match signer"}
	}
	switch typ {
	case "Follow":
		return s.dispatchFollow(ctx, q, act, signer)
	case "Accept", "Reject":
		return s.dispatchAcceptReject(ctx, q, act, signer)
	case "Undo":
		return s.dispatchUndo(ctx, q, act, signer)
	case "Like":
		return s.dispatchLike(ctx, q, act, signer)
	case "Announce":
		return s.dispatchAnnounce(ctx, q, act, signer)
	case "Create":
		return s.dispatchCreate(ctx, q, act, signer)
	case "Update":
		return s.dispatchUpdate(ctx, q, act, signer)
	case "Delete":
		return s.dispatchDelete(ctx, q, act, signer)
	default:
		return dispatchResult{status: "ignored", reason: "unsupported activity type " + typ}
	}
}

// localActorForObject resolves object → the local actor it addresses.
// Only our actor-URL shape is a valid Follow target, and the profile join
// keeps suspended/deleted members out (users.status = 'active').
func (s *Service) localActorForObject(ctx context.Context, q *gen.Queries, objectID string) (gen.FederatedActor, gen.GetActorProfileByUsernameRow, error) {
	prefix := s.apiBase() + "/federation/actors/"
	username, ok := strings.CutPrefix(objectID, prefix)
	if !ok || username == "" || strings.Contains(username, "/") {
		return gen.FederatedActor{}, gen.GetActorProfileByUsernameRow{}, fmt.Errorf("object %q is not a local actor", objectID)
	}
	actor, err := q.GetFederatedActorByUsername(ctx, username)
	if err != nil {
		return gen.FederatedActor{}, gen.GetActorProfileByUsernameRow{}, err
	}
	profile, err := q.GetActorProfileByUsername(ctx, username)
	if err != nil {
		return gen.FederatedActor{}, gen.GetActorProfileByUsernameRow{}, err
	}
	return actor, profile, nil
}

// dispatchFollow handles a remote Follow of a local actor (§126): upsert
// the federation_follows row, auto-Accept unless the member approves
// followers manually, mirror the follows provenance row, notify.
func (s *Service) dispatchFollow(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned follow"}
	}
	actor, profile, err := s.localActorForObject(ctx, q, apObjectID(act))
	if err != nil {
		return dispatchResult{status: "ignored", reason: "follow target is not a federated local actor"}
	}
	if actor.Status != "active" {
		return dispatchResult{status: "ignored", reason: "target actor not active"}
	}

	status := "accepted"
	if actor.ManuallyApprovesFollowers {
		status = "pending"
	}
	follow, err := q.UpsertFederationFollow(ctx, gen.UpsertFederationFollowParams{
		LocalActorID:  actor.ID,
		RemoteActorID: signer.ID,
		ApActivityID:  stringPtrOrNil(apStr(act["id"])),
		Status:        status,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "follow upsert: " + err.Error()}
	}

	// Replays and re-Follows converge: an undone edge stays undone.
	if follow.Status == "undone" {
		return dispatchResult{status: "ignored", reason: "follow was undone"}
	}
	if status == "pending" {
		return dispatchResult{
			status:  "processed",
			intents: []notifyIntent{s.followIntent(actor, profile, *signer)},
		}
	}

	// accepted — mirror the §49 follower edge and emit Accept. Mastodon
	// sends Accept for every honored Follow, so the delivery fires on
	// replays too; the notification lands only when the edge is new.
	n, err := q.InsertRemoteFollow(ctx, gen.InsertRemoteFollowParams{
		RemoteActorID: signer.ID,
		UserID:        actor.UserID,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "follows insert: " + err.Error()}
	}
	deliverID, err := s.insertAcceptDelivery(ctx, q, actor, signer, act)
	if err != nil {
		return dispatchResult{status: "error", reason: "accept delivery: " + err.Error()}
	}
	res := dispatchResult{status: "processed", deliverID: deliverID}
	if n > 0 {
		res.intents = []notifyIntent{s.followIntent(actor, profile, *signer)}
	}
	return res
}

// insertAcceptDelivery builds the Accept{Follow} and ledgers it for the
// delivery worker. The embedded object is the verbatim incoming Follow —
// remote servers correlate on the original activity id.
func (s *Service) insertAcceptDelivery(ctx context.Context, q *gen.Queries, actor gen.FederatedActor, remote *gen.RemoteActor, followAct map[string]any) (pgtype.UUID, error) {
	return s.insertFollowResponseDelivery(ctx, q, "Accept", actor, remote, followAct)
}

// insertRejectDelivery is the member-driven counterpart — Reject{Follow}
// when a pending request is declined.
func (s *Service) insertRejectDelivery(ctx context.Context, q *gen.Queries, actor gen.FederatedActor, remote *gen.RemoteActor, followAct map[string]any) (pgtype.UUID, error) {
	return s.insertFollowResponseDelivery(ctx, q, "Reject", actor, remote, followAct)
}

// insertFollowResponseDelivery ledgers one Accept|Reject{Follow} row.
func (s *Service) insertFollowResponseDelivery(ctx context.Context, q *gen.Queries, typ string, actor gen.FederatedActor, remote *gen.RemoteActor, followAct map[string]any) (pgtype.UUID, error) {
	inbox := remote.InboxUrl
	if remote.SharedInboxUrl != nil && *remote.SharedInboxUrl != "" {
		inbox = remote.SharedInboxUrl
	}
	if inbox == nil || *inbox == "" {
		return pgtype.UUID{}, fmt.Errorf("remote actor has no inbox")
	}
	responseID := actor.ActorUrl + "#" + lowerType(typ) + "s/" + followActivityRef(followAct)
	payload, _ := json.Marshal(map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       responseID,
		"type":     typ,
		"actor":    actor.ActorUrl,
		"object":   followAct,
		"to":       []string{remote.ActorUrl},
	})
	row, err := q.InsertFederationDelivery(ctx, gen.InsertFederationDeliveryParams{
		ActivityApID:  responseID,
		ActivityType:  typ,
		ActivityJson:  payload,
		LocalActorID:  actor.ID,
		RemoteActorID: remote.ID,
		InboxUrl:      *inbox,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil // response already ledgered — replay no-ops
	}
	if err != nil {
		return pgtype.UUID{}, err
	}
	return row.ID, nil
}

// followActivityRef keys the Accept/Reject delivery on the Follow's
// identity. Id-less Follows (several implementations omit @id) fall back
// to a payload hash so distinct Follows sharing a shared inbox don't
// collide on one "#accepts/" key — replays still dedup because a Go map
// marshals canonically.
func followActivityRef(followAct map[string]any) string {
	if id := apStr(followAct["id"]); id != "" {
		return id
	}
	raw, _ := json.Marshal(followAct)
	sum := sha256.Sum256(raw)
	return "sha256-" + hex.EncodeToString(sum[:16])
}

// followIntent notifies the member of a new remote follower/request.
func (s *Service) followIntent(actor gen.FederatedActor, profile gen.GetActorProfileByUsernameRow, remote gen.RemoteActor) notifyIntent {
	return notifyIntent{
		recipientID: actor.UserID,
		nType:       notifications.TypeNewFollower,
		actorID:     remote.ID,
		subjectType: "user",
		subjectID:   actor.UserID,
		title:       "New fediverse follower",
		body:        remoteHandle(remote) + " followed you via the fediverse",
		action:      "/@" + profile.Username,
	}
}

// remoteHandle renders the actor's acct for notification text.
func remoteHandle(remote gen.RemoteActor) string {
	if remote.Acct != nil && *remote.Acct != "" {
		return "@" + *remote.Acct
	}
	if u, err := url.Parse(remote.ActorUrl); err == nil {
		return "@" + u.Hostname()
	}
	return remote.ActorUrl
}

// ---------------------------------------------------------------------------
// Member follow-request resolution (/me/federation/follow-requests, §126)
// ---------------------------------------------------------------------------

// ListFollowRequests returns the member's pending remote follow requests —
// the rows manual approval (profile_visibility='followers') parks on.
func (s *Service) ListFollowRequests(ctx context.Context, userID pgtype.UUID) ([]gen.ListPendingFederationFollowsByUserRow, error) {
	return s.store.pendingFollowRequests(ctx, userID)
}

// ResolveFollowRequest accepts or rejects one pending remote follow. The
// status='pending' predicate is the whole authorization: a resolved or
// nonexistent edge updates zero rows → ErrNotFound, so double-clicks and
// replayed resolutions can't resurrect or re-notify. Accept mirrors the
// local follows edge and ledgers Accept{Follow}; reject ledgers
// Reject{Follow}. Both embed the verbatim Follow so the remote correlates.
func (s *Service) ResolveFollowRequest(ctx context.Context, userID, remoteActorID pgtype.UUID, accept bool) error {
	actor, err := s.store.actorByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if actor.Status != "active" {
		return ErrNotFound // opted out — pending edges are unresolvable
	}
	remote, err := s.store.remoteActorByID(ctx, remoteActorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	follow, err := s.store.followByPair(ctx, actor.ID, remoteActorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	// The response embeds the verbatim Follow — remotes correlate on it.
	// Id-less Follows left ap_activity_id NULL (and a pruned inbound row
	// resolves nothing), so fall back to a minimal reconstruction.
	followAct := map[string]any{
		"type":   "Follow",
		"actor":  remote.ActorUrl,
		"object": actor.ActorUrl,
	}
	if follow.ApActivityID != nil && *follow.ApActivityID != "" {
		if inb, rerr := s.store.inboundByActivityID(ctx, *follow.ApActivityID); rerr == nil {
			var act map[string]any
			if jerr := json.Unmarshal(inb.Payload, &act); jerr == nil {
				followAct = act
			}
		}
	}

	status := "rejected"
	if accept {
		status = "accepted"
	}
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := gen.New(tx)
	n, err := q.ResolveFederationFollow(ctx, gen.ResolveFederationFollowParams{
		LocalActorID:  actor.ID,
		RemoteActorID: remoteActorID,
		Status:        status,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound // already resolved or never pending
	}
	var deliverID pgtype.UUID
	// A tombstoned/blocked remote has no live inbox — resolve the edge
	// honestly and skip the response POST that could only dead-letter.
	if remote.Status == "active" {
		if accept {
			if _, err := q.InsertRemoteFollow(ctx, gen.InsertRemoteFollowParams{
				RemoteActorID: remoteActorID,
				UserID:        userID,
			}); err != nil {
				return err
			}
			deliverID, err = s.insertAcceptDelivery(ctx, q, actor, &remote, followAct)
		} else {
			deliverID, err = s.insertRejectDelivery(ctx, q, actor, &remote, followAct)
		}
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if deliverID.Valid {
		if err := s.enqueueDelivery(ctx, deliverID); err != nil {
			slog.Warn("federation: delivery enqueue failed — sweeper picks up pending rows", "delivery_id", deliverID, "error", err)
		}
	}
	return nil
}

// dispatchAcceptReject flips a pending outbound federation_follows edge.
// M2 sends no outbound Follows, so most Accept traffic matches nothing —
// kept for protocol completeness and future outbound-follow work.
func (s *Service) dispatchAcceptReject(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	followID := apObjectID(act)
	if followID == "" || signer == nil {
		return dispatchResult{status: "ignored", reason: "accept without object"}
	}
	follow, err := q.GetFederationFollowByActivityID(ctx, &followID)
	if err != nil {
		return dispatchResult{status: "ignored", reason: "no matching follow"}
	}
	if follow.RemoteActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "accept from non-followed actor"}
	}
	newStatus := "accepted"
	if apStr(act["type"]) == "Reject" {
		newStatus = "rejected"
	}
	if err := q.UpdateFederationFollowStatus(ctx, gen.UpdateFederationFollowStatusParams{
		LocalActorID: follow.LocalActorID, RemoteActorID: follow.RemoteActorID, Status: newStatus,
	}); err != nil {
		return dispatchResult{status: "error", reason: "follow status: " + err.Error()}
	}
	return dispatchResult{status: "processed"}
}

// dispatchUndo reverses Follow / Like / Announce / Create by wrapping
// activity. The inner object is usually a full or partial copy of the
// original; bare-IRI undos resolve against the follow ledger.
func (s *Service) dispatchUndo(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned undo"}
	}
	inner := act["object"]
	innerType := apType(inner)
	innerID := apID(inner)

	if innerType == "Follow" || innerType == "" {
		return s.undoFollow(ctx, q, innerID, inner, signer)
	}
	target := apID(apObj(inner)["object"])
	switch innerType {
	case "Like":
		return s.undoLike(ctx, q, target, signer)
	case "Announce":
		return s.undoAnnounce(ctx, q, target, signer)
	case "Create":
		return s.undoCreate(ctx, q, innerID, signer)
	default:
		return dispatchResult{status: "ignored", reason: "undo of unsupported type " + innerType}
	}
}

// undoFollow undoes the edge identified by the original Follow's ap id,
// falling back to the local/remote pair when the remote omits the id.
func (s *Service) undoFollow(ctx context.Context, q *gen.Queries, innerID string, inner any, signer *gen.RemoteActor) dispatchResult {
	var follow gen.FederationFollow
	var err error
	if innerID != "" {
		follow, err = q.GetFederationFollowByActivityID(ctx, &innerID)
	}
	if innerID == "" || err != nil {
		objectID := innerID
		if obj := apObj(inner); obj != nil {
			if t := apID(obj["object"]); t != "" {
				objectID = t
			}
		}
		actor, _, perr := s.localActorForObject(ctx, q, objectID)
		if perr != nil {
			return dispatchResult{status: "ignored", reason: "undo follow target unknown"}
		}
		follow, err = q.GetFederationFollowByPair(ctx, gen.GetFederationFollowByPairParams{
			LocalActorID: actor.ID, RemoteActorID: signer.ID,
		})
		if err != nil {
			return dispatchResult{status: "ignored", reason: "no matching follow"}
		}
	}
	if follow.RemoteActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "undo from non-follower"}
	}
	if err := q.UpdateFederationFollowStatus(ctx, gen.UpdateFederationFollowStatusParams{
		LocalActorID: follow.LocalActorID, RemoteActorID: follow.RemoteActorID, Status: "undone",
	}); err != nil {
		return dispatchResult{status: "error", reason: "follow undo: " + err.Error()}
	}
	if _, err := q.DeleteRemoteFollow(ctx, gen.DeleteRemoteFollowParams{
		RemoteActorID: signer.ID, UserID: followUserID(ctx, q, follow.LocalActorID),
	}); err != nil {
		slog.Warn("federation: remote follows row delete failed", "error", err)
	}
	return dispatchResult{status: "processed"}
}

// followUserID resolves local_actor → users.id for the follows mirror row.
func followUserID(ctx context.Context, q *gen.Queries, localActorID pgtype.UUID) pgtype.UUID {
	actor, err := q.GetFederatedActorByID(ctx, localActorID)
	if err != nil {
		return pgtype.UUID{}
	}
	return actor.UserID
}

// undoLike removes a remote Like reaction and refreshes the score.
func (s *Service) undoLike(ctx context.Context, q *gen.Queries, targetID string, signer *gen.RemoteActor) dispatchResult {
	content, ok := s.localContentForObject(ctx, q, targetID)
	if !ok {
		return dispatchResult{status: "ignored", reason: "unlike target unknown"}
	}
	n, err := q.RemoveRemoteReaction(ctx, gen.RemoveRemoteReactionParams{
		EntityType: "content", EntityID: content.ID,
		RemoteActorID: signer.ID, ReactionKey: "like",
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "reaction delete: " + err.Error()}
	}
	if n > 0 {
		s.refreshReactionCount(ctx, q, content.ID)
	}
	return dispatchResult{status: "processed"}
}

// undoAnnounce drops the share ledger row; only a real delete decrements.
func (s *Service) undoAnnounce(ctx context.Context, q *gen.Queries, targetID string, signer *gen.RemoteActor) dispatchResult {
	content, ok := s.localContentForObject(ctx, q, targetID)
	if !ok {
		return dispatchResult{status: "ignored", reason: "unannounce target unknown"}
	}
	n, err := q.DeleteRemoteShare(ctx, gen.DeleteRemoteShareParams{
		RemoteActorID: signer.ID,
		ContentID:     content.ID,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "share delete: " + err.Error()}
	}
	if n > 0 {
		s.bumpCounter(q.DecrementContentShareCount(ctx, content.ID))
	}
	return dispatchResult{status: "processed"}
}

// undoCreate tombstones a remote comment by its remote_uri — only when
// the signer is the actor that created it (provenance check mirrors
// dispatchDelete).
func (s *Service) undoCreate(ctx context.Context, q *gen.Queries, objectID string, signer *gen.RemoteActor) dispatchResult {
	c, err := q.GetAtprotoCommentByRemoteURI(ctx, &objectID)
	if err != nil || !c.ContentID.Valid {
		return dispatchResult{status: "ignored", reason: "undo target unknown"}
	}
	if c.RemoteApActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "undo of another actor's object"}
	}
	n, err := q.SoftDeleteAtprotoCommentByRemoteURI(ctx, &objectID)
	if err != nil {
		return dispatchResult{status: "error", reason: "comment tombstone: " + err.Error()}
	}
	if n > 0 {
		s.unwindCommentCounters(ctx, q, c)
	}
	return dispatchResult{status: "processed"}
}

// dispatchLike lands a remote Like as a §49 reaction row — the partial
// unique on (entity, remote_actor) makes replays no-ops, and the recount
// query keeps content_scores honest.
func (s *Service) dispatchLike(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned like"}
	}
	content, ok := s.localContentForObject(ctx, q, apObjectID(act))
	if !ok {
		return dispatchResult{status: "ignored", reason: "like target is not a local object"}
	}
	n, err := q.AddRemoteReaction(ctx, gen.AddRemoteReactionParams{
		EntityType: "content", EntityID: content.ID,
		RemoteActorID: signer.ID, ReactionKey: "like",
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "reaction insert: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: "like replayed"}
	}
	s.refreshReactionCount(ctx, q, content.ID)
	if in := s.contentIntent(ctx, q, content.ID, *signer, notifications.TypeContentReaction,
		"Fediverse like", "liked your post on the fediverse"); in != nil {
		return dispatchResult{status: "processed", intents: []notifyIntent{*in}}
	}
	return dispatchResult{status: "processed"}
}

// dispatchAnnounce lands a remote boost: the shares ledger gates the
// share_count bump so replays and double-boosts can't drift the counter.
func (s *Service) dispatchAnnounce(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned announce"}
	}
	content, ok := s.localContentForObject(ctx, q, apObjectID(act))
	if !ok {
		return dispatchResult{status: "ignored", reason: "announce target is not a local object"}
	}
	n, err := q.InsertRemoteShare(ctx, gen.InsertRemoteShareParams{
		RemoteActorID: signer.ID,
		ContentID:     content.ID,
		ApActivityID:  stringPtrOrNil(apStr(act["id"])),
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "share insert: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: "announce replayed"}
	}
	s.bumpCounter(q.IncrementContentShareCount(ctx, content.ID))
	if in := s.contentIntent(ctx, q, content.ID, *signer, notifications.TypeContentReshare,
		"Fediverse boost", "boosted your post on the fediverse"); in != nil {
		return dispatchResult{status: "processed", intents: []notifyIntent{*in}}
	}
	return dispatchResult{status: "processed"}
}

// dispatchCreate inserts a remote reply comment. Only objects with an
// inReplyTo pointing at a local object or a known remote comment count —
// remote Create{article} without a local anchor isn't our concern.
func (s *Service) dispatchCreate(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned create"}
	}
	obj := apObj(act["object"])
	if obj == nil {
		return dispatchResult{status: "ignored", reason: "create without object"}
	}
	replyTo := apID(obj["inReplyTo"])
	if replyTo == "" {
		return dispatchResult{status: "ignored", reason: "create is not a reply"}
	}
	remoteURI := apStr(obj["id"])
	if remoteURI == "" {
		return dispatchResult{status: "rejected", reason: "remote object missing id"}
	}
	body := htmlToText(apStr(obj["content"]))
	if body == "" {
		return dispatchResult{status: "rejected", reason: "empty reply text"}
	}
	if len(body) > maxRemoteCommentText {
		return dispatchResult{status: "rejected", reason: "reply text too large"}
	}

	// inReplyTo resolves either to a local object URL (top-level reply)
	// or to a remote comment already ingested (nested reply).
	var contentID pgtype.UUID
	var parentID pgtype.UUID
	depth := int32(0)
	if content, ok := s.localContentForObject(ctx, q, replyTo); ok {
		contentID = content.ID
	} else if pc, perr := q.GetAtprotoCommentByRemoteURI(ctx, &replyTo); perr == nil && !pc.DeletedAt.Valid {
		contentID = pc.ContentID
		if pc.Depth < int32(maxCommentDepth) {
			parentID = pc.ID
			depth = pc.Depth + 1
		}
	} else {
		return dispatchResult{status: "ignored", reason: "reply target unknown"}
	}

	if _, err := q.InsertAPRemoteComment(ctx, gen.InsertAPRemoteCommentParams{
		ContentID:       contentID,
		ParentID:        parentID,
		Body:            proseMirrorDoc(body),
		Depth:           depth,
		RemoteApActorID: signer.ID,
		RemoteUri:       &remoteURI,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dispatchResult{status: "ignored", reason: "comment already ingested"}
		}
		return dispatchResult{status: "error", reason: "comment insert: " + err.Error()}
	}
	s.bumpCounter(q.IncrementContentCommentCount(ctx, contentID))
	s.bumpCounter(q.IncrementLinkedContentCommentCount(ctx, contentID))
	if parentID.Valid {
		s.bumpCounter(q.IncrementCommentReplyCount(ctx, parentID))
	}
	if in := s.contentIntent(ctx, q, contentID, *signer, notifications.TypeContentComment,
		"Fediverse reply", "replied to your post on the fediverse"); in != nil {
		return dispatchResult{status: "processed", intents: []notifyIntent{*in}}
	}
	return dispatchResult{status: "processed"}
}

// dispatchUpdate refreshes a remote comment body (object = prior remote
// URI) — only when the signer is the actor that created it. Actor
// self-updates need no work — the resolver cache TTL covers re-fetch.
func (s *Service) dispatchUpdate(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned update"}
	}
	obj := act["object"]
	objectID := apID(obj)
	if objectID == "" {
		return dispatchResult{status: "ignored", reason: "update without object"}
	}
	if strings.EqualFold(objectID, signer.ActorUrl) {
		return dispatchResult{status: "processed"}
	}
	objMap := apObj(obj)
	if objMap == nil {
		return dispatchResult{status: "ignored", reason: "update object is a bare IRI"}
	}
	c, err := q.GetAtprotoCommentByRemoteURI(ctx, &objectID)
	if err != nil || !c.ContentID.Valid {
		return dispatchResult{status: "ignored", reason: "updated object unknown"}
	}
	// Provenance gate: an Update may only rewrite a comment this same
	// actor created — otherwise any instance could edit another's replies.
	if c.RemoteApActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "update of another actor's object"}
	}
	body := htmlToText(apStr(objMap["content"]))
	if body == "" || len(body) > maxRemoteCommentText {
		return dispatchResult{status: "rejected", reason: "bad updated text"}
	}
	n, err := q.UpdateAtprotoRemoteCommentBody(ctx, gen.UpdateAtprotoRemoteCommentBodyParams{
		RemoteUri: &objectID,
		Body:      proseMirrorDoc(body),
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "comment update: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: "updated object unknown"}
	}
	return dispatchResult{status: "processed"}
}

// dispatchDelete handles Delete{actor} (full remote provenance unwind)
// and Delete{object} (single comment tombstone).
func (s *Service) dispatchDelete(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned delete"}
	}
	objectID := apObjectID(act)
	if objectID == "" {
		return dispatchResult{status: "ignored", reason: "delete without object"}
	}
	// Actor self-delete: object is the actor's own document. A signer can
	// only delete itself — an embedded {type:Person} for a different actor
	// is ignored, never tombstoned.
	if strings.EqualFold(objectID, signer.ActorUrl) {
		return s.deleteRemoteActor(ctx, q, signer)
	}
	c, err := q.GetAtprotoCommentByRemoteURI(ctx, &objectID)
	if err != nil || !c.ContentID.Valid {
		return dispatchResult{status: "ignored", reason: "delete target unknown"}
	}
	// Provenance gate: Delete may only tombstone a comment this same actor
	// created — a foreign Delete{object} must never remove another
	// instance's reply.
	if c.RemoteApActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "delete of another actor's object"}
	}
	n, err := q.SoftDeleteAtprotoCommentByRemoteURI(ctx, &objectID)
	if err != nil {
		return dispatchResult{status: "error", reason: "comment tombstone: " + err.Error()}
	}
	if n > 0 {
		s.unwindCommentCounters(ctx, q, c)
	}
	return dispatchResult{status: "processed"}
}

// deleteRemoteActor tombstones the actor and unwinds follows/reactions/
// shares/comments — the inverse of everything inbound ever wrote for it.
func (s *Service) deleteRemoteActor(ctx context.Context, q *gen.Queries, signer *gen.RemoteActor) dispatchResult {
	if err := q.SetRemoteActorStatus(ctx, gen.SetRemoteActorStatusParams{
		ID: signer.ID, Status: "tombstone",
	}); err != nil {
		return dispatchResult{status: "error", reason: "actor tombstone: " + err.Error()}
	}
	if err := q.UndoFederationFollowsByRemoteActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: follow unwind failed", "error", err)
	}
	if err := q.DeleteRemoteFollowsByActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: follows unwind failed", "error", err)
	}
	if reactions, err := q.DeleteRemoteReactionsByActor(ctx, signer.ID); err == nil {
		seen := map[pgtype.UUID]bool{}
		for _, r := range reactions {
			if r.EntityType == "content" && !seen[r.EntityID] {
				seen[r.EntityID] = true
				s.refreshReactionCount(ctx, q, r.EntityID)
			}
		}
	} else {
		slog.Warn("federation: reactions unwind failed", "error", err)
	}
	if shares, err := q.DeleteRemoteSharesByActor(ctx, signer.ID); err == nil {
		seen := map[pgtype.UUID]bool{}
		for _, cid := range shares {
			if !seen[cid] {
				seen[cid] = true
				s.bumpCounter(q.DecrementContentShareCount(ctx, cid))
			}
		}
	}
	if comments, err := q.SoftDeleteAPCommentsByActor(ctx, signer.ID); err == nil {
		byContent := map[pgtype.UUID]int32{}
		byParent := map[pgtype.UUID]int32{}
		for _, c := range comments {
			byContent[c.ContentID]++
			if c.ParentID.Valid {
				byParent[c.ParentID]++
			}
		}
		for cid, n := range byContent {
			s.bumpCounter(q.DecrementContentCommentCountBy(ctx, gen.DecrementContentCommentCountByParams{ID: cid, N: n}))
			s.bumpCounter(q.DecrementLinkedContentCommentCountBy(ctx, gen.DecrementLinkedContentCommentCountByParams{ThreadID: cid, N: n}))
		}
		for pid, n := range byParent {
			s.bumpCounter(q.DecrementCommentReplyCountBy(ctx, gen.DecrementCommentReplyCountByParams{ParentID: pid, N: n}))
		}
	}
	return dispatchResult{status: "processed"}
}

// localContentForObject resolves a local object URL
// ({api}/federation/objects/contents/{uuid}) to a live, federatable
// content row — Likes/Announces/replies against unpublished, private,
// deleted, or opted-out content are ignored, not counted.
func (s *Service) localContentForObject(ctx context.Context, q *gen.Queries, objectID string) (gen.GetFederatableContentByIDRow, bool) {
	prefix := s.apiBase() + "/federation/objects/contents/"
	raw, ok := strings.CutPrefix(objectID, prefix)
	if !ok {
		return gen.GetFederatableContentByIDRow{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(strings.Trim(raw, "/")); err != nil || !id.Valid {
		return gen.GetFederatableContentByIDRow{}, false
	}
	row, err := q.GetFederatableContentByID(ctx, gen.GetFederatableContentByIDParams{
		ID:           id,
		ContentTypes: s.federatableTypesFor(ctx),
	})
	if err != nil {
		return gen.GetFederatableContentByIDRow{}, false
	}
	return row, true
}

// refreshReactionCount recomputes content_scores.reaction_count from the
// live reactions table — recount can't drift under retry or out-of-order
// undo.
func (s *Service) refreshReactionCount(ctx context.Context, q *gen.Queries, contentID pgtype.UUID) {
	if err := q.IncrementScoreReaction(ctx, contentID); err != nil {
		slog.Warn("federation: reaction recount failed", "content_id", contentID, "error", err)
	}
}

// unwindCommentCounters reverses the denormalized bumps a remote comment
// took at insert (content + linked thread + parent reply count).
func (s *Service) unwindCommentCounters(ctx context.Context, q *gen.Queries, c gen.GetAtprotoCommentByRemoteURIRow) {
	s.bumpCounter(q.DecrementContentCommentCount(ctx, c.ContentID))
	s.bumpCounter(q.DecrementLinkedContentCommentCount(ctx, c.ContentID))
	if c.ParentID.Valid {
		s.bumpCounter(q.DecrementCommentReplyCount(ctx, c.ParentID))
	}
}

// contentIntent builds the notification to the content's author for a
// remote engagement — mirrors §127's inboundIntent.
func (s *Service) contentIntent(ctx context.Context, q *gen.Queries, contentID pgtype.UUID, actor gen.RemoteActor, nType, title, verb string) *notifyIntent {
	owner, err := q.GetContentByID(ctx, contentID)
	if err != nil || !owner.AuthorID.Valid {
		return nil
	}
	return &notifyIntent{
		recipientID: owner.AuthorID,
		nType:       nType,
		actorID:     actor.ID,
		subjectType: "content",
		subjectID:   contentID,
		title:       title,
		body:        remoteHandle(actor) + " " + verb,
		action:      "/content/" + owner.Slug,
	}
}

// bumpCounter logs denormalized-counter failures — drift here silently
// desyncs feed cards and thread counters.
func (s *Service) bumpCounter(err error) {
	if err != nil {
		slog.Warn("federation: counter bump failed", "error", err)
	}
}

// proseMirrorDoc wraps remote plain text into the TipTap/ProseMirror doc
// shape comments.body stores (shared shape with §127 inbound).
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

// htmlToText flattens AP HTML content to plain text for comment storage.
// Remote Note.content is always HTML; block-level boundaries become
// newlines before tags drop, then entities unescape.
func htmlToText(htmlStr string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"</p>", "\n\n", "</div>", "\n", "</li>", "\n", "</h1>", "\n", "</h2>", "\n", "</h3>", "\n",
	)
	s := replacer.Replace(htmlStr)
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(html.UnescapeString(b.String()))
}
