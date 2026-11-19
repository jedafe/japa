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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/spam"
)

// Notifier is the narrow §35 seam — implemented by notifications.Service.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// SetNotifier injects the notifications seam for inbound events.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SpamChecker is the §41 seam — implemented by spam.SpamPipeline. Remote
// comments route through the same content layers member comments do;
// user/IP-scoped layers skip on the zero identity a remote actor carries.
type SpamChecker interface {
	Check(ctx context.Context, req spam.SpamCheckRequest) (spam.SpamCheckResult, error)
}

// SetSpamChecker injects the spam pipeline for inbound Create bodies.
func (s *Service) SetSpamChecker(c SpamChecker) { s.spam = c }

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
	// deliverIDs carries the multi-row case (Move auto-refollow ledgers
	// one Follow per repointed outbound edge).
	deliverIDs []pgtype.UUID
	// invalidateRemote is a remote-actor URL whose jc:fed:remote:* cache
	// entry must drop post-commit — the dispatch tombstoned the row and a
	// stale 'active' snapshot must not keep resolving it.
	invalidateRemote string
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
		return s.finishInbound(ctx, row.ID, "rejected", "malformed activity json", nil, pgtype.UUID{}, nil)
	}

	// The signer is the activity's author-of-record for state changes.
	var signer *gen.RemoteActor
	if row.ActorUrl != nil && *row.ActorUrl != "" {
		signer, err = s.resolveRemoteActor(ctx, *row.ActorUrl)
		if err != nil {
			if errors.Is(err, ErrRemoteBlocked) {
				return s.finishInbound(ctx, row.ID, "rejected", "signer instance blocked", nil, pgtype.UUID{}, nil)
			}
			// Freshness fetches are best-effort: the stale cached row still
			// carries the identity dispatch needs — and a Delete{actor} from
			// a remote that just died can NEVER refresh, so it must run off
			// the stale row or the dead actor's data never unwinds.
			stale, derr := s.store.remoteActorByURL(ctx, *row.ActorUrl)
			if derr != nil {
				return fmt.Errorf("resolve signer: %w", err) // retryable fetch failure
			}
			if stale.Status != "active" {
				return s.finishInbound(ctx, row.ID, "rejected", "signer actor "+stale.Status, nil, pgtype.UUID{}, nil)
			}
			signer = &stale
		}
	}

	// §126: remote comments route through the existing spam pipeline.
	// Pre-tx on purpose — the AI layer can take seconds and must not hold
	// a database transaction open.
	if reason := s.spamCheckReply(ctx, act); reason != "" {
		return s.finishInbound(ctx, row.ID, "rejected", reason, nil, pgtype.UUID{}, nil)
	}

	// M3: relay couriers carry activities whose actor is NOT the signer —
	// the relay re-signs subscribers' public content for its audience.
	// Only an accepted relay subscription earns this: the embedded actor
	// is resolved fresh and becomes the effective signer, so every
	// downstream provenance check binds to the real author, not the
	// courier. Pre-tx on purpose like the spam check — a fresh couriered
	// actor triggers a remote fetch that must not hold the dispatch tx
	// open. A mismatched non-relay signer is lying about authorship.
	if signer != nil {
		if actorID := apActorID(act); actorID != "" && !strings.EqualFold(actorID, signer.ActorUrl) {
			if !s.isAcceptedRelay(ctx, signer.ActorUrl) {
				return s.finishInbound(ctx, row.ID, "rejected", "activity actor does not match signer", nil, pgtype.UUID{}, nil)
			}
			resolved, rerr := s.resolveRemoteActor(ctx, actorID)
			if rerr != nil || resolved == nil || resolved.Status != "active" {
				return s.finishInbound(ctx, row.ID, "rejected", "couriered actor unresolvable", nil, pgtype.UUID{}, nil)
			}
			signer = resolved
		}
	}

	// §126 M4: Move{actor→target} needs the TARGET's actor document —
	// its alsoKnownAs is the sole evidence the migration is endorsed.
	// Fetch + verify pre-tx like the courier hoist: a remote fetch must
	// not hold the dispatch transaction open. Verified moves hand the
	// upserted target row to dispatch; rejected ones carry the reason.
	var moveTarget *gen.RemoteActor
	moveReject := ""
	if apStr(act["type"]) == "Move" {
		targetIRI := apID(act["target"])
		switch {
		case signer == nil:
			moveReject = "unsigned move"
		case !strings.EqualFold(apObjectID(act), apActorID(act)):
			moveReject = "move object is not the moving actor"
		case targetIRI == "" || strings.EqualFold(targetIRI, signer.ActorUrl):
			moveReject = "move target missing or is the moving actor"
		case sameURLHost(targetIRI, s.apiBase()):
			moveReject = "move target is a local actor"
		default:
			mt, merr := s.verifyMoveTarget(ctx, signer.ActorUrl, targetIRI)
			switch {
			case merr == nil:
				moveTarget = mt
			case errors.Is(merr, ErrMoveNotConfirmed) || errors.Is(merr, ErrRemoteBlocked):
				moveReject = merr.Error()
			default:
				return fmt.Errorf("move target resolve: %w", merr) // retryable
			}
		}
	}

	// §126 tag discovery: a relay Announce{Note} is the Follow{Hashtag}
	// delivery form — ingesting it needs the object doc + its author's
	// actor resolved, remote fetches that hoist pre-tx like the Move
	// target. Nil when no subscription warrant could cover the host.
	var announced *announceIngest
	if apStr(act["type"]) == "Announce" && signer != nil {
		announced = s.resolveAnnounceTarget(ctx, act)
		// The Announce arm ingests the announced object into member
		// timelines under a tag-subscription warrant — content arriving
		// this way is as unvetted as a direct Create, so the fetched
		// object body runs the same pipeline arm. A bare-IRI announce
		// resolves nothing here (announced nil) and ingests nothing.
		if announced != nil {
			if reason := s.spamCheckBody(ctx, htmlToText(apStr(announced.obj["content"]))); reason != "" {
				return s.finishInbound(ctx, row.ID, "rejected", reason, nil, pgtype.UUID{}, nil)
			}
		}
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inbound tx: %w", err)
	}
	defer tx.Rollback(ctx)
	res := s.dispatch(ctx, gen.New(tx), row, act, signer, moveTarget, moveReject, announced)
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
	// Post-commit invalidation — inside the dispatch tx a concurrent
	// resolve could re-cache the pre-delete row between the tombstone
	// write and commit; dropping the key after commit is the point the
	// ledger actually says tombstone.
	if res.invalidateRemote != "" {
		s.cacheDelRemote(ctx, res.invalidateRemote)
	}
	return s.finishInbound(ctx, row.ID, res.status, res.reason, res.intents, res.deliverID, res.deliverIDs)
}

// spamCheckReply runs the §41 pipeline on inbound content bodies — the
// same check member comments and member posts get. Create objects are
// checked whether or not they carry inReplyTo: replies become comments
// and top-level posts land in member timelines via storeRemotePost, so
// both ingest paths need the gate. Update objects get checked when they
// carry a rewritten content body — otherwise a clean Create followed by
// a spammy Update would sidestep the pipeline entirely. Actor
// self-updates (object = the activity's actor) carry no comment body and
// are skipped.
func (s *Service) spamCheckReply(ctx context.Context, act map[string]any) string {
	var body string
	switch apStr(act["type"]) {
	case "Create":
		obj := apObj(act["object"])
		if obj == nil {
			return ""
		}
		body = htmlToText(apStr(obj["content"]))
	case "Update":
		obj := apObj(act["object"])
		if obj == nil || strings.EqualFold(apID(obj["id"]), apActorID(act)) {
			return ""
		}
		body = htmlToText(apStr(obj["content"]))
	default:
		return ""
	}
	return s.spamCheckBody(ctx, body)
}

// spamCheckBody is the pipeline arm shared by Create/Update bodies and
// the relay-announced object resolved pre-tx. A zero-identity request
// (no local user, no client IP) lets the user/IP-scoped layers skip
// while the content layers — censored words, content hash, link
// analysis, length, AI classification — apply exactly as they do for
// members. Like the member path, a pipeline failure is fail-open; a
// blocked verdict is not — the activity is rejected outright.
func (s *Service) spamCheckBody(ctx context.Context, body string) string {
	if s.spam == nil || body == "" {
		return ""
	}
	res, err := s.spam.Check(ctx, spam.SpamCheckRequest{
		Content:    body,
		UserType:   "existing",
		EntityType: "comment",
	})
	if err != nil {
		slog.Warn("federation: inbound spam check failed", "error", err)
		return ""
	}
	if res.Blocked {
		return "content blocked by safety filters"
	}
	return ""
}

// finishInbound stamps the ledger row, fires deferred notifications, and
// enqueues any delivery the dispatch produced. Notifications are
// best-effort post-commit — a lost ping must not replay the activity.
func (s *Service) finishInbound(ctx context.Context, id pgtype.UUID, status, reason string, intents []notifyIntent, deliverID pgtype.UUID, deliverIDs []pgtype.UUID) error {
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
	for _, did := range deliverIDs {
		if !did.Valid {
			continue
		}
		if err := s.enqueueDelivery(ctx, did); err != nil {
			slog.Warn("federation: delivery enqueue failed — sweeper picks up pending rows", "delivery_id", did, "error", err)
		}
	}
	return nil
}

// dispatch routes the activity. All writes run inside the caller's tx.
// The inbound row id keys per-event response identity (Accept/Reject
// dedup) — it is stable across retries of this row and unique across
// Follow events. moveTarget/moveReject carry the pre-tx Move verdict —
// nil + "" for non-Move activities.
func (s *Service) dispatch(ctx context.Context, q *gen.Queries, row gen.FederationInbound, act map[string]any, signer *gen.RemoteActor, moveTarget *gen.RemoteActor, moveReject string, announced *announceIngest) dispatchResult {
	typ := apStr(act["type"])
	actorID := apActorID(act)
	// The relay-courier swap ran pre-tx in ProcessInbound (a remote fetch
	// must not hold this tx open) — a mismatched actor reaching dispatch
	// can only be a non-relay signer lying about authorship.
	if signer != nil && actorID != "" && !strings.EqualFold(actorID, signer.ActorUrl) {
		return dispatchResult{status: "rejected", reason: "activity actor does not match signer"}
	}
	switch typ {
	case "Follow":
		return s.dispatchFollow(ctx, q, act, signer, row.ID)
	case "Accept", "Reject":
		return s.dispatchAcceptReject(ctx, q, act, signer)
	case "Undo":
		return s.dispatchUndo(ctx, q, act, signer)
	case "Like":
		return s.dispatchLike(ctx, q, act, signer)
	case "Announce":
		return s.dispatchAnnounce(ctx, q, act, signer, announced)
	case "Create":
		return s.dispatchCreate(ctx, q, act, signer)
	case "Update":
		return s.dispatchUpdate(ctx, q, act, signer)
	case "Move":
		return s.dispatchMove(ctx, q, signer, moveTarget, moveReject)
	case "Delete":
		return s.dispatchDelete(ctx, q, act, signer)
	case "Flag":
		return s.dispatchFlag(ctx, q, act, signer)
	case "Block":
		return s.dispatchBlock(ctx, q, act, signer)
	case "EmojiReact":
		return s.dispatchEmojiReact(ctx, q, act, signer)
	case "Add", "Remove":
		return s.dispatchAddRemove(ctx, q, act, signer)
	default:
		return dispatchResult{status: "ignored", reason: "unsupported activity type " + typ}
	}
}

// localActorForObject resolves object → the local actor it addresses.
// Three actor-URL shapes are valid Follow targets (§126 M3):
//
//	{api}/federation/actors/{username} — member Person actors (profile
//	  join keeps suspended/deleted members out)
//	{api}/federation/instance — the singleton Application actor (M3)
//	{api}/federation/groups/{slug} — a public space's Group actor (M3)
//
// The profile row is only populated for person actors; non-person
// callers must branch on actor.Kind.
func (s *Service) localActorForObject(ctx context.Context, q *gen.Queries, objectID string) (gen.FederatedActor, gen.GetActorProfileByUsernameRow, error) {
	base := s.apiBase() + "/federation/"
	if objectID == base+"instance" {
		actor, err := s.ensureInstanceActor(ctx)
		return actor, gen.GetActorProfileByUsernameRow{}, err
	}
	if slug, ok := strings.CutPrefix(objectID, base+"groups/"); ok && slug != "" && !strings.Contains(slug, "/") {
		row, err := q.GetGroupActorDocBySlug(ctx, slug)
		if err != nil {
			return gen.FederatedActor{}, gen.GetActorProfileByUsernameRow{}, err
		}
		// A flipped-private, inactive, or non-member-created space stops
		// being a follow target — its actor only answers while the space
		// resolves (same envelope as groupDocRow).
		if !spaceGroupEligible(row) {
			return gen.FederatedActor{}, gen.GetActorProfileByUsernameRow{}, pgx.ErrNoRows
		}
		return groupActorFromRow(row), gen.GetActorProfileByUsernameRow{}, nil
	}
	prefix := base + "actors/"
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
func (s *Service) dispatchFollow(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor, rowID pgtype.UUID) dispatchResult {
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

	// Manual approval reads the live profile-join flag — the stored
	// federated_actors column only refreshes at enable/reactivate, so a
	// privacy change while the actor is active must still gate new edges.
	// Non-person actors (instance/group) auto-accept — the approval
	// semantics are member-only.
	status := "accepted"
	if actor.Kind == ActorKindPerson && profile.ManuallyApprovesFollowers {
		status = "pending"
	}
	// Prior edge state gates the notification: remote implementations
	// re-send Follow (new activity id) until they get an answer — each
	// re-send must not re-ping the member.
	priorStatus := ""
	if prior, perr := q.GetFederationFollowByPair(ctx, gen.GetFederationFollowByPairParams{
		LocalActorID: actor.ID, RemoteActorID: signer.ID, Direction: "inbound",
	}); perr == nil {
		priorStatus = prior.Status
	}
	follow, err := q.UpsertFederationFollow(ctx, gen.UpsertFederationFollowParams{
		LocalActorID:  actor.ID,
		RemoteActorID: signer.ID,
		ApActivityID:  stringPtrOrNil(apStr(act["id"])),
		Status:        status,
		Direction:     "inbound",
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "follow upsert: " + err.Error()}
	}

	// Replays and re-Follows converge: an undone edge stays undone.
	if follow.Status == "undone" {
		return dispatchResult{status: "ignored", reason: "follow was undone"}
	}
	// Check the STORED status, not the desired one — sticky 'accepted'
	// survives a pending-desired upsert and must not notify as a request.
	if follow.Status == "pending" {
		res := dispatchResult{status: "processed"}
		if actor.Kind == ActorKindPerson && priorStatus != "pending" { // fresh edge, or re-request after rejection
			res.intents = []notifyIntent{s.followIntent(actor, profile, *signer)}
		}
		return res
	}

	// accepted — mirror the §49 follower edge and emit Accept. Mastodon
	// sends Accept for every honored Follow, so the delivery fires on
	// replays too; the notification lands only when the edge is new.
	// The follows-mirror, follower_count, and member notification are
	// person-only: group/instance edges live solely in federation_follows.
	var n int64
	if actor.Kind == ActorKindPerson {
		n, err = q.InsertRemoteFollow(ctx, gen.InsertRemoteFollowParams{
			RemoteActorID: signer.ID,
			UserID:        actor.UserID,
		})
		if err != nil {
			return dispatchResult{status: "error", reason: "follows insert: " + err.Error()}
		}
		if n > 0 {
			// The remote follower is a real follows edge — the member's
			// denormalized follower_count must count it like a local follow.
			s.bumpCounter(q.AdjustUserFollowerCount(ctx, gen.AdjustUserFollowerCountParams{
				UserID: actor.UserID, Delta: 1,
			}))
		}
	}
	deliverID, err := s.insertAcceptDelivery(ctx, q, actor, signer, act, rowID.String())
	res := dispatchResult{status: "processed", deliverID: deliverID}
	switch {
	case errors.Is(err, errNoRemoteInbox):
		// The edge is honored; a follower advertising no inbox simply can
		// never receive the Accept — a permanent condition must not spin
		// the retry queue on every re-sent Follow.
		res.reason = "follower advertises no inbox"
	case err != nil:
		return dispatchResult{status: "error", reason: "accept delivery: " + err.Error()}
	}
	if n > 0 && actor.Kind == ActorKindPerson {
		res.intents = []notifyIntent{s.followIntent(actor, profile, *signer)}
	}
	return res
}

// insertAcceptDelivery builds the Accept{Follow} and ledgers it for the
// delivery worker. The embedded object is the verbatim incoming Follow —
// remote servers correlate on the original activity id.
func (s *Service) insertAcceptDelivery(ctx context.Context, q *gen.Queries, actor gen.FederatedActor, remote *gen.RemoteActor, followAct map[string]any, discriminator string) (pgtype.UUID, error) {
	return s.insertFollowResponseDelivery(ctx, q, "Accept", actor, remote, followAct, discriminator)
}

// insertRejectDelivery is the member-driven counterpart — Reject{Follow}
// when a pending request is declined.
func (s *Service) insertRejectDelivery(ctx context.Context, q *gen.Queries, actor gen.FederatedActor, remote *gen.RemoteActor, followAct map[string]any, discriminator string) (pgtype.UUID, error) {
	return s.insertFollowResponseDelivery(ctx, q, "Reject", actor, remote, followAct, discriminator)
}

// errNoRemoteInbox marks the permanent "actor advertises no inbox"
// condition — the edge is real but no response can ever be POSTed, so
// callers must not treat it as a retryable failure.
var errNoRemoteInbox = errors.New("federation: remote actor has no inbox")

// insertFollowResponseDelivery ledgers one Accept|Reject{Follow} row.
// The response id carries a per-event discriminator — the inbound row id
// in dispatch, a fresh uuid in member resolution — because the delivery
// dedup key is (activity_ap_id, inbox_url): a repeated Follow event must
// produce a NEW response row, or a remote that missed the first answer
// hangs on a deduped key while we report the edge resolved.
func (s *Service) insertFollowResponseDelivery(ctx context.Context, q *gen.Queries, typ string, actor gen.FederatedActor, remote *gen.RemoteActor, followAct map[string]any, discriminator string) (pgtype.UUID, error) {
	inbox := remote.InboxUrl
	if remote.SharedInboxUrl != nil && *remote.SharedInboxUrl != "" {
		inbox = remote.SharedInboxUrl
	}
	if inbox == nil || *inbox == "" {
		return pgtype.UUID{}, errNoRemoteInbox
	}
	responseID := actor.ActorUrl + "#" + lowerType(typ) + "s/" + followActivityRef(followAct) + "/" + discriminator
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
		action:      "/u/" + profile.Username,
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
	follow, err := s.store.followByPair(ctx, actor.ID, remoteActorID, "inbound")
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
			n, err := q.InsertRemoteFollow(ctx, gen.InsertRemoteFollowParams{
				RemoteActorID: remoteActorID,
				UserID:        userID,
			})
			if err != nil {
				return err
			}
			if n > 0 {
				s.bumpCounter(q.AdjustUserFollowerCount(ctx, gen.AdjustUserFollowerCountParams{
					UserID: userID, Delta: +1,
				}))
			}
			deliverID, err = s.insertAcceptDelivery(ctx, q, actor, &remote, followAct, uuid.NewString())
		} else {
			deliverID, err = s.insertRejectDelivery(ctx, q, actor, &remote, followAct, uuid.NewString())
		}
		if err != nil {
			if errors.Is(err, errNoRemoteInbox) {
				deliverID = pgtype.UUID{} // response undeliverable — edge still resolved
			} else {
				return err
			}
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

// dispatchAcceptReject answers outbound Follows. Two arms: relay
// subscriptions (M3 — federation_relays.follow_ap_id, the relay actor
// answers), and member outbound follows (M5 — the followed remote actor
// answers its own request). Both enforce "signer IS the answering party":
// a third party embedding our Follow id must not flip either state.
func (s *Service) dispatchAcceptReject(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned accept/reject"}
	}
	followID := apID(act["object"])
	if followID == "" {
		if obj := apObj(act["object"]); obj != nil {
			followID = apID(obj["id"])
		}
	}
	if followID == "" {
		return dispatchResult{status: "ignored", reason: "accept/reject without follow id"}
	}
	relay, err := q.GetFederationRelayByFollowApID(ctx, &followID)
	if err != nil {
		return s.resolveOutboundFollowAnswer(ctx, q, act, signer, followID)
	}
	// Only the relay actor may resolve its own subscription — a third
	// party embedding our Follow id must not flip state.
	if !strings.EqualFold(signer.ActorUrl, relay.ActorUrl) {
		return dispatchResult{status: "rejected", reason: "accept/reject from non-relay signer"}
	}
	newStatus := "accepted"
	if apStr(act["type"]) == "Reject" {
		newStatus = "rejected"
	}
	if relay.Status == "pending" {
		if err := q.SetFederationRelayStatus(ctx, gen.SetFederationRelayStatusParams{
			ID: relay.ID, Status: newStatus,
		}); err != nil {
			return dispatchResult{status: "error", reason: "relay status: " + err.Error()}
		}
	}
	return dispatchResult{status: "processed"}
}

// resolveOutboundFollowAnswer — M5 member-follow arm of
// dispatchAcceptReject: the Follow @id matches an outbound edge and the
// signer must BE the followed actor. First answer wins (the conditional
// update only fires from 'pending'); an Accept mirrors the follow into
// the member's native graph (entity_type='remote_actor') and notifies.
func (s *Service) resolveOutboundFollowAnswer(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor, followID string) dispatchResult {
	edge, err := q.GetOutboundFederationFollowByActivityID(ctx, &followID)
	if err != nil {
		return dispatchResult{status: "ignored", reason: "accept/reject answers an unknown follow"}
	}
	if edge.RemoteActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "accept/reject from non-target signer"}
	}
	newStatus := "accepted"
	if apStr(act["type"]) == "Reject" {
		newStatus = "rejected"
	}
	n, err := q.ResolveOutboundFederationFollow(ctx, gen.ResolveOutboundFederationFollowParams{
		ID: edge.ID, Status: newStatus,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "outbound follow resolve: " + err.Error()}
	}
	if n == 0 {
		// Sticky: the edge already got its answer — a later Reject must
		// not bury a live Accept (withdrawal is Block/Undo business).
		return dispatchResult{status: "processed"}
	}
	if newStatus != "accepted" {
		// A Reject tears down any optimistic mirror (a Move-repointed
		// edge carries one forward) — no edge, no following-graph row.
		userID := followUserID(ctx, q, edge.LocalActorID)
		if userID.Valid {
			if _, derr := q.DeleteOutboundFollowMirror(ctx, gen.DeleteOutboundFollowMirrorParams{
				FollowerID: userID, RemoteActorID: signer.ID,
			}); derr != nil {
				slog.Warn("federation: reject mirror delete failed", "error", derr)
			}
		}
		return dispatchResult{status: "processed"}
	}
	userID := followUserID(ctx, q, edge.LocalActorID)
	if _, err := q.InsertOutboundFollowMirror(ctx, gen.InsertOutboundFollowMirrorParams{
		FollowerID: userID, RemoteActorID: signer.ID,
	}); err != nil {
		return dispatchResult{status: "error", reason: "follows mirror: " + err.Error()}
	}
	return dispatchResult{status: "processed", intents: []notifyIntent{
		followAcceptIntent(userID, signer),
	}}
}

// followAcceptIntent notifies the member that a remote instance accepted
// their outbound Follow — deep-linked at their federation settings page
// (the surface where the pending edge flipped to Following).
func followAcceptIntent(userID pgtype.UUID, signer *gen.RemoteActor) notifyIntent {
	return notifyIntent{
		recipientID: userID,
		nType:       notifications.TypeNewFollower,
		actorID:     signer.ID,
		subjectType: "user",
		subjectID:   userID,
		title:       "Follow request accepted",
		body:        remoteHandle(*signer) + " accepted your follow request on the fediverse",
		action:      "/settings/federation",
	}
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
		return s.undoLike(ctx, q, target, signer, "like")
	case "EmojiReact":
		return s.undoLike(ctx, q, target, signer, s.emojiReactionKey(ctx, q, apStr(apObj(inner)["content"])))
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
// M5 direction-aware: an Undo quoting back OUR outbound Follow (the
// signer is the followed actor) is a withdrawal of prior acceptance —
// the edge dies and the remote_actor mirror deletes; an Undo of the
// remote's own inbound Follow runs the original mirror+count unwind.
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
			LocalActorID: actor.ID, RemoteActorID: signer.ID, Direction: "inbound",
		})
		if err != nil {
			return dispatchResult{status: "ignored", reason: "no matching follow"}
		}
	}
	if follow.RemoteActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "undo from non-follower"}
	}
	if err := q.UpdateFederationFollowStatus(ctx, gen.UpdateFederationFollowStatusParams{
		LocalActorID: follow.LocalActorID, RemoteActorID: follow.RemoteActorID,
		Direction: follow.Direction, Status: "undone",
	}); err != nil {
		return dispatchResult{status: "error", reason: "follow undo: " + err.Error()}
	}
	userID := followUserID(ctx, q, follow.LocalActorID)
	if follow.Direction == "outbound" {
		// Withdrawal of our outbound follow — the remote_actor mirror
		// goes; no counter unwind (following_count derives on demand).
		if _, err := q.DeleteOutboundFollowMirror(ctx, gen.DeleteOutboundFollowMirrorParams{
			FollowerID: userID, RemoteActorID: signer.ID,
		}); err != nil {
			slog.Warn("federation: outbound mirror delete failed", "error", err)
		}
		return dispatchResult{status: "processed"}
	}
	n, err := q.DeleteRemoteFollow(ctx, gen.DeleteRemoteFollowParams{
		RemoteActorID: signer.ID, UserID: userID,
	})
	if err != nil {
		slog.Warn("federation: remote follows row delete failed", "error", err)
	}
	if n > 0 && userID.Valid {
		s.bumpCounter(q.AdjustUserFollowerCount(ctx, gen.AdjustUserFollowerCountParams{
			UserID: userID, Delta: -1,
		}))
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

// contentIDForObject parses a local object URL back to the contents id —
// deliberately ungated: undo paths must run against whatever the ledger
// holds, even after the object left the federatable set (unpublish,
// private flip). A gated lookup would make un-likes/un-boosts on
// un-federated objects unanswerable — the row in reactions/shares is the
// authority on whether a decrement is owed.
func (s *Service) contentIDForObject(objectID string) (pgtype.UUID, bool) {
	raw, ok := strings.CutPrefix(objectID, s.apiBase()+"/federation/objects/contents/")
	if !ok {
		return pgtype.UUID{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(strings.Trim(raw, "/")); err != nil || !id.Valid {
		return pgtype.UUID{}, false
	}
	return id, true
}

// undoLike removes a remote Like/EmojiReact reaction and refreshes the
// score — the reaction key travels with the inner activity's content so
// an EmojiReact undo targets the same key its insert minted.
func (s *Service) undoLike(ctx context.Context, q *gen.Queries, targetID string, signer *gen.RemoteActor, reactionKey string) dispatchResult {
	contentID, ok := s.contentIDForObject(targetID)
	if !ok {
		return dispatchResult{status: "ignored", reason: "unlike target unknown"}
	}
	n, err := q.RemoveRemoteReaction(ctx, gen.RemoveRemoteReactionParams{
		EntityType: "content", EntityID: contentID,
		RemoteActorID: signer.ID, ReactionKey: reactionKey,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "reaction delete: " + err.Error()}
	}
	if n > 0 {
		s.refreshReactionCount(ctx, q, contentID)
	}
	return dispatchResult{status: "processed"}
}

// undoAnnounce drops the share ledger row; only a real delete decrements.
func (s *Service) undoAnnounce(ctx context.Context, q *gen.Queries, targetID string, signer *gen.RemoteActor) dispatchResult {
	contentID, ok := s.contentIDForObject(targetID)
	if !ok {
		// M5: an unboost of a stored remote post reverses the same way.
		return s.unshareRemotePost(ctx, q, targetID, signer)
	}
	n, err := q.DeleteRemoteShare(ctx, gen.DeleteRemoteShareParams{
		RemoteActorID: signer.ID,
		ContentID:     contentID,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "share delete: " + err.Error()}
	}
	if n > 0 {
		s.bumpCounter(q.DecrementContentShareCount(ctx, contentID))
	}
	return dispatchResult{status: "processed"}
}

// undoCreate tombstones a remote comment by its remote_uri — only when
// the signer is the actor that created it (provenance check mirrors
// dispatchDelete).
func (s *Service) undoCreate(ctx context.Context, q *gen.Queries, objectID string, signer *gen.RemoteActor) dispatchResult {
	c, err := q.GetAtprotoCommentByRemoteURI(ctx, &objectID)
	if err != nil || !c.ContentID.Valid {
		// M5: an undo of a stored remote post tombstones it.
		return s.tombstoneRemotePost(ctx, q, objectID, signer)
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
	return s.remoteReaction(ctx, q, act, signer, "like",
		"liked your post on the fediverse", "like replayed")
}

// dispatchEmojiReact is the Misskey-family reaction verb (§126 M5): the
// activity's `content` emoji maps through reaction_types.emoji → key;
// unknown glyphs and tag-carried custom emoji fall back to 'like' (a
// remote custom emoji can't render here anyway — it still counts as
// appreciation rather than dropping on the floor).
func (s *Service) dispatchEmojiReact(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned emojireact"}
	}
	return s.remoteReaction(ctx, q, act, signer, s.emojiReactionKey(ctx, q, apStr(act["content"])),
		"reacted to your post on the fediverse", "emojireact replayed")
}

// emojiReactionKey resolves an EmojiReact content glyph to a seeded
// reaction_types key — deterministic, so Undo{EmojiReact} maps to the
// same row the insert wrote.
func (s *Service) emojiReactionKey(ctx context.Context, q *gen.Queries, content string) string {
	if content != "" {
		if key, err := q.GetReactionKeyByEmoji(ctx, content); err == nil && key != "" {
			return key
		}
	}
	return "like"
}

// remoteReaction is the shared Like/EmojiReact insert path — one
// reaction row per (entity, remote actor, key), deduped by the partial
// unique.
func (s *Service) remoteReaction(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor, reactionKey, verb, replayReason string) dispatchResult {
	content, ok := s.localContentForObject(ctx, q, apObjectID(act))
	if !ok {
		return dispatchResult{status: "ignored", reason: "reaction target is not a local object"}
	}
	n, err := q.AddRemoteReaction(ctx, gen.AddRemoteReactionParams{
		EntityType: "content", EntityID: content.ID,
		RemoteActorID: signer.ID, ReactionKey: reactionKey,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "reaction insert: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: replayReason}
	}
	s.refreshReactionCount(ctx, q, content.ID)
	if in := s.contentIntent(ctx, q, content.ID, *signer, notifications.TypeContentReaction,
		"Fediverse reaction", verb); in != nil {
		return dispatchResult{status: "processed", intents: []notifyIntent{*in}}
	}
	return dispatchResult{status: "processed"}
}

// dispatchAnnounce lands a remote boost: the shares ledger gates the
// share_count bump so replays and double-boosts can't drift the counter.
// A pre-resolved announceIngest takes the tag-discovery ingest arm
// first — relay Announce{Note} is how Follow{Hashtag} deliveries arrive.
func (s *Service) dispatchAnnounce(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor, announced *announceIngest) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned announce"}
	}
	content, ok := s.localContentForObject(ctx, q, apObjectID(act))
	if !ok {
		// §126 tag discovery — a relay Announce{Note} ingests under the
		// same warrant as Create, authored by the resolved actor (the
		// signer is the relay). The boost then ledgers via the share arm.
		if announced != nil {
			res := s.storeRemotePost(ctx, q, announced.obj, announced.objectID,
				apID(announced.obj["inReplyTo"]), "", announced.author)
			if res.status == "error" || res.status == "rejected" {
				return res
			}
			// "processed" → the post exists for the share arm; "ignored"
			// → the warrant raced away — the arm ignores likewise.
		}
		// M5: a boost of a stored remote post counts too — the
		// remote-post share ledger mirrors federation_remote_shares.
		return s.shareRemotePost(ctx, q, act, signer)
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
	remoteURI := apStr(obj["id"])
	if remoteURI == "" {
		return dispatchResult{status: "rejected", reason: "remote object missing id"}
	}
	// Object ids must live on the signer's domain — an actor cannot mint
	// remote_uri values on our host (squatting on local object URLs) or
	// another instance's (shadowing someone else's statuses).
	ru, rerr := url.Parse(remoteURI)
	su, serr := url.Parse(signer.ActorUrl)
	if rerr != nil || serr != nil || ru.Host == "" ||
		!strings.EqualFold(ru.Hostname(), su.Hostname()) {
		return dispatchResult{status: "rejected", reason: "object id outside actor domain"}
	}
	body := htmlToText(apStr(obj["content"]))
	if body == "" {
		return dispatchResult{status: "rejected", reason: "empty reply text"}
	}
	if len(body) > maxRemoteCommentText {
		return dispatchResult{status: "rejected", reason: "reply text too large"}
	}

	// inReplyTo resolves either to a local object URL (top-level reply)
	// or to a remote comment already ingested (nested reply). Everything
	// else — top-level posts, replies into remote threads — is timeline
	// material, gated on a member actually following the author (M5).
	var contentID pgtype.UUID
	var parentID pgtype.UUID
	depth := int32(0)
	isLocalReply := false
	if replyTo != "" {
		if content, ok := s.localContentForObject(ctx, q, replyTo); ok {
			contentID = content.ID
			isLocalReply = true
		} else if pc, perr := q.GetAtprotoCommentByRemoteURI(ctx, &replyTo); perr == nil && !pc.DeletedAt.Valid {
			contentID = pc.ContentID
			if pc.Depth < int32(maxCommentDepth) {
				parentID = pc.ID
				depth = pc.Depth + 1
			}
			isLocalReply = true
		}
	}
	if !isLocalReply {
		return s.storeRemotePost(ctx, q, obj, remoteURI, replyTo, body, signer)
	}

	// §49: a locked or deleted thread takes no new comments — the remote
	// path must honor the same invariant member replies do.
	if lite, lerr := q.GetContentLiteByID(ctx, contentID); lerr == nil &&
		(lite.IsLocked || lite.DeletedAt.Valid) {
		return dispatchResult{status: "ignored", reason: "comments closed"}
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
// URI) — only when the signer is the actor that created it. An actor
// self-update (Update{Person} — profile edits federate this way) kicks
// a low-tier resolve_actor task so the remote_actors row re-fetches its
// display fields, inbox endpoints, and key instead of waiting out the
// resolver cache TTL.
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
		if err := s.enqueueResolveActor(ctx, signer.ActorUrl); err != nil {
			slog.Warn("federation: actor-update resolve enqueue failed", "error", err)
		}
		return dispatchResult{status: "processed"}
	}
	objMap := apObj(obj)
	if objMap == nil {
		return dispatchResult{status: "ignored", reason: "update object is a bare IRI"}
	}
	c, err := q.GetAtprotoCommentByRemoteURI(ctx, &objectID)
	if err != nil || !c.ContentID.Valid {
		// M5: stored remote posts refresh the same way — provenance is
		// bound inside the arm.
		return s.updateRemotePost(ctx, q, objectID, objMap, signer)
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

// dispatchMove applies a verified Move{actor→target} (§126 M4): the
// mover's live edges rebind to the target actor row — federation_follows
// keeps following them at their new inbox, the native follows mirror
// keeps their follower edges/counts — then the old actor tombstones so
// the dead URL stops resolving. Edges that already exist on the target
// (the member was followed from both accounts) stay with the new row;
// the old side's leftovers go 'undone'/delete as duplicates. Reactions,
// shares and comments keep the OLD actor's attribution — that content
// did come from the old account. Replays are idempotent: a second Move
// finds no rows left to repoint.
func (s *Service) dispatchMove(ctx context.Context, q *gen.Queries, signer, moveTarget *gen.RemoteActor, moveReject string) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned move"}
	}
	if moveReject != "" {
		return dispatchResult{status: "rejected", reason: moveReject}
	}
	if moveTarget == nil || moveTarget.ID == signer.ID {
		return dispatchResult{status: "ignored", reason: "move target did not verify"}
	}
	// The target row may pre-exist as tombstone/blocked — repointing live
	// edges onto a non-active actor would strand followers on a dead row
	// (inbox expansion filters ra.status='active'). Only an active target
	// may absorb migrated edges; anything else rejects.
	if moveTarget.Status != "active" {
		return dispatchResult{status: "rejected", reason: "move target is " + moveTarget.Status}
	}
	if err := q.SetRemoteActorStatus(ctx, gen.SetRemoteActorStatusParams{
		ID: signer.ID, Status: "tombstone",
	}); err != nil {
		return dispatchResult{status: "error", reason: "moved-actor tombstone: " + err.Error()}
	}
	// Colliding pairs merge FIRST — the live status wins (a stale
	// 'undone' on the new account must not bury a live follow from the
	// old); then non-colliding edges repoint; then redundant leftovers
	// delete. Order matters — repoint before merge would collide.
	if _, err := q.MergeFederationFollowsToRemoteActor(ctx, gen.MergeFederationFollowsToRemoteActorParams{
		OldRemoteActorID: signer.ID, NewRemoteActorID: moveTarget.ID,
	}); err != nil {
		return dispatchResult{status: "error", reason: "follow merge: " + err.Error()}
	}
	if _, err := q.RepointFederationFollowsToRemoteActor(ctx, gen.RepointFederationFollowsToRemoteActorParams{
		OldRemoteActorID: signer.ID, NewRemoteActorID: moveTarget.ID,
	}); err != nil {
		return dispatchResult{status: "error", reason: "follow repoint: " + err.Error()}
	}
	if err := q.DeleteFederationFollowsByRemoteActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: move leftover edge cleanup failed", "error", err)
	}
	// The old inbox is dead — pending/retrying deliveries to it can never
	// succeed, so deaden now instead of burning the retry schedule.
	if _, err := q.DeadenDeliveriesByRemoteActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: move delivery deaden failed", "error", err)
	}
	if _, err := q.RepointRemoteFollowsByActor(ctx, gen.RepointRemoteFollowsByActorParams{
		OldRemoteActorID: signer.ID, NewRemoteActorID: moveTarget.ID,
	}); err != nil {
		return dispatchResult{status: "error", reason: "follows mirror repoint: " + err.Error()}
	}
	// M5: members' follows OF the mover ride the migration too — outbound
	// edges repointed above keep their remote_actor mirrors pointing at a
	// live row; dup leftovers (member already followed the new account)
	// delete like the inbound side.
	if _, err := q.RepointOutboundFollowMirrorsByActor(ctx, gen.RepointOutboundFollowMirrorsByActorParams{
		OldRemoteActorID: signer.ID, NewRemoteActorID: moveTarget.ID,
	}); err != nil {
		return dispatchResult{status: "error", reason: "outbound mirror repoint: " + err.Error()}
	}
	if err := q.DeleteOutboundFollowMirrorsByActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: move outbound mirror cleanup failed", "error", err)
	}
	// M5 auto-refollow: the new instance never saw our members' Follows,
	// so the repointed outbound edges are pending+id-less — each gets a
	// fresh Follow activity and a ledgered delivery to the target's
	// inbox. Without this the member's timeline silently dries up the
	// moment the author migrates (Mastodon's MoveWorker semantics).
	res := dispatchResult{status: "processed"}
	moveInbox := moveTarget.InboxUrl
	if moveTarget.SharedInboxUrl != nil && *moveTarget.SharedInboxUrl != "" {
		moveInbox = moveTarget.SharedInboxUrl
	}
	if moveInbox != nil && *moveInbox != "" {
		edges, rerr := q.ListOutboundFollowsForRefollow(ctx, moveTarget.ID)
		if rerr != nil {
			return dispatchResult{status: "error", reason: "refollow list: " + rerr.Error()}
		}
		for _, e := range edges {
			followID := e.ActorUrl + "#follow/" + uuid.NewString()
			if err := q.RearmOutboundFollow(ctx, gen.RearmOutboundFollowParams{
				ID: e.ID, ApActivityID: &followID,
			}); err != nil {
				return dispatchResult{status: "error", reason: "refollow arm: " + err.Error()}
			}
			payload, _ := json.Marshal(map[string]any{
				"@context": asContext,
				"id":       followID,
				"type":     "Follow",
				"actor":    e.ActorUrl,
				"object":   moveTarget.ActorUrl,
				"to":       []string{moveTarget.ActorUrl},
			})
			row, derr := q.InsertFederationDelivery(ctx, gen.InsertFederationDeliveryParams{
				ActivityApID:  followID,
				ActivityType:  "Follow",
				ActivityJson:  payload,
				LocalActorID:  e.LocalActorID,
				RemoteActorID: moveTarget.ID,
				InboxUrl:      *moveInbox,
			})
			if derr != nil && !errors.Is(derr, pgx.ErrNoRows) {
				return dispatchResult{status: "error", reason: "refollow ledger: " + derr.Error()}
			}
			if derr == nil {
				res.deliverIDs = append(res.deliverIDs, row.ID)
			}
		}
	}
	// Deleted mirrors are follower edges that MERGED into the new
	// actor's — each was counted at its own Follow, so each delete
	// unwinds users.follower_count by one.
	if entities, err := q.DeleteDuplicateRemoteFollowsByActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: move duplicate mirror cleanup failed", "error", err)
	} else {
		for _, entityID := range entities {
			s.bumpCounter(q.AdjustUserFollowerCount(ctx, gen.AdjustUserFollowerCountParams{
				UserID: entityID, Delta: -1,
			}))
		}
	}
	// The old URL's relay subscription is as dead as the actor — fail it
	// the way Delete{actor} does instead of letting deliveries retry to
	// exhaustion (re-subscribing under the new URL is the remote's job).
	if err := q.SetFederationRelayStatusByActorURL(ctx, gen.SetFederationRelayStatusByActorURLParams{
		ActorUrl: signer.ActorUrl, Status: "failed",
	}); err != nil {
		slog.Warn("federation: moved-actor relay unwind failed", "error", err)
	}
	res.invalidateRemote = signer.ActorUrl
	return res
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
		// M5: a stored remote post tombstones the same way.
		return s.tombstoneRemotePost(ctx, q, objectID, signer)
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
	res := dispatchResult{status: "processed", invalidateRemote: signer.ActorUrl}
	// A dead relay actor kills the subscription too — fanout must stop
	// targeting it (re-subscribe re-arms via UpsertFederationRelay).
	if err := q.SetFederationRelayStatusByActorURL(ctx, gen.SetFederationRelayStatusByActorURLParams{
		ActorUrl: signer.ActorUrl, Status: "failed",
	}); err != nil {
		slog.Warn("federation: relay unwind failed", "error", err)
	}
	// Same tombstone consequence: nothing we ever queued to that inbox
	// can still land — deaden pending/retrying deliveries immediately.
	if _, err := q.DeadenDeliveriesByRemoteActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: delivery deaden failed", "error", err)
	}
	if err := q.UndoFederationFollowsByRemoteActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: follow unwind failed", "error", err)
	}
	// M5: members' follows OF the dead actor die with it — outbound edges
	// went 'undone' above; their remote_actor mirrors delete here.
	if err := q.DeleteOutboundFollowMirrorsByActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: outbound mirror unwind failed", "error", err)
	}
	// M5: the dead actor's timeline posts tombstone too — the actor row
	// itself only goes 'tombstone' so the FK cascade never fires. Their
	// share rows on OTHER actors' posts unwind the same way, and each
	// boosted post's share_count drops with the deleted rows.
	if err := q.TombstoneFederationRemotePostsByActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: remote post tombstone failed", "error", err)
	}
	if postIDs, err := q.DeleteRemotePostSharesByActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: remote post share unwind failed", "error", err)
	} else {
		seen := map[pgtype.UUID]bool{}
		for _, pid := range postIDs {
			if seen[pid] {
				continue
			}
			seen[pid] = true
			s.bumpCounter(q.AdjustRemotePostShareCount(ctx, gen.AdjustRemotePostShareCountParams{
				ID: pid, Delta: -1,
			}))
		}
	}
	if entities, err := q.DeleteRemoteFollowsByActor(ctx, signer.ID); err != nil {
		slog.Warn("federation: follows unwind failed", "error", err)
	} else {
		for _, entityID := range entities {
			s.bumpCounter(q.AdjustUserFollowerCount(ctx, gen.AdjustUserFollowerCountParams{
				UserID: entityID, Delta: -1,
			}))
		}
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
	return res
}

// localContentForObject resolves a local object URL
// ({api}/federation/objects/contents/{uuid}) to a live, federatable
// content row — Likes/Announces/replies against unpublished, private,
// deleted, or opted-out content are ignored, not counted. The author's
// federated actor must also be active: a disabled/tombstoned member's
// objects answer 410, so engagement landing (and notifying) on them
// after the Delete{actor} fanout would be incoherent.
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
	actor, aerr := q.GetFederatedActorByUserID(ctx, row.AuthorID)
	if aerr != nil || actor.Status != ActorStatusActive {
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
